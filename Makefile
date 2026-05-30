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
cluster-up: ## Create local Kind cluster only if it doesn't exist
	@echo "🚀 Checking Kind cluster '$(CLUSTER_NAME)'..."
	@kind get clusters | grep -q "^$(CLUSTER_NAME)$$" || kind create cluster --name $(CLUSTER_NAME)

.PHONY: cluster-down
cluster-down: ## Delete local Kind cluster
	@echo "🗑️  Deleting Kind cluster '$(CLUSTER_NAME)'..."
	kind delete cluster --name $(CLUSTER_NAME)

.PHONY: vendor-manifests
vendor-manifests: ## Download and template third-party manifests for embedding
	@echo "📥 Vendoring Tetragon manifests (v$(TETRAGON_VERSION))..."
	@helm repo add cilium https://helm.cilium.io/ > /dev/null 2>&1
	@helm repo update > /dev/null 2>&1
	@mkdir -p hack
	@helm template tetragon cilium/tetragon \
		--namespace kube-system \
		--version $(TETRAGON_VERSION) \
		> hack/tetragon.yaml
	@echo "✅ Vendored into hack/tetragon.yaml"

.PHONY: dev-bootstrap
dev-bootstrap: cluster-down cluster-up dev-update

.PHONY: dev-update
dev-update: manifests vendor-manifests kind-load install deploy
	@echo "⚙️  Forcing Kubernetes to pick up the new image layers..."
	kubectl rollout restart deployment sovereign-sensor-controller-manager -n sovereign-sensor-system
	@echo "⏳ Waiting for controller manager rollout..."
	kubectl rollout status deployment sovereign-sensor-controller-manager -n sovereign-sensor-system --timeout=90s
	@echo "🚀 Applying sample configurations..."
	kubectl apply -f config/samples/sensor.yaml || true
	kubectl apply -f config/samples/policy.yaml || true
	kubectl apply -f config/samples/violator.yaml || true
	@echo "Forwarding Port to access frontend... (Press Ctrl+C to stop)"
	kubectl port-forward svc/sovereign-sensor-controller-manager-api -n sovereign-sensor-system 8080:8080


# Keep your original 'make dev' mapping to the fast update route for daily use
.PHONY: dev
dev: cluster-up dev-update

.PHONY: zip
zip: ## Package workspace archive
	zip -r ss.zip . -x ".devcontainer/*" ".github/*" "bin/*" "frontend/node_modules/*" "frontend/public/*" "internal/api/dist/*" "internal/geo/*.mmdb"

# =============================================================================
# Build
# =============================================================================

.PHONY: build
build: manifests generate fmt vet build-frontend ## Build manager binary locally
	go build -o bin/manager cmd/controller/main.go

.PHONY: build-frontend
build-frontend: ## Build React frontend assets
	@echo "🏗️  Building React frontend..."
	cd frontend && npm install && npm run build

# REFACTOR: Explicitly depend on build-frontend so parallel compilation stays safe
.PHONY: docker-build
docker-build: build-frontend ## Build controller Docker image
	@echo "🔨 Building controller image '$(IMG)'..."
	$(CONTAINER_TOOL) build -t $(IMG) .

.PHONY: docker-build-agent
docker-build-agent: ## Build eBPF agent Docker image
	@echo "🔨 Building agent image '$(AGENT_IMAGE)'..."
	$(CONTAINER_TOOL) build -f agent.Dockerfile -t $(AGENT_IMAGE) .

# REFACTOR: Cleaned redundancy; kind-load handles building its images implicitly
.PHONY: kind-load
kind-load: docker-build docker-build-agent ## Load images into Kind cluster
	@echo "📦 Loading updated images into Kind cluster '$(CLUSTER_NAME)'..."
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

.PHONY: sync-helm
sync-helm: manifests kustomize ## Export Kustomize definitions seamlessly into Helm templates
	@echo "🔄 Syncing generated Kustomize bases to Helm templates folder..."
	@mkdir -p charts/sovereign-sensor/templates
	@mkdir -p charts/sovereign-sensor/crds
	
	@echo "🧹 Cleaning up old generated files to prevent duplicates..."
	@rm -f charts/sovereign-sensor/templates/crds.yaml
	@rm -f charts/sovereign-sensor/templates/operator-manifests.yaml
	
	# Put CRDs in the dedicated crds/ folder so Helm installs them FIRST
	@echo "# Auto-generated from Kustomize CRD bases." > charts/sovereign-sensor/crds/crds.yaml
	@"$(KUSTOMIZE)" build config/crd >> charts/sovereign-sensor/crds/crds.yaml
	
	# Export your unified RBAC rules safely
	@echo "# Auto-generated from Kustomize RBAC configuration." > charts/sovereign-sensor/templates/rbac.yaml
	@echo "{{- if .Values.rbac.create -}}" >> charts/sovereign-sensor/templates/rbac.yaml
	@"$(KUSTOMIZE)" build config/rbac | sed 's/namespace: system/namespace: {{ .Release.Namespace }}/g' >> charts/sovereign-sensor/templates/rbac.yaml
	@echo "" >> charts/sovereign-sensor/templates/rbac.yaml
	@echo "{{- end -}}" >> charts/sovereign-sensor/templates/rbac.yaml
	
	@echo "✅ Sync complete! Your Helm templates match your Kubebuilder configs."

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


# =============================================================================
# Release
# =============================================================================

.PHONY: release
release: sync-helm test lint ## Auto-bump Chart.yaml, commit, tag, and push (e.g., make release VERSION=v0.1.1)
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ Error: VERSION is not set."; \
		echo "💡 Usage: make release VERSION=v0.1.1"; \
		exit 1; \
	fi
	@if ! echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+.*$$'; then \
		echo "❌ Error: VERSION must start with 'v' and follow semantic versioning (e.g., v0.1.1)."; \
		exit 1; \
	fi
	@if [ -n "$$(git status --porcelain charts/sovereign-sensor/Chart.yaml)" ]; then \
		echo "❌ Error: charts/sovereign-sensor/Chart.yaml has uncommitted changes. Please stash or commit them first."; \
		exit 1; \
	fi
	@echo "🧪 Tests and linting passed! Proceeding with release..."
	@echo "📝 Updating Chart.yaml to version $(shell echo $(VERSION) | sed 's/^v//')..."
	@# We use perl instead of sed here to ensure cross-platform compatibility between macOS and Linux
	@perl -pi -e 's/^version: .*/version: $(shell echo $(VERSION) | sed s/^v//)/' charts/sovereign-sensor/Chart.yaml
	@perl -pi -e 's/^appVersion: .*/appVersion: "$(shell echo $(VERSION) | sed s/^v//)"/' charts/sovereign-sensor/Chart.yaml
	@echo "📦 Committing version bump to main..."
	git add charts/sovereign-sensor/Chart.yaml
	git commit -m "chore: bump chart version to $(VERSION)"
	git push origin main
	@echo "🏷️  Creating git tag $(VERSION)..."
	git tag $(VERSION)
	@echo "🚀 Pushing tag $(VERSION) to origin..."
	git push origin $(VERSION)
	@echo "✅ Release fully automated and pushed! GitHub Actions is taking over."