# GTD API Gateway - API Client

## Overview

Indonesian fintech API gateway for PPOB (Payment Points of Business), Payment, Disbursement, and Identity Verification services. Built with Go + Gin framework, PostgreSQL, and Redis.

Module: `github.com/GTDGit/gtd_api`

## Tech Stack

- **Language**: Go 1.24
- **Framework**: Gin (HTTP), sqlx (DB), zerolog (logging)
- **Database**: PostgreSQL 15, Redis 7
- **Deploy**: Docker Compose (multi-stage Alpine build)

## Project Structure

```
cmd/api/main.go          # Entry point, routing, DI
internal/
  config/                # Env var loading
  database/              # PostgreSQL connection
  cache/                 # Redis client & inquiry cache
  models/                # Domain entities (struct tags: db, json)
  repository/            # Data access layer (sqlx queries)
  service/               # Business logic & provider adapters
  handler/               # HTTP handlers (Gin)
  middleware/             # Auth, CORS, logging, rate-limit
  worker/                # Background goroutines (sync, retry, status check)
  utils/                 # Response formatting, errors, JWT, HMAC
  sse/                   # Server-Sent Events hub
pkg/
  digiflazz/             # Digiflazz PPOB provider client
  kiosbank/              # Kiosbank PPOB provider client
  alterra/               # Alterra PPOB provider client
  bnc/                   # Bank Neo Commerce (BNC) SNAP BI client
  bri/                   # BRI SNAP BI client
  pakailink/             # Pakailink SNAP BI (VA + QRIS payment acceptance)
  dana/                  # DANA Direct (e-wallet + QRIS)
  midtrans/              # Midtrans Core API (GoPay, ShopeePay)
  xendit/                # Xendit Payment Request API (Indomaret, Alfamart)
  identity/              # KYC verification
migrations/              # PostgreSQL migrations (golang-migrate)
frontend/                # React/Next.js admin dashboard (separate)
docs/                    # Technical documentation
scripts/                 # Utility scripts & nginx config
```

## Architecture

Clean layered architecture: **Handler -> Service -> Repository -> Database**

Data flow:
1. HTTP Request -> Handler (validate, extract client context)
2. Service (business logic, state transitions, provider calls)
3. Repository (PostgreSQL via sqlx)
4. Async webhook callbacks & background worker retries

### Key Patterns

- Constructor-based DI: `NewXxxService(repo, cache, ...)`
- Multi-provider adapter pattern with smart routing & failover
- Exponential backoff retry for transactions and webhooks
- SSE for real-time admin dashboard updates

## Common Commands

```bash
# Docker (recommended)
make build           # Build Docker images
make dev             # Run foreground with logs
make run             # Run background
make stop            # Stop containers
make restart         # Restart API container
make logs            # Tail API logs
make psql            # Connect to PostgreSQL

# Local development
make run-local       # go run cmd/api/main.go
make build-local     # go build -o bin/gtd cmd/api/main.go

# Testing & linting
make test            # go test -v ./...
make test-cover      # Coverage report
make lint            # golangci-lint
```

## Authentication

- **Client API**: `Authorization: Bearer <api_key>` or `X-API-Key` header
  - Sandbox keys: `sk_sandbox_xxx`, Production keys: `sk_production_xxx`
- **Admin JWT**: `POST /v1/admin/auth/login` -> Bearer token
  - Also supports `?token=` query param for SSE/EventSource

## Environment

See `.env.example` for all required variables. Key groups:
- `DB_*` - PostgreSQL connection
- `REDIS_*` - Redis connection
- `JWT_SECRET` - Admin auth
- `DIGIFLAZZ_*`, `KIOSBANK_*`, `ALTERRA_*` - PPOB providers
- `BRI_*`, `BNC_*`, `BCA_*`, `BNI_*`, `MANDIRI_*` - Banking
- `PAKAILINK_*`, `DANA_*`, `OVO_*`, `MIDTRANS_*`, `XENDIT_*` - Payment
- `*_INTERVAL` - Background worker schedules

## API Endpoints

**Public**: `GET /v1/health`, `POST /webhook/*` (provider callbacks)

**Protected (API Key)**:
- `GET/POST /v1/ppob/*` - Products, balance, transactions
- `POST /v1/transfer`, `GET /v1/transfer/:id` - Bank transfers
- `GET /v1/bank-codes`
- `GET /v1/payment/methods`, `POST /v1/payment/create`, `GET /v1/payment/:id`, `POST /v1/payment/:id/cancel|refund`

