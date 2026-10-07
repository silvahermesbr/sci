package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// HubConferencia gerencia os canais SSE conectados a cada conferência aberta.
type HubConferencia struct {
	mu       sync.RWMutex
	clientes map[int64]map[chan []byte]bool
}

// NovoHubConferencia cria uma nova instância do hub em tempo real.
func NovoHubConferencia() *HubConferencia {
	return &HubConferencia{
		clientes: make(map[int64]map[chan []byte]bool),
	}
}

// AdicionarCliente registra um novo canal consumidor para o stream da conferência.
func (h *HubConferencia) AdicionarCliente(confID int64, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clientes[confID]; !ok {
		h.clientes[confID] = make(map[chan []byte]bool)
	}
	h.clientes[confID][ch] = true
}

// RemoverCliente encerra o canal do cliente e limpa a conferência quando vazia.
func (h *HubConferencia) RemoverCliente(confID int64, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.clientes[confID]; ok {
		delete(subs, ch)
		close(ch)
		if len(subs) == 0 {
			delete(h.clientes, confID)
		}
	}
}

// Broadcast distribui um evento para todos os clientes conectados à conferência.
func (h *HubConferencia) Broadcast(confID int64, payload map[string]any) {
	dados, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if subs, ok := h.clientes[confID]; ok {
		for ch := range subs {
			select {
			case ch <- dados:
			default:
				// buffer cheio: descarta sem bloquear os demais clientes
			}
		}
	}
}

// hConferenciaStream estabelece o fluxo SSE (Server-Sent Events) da conferência.
func (a *App) hConferenciaStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming não suportado pelo servidor", http.StatusInternalServerError)
		return
	}

	confID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || confID <= 0 {
		http.Error(w, "ID de conferência inválido", http.StatusBadRequest)
		return
	}

	u := usuarioDoCtx(r)
	if u == nil {
		http.Error(w, "Não autenticado", http.StatusUnauthorized)
		return
	}

	// Verificar se conferência pertence ao escopo do usuário (admin tem acesso global)
	if esc := escopoDoUsuario(u); esc > 0 {
		var gid *int64
		err := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, confID).Scan(&gid)
		if err != nil {
			http.Error(w, "Conferência não encontrada", http.StatusNotFound)
			return
		}
		if gid == nil || *gid != esc {
			http.Error(w, "Acesso restrito ao grupo da conferência", http.StatusForbidden)
			return
		}
	}

	// Headers oficiais para Server-Sent Events (SSE)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan []byte, 64)
	a.confHub.AdicionarCliente(confID, ch)
	defer a.confHub.RemoverCliente(confID, ch)

	// Handshake inicial confirmando conexão SSE estabelecida
	fmt.Fprintf(w, "data: %s\n\n", `{"tipo":"conectado"}`)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			// Heartbeat periódico para prevenir timeouts em firewalls/proxies
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
