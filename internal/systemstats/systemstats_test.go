package systemstats

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotIncludesUptime(t *testing.T) {
	body, err := json.Marshal(SystemStatsSnapshot{Uptime: 42})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"uptimeSeconds":42`) {
		t.Fatalf("uptime missing from snapshot: %s", body)
	}
}
