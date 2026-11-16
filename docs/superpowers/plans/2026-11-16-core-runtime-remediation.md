# Core Runtime Compatibility Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep panel state, user isolation, traffic accounting, and configured policies correct across upgraded Xray, sing-box, and Hysteria2 runtime failures and retries.

**Architecture:** Keep panel fetches as pending snapshots until node state is applied, serialize controller lifecycle transitions, and give each core node explicit ownership of runtime resources. Protect mutable limiter/accounting state and prove behavior through mock-panel, lifecycle, race, protocol, and staging checks.

**Tech Stack:** Go 1.27, Go `testing`/`httptest`, `-race`, current XBoard UniProxy API, current pinned Xray/sing-box/Hysteria dependencies.

**Spec:** `docs/superpowers/specs/2026-11-16-core-runtime-remediation-design.md`

## Global Constraints

- Preserve all pre-existing work in the `upgrade/core-2026` working tree.
- Do not reset, stash, broadly stage, or overwrite unrelated local changes.
- Preserve `/api/v1/server/UniProxy/config`, `/user`, `/alivelist`, `/push`, and `/alive` request contracts.
- An empty successful user list means remove all users; a `nil` list means no new snapshot.
- Do not report success for limits, traffic, or node activation unless the runtime behavior is enforced and verified.
- Keep Xray and sing-box source snapshot patch allowlists and dependency pins consistent with the current compatibility checker.
- Do not enable new upstream options until their panel/config mapping and tests are present.
- Keep errors recoverable; data from the panel or node configuration must not cause process-level `panic` or `Fatal`.

---

## File and Workstream Map

| Area | Files | Responsibility |
| --- | --- | --- |
| Panel acknowledgement | `api/panel/panel.go`, `api/panel/node.go`, `api/panel/user.go` | Hold candidate ETags/body hashes and acknowledge only after apply. |
| Reconciliation and tasks | `node/controller.go`, `node/task.go`, `node/user.go`, `common/task/task.go` | Serialize apply/report/close; recover node/user state and preserve report batches. |
| Policy and counters | `limiter/limiter.go`, `limiter/rule.go`, `limiter/dynamic.go`, `common/counter/traffic.go`, `common/counter/conn.go` | Synchronize limiter state and make byte snapshots exact and retryable. |
| Core lifecycle | `core/selector.go`, `core/xray/node.go`, `core/sing/node.go`, `core/hy2/node.go`, `core/hy2/config.go`, `core/hy2/user.go` | Deterministic selection, per-node ownership, cleanup and user isolation. |
| Adapter verification | `core/sing/node.go`, `core/xray/inbound.go`, `core/xray/ss.go`, `api/panel/*_test.go`, `tools/core-upgrade/*` | Validate panel fields, document mappings and gate release evidence. |

## Task 1: Lock down panel acknowledgement behavior

**Files:**
- Modify: `api/panel/panel.go`
- Modify: `api/panel/node.go`
- Modify: `api/panel/user.go`
- Test: `api/panel/client_http_test.go`

**Interfaces:**
- Produces `CommitNodeInfo()` and `CommitUserList()` (or equivalently named methods) that move only the most recently fetched candidate token into the acknowledged token.
- `GetNodeInfo` and `GetUserList` continue returning `nil` on a true 304/no-change response.

- [ ] Add mock-server tests for changed body + ETag, identical body + changed ETag, 304, malformed body, and candidate fetch without acknowledgement. Assert the next request still sends the prior acknowledged ETag until commit.
- [ ] Run `go test ./api/panel -run 'Test.*(ETag|Snapshot|304)' -count=1`; confirm the new acknowledgement tests fail against the current client.
- [ ] Add separate pending node ETag/body-hash and pending user ETag fields; update acknowledged fields only in commit methods. A response whose body hash equals the already-applied hash can be committed without reapplying.
- [ ] Re-run the targeted tests and `go test ./api/panel -count=1`; all tests must pass.

