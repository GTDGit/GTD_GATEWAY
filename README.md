# GTD API Gateway

PPOB API Gateway dengan smart SKU switching dan auto-retry untuk 99.9% success rate.

## Quick Start

1. Setup environment
```bash
cp .env.example .env
# Edit .env dengan Digiflazz credentials
```

2. Build & Run
```bash
make build
make run
```

3. Test
```bash
curl http://localhost:8080/v1/health
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/health` | Health check |
| GET | `/v1/products` | Get products |
| GET | `/v1/balance` | Get balance |
| POST | `/v1/transaction` | Create transaction |
| GET | `/v1/transaction/:id` | Get transaction |

## Authentication

```
Authorization: Bearer <api_key>
X-Client-Id: <client_id>
```

Sandbox mode: gunakan `sk_sandbox_xxx`

## Transaction Types

- `prepaid` - Pulsa, data, token PLN, game
- `inquiry` - Cek tagihan postpaid
- `payment` - Bayar tagihan postpaid

## Commands

```bash
make build    # Build images
make run      # Run background
make logs     # View logs
make stop     # Stop
make psql     # Connect DB
```
