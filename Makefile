.PHONY: build run dev stop logs logs-db clean psql run-local build-local test test-cover lint help restart

# Docker commands
build:
	docker-compose build

run:
	docker-compose up -d

dev:
	docker-compose up

stop:
	docker-compose down

restart:
	docker-compose restart api

logs:
	docker-compose logs -f api

logs-db:
	docker-compose logs -f postgres

# Clean up
clean:
	docker-compose down -v
	docker system prune -f

# Database
psql:
	docker-compose exec postgres psql -U gtd -d gtd

# Local development
run-local:
	go run cmd/api/main.go

build-local:
	go build -o bin/gtd cmd/api/main.go

# Testing
test:
	go test -v ./...

test-cover:
	go test -v -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

# Help
help:
	@echo "GTD API Gateway"
	@echo ""
	@echo "Docker:"
	@echo "  make build      - Build Docker images"
	@echo "  make run        - Run in background"
	@echo "  make dev        - Run with logs"
	@echo "  make stop       - Stop containers"
	@echo "  make restart    - Restart API"
	@echo "  make logs       - View API logs"
	@echo "  make logs-db    - View DB logs"
	@echo "  make clean      - Remove everything"
	@echo ""
	@echo "Database:"
	@echo "  make psql       - Connect to PostgreSQL"
	@echo ""
	@echo "Local:"
	@echo "  make run-local  - Run without Docker"
	@echo "  make test       - Run tests"
