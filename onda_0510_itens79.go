package main

// onda_0510_itens79.go — DELIB-0010 itens 7 e 9 (fechamento da fila).
//
// ITEM 7 — "Arquivo do grupo" como VISÃO AGREGADA no drive do gerente
// (cópia física RECUSADA na deliberação — r2/r3: "RECUSAR a letra física,
// substituir por visão agregada sem cópia; nessa forma o item vive").
//
//   - Rota GET /api/drive/arquivo_grupo — SÓ GERENTE (admin fica 403 como em
//     todo o drive operacional; operador/chefe_setor 403).
//   - Escopo = PRÓPRIO grupo + GRUPOS SUBORDINADOS ativos (padrão hDriveItens
//     Legado/hUsuariosList da onda 04/10).
//   - Devolve METADADOS das pastas/arquivos do corpus do grupo (com nome da
//     função proprietária quando possesso — v34 — e do grupo de origem). O
//     DOWNLOAD não passa por aqui: continua sujeito a checarAcessoArquivo
//     (autor ∪ gerência ∪ grants ∪ função). Nada se move, nada se copia,
//     nada de migration (v34 basta).
//
// ITEM 9 — setores como entidade gerenciável (gap restante; nomear/destituir
// chefe JÁ existe na onda escalas):
//
//   - GET /api/setores/agregado — visão agregada por setor: pessoas ativas,
//     contas ativas, chefe atual (papel chefe_setor + usuarios.setor_id) e
//     cadeia hierárquica do catálogo. Admin: todos os grupos; gerente: próprio
//     grupo + subordinados.
//   - POST /api/usuarios/{id}/papeis (hUsuarioPapelAdd) aceita setor_id
//     opcional quando papel = chefe_setor — o gerente nomeia o chefe JÁ com o
//     setor no ato de atribuir o papel (o endpoint de nomear_chefe continua
//     sendo a via canônica; aqui é conveniência do formulário).

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ---------- ITEM 7: arquivo do grupo (visão agregada) ----------

// hDriveArquivoGrupo: GET /api/drive/arquivo_grupo
func (a *App) hDriveArquivoGrupo(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "gerente" || u.GrupoID == nil || *u.GrupoID <= 0 {
		jsonErro(w, http.StatusForbidden, "arquivo do grupo: apenas gerente com grupo ativo")
		return
	}
	escopo := *u.GrupoID
	ids := append([]int64{escopo}, a.gruposSubordinadosAtivos(escopo)...)
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	pastas := []map[string]any{}
	pRows, err := a.st.db.Query(`
		SELECT dp.id, dp.nome, COALESCE(dp.funcao_id, 0),
		       COALESCE(f.nome, ''), COALESCE(g.nome, ''),
		       COALESCE(NULLIF(u2.nome_guerra, ''), u2.login, '—'),
		       dp.criado_em,
		       (SELECT COUNT(*) FROM drive_pastas sp WHERE sp.pai_id = dp.id) +
		       (SELECT COUNT(*) FROM drive_arquivos sa WHERE sa.pasta_id = dp.id)
		FROM drive_pastas dp
		LEFT JOIN funcoes f ON f.id = dp.funcao_id
		LEFT JOIN grupos g ON g.id = dp.grupo_id
		LEFT JOIN usuarios u2 ON u2.id = dp.autor_usuario_id
		WHERE dp.grupo_id IN (`+ph+`)
		ORDER BY g.nome, dp.nome ASC`, args...)
	if err == nil {
		for pRows.Next() {
			var id, funcaoID, qtd int64
			var nome, funcaoNome, grupoNome, autor, criado string
			if pRows.Scan(&id, &nome, &funcaoID, &funcaoNome, &grupoNome, &autor, &criado, &qtd) == nil {
				pastas = append(pastas, map[string]any{
					"id": id, "nome": nome, "grupo_nome": grupoNome,
					"funcao_nome": funcaoNome, "funcao_id": funcaoID,
					"autor_nome": autor, "criado_em": criado, "qtd_itens": qtd,
				})
			}
		}
		pRows.Close()
	}

	arquivos := []map[string]any{}
	aRows, errA := a.st.db.Query(`
		SELECT da.id, COALESCE(da.pasta_id, 0), da.nome_original, da.tipo, da.tamanho,
		       COALESCE(da.funcao_id, 0),
		       COALESCE(f.nome, ''), COALESCE(g.nome, ''),
		       COALESCE(NULLIF(u2.nome_guerra, ''), u2.login, '—'),
		       da.criado_em
		FROM drive_arquivos da
		LEFT JOIN funcoes f ON f.id = da.funcao_id
		LEFT JOIN grupos g ON g.id = da.grupo_id
		LEFT JOIN usuarios u2 ON u2.id = da.autor_usuario_id
		WHERE da.grupo_id IN (`+ph+`)
		ORDER BY da.criado_em DESC`, args...)
	if errA == nil {
		for aRows.Next() {
			var id, pastaID, funcaoID, tamanho int64
			var nome, tipo, funcaoNome, grupoNome, autor, criado string
			if aRows.Scan(&id, &pastaID, &nome, &tipo, &tamanho, &funcaoID,
				&funcaoNome, &grupoNome, &autor, &criado) == nil {
				arquivos = append(arquivos, map[string]any{
					"id": id, "pasta_id": pastaID, "nome_original": nome,
					"tipo": tipo, "tamanho": tamanho, "grupo_nome": grupoNome,
					"funcao_nome": funcaoNome, "funcao_id": funcaoID,
					"autor_nome": autor, "criado_em": criado,
				})
			}
		}
		aRows.Close()
	}

	var totP, totA int64
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM drive_pastas WHERE grupo_id IN (`+ph+`)`, args...).Scan(&totP)
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM drive_arquivos WHERE grupo_id IN (`+ph+`)`, args...).Scan(&totA)

	a.st.Auditoria(&u.ID, "arquivo_grupo", "drive_pastas", nil,
		fmt.Sprintf("gerencia_uf=%d pastas=%d arquivos=%d", escopo, totP, totA), ipDe(r))

	jsonOK(w, map[string]any{
		"escopo_grupo_id": escopo,
		"grupos":          len(ids),
		"total_pastas":    totP,
		"total_arquivos":  totA,
		"pastas":          pastas,
		"arquivos":        arquivos,
	})
}

