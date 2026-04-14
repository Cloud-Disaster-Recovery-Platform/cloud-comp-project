APP_NAME   := state-sync-engine
CMD_DIR    := ./cmd/state-sync
IMAGE_NAME := cloud-mirror/state-sync-engine
IMAGE_TAG  ?= latest

# ─── Build ───────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build the Go binary
	go build -ldflags="-s -w" -o bin/$(APP_NAME) $(CMD_DIR)

.PHONY: build-linux
build-linux: ## Cross-compile for Linux amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(APP_NAME)-linux $(CMD_DIR)

# ─── Test ────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: test-verbose
test-verbose: ## Run all tests with verbose output
	go test -v ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

# ─── Docker ──────────────────────────────────────────────────────────────────

.PHONY: docker-build
docker-build: ## Build the Docker image
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

.PHONY: docker-push
docker-push: ## Push the Docker image (set IMAGE_NAME for your registry)
	docker push $(IMAGE_NAME):$(IMAGE_TAG)

# ─── Dev helpers ─────────────────────────────────────────────────────────────

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/

.PHONY: run
run: build ## Build and run with the example config
	./bin/$(APP_NAME) -config config.yaml

# ─── Help ────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
