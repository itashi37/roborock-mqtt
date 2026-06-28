## Why

When the Roborock cloud session expires, the cloud-MQTT client is rejected with `not Authorized` and the bridge gets permanently stuck: `PollAll()` silently skips disconnected devices, so the retained `home/roborock/<slug>/status` keeps asserting the last-known state (e.g. `charging`) forever. Downstream consumers (the wall-display Wall API) have no way to tell the data is stale — a vacuum that is actually running appears docked, and nothing prompts a recovery. The only fix today is a manual pod restart.

This change makes the bridge honest about its connection health: it publishes real availability, surfaces it in the web UI, and exposes a k8s liveness signal so the cluster auto-recovers the proven-fix restart.

## What Changes

- Track real per-device cloud-MQTT connection state from the existing connect / connection-lost / failed-reconnect transitions (the `CloudMQTT.IsConnected()` / `ManagedDevice.Online` state already exists but is never acted on).
- Publish a **retained** `home/roborock/<slug>/availability` topic (`online` / `offline`) reflecting that state, so consumers can mark a stale device unavailable instead of trusting the last `status`.
- Register an MQTT **Last-Will** (`offline`, retained) on the local-broker publisher connection so a hard bridge crash also flips devices `offline`.
- Surface the per-device `Online` connection state in the web UI as a connection indicator, updated live.
- Add a k8s **liveness** health endpoint that returns non-200 when the bridge is unrecoverably stuck (not authenticated, or all devices disconnected) for longer than a short grace window (~3–5 min), so k8s restarts the pod. The grace window prevents restart loops on transient cloud blips. The existing `/health` endpoint always returns 200 today.

## Capabilities

### New Capabilities
- `device-availability`: real per-device cloud-connection state tracking, published as a retained MQTT availability topic (with Last-Will) and shown in the web UI.
- `health-probes`: HTTP health endpoints whose status reflects whether the bridge is healthy vs. unrecoverably stuck, suitable for k8s liveness probing.

### Modified Capabilities
<!-- None — no existing specs in openspec/specs/. -->

## Impact

- **roborock-mqtt (Go)**: `roborock/mqtt.go` (connection-state transitions, Last-Will), `roborock/manager.go` (availability callback / aggregate health), `main.go` (publish `…/availability`, wire Last-Will on the local publisher), `web/web.go` (health endpoint semantics + connection indicator), `web/` frontend (per-device online badge).
- **MQTT contract**: new retained topic `home/roborock/<slug>/availability` consumed by the wall-display Wall API (follow-up change in wall-display2 maps `offline` → entity `unavailable`).
- **Deployment**: `homeserver-gitops/cluster/charts/mqtt/roborock` Helm chart gains a `livenessProbe` on the health endpoint (no probe today).
- **No breaking changes**: the `…/status` topic and existing `/health` path are preserved; availability is additive.
