.PHONY: help build test run dry-run clean check-env

help: ## Show this help message
	@echo "Lab Monitor - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the lab monitor binary
	@echo "Building lab monitor..."
	@go build -o bin/labmonitor ./cmd/labmonitor
	@echo "✅ Built: bin/labmonitor"

test: ## Run all tests
	@echo "Running tests..."
	@go test ./... -v

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test ./... -cover

dry-run: check-env ## Run in dry-run mode (no emails sent)
	@echo "Running in DRY-RUN mode..."
	@go run ./cmd/labmonitor -dry-run

run: check-env ## Run the lab monitor (sends real emails!)
	@echo "Running lab monitor..."
	@go run ./cmd/labmonitor

check-env: ## Check if required environment variables are set
	@if [ -z "$$OPENAI_API_KEY" ]; then \
		echo "❌ Error: OPENAI_API_KEY is not set"; \
		echo "   Run: export OPENAI_API_KEY='your-key'"; \
		echo "   Or see TROUBLESHOOTING.md"; \
		exit 1; \
	fi
	@echo "✅ OPENAI_API_KEY is set"

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

lint: fmt vet test ## Run all code quality checks

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf dist/
	@echo "✅ Cleaned"

install-deps: ## Download Go module dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@echo "✅ Dependencies installed"

.DEFAULT_GOAL := help
