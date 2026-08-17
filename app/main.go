package main

import (
	"encoding/json"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mqtt-home/roborock-mqtt/config"
	"github.com/mqtt-home/roborock-mqtt/roborock"
	"github.com/mqtt-home/roborock-mqtt/version"
	"github.com/mqtt-home/roborock-mqtt/web"
	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/mqtt"
)

var (
	deviceManager   *roborock.DeviceManager
	scheduleEngine     *roborock.ScheduleEngine
	notAtHomeStore     *roborock.NotAtHomeStore
	scheduleStore      *roborock.ScheduleStore
	maintenanceChecker *roborock.MaintenanceChecker
	webServer          *web.WebServer
	stopPolling        chan struct{}
	stopSchedule       chan struct{}
	dataDir            string
)

func publishDeviceMap(slug string, pngData []byte) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/map"
	mqtt.PublishAbsolute(topic, pngData, cfg.MQTT.Retain)
	logger.Debug("Published map", "device", slug, "topic", topic, "size", len(pngData))
}

func publishDeviceCurrentRoom(slug string, room *roborock.CurrentRoom) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/current_room"

	data, err := json.Marshal(room)
	if err != nil {
		logger.Error("Failed to marshal current room", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published current room", "device", slug, "topic", topic, "payload", string(data))
}

func publishDeviceScenes(slug string, scenes []roborock.Scene) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/scenes"

	data, err := json.Marshal(scenes)
	if err != nil {
		logger.Error("Failed to marshal scenes", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published scenes", "device", slug, "topic", topic, "count", len(scenes))
}

func publishDeviceSchedule(slug string, state *roborock.ScheduleState) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/schedule"

	data, err := json.Marshal(state)
	if err != nil {
		logger.Error("Failed to marshal schedule state", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published schedule state", "device", slug, "topic", topic, "dayType", state.ActiveDay)
}

// publishDeviceAvailability publishes a device's cloud-connection state to the
// local broker as a retained `<topic>/<slug>/availability` message. Consumers
// (e.g. the wall-display Wall API) use this to mark a device unavailable rather
// than trust a stale retained `<topic>/<slug>/status`.
func publishDeviceAvailability(slug string, online bool) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/availability"
	payload := "offline"
	if online {
		payload = "online"
	}
	// Availability is always retained so late subscribers see the current state.
	mqtt.PublishAbsolute(topic, payload, true)
	logger.Debug("Published availability", "device", slug, "topic", topic, "state", payload)
}

func publishDeviceStatus(slug string, status *roborock.PublishedStatus) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/status"

	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("Failed to marshal status", "error", err)
		return
	}

	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published status", "device", slug, "topic", topic)
}

func subscribeToCommands() {
	cfg := config.Get()

	for _, md := range deviceManager.GetDevices() {
		dev := md // capture
		topic := cfg.MQTT.Topic + "/" + dev.Slug + "/set"

		logger.Info("Subscribing to MQTT commands", "device", dev.Slug, "topic", topic)

		mqtt.Subscribe(topic, func(topic string, payload []byte) {
			logger.Debug("Received MQTT command", "device", dev.Slug, "topic", topic, "payload", string(payload))

			if dev.CloudMQTT == nil {
				logger.Warn("Device not connected, ignoring command", "device", dev.Slug)
				return
			}

			var cmd struct {
				Action   string `json:"action"`
				Segments []int  `json:"segments,omitempty"`
				Speed    string `json:"speed,omitempty"`
				Mode     string `json:"mode,omitempty"`
				Level    string `json:"level,omitempty"`
				SceneID  int    `json:"scene_id,omitempty"`
			}

			if err := json.Unmarshal(payload, &cmd); err != nil {
				logger.Error("Failed to parse command", "error", err)
				return
			}

			go dispatchCommand(dev, cmd.Action, cmd.Segments, cmd.Speed, cmd.Mode, cmd.Level, cmd.SceneID)
		})
	}
}

