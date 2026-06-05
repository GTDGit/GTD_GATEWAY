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
	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/sse"
	"github.com/GTDGit/gtd_gateway/internal/utils"
	"github.com/GTDGit/gtd_gateway/pkg/alterra"
	"github.com/GTDGit/gtd_gateway/pkg/bnc"
	"github.com/GTDGit/gtd_gateway/pkg/bri"
	"github.com/GTDGit/gtd_gateway/pkg/dana"
	"github.com/GTDGit/gtd_gateway/pkg/kiosbank"
	"github.com/GTDGit/gtd_gateway/pkg/midtrans"
	"github.com/GTDGit/gtd_gateway/pkg/pakailink"
	"github.com/GTDGit/gtd_gateway/pkg/xendit"
)

// main is the application entrypoint for the GTD API Gateway (Phase 1).
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

	// 3c. Initialize inquiry cache
	inquiryCache := cache.NewInquiryCache(redisClient)

	// 4. Digiflazz disabled - provider soft-deleted; clients passed as nil.
	// TransactionService and HealthHandler nil-guard the Digiflazz client.
	// 5. Initialize repositories
	clientRepo := repository.NewClientRepository(db)
	productRepo := repository.NewProductRepository(db)
	skuRepo := repository.NewSKURepository(db)
	trxRepo := repository.NewTransactionRepository(db)
	cbRepo := repository.NewCallbackRepository(db)
	adminRepo := repository.NewAdminUserRepository(db)
	bankCodeRepo := repository.NewBankCodeRepository(db)
	transferRepo := repository.NewTransferRepository(db)
	ppobProviderRepo := repository.NewPPOBProviderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)

	// 5a. Initialize PPOB provider clients
	kioskbankProdClient, kioskbankDevClient := buildKiosbankClients(cfg.Kiosbank)

	var alterraClient *alterra.Client
	if cfg.Alterra.ClientID != "" && (cfg.Alterra.PrivateKeyPath != "" || cfg.Alterra.PrivateKeyPEM != "") {
		var err error
		alterraClient, err = alterra.NewClient(alterra.Config{
			BaseURL:        cfg.Alterra.BaseURL,
			ClientID:       cfg.Alterra.ClientID,
			PrivateKeyPath: cfg.Alterra.PrivateKeyPath,
			PrivateKeyPEM:  cfg.Alterra.PrivateKeyPEM,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Alterra client initialization failed - provider will be disabled")
		}
	}

	var bncClient *bnc.Client
	if cfg.Disbursement.BNC.ClientID != "" &&
		cfg.Disbursement.BNC.ClientSecret != "" &&
		cfg.Disbursement.BNC.PartnerID != "" &&
		cfg.Disbursement.BNC.ChannelID != "" &&
		cfg.Disbursement.BNC.SourceAccount != "" &&
		cfg.Disbursement.BNC.PrivateKeyPath != "" {
		var err error
		bncClient, err = bnc.NewClient(bnc.Config{
			BaseURL:        cfg.Disbursement.BNC.BaseURL,
			ClientID:       cfg.Disbursement.BNC.ClientID,
			ClientSecret:   cfg.Disbursement.BNC.ClientSecret,
			PartnerID:      cfg.Disbursement.BNC.PartnerID,
			ChannelID:      cfg.Disbursement.BNC.ChannelID,
			SourceAccount:  cfg.Disbursement.BNC.SourceAccount,
			PrivateKeyPath: cfg.Disbursement.BNC.PrivateKeyPath,
		})
		if err != nil {
			log.Warn().Err(err).Msg("BNC disbursement client initialization failed - transfer API will be disabled")
		} else {
			log.Info().Msg("BNC disbursement client registered")
		}
	} else {
		log.Info().Msg("BNC disbursement config incomplete - transfer API will be disabled")
	}

	var briClient *bri.Client
	if cfg.BRI.ClientID != "" && cfg.BRI.ClientSecret != "" {
		var err error
		briClient, err = bri.NewClient(bri.Config{
			BaseURL:        cfg.BRI.BaseURL,
			ClientID:       cfg.BRI.ClientID,
			ClientSecret:   cfg.BRI.ClientSecret,
			PartnerID:      cfg.BRI.PartnerID,
			ChannelID:      cfg.BRI.ChannelID,
			SourceAccount:  cfg.BRI.SourceAccount,
			PrivateKeyPath: cfg.BRI.PrivateKeyPath,
			BRIZZIUsername: cfg.BRI.BRIZZIUsername,
		})
		if err != nil {
			log.Warn().Err(err).Msg("BRI client initialization failed - BRIVA/BRIZZI/transfer BRI will be partially disabled")
		} else {
			log.Info().Msg("BRI client registered")
		}
	} else {
		log.Info().Msg("BRI config incomplete - BRIVA/BRIZZI/transfer BRI will be disabled")
	}

	// 5b. Initialize Payment provider clients (optional per-provider)
	var pakailinkClient *pakailink.Client
	if cfg.Payment.Pakailink.ClientID != "" && cfg.Payment.Pakailink.ClientSecret != "" &&
		(cfg.Payment.Pakailink.PrivateKeyPath != "" || cfg.Payment.Pakailink.PrivateKeyPEM != "") {
		var err error
		pakailinkClient, err = pakailink.NewClient(pakailink.Config{
			BaseURL:        cfg.Payment.Pakailink.BaseURL,
			ClientID:       cfg.Payment.Pakailink.ClientID,
			ClientSecret:   cfg.Payment.Pakailink.ClientSecret,
			PartnerID:      cfg.Payment.Pakailink.PartnerID,
			ChannelID:      cfg.Payment.Pakailink.ChannelID,
			PrivateKeyPath: cfg.Payment.Pakailink.PrivateKeyPath,
			PrivateKeyPEM:  cfg.Payment.Pakailink.PrivateKeyPEM,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Pakailink client initialization failed - VA/QRIS via Pakailink disabled")
			pakailinkClient = nil
		} else {
			log.Info().Msg("Pakailink client registered")
		}
	} else {
		log.Info().Msg("Pakailink config incomplete - VA/QRIS via Pakailink disabled")
	}

	var danaClient *dana.Client
	if cfg.Payment.Dana.ClientID != "" && cfg.Payment.Dana.ClientSecret != "" &&
		cfg.Payment.Dana.MerchantID != "" &&
		(cfg.Payment.Dana.PrivateKeyPath != "" || cfg.Payment.Dana.PrivateKeyPEM != "") {
		var err error
		danaClient, err = dana.NewClient(dana.Config{
			BaseURL:        cfg.Payment.Dana.BaseURL,
			MerchantID:     cfg.Payment.Dana.MerchantID,
			ClientID:       cfg.Payment.Dana.ClientID,
			ClientSecret:   cfg.Payment.Dana.ClientSecret,
			PartnerID:      cfg.Payment.Dana.PartnerID,
			PrivateKeyPath: cfg.Payment.Dana.PrivateKeyPath,
			PrivateKeyPEM:  cfg.Payment.Dana.PrivateKeyPEM,
		})
		if err != nil {
			log.Warn().Err(err).Msg("DANA client initialization failed - DANA e-wallet disabled")
			danaClient = nil
		} else {
			log.Info().Msg("DANA client registered")
		}
	} else {
		log.Info().Msg("DANA config incomplete - DANA e-wallet disabled")
	}

	var midtransClient *midtrans.Client
	if cfg.Payment.Midtrans.ServerKey != "" {
		var err error
		midtransClient, err = midtrans.NewClient(midtrans.Config{
			BaseURL:    cfg.Payment.Midtrans.BaseURL,
			ServerKey:  cfg.Payment.Midtrans.ServerKey,
			ClientKey:  cfg.Payment.Midtrans.ClientKey,
			MerchantID: cfg.Payment.Midtrans.MerchantID,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Midtrans client initialization failed - GoPay/ShopeePay disabled")
			midtransClient = nil
		} else {
			log.Info().Msg("Midtrans client registered")
		}
	} else {
		log.Info().Msg("Midtrans config incomplete - GoPay/ShopeePay disabled")
	}

	var xenditClient *xendit.Client
	if cfg.Payment.Xendit.APIKey != "" {
		var err error
		xenditClient, err = xendit.NewClient(xendit.Config{
			BaseURL:      cfg.Payment.Xendit.BaseURL,
			APIKey:       cfg.Payment.Xendit.APIKey,
			APIVersion:   cfg.Payment.Xendit.APIVersion,
			WebhookToken: cfg.Payment.Xendit.WebhookToken,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Xendit client initialization failed - Indomaret/Alfamart disabled")
			xenditClient = nil
		} else {
			log.Info().Msg("Xendit client registered")
		}
	} else {
		log.Info().Msg("Xendit config incomplete - Indomaret/Alfamart disabled")
	}

	// 6. Initialize services
	adminAuthSvc := service.NewAdminAuthService(adminRepo)
	clientSvc := service.NewClientService(clientRepo)
	productSvc := service.NewProductService(productRepo, skuRepo)
	productMasterRepo := repository.NewProductMasterRepository(db)
	productMasterSvc := service.NewProductMasterService(productMasterRepo)
	productMgmtSvc := service.NewProductManagementService(productRepo, skuRepo, productMasterSvc)
	callbackSvc := service.NewCallbackService(clientRepo, cbRepo, trxRepo)
	trxSvc := service.NewTransactionService(trxRepo, productRepo, skuRepo, cbRepo, nil, nil, productSvc, callbackSvc, inquiryCache)

	// Wire up callback service to transaction service for immediate retry on webhook
	callbackSvc.SetTransactionRetrier(trxSvc)

	// Initialize SSE hub and in-process notifier for admin-triggered events.
	// API-originated events arrive via the Redis subscriber started below.
	sseHub := sse.NewHub()
	sseNotifier := sse.NewHubNotifier(sseHub)
	trxSvc.SetNotifier(sseNotifier)
	callbackSvc.SetNotifier(sseNotifier)

	// Initialize Admin Transaction service
	adminTrxSvc := service.NewAdminTransactionService(trxRepo, cbRepo, productSvc, trxSvc, callbackSvc)

	// Initialize Provider Router for multi-provider PPOB
	providerRouter := service.NewProviderRouter(ppobProviderRepo)
	if kioskbankProdClient != nil {
		kiosbankAdapter := service.NewKiosbankProviderClient(kioskbankProdClient, kioskbankDevClient, trxRepo, cbRepo, ppobProviderRepo)
		providerRouter.RegisterProvider(models.ProviderKiosbank, kiosbankAdapter)
		log.Info().Msg("Kiosbank provider registered")
	}
	if alterraClient != nil {
		alterraAdapter := service.NewAlterraProviderClient(alterraClient, alterraClient) // Same client for prod/dev
		providerRouter.RegisterProvider(models.ProviderAlterra, alterraAdapter)
		log.Info().Msg("Alterra provider registered")
	}
	if briClient != nil && cfg.BRI.BRIZZIUsername != "" {
		briAdapter := service.NewBRIProviderClient(briClient, cfg.BRI.BRIZZIDenominations)
		providerRouter.RegisterProvider(models.ProviderBRI, briAdapter)
		log.Info().Msg("BRI provider registered for BRIZZI")
	}

	// Wire provider router to transaction service for multi-provider support
	trxSvc.SetProviderRouter(providerRouter)
	log.Info().Msg("Provider router connected to transaction service")

	// Update product service with provider-aware version for best price
	productSvc = service.NewProductServiceWithProviders(productRepo, skuRepo, ppobProviderRepo)

	transferCallbackSvc := service.NewTransferCallbackService(clientRepo, bankCodeRepo)
	transferSvc := service.NewTransferService(
		transferRepo,
		bankCodeRepo,
		bncClient,
		briClient,
		transferCallbackSvc,
		cfg.Disbursement.BNC.SourceAccount,
		cfg.BRI.SourceAccount,
	)
	if pakailinkClient != nil && cfg.Disbursement.Pakailink.Enabled {
		transferSvc.SetPakailinkClient(
			service.NewPakailinkTransferAdapter(pakailinkClient, cfg.Disbursement.Pakailink.CallbackURL),
			cfg.Disbursement.Pakailink.SourceLabel,
		)
		log.Info().Msg("PakaiLink disbursement adapter registered (handles all banks)")
	}

	// 6b. Payment module wiring
	paymentRouter := service.NewPaymentProviderRouter()
	if pakailinkClient != nil {
		paymentRouter.Register(service.NewPakailinkProviderClient(pakailinkClient, cfg.Payment.Pakailink.CallbackURL))
		log.Info().Msg("Pakailink payment adapter registered")
	}
	if danaClient != nil {
		paymentRouter.Register(service.NewDanaProviderClient(danaClient, cfg.Payment.Dana.CallbackURL, cfg.Payment.Dana.ReturnURL))
		log.Info().Msg("DANA payment adapter registered")
	}
	if midtransClient != nil {
		paymentRouter.Register(service.NewMidtransProviderClient(midtransClient, cfg.Payment.Midtrans.CallbackURL))
		log.Info().Msg("Midtrans payment adapter registered")
	}
	if xenditClient != nil {
		paymentRouter.Register(service.NewXenditProviderClient(xenditClient))
		log.Info().Msg("Xendit payment adapter registered")
	}

	paymentCallbackSvc := service.NewPaymentCallbackService(paymentRepo, clientRepo)
	paymentSvc := service.NewPaymentService(paymentRepo, clientRepo, paymentRouter, paymentCallbackSvc)
	paymentSvc.SetNotifier(sseNotifier)
	adminPaymentSvc := service.NewAdminPaymentService(paymentRepo, clientRepo, paymentSvc, paymentCallbackSvc)
	adminTransferSvc := service.NewAdminTransferService(transferRepo)

	// 7. Initialize handlers (admin + health only)
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
		AdminTransfer:     handler.NewAdminTransferHandler(adminTransferSvc),
	}

	// 8. Initialize middleware (admin uses JWT only)
	jwtMw := middleware.NewJWTMiddleware()

	// 9. Setup router
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.LoggingMiddleware())
	setupRoutes(router, handlers, jwtMw)

	// 10. Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 10a. Start Redis subscriber to bridge API-originated domain events into
	// the admin SSE hub (Req 7.3, 7.4, 7.6). Admin-triggered events are
	// delivered locally via sseNotifier above.
	subscriber := sse.NewRedisSubscriber(redisClient.Raw(), sseHub)
	go subscriber.Start(ctx)

	// 11. No background workers (Req 2.5) - the Gateway runs admin + health only.

	// 12. Start HTTP server
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

	// 13. Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// 14. Cancel context to stop workers
	cancel()

	// 15. Shutdown HTTP server with timeout
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
	AdminTransfer     *handler.AdminTransferHandler
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
		admin.GET("/payments/:id/refunds", handlers.AdminPayment.ListRefunds)
		admin.POST("/payments/:id/retry-callback", handlers.AdminPayment.RetryCallback)
		admin.POST("/payments/:id/refund", handlers.AdminPayment.Refund)

		// Payment method admin
		admin.GET("/payment-methods", handlers.AdminPayment.ListMethods)
		admin.PUT("/payment-methods/:id", handlers.AdminPayment.UpdateMethod)

		// Disbursement transfer admin
		admin.GET("/transfers", handlers.AdminTransfer.ListTransfers)
		admin.GET("/transfers/stats", handlers.AdminTransfer.Stats)
		admin.GET("/transfers/:id", handlers.AdminTransfer.GetTransfer)
		admin.GET("/transfers/:id/callbacks", handlers.AdminTransfer.ListCallbacks)

		// Bank code admin (controls disbursement bank availability + VA support)
		admin.GET("/bank-codes", handlers.BankCode.AdminListBankCodes)
		admin.PUT("/bank-codes/:id", handlers.BankCode.AdminUpdateBankCode)
	}
}

