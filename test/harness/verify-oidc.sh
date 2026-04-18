#!/bin/bash
set -euo pipefail

# Helper script to verify OIDC Client creation and Secret generation.
# Usage: ./verify-oidc.sh

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$(dirname "$DIR")")"
KUBECONFIG="${PROJECT_ROOT}/bin/kind.kubeconfig"

if [ ! -f "$KUBECONFIG" ]; then
    echo "❌ Kubeconfig not found at $KUBECONFIG"
    exit 1
fi
export KUBECONFIG

CLIENT_NAME="test-client"
SECRET_NAME="test-client-credentials"
TEST_MANIFEST="${PROJECT_ROOT}/config/samples/test-oidc-client.yaml"

echo "🔍 Verifying OIDC Client Controller..."

# 1. Clean up old resources
echo "🧹 Cleaning up previous client..."
kubectl delete pocketidoidcclient "$CLIENT_NAME" --ignore-not-found=true
kubectl delete secret "$SECRET_NAME" --ignore-not-found=true

# 2. Apply resource
echo "🚀 Applying OIDC Client manifest..."
if [ ! -f "$TEST_MANIFEST" ]; then
    echo "❌ Manifest not found: $TEST_MANIFEST"
    exit 1
fi
kubectl apply -f "$TEST_MANIFEST"

# 3. Wait for Ready condition
echo "⏳ Waiting for OIDC Client status..."
if kubectl wait --for=condition=Ready --timeout=60s pocketidoidcclient/"$CLIENT_NAME"; then
    echo "✅ Client is Ready!"
else
    echo "❌ Client failed to become Ready."
    kubectl get pocketidoidcclient "$CLIENT_NAME" -o yaml
    exit 1
fi

# 4. Verify Secret
echo "🔐 Verifying Credentials Secret..."
# Allow a moment for secret creation after status update (though it should be sync)
sleep 2

if kubectl get secret "$SECRET_NAME" >/dev/null 2>&1; then
    CLIENT_ID=$(kubectl get secret "$SECRET_NAME" -o jsonpath='{.data.OIDC_CLIENT_ID}' | base64 -d)
    CLIENT_SECRET=$(kubectl get secret "$SECRET_NAME" -o jsonpath='{.data.OIDC_CLIENT_SECRET}' | base64 -d)
    
    if [ -n "$CLIENT_ID" ] && [ -n "$CLIENT_SECRET" ]; then
        echo "✅ Secret created successfully!"
        echo "   ID: $CLIENT_ID"
        echo "   Secret: [REDACTED]"
    else
        echo "❌ Secret created but fields missing/empty."
        kubectl get secret "$SECRET_NAME" -o yaml
        exit 1
    fi
else
    echo "❌ Credentials Secret '$SECRET_NAME' not found."
    exit 1
fi

# 5. Clean up (Optional - keep for inspection?)
# echo "🧹 Cleaning up..."
# kubectl delete pocketidoidcclient "$CLIENT_NAME"

echo "🎉 Verification SUCCESS!"
