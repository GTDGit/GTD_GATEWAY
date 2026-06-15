package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/cache"
	"github.com/GTDGit/gtd_gateway/internal/config"
	"github.com/GTDGit/gtd_gateway/internal/database"
	"github.com/GTDGit/gtd_gateway/internal/handler"
	"github.com/GTDGit/gtd_gateway/internal/middleware"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/sse"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// main is the application entrypoint for the GTD API Gateway (admin read/view mode).
func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Setup logger
	setupLogger(cfg.Env)
	log.Info().Str("env", cfg.Env).Msg("starting gtd api")
	utils.SetJWTSecret(cfg.JWTSecret)

	// 3. Connect database
	db, err := database.Connect(&cfg.DB)
	if err != nil {
		log.Error().Err(err).Msg("database connection failed")
		fmt.Fprintf(os.Stderr, "database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 3b. Connect to Redis
	redisClient, err := cache.NewRedisClient(&cfg.Redis)
	if err != nil {
		log.Error().Err(err).Msg("redis connection failed")
		fmt.Fprintf(os.Stderr, "redis connection failed: %v\n", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	log.Info().Msg("redis connected successfully")

	// 4. Initialize repositories
	clientRepo := repository.NewClientRepository(db)
	productRepo := repository.NewProductRepository(db)
	skuRepo := repository.NewSKURepository(db)
	trxRepo := repository.NewTransactionRepository(db)
	cbRepo := repository.NewCallbackRepository(db)
	adminRepo := repository.NewAdminUserRepository(db)
	bankCodeRepo := repository.NewBankCodeRepository(db)
	payoutRepo := repository.NewPayoutRepository(db)
	ppobProviderRepo := repository.NewPPOBProviderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	productMasterRepo := repository.NewProductMasterRepository(db)
	reconRepo := repository.NewReconciliationRepository(db)
	qrisMerchantRepo := repository.NewQRISMerchantRepository(db)
	qrisPaymentRepo := repository.NewQRISPaymentRepository(db)
	qrisDocRepo := repository.NewQRISDocRepository(db)

	// 5. Initialize services (no provider clients — admin read/view only)
	adminAuthSvc := service.NewAdminAuthService(adminRepo)
	clientSvc := service.NewClientService(clientRepo)
	productSvc := service.NewProductService(productRepo, skuRepo)
	productMasterSvc := service.NewProductMasterService(productMasterRepo)
	productMgmtSvc := service.NewProductManagementService(productRepo, skuRepo, productMasterSvc)
	callbackSvc := service.NewCallbackService(clientRepo, cbRepo, trxRepo)

	// trxSvc: nil digiflazz clients + nil providerRouter + nil inquiryCache.
	// ManualRetry on admin routes will gracefully fail at runtime without live providers.
	trxSvc := service.NewTransactionService(trxRepo, productRepo, skuRepo, cbRepo, nil, nil, productSvc, callbackSvc, nil)

	// Wire callback service to transaction service for in-process retry on webhook.
	callbackSvc.SetTransactionRetrier(trxSvc)

	// SSE hub and in-process notifier for admin-triggered events.
	sseHub := sse.NewHub()
	sseNotifier := sse.NewHubNotifier(sseHub)
	trxSvc.SetNotifier(sseNotifier)
	callbackSvc.SetNotifier(sseNotifier)

	adminTrxSvc := service.NewAdminTransactionService(trxRepo, cbRepo, productSvc, trxSvc, callbackSvc)

	// paymentCallbackSvc and paymentSvc: nil paymentRouter.
	// AdminPaymentService needs paymentSvc for refund (will fail gracefully without provider).
	paymentCallbackSvc := service.NewPaymentCallbackService(paymentRepo, clientRepo)
	paymentSvc := service.NewPaymentService(paymentRepo, clientRepo, nil, paymentCallbackSvc)
	paymentSvc.SetNotifier(sseNotifier)
	adminPaymentSvc := service.NewAdminPaymentService(paymentRepo, clientRepo, paymentSvc, paymentCallbackSvc)
	adminReconciliationSvc := service.NewAdminReconciliationService(reconRepo, paymentRepo, paymentSvc)

	// adminPayoutSvc only needs payoutRepo for listing/viewing + route management.
	adminPayoutSvc := service.NewAdminPayoutService(payoutRepo)

	// QRIS: gateway owns merchant CRUD + payment views (read from the shared DB).
	// Nobu onboarding (registration intake, Excel batches, activation + QR
	// generation, client webhooks) lives in the api service; admin requests for
	// those are proxied to api with the caller's admin JWT (shared JWT_SECRET).
	apiAdminProxy := service.NewAPIAdminProxy(cfg.APIInternalURL)
	qrisSvc := service.NewQRISService(qrisMerchantRepo, qrisPaymentRepo)

	// QRIS document portal: upload to private S3 + token-gated shareable links.
	qrisDocSvc := service.NewQRISDocService(qrisDocRepo, cfg.FilesBaseURL)

	// 6. Initialize handlers (admin + health only)
	handlers := &Handlers{
		Health:            handler.NewHealthHandler(nil),
		Client:            handler.NewClientHandler(clientSvc),
		ProductManagement: handler.NewProductManagementHandler(productMgmtSvc),
		ProductMaster:     handler.NewProductMasterHandler(productMasterSvc),
		AdminTransaction:  handler.NewAdminTransactionHandler(adminTrxSvc),
		Auth:              handler.NewAuthHandler(adminAuthSvc),
		BankCode:          handler.NewBankCodeHandler(bankCodeRepo),
		PPOBProvider:      handler.NewPPOBProviderHandler(ppobProviderRepo),
		SSE:               handler.NewSSEHandler(sseHub),
		AdminPayment:      handler.NewAdminPaymentHandler(adminPaymentSvc),
		AdminReconciliation: handler.NewAdminReconciliationHandler(adminReconciliationSvc),
		AdminPayout:       handler.NewAdminPayoutHandler(adminPayoutSvc),
		QRIS:              handler.NewQRISHandler(qrisSvc, apiAdminProxy),
		QRISDoc:           handler.NewQRISDocHandler(qrisDocSvc),
	}

	// 7. Initialize middleware (admin uses JWT only)
	jwtMw := middleware.NewJWTMiddleware()

	// 8. Setup router
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.LoggingMiddleware())
	setupRoutes(router, handlers, jwtMw)

	// 9. Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 9a. Start Redis subscriber to bridge API-originated domain events into
	// the admin SSE hub. Admin-triggered events are delivered locally via sseNotifier.
	subscriber := sse.NewRedisSubscriber(redisClient.Raw(), sseHub)
	go subscriber.Start(ctx)

	// 10. Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Info().Str("port", cfg.Port).Msg("Starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// 11. Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// 12. Cancel context to stop workers
	cancel()

	// 13. Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}
	log.Info().Msg("Server exited")
}

// Handlers groups all HTTP handlers used by the server (admin + health only).
type Handlers struct {
	Health            *handler.HealthHandler
	Client            *handler.ClientHandler
	ProductManagement *handler.ProductManagementHandler
	ProductMaster     *handler.ProductMasterHandler
	AdminTransaction  *handler.AdminTransactionHandler
	Auth              *handler.AuthHandler
	BankCode          *handler.BankCodeHandler
	PPOBProvider      *handler.PPOBProviderHandler
	SSE               *handler.SSEHandler
	AdminPayment      *handler.AdminPaymentHandler
	AdminReconciliation *handler.AdminReconciliationHandler
	AdminPayout       *handler.AdminPayoutHandler
	QRIS              *handler.QRISHandler
	QRISDoc           *handler.QRISDocHandler
}

// setupRoutes registers the admin route group and the health endpoint only.
func setupRoutes(router *gin.Engine, handlers *Handlers, jwtMiddleware *middleware.JWTMiddleware) {
	router.GET("/v1/health", handlers.Health.GetHealth)

	// Admin routes
	admin := router.Group("/v1/admin")
	admin.POST("/auth/login", handlers.Auth.Login)
	admin.GET("/sse", handlers.SSE.Stream) // JWT via ?token= query param (EventSource can't set headers)
	admin.Use(jwtMiddleware.Handle())
	{
		// Client Management
		admin.POST("/clients", handlers.Client.CreateClient)
		admin.GET("/clients", handlers.Client.ListClients)
		admin.GET("/clients/:id", handlers.Client.GetClient)
		admin.GET("/clients/by-client-id/:client_id", handlers.Client.GetClientByClientID)
		admin.PUT("/clients/:id", handlers.Client.UpdateClient)
		admin.POST("/clients/:id/regenerate", handlers.Client.RegenerateKeys)

		// Product Management
		admin.GET("/products", handlers.ProductManagement.ListProducts)
		admin.GET("/products/categories", handlers.ProductManagement.GetCategories)
		admin.GET("/products/brands", handlers.ProductManagement.GetBrands)
		admin.GET("/products/variants", handlers.ProductManagement.GetVariants)
		admin.POST("/products", handlers.ProductManagement.CreateProduct)
		admin.GET("/products/:id", handlers.ProductManagement.GetProduct)
		admin.PUT("/products/:id", handlers.ProductManagement.UpdateProduct)
		admin.DELETE("/products/:id", handlers.ProductManagement.DeleteProduct)

		// Product Master (categories, brands, variants) CRUD
		admin.GET("/product-master/categories", handlers.ProductMaster.ListCategories)
		admin.POST("/product-master/categories", handlers.ProductMaster.CreateCategory)
		admin.PUT("/product-master/categories/:id", handlers.ProductMaster.UpdateCategory)
		admin.DELETE("/product-master/categories/:id", handlers.ProductMaster.DeleteCategory)
		admin.GET("/product-master/brands", handlers.ProductMaster.ListBrands)
		admin.POST("/product-master/brands", handlers.ProductMaster.CreateBrand)
		admin.PUT("/product-master/brands/:id", handlers.ProductMaster.UpdateBrand)
		admin.DELETE("/product-master/brands/:id", handlers.ProductMaster.DeleteBrand)
		admin.GET("/product-master/variants", handlers.ProductMaster.ListVariants)
		admin.POST("/product-master/variants", handlers.ProductMaster.CreateVariant)
		admin.PUT("/product-master/variants/:id", handlers.ProductMaster.UpdateVariant)
		admin.DELETE("/product-master/variants/:id", handlers.ProductMaster.DeleteVariant)

		// SKU Management
		admin.POST("/products/:id/skus", handlers.ProductManagement.CreateSKU)
		admin.GET("/products/:id/skus", handlers.ProductManagement.GetProductSKUs)
		admin.GET("/skus/:id", handlers.ProductManagement.GetSKU)
		admin.PUT("/skus/:id", handlers.ProductManagement.UpdateSKU)
		admin.DELETE("/skus/:id", handlers.ProductManagement.DeleteSKU)

		// Transaction Management (Admin)
		admin.GET("/transactions", handlers.AdminTransaction.ListTransactions)
		admin.GET("/transactions/stats", handlers.AdminTransaction.GetStats)
		admin.GET("/transactions/:id", handlers.AdminTransaction.GetTransaction)
		admin.GET("/transactions/:id/logs", handlers.AdminTransaction.GetTransactionLogs)
		admin.POST("/transactions/:id/retry", handlers.AdminTransaction.ManualRetry)

		// PPOB Provider Management
		admin.GET("/ppob/providers", handlers.PPOBProvider.ListProviders)
		admin.GET("/ppob/providers/:id", handlers.PPOBProvider.GetProvider)
		admin.PUT("/ppob/providers/:id/status", handlers.PPOBProvider.UpdateProviderStatus)

		// PPOB Provider SKU Management
		admin.GET("/ppob/providers/:id/skus", handlers.PPOBProvider.ListProviderSKUs)
		admin.POST("/ppob/providers/:id/skus", handlers.PPOBProvider.CreateProviderSKU)
		admin.GET("/ppob/products/:productId/provider-skus", handlers.PPOBProvider.GetProviderSKUsByProduct)
		admin.PUT("/ppob/skus/:id", handlers.PPOBProvider.UpdateProviderSKU)
		admin.DELETE("/ppob/skus/:id", handlers.PPOBProvider.DeleteProviderSKU)

		// PPOB Provider Health
		admin.GET("/ppob/health", handlers.PPOBProvider.GetAllProviderHealthToday)
		admin.GET("/ppob/providers/:id/health", handlers.PPOBProvider.GetProviderHealth)

		// Payment admin
		admin.GET("/payments", handlers.AdminPayment.ListPayments)
		admin.GET("/payments/stats", handlers.AdminPayment.Stats)
		admin.GET("/payments/:id", handlers.AdminPayment.GetPayment)
		admin.GET("/payments/:id/logs", handlers.AdminPayment.GetPaymentLogs)
		admin.GET("/payments/:id/callbacks", handlers.AdminPayment.GetPaymentCallbacks)
		admin.GET("/payments/:id/callback-logs", handlers.AdminPayment.ListCallbackLogs)
		admin.POST("/payments/:id/retry-callback", handlers.AdminPayment.RetryCallback)
		admin.POST("/payments/:id/refund", handlers.AdminPayment.Refund)

		// Reconciliation admin (verify-by-inquiry mismatches)
		admin.GET("/reconciliations", handlers.AdminReconciliation.ListReconciliations)
		admin.GET("/reconciliations/:id", handlers.AdminReconciliation.GetReconciliation)
		admin.POST("/reconciliations/:id/resolve", handlers.AdminReconciliation.ResolveReconciliation)

		// Payment method admin
		admin.GET("/payment-methods", handlers.AdminPayment.ListMethods)
		admin.PUT("/payment-methods/:method", handlers.AdminPayment.UpdateMethod)
		admin.GET("/payment-methods/:method/:code/providers", handlers.AdminPayment.ListProviders)
		admin.PUT("/payment-methods/:method/:code/providers", handlers.AdminPayment.UpdateProviders)

		// Disbursement payout admin
		admin.GET("/payouts", handlers.AdminPayout.ListPayouts)
		admin.GET("/payouts/stats", handlers.AdminPayout.Stats)
		admin.GET("/payouts/routes", handlers.AdminPayout.ListRoutes)
		admin.PUT("/payouts/routes/:id", handlers.AdminPayout.UpdateRoute)
		admin.GET("/payout-methods", handlers.AdminPayout.ListMethods)
		admin.PUT("/payout-methods/:id", handlers.AdminPayout.UpdateMethod)
		admin.GET("/payouts/:id", handlers.AdminPayout.GetPayout)
		admin.GET("/payouts/:id/callbacks", handlers.AdminPayout.ListCallbacks)

		// Bank code admin (controls disbursement bank availability + VA support)
		admin.GET("/bank-codes", handlers.BankCode.AdminListBankCodes)
		admin.PUT("/bank-codes/:id", handlers.BankCode.AdminUpdateBankCode)

		// Static QRIS merchant management + payment views
		admin.GET("/qris/merchants", handlers.QRIS.ListMerchants)
		admin.POST("/qris/merchants", handlers.QRIS.CreateMerchant)
		admin.GET("/qris/merchants/:id", handlers.QRIS.GetMerchant)
		admin.PUT("/qris/merchants/:id", handlers.QRIS.UpdateMerchant)
		admin.GET("/qris/payments", handlers.QRIS.ListPayments)

		// Nobu onboarding — proxied to the api service (which owns the Nobu
		// generate client + client webhooks + rendered Excel batch files).
		admin.GET("/qris/registrations", handlers.QRIS.ListRegistrations)
		admin.POST("/qris/registrations/:id/activate", handlers.QRIS.ActivateRegistration)
		admin.GET("/qris/batches", handlers.QRIS.ListBatches)
		admin.GET("/qris/batches/:id/download", handlers.QRIS.DownloadBatch)
		admin.POST("/qris/batches/:id/sent", handlers.QRIS.MarkBatchSent)

		// File portal links (read-only): list/inspect bundles + force-close.
		// Upload lives in the standalone portal (dev-files.gtd.co.id).
		admin.GET("/qris/documents", handlers.QRISDoc.ListBundles)
		admin.GET("/qris/documents/:token", handlers.QRISDoc.GetBundle)
		admin.POST("/qris/documents/:token/revoke", handlers.QRISDoc.RevokeBundle)
	}
}

// runMigrations removed: the Gateway does not run schema migrations.

func setupLogger(env string) {
	if env == "production" {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}