func dispatchCommand(dev *roborock.ManagedDevice, action string, segments []int, speed, mode, level string, sceneID int) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic in command processing", "panic", r)
		}
	}()

	var err error
	switch action {
	case "start":
		logger.Info("Starting vacuum", "device", dev.Slug)
		err = dev.CloudMQTT.Start()
	case "pause":
		logger.Info("Pausing vacuum", "device", dev.Slug)
		err = dev.CloudMQTT.Pause()
	case "dock":
		logger.Info("Sending vacuum to dock", "device", dev.Slug)
		err = dev.CloudMQTT.Dock()
	case "segment_clean":
		logger.Info("Starting segment clean", "device", dev.Slug, "segments", segments)
		deviceManager.NoteSegmentClean(dev.Slug, segments)
		err = dev.CloudMQTT.SegmentClean(segments)
	case "set_fan_speed":
		logger.Info("Setting fan speed", "device", dev.Slug, "speed", speed)
		err = dev.CloudMQTT.SetFanSpeed(speed)
	case "set_mop_mode":
		logger.Info("Setting mop mode", "device", dev.Slug, "mode", mode)
		err = dev.CloudMQTT.SetMopMode(mode)
	case "set_water_box":
		logger.Info("Setting water box level", "device", dev.Slug, "level", level)
		err = dev.CloudMQTT.SetWaterBox(level)
	case "scene":
		logger.Info("Executing scene", "device", dev.Slug, "sceneID", sceneID)
		deviceManager.NoteSceneStarted(dev.Slug, sceneID)
		err = deviceManager.ExecuteScene(sceneID)
	default:
		logger.Warn("Unknown action", "device", dev.Slug, "action", action)
		return
	}

	if err != nil {
		logger.Error("Command failed", "device", dev.Slug, "action", action, "error", err)
	}
}

// startBridge initializes the MQTT bridge after successful authentication.
func startBridge(restClient *roborock.Client) {
	cfg := config.Get()

	// Start local MQTT
	mqtt.Start(cfg.MQTT, "roborock_mqtt")

	// Initialize maintenance checker
	maintenanceChecker = roborock.NewMaintenanceChecker(dataDir)

	// Create device manager for all devices
	deviceManager = roborock.NewDeviceManager(restClient.GetLoginData(), restClient.GetDevices(), restClient, dataDir)
	deviceManager.SetStatusCallback(func(slug string, status *roborock.PublishedStatus) {
		publishDeviceStatus(slug, status)
		// Push live status (including the cleaning ETA) to connected web clients.
		if webServer != nil {
			webServer.BroadcastDeviceStatus(slug, status)
		}
		// Check maintenance thresholds — only when consumable data was actually fetched
		// (if ALL values are zero, the poll likely failed)
		c := status.Consumables
		if c.MainBrushWorkTime > 0 || c.SideBrushWorkTime > 0 || c.FilterWorkTime > 0 || c.SensorDirtyTime > 0 || c.DustCollectionWorkTimes > 0 {
			if dev := deviceManager.GetDevice(slug); dev != nil {
				maintenanceChecker.Check(dev.Info.Name, &status.ConsumablePercents, &status.Consumables)
			}
		}
	})
	deviceManager.SetMapCallback(func(slug string, pngData []byte) {
		publishDeviceMap(slug, pngData)

		dev := deviceManager.GetDevice(slug)
		if dev == nil {
			return
		}
		apiNames := restClient.GetRoomNameMap()
		configuredNames := cfg.Roborock.RoomNames[dev.Info.Name]
		roomNames := roborock.MergeRoomNames(apiNames, configuredNames)
		room, err := roborock.CurrentRoomFromVectorJSON(dev.GetVectorMapJSON(), roomNames)
		if err != nil {
			logger.Warn("Failed to determine current room", "device", slug, "error", err)
			return
		}
		publishDeviceCurrentRoom(slug, room)
	})
	deviceManager.SetAvailabilityCallback(func(slug string, online bool) {
		publishDeviceAvailability(slug, online)
		if webServer != nil {
			webServer.BroadcastDeviceAvailability(slug, online)
		}
	})

	// Seed a retained `offline` for every device before connecting, so the
	// availability topic is never absent and consumers start from a safe default.
	for _, md := range deviceManager.GetDevices() {
		publishDeviceAvailability(md.Slug, false)
	}

	// Load cached maps from disk (available before first poll)
	deviceManager.LoadMapCaches()
	for _, md := range deviceManager.GetDevices() {
		if png := md.GetMapPNG(); png != nil {
			publishDeviceMap(md.Slug, png)
		}
	}

	// Connect all devices to Roborock cloud MQTT
	deviceManager.ConnectAll()

	// Publish scenes per device
	for _, md := range deviceManager.GetDevices() {
		if len(md.Scenes) > 0 {
			publishDeviceScenes(md.Slug, md.Scenes)
		}
	}

	// Initial poll
	deviceManager.PollAll()

	// Subscribe to local MQTT commands per device
	subscribeToCommands()

	// Start polling
	stopPolling = make(chan struct{})
	go deviceManager.StartPolling(time.Duration(cfg.Roborock.PollingInterval)*time.Second, stopPolling)

	// Initialize schedule engine (provisioned from config + user from data dir)
	notAtHomeStore = roborock.NewNotAtHomeStore(dataDir)
	scheduleStore = roborock.NewScheduleStore(dataDir)

	signals := roborock.NewSignalListener(
		cfg.Roborock.ScheduleSignals.PublicHoliday,
		cfg.Roborock.ScheduleSignals.Vacation,
	)
	signals.Subscribe()

	scheduleEngine = roborock.NewScheduleEngine(cfg.Roborock.Schedules, scheduleStore, deviceManager, signals, notAtHomeStore)

	scheduleCallback := func(slug string, state *roborock.ScheduleState) {
		publishDeviceSchedule(slug, state)
	}
	scheduleEngine.SetStateChangeCallback(scheduleCallback)
	scheduleEngine.SetActionCallback(scheduleCallback)

	signals.SetOnChange(func() {
		scheduleEngine.CheckDayTypeChanges()
	})

	stopSchedule = make(chan struct{})
	go scheduleEngine.StartTicker(stopSchedule)

	logger.Info("Bridge started", "devices", len(restClient.GetDevices()))
}

