## Context

The bridge consumes the Roborock **cloud** MQTT (per device, `CloudMQTT` in `roborock/mqtt.go`) and republishes device state to the **local** broker (`home/roborock/<slug>/…` via `mqtt.PublishAbsolute` in `main.go`). MQTT credentials for the cloud connection are derived from the login session (`deriveMQTTCredentials(RRIoT.UserID, SessionID, MQTTKey)`); when that session expires the broker rejects reconnect with `not Authorized`. The reconnect paths (`forceReconnect`, the 90-min proactive reconnect, paho auto-reconnect) reuse the same stale `loginData` and never re-login, so the bridge stays stuck. Meanwhile `PollAll()` skips disconnected devices (`manager.go:234`), so the retained `…/status` keeps its last value and consumers can't tell it's stale.

Connection state already exists (`CloudMQTT.IsConnected()`, `ManagedDevice.Online`) but is only read by `GetDeviceStatuses`. The `/health` endpoint (`web/web.go:432`) always returns 200. The deployment Helm chart (`homeserver-gitops/cluster/charts/mqtt/roborock`) defines no probes.

## Goals / Non-Goals

**Goals:**
- Make staleness observable: publish real per-device availability so the Wall API can show `unavailable` instead of a stale `docked`.
- Auto-recover the stuck-session case via a k8s liveness restart (the proven manual fix), without restart loops on transient blips.
- Surface connection state in the web UI.

**Non-Goals:**
- Fixing the *root* re-auth gap (making `forceReconnect` re-login on `not Authorized`). That is a separate, larger change; this one makes the failure observable and auto-recoverable. (Captured as an open question / follow-up.)
- Changing the `…/status` schema or the cloud-MQTT command path.
- The wall-display2 consumption change (separate follow-up change mapping `offline` → entity `unavailable`).

## Decisions

### 1. Availability as a dedicated retained topic, not a field on `status`
Publish `home/roborock/<slug>/availability` = `online`/`offline`, retained. Rationale: a separate topic lets consumers treat availability as the staleness authority independent of the (possibly stale, retained) `status`. It also maps cleanly onto HA-style `availability` semantics the Wall API already understands (entity → `unavailable`). Alternative considered: add `"available": false` into the `status` JSON — rejected because the whole problem is that `status` stops updating while offline, so the flag inside it would itself be stale.

### 2. Drive transitions from the existing connection lifecycle
Emit availability from the points that already flip `connected`: `onConnect` (→ online, after subscribe), `onConnectionLost` (→ offline), and the failed branch of `forceReconnect` (→ offline). Expose a callback from `CloudMQTT` (mirroring `SetStatusCallback`) so `manager.go` / `main.go` publish without `mqtt.go` importing the local publisher. Publish the initial `offline` at startup before the first successful connect, so the topic is never absent.

### 3. Last-Will on the local publisher connection
Register an MQTT LWT (`SetWill`) on the **local-broker** publisher client (the one the bridge owns under its control), payload `offline`, retained, per device topic. Rationale: covers ungraceful death (crash/OOM/kill) that no in-process transition can. Constraint: a single client can carry one Will; for multiple devices we either (a) use one combined bridge-level availability Will plus per-device app-published availability, or (b) ensure the publisher connection’s Will covers each device topic. Decision: bridge-level Will topic `home/roborock/bridge/availability` for the hard-crash signal, AND per-device app-published `…/availability` for normal transitions; the Wall API treats a device as available only when both the bridge and the device are online. (If the local publisher is shared infra without per-bridge Will control, fall back to bridge-level Will only — see open questions.)

### 4. Liveness = stuck-for-longer-than-grace, with a healthy-timer reset
Track an `unhealthySince *time.Time`: set when (not authenticated) OR (no device connected) first becomes true, cleared as soon as the bridge is authenticated AND ≥1 device is connected. Liveness returns non-2xx only when `unhealthySince != nil && now-unhealthySince > grace`. Default grace 4 min (configurable via config). Rationale: a flat "fail when disconnected" would restart-loop on every cloud blip; the grace window matches the proven fix (restart only when genuinely stuck). Use a **liveness** probe (restart), not readiness, because the device has no consumers to gate — the only useful action is a restart.

### 5. Probe wiring in the Helm chart
Add a `livenessProbe` (httpGet on the health path, port 8080) to `chart/templates/deployment.yaml`, parameterized in `values.yaml` (path, initialDelay, period, failureThreshold) so probe timing composes with the in-app grace window. Keep `initialDelaySeconds` generous to allow first login.

## Risks / Trade-offs

- **Restart loop if Roborock cloud is down for an extended outage** → grace window + k8s `failureThreshold`/`periodSeconds` tuned so a restart only happens after sustained failure; a restart is cheap and harmless, and re-login on boot is the actual recovery.
- **Liveness masks the real bug (no MQTT re-auth)** → documented as the intended follow-up; this change is the safety net, not the cure. Tracked in Open Questions.
- **Multiple-device Will semantics** → mitigated by the bridge-level Will + per-device app availability split (Decision 3); worst case is a slightly coarser hard-crash signal.
- **Retained `offline` left behind after a clean shutdown** → publish `offline` on graceful `Disconnect()` too, and `online` only after a confirmed subscribe, so the retained value is always intentional.

## Migration Plan

1. Ship roborock-mqtt with availability publishing + health semantics (backward compatible: `…/status` and `/health` path unchanged; new topic + new status-code behavior are additive).
2. Roll the Helm chart with the `livenessProbe`.
3. Follow-up: wall-display2 Wall API maps `…/availability: offline` → vacuum entity `unavailable`.
- **Rollback**: remove the `livenessProbe` from the chart (reverts to never-restart); the extra MQTT topic is harmless to consumers that ignore it.

## Open Questions

- Should this change also fix `forceReconnect` to re-login via `restClient.Login()` on auth failure (root cause), or keep that strictly as a follow-up? (Leaning follow-up to keep this change focused on observability + auto-recovery.)
- Does the local broker / publisher client allow per-bridge LWT configuration, or must we rely on the bridge-level Will only?
- Exact health path: extend `/health` status-code behavior vs. add a dedicated `/livez` (keeping `/health` always-200 for humans). Leaning dedicated `/livez` to avoid surprising existing `/health` consumers.
