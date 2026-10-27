package realtime

import (
	"strconv"
	"testing"
)

func TestHubScopeReplayGapAndBackpressure(t *testing.T) {
	hub := NewHub(1)
	sub, err := hub.Subscribe("p1", []string{"orders"}, []string{"create"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Subscribe("p1", nil, nil, false); err != ErrConnectionLimit {
		t.Fatalf("connection limit: %v", err)
	}
	hub.Publish(Event{ProjectID: "p2", Collection: "orders", Action: "create", RecordID: "other"})
	hub.Publish(Event{ProjectID: "p1", Collection: "orders", Action: "create", RecordID: "r1", Data: map[string]any{"secret": true}})
	event := <-sub.C
	if event.ID != "2" || event.Data != nil || event.RecordID != "r1" {
		t.Fatalf("unexpected event: %+v", event)
	}
	for i := 0; i < 20; i++ {
		hub.Publish(Event{ProjectID: "p1", Collection: "orders", Action: "create", RecordID: strconv.Itoa(i)})
	}
	if hub.Dropped() == 0 {
		t.Fatal("full subscriber queue did not drop")
	}
	replay, _ := hub.Replay(sub, 0)
	if len(replay) == 0 || replay[0].Data != nil {
		t.Fatalf("replay: %+v", replay)
	}
	if _, gap := hub.Replay(sub, 0); gap {
		t.Fatal("zero cursor must not gap")
	}
	hub.Close(sub.ID)
	if err := hub.Update(sub.ID, nil, nil); err != ErrSubscription {
		t.Fatalf("closed subscription update: %v", err)
	}
}