func main() {
	logger.Init("info", logger.Logger())
	logger.Info("roborock-mqtt", "version", version.Info())
	initPprof()

	if len(os.Args) < 2 {
		logger.Error("No configuration file specified")
		os.Exit(1)
	}

	configFile := os.Args[1]
	logger.Info("Configuration file", "path", configFile)

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		return
	}

	logger.SetLevel(cfg.LogLevel)

	// Data and session directories next to the config file
	dataDir = filepath.Dir(configFile)
	sessionDir := filepath.Join(dataDir, ".session")

	// Initialize Roborock REST client
	restClient := roborock.NewClient(cfg.Roborock.BaseURL, cfg.Roborock.Username, cfg.Roborock.Password, cfg.Roborock.ClientID)
	restClient.SetSessionDir(sessionDir)

	// Try to restore a saved session
	if restClient.LoadSession() {
		if !restClient.IsAuthenticated() {
			logger.Info("Saved session found but no devices, discovering...")
			if err := restClient.DiscoverDevice(); err != nil {
				logger.Warn("Device discovery failed, will retry via web UI", "error", err)
			} else {
				_ = restClient.SaveSession()
			}
		}
		if restClient.IsAuthenticated() {
			logger.Info("Using saved session, starting bridge...")
			startBridge(restClient)
		}
	} else {
		logger.Info("No saved session. Waiting for authentication via web UI...")
	}

	// Start web server (always, needed for login UI when not authenticated)
	webServer = web.NewWebServer(deviceManager, restClient, func() {
		startBridge(restClient)
		webServer.SetDeviceManager(deviceManager)
		if scheduleEngine != nil {
			webServer.SetScheduleEngine(scheduleEngine)
			webServer.SetNotAtHomeStore(notAtHomeStore)
			webServer.SetScheduleStore(scheduleStore)
			scheduleEngine.SetStateChangeCallback(func(slug string, state *roborock.ScheduleState) {
				publishDeviceSchedule(slug, state)
				webServer.BroadcastScheduleState(slug, state)
			})
			scheduleEngine.SetActionCallback(func(slug string, state *roborock.ScheduleState) {
				publishDeviceSchedule(slug, state)
				webServer.BroadcastScheduleState(slug, state)
			})
		}
	})

	// Wire schedule engine to web server if bridge already started
	if scheduleEngine != nil {
		webServer.SetScheduleEngine(scheduleEngine)
		webServer.SetNotAtHomeStore(notAtHomeStore)
		webServer.SetScheduleStore(scheduleStore)
		scheduleEngine.SetStateChangeCallback(func(slug string, state *roborock.ScheduleState) {
			publishDeviceSchedule(slug, state)
			webServer.BroadcastScheduleState(slug, state)
		})
		scheduleEngine.SetActionCallback(func(slug string, state *roborock.ScheduleState) {
			publishDeviceSchedule(slug, state)
			webServer.BroadcastScheduleState(slug, state)
		})
	}

	go func() {
		port := cfg.Web.Port
		if port == 0 {
			port = 8080
		}
		logger.Info("Web interface available", "url", "http://localhost:"+strconv.Itoa(port))
		if err := webServer.Start(port); err != nil {
			logger.Error("Failed to start web server", "error", err)
		}
	}()

	logger.Info("Application ready")
	roborock.SendEmail("[Info] roborock-mqtt started", "The roborock-mqtt application has started and is ready.")

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel

	if stopSchedule != nil {
		close(stopSchedule)
	}
	if stopPolling != nil {
		close(stopPolling)
	}
	if deviceManager != nil {
		deviceManager.DisconnectAll()
	}
	logger.Info("Shutdown complete")
}

func initPprof() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}
