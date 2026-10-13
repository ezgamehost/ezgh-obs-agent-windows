//go:build windows

package diagnostics

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func Process(s *State) {
	h := windows.CurrentProcess()
	var created, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exit, &kernel, &user) == nil {
		ticks := uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)
		ticks += uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime)
		s.Gauge("process_cpu_seconds", float64(ticks)/1e7)
	}
	var m struct {
		Size, Faults                                                                                    uint32
		PeakWorking, Working, PeakPaged, Paged, PeakNonPaged, NonPaged, Pagefile, PeakPagefile, Private uintptr
	}
	m.Size = uint32(unsafe.Sizeof(m))
	r, _, _ := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo").Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.Size))
	if r != 0 {
		s.Gauge("process_resident_memory_bytes", float64(m.Working))
		s.Gauge("process_private_bytes", float64(m.Private))
	}
	var count uint32
	r, _, _ = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount").Call(uintptr(h), uintptr(unsafe.Pointer(&count)))
	if r != 0 {
		s.Gauge("process_handles", float64(count))
	}
}
