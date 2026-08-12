package systemstats

import (
	"context"
	"sync/atomic"
	"time"
)

type SystemStatsSnapshot struct {
	CPU         float64  `json:"cpu"`
	Memory      float64  `json:"memory"`
	MemoryUsed  uint64   `json:"memoryUsedBytes"`
	MemoryTotal uint64   `json:"memoryTotalBytes"`
	GPU         *float64 `json:"gpu,omitempty"`
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
