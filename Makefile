CLUSTER_NAME := sovereign-test

run:
	go run ./cmd/agent/main.go

# Sets up the Kind cluster and installs Tetragon
env-up:
	./test/setup-env.sh

# Tears down the ephemeral test cluster
env-down:
	@echo "🗑️ Destroying Kind cluster..."
	kind delete cluster --name $(CLUSTER_NAME)

# Port-forwards the Tetragon gRPC service to localhost
port-forward:
	@echo "🔌 Forwarding Tetragon gRPC port to 127.0.0.1:54321..."
	kubectl port-forward -n kube-system ds/tetragon 54321:54321

# Runs the unit tests
unit-test:
	go test -short -v ./pkg/...

# ─── Code Generation ─────────────────────────────────────────────────────────

.PHONY: manifests
manifests:
	@echo "⚙️  Generating Webhook, RBAC, and CRD manifests..."
	controller-gen crd paths="./api/..." output:crd:artifacts:config=deploy/

# ─── Build & Deploy ──────────────────────────────────────────────────────────

IMAGE_NAME := sovereign-sensor:latest


.PHONY: build
build:
	@echo "🔨 Building Docker image..."
	docker build -t $(IMAGE_NAME) .

.PHONY: load
load: build
	@echo "📦 Loading image into Kind cluster..."
	kind load docker-image $(IMAGE_NAME) --name $(CLUSTER_NAME)

.PHONY: deploy
deploy: load
	@echo "⚓ Deploying via Helm..."
	# Upgrade or install the chart
	helm upgrade --install sovereign-sensor ./charts/sovereign-sensor \
		--namespace kube-system \
		--wait
	@echo "✅ Deployment complete!"

e2e: env-up deploy
	@echo "🚀 Running end-to-end tests..."
	go test -v ./test/e2e_test.go

violate:
	kubectl create namespace eu-prod || true
	kubectl run violator --namespace eu-prod \
    	--image=curlimages/curl \
    	--restart=Never \
    	-- sh -c "curl -s https://api.datadoghq.com"

	kubectl wait --namespace eu-prod --for=condition=Ready pod/violator --timeout=30s
	kubectl delete pod violator --namespace eu-prod --ignore-not-found
	kubectl delete namespace eu-prod --ignore-not-found