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

func TestAggregateGPUUsageGroupsProcessesAndUsesBusiestEngine(t *testing.T) {
	samples := []gpuEngineSample{
		{name: "pid_10_luid_0x0_0xabc_phys_0_eng_0_engtype_3D", value: 12.25},
		{name: "pid_20_luid_0x0_0xabc_phys_0_eng_0_engtype_3D", value: 8.24},
		{name: "pid_10_luid_0x0_0xabc_phys_0_eng_1_engtype_Copy", value: 15},
		{name: "pid_30_luid_0x0_0xdef_phys_1_eng_0_engtype_3D", value: 18},
	}
	if got := aggregateGPUUsage(samples); got != 20.5 {
		t.Fatalf("got %.1f, want 20.5", got)
	}
}