// ---------- ITEM 9: visão agregada por setor ----------

// hSetoresAgregado: GET /api/setores/agregado
func (a *App) hSetoresAgregado(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 {
		if u.Papel != "admin" {
			jsonErro(w, http.StatusForbidden, "sem escopo para visão agregada de setores")
			return
		}
	}
	var ids []int64
	if escopo > 0 {
		ids = append([]int64{escopo}, a.gruposSubordinadosAtivos(escopo)...)
	} else {
		rows, err := a.st.db.Query(`SELECT id FROM grupos WHERE ativo = 1`)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	if len(ids) == 0 {
		jsonOK(w, map[string]any{"setores": []any{}})
		return
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	qArgs := make([]any, len(ids))
	for i, id := range ids {
		qArgs[i] = id
	}

	q := `
		SELECT s.id, s.nome, COALESCE(s.sigla, ''), COALESCE(s.pai_id, 0), s.grupo_id,
		       COALESCE(g.nome, ''),
		       (SELECT COUNT(*) FROM pessoas p WHERE p.setor_id = s.id AND p.status = 'ativo'),
		       (SELECT COUNT(*) FROM usuarios us WHERE us.setor_id = s.id AND us.ativo = 1),
		       (SELECT COUNT(*) FROM usuarios us2
		         JOIN usuario_papeis up2 ON up2.usuario_id = us2.id
		           AND up2.papel = 'chefe_setor' AND up2.grupo_id = s.grupo_id
		         WHERE us2.setor_id = s.id AND us2.ativo = 1)
		FROM setores s
		LEFT JOIN grupos g ON g.id = s.grupo_id
		WHERE s.ativo = 1 AND s.grupo_id IN (` + ph + `)
		ORDER BY g.nome, s.nome`
	rows, err := a.st.db.Query(q, qArgs...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	setores := []map[string]any{}
	for rows.Next() {
		var id, paiID, grupoID, qtdPessoas, qtdContas, qtdChefes int64
		var nome, sigla, grupoNome string
		if rows.Scan(&id, &nome, &sigla, &paiID, &grupoID, &grupoNome,
			&qtdPessoas, &qtdContas, &qtdChefes) == nil {
			var chefeNome string
			_ = a.st.db.QueryRow(`
				SELECT COALESCE(NULLIF(us.nome_guerra, ''), us.login, '')
				FROM usuarios us
				JOIN usuario_papeis up ON up.usuario_id = us.id
				 AND up.papel = 'chefe_setor' AND up.grupo_id = s.grupo_id
				WHERE us.setor_id = ? AND us.ativo = 1
				ORDER BY us.id LIMIT 1`, id).Scan(&chefeNome)
			setores = append(setores, map[string]any{
				"id": id, "nome": nome, "sigla": sigla, "pai_id": paiID,
				"grupo_id": grupoID, "grupo_nome": grupoNome,
				"pessoas": qtdPessoas, "contas": qtdContas,
				"chefe_nome": chefeNome, "tem_chefe": chefeNome != "",
			})
		}
	}

	// contas do escopo p/ os dropdowns do painel (nomear a partir da visão)
	contas := []map[string]any{}
	cRows, errC := a.st.db.Query(`
		SELECT us.id, COALESCE(NULLIF(us.nome_guerra, ''), us.login, ''), us.login,
		       COALESCE(us.setor_id, 0)
		FROM usuarios us
		WHERE us.ativo = 1 AND us.grupo_id IN (`+ph+`) AND us.papel IN ('operador','chefe_setor')
		ORDER BY nome_guerra`, qArgs...)
	if errC == nil {
		for cRows.Next() {
			var id, setorID int64
			var guerra, login string
			if cRows.Scan(&id, &guerra, &login, &setorID) == nil {
				contas = append(contas, map[string]any{
					"id": id, "nome_guerra": guerra, "login": login, "setor_id": setorID,
				})
			}
		}
		cRows.Close()
	}

	jsonOK(w, map[string]any{"setores": setores, "contas": contas, "escopo": escopo})
}

// setorIDValidoNoGrupo: existe e está ativo no escopo (ou global).
func (a *App) setorIDValidoNoGrupo(setorID, grupoID int64) bool {
	if setorID <= 0 {
		return false
	}
	var n int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM setores WHERE id = ? AND ativo = 1 AND (grupo_id = ? OR grupo_id IS NULL)`,
		setorID, grupoID).Scan(&n)
	return n > 0
}

// setorInt64 helper: parse tolerante para PathValue opcional.
func setorInt64De(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}
