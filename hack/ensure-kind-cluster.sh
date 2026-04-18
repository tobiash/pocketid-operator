#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Default value for the cluster name, can be overridden by env var
CLUSTER_NAME="${KIND_CLUSTER_NAME:-pocketid-test}"
# Config path, defaults to local bin directory
KUBECONFIG_PATH="${KIND_KUBECONFIG:-$(pwd)/bin/kind.kubeconfig}"
KIND_CONFIG="${KIND_CONFIG:-test/harness/kind-config.yaml}"

# Ensure directory for kubeconfig exists
mkdir -p "$(dirname "${KUBECONFIG_PATH}")"

if ! command -v kind &> /dev/null; then
  echo "kind is not installed. Please install kind."
  exit 1
fi

echo "Ensuring Kind cluster '${CLUSTER_NAME}' exists..."

# Check if cluster exists
if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "Cluster '${CLUSTER_NAME}' not found. Creating..."
  kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}" --kubeconfig "${KUBECONFIG_PATH}"
else
  echo "Cluster '${CLUSTER_NAME}' already exists."
  # Ensure kubeconfig is exported even if cluster exists (in case it was created without the flag previously or we just want to update the file)
  # kind export kubeconfig --name "${CLUSTER_NAME}" --kubeconfig "${KUBECONFIG_PATH}"
  # Actually, 'kind create' with --kubeconfig writes it. 'kind export' also writes it.
  echo "Exporting kubeconfig to ${KUBECONFIG_PATH}..."
  kind export kubeconfig --name "${CLUSTER_NAME}" --kubeconfig "${KUBECONFIG_PATH}"
fi

echo "Cluster '${CLUSTER_NAME}' is ready."
echo "Kubeconfig is at: ${KUBECONFIG_PATH}"
echo "To use this cluster with kubectl, run:"
echo "export KUBECONFIG=${KUBECONFIG_PATH}"