## Task 2: Acknowledge only successfully applied controller snapshots

**Files:**
- Modify: `node/controller.go`
- Modify: `node/task.go`
- Test: `node/task_test.go`
- Test: `api/panel/client_http_test.go`

**Interfaces:**
- Controller start and reconcile paths call the panel commit methods only after all required fetches and core operations succeed.
- Errors remain visible to logs and retry state; failed work does not advance controller `info`, `userList`, or acknowledged ETags.

- [ ] Add controller tests for: config changed then alive-list request fails; node add fails; initial user add fails; user delete fails; user add fails after delete; a later retry succeeds. Assert the old ETag is sent again and the old in-memory snapshot remains authoritative until success.
- [ ] Run `go test ./node ./api/panel -run 'Test.*(Retry|Apply|ETag|Alive)' -count=1`; record expected failures before implementation.
- [ ] Move `c.info`, `c.userList`, limiter publication, and ETag commits to the success point. Preserve pending candidate snapshots when any prerequisite request or core mutation fails.
- [ ] Make user mutation retry-safe: repeat deletes/adds against the same target list without duplicating users, and update `c.userList` only after both operations complete.
- [ ] Re-run `go test ./node ./api/panel -count=1` and the package suite `go test ./...`.

## Task 3: Make node replacement recoverable

**Files:**
- Modify: `node/task.go`
- Modify: `node/controller.go`
- Modify: `core/interface.go` only if a validation/preparation method is needed by all cores
- Test: `node/task_test.go`

- [ ] Add a fake `Core` test double that records add/delete/user calls and can fail at each call. Cover node delete failure, new add failure, user add failure, and successful restoration.
- [ ] Run `go test ./node -run 'Test.*(NodeChange|Rollback|Restore)' -count=1`; confirm each failure case exposes current destructive behavior.
- [ ] Build the replacement snapshot and limiter candidate before deleting the live node. Retain old node info, user list, tag, limiter, interval settings, and pending traffic until activation commits.
- [ ] If activation fails after old-node deletion, recreate the old node/users and keep the panel candidate unacknowledged. Return a typed/contextual error and avoid `log.Panic`.
- [ ] Verify successful change commits exactly once and failed change can be retried by `nodeInfoMonitor` without process restart.

## Task 4: Make task close/restart single-owner and interval-safe

**Files:**
- Modify: `common/task/task.go`
- Test: `common/task/task_test.go`
- Modify: `node/controller.go`
- Modify: `node/task.go`
- Test: `node/task_test.go`

- [ ] Add deterministic tests with channels for close-during-execute, close-then-start, execute-error concurrent with close, repeated close, and zero interval rejection. Do not use sleeps to establish ordering.
- [ ] Run `go test ./common/task -count=1`; confirm race/panic tests fail before changes.
- [ ] Give each task generation its own stop and done channels. Never have an old goroutine read or close a channel installed by a later `Start`; `Close` cancels and joins the active generation.
- [ ] Validate intervals before task start and reject `Interval <= 0` with an error; ensure controller startup returns that error instead of silently skipping the task.
- [ ] Serialize node reconciliation against reporting and close with a controller lifecycle lock or one reconcile queue; update intervals through a synchronized task method rather than direct field writes.
- [ ] Run `go test -race ./common/task ./node -count=1` and repeat the deterministic lifecycle tests 100 times.

## Task 5: Fix byte accounting and report retry semantics

**Files:**
- Modify: `common/counter/conn.go`
- Modify: `common/counter/traffic.go`
- Modify: `core/sing/user.go`
- Modify: `core/xray/user.go`
- Modify: `core/hy2/user.go`
- Modify: `node/user.go`
- Test: `common/counter/*_test.go`
- Test: `node/task_test.go`

