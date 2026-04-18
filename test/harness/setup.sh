#!/bin/bash
# Sets up kind cluster with podman - supports fresh or reuse modes
set -euo pipefail

MODE="${1:---reuse}"  # Default to reuse for faster iteration
CLUSTER_NAME="${KIND_CLUSTER_NAME:-pocketid-test}"

export KIND_EXPERIMENTAL_PROVIDER=podman

case "$MODE" in
  --fresh)
    echo "🗑️ Deleting existing cluster (if any)..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    echo "🚀 Creating new cluster..."
    kind create cluster --name "$CLUSTER_NAME" --config "$(dirname "$0")/kind-config.yaml"
    FRESH_CLUSTER=true
    ;;
  --reuse)
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
      echo "♻️ Reusing existing cluster: $CLUSTER_NAME"
      FRESH_CLUSTER=false
    else
      echo "🚀 No existing cluster, creating new one..."
      kind create cluster --name "$CLUSTER_NAME" --config "$(dirname "$0")/kind-config.yaml"
      FRESH_CLUSTER=true
    fi
    ;;
  *)
    echo "Usage: $0 [--fresh|--reuse]"
    exit 1
    ;;
esac

# Set kubectl context
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# Install Gateway API CRDs and Envoy Gateway (only on fresh cluster)
if [ "$FRESH_CLUSTER" = true ]; then
  echo "📦 Installing Gateway API CRDs..."
  kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

  echo "📦 Installing Envoy Gateway..."
  helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
    --version v1.2.0 \
    --namespace envoy-gateway-system \
    --create-namespace \
    --wait

  echo "⏳ Waiting for Envoy Gateway to be ready..."
  kubectl wait --timeout=5m -n envoy-gateway-system deployment/envoy-gateway --for=condition=Available
fi

echo "✅ Cluster ready: kind-${CLUSTER_NAME}"
echo ""
echo "To use this cluster:"
echo "  export KUBECONFIG=\$(kind get kubeconfig-path --name=$CLUSTER_NAME)"
echo "  # or"
echo "  kubectl config use-context kind-${CLUSTER_NAME}"
