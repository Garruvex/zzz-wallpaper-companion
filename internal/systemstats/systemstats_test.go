package systemstats

import (
	"encoding/json"
	"math"
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

func TestAggregateNetworkCountsEachPhysicalAdapterOnce(t *testing.T) {
	samples := []networkSample{
		{name: "Intel[R] Ethernet Controller I226-V", value: 1000},
		{name: "Intel[R] Ethernet Controller I226-V-WFP Native MAC Layer LightWeight Filter-0000", value: 990},
		{name: "Intel[R] Ethernet Controller I226-V-QoS Packet Scheduler-0000", value: 1000},
		{name: "MediaTek Wi-Fi 7 MT7927 Wireless LAN Card", value: 200},
		{name: "MediaTek Wi-Fi 7 MT7927 Wireless LAN Card _2", value: 200},
		{name: "MediaTek Wi-Fi 7 MT7927 Wireless LAN Card _3", value: 195},
	}
	if got := aggregateNetwork(samples); got != 1200 {
		t.Fatalf("got %.0f, want 1200 (ethernet 1000 + wifi 200)", got)
	}
}

func TestAggregateNetworkSkipsVirtualAdapters(t *testing.T) {
	samples := []networkSample{
		{name: "Realtek PCIe GbE Family Controller", value: 500},
		{name: "Loopback Pseudo-Interface 1", value: 9000},
		{name: "vEthernet (WSL (Hyper-V firewall))", value: 4000},
		{name: "Hyper-V Virtual Ethernet Adapter", value: 3000},
		{name: "VMware Virtual Ethernet Adapter for VMnet8", value: 2000},
		{name: "WireGuard Tunnel", value: 1500},
		{name: "TAP-Windows Adapter V9", value: 1000},
		{name: "Bluetooth Device (Personal Area Network)", value: 100},
	}
	if got := aggregateNetwork(samples); got != 500 {
		t.Fatalf("got %.0f, want 500 (only the physical adapter)", got)
	}
}

func TestAggregateNetworkIgnoresIdleAndInvalidReadings(t *testing.T) {
	samples := []networkSample{
		{name: "Intel[R] Ethernet Controller I226-V", value: 0},
		{name: "Realtek USB GbE Family Controller", value: math.NaN()},
		{name: "", value: 50},
	}
	if got := aggregateNetwork(samples); got != 0 {
		t.Fatalf("got %.0f, want 0", got)
	}
}

func TestSmoothRateEasesHalfwayAndStartsAtTheFirstReading(t *testing.T) {
	if got := smoothRate(nil, 800); got != 800 {
		t.Fatalf("first reading: got %.0f, want 800", got)
	}
	previous := 200.0
	if got := smoothRate(&previous, 600); got != 400 {
		t.Fatalf("got %.0f, want 400", got)
	}
}

func TestSnapshotOmitsNetworkWhenUnavailableAndIncludesItWhenPresent(t *testing.T) {
	body, _ := json.Marshal(SystemStatsSnapshot{})
	if strings.Contains(string(body), "netDownBytesPerSec") || strings.Contains(string(body), "netUpBytesPerSec") {
		t.Fatalf("network fields should be omitted: %s", body)
	}
	down, up := uint64(1500000), uint64(250000)
	body, _ = json.Marshal(SystemStatsSnapshot{NetDown: &down, NetUp: &up})
	if !strings.Contains(string(body), `"netDownBytesPerSec":1500000`) || !strings.Contains(string(body), `"netUpBytesPerSec":250000`) {
		t.Fatalf("network fields missing: %s", body)
	}
}
