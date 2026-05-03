# =============================================================================
# Sovereign Sensor - Makefile
# =============================================================================

# Image URLs
IMG ?= sovereign-controller:dev
AGENT_IMAGE ?= sovereign-sensor-agent:dev

# Cluster settings
CLUSTER_NAME ?= sovereign-test
KIND_CLUSTER ?= sovereign-sensor-test-e2e

# Container tool (docker or podman)
CONTAINER_TOOL ?= docker

# Tool versions
TETRAGON_VERSION ?= 1.6.1
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.20.1
GOLANGCI_LINT_VERSION ?= v2.8.0

# Local bin directory for tools
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

# =============================================================================
# Tool Binaries
# =============================================================================

KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

# =============================================================================
# General
# =============================================================================

.PHONY: all
all: build

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

# =============================================================================
# Local Cluster & Development
# =============================================================================

.PHONY: cluster-up
cluster-up: ## Create local Kind cluster
	@echo "🚀 Starting Kind cluster '$(CLUSTER_NAME)'..."
	kind create cluster --name $(CLUSTER_NAME)

.PHONY: cluster-down
cluster-down: ## Delete local Kind cluster
	@echo "🗑️  Deleting Kind cluster '$(CLUSTER_NAME)'..."
	kind delete cluster --name $(CLUSTER_NAME)

.PHONY: vendor-manifests
vendor-manifests: ## Download and template third-party manifests for embedding
	@echo "📥 Vendoring Tetragon manifests (v$(TETRAGON_VERSION))..."
	helm repo add cilium https://helm.cilium.io/ > /dev/null 2>&1
	helm repo update > /dev/null 2>&1
	helm template tetragon cilium/tetragon \
		--namespace kube-system \
		--version $(TETRAGON_VERSION) \
		> hack/tetragon.yaml
	@echo "✅ Vendored into hack/tetragon.yaml"

.PHONY: dev
dev: cluster-up manifests vendor-manifests build-frontend docker-build docker-build-agent kind-load install deploy ## Full local development setup
	@echo "⏳ Installing Tetragon $(TETRAGON_VERSION)..."
	kubectl apply -f hack/tetragon.yaml
	@echo "⚙️  Waiting for controller manager..."
	kubectl rollout status deployment sovereign-sensor-controller-manager -n sovereign-sensor-system --timeout=90s
	@echo "🚀 Applying sample resources..."
	kubectl apply -f config/samples/sensor.yaml
	kubectl apply -f config/samples/policy.yaml
	kubectl apply -f config/samples/violator.yaml
	@echo "✅ Development environment ready!"

# =============================================================================
# Build
# =============================================================================

.PHONY: build
build: manifests generate fmt vet ## Build manager binary
	go build -o bin/manager cmd/controller/main.go

.PHONY: build-frontend
build-frontend: ## Build React frontend
	@echo "🏗️  Building React frontend..."
	cd frontend && npm run build

.PHONY: docker-build
docker-build: build-frontend ## Build controller Docker image
	@echo "🔨 Building controller image '$(IMG)'..."
	$(CONTAINER_TOOL) build -t $(IMG) .

.PHONY: docker-build-agent
docker-build-agent: ## Build eBPF agent Docker image
	@echo "🔨 Building agent image '$(AGENT_IMAGE)'..."
	$(CONTAINER_TOOL) build -f agent.Dockerfile -t $(AGENT_IMAGE) .

.PHONY: docker-push
docker-push: ## Push controller Docker image
	$(CONTAINER_TOOL) push $(IMG)

.PHONY: kind-load
kind-load: docker-build docker-build-agent ## Load images into Kind cluster
	@echo "📦 Loading images into Kind cluster '$(CLUSTER_NAME)'..."
	kind load docker-image $(IMG) --name $(CLUSTER_NAME)
	kind load docker-image $(AGENT_IMAGE) --name $(CLUSTER_NAME)

# =============================================================================
# Code Generation & Quality
# =============================================================================

.PHONY: manifests
manifests: controller-gen ## Generate CRDs, RBAC, etc.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate DeepCopy methods
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run unit tests
	KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" \
	go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run linter and auto-fix
	"$(GOLANGCI_LINT)" run --fix

# =============================================================================
# E2E Testing
# =============================================================================

.PHONY: setup-test-e2e
setup-test-e2e: ## Create Kind cluster for e2e tests if needed
	@command -v $(KIND) >/dev/null 2>&1 || { echo "Kind not found. Please install it."; exit 1; }
	@$(KIND) get clusters | grep -q "$(KIND_CLUSTER)" || \
		{ echo "Creating e2e Kind cluster..."; $(KIND) create cluster --name $(KIND_CLUSTER); }

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run e2e tests
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: e2e-manual
e2e-manual: cluster-up kind-load ## Run e2e tests manually (no auto-cleanup)
	@bash test/e2e-manual.sh

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Delete e2e Kind cluster
	@$(KIND) delete cluster --name $(KIND_CLUSTER) 2>/dev/null || true

# =============================================================================
# Deployment
# =============================================================================

.PHONY: install
install: manifests kustomize ## Install CRDs
	"$(KUSTOMIZE)" build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs
	"$(KUSTOMIZE)" build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=$(IMG)
	"$(KUSTOMIZE)" build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller
	"$(KUSTOMIZE)" build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: build-installer
build-installer: manifests generate kustomize ## Build consolidated install YAML
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=$(IMG)
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

# =============================================================================
# Dependencies & Tools
# =============================================================================

ifndef ignore-not-found
  ignore-not-found = false
endif

##@ Tools

.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST)
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download envtest binaries
	@echo "Setting up envtest for Kubernetes $(ENVTEST_K8S_VERSION)..."
	"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# =============================================================================
# Helper functions
# =============================================================================

# go-install-tool <target> <package> <version>
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
	set -e; \
	echo "Downloading $(2)@$(3)..."; \
	rm -f "$(1)"; \
	GOBIN="$(LOCALBIN)" go install "$(2)@$(3)"; \
	mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)"; \
	ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"; \
}
endef

# Extract version from go.mod
define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef

# Dynamic envtest versions (based on go.mod)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')