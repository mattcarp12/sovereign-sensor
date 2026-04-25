#!/usr/bin/env bash
set -e

CLUSTER_NAME="sovereign-test"

echo "🚀 Starting E2E Test Environment Setup..."

# 1. Check for required tools
for tool in kind kubectl helm; do
    if ! command -v $tool &> /dev/null; then
        echo "❌ Error: $tool is not installed."
        exit 1
    fi
done

# 2. Spin up the Kind cluster (if it doesn't already exist)
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "✅ Kind cluster '${CLUSTER_NAME}' already exists."
else
    echo "📦 Creating Kind cluster '${CLUSTER_NAME}'..."
    kind create cluster --name ${CLUSTER_NAME}
fi

# 3. Install Tetragon via Helm
echo "⚓ Adding Cilium Helm repo..."
helm repo add cilium https://helm.cilium.io
helm repo update

# Check if Tetragon is already installed to save time
if helm ls -n kube-system | grep -q tetragon; then
    echo "✅ Tetragon is already installed."
else
    echo "🛡️  Installing Tetragon DaemonSet..."
    helm install tetragon cilium/tetragon -n kube-system \
        --set tetragon.grpc.enabled=true \
        --wait
fi

echo "🎉 Environment is ready!"
echo ""
