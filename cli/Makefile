# Binary name
BINARY_NAME ?= aks-flex-cli

# Go build variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GO_LDFLAGS ?= -s -w

# Git metadata for ldflags
GIT_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Setting SHELL to bash allows bash commands in recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: help

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GORELEASER ?= $(LOCALBIN)/goreleaser
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

## Tool Versions
GORELEASER_VERSION ?= v2.8.2
GOLANGCI_LINT_VERSION ?= v2.10.1

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# temporary file assignment category, and for line with descriptions of
# temporary file assignment rules.
# Source: https://www.padok.fr/en/blog/beautiful-makefile-awk
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: goreleaser ## Build CLI binaries for all platforms using goreleaser.
	$(GORELEASER) build --snapshot --clean

.PHONY: build-local
build-local: fmt vet ## Build CLI binary for the current platform only.
	CGO_ENABLED=0 go build -trimpath \
		-ldflags "$(GO_LDFLAGS) -X main.version=$(GIT_VERSION) -X main.commit=$(GIT_COMMIT) -X main.date=$(BUILD_DATE)" \
		-o $(LOCALBIN)/$(BINARY_NAME) .

.PHONY: build-archive
build-archives: ## Build and archive CLI binaries for all platforms using goreleaser.
	$(GORELEASER) release --snapshot --clean --skip=publish

.PHONY: run
run: ## Run the CLI from source.
	go run .

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf dist/ $(LOCALBIN)/

##@ Dependencies

.PHONY: goreleaser
goreleaser: $(GORELEASER) ## Download goreleaser locally if necessary.
$(GORELEASER): $(LOCALBIN)
	$(call go-install-tool,$(GORELEASER),github.com/goreleaser/goreleaser/v2,$(GORELEASER_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint/v2,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and target version.
# The install directory is $(LOCALBIN).
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
	set -e; \
	package=$(2)@$(3) ;\
	echo "Downloading $${package}" ;\
	rm -f $(1) || true ;\
	GOBIN=$(LOCALBIN) go install $${package} ;\
	mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
