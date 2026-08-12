//go:build windows

package systemstats

import (
	"math"
	"syscall"
	"time"
	"unsafe"
)

type filetime struct{ Low, High uint32 }

func (f filetime) value() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }

type memoryStatusEx struct {
	Length                                                                                               uint32
	MemoryLoad                                                                                           uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
}

type windowsStatsCollector struct {
	getSystemTimes                 *syscall.LazyProc
	globalMemoryStatusEx           *syscall.LazyProc
	getTickCount64                 *syscall.LazyProc
	lastIdle, lastKernel, lastUser uint64
	pdhCollect, pdhArray, pdhClose *syscall.LazyProc
	pdhQuery, pdhCounter           uintptr
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

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

func newSystemStatsCollector() systemStatsCollector {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	c := &windowsStatsCollector{
		getSystemTimes:       kernel32.NewProc("GetSystemTimes"),
		globalMemoryStatusEx: kernel32.NewProc("GlobalMemoryStatusEx"),
		getTickCount64:       kernel32.NewProc("GetTickCount64"),
	}
	pdh := syscall.NewLazyDLL("pdh.dll")
	open, add := pdh.NewProc("PdhOpenQueryW"), pdh.NewProc("PdhAddEnglishCounterW")
	c.pdhCollect, c.pdhArray, c.pdhClose = pdh.NewProc("PdhCollectQueryData"), pdh.NewProc("PdhGetFormattedCounterArrayW"), pdh.NewProc("PdhCloseQuery")
	if result, _, _ := open.Call(0, 0, uintptr(unsafe.Pointer(&c.pdhQuery))); result == 0 {
		path, _ := syscall.UTF16PtrFromString(`\GPU Engine(*)\Utilization Percentage`)
		if result, _, _ := add.Call(c.pdhQuery, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&c.pdhCounter))); result != 0 {
			c.pdhClose.Call(c.pdhQuery)
			c.pdhQuery = 0
		} else {
			c.pdhCollect.Call(c.pdhQuery)
		}
	}
	c.readCPU()
	return c
}

func (c *windowsStatsCollector) readCPU() float64 {
	var idle, kernel, user filetime
	ok, _, _ := c.getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 {
		return 0
	}
	i, k, u := idle.value(), kernel.value(), user.value()
	if c.lastKernel == 0 {
		c.lastIdle, c.lastKernel, c.lastUser = i, k, u
		return 0
	}
	idleDelta, totalDelta := i-c.lastIdle, (k-c.lastKernel)+(u-c.lastUser)
	c.lastIdle, c.lastKernel, c.lastUser = i, k, u
	if totalDelta == 0 {
		return 0
	}
	return math.Round(math.Max(0, math.Min(100, 100*(1-float64(idleDelta)/float64(totalDelta))))*10) / 10
}

func (c *windowsStatsCollector) Sample() SystemStatsSnapshot {
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	ok, _, _ := c.globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	uptimeMS, _, _ := c.getTickCount64.Call()
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
	if c.pdhQuery == 0 || c.pdhCounter == 0 {
		return nil
	}
	if result, _, _ := c.pdhCollect.Call(c.pdhQuery); result != 0 {
		return nil
	}
	var size, count uint32
	result, _, _ := c.pdhArray.Call(c.pdhCounter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if result != pdhMoreData || size == 0 || count == 0 {
		return nil
	}
	buffer := make([]byte, size)
	result, _, _ = c.pdhArray.Call(c.pdhCounter, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buffer[0])))
	if result != 0 {
		return nil
	}
	items := unsafe.Slice((*pdhDoubleItem)(unsafe.Pointer(&buffer[0])), int(count))
	total := 0.0
	for _, item := range items {
		if item.Value.Status == 0 && !math.IsNaN(item.Value.Value) && item.Value.Value > 0 {
			total += item.Value.Value
		}
	}
	total = math.Round(math.Min(100, total)*10) / 10
	return &total
}

func (c *windowsStatsCollector) Close() {
	if c.pdhQuery != 0 {
		c.pdhClose.Call(c.pdhQuery)
		c.pdhQuery = 0
	}
}
