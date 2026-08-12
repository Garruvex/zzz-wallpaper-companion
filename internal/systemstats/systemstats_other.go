//go:build !windows

package systemstats

import "time"

type unsupportedStatsCollector struct{ startedAt time.Time }

func newSystemStatsCollector() systemStatsCollector {
	return &unsupportedStatsCollector{startedAt: time.Now()}
}
func (c *unsupportedStatsCollector) Sample() SystemStatsSnapshot {
	now := time.Now()
	return SystemStatsSnapshot{Uptime: uint64(now.Sub(c.startedAt) / time.Second), SampledAt: now.UnixMilli()}
}
func (*unsupportedStatsCollector) Close() {}
