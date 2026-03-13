.PHONY: dev dev-backend dev-frontend build build-backend build-frontend test lint migrate sqlc docker-images clean

# Development
dev:
	@echo "Starting development servers..."
	$(MAKE) dev-backend &
	$(MAKE) dev-frontend &
	wait

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

# Build
build: build-backend build-frontend

build-backend:
	cd backend && go build -o bin/codegym-server ./cmd/server
	cd backend && go build -o bin/codegym ./cmd/cli

build-frontend:
	cd frontend && npm run build

# Test
test:
	cd backend && go test ./...

test-verbose:
	cd backend && go test -v ./...

# Database
migrate:
	cd backend && go run github.com/pressly/goose/v3/cmd/goose@latest -dir internal/store/db/migrations sqlite3 ../data/codegym.db up

migrate-down:
	cd backend && go run github.com/pressly/goose/v3/cmd/goose@latest -dir internal/store/db/migrations sqlite3 ../data/codegym.db down

sqlc:
	cd backend && sqlc generate

# Docker runtime images
docker-images:
	cd docker && ./build-all.sh

# Lint
lint:
	cd backend && golangci-lint run ./...
	cd frontend && npm run lint

# Clean
clean:
	rm -rf backend/bin/
	rm -rf frontend/dist/
	rm -rf data/*.db
