#!/bin/bash
# Sets up kind cluster and deploys operator + PocketID for e2e tests
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-pocketid-test}"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONTAINER_TOOL="${CONTAINER_TOOL:-podman}"

# Add local bin to PATH
export PATH="$PROJECT_ROOT/bin:$PATH"

echo "=== E2E Test Harness Setup ==="

# Check prerequisites
for cmd in kind kubectl "$CONTAINER_TOOL"; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "ERROR: $cmd is required but not installed"
        exit 1
    fi
done

echo "✓ All prerequisites found"

# Ensure podman socket is running (required by kind podman provider)
if [ "$CONTAINER_TOOL" = "podman" ]; then
    if ! systemctl --user is-active podman.socket &>/dev/null; then
        echo "🔌 Starting podman socket..."
        systemctl --user start podman.socket
    fi
fi

# Create kind cluster
echo "🚀 Creating kind cluster..."
if [ "$CONTAINER_TOOL" = "podman" ]; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
fi

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "♻️ Cluster '$CLUSTER_NAME' already exists, deleting..."
    if [ "$CONTAINER_TOOL" = "podman" ]; then
        systemd-run --user -p Delegate=yes --scope kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || kind delete cluster --name "$CLUSTER_NAME"
    else
        kind delete cluster --name "$CLUSTER_NAME"
    fi
fi

# Create cluster
if [ "$CONTAINER_TOOL" = "podman" ]; then
    systemd-run --user -p Delegate=yes --scope kind create cluster --name "$CLUSTER_NAME" --config "$PROJECT_ROOT/test/harness/kind-config.yaml" --wait 120s
else
    kind create cluster --name "$CLUSTER_NAME" --config "$PROJECT_ROOT/test/harness/kind-config.yaml" --wait 120s
fi
echo "✓ Cluster created"

# Set up kubeconfig for kubectl commands
KUBECONFIG_PATH="$PROJECT_ROOT/bin/kind.kubeconfig"
kind get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG_PATH"
export KUBECONFIG="$KUBECONFIG_PATH"
echo "✓ Kubeconfig set to $KUBECONFIG_PATH"

# Build operator image
echo "🔨 Building operator image..."
cd "$PROJECT_ROOT"
if [ "$CONTAINER_TOOL" = "podman" ]; then
    OPERATOR_IMG="localhost/controller:latest"
else
    OPERATOR_IMG="controller:latest"
fi
make docker-build IMG="$OPERATOR_IMG" CONTAINER_TOOL="$CONTAINER_TOOL" DOCKER_BUILD_FLAGS="--no-cache"
echo "✓ Operator image built"

# Pull PocketID image
echo "📦 Pulling PocketID image..."
$CONTAINER_TOOL pull ghcr.io/pocket-id/pocket-id:latest
echo "✓ PocketID image pulled"

# Load images into kind (using podman exec + ctr for podman compatibility)
echo "📤 Loading images into kind..."
NODE="${CLUSTER_NAME}-control-plane"
$CONTAINER_TOOL save "$OPERATOR_IMG" | $CONTAINER_TOOL exec -i "$NODE" ctr -n k8s.io images import -
$CONTAINER_TOOL save ghcr.io/pocket-id/pocket-id:latest | $CONTAINER_TOOL exec -i "$NODE" ctr -n k8s.io images import -
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
if [ "$CONTAINER_TOOL" = "podman" ]; then
    kubectl kustomize config/default | sed 's|image: controller:latest|image: localhost/controller:latest|g' | kubectl apply -f -
else
    kubectl kustomize config/default | kubectl apply -f -
fi
echo "✓ Operator deployed"

# Wait for operator deployment
echo "⏳ Waiting for operator to be ready..."
if ! kubectl wait --timeout=180s -n pocketid-operator-system deployment/pocketid-operator-controller-manager --for=condition=Available; then
    echo "⚠️ Operator deployment not ready after 180s, showing status:"
    kubectl get pods -n pocketid-operator-system
    echo "Continuing anyway (pods may still be starting)..."
fi
echo "✓ Operator ready"

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