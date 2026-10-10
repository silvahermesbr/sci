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
//   - v1.5.4-E2 (R-6): o comando sobre o setor sai junto (chefe_setores; papel
//     chefe_setor purgado quando era o último comando no grupo) e as
//     referências de setor do módulo Material (itens/cautelas/conferências) e
//     os responsáveis (grupo_setor_responsaveis) são limpos/remanejados — nada
//     de FK NO ACTION abortando nem órfãos;
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
	// v367: o ramo restrito é por DESIGNAÇÃO na cadeira 'enc_pessoal', qualquer
	// papel (operador/chefe_setor designados entram). Gerente/admin NÃO entram:
	// podeGestaoPessoal(gerente)=true e o ramo restrito não consulta subordinados
	// — excluí-los aqui preserva a hierarquia ampla deles (bug já cometido).
	encAux := u != nil && u.Papel != "gerente" && u.Papel != "admin" && a.podeGestaoPessoal(u)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin" && !encAux) {
		jsonErro(w, http.StatusForbidden, "gestão de setores é do gerente ou do encarregado/auxiliar de pessoal")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
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
	// apaga o setor. Pool = 1 conn: dentro da tx, só Exec/Query pela tx — sem
	// acesso ao pool com cursor aberto.
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
	// v1.5.4-E2 (R-6): o setor morre, o COMANDO sobre ele vai junto —
	// chefe_setores.setor_id é FK sem cascade e a linha remanescente era
	// chefe-zumbi. Mesma doutrina do D1/hUsuarioPapelDel: o papel chefe_setor
	// existe enquanto o usuário comanda ≥1 setor no grupo — quem perdeu aqui o
	// ÚLTIMO comando perde o papel (sessão re-chaveada para outro papel, como
	// no Del de papel; sem outro papel, a sessão cai por CASCADE e ele re-loga).
	if _, e := tx.Exec(`DELETE FROM chefe_setores WHERE setor_id = ?`, id); e != nil {
		jsonErro(w, http.StatusInternalServerError, "revogação dos comandos do setor falhou: "+e.Error())
		return
	}
	rowsZ, e := tx.Query(`SELECT id, usuario_id FROM usuario_papeis
		WHERE papel = 'chefe_setor' AND grupo_id = ?
		  AND NOT EXISTS (SELECT 1 FROM chefe_setores cs
		                  WHERE cs.usuario_id = usuario_papeis.usuario_id AND cs.grupo_id = usuario_papeis.grupo_id)`, donoGrupo)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, "varredura de chefias órfãs falhou: "+e.Error())
		return
	}
	type papelOrfao struct {
		id, uid int64
	}
	var orfaos []papelOrfao
	for rowsZ.Next() {
		var p papelOrfao
		if rowsZ.Scan(&p.id, &p.uid) == nil {
			orfaos = append(orfaos, p)
		}
	}
	rowsZ.Close()
	for _, pz := range orfaos {
		var outro int64
		_ = tx.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND id != ?
			ORDER BY (papel = 'operador') DESC LIMIT 1`, pz.uid, pz.id).Scan(&outro)
		if outro > 0 {
			if _, e := tx.Exec(`UPDATE sessoes SET papel_ativo_id = ? WHERE papel_ativo_id = ?`, outro, pz.id); e != nil {
				jsonErro(w, http.StatusInternalServerError, "re-chaveamento de sessão falhou: "+e.Error())
				return
			}
		}
		if _, e := tx.Exec(`DELETE FROM usuario_papeis WHERE id = ?`, pz.id); e != nil {
			jsonErro(w, http.StatusInternalServerError, "purga de chefia órfã falhou: "+e.Error())
			return
		}
	}
	// v1.5.4-E2 (R-6): responsáveis de material do setor (FK sem cascade é
	// CASCADE aqui, mas a saída é explícita) e referências de setor do módulo
	// Material — remanejadas para SEM SETOR, mesma doutrina do pessoal.
	if _, e := tx.Exec(`DELETE FROM grupo_setor_responsaveis WHERE setor_id = ?`, id); e != nil {
		jsonErro(w, http.StatusInternalServerError, "limpeza dos responsáveis do setor falhou: "+e.Error())
		return
	}
	for _, q := range []string{
		`UPDATE material_itens SET setor_id = NULL WHERE setor_id = ?`,
		`UPDATE material_cautelas SET setor_id = NULL WHERE setor_id = ?`,
		`UPDATE material_conferencias SET setor_id = NULL WHERE setor_id = ?`,
	} {
		if _, e := tx.Exec(q, id); e != nil {
			jsonErro(w, http.StatusInternalServerError, "remanejamento do material do setor falhou: "+e.Error())
			return
		}
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
