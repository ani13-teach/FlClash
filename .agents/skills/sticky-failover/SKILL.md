---
name: sticky-failover
description: Use when changing, debugging, testing, or packaging FlClash's [Sticky] proxy failover, its subscription override script, Core probe and selection behavior, persisted choices, or Flutter selection display. Use even when the request only mentions sticky groups, failback, regional nodes, or the arm64 APK for this feature.
---

# Sticky Failover

Read `tool/sticky-failover.md` for current behavior and user-facing limitations. Pair this skill with `core-platform` when a change also touches Core lifecycle, Android services, or platform build hooks. Follow `.agents/rules.md` and `.agents/commands.md` for repo-wide checks.

## Ownership

- `tool/sticky_override.js` transforms a subscription at apply time. It creates the entry, regional and manual `select` groups, excludes Hong Kong nodes by *name*, disables provider-wide periodic checks, and rewrites supported references. It does not run probes. Review `tool/sticky_override_test.cjs` before changing its group or rule semantics. Do not alter a user's separate override file.
- `core/sticky_engine.go` owns the probe-and-commit state machine; `core/sticky.go` adapts it to Mihomo selectors, probe sharing and persistence. A group is managed only if its name starts with the exact `[Sticky] ` prefix and it is a `select` group. Do not patch `core/Clash.Meta` for this feature.
- `core/common.go` and `core/hub.go` connect configuration, manual selection, start/stop and suspension. Flutter's `lib/models/common.dart` and `lib/manager/core_manager.dart` display the Core's selection; Flutter must not become another failover engine.

## Behavioral Contract

1. When the current direct proxy is healthy, keep it without testing all alternatives. Probe through Mihomo's `URLTest` at roughly one-second intervals, with a two-second request deadline; success at `https://cp.cloudflare.com` does not guarantee every site or UDP works.
2. On failure, start eligible direct-node probes concurrently and commit the first healthy result without waiting for slower candidates. The backend limits actual concurrent probes to 16; a full group scan is neither a latency ranking nor a reason to fail back automatically. If all candidates fail, keep the current selection and retry after the backoff.
3. Exclude DIRECT, REJECT and nested groups from automatic candidates. Do not interpret a queue timeout or canceled probe as a node failure, and do not cache such errors across groups. Share short-lived results by proxy *instance*, not name.
4. Preserve latest intent: a manual selection, replaced provider/group, new configuration, suspension or stop invalidates an older scan. Guard the commit against a changed snapshot and cancel unnecessary in-flight probes. Selection writes must follow the existing `configMu` then `selectMu` ordering.
5. Persist Sticky selections under `sticky-selections.json`, scoped to the SHA-256 of the applied `config.yaml`. Restore after the host's stale `selectedMap` is applied. UI reads the Core's `now` for Sticky groups and only refreshes while foregrounded.

## Change And Verification

1. Inspect the relevant code and `git status` first; retain existing edits. Keep generated files generated, and keep both native `build_assets` hooks enabled for an APK.
2. Add a focused Go test for state-machine or backend changes in `core/sticky_engine_test.go` or `core/sticky_test.go`. For parallel probes, cover a healthy late candidate behind blocked early candidates, cancellation/manual-choice races, and shared-probe cache behavior. Update `tool/sticky_override_test.cjs` for script changes; update focused Flutter tests only when display behavior changes.
3. From `core/`, run `CGO_ENABLED=0 go test .` and `CGO_ENABLED=0 go vet .`; from the repo root, run `node --test tool/sticky_override_test.cjs`. Use `flutter test`, not `dart test`, for affected Flutter tests. Run code generation after model/provider changes, never hand-edit generated output.
4. Build an Android APK when requested, using the repo's toolchain versions and `.agents/commands.md`. Verify the actual build exit status, ABI, included `libclash.so`/`librust_api.so`/`libsqlite3.so`, package ID and signing; do not mistake a previously built APK for a new one. Without a release keystore the existing Gradle fallback signs a `.dev` package with the debug key. Do not modify signing to make a build pass.
5. Report what was verified versus what still needs a device. Process death needs the Android service to restore Core; Doze does not guarantee strict one-second probes. A selection change affects new connections, not established ones.