func buildKiosbankClients(cfg config.KiosbankConfig) (*kiosbank.Client, *kiosbank.Client) {
	if cfg.Username == "" {
		return nil, nil
	}
	if cfg.MerchantName == "" {
		log.Warn().Msg("KIOSBANK_MERCHANT_NAME is empty; falling back to KIOSBANK_MERCHANT_ID for sign-on")
	}
	if cfg.DevelopmentURL != "" && cfg.DevelopmentCreds.MerchantName == "" {
		log.Warn().Msg("KIOSBANK_DEV_MERCHANT_NAME is empty; falling back to development merchant ID for sign-on")
	}

	prodClient := kiosbank.NewClient(kiosbankClientConfig(cfg, false))
	devClient := kiosbank.NewClient(kiosbankClientConfig(cfg, true))

	return prodClient, devClient
}

func kiosbankClientConfig(cfg config.KiosbankConfig, development bool) kiosbank.Config {
	if !development {
		merchantName := cfg.MerchantName
		if merchantName == "" {
			merchantName = cfg.MerchantID
		}
		return kiosbank.Config{
			BaseURL:            cfg.BaseURL,
			MerchantID:         cfg.MerchantID,
			MerchantName:       merchantName,
			CounterID:          cfg.CounterID,
			AccountID:          cfg.AccountID,
			Mitra:              cfg.Mitra,
			Username:           cfg.Username,
			Password:           cfg.Password,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
	}

	devCreds := cfg.DevelopmentCreds
	if devCreds.Username == "" {
		devCreds.Username = cfg.Username
		devCreds.Password = cfg.Password
		devCreds.MerchantID = cfg.MerchantID
		devCreds.MerchantName = cfg.MerchantName
		devCreds.CounterID = cfg.CounterID
		devCreds.AccountID = cfg.AccountID
		devCreds.Mitra = cfg.Mitra
	}
	if devCreds.MerchantName == "" {
		devCreds.MerchantName = devCreds.MerchantID
	}

	return kiosbank.Config{
		BaseURL:            cfg.DevelopmentURL,
		MerchantID:         devCreds.MerchantID,
		MerchantName:       devCreds.MerchantName,
		CounterID:          devCreds.CounterID,
		AccountID:          devCreds.AccountID,
		Mitra:              devCreds.Mitra,
		Username:           devCreds.Username,
		Password:           devCreds.Password,
		InsecureSkipVerify: cfg.DevelopmentInsecureSkipVerify,
	}
}

// runMigrations removed: the Gateway does not run schema migrations (Req 8.2, 8.3).

func setupLogger(env string) {
	if env == "production" {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}
