package systemstats

import (
	"context"
	"math"
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

type SystemStatsSnapshot struct {
	CPU         float64  `json:"cpu"`
	Memory      float64  `json:"memory"`
	MemoryUsed  uint64   `json:"memoryUsedBytes"`
	MemoryTotal uint64   `json:"memoryTotalBytes"`
	GPU         *float64 `json:"gpu,omitempty"`
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
