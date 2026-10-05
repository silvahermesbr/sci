package main

// Email Interno (ordem Diretor 04/10): finalização de despacho.
// O DESTINATÁRIO do despacho pode finalizar SEM responder; ao finalizar, o
// despacho deixa de exigir resposta e vira mensagem comum (some das
// pendências de despacho e passa a aparecer na caixa convencional, podendo
// ser arquivado/encaminhado como mensagem normal).

import (
	"net/http"
	"strconv"
	"time"
)

func (a *App) hMensagensFinalizar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}
	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var tipo string
	var exigeResp int
	var finalizadoEm *string
	err = a.st.db.QueryRow(`SELECT COALESCE(tipo,'comum'), COALESCE(exige_resposta,0), finalizado_em FROM mensagens WHERE id = ?`, msgID).Scan(&tipo, &exigeResp, &finalizadoEm)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "mensagem não encontrada")
		return
	}
	if tipo != "despacho" {
		jsonErro(w, http.StatusConflict, "apenas despachos são finalizados")
		return
	}
	// Onda 05/10: finalização é única — re-finalizar não reescreve o selo.
	if finalizadoEm != nil {
		jsonErro(w, http.StatusConflict, "despacho já finalizado")
		return
	}

	// Só o DESTINATÁRIO finaliza (o remetente encerra o despacho respondendo;
	// a ordem dá o botão de FINALIZAR a quem RECEBEU o despacho).
	var isDest int
	_ = a.st.db.QueryRow(`SELECT 1 FROM mensagem_destinatarios WHERE mensagem_id = ? AND destinatario_papel_id = ?`, msgID, *u.PapelAtivoID).Scan(&isDest)
	if isDest != 1 {
		jsonErro(w, http.StatusForbidden, "apenas o destinatário do despacho pode finalizá-lo")
		return
	}

	agora := time.Now().UTC().Format(time.RFC3339)
	// Finaliza: vira mensagem comum — exigência baixada E pendência do
	// destinatário marcada como atendida (respondido_em), para nunca mais
	// contar como pendência em contador/badge/arquivo.
	if _, err := a.st.db.Exec(`
		UPDATE mensagens SET finalizado_em = ?, exige_resposta = 0
		WHERE id = ?`, agora, msgID); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao finalizar despacho: "+err.Error())
		return
	}
	if _, err := a.st.db.Exec(`
		UPDATE mensagem_destinatarios SET respondido_em = COALESCE(respondido_em, ?)
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`, agora, msgID, *u.PapelAtivoID); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao baixar pendência do despacho: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "finalizar_despacho", "mensagens", &msgID, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "finalizado_em": agora})
}
