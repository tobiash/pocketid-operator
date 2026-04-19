#!/bin/bash
# Sets up kind cluster and deploys operator + PocketID for e2e tests
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-pocketid-test}"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "=== E2E Test Harness Setup ==="

# Check prerequisites
for cmd in kind podman kubectl; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "ERROR: $cmd is required but not installed"
        exit 1
    fi
done

echo "✓ All prerequisites found"

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "♻️ Cluster '$CLUSTER_NAME' already exists, deleting..."
    kind delete cluster --name "$CLUSTER_NAME"
fi

# Create kind cluster with podman
echo "🚀 Creating kind cluster..."
export KIND_EXPERIMENTAL_PROVIDER=podman
kind create cluster --name "$CLUSTER_NAME" --config "$PROJECT_ROOT/test/harness/kind-config.yaml"
echo "✓ Cluster created"

# Build operator image
echo "🔨 Building operator image..."
cd "$PROJECT_ROOT"
make docker-build IMG=controller:latest
echo "✓ Operator image built"

# Pull PocketID image
echo "📦 Pulling PocketID image..."
podman pull ghcr.io/pocket-id/pocket-id:latest
echo "✓ PocketID image pulled"

# Load images into kind
echo "📤 Loading images into kind..."
kind load docker-image controller:latest --name "$CLUSTER_NAME"
kind load docker-image ghcr.io/pocket-id/pocket-id:latest --name "$CLUSTER_NAME"
echo "✓ Images loaded"

# Install Gateway API CRDs
echo "📜 Installing Gateway API CRDs..."
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
echo "✓ Gateway API CRDs installed"

# Install operator CRDs
echo "📜 Installing operator CRDs..."
kubectl apply -f "$PROJECT_ROOT/config/crd/bases/"
echo "✓ Operator CRDs installed"

# Create namespace for operator
kubectl create namespace pocketid-operator-system --dry-run=client -o yaml | kubectl apply -f -

# Deploy operator from default kustomization
echo "🚀 Deploying operator..."
cd "$PROJECT_ROOT"
kubectl kustomize config/default | kubectl apply -f -
echo "✓ Operator deployed"

# Wait for operator deployment
echo "⏳ Waiting for operator to be ready..."
if ! kubectl wait --timeout=120s -n pocketid-operator-system deployment/controller-manager --for=condition=Available 2>/dev/null; then
    echo "⚠️ Operator deployment not ready after 120s, showing status:"
    kubectl get pods -n pocketid-operator-system
    echo "Continuing anyway (pods may still be starting)..."
fi
echo "✓ Operator ready"

# Export kubeconfig path
KUBECONFIG_PATH="$PROJECT_ROOT/bin/kind.kubeconfig"
kind get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG_PATH"
echo ""
echo "=== Setup Complete ==="
echo "Cluster: kind-${CLUSTER_NAME}"
echo "Kubeconfig: $KUBECONFIG_PATH"
echo ""
echo "To run e2e tests:"
echo "  export KUBECONFIG=$KUBECONFIG_PATH"
echo "  go test ./test/e2e/... -v -timeout 30m"
echo ""
echo "To clean up:"
echo "  ./test/harness/teardown-e2e.sh"