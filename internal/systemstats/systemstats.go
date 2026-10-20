package systemstats

import (
	"context"
	"math"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

type gpuEngineSample struct {
	name  string
	value float64
}

// aggregateGPUUsage matches Windows Task Manager's overall GPU convention:
// combine per-process counters for the same physical engine, then report the
// busiest engine rather than adding independent 3D/copy/video engines.
func aggregateGPUUsage(samples []gpuEngineSample) float64 {
	engines := make(map[string]float64)
	for _, sample := range samples {
		if math.IsNaN(sample.value) || sample.value <= 0 {
			continue
		}
		key := strings.ToLower(sample.name)
		if index := strings.Index(key, "_luid_"); index >= 0 {
			key = key[index+1:]
		}
		engines[key] += sample.value
	}
	usage := 0.0
	for _, value := range engines {
		usage = math.Max(usage, value)
	}
	return math.Round(math.Min(100, usage)*10) / 10
}

type networkSample struct {
	name  string
	value float64
}

// Adapters that are not the machine's real connection: loopback, tunnels, virtual switches and
// VPN or capture drivers. Their traffic is a copy of, or unrelated to, what the physical adapter carries.
var virtualAdapter = regexp.MustCompile(`(?i)loopback|isatap|teredo|6to4|pseudo|vethernet|virtual|vmware|virtualbox|hyper-v|wsl|tap-|wireguard|vpn|bluetooth|miniport|npcap|wan miniport`)

// Windows lists one instance per filter driver on top of an adapter ("Adapter-WFP Native MAC Layer
// LightWeight Filter-0000", "Adapter-QoS Packet Scheduler-0000", "Adapter _2"), each repeating the same traffic.
var filterSuffix = regexp.MustCompile(`(?i)(-(wfp|qos|npcap|native wifi|virtual wifi|ethernet switch|packet scheduler)[^-]*(-\d+)?| _\d+)$`)

// aggregateNetwork sums the physical adapters' rates in bytes per second. Filter instances of one
// adapter count once (the busiest), and virtual adapters are left out.
func aggregateNetwork(samples []networkSample) float64 {
	adapters := make(map[string]float64)
	for _, sample := range samples {
		if math.IsNaN(sample.value) || sample.value <= 0 {
			continue
		}
		name := strings.TrimSpace(filterSuffix.ReplaceAllString(sample.name, ""))
		if name == "" || virtualAdapter.MatchString(name) {
			continue
		}
		key := strings.ToLower(name)
		adapters[key] = math.Max(adapters[key], sample.value)
	}
	total := 0.0
	for _, value := range adapters {
		total += value
	}
	return math.Round(total)
}

// smoothRate eases a rate halfway toward its new reading, so one burst of packets does not read as a speed.
func smoothRate(previous *float64, current float64) float64 {
	if previous == nil {
		return current
	}
	return math.Round(*previous + (current-*previous)*0.5)
}

type SystemStatsSnapshot struct {
	CPU         float64  `json:"cpu"`
	Memory      float64  `json:"memory"`
	MemoryUsed  uint64   `json:"memoryUsedBytes"`
	MemoryTotal uint64   `json:"memoryTotalBytes"`
	GPU         *float64 `json:"gpu,omitempty"`
	NetDown     *uint64  `json:"netDownBytesPerSec,omitempty"`
	NetUp       *uint64  `json:"netUpBytesPerSec,omitempty"`
	Uptime      uint64   `json:"uptimeSeconds"`
	SampledAt   int64    `json:"sampledAt"`
}

type systemStatsCollector interface {
	Sample() SystemStatsSnapshot
	Close()
}

type SystemStatsService struct {
	latest atomic.Value
	cancel context.CancelFunc
	done   chan struct{}
}

func NewService() *SystemStatsService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &SystemStatsService{cancel: cancel, done: make(chan struct{})}
	s.latest.Store(SystemStatsSnapshot{SampledAt: time.Now().UnixMilli()})
	collector := newSystemStatsCollector()
	go func() {
		defer close(s.done)
		defer collector.Close()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			s.latest.Store(collector.Sample())
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return s
}

func (s *SystemStatsService) Snapshot() SystemStatsSnapshot {
	return s.latest.Load().(SystemStatsSnapshot)
}

func (s *SystemStatsService) Close() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
	s.cancel = nil
}
