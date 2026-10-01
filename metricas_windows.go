//go:build windows

package main

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	winLastTime    time.Time
	winLastProcCPU int64
)

func estimarCPU() float64 {
	h := syscall.Handle(^uintptr(0)) // Processo atual
	var c, e, k, u syscall.Filetime
	err := syscall.GetProcessTimes(h, &c, &e, &k, &u)
	if err != nil {
		return 1.2
	}
	agora := time.Now()
	proc := (int64(k.HighDateTime)<<32 + int64(k.LowDateTime)) + (int64(u.HighDateTime)<<32 + int64(u.LowDateTime))

	if winLastTime.IsZero() {
		winLastTime = agora
		winLastProcCPU = proc
		lastCPUValue = 1.5
		return 1.5
	}

	deltaSec := agora.Sub(winLastTime).Seconds()
	if deltaSec <= 0.1 {
		return lastCPUValue
	}

	deltaCPU := float64(proc - winLastProcCPU) / 10000000.0 // Segundos de tempo de CPU
	numCPU := float64(runtime.NumCPU())
	pct := (deltaCPU / (deltaSec * numCPU)) * 100.0

	winLastTime = agora
	winLastProcCPU = proc

	if pct < 0.2 {
		pct = 0.5
	}
	if pct > 100.0 {
		pct = 100.0
	}
	lastCPUValue = pct
	return pct
}

func obterEspacoDisco(dir string) (totalBytes float64, livreBytes float64) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	dirPtr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64
	r1, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0
	}

	return float64(totalNumberOfBytes), float64(freeBytesAvailable)
}
