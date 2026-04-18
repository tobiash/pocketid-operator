---
description: Standard procedure for working on the PocketID Operator
---

## Development Workflow

### Quick Start

1.  **Start a fresh environment:**
    ```bash
    ./test/harness/setup.sh --fresh
    ```

2.  **Start Local Development:**
    This command handles scaling down the in-cluster operator, setting up port-forwarding, and running the operator locally.
    ```bash
    make dev
    ```

3.  **Verify Changes:**
    Run automated verification scripts to test specific features.
    ```bash
    make verify-oidc
    ```

### Routine Tasks

1.  Read `/home/tobias/github/pocketid-operator.2/README.md` for project context
2.  Use `make generate manifests` after CRD type changes
3.  Run `make test` before committing
4.  For e2e tests: `make test-e2e` (uses podman)
5.  Never modify local kubeconfig; always use `KUBECONFIG` env var
6.  Reference pocket-id source at `/home/tobias/github/pocket-id` for API details
8.  **Testing Strategy:**
    - **Unit/Integration (EnvTest):** Use for controller logic validation without a full cluster.
      - Location: `internal/controller/*_test.go`
      - Run: `make test`
    - **End-to-End (E2E):** Use for full feature verification in a Kind cluster.
      - Location: `test/e2e/*_test.go`
      - Run: `make test-e2e` (Ensure `podman` is installed)
    - **Development:** Use `make verify-oidc` or similar scripts in `test/harness/` for quick local checks.

### Adding New Features
1.  **Define CRD:** Update `api/v1alpha1/` and run `make manifests generate`.
2.  **Implement Controller:** Add logic in `internal/controller/`.
3.  **Add Tests:**
    - Create an envtest suite in `internal/controller/` to test reconciliation logic.
    - Create an E2E test in `test/e2e/` to verify the feature in a real cluster.
4.  **Verify:** Run `make test` and `make test-e2e` to ensure no regressions.
