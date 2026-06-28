## 1. Connection-state tracking (CloudMQTT)

- [x] 1.1 Add an availability callback to `CloudMQTT` (mirror `SetStatusCallback`): `SetAvailabilityCallback(func(online bool))`, stored on the struct.
- [x] 1.2 Invoke the callback with `true` from `onConnect` (after the subscribe succeeds) and with `false` from `onConnectionLost`.
- [x] 1.3 Invoke the callback with `false` from the failed branch of `forceReconnect` (the `not Authorized` / reconnect-error path) and from `Disconnect()`.
- [x] 1.4 Ensure an initial `false` (offline) is emitted before the first successful connect so the state is never undefined. (Handled via seed publish in 2.3 + change-only `setAvailability`.)

## 2. Publish availability to the local broker (main.go / manager.go)

- [x] 2.1 Add `publishDeviceAvailability(slug string, online bool)` in `main.go` publishing `online`/`offline` to `cfg.MQTT.Topic + "/" + slug + "/availability"` as **retained**.
- [x] 2.2 Wire each device's `CloudMQTT.SetAvailabilityCallback` (in `manager.go` `ConnectAll`) → `DeviceManager.onAvailability` → `publishDeviceAvailability` (set in `startBridge`).
- [x] 2.3 Publish an initial retained `offline` for every device at bridge startup, before connect.
- [x] 2.4 In `PollAll()` confirm no `status` is published while a device is disconnected (already skips; `TestPollAllSkipsDisconnected` covers it).

## 3. Last-Will for hard-crash detection

> NOTE: the `mqtt-gateway` library already registers a retained Last-Will at
> `<topic>/bridge/state` = `offline` and publishes `online` on connect, so the
> hard-crash signal is provided by the library — no new Will needed in this repo.

- [x] 3.1 Bridge-level retained Last-Will exists via `mqtt-gateway` at `home/roborock/bridge/state` (`offline`).
- [x] 3.2 Library publishes retained `online` on (re)connect to the same topic.
- [x] 3.3 Consumer rule documented in `design.md`: a device is available only when bridge (`bridge/state`) AND per-device (`<slug>/availability`) are `online`.

## 4. Web UI connection indicator

- [x] 4.1 `GetSummaries()` already carries per-device `Online`; added `BroadcastDeviceAvailability` (SSE `type:"availability"`) on transitions + initial availability event on SSE connect.
- [x] 4.2 The web UI already renders an online/offline dot (`DeviceSwitcher.tsx`); kept it, now fed by live state.
- [x] 4.3 `useSSE` tracks `availabilities`; `App.tsx` overlays it onto the device list so the dot updates live without a refetch.

## 5. Health / liveness endpoint

- [x] 5.1 Added `DeviceManager.ConnectedCount()` / `ConnectionStates()`; `WebServer` tracks `unhealthySince *time.Time` (set when not-auth OR zero connected; cleared when auth AND ≥1 connected).
- [x] 5.2 Added configurable grace window (`web.liveness_grace_seconds`, default 4 min).
- [x] 5.3 Added `/api/livez` (200 while healthy/within grace, 503 once stuck beyond grace); `/api/health` stays always-200 with auth + per-device connection state in the body.
- [x] 5.4 `evaluateLiveness` extracted as a pure function; `liveness_test.go` covers healthy→200, within-grace→200, stuck→503, recovery resets timer.

## 6. Deployment (Helm chart)

- [x] 6.1 Added `livenessProbe` (httpGet `/api/livez`, port `http`) to `homeserver-gitops/cluster/charts/mqtt/roborock/chart/templates/deployment.yaml`.
- [x] 6.2 Parameterized probe timing in `values.yaml` (enabled/path/initialDelay/period/timeout/failureThreshold) with a 60s initial delay for first login.
- [x] 6.3 Verified the rendered manifest (`helm template -f values/values.yaml`) emits the probe; documented that grace (~240s) + probe (~90s) ≈ 5.5 min to restart.

## 7. Verification

- [x] 7.1 `go build ./...`, `go vet ./...`, `go test ./...` green in roborock-mqtt; frontend `tsc --noEmit` + `vite build` green.
- [ ] 7.2 Manually verify on the live bridge: expire the cloud session → `…/availability` flips to `offline`, `/api/livez` returns 503 after the grace window; restart → `online` + 200. (Requires deploying this build.)
- [ ] 7.3 Confirm the retained `…/availability` topic is observable via `mosquitto_sub` and reflects real state. (Requires deploying this build.)
