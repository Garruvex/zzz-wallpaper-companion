//go:build !windows

package systemstats

import "time"

type unsupportedStatsCollector struct{}

func newSystemStatsCollector() systemStatsCollector { return &unsupportedStatsCollector{} }
func (*unsupportedStatsCollector) Sample() SystemStatsSnapshot {
	return SystemStatsSnapshot{SampledAt: time.Now().UnixMilli()}
}
func (*unsupportedStatsCollector) Close() {}
