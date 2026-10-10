package main

// onda_v154_e2.go — pacote E2 da onda v1.5.4 (defeitos R-21 do ARQUITETURA.md §14).
//
// R-21 (mural): leituras/registros por id sem checar o grupo do objeto —
// hAvisosDetalhes e hAvisosCiente aceitavam id de aviso de QUALQUER grupo
// (IDOR de leitura de PII e de escrita de ciente). Este helper é a régua
// única dos dois handlers, espelho do filtro da listagem (hAvisosList):
// admin (escopo 0) lê tudo; conta de grupo só aviso do PRÓPRIO grupo.

import "net/http"

// avisoNoEscopo verifica que o aviso existe e pertence ao escopo do
// solicitante. Responde sozinha (404 inexistente / 403 fora do escopo /
// 403 sem grupo) e devolve false quando o handler deve parar.
func (a *App) avisoNoEscopo(w http.ResponseWriter, u *Usuario, avisoID int64) bool {
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return false
	}
	var grupoAviso int64
	if e := a.st.db.QueryRow(`SELECT grupo_id FROM avisos WHERE id = ?`, avisoID).Scan(&grupoAviso); e != nil {
		jsonErro(w, http.StatusNotFound, "aviso inexistente")
		return false
	}
	if esc > 0 && grupoAviso != esc {
		jsonErro(w, http.StatusForbidden, "aviso de outro grupo")
		return false
	}
	return true
}
