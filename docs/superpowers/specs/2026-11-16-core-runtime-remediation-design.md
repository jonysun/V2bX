# Core Runtime Compatibility Remediation Design

**Date:** 2026-11-16  
**Repository:** `github.com/jonysun/V2bX`  
**Branch under work:** `upgrade/core-2026`  
**Status:** Approved by the user for implementation  
**Scope:** XBoard/V2Board synchronization, node lifecycle, Xray/Sing/Hysteria2 adapters, limits, accounting, and release verification.

## Goal

Make a release built from the upgraded cores preserve panel state, user isolation, traffic accounting, and configured policies through normal operation, failures, retries, and node reconfiguration. A successful compile is necessary but is not sufficient for release acceptance.

## Current evidence and boundaries

The report `tools/core-upgrade/reports/core-compat-all-release-20261116T055908Z-97.md` records successful Docker compilation, package tests, source snapshot checks, and compatibility tests for the patched Xray and sing-box sources. It does not contain a live XBoard exchange, protocol traffic evidence, failure-injection results, or race-detector results.

The working tree already contains uncommitted core-upgrade changes. Work must preserve those changes and edit only files required by this design. No reset, checkout of user changes, or broad staging is permitted.

## Architecture decisions

### 1. Treat panel snapshots as pending until applied

`api/panel.Client` will distinguish the last acknowledged ETag/body hash from the most recently fetched candidate. Fetching or decoding a response must not acknowledge it. The node controller acknowledges a node snapshot only after the corresponding node configuration and users are applied; it acknowledges a user snapshot only after all user mutations succeed. Failed dependent fetches and failed core operations retain the previous acknowledged token so the next monitor cycle retries.

The user and node paths remain compatible with the current UniProxy API: `/config`, `/user`, `/alivelist`, `/push`, and `/alive`. Empty user lists remain valid snapshots and mean removal of all current users.

### 2. Reconcile node state as a recoverable transition

Node replacement must validate/build as much as possible before removing the live node, preserve the last-known-good snapshot and accounting data, clean up partially created core objects, and attempt restoration when activation fails. Runtime failures are returned/logged as state transitions rather than panics. Pending panel snapshots are retried until success.

Core add/delete operations must be idempotent and own every resource they create. Xray inbound/outbound registration and Hysteria UDP/masquerade listeners must be rolled back on later failure. Sing/Xray/Hysteria2 per-node maps and counters must be removed on successful deletion.

### 3. Serialize controller lifecycle work and make task shutdown joinable

Each periodic task has one execution owner, a cancellation signal, and a completion signal. Closing a task prevents a new execution and waits for an active execution to finish. Intervals are validated as positive before starting. Node reconciliation and user reporting must not concurrently observe half-updated controller state.

### 4. Enforce limits at the adapter boundary and protect mutable policy state

Mutable limiter state will use synchronization or immutable snapshots. Rule updates compile completely before publication. A changed speed policy updates the active token bucket. Hysteria2 limits must be enforced through supported Hysteria hooks; if a configured policy cannot be enforced, startup must fail clearly instead of silently accepting it.

### 5. Account traffic with acknowledged snapshots

Connection counters add transferred bytes. Traffic reads use an atomic snapshot/rotation that cannot erase concurrent increments. A report batch remains pending until `/push` succeeds. Online IP snapshots remain pending until `/alive` succeeds, and an empty snapshot is reported to clear stale panel state.

### 6. Make core selection and panel-to-core conversion deterministic

When more than one configured core supports a protocol, selection follows an explicit configured core name/type or returns a clear ambiguity error. Panel-derived ports, durations, paths, ciphers, and transports are validated before activation. Version-specific core capabilities are recorded in a mapping matrix; upstream options are not claimed as supported until they have an adapter mapping and test.

## Required regression coverage

- Panel mock contract tests for JSON and MessagePack, ETag/304, changed ETag with identical body, malformed responses, empty users, timeout, HTTP errors, and endpoint failure between config fetch and apply.
- Controller tests for node changes, failed `/alivelist`, failed user add/delete, rollback, retries, empty-user removal, interval changes, close during execution, and overlapping report/reconcile triggers.
- Core lifecycle tests for partial Xray registration, Hysteria listener cleanup, idempotent deletion, and same UUID on two Hysteria2 nodes.
- Limiter tests for concurrent rule/user/online updates, expiry, dynamic speed changes on existing connections, and protocol enforcement behavior.
- Accounting tests for repeated reads/writes, reset concurrent with traffic, failed report retry, empty online report, and node replacement preserving pending bytes.
- Adapter conversion tables for every supported protocol/transport/security combination and invalid boundary values.
- `go test ./...`, `go test -race ./...`, core-upgrade release checks, Linux amd64/arm64 build/start smoke tests, and staging panel/protocol tests.

## Release blockers

Release is blocked by any cross-node authentication, configuration/user state that cannot retry, lost traffic or online snapshots after a failed request, a configured limit that is not enforced, a process panic from external panel/config input, a leaked listener after failed activation, a race-detector report in lifecycle paths, or a regression in a currently supported protocol.

## Rollout

Keep the existing release artifact and config as rollback points. First validate against the local panel simulator, then a staging XBoard node with representative protocols. Promote one low-traffic node only after a full pull/push cycle, protocol smoke tests, and restart checks pass. Expand only after accounting and limiter behavior match the previous release.