- [ ] Add counter tests proving multiple `Read` and `Write` calls accumulate the exact returned byte counts and proving an atomic snapshot does not erase a concurrent increment.
- [ ] Add reporting tests where `/push` and `/alive` first fail and then succeed; assert the retry contains the original traffic and online snapshot, including an empty alive map.
- [ ] Run `go test ./common/counter ./node -run 'Test.*(Counter|Traffic|Alive|Report)' -count=1`; confirm current implementation loses counts or pending batches.
- [ ] Change direct read/write accounting from overwrite to byte accumulation. Add a snapshot operation that atomically rotates or swaps each user's counters while preserving increments after the snapshot boundary.
- [ ] Keep a pending traffic batch until the panel acknowledges it; merge new bytes into the next pending batch without double-counting already acknowledged bytes.
- [ ] Keep online IP state until `/alive` succeeds and send an empty map when the snapshot is empty. Update the panel online filter using that report's traffic snapshot.
- [ ] Run `go test -race ./common/counter ./node ./core/sing ./core/xray ./core/hy2 -count=1`.

## Task 6: Synchronize limiter state and make configured policies effective

**Files:**
- Modify: `limiter/limiter.go`
- Modify: `limiter/rule.go`
- Modify: `limiter/dynamic.go`
- Modify: `core/hy2/logger.go`
- Modify: `core/hy2/hook.go`
- Modify: `core/sing/hook.go`
- Modify: `core/xray/app/dispatcher/default.go`
- Test: `limiter/*_test.go`
- Test: `core/hy2/*_test.go`

- [ ] Add tests for concurrent `CheckLimit` with user/rule/alive updates, invalid regex, dynamic speed update on an already-used user bucket, and speed-limit expiry restoring the base speed.
- [ ] Run `go test -race ./limiter ./core/hy2 ./core/sing ./core/xray/app/dispatcher -count=1`; save the initial race evidence.
- [ ] Compile rule updates into local slices and publish a complete immutable rule snapshot only after every regex compiles; return an error for invalid expressions.
- [ ] Protect ordinary `UserLimitInfo` fields and map/slice pointer publication. Ensure user add/delete updates `UUIDtoUID`, alive counts, online IP state, and speed buckets as one synchronized policy update.
- [ ] Reconfigure or replace an existing token bucket when the effective speed limit changes and when a dynamic limit expires.
- [ ] Implement HY2 rejection and shaping through runtime hooks that can enforce them. If a specific policy cannot be implemented through Hysteria 2.14.0 callbacks, reject that enabled config at startup and state the unsupported policy in the error.
- [ ] Run the targeted tests and race checks again; zero race reports and asserted policy behavior are required.

## Task 7: Scope core state per node and roll back partial core setup

**Files:**
- Modify: `core/selector.go`
- Modify: `core/xray/node.go`
- Modify: `core/sing/node.go`
- Modify: `core/hy2/node.go`
- Modify: `core/hy2/config.go`
- Modify: `core/hy2/user.go`
- Test: `core/selector_test.go`
- Test: `core/xray/*_test.go`
- Test: `core/sing/*_test.go`
- Test: `core/hy2/*_test.go`

- [ ] Add tests proving ambiguous VMess/VLESS selection is deterministic or rejected, duplicate core aliases are rejected, same UUID on two HY2 nodes cannot authenticate cross-node, and deleting one node does not remove the other node's user.
- [ ] Add injected-failure tests for Xray outbound build/registration after inbound registration and HY2 failures after UDP listener creation, request hook construction, outbound construction, masquerade startup, and server creation.
- [ ] Run `go test ./core/... -run 'Test.*(Selector|Partial|Cleanup|CrossNode)' -count=1`; establish current failures.
- [ ] Require explicit `CoreName`/core type when multiple cores match a protocol; reject duplicate names/types during `NewSelector` construction.
- [ ] Store per-node HY2 authenticator/user state. Close every opened listener on all error paths; make masquerade servers owned, stoppable, and non-fatal on listen errors.
- [ ] Roll back Xray inbound if outbound creation/registration fails. Make deletion attempt cleanup of both handlers and clear node maps/counters only after cleanup results are known; make repeated deletion safe.
- [ ] Clear Sing/Xray/HY2 per-node report thresholds, user mappings, counters, and runtime resources after successful node deletion.
- [ ] Run `go test -race ./core/... -count=1` and verify no listener remains open after each injected failure.

