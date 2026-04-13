BINARY := gitlab-otel-exporter
MODULE := $(shell head -1 go.mod | awk '{print $$2}')
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: build test lint fmt vet clean run docker help

build: ## Build the binary
	go build $(LDFLAGS) -o $(BINARY) cmd/main.go

test: ## Run tests with coverage
	go test -v -cover ./...

test-report: ## Run tests and generate JUnit XML report
	go install github.com/jstemmer/go-junit-report/v2@latest
	go test -v -cover ./... 2>&1 | go-junit-report -set-exit-code > report.xml

lint: fmt vet ## Run all linters

fmt: ## Check formatting
	@test -z "$$(gofmt -l .)" || (gofmt -d . && exit 1)

vet: ## Run go vet
	go vet ./...

clean: ## Remove build artifacts
	rm -f $(BINARY) report.xml

run: build ## Build and run
	./$(BINARY)

docker: ## Build Docker image
	docker build -t $(BINARY):$(VERSION) -t $(BINARY):latest .

tidy: ## Tidy and verify dependencies
	go mod tidy
	go mod verify

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
