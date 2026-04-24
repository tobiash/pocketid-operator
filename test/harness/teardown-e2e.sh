#!/bin/bash
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-pocketid-test}"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONTAINER_TOOL="${CONTAINER_TOOL:-podman}"

export PATH="$PROJECT_ROOT/bin:$PATH"

if [ "$CONTAINER_TOOL" = "podman" ]; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
fi

echo "Deleting kind cluster: $CLUSTER_NAME"
kind delete cluster --name "$CLUSTER_NAME"
echo "Cluster deleted"