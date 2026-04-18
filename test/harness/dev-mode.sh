#!/bin/bash
set -euo pipefail

# Directory of this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$(dirname "$DIR")")"
KUBECONFIG="${PROJECT_ROOT}/bin/kind.kubeconfig"

# Ensure bin/kind.kubeconfig exists
if [ ! -f "$KUBECONFIG" ]; then
    echo "❌ Kubeconfig not found at $KUBECONFIG. Run 'make install' or setup.sh first."
    exit 1
fi

export KUBECONFIG

echo "🔧 Preparing development environment..."

# 1. Scale down in-cluster operator if it exists
if kubectl get deployment -n pocketid-operator-system pocketid-operator-controller-manager &>/dev/null; then
    echo "📉 Scaling down in-cluster operator..."
    kubectl scale deployment -n pocketid-operator-system pocketid-operator-controller-manager --replicas=0
fi

# 2. Wait for PocketID instance availability
echo "⏳ Waiting for PocketID instance 'test'..."
if ! kubectl wait --for=condition=Available --timeout=120s pocketidinstance/test; then
    echo "❌ PocketID instance not available. Is it deployed?"
    exit 1
fi

# 3. Setup Port Forwarding
echo "🔌 Setting up port-forward to PocketID service..."
PF_LOG=$(mktemp)
kubectl port-forward svc/test-svc 8080:80 > "$PF_LOG" 2>&1 &
PF_PID=$!
echo "   PID: $PF_PID"

# Cleanup function
cleanup() {
    echo "🧹 Cleaning up..."
    if [ -n "${PF_PID:-}" ]; then
        kill "$PF_PID" || true
    fi
    rm -f "$PF_LOG"
}
trap cleanup EXIT

# Wait for port-forward to be ready
sleep 2
if ! kill -0 "$PF_PID"; then
    echo "❌ Port forward failed:"
    cat "$PF_LOG"
    exit 1
fi

# 3.5. Monitor port availability (operator uses 8085)
if lsof -t -i:8085 >/dev/null; then
    echo "❌ Port 8085 is in use. Please stop any existing operator process."
    exit 1
fi

echo "✅ Dev environment ready!"
echo "🚀 Starting operator locally... (Press Ctrl+C to stop)"
echo "---------------------------------------------------"

# 4. Run the operator (blocking)
cd "$PROJECT_ROOT"
make run
