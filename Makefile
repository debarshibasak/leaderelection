.PHONY: help build run test clean docker-build docker-up docker-down db-up db-down migrate fmt vet lint

# Variables
BINARY_NAME=leaderelection
DOCKER_IMAGE=leaderelection:latest
MAIN_PATH=./cmd/main.go

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BINARY_NAME)"

run: ## Run the application locally
	@echo "Running $(BINARY_NAME)..."
	@go run $(MAIN_PATH)

test: ## Run tests
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@go clean
	@echo "Clean complete"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

lint: ## Run golangci-lint (requires golangci-lint to be installed)
	@echo "Running linter..."
	@golangci-lint run || echo "golangci-lint not installed. Install with: brew install golangci-lint"

tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	@go mod tidy

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE) .
	@echo "Docker image built: $(DOCKER_IMAGE)"

docker-up: ## Start all services with docker-compose
	@echo "Starting services with docker-compose..."
	@docker-compose up -d
	@echo "Services started. Use 'docker-compose logs -f' to view logs"

docker-down: ## Stop all services
	@echo "Stopping services..."
	@docker-compose down
	@echo "Services stopped"

docker-logs: ## Show docker-compose logs
	@docker-compose logs -f

docker-restart: docker-down docker-up ## Restart all docker services

# Database targets
db-up: ## Start PostgreSQL database only
	@echo "Starting PostgreSQL..."
	@docker-compose up -d postgres
	@echo "PostgreSQL started on port 5432"

db-down: ## Stop PostgreSQL database
	@echo "Stopping PostgreSQL..."
	@docker-compose stop postgres
	@echo "PostgreSQL stopped"

db-shell: ## Connect to PostgreSQL shell
	@docker-compose exec postgres psql -U postgres -d leaderelection

db-reset: ## Reset database (WARNING: destroys all data)
	@echo "Resetting database..."
	@docker-compose down -v
	@docker-compose up -d postgres
	@sleep 2
	@echo "Database reset complete"

# Development targets
dev-node1: ## Run node 1 (high priority)
	@echo "Starting node-1 (priority: 100)..."
	@NODE_ID=node-1 NODE_PRIORITY=100 DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable" go run $(MAIN_PATH)

dev-node2: ## Run node 2 (medium priority)
	@echo "Starting node-2 (priority: 50)..."
	@NODE_ID=node-2 NODE_PRIORITY=50 DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable" go run $(MAIN_PATH)

dev-node3: ## Run node 3 (low priority)
	@echo "Starting node-3 (priority: 10)..."
	@NODE_ID=node-3 NODE_PRIORITY=10 DATABASE_URL="host=localhost user=postgres password=postgres dbname=leaderelection port=5432 sslmode=disable" go run $(MAIN_PATH)

# Quality checks
check: fmt vet test ## Run all quality checks (format, vet, test)

# Installation
install: ## Install the binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@go install $(MAIN_PATH)
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# Full workflow
all: clean deps build test ## Clean, download deps, build, and test

# Watch mode (requires entr: brew install entr)
watch: ## Watch for changes and rebuild (requires entr)
	@echo "Watching for changes..."
	@find . -name '*.go' | entr -r make run