## Task 8: Validate panel-derived protocol configuration

**Files:**
- Modify: `core/sing/node.go`
- Modify: `core/sing/user.go`
- Modify: `core/xray/inbound.go`
- Modify: `core/xray/ss.go`
- Modify: `api/panel/node.go` if validation belongs at response decoding
- Test: `core/sing/*_test.go`
- Test: `core/xray/*_test.go`
- Test: `api/panel/node_test.go`

- [ ] Add table tests for valid/invalid Reality server port, max time difference, empty HTTP request path, invalid WebSocket early-data value, unsupported transport, undersized UUID for Shadowsocks 2022, and supported/unsupported Shadowsocks cipher values.
- [ ] Run `go test ./api/panel ./core/sing ./core/xray -run 'Test.*(Reality|Transport|Cipher|Path|Validation)' -count=1`; record current panic/ignored-error cases.
- [ ] Replace ignored parse errors and unchecked indexing with contextual validation errors; validate ports against 1–65535 and ensure empty path has a defined safe behavior or explicit error.
- [ ] Add explicit Xray `none/plain` compatibility if the target core supports the panel contract; otherwise fail node activation with a precise unsupported-cipher message before registering an inbound.
- [ ] Run `go test ./api/panel ./core/sing ./core/xray -count=1` and verify all malformed cases return errors without panic.

## Task 9: Prove endpoint, protocol, and release behavior

**Files:**
- Modify: `tools/core-upgrade/check.sh`
- Modify: `tools/core-upgrade/README.md`
- Create/update: `docs/core-upgrade-execution-checklist.md`
- Test: `api/panel/client_http_test.go`, core adapter tests, node tests

- [ ] Add a mapping table for Xray, Sing, and HY2: panel protocol/field, generated core option, runtime mutation path, statistics path, limit path, and verification test. Mark each upstream-only feature explicitly as mapped, unsupported, or deferred.
- [ ] Extend release checks to run `go test -race ./...`, endpoint mock contract tests, behavior tests, source snapshot allowlists, and Linux amd64/arm64 builds.
- [ ] Run local mock-panel scenarios for all five UniProxy endpoints. Verify `/config` and `/user` retry after downstream apply failure; verify traffic/alive payloads and empty-alive clearing.
- [ ] Run per-core protocol smoke tests for Xray VMess/VLESS/SS/Trojan, Sing VMess/VLESS/SS/Trojan/TUIC/AnyTLS/Hysteria/Hysteria2, and standalone HY2; exercise TCP, UDP/QUIC, TLS, Reality, WebSocket, gRPC, HTTP Upgrade, and XHTTP where currently supported.
- [ ] Run `go test ./...`, `go test -race ./...`, and `tools/core-upgrade/check.sh all release`; verify the report records each required evidence source and fails when panel/protocol evidence is absent.
- [ ] Before production rollout, run the same scenarios against staging XBoard and retain the old artifact/config for rollback. Do not label a live endpoint or protocol supported from compile-only evidence.

## Plan Self-Review

- Panel fetch/apply acknowledgement: Tasks 1–3 and 9.
- Node rollback, core resource ownership, deterministic selection, and per-node identity: Tasks 3 and 7.
- Task lifecycle and controller races: Task 4.
- Traffic accounting, panel report retry, and empty alive state: Task 5.
- Limiter synchronization and actual enforcement: Task 6.
- Panel field validation and unsupported cipher handling: Task 8.
- Live endpoint, supported protocol, race, build, and rollout evidence: Task 9.
- Every task names the files it changes and the command/test evidence required; no external panel access is assumed for local tests.
