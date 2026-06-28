# device-availability Specification

## Purpose
TBD - created by archiving change device-availability-health. Update Purpose after archive.
## Requirements
### Requirement: Per-device cloud-connection state is tracked

The bridge SHALL maintain, per managed device, a boolean cloud-connection state derived from the cloud-MQTT lifecycle: `online` once the cloud-MQTT client connects and subscribes successfully, and `offline` on connection loss or a failed (re)connect — including the `not Authorized` case where the Roborock session has expired.

#### Scenario: Connection established

- **WHEN** a device's cloud-MQTT client connects and its subscription succeeds
- **THEN** the device's connection state becomes `online`

#### Scenario: Connection lost

- **WHEN** a device's cloud-MQTT connection is lost or a (re)connect attempt fails
- **THEN** the device's connection state becomes `offline`

#### Scenario: Reconnect rejected as not authorized

- **WHEN** a reconnect attempt is rejected by the broker (e.g. `not Authorized` after the cloud session expired)
- **THEN** the device's connection state remains `offline` until a connect+subscribe succeeds

### Requirement: Availability is published as a retained MQTT topic

The bridge SHALL publish each device's connection state to the local broker at `<topic>/<slug>/availability` as a retained message with payload `online` or `offline`, and SHALL publish on every state transition so consumers always see the current value (including after their own reconnect).

#### Scenario: Device goes offline

- **WHEN** a device's connection state transitions to `offline`
- **THEN** a retained `offline` message is published to `<topic>/<slug>/availability`

#### Scenario: Device comes back online

- **WHEN** a device's connection state transitions to `online`
- **THEN** a retained `online` message is published to `<topic>/<slug>/availability`

#### Scenario: Late subscriber sees current state

- **WHEN** a consumer subscribes to `<topic>/<slug>/availability` after the bridge has already published
- **THEN** the broker immediately delivers the retained current value

### Requirement: Stale status is not asserted while offline

While a device is `offline`, the bridge SHALL NOT publish fresh `<topic>/<slug>/status` updates that imply a live reading, so that the `availability` topic is the authority on staleness and consumers can mark the device unavailable rather than trust a stale `status`.

#### Scenario: No status churn while disconnected

- **WHEN** a device is `offline` and the poll loop runs
- **THEN** the bridge does not publish a new `status` message for that device

### Requirement: Hard crash flips devices offline via Last-Will

The bridge SHALL register an MQTT Last-Will on its local-broker publisher connection that publishes `offline` (retained) for the bridge's devices, so that an ungraceful bridge termination (crash, OOM, kill) also marks devices `offline` without the bridge running.

#### Scenario: Bridge process dies ungracefully

- **WHEN** the bridge's connection to the local broker drops without a clean disconnect
- **THEN** the broker publishes the retained Last-Will marking the device(s) `offline`

### Requirement: Web UI shows per-device connection state

The web UI SHALL display each device's current connection state (`online` / `offline`) as an indicator, and SHALL update it live as the state changes.

#### Scenario: Indicator reflects offline device

- **WHEN** a device is `offline`
- **THEN** the web UI shows that device with an offline/disconnected indicator

#### Scenario: Indicator updates live on recovery

- **WHEN** a device transitions from `offline` to `online` while the web UI is open
- **THEN** the indicator updates to online without a manual page reload

