//go:build windows

package systemstats

import (
	"math"
	"syscall"
	"time"
	"unsafe"
)

type memoryStatusEx struct {
	Length                                                                                               uint32
	MemoryLoad                                                                                           uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
}

type windowsStatsCollector struct {
	globalMemoryStatusEx                     *syscall.LazyProc
	getTickCount64                           *syscall.LazyProc
	pdhCollect, pdhValue, pdhArray, pdhClose *syscall.LazyProc
	pdhQuery, pdhCPUCounter, pdhGPUCounter   uintptr
}

type pdhDoubleValue struct {
	Status uint32
	_      uint32
	Value  float64
}
type pdhDoubleItem struct {
	Name  *uint16
	Value pdhDoubleValue
}

func utf16PtrString(value *uint16) string {
	if value == nil {
		return ""
	}
	units := unsafe.Slice(value, 32*1024)
	for index, unit := range units {
		if unit == 0 {
			return syscall.UTF16ToString(units[:index])
		}
	}
	return ""
}

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

func newSystemStatsCollector() systemStatsCollector {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	c := &windowsStatsCollector{
		globalMemoryStatusEx: kernel32.NewProc("GlobalMemoryStatusEx"),
		getTickCount64:       kernel32.NewProc("GetTickCount64"),
	}
	pdh := syscall.NewLazyDLL("pdh.dll")
	open, add := pdh.NewProc("PdhOpenQueryW"), pdh.NewProc("PdhAddEnglishCounterW")
	c.pdhCollect = pdh.NewProc("PdhCollectQueryData")
	c.pdhValue = pdh.NewProc("PdhGetFormattedCounterValue")
	c.pdhArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	c.pdhClose = pdh.NewProc("PdhCloseQuery")
	if result, _, _ := open.Call(0, 0, uintptr(unsafe.Pointer(&c.pdhQuery))); result == 0 {
		cpuPath, _ := syscall.UTF16PtrFromString(`\Processor Information(_Total)\% Processor Utility`)
		gpuPath, _ := syscall.UTF16PtrFromString(`\GPU Engine(*)\Utilization Percentage`)
		add.Call(c.pdhQuery, uintptr(unsafe.Pointer(cpuPath)), 0, uintptr(unsafe.Pointer(&c.pdhCPUCounter)))
		add.Call(c.pdhQuery, uintptr(unsafe.Pointer(gpuPath)), 0, uintptr(unsafe.Pointer(&c.pdhGPUCounter)))
		if c.pdhCPUCounter == 0 && c.pdhGPUCounter == 0 {
			c.pdhClose.Call(c.pdhQuery)
			c.pdhQuery = 0
		} else {
			c.pdhCollect.Call(c.pdhQuery)
		}
	}
	return c
}

func (c *windowsStatsCollector) readCPU() float64 {
	if c.pdhCPUCounter == 0 {
		return 0
	}
	var value pdhDoubleValue
	if result, _, _ := c.pdhValue.Call(c.pdhCPUCounter, pdhFmtDouble, 0, uintptr(unsafe.Pointer(&value))); result != 0 || value.Status != 0 || math.IsNaN(value.Value) {
		return 0
	}
	return math.Round(math.Max(0, math.Min(100, value.Value))*10) / 10
}

func (c *windowsStatsCollector) Sample() SystemStatsSnapshot {
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	ok, _, _ := c.globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	uptimeMS, _, _ := c.getTickCount64.Call()
	if c.pdhQuery != 0 {
		c.pdhCollect.Call(c.pdhQuery)
	}
	s := SystemStatsSnapshot{CPU: c.readCPU(), Uptime: uint64(uptimeMS) / 1000, SampledAt: time.Now().UnixMilli()}
	if ok != 0 {
		s.Memory = float64(m.MemoryLoad)
		s.MemoryTotal = m.TotalPhys
		s.MemoryUsed = m.TotalPhys - m.AvailPhys
	}
	s.GPU = c.readGPU()
	return s
}

func (c *windowsStatsCollector) readGPU() *float64 {
	if c.pdhGPUCounter == 0 {
		return nil
	}
	var size, count uint32
	result, _, _ := c.pdhArray.Call(c.pdhGPUCounter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if result != pdhMoreData || size == 0 || count == 0 {
		return nil
	}
	buffer := make([]byte, size)
	result, _, _ = c.pdhArray.Call(c.pdhGPUCounter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buffer[0])))
	if result != 0 {
		return nil
	}
	items := unsafe.Slice((*pdhDoubleItem)(unsafe.Pointer(&buffer[0])), int(count))
	samples := make([]gpuEngineSample, 0, len(items))
	for _, item := range items {
		if item.Value.Status == 0 && item.Name != nil {
			samples = append(samples, gpuEngineSample{
				name:  utf16PtrString(item.Name),
				value: item.Value.Value,
			})
		}
	}
	usage := aggregateGPUUsage(samples)
	return &usage
}

func (c *windowsStatsCollector) Close() {
	if c.pdhQuery != 0 {
		c.pdhClose.Call(c.pdhQuery)
		c.pdhQuery = 0
	}
}