**Admin (JWT)**:
- `/v1/admin/auth/*` - Login
- `/v1/admin/clients/*` - Client management (includes `paymentCallbackUrl` + `key_type: payment_webhook`)
- `/v1/admin/products/*`, `/v1/admin/skus/*` - Product catalog
- `/v1/admin/transactions/*` - Transaction monitoring & retry
- `/v1/admin/providers/*` - Provider health & config
- `/v1/admin/payments/*` - Payment monitoring, refunds, callback retry
- `/v1/admin/payment-methods/*` - Method config (provider, fees, maintenance)
- `/v1/admin/sse` - Real-time updates (emits `payment.*` events)

## Code Conventions

- No global variables; package-scoped types with constructors
- Error propagation with wrapping (`fmt.Errorf("...: %w", err)`)
- Context passed through for cancellation & timeouts
- Struct tags: `db:"col"` for sqlx, `json:"field"` for API
- Enums as string constants (transaction status, provider type, etc.)
- Parameterized SQL queries (no string interpolation)

## Database

PostgreSQL with golang-migrate. Migrations in `migrations/` directory.
Key tables: `clients`, `products`, `skus`, `transactions`, `transfers`, `payments`, `payment_methods`, `payment_callbacks`, `payment_callback_logs`, `payment_logs`, `refunds`, `ppob_providers`, `provider_skus`, `callbacks`, `bank_codes`, `admin_users`

## PPOB Providers

Multi-provider support with intelligent routing:
- **Digiflazz** - Primary PPOB (pulsa, data, PLN, game vouchers)
- **Kiosbank** - Alternative PPOB
- **Alterra** - Alternative PPOB
- **BRI BRIZZI** - BRI-specific products

Provider selection logic in `service/provider_router.go`. Each provider has its own adapter in `pkg/` and service layer in `internal/service/`.

## Payment Module

Phase 1 provider routing (dispatcher: `payment_methods.provider` column):
- **Pakailink** — VA (BCA/BSI/OCBC/CIMB/Permata/Muamalat + fallback for BNI/BRI/Mandiri/BNC) and QRIS MPM
- **DANA Direct** — DANA e-wallet + QRIS (default)
- **Midtrans** — GoPay, ShopeePay
- **Xendit** — Indomaret, Alfamart retail
- **OVO** — disabled in Phase 1 (`is_active=false`)

Client webhooks use dedicated `payment_callback_url` + `payment_callback_secret` (falls back to generic `callback_url`/`callback_secret`). Signature header: `X-GTD-Payment-Signature: sha256=<hex>`. Retry backoff: 30s/1m/5m/30m/2h (max 5 attempts). QRIS provider is switchable from admin UI. Pakailink dual-webhook dedupe: `callbackType=settlement` is ACK-only.

Workers: `PaymentStatusWorker` (pending inquiry), `PaymentExpiryWorker` (mark expired), `PaymentCallbackWorker` (retry client webhooks).

## Alterra Integration

- **Status**: Staging (belum production)
- **Base URL**: `https://horven-api.sumpahpalapa.com/api`
- **Client ID**: `gtd`
- **Auth**: RSA-SHA256 signature, private key at `keys/alterra/private_key.pem`
- **Callback URL**: `https://api.gtd.co.id/v1/webhook/alterra` (POST)
- **Callback verification**: RSA-SHA256 via `X-Signature` header, public key at `keys/alterra/public_key.pem`
- **IP whitelist**: `13.250.242.158` (Alterra callback server)
- **Docs**: `docs/ppob/alterra/` (API docs + product catalog Excel)
- **Product catalog**: `docs/ppob/alterra/20262801 - BPA Product Catalogue (OPEN).xlsx`
- **Categories**: Pulsa, PLN, BPJS, Postpaid, Gas & PDAM, Streaming & TV, Voucher Deals, Edukasi

## VPS / Deployment

- **SSH**: `ssh -i ~/.ssh/gtd ubuntu@15.235.143.72`
- **Path**: `~/backend/` (docker-compose deployment)
- **Domain**: `api.gtd.co.id` (Nginx reverse proxy → port 8080)
- **Nginx config**: `scripts/nginx-api.gtd.co.id.conf`
- **Deploy workflow**:
  1. `git push origin main`
  2. SSH to VPS
  3. `cd ~/backend && git pull origin main`
  4. `make build && make restart`
  5. `make logs` (verify)
