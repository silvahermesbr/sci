package main

// SCI — Sistema de Controle Interno (3º B Com GE)
// Binário único: Go + SQLite (arquivo) + frontend embutido.

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"
)

// versaoSchemaBinario: maior versão de schema_migrations que ESTE binário conhece
// (v37 = fix/r3-poderes-designacao: poderes de gestão de pessoal passam a vir
// da DESIGNAÇÃO (funcao_membros), não do nome da função no cadastro).
// Valida imports de backup (R9): arquivo mais novo que o binário = rejeita.
const versaoSchemaBinario = 37

func main() {
	// footprint: teto suave de heap — GC age antes de o RSS crescer sem freio
	debug.SetMemoryLimit(96 << 20)
	debug.SetGCPercent(40)

	// CLI: `sci backup` faz cópia consistente e sai (p/ cron), sem servidor
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		dataDir := env("SCI_DATA_DIR", "./dados")
		st, err := AbrirStore(dataDir)
		if err != nil {
			log.Fatalf("banco: %v", err)
		}
		defer st.Close()
		a := NovaApp(st)
		nome, sha, err := a.backupAgora()
		if err != nil {
			a.backupFlag("cron", err)
			log.Fatalf("backup: %v", err)
		}
		_ = os.Remove(filepath.Join(dataDir, "FLAG_BACKUP.txt"))
		log.Printf("backup OK: %s (sha256 %s)", nome, sha)
		return
	}

	dataDir := env("SCI_DATA_DIR", "./dados")
	porta := env("SCI_PORT", "10003")
	senhaAdmin := env("SCI_ADMIN_SENHA", "admin") // ordem Tenente 28/09: senha padrão = admin

	st, err := AbrirStore(dataDir)
	if err != nil {
		log.Fatalf("banco: %v", err)
	}
	defer st.Close()

	if err := st.migrarV2(); err != nil {
		log.Fatalf("migração v2: %v", err)
	}
	if err := st.SeedIfEmpty(senhaAdmin); err != nil {
		log.Fatalf("seed: %v", err)
	}

	app := NovaApp(st)

	// zero-perda: primeiro backup da sessão já no boot; falha vira FLAG visível
	if _, _, err := app.backupAgora(); err != nil {
		app.backupFlag("boot", err)
	} else {
		_ = os.Remove(filepath.Join(dataDir, "FLAG_BACKUP.txt"))
	}

	// limpeza de sessões expiradas a cada hora
	go func() {
		for range time.Tick(time.Hour) {
			st.LimparSessoesExpiradas()
		}
	}()

	srv := &http.Server{
		Addr:              ":" + porta,
		Handler:           recoveryMiddleware(app.mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("SCI no ar — porta %s — dados em %s", porta, dataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("erro no servidor: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	// FIX P2-2 (revisão DEV-L 30/09): deploy mata por PID (SIGTERM) — sem ele na lista,
	// o shutdown elegante nunca rodava no fluxo real. os.Kill não é capturável (mantido por doc).
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Encerrando o servidor (graceful shutdown)...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Erro ao encerrar o servidor: %v", err)
	}
	log.Println("Servidor encerrado com sucesso.")
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fix de blindagem (recomendação da auditoria v1.3): CSP em TODA resposta.
		// 'unsafe-inline' é necessário: o front usa onclick=/style= inline em massa
		// (remover quebra todos os botões). Mesmo assim a CSP mata <script> remoto,
		// object/embed e framing externo — eleva o custo de qualquer XSS residual.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; connect-src 'self'; font-src 'self' data:; object-src 'none'; "+
				"base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC RECOVERED: %v\n%s", err, debug.Stack())
				http.Error(w, "Erro Interno do Servidor", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
