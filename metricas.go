package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type MetricasSistema struct {
	CPUPercent     float64 `json:"cpu_percent"`
	NumCPU         int     `json:"num_cpu"`
	Goroutines     int     `json:"goroutines"`
	RAMProcessoMB  float64 `json:"ram_processo_mb"`
	RAMSistemaMB   float64 `json:"ram_sistema_mb"`
	RAMHeapMB      float64 `json:"ram_heap_mb"`
	BancoBytes     int64   `json:"banco_bytes"`
	BancoMB        float64 `json:"banco_mb"`
	DadosBytes     int64   `json:"dados_bytes"`
	DadosMB        float64 `json:"dados_mb"`
	DiscoTotalGB   float64 `json:"disco_total_gb"`
	DiscoLivreGB   float64 `json:"disco_livre_gb"`
	DiscoUsadoPct  float64 `json:"disco_usado_pct"`
	Timestamp      string  `json:"timestamp"`
}

var (
	metricasMu       sync.Mutex
	lastCPUSample    time.Time
	lastCPUValue     float64
	lastFolderSample time.Time
	lastDadosBytes   int64
)

func medirTamanhoDiretorio(raiz string) int64 {
	var total int64
	_ = filepath.Walk(raiz, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func coletarMetricas(dataDir string) MetricasSistema {
	metricasMu.Lock()
	defer metricasMu.Unlock()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	agora := time.Now()

	// Tamanho do banco de dados (sci.db)
	var bancoBytes int64
	dbPath := filepath.Join(dataDir, "sci.db")
	if fi, err := os.Stat(dbPath); err == nil {
		bancoBytes = fi.Size()
	}

	// Tamanho da pasta DATA (cacheado por 3 segundos para não sobrecarregar I/O)
	if agora.Sub(lastFolderSample) > 3*time.Second || lastDadosBytes == 0 {
		lastDadosBytes = medirTamanhoDiretorio(dataDir)
		lastFolderSample = agora
	}

	// CPU estimado com base em atividade de goroutines e carga
	cpuPct := estimarCPU()

	discoTotal, discoLivre := obterEspacoDisco(dataDir)
	discoUsadoPct := 0.0
	if discoTotal > 0 {
		discoUsadoPct = ((discoTotal - discoLivre) / discoTotal) * 100.0
	}

	return MetricasSistema{
		CPUPercent:     cpuPct,
		NumCPU:         runtime.NumCPU(),
		Goroutines:     runtime.NumGoroutine(),
		RAMProcessoMB:  float64(ms.Alloc) / (1024 * 1024),
		RAMSistemaMB:   float64(ms.Sys) / (1024 * 1024),
		RAMHeapMB:      float64(ms.HeapAlloc) / (1024 * 1024),
		BancoBytes:     bancoBytes,
		BancoMB:        float64(bancoBytes) / (1024 * 1024),
		DadosBytes:     lastDadosBytes,
		DadosMB:        float64(lastDadosBytes) / (1024 * 1024),
		DiscoTotalGB:   discoTotal / (1024 * 1024 * 1024),
		DiscoLivreGB:   discoLivre / (1024 * 1024 * 1024),
		DiscoUsadoPct:  discoUsadoPct,
		Timestamp:      agora.UTC().Format(time.RFC3339),
	}
}
