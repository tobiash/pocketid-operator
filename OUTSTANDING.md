# Outstanding Issues

## Housekeeping

### 1. Teardown script can't find `kind`

`test/harness/teardown-e2e.sh` calls bare `kind` but the binary lives at `bin/kind`. Should use `export PATH="$PROJECT_ROOT/bin:$PATH"` like `setup-e2e.sh` does.

**File:** `test/harness/teardown-e2e.sh`

### 2. HTTPRoute test leaves orphaned OIDC client

`TestHTTPRoute` creates an OIDC client CR with an ownerReference on the HTTPRoute, but removing the OIDC annotation from the route doesn't delete the client. This blocks namespace deletion during cleanup.

**File:** `test/e2e/httproute_test.go`

### 3. Integration test race conditions (8/11 failures)

The integration tests run a background controller manager that watches and reconciles CRs, while tests also manually call reconcile functions. This causes conflict errors when both try to update the same object simultaneously. Pre-existing issue, not caused by v2 API changes.

**File:** `test/integration/suite_test.go`

**Possible fixes:**
- Pause the background controller before manual reconcile calls
- Use separate namespaces per test
- Remove the background controller and only use manual reconcile in tests

## Consistency

### 4. Missing `updateErrorStatus` on deletion error paths

The deletion paths in all controllers (group, user, OIDC client, instance) return errors directly without setting status conditions. This is a consistency gap — the happy path and other error paths all set conditions, but deletion failures don't.

**Files:**
- `internal/controller/pocketidusergroup_controller.go` (~line 87)
- `internal/controller/pocketiduser_controller.go` (~line 91)
- `internal/controller/pocketidoidcclient_controller.go` (~line 76)

### 5. Missing re-fetch before status update in user and OIDC client controllers

The group controller already re-fetches the object before `r.Status().Update()` to avoid conflict errors. The user and OIDC client controllers don't do this, making them susceptible to the same conflict pattern (finalizer `r.Update` changes resourceVersion, then `r.Status().Update` fails).

**Files:**
- `internal/controller/pocketiduser_controller.go` (~line 217)
- `internal/controller/pocketidoidcclient_controller.go` (~line 223)

## Not blocking

None of these block deploying to a real cluster. The core reconciliation logic (create, update, delete, group membership, onboarding) all pass end-to-end tests.
