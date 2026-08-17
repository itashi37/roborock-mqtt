package roborock

import "testing"

func vectorMapWithRobot(x, y int) *VectorMap {
	return &VectorMap{
		Robot: &VectorPosition{X: x, Y: y},
		Rooms: []VectorRoom{
			{ID: 12, Spans: []VectorSpan{{X: 0, Y: 5, W: 10}}},
			{ID: 23, Spans: []VectorSpan{{X: 10, Y: 5, W: 8}, {X: 12, Y: 6, W: 4}}},
		},
	}
}

func TestFindCurrentRoomUsesExplicitRobotPositionAndSpans(t *testing.T) {
	room := FindCurrentRoom(vectorMapWithRobot(14, 5), map[string]string{"23": "Cuisine"})
	if room == nil {
		t.Fatal("expected a room, got nil")
	}
	if room.ID != 23 || room.Name != "Cuisine" {
		t.Fatalf("got %+v, want id 23 named Cuisine", room)
	}
}

func TestFindCurrentRoomSpanBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		x      int
		wantID int
	}{
		{name: "left edge inclusive", x: 10, wantID: 23},
		{name: "right edge exclusive belongs to no room", x: 18, wantID: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			room := FindCurrentRoom(vectorMapWithRobot(tt.x, 5), nil)
			if tt.wantID == 0 {
				if room != nil {
					t.Fatalf("got %+v, want nil", room)
				}
				return
			}
			if room == nil || room.ID != tt.wantID {
				t.Fatalf("got %+v, want room %d", room, tt.wantID)
			}
		})
	}
}

func TestFindCurrentRoomFallsBackToGeneratedName(t *testing.T) {
	room := FindCurrentRoom(vectorMapWithRobot(14, 5), nil)
	if room == nil || room.Name != "Room 23" {
		t.Fatalf("got %+v, want fallback name Room 23", room)
	}
}

func TestFindCurrentRoomReturnsNilWithoutReliablePosition(t *testing.T) {
	tests := []struct {
		name string
		vm   *VectorMap
	}{
		{name: "nil map", vm: nil},
		{name: "no explicit robot", vm: &VectorMap{Path: [][2]int{{14, 5}}}},
		{name: "outside rooms", vm: vectorMapWithRobot(99, 99)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if room := FindCurrentRoom(tt.vm, nil); room != nil {
				t.Fatalf("got %+v, want nil", room)
			}
		})
	}
}

func TestMergeRoomNamesConfiguredNamesOverrideAPI(t *testing.T) {
	merged := MergeRoomNames(
		map[string]string{"12": "Salon", "23": "Kitchen"},
		map[string]string{"23": "Cuisine", "42": "Bureau"},
	)

	want := map[string]string{"12": "Salon", "23": "Cuisine", "42": "Bureau"}
	for id, name := range want {
		if merged[id] != name {
			t.Errorf("room %s: got %q, want %q", id, merged[id], name)
		}
	}
}

func TestCurrentRoomFromVectorJSON(t *testing.T) {
	data := []byte(`{"width":20,"height":10,"rooms":[{"id":23,"color":"#fff","center":[14,5],"spans":[{"x":10,"y":5,"w":8}]}],"walls":null,"floor":null,"path":[[1,1]],"robot":{"x":14,"y":5}}`)
	room, err := CurrentRoomFromVectorJSON(data, map[string]string{"23": "Cuisine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if room == nil || room.ID != 23 || room.Name != "Cuisine" {
		t.Fatalf("got %+v, want Cuisine (23)", room)
	}
}
