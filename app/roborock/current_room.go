package roborock

import (
	"encoding/json"
	"fmt"
)

// CurrentRoom is the compact room payload published to local MQTT consumers.
type CurrentRoom struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// MergeRoomNames combines room names discovered through the Roborock API with
// user-configured overrides. Configured names take precedence.
func MergeRoomNames(apiNames, configuredNames map[string]string) map[string]string {
	merged := make(map[string]string, len(apiNames)+len(configuredNames))
	for id, name := range apiNames {
		merged[id] = name
	}
	for id, name := range configuredNames {
		merged[id] = name
	}
	return merged
}

// FindCurrentRoom locates the map's explicit robot position within the
// run-length encoded spans of a room. It deliberately does not infer the
// position from the cleaning path.
func FindCurrentRoom(vm *VectorMap, roomNames map[string]string) *CurrentRoom {
	if vm == nil || vm.Robot == nil {
		return nil
	}

	for _, room := range vm.Rooms {
		for _, span := range room.Spans {
			if vm.Robot.Y == span.Y && vm.Robot.X >= span.X && vm.Robot.X < span.X+span.W {
				name := roomNames[fmt.Sprintf("%d", room.ID)]
				if name == "" {
					name = fmt.Sprintf("Room %d", room.ID)
				}
				return &CurrentRoom{ID: room.ID, Name: name}
			}
		}
	}

	return nil
}

// CurrentRoomFromVectorJSON extracts the current room from the vector map JSON
// already stored by the device manager.
func CurrentRoomFromVectorJSON(data []byte, roomNames map[string]string) (*CurrentRoom, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var vm VectorMap
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("unmarshal vector map: %w", err)
	}
	return FindCurrentRoom(&vm, roomNames), nil
}
