#!/bin/bash
# Tears down the test kind cluster
set -euo pipefail

CLUSTER_NAME="pocketid-test"

export KIND_EXPERIMENTAL_PROVIDER=podman

echo "🗑️ Deleting cluster: $CLUSTER_NAME"
kind delete cluster --name "$CLUSTER_NAME"
echo "✅ Cluster deleted"
