//go:build ignore

package main

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	var sucesso int64
	var erro int64
	var wg sync.WaitGroup

	numReqs := 2000
	concorrencia := 100

	sem := make(chan struct{}, concorrencia)
	inicio := time.Now()

	for i := 0; i < numReqs; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			
			cliente := &http.Client{Timeout: 5 * time.Second}
			resp, err := cliente.Get("http://localhost:10003/api/ping")
			if err != nil {
				atomic.AddInt64(&erro, 1)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				atomic.AddInt64(&sucesso, 1)
			} else {
				atomic.AddInt64(&erro, 1)
			}
		}()
	}
	wg.Wait()
	duracao := time.Since(inicio)

	fmt.Printf("STRESS TEST CONCLUÍDO\n")
	fmt.Printf("Requisições totais: %d\n", numReqs)
	fmt.Printf("Sucessos: %d\n", sucesso)
	fmt.Printf("Erros: %d\n", erro)
	fmt.Printf("Tempo total: %v\n", duracao)
	fmt.Printf("Requisições por segundo: %.2f\n", float64(numReqs)/duracao.Seconds())
}
