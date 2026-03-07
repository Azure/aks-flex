
# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/azure/aks-flex/karpenter
# Image tag: branch name + short commit hash (e.g. main-abc1234)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD | sed 's/[^a-zA-Z0-9._-]/-/g')
GIT_SHORT_SHA := $(shell git rev-parse --short HEAD)
TAG ?= $(GIT_BRANCH)-$(GIT_SHORT_SHA)
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

# Get the currently used golang install path (in GOBIN, currentdir/bin, or GOPATH/bin)
GOBIN = $(shell go env GOBIN)
ifeq (,$(GOBIN))
GOBIN = $(shell go env GOPATH)/bin
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands in recipes, e.g., using process substitution.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Go build variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GO_LDFLAGS ?=
BINARY_NAME ?= controller

# Helper for subst in platform iteration
comma := ,

.PHONY: all
all: help

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

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=pkg/apis/crds

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run the e2e tests against a Kind k8s instance that is spun up.
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Vendor & Patches

.PHONY: vendor
vendor: ## Run go mod vendor to populate the vendor directory.
	go mod vendor

.PHONY: patch
patch: ## Apply patches from patches/ directory to vendored code.
	@for p in patches/*.diff; do \
		echo "Applying $$p ..."; \
		git apply --verbose "$$p" || exit 1; \
	done

.PHONY: vendor-patch
vendor-patch: vendor patch ## Vendor dependencies and apply patches.

.PHONY: verify-vendor
verify-vendor: vendor-patch ## Verify vendor directory is up-to-date with patches applied.
	@if [ -n "$$(git status --porcelain vendor/)" ]; then \
		echo "ERROR: vendor/ directory is out of date. Run 'make vendor-patch' and commit the changes."; \
		git diff --stat vendor/; \
		exit 1; \
	fi

##@ Build

.PHONY: build
build: vendor-patch fmt vet ## Build controller binary.
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/$(BINARY_NAME)-$(GOOS)-$(GOARCH) ./cmd/controller

.PHONY: run
run: vendor-patch fmt vet ## Run a controller from your host.
	go run ./cmd/controller

# Build the binary for linux targeting the given architecture, then package it
# into a container image. The Dockerfile simply copies the pre-built binary into
# a distroless base image.
.PHONY: docker-build
docker-build: ## Build docker image with the controller.
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build
	$(CONTAINER_TOOL) build --build-arg TARGETARCH=$(GOARCH) -t ${IMG}:${TAG} .

.PHONY: docker-push
docker-push: ## Push docker image with the controller.
	$(CONTAINER_TOOL) push ${IMG}:${TAG}

# PLATFORMS defines the target platforms for the controller image to be built and pushed.
# To use this option you need to:
# - be able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To properly provided solutions that support more than one platform, you should use this option.
PLATFORMS ?= linux/amd64
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the controller for cross-platform support.
	@for platform in $(subst $(comma), ,$(PLATFORMS)); do \
		os=$${platform%%/*}; arch=$${platform##*/}; \
		echo "Building binary for $${os}/$${arch}..."; \
		GOOS=$${os} GOARCH=$${arch} $(MAKE) build || exit 1; \
	done
	- $(CONTAINER_TOOL) buildx create --name karpenter-builder
	$(CONTAINER_TOOL) buildx use karpenter-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG}:${TAG} -f Dockerfile .
	- $(CONTAINER_TOOL) buildx rm karpenter-builder

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Helm Chart

.PHONY: helm-lint
helm-lint: ## Lint the Karpenter Helm chart.
	helm lint charts/karpenter

.PHONY: helm-package
helm-package: ## Lint and package the Karpenter Helm chart into a .tgz archive.
	hack/release-chart.sh

.PHONY: helm-push
helm-push: ## Lint, package, and push the Karpenter Helm chart to an OCI registry. Requires HELM_REGISTRY (e.g. oci://ghcr.io/azure/aks-flex/charts).
	hack/release-chart.sh --registry $(HELM_REGISTRY) --push

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during cleanup.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}:${TAG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during cleanup.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.20.1
ENVTEST_VERSION ?= release-0.22
GOLANGCI_LINT_VERSION ?= v2.10.1

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

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
