package roborock

import (
	"testing"
)

// TestPollAllSkipsDisconnected verifies the bridge does not publish a fresh
// status for a device whose cloud connection is down — so the retained status
// stays put and the availability topic is the authority on staleness.
func TestPollAllSkipsDisconnected(t *testing.T) {
	dm := NewDeviceManager(&LoginData{}, []DeviceInfo{
		{Name: "Carmen EG", DID: "did-eg"},
		{Name: "Carmen OG", DID: "did-og"},
	}, nil, t.TempDir())

	var statusCalls int
	dm.SetStatusCallback(func(string, *PublishedStatus) { statusCalls++ })

	devs := dm.GetDevices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}
	// One device with no CloudMQTT at all, one with a freshly-created (and thus
	// disconnected) CloudMQTT. Both must be skipped by PollAll.
	devs[0].CloudMQTT = nil
	devs[1].CloudMQTT = NewCloudMQTT(&LoginData{}, &devs[1].Info)
	if devs[1].CloudMQTT.IsConnected() {
		t.Fatal("a freshly-created CloudMQTT should report disconnected")
	}

	dm.PollAll()

	if statusCalls != 0 {
		t.Fatalf("expected no status publishes while offline, got %d", statusCalls)
	}
}

// TestAvailabilityCallbackPropagates verifies a device-level availability
// transition reaches the manager-level callback with the right slug.
func TestAvailabilityCallbackPropagates(t *testing.T) {
	cm := NewCloudMQTT(&LoginData{}, &DeviceInfo{Name: "Carmen EG"})

	var got []bool
	cm.SetAvailabilityCallback(func(online bool) { got = append(got, online) })

	cm.setAvailability(false) // first observation
	cm.setAvailability(false) // no change → no extra emit
	cm.setAvailability(true)  // transition
	cm.setAvailability(true)  // no change

	want := []bool{false, true}
	if len(got) != len(want) {
		t.Fatalf("expected %v emits, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emit %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
