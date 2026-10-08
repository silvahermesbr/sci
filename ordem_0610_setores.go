package main

// ordem_0610_setores.go — Frente C, ITEM 8c (ordem Diretor 06/10): exclusão de
// SETOR com remanejamento de pessoal em UMA transação.
//
// Regras:
//   - só gerente (admin e gestões de catálogo permanecem nos handlers genéricos
//     de /api/catalogo — este endpoint é o fluxo do gerente no modo Setores do
//     Gerenciar);
//   - pool = 1 conexão: nada de QueryRow dentro de rows.Next() — todo o trabalho
//     é transacional (tx) e sem cursor aberto durante escrita;
//   - pessoal remanejado: pessoas.setor_id=NULL e usuarios.setor_id=NULL → na UI
//     exibem "SEM SETOR";
//   - setor REFERENCIADO por histórico (conferencia_setores FK real) NÃO é
//     apagado: 409 "possui histórico — desative" (doutrina de imutabilidade do
//     histórico de conferências preservada);
//   - gerente fora do escopo (setor de outro grupo) → 403.

import (
	"net/http"
	"strconv"
	"strings"
)

// hSetorExcluir: DELETE /api/setores/{id} — exclusão com remanejamento.
func (a *App) hSetorExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	// módulo Pessoal (f2): enc/aux de pessoal EXCLUEM setor SÓ do PRÓPRIO grupo
	// (sem subordinados — mesmo padrão do hGrupoNomearChefe); gerente mantém a
	// hierarquia (próprio + subordinados); admin entra sempre.
	// NOTE: função de pessoal = tem poder SEM papel do sistema — por isso o
	// filtro é podeGestaoPessoal && Papel vazio, senão o gerente (que tem
	// podeGestaoPessoal=true) cairia no ramo restrito e perderia subordinados.
	encAux := u != nil && (u.Papel == "" || u.Papel == "encarregado") && a.podeGestaoPessoal(u)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin" && !encAux) {
		jsonErro(w, http.StatusForbidden, "gestão de setores é do gerente ou do encarregado/auxiliar de pessoal")
		return
	}
	esc := escopoDoUsuario(u)
	if esc <= 0 {
		jsonErro(w, http.StatusForbidden, "sem escopo de grupo para excluir setor")
		return
	}

	// Setor deve existir e pertencer ao escopo (próprio grupo) ou a subordinados
	// (mesma doutrina dos handlers de catálogo: gerente gere o próprio grupo e
	// os subordinados). Função de pessoal: restrita ao próprio grupo.
	var donoGrupo int64
	if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id, 0) FROM setores WHERE id = ?`, id).Scan(&donoGrupo); e != nil {
		jsonErro(w, http.StatusNotFound, "setor não encontrado")
		return
	}
	permitido := donoGrupo == esc
	if !permitido && !encAux {
		for _, sub := range a.gruposSubordinadosAtivos(esc) {
			if sub == donoGrupo {
				permitido = true
				break
			}
		}
	}
	if !permitido {
		jsonErro(w, http.StatusForbidden, "setor de grupo fora da sua hierarquia")
		return
	}

	// Histórico de conferências referencia o setor? (FK real conferencia_setores.
	// setor_id) → NÃO apagar: 409 com doutrina de desativação.
	var nHist int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM conferencia_setores WHERE setor_id = ?`, id).Scan(&nHist); e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	if nHist > 0 {
		jsonErro(w, http.StatusConflict,
			"setor possui histórico de conferências — desative em vez de excluir")
		return
	}

	// UMA transação: remaneja todo o pessoal (pessoas + contas) para SEM SETOR e
	// apaga o setor. Pool = 1 conn: dentro da tx, só Exec — sem QueryRow/Query
	// aninhados (nada de cursor aberto durante escrita).
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	var nPessoas, nContas int64
	if res, e := tx.Exec(`UPDATE pessoas SET setor_id = NULL, atualizado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE setor_id = ?`, id); e != nil {
		jsonErro(w, http.StatusInternalServerError, "remanejamento do pessoal falhou: "+e.Error())
		return
	} else if n, e2 := res.RowsAffected(); e2 == nil {
		nPessoas = n
	}
	if res, e := tx.Exec(`UPDATE usuarios SET setor_id = NULL WHERE setor_id = ?`, id); e != nil {
		jsonErro(w, http.StatusInternalServerError, "remanejamento das contas falhou: "+e.Error())
		return
	} else if n, e2 := res.RowsAffected(); e2 == nil {
		nContas = n
	}
	if _, e := tx.Exec(`DELETE FROM setores WHERE id = ?`, id); e != nil {
		// corrida com histórico criado entre o COUNT e a tx (defesa em
		// profundidade): FK violation aqui significa referência nova.
		msg := e.Error()
		if strings.Contains(msg, "FOREIGN KEY") || strings.Contains(msg, "constraint") {
			jsonErro(w, http.StatusConflict,
				"setor possui histórico de conferências — desative em vez de excluir")
			return
		}
		jsonErro(w, http.StatusInternalServerError, msg)
		return
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir_setor", "setores", &id,
		"remanejados: "+strconv.FormatInt(nPessoas, 10)+" pessoa(s), "+
			strconv.FormatInt(nContas, 10)+" conta(s) → SEM SETOR", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id,
		"pessoas_remanejadas": nPessoas, "contas_remanejadas": nContas})
}
