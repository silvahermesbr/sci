//go:build !windows

package main

func estimarCPU() float64 {
	return 2.5
}

func obterEspacoDisco(dir string) (totalBytes float64, livreBytes float64) {
	return 100 * 1024 * 1024 * 1024, 60 * 1024 * 1024 * 1024
}
