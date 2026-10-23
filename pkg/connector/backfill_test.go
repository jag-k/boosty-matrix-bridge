package connector

import (
	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/bridgeconfig"
	"maunium.net/go/mautrix/bridgev2/database"
	"testing"
	"time"
)

func TestBackfillWindow(t *testing.T) {
	messages := []boosty.Message{{ID: 1, CreatedAt: 10}, {ID: 3, CreatedAt: 10}, {ID: 4, CreatedAt: 11}}
	for _, tc := range []struct {
		name    string
		anchor  *database.Message
		forward bool
		ids     []int
	}{
		{"initial", nil, true, []int{1, 3, 4}},
		{"catchup", &database.Message{ID: "3", Timestamp: time.Unix(10, 0)}, true, []int{4}},
		{"older", &database.Message{ID: "3", Timestamp: time.Unix(10, 0)}, false, []int{1}},
		{"missing forward anchor", &database.Message{ID: "2", Timestamp: time.Unix(10, 0)}, true, []int{3, 4}},
		{"missing backward anchor", &database.Message{ID: "2", Timestamp: time.Unix(10, 0)}, false, []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := messagesInBackfillWindow(messages, tc.anchor, tc.forward)
			if len(got) != len(tc.ids) {
				t.Fatalf("got %v, want %v", got, tc.ids)
			}
			for i, m := range got {
				if m.ID != tc.ids[i] {
					t.Fatalf("got ID %d want %d", m.ID, tc.ids[i])
				}
			}
		})
	}
}

func TestDMBackfillOverride(t *testing.T) {
	b := &BoostyClient{main: &BoostyConnector{Bridge: &bridgev2.Bridge{Config: &bridgeconfig.BridgeConfig{}}}}
	q := &b.main.Bridge.Config.Backfill.Queue
	q.MaxBatches = 0
	q.MaxBatchesOverride = map[string]int{"dm": -1}
	if got := b.GetBackfillMaxBatchCount(t.Context(), nil, nil); got != -1 {
		t.Fatalf("DM override ignored: %d", got)
	}
	delete(q.MaxBatchesOverride, "dm")
	if got := b.GetBackfillMaxBatchCount(t.Context(), nil, nil); got != 0 {
		t.Fatalf("explicit global limit ignored: %d", got)
	}
}
