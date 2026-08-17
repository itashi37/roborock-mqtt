# roborock-mqtt

A bridge between Roborock vacuum robots and a local MQTT broker, with a built-in web UI for control and scheduling.

## Features

- Bridges Roborock devices to a local MQTT broker
- Web UI for device control, cleaning programs, and schedule management
- Context-aware cleaning schedules with four day types (normal, weekend, free day, not at home)
- MQTT signal integration for public holidays and vacation detection
- Per-device scene (program) execution
- Live device status, map visualization, and SSE real-time updates
- Multi-device support

## Quick Start

### Docker

```bash
docker run -d \
  -v /path/to/config:/var/lib/roborock-mqtt \
  -p 8080:8080 \
  pharndt/roborock-mqtt:latest
```

### From Source

```bash
cd app
make dev
```

This builds the frontend, builds the backend, and starts the server using `production/config/config.json`.

## Configuration

Create a `config.json` file:

```json
{
  "mqtt": {
    "url": "tcp://localhost:1883",
    "topic": "home/roborock",
    "qos": 2,
    "retain": true
  },
  "roborock": {
    "username": "your-email@example.com",
    "password": "your-password",
    "client_id": "<client-id>",
    "base_url": "https://euiot.roborock.com",
    "polling_interval": 30
  },
  "web": {
    "enabled": true,
    "port": 8080
  },
  "loglevel": "info"
}
```

Environment variables can be used in the config file with `${VAR_NAME}` syntax.

### Schedules

Schedules can be provisioned via the config file (read-only) or created in the web UI (persisted in the data directory).

Add a `schedules` section under `roborock` to provision schedules:

```json
{
  "roborock": {
    "schedules": {
      "My Vacuum": {
        "normal": [
          { "time": "09:00", "action": "scene", "scene_id": 12345 }
        ],
        "weekend": [
          { "time": "11:00", "action": "scene", "scene_id": 12345 }
        ],
        "free": [
          { "time": "10:00", "action": "start" }
        ],
        "notAtHome": []
      }
    },
    "schedule_signals": {
      "public_holiday": "rules/public-holiday",
      "vacation": "rules/free-day"
    }
  }
}
```

**Day type priority** (highest first): Not at Home > Weekend / Holiday > Free Day > Normal

- **Not at Home** is a manual toggle in the web UI, persisted in the data directory
- **Weekend** includes Saturday, Sunday, and days where the public holiday MQTT signal is `true`
- **Free Day** is active when the vacation MQTT signal is `true`
- **Normal** is the fallback for regular weekdays

All schedule times use the `Europe/Berlin` timezone.

### Data Directory

The application stores persistent state in the config file's parent directory:

```
config-dir/
  config.json
  .session/             # Roborock session data
  schedules/
    not-at-home.json    # Global not-at-home toggle state
    devices/            # User-created schedules (one JSON file per device)
```

In Kubernetes, mount this directory as a persistent volume.

## MQTT Topics

Published topics (per device):

| Topic | Description |
|-------|-------------|
| `{topic}/{slug}/status` | Device status (JSON) |
| `{topic}/{slug}/map` | Map image (PNG) |
| `{topic}/{slug}/map.json` | Vector map data (JSON) |
| `{topic}/{slug}/current_room` | Current robot room (`{"id":23,"name":"Cuisine"}` or `null` when unknown) |
| `{topic}/{slug}/scenes` | Available cleaning programs (JSON) |
| `{topic}/{slug}/schedule` | Schedule state (JSON) |

Command topic (per device):

| Topic | Description |
|-------|-------------|
| `{topic}/{slug}/set` | Send commands (JSON) |

Command payload format:

```json
{ "action": "start" }
{ "action": "pause" }
{ "action": "dock" }
{ "action": "segment_clean", "segments": [1, 2] }
{ "action": "set_fan_speed", "speed": "quiet|balanced|turbo|max" }
{ "action": "set_mop_mode", "mode": "standard|deep|deep_plus" }
{ "action": "set_water_box", "level": "off|mild|moderate|intense" }
{ "action": "scene", "scene_id": 12345 }
```

## Web UI

The web UI is available at `http://localhost:8080` (default port).

On first launch, you authenticate with your Roborock account via email verification code. The session is persisted so you don't need to re-authenticate on restart.

The main view shows:
- Device status with battery, fan speed, and mop mode
- Cleaning programs (scenes) as the primary action
- Pause/Dock buttons during active cleaning
- Controls page for manual start, fan speed, and mop mode settings
- Schedule summary with link to the full schedule page
- Interactive map

## Development

```bash
cd app

# Full build + run
make dev

# Frontend dev server (with hot reload)
make dev-frontend

# Backend only
make dev-backend

# Build Docker image
make docker
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/auth/status` | Authentication status |
| `POST` | `/api/auth/request-code` | Request verification code |
| `POST` | `/api/auth/login` | Login with code |
| `POST` | `/api/auth/logout` | Logout |
| `GET` | `/api/devices` | List devices |
| `GET` | `/api/devices/{slug}/status` | Device status |
| `POST` | `/api/devices/{slug}/start` | Start cleaning |
| `POST` | `/api/devices/{slug}/pause` | Pause cleaning |
| `POST` | `/api/devices/{slug}/dock` | Return to dock |
| `POST` | `/api/devices/{slug}/fan-speed` | Set fan speed |
| `POST` | `/api/devices/{slug}/mop-mode` | Set mop mode |
| `GET` | `/api/devices/{slug}/scenes` | List scenes |
| `POST` | `/api/devices/{slug}/scenes/{id}/execute` | Execute scene |
| `GET` | `/api/devices/{slug}/map` | Map PNG |
| `GET` | `/api/devices/{slug}/map.json` | Vector map JSON |
| `GET` | `/api/devices/{slug}/schedule` | Schedule state |
| `POST` | `/api/devices/{slug}/schedule` | Save user schedule |
| `DELETE` | `/api/devices/{slug}/schedule` | Delete user schedule |
| `PUT` | `/api/not-at-home` | Toggle not-at-home |
| `GET` | `/api/schedule/status` | Global schedule status |
| `GET` | `/api/events` | SSE event stream |
