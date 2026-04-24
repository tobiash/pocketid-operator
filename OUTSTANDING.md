# Outstanding Issues

All previously identified issues have been resolved:

1. ~~Teardown script can't find `kind`~~ — Fixed: added PATH setup and container tool detection
2. ~~HTTPRoute test leaves orphaned OIDC client~~ — Fixed: explicit cleanup of OIDC client and HTTPRoute
3. ~~Integration test race conditions~~ — Fixed: removed background controller manager, tests use direct client
4. ~~Missing `updateErrorStatus` on deletion paths~~ — Fixed: all deletion error paths now set conditions
5. ~~Missing re-fetch before status update~~ — Fixed: user and OIDC client controllers re-fetch before status update

## Test Status

- Unit tests (internal/controller): PASS
- Integration tests (test/integration): 11/11 PASS
- E2E tests (test/e2e): 4/4 PASS
