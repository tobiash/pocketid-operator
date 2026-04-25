# PocketID Operator

A Kubernetes operator that manages [PocketID](https://github.com/pocket-id/pocket-id) identity provider resources using custom resource definitions.

## Overview

The operator automates lifecycle management of PocketID resources:

- **PocketIDInstance** — Deploys and configures a PocketID StatefulSet with secrets, config, service, and optional admin initialization
- **PocketIDOIDCClient** — Manages OIDC clients and stores credentials in Kubernetes secrets
- **PocketIDUser** — Manages users with group membership sync and onboarding token support
- **PocketIDUserGroup** — Manages user groups
- **HTTPRoute integration** — Automatically creates OIDC clients for HTTPRoutes annotated with `pocket-id.io/oidc-enabled`

Compatible with **PocketID v2**.

## Prerequisites

- Go 1.26+
- Docker or Podman
- kubectl
- Access to a Kubernetes 1.29+ cluster

## Installation

### Flux (OCI Artifact)

The operator publishes manifests as an OCI artifact to GHCR on every release and main branch push. Create an image pull secret for GHCR if the repo is private, then:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: pocketid-operator
  namespace: flux-system
spec:
  interval: 10m
  url: oci://ghcr.io/tobiash/pocketid-operator-manifests
  ref:
    semver: ">=0.0.0-0"
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: pocketid-operator
  namespace: flux-system
spec:
  interval: 10m
  prune: true
  wait: true
  sourceRef:
    kind: OCIRepository
    name: pocketid-operator
  path: ./
```

For pre-releases, pin the tag directly:

```yaml
spec:
  ref:
    tag: v0.1.0-rc.1
```

### Manual

```sh
make docker-build docker-push IMG=<your-registry>/pocketid-operator:latest
make deploy IMG=<your-registry>/pocketid-operator:latest
```

## Quick Start

### Create a PocketID Instance

```yaml
apiVersion: pocketid.tobiash.github.io/v1alpha1
kind: PocketIDInstance
metadata:
  name: my-instance
spec:
  appUrl: "https://auth.example.com"
  image: "ghcr.io/pocket-id/pocket-id:latest"
  replicas: 1
  trustProxy: true
  sessionDuration: 60
  database:
    provider: sqlite
  storage:
    pvc:
      size: "1Gi"
```

### Create an OIDC Client

```yaml
apiVersion: pocketid.tobiash.github.io/v1alpha1
kind: PocketIDOIDCClient
metadata:
  name: my-app
spec:
  instanceRef:
    name: my-instance
  name: My Application
  callbackURLs:
    - https://myapp.example.com/callback
  credentialsSecretRef:
    name: my-app-oidc-credentials
```

The operator creates the OIDC client in PocketID and stores `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and `OIDC_ISSUER_URL` in the referenced secret.

### Create Users and Groups

```yaml
apiVersion: pocketid.tobiash.github.io/v1alpha1
kind: PocketIDUserGroup
metadata:
  name: developers
spec:
  instanceRef:
    name: my-instance
  name: developers
  friendlyName: Development Team
---
apiVersion: pocketid.tobiash.github.io/v1alpha1
kind: PocketIDUser
metadata:
  name: jdoe
spec:
  instanceRef:
    name: my-instance
  username: jdoe
  email: jdoe@example.com
  firstName: John
  lastName: Doe
  displayName: John Doe
  userGroupRefs:
    - name: developers
```

### HTTPRoute OIDC Integration

Annotate an HTTPRoute to automatically create an OIDC client:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app
  annotations:
    pocket-id.io/oidc-enabled: "true"
    pocket-id.io/instance: "my-instance"
spec:
  hostnames:
    - myapp.example.com
```

The operator creates an OIDC client named `<route-name>-oidc`.

## Development

### Run Tests

```sh
# Unit + integration tests
make test

# End-to-end tests (requires podman or docker)
CONTAINER_TOOL=podman make test-e2e
```

### Run Locally

```sh
make dev
```

### Make Targets

Run `make help` for the full list.

## Architecture

All controllers follow a consistent pattern:

- **Finalizers** prevent deletion before cleanup in PocketID
- **Status conditions** are set on all paths (success, error, deletion) with machine-readable reasons
- **Re-fetch before status update** avoids conflict errors
- **Cross-namespace references** are validated against the instance's allowed namespaces

## License

Apache License 2.0
