#!/bin/bash
# Tears down kind cluster used for e2e tests
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-pocketid-test}"

echo "🧹 Deleting kind cluster: $CLUSTER_NAME"
export KIND_EXPERIMENTAL_PROVIDER=podman
kind delete cluster --name "$CLUSTER_NAME"
echo "✓ Cluster deleted"