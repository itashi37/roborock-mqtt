# health-probes Specification

## Purpose
TBD - created by archiving change device-availability-health. Update Purpose after archive.
## Requirements
### Requirement: Liveness reflects unrecoverable-stuck state

The bridge SHALL expose an HTTP liveness endpoint that returns a success status (2xx) while the bridge is healthy, and a failure status (non-2xx) when the bridge is unrecoverably stuck — defined as: not authenticated to the Roborock cloud, OR every managed device disconnected — continuously for longer than a configured grace window. This lets a k8s liveness probe restart the pod, which is the proven recovery for an expired cloud session.

#### Scenario: Healthy bridge

- **WHEN** the bridge is authenticated and at least one device is connected
- **THEN** the liveness endpoint returns a 2xx status

#### Scenario: Stuck beyond the grace window

- **WHEN** the bridge has been not-authenticated, or all devices have been disconnected, continuously for longer than the grace window
- **THEN** the liveness endpoint returns a non-2xx status

#### Scenario: Unauthenticated bridge

- **WHEN** the bridge is not authenticated to the Roborock cloud beyond the grace window
- **THEN** the liveness endpoint returns a non-2xx status

### Requirement: Transient disconnects do not fail liveness

The bridge SHALL keep the liveness endpoint successful while a disconnect is within the grace window, so that transient cloud blips and normal reconnects do not trigger a pod restart loop.

#### Scenario: Brief disconnect within grace window

- **WHEN** all devices disconnect but the outage has lasted less than the grace window
- **THEN** the liveness endpoint still returns a 2xx status

#### Scenario: Recovery resets the timer

- **WHEN** a device reconnects after a disconnect shorter than the grace window
- **THEN** the unhealthy timer is cleared and liveness continues to return 2xx

### Requirement: Health detail remains observable

The bridge SHALL continue to expose health detail (authentication state, per-device connection state, timestamp) in a machine-readable response body for diagnostics, independent of the pass/fail status code used by the probe.

#### Scenario: Diagnostic body present

- **WHEN** the health/liveness endpoint is queried
- **THEN** the response body reports authentication state and per-device connection state

### Requirement: Grace window is configurable

The grace window before liveness fails SHALL be configurable, with a safe default in the 3–5 minute range.

#### Scenario: Default grace window

- **WHEN** no grace window is configured
- **THEN** the bridge uses a default in the 3–5 minute range

