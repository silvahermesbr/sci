// server_catalogo.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (a *App) hTagsDisponiveis(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		ids := append([]int64{escopo}, a.gruposSuperioresAtivos(escopo)...)
		marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		args := make([]any, len(ids))
		for i, v := range ids {
			args[i] = v
		}
		rows, err = a.st.db.Query(`SELECT id, nome FROM tags WHERE ativo = 1 AND grupo_id IN (`+marks+`) ORDER BY nome`, args...)
	} else {
		rows, err = a.st.db.Query(`SELECT id, nome FROM tags WHERE ativo = 1 ORDER BY nome`)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome string
		if rows.Scan(&id, &nome) == nil {
			out = append(out, map[string]any{"id": id, "nome": nome})
		}
	}
	jsonOK(w, out)
}

func (a *App) hCatalogoReparentar(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	// ordem 06/10: gerente/encarregado/auxiliar gerenciam catálogos (o escopo
	// específico do item vem logo abaixo — herdados são somente leitura)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin" && !a.podeGestaoPessoal(u)) {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
		return
	}
	esc := escopoDoUsuario(u)
	var req struct {
		PaiID       *int64 `json:"pai_id"`
		Antiguidade *int   `json:"antiguidade"`
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	// item precisa ser do grupo do gerente (dono)
	var donoGrupo int64
	if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); e != nil {
		jsonErro(w, http.StatusNotFound, "item inexistente")
		return
	}
	if esc > 0 && donoGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item herdado de grupo superior — não pode ser movido")
		return
	}
	if req.PaiID != nil {
		// pai válido: existe, do mesmo grupo, não é o próprio item
		if *req.PaiID == id {
			jsonErro(w, http.StatusBadRequest, "um item não pode ser pai de si mesmo")
			return
		}
		var paiDono int64
		if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, *req.PaiID).Scan(&paiDono); e != nil || paiDono != donoGrupo {
			jsonErro(w, http.StatusForbidden, "pai de outro grupo")
			return
		}
		// anti-ciclo: subir a ancestralidade do novo pai; se encontrar o item → ciclo
		pai := *req.PaiID
		prof := 0
		for pai != 0 && prof < 16 {
			var avo *int64
			if e := a.st.db.QueryRow(`SELECT pai_id FROM `+t+` WHERE id = ?`, pai).Scan(&avo); e != nil {
				break
			}
			if avo != nil && *avo == id {
				jsonErro(w, http.StatusBadRequest, "não é possível: o item é ancestral do novo pai (ciclo)")
				return
			}
			if avo == nil {
				break
			}
			pai = *avo
			prof++
		}
		// profundidade resultante do item ≤ 8
		prof = 1
		pai = *req.PaiID
		for pai != 0 && prof <= 8 {
			var avo *int64
			if e := a.st.db.QueryRow(`SELECT pai_id FROM `+t+` WHERE id = ?`, pai).Scan(&avo); e != nil || avo == nil {
				break
			}
			pai = *avo
			prof++
		}
		if prof > 8 {
			jsonErro(w, http.StatusBadRequest, "profundidade máxima da hierarquia é 8")
			return
		}
	}
	if req.Antiguidade != nil {
		if _, err = a.st.db.Exec(`UPDATE `+t+` SET antiguidade = ? WHERE id = ?`, *req.Antiguidade, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "antiguidade", t, &id, fmt.Sprintf("posição %d", *req.Antiguidade), ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": id, "antiguidade": *req.Antiguidade})
		return
	}
	if _, err = a.st.db.Exec(`UPDATE `+t+` SET pai_id = ? WHERE id = ?`, req.PaiID, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "reparentar", t, &id, "novo pai", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id, "pai_id": req.PaiID})
}

func (a *App) hCatalogoEditar(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin" && !a.podeGestaoPessoal(u)) {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
		return
	}
	if t == "funcoes" {
		var itemTipo string
		_ = a.st.db.QueryRow(`SELECT COALESCE(tipo, 'antiguidade') FROM funcoes WHERE id = ?`, id).Scan(&itemTipo)
		// v39 (ordem Diretor): funções de grupo são HARDCODED — ninguém edita nem exclui
		if itemTipo == "grupo" {
			jsonErro(w, http.StatusForbidden, "funções de grupo são fixas do sistema e não podem ser editadas")
			return
		}
	}
	var req struct {
		Nome  string `json:"nome"`
		Sigla string `json:"sigla"`
		Cor   string `json:"cor"`
	}
	if err = decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome obrigatório")
		return
	}
	nome := strings.TrimSpace(req.Nome)
	if esc := escopoDoUsuario(u); esc > 0 {
		var donoGrupo int64
		if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); e != nil {
			jsonErro(w, http.StatusNotFound, "item não encontrado")
			return
		}
		subordinados := a.gruposSubordinadosAtivos(esc)
		ehSubordinado := false
		for _, sub := range subordinados {
			if sub == donoGrupo {
				ehSubordinado = true
				break
			}
		}
		if donoGrupo != esc && !ehSubordinado {
			jsonErro(w, http.StatusForbidden, "item herdado de grupo superior ou global — somente leitura")
			return
		}
	}
	switch t {
	case "setores":
		_, err = a.st.db.Exec(`UPDATE setores SET nome = ?, sigla = NULLIF(?,'') WHERE id = ?`, nome, req.Sigla, id)
	case "tags":
		_, err = a.st.db.Exec(`UPDATE tags SET nome = ?, cor = NULLIF(?,'') WHERE id = ?`, nome, req.Cor, id)
	default:
		_, err = a.st.db.Exec(`UPDATE `+t+` SET nome = ? WHERE id = ?`, nome, id)
	}
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não atualizado (duplicado?): "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "editar", t, &id, nome, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id, "nome": nome})
}

func (a *App) filtroArvore(escopo int64, alias string) treeFilter {
	// FIX S4 (verif5): guarda AQUI — chamadores diretos (query de pessoas do
	// bundle) não podem gerar "grupo_id IN (0)" no escopo global (esvaziava a lista).
	if escopo <= 0 {
		return treeFilter{}
	}
	ids := append([]int64{escopo}, a.gruposSubordinadosAtivos(escopo)...)
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return treeFilter{clause: ` AND ` + alias + `.grupo_id IN (` + ph + `)`, args: args}
}

func (a *App) clSetor(escopo int64) string {
	if escopo <= 0 {
		return ""
	}
	return a.filtroArvore(escopo, "f").clause
}

func (a *App) hCatalogoList(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	// colunas extras por tabela: {expressão SQL, alias no JSON}
	type colExtra struct{ expr, alias string }
	extras := []colExtra{}
	switch t {
	case "setores":
		// multi-chefia (ordem 06/10 item 14): o COMANDO por setor vem de
		// chefe_setores — subselects correlacionados (1 chefe por setor, UNIQUE).
		extras = append(extras,
			colExtra{"sigla", "sigla"},
			colExtra{"(SELECT cs.usuario_id FROM chefe_setores cs WHERE cs.setor_id = " + t + ".id LIMIT 1)", "chefe_usuario_id"},
			colExtra{"(SELECT COALESCE(u.nome_guerra, u.login, '') FROM chefe_setores cs LEFT JOIN usuarios u ON u.id = cs.usuario_id WHERE cs.setor_id = " + t + ".id LIMIT 1)", "chefe_nome"},
		)
	case "tags":
		extras = append(extras, colExtra{"cor", "cor"})
	}
	extra := ""
	for _, e := range extras {
		extra += ", " + e.expr
	}
	var args []any
	q := `WITH RECURSIVE cam(id, caminho) AS (
		  SELECT id, nome FROM ` + t + ` WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cm.caminho || ' > ' || f.nome FROM ` + t + ` f JOIN cam cm ON f.pai_id = cm.id
		)
		SELECT ` + t + `.id, ` + t + `.nome` + extra + `, ` + t + `.pai_id, ` + t + `.ativo, ` + t + `.grupo_id, ` + t + `.antiguidade
		FROM ` + t + ` LEFT JOIN cam ON cam.id = ` + t + `.id`
	whereClauses := []string{}
	if esc := escopoDoUsuario(usuarioDoCtx(r)); esc > 0 {
		// Doutrina: todos os membros da hierarquia têm visibilidade das tags de superiores, do próprio grupo e de subordinados
		ids := append([]int64{esc}, a.gruposSuperioresAtivos(esc)...)
		ids = append(ids, a.gruposSubordinadosAtivos(esc)...)
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		for _, id := range ids {
			args = append(args, id)
		}
		whereClauses = append(whereClauses, `(`+t+`.grupo_id IS NULL OR `+t+`.grupo_id IN (`+ph+`))`)
	}
	if t == "funcoes" {
		// v368 (doutrina v359): o catálogo /api/catalogo/funcoes é SOMENTE
		// ANTIGUIDADE — cadeiras de grupo (tipo='grupo': Encarregado de
		// Pessoal/Material, Gerente) NUNCA aparecem aqui; elas são designadas
		// exclusivamente no módulo Pessoal › Aba Funções (funcao_membros, via
		// /api/grupo/funcoes/membros). O envelope ganhou {funcoes, total} para o
		// front provar o filtro; a lista continua em "funcoes" (compatível).
		// Cravado por v368_admin_antiguidade_test.go (TestCatalogoFuncoes*).
		whereClauses = append(whereClauses, `(`+t+`.tipo = 'antiguidade' OR `+t+`.tipo IS NULL)`)
	}
	if len(whereClauses) > 0 {
		q += ` WHERE ` + strings.Join(whereClauses, " AND ")
	}
	// v368: catálogo de funções ordena por ANTIGUIDADE (nulos por último —
	// legado sem grau); demais catálogos mantêm caminho hierárquico.
	orderPor := "cam.caminho, antiguidade, nome"
	if t == "funcoes" {
		orderPor = "(antiguidade IS NULL), antiguidade, nome"
	}
	q += ` ORDER BY ` + orderPor
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	cols := []string{"id", "nome"}
	for _, e := range extras {
		cols = append(cols, e.alias)
	}
	cols = append(cols, "pai_id", "ativo", "grupo_id", "antiguidade")
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err == nil {
			linha := map[string]any{}
			for i, c := range cols {
				linha[c] = vals[i]
			}
			out = append(out, linha)
		}
	}
	// v368, correção 09/10 (regressão em produção): o envelope {funcoes, total}
	// é EXCLUSIVO do catálogo de funções (contrato v368 cravado em
	// TestCatalogoFuncoes*). Estava no fim do handler SEM escopo de `t` e
	// embrulhava TODOS os catálogos ({t}); o front consome setores/tags/etc.
	// como array (ativosDe/forEach) → TypeError → toast "Falha ao carregar a
	// tela" ao abrir o módulo Pessoal como gerente. Consumidores do envelope
	// de funções usam `funcoesRes.funcoes || funcoesRes` — seguem compatíveis.
	if t == "funcoes" {
		jsonOK(w, map[string]any{"funcoes": out, "total": len(out)})
		return
	}
	jsonOK(w, out)
}

func (a *App) hCatalogoAdd(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Nome    string `json:"nome"`
		Sigla   string `json:"sigla"`
		Cor     string `json:"cor"`
		GrupoID *int64 `json:"grupo_id"`
		Tipo    string `json:"tipo"`
	}
	if err = decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome obrigatório")
		return
	}
	tipo := "antiguidade"
	if t == "funcoes" {
		// v39 (ordem Diretor): funções de grupo são HARDCODED (só as 3 cadeiras
		// semeadas pela migração). API só cria itens de ANTIGUIDADE — tentar
		// tipo=grupo é erro.
		if strings.TrimSpace(req.Tipo) != "" {
			tipo = strings.ToLower(strings.TrimSpace(req.Tipo))
		}
		if tipo == "grupo" {
			jsonErro(w, http.StatusBadRequest, "funções de grupo são fixas do sistema (Gerente, Encarregado de Pessoal, Encarregado de Material) e não podem ser criadas")
			return
		}
	}
	nome := strings.TrimSpace(req.Nome)
	u := usuarioDoCtx(r)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin" && !a.podeGestaoPessoal(u)) {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
		return
	}
	var grupoID any
	if u.Papel == "admin" && req.GrupoID != nil {
		// Fix P2: admin só cria catálogo em grupo EXISTENTE (antes gravava FK cru
		// de grupo inexistente e quebrava herança de catálogos em silêncio).
		if *req.GrupoID <= 0 {
			jsonErro(w, http.StatusBadRequest, "grupo_id inválido")
			return
		}
		var n int
		if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE id = ?`, *req.GrupoID).Scan(&n); err != nil || n == 0 {
			jsonErro(w, http.StatusBadRequest, "grupo inexistente")
			return
		}
		grupoID = *req.GrupoID
	} else if u.GrupoID != nil {
		grupoID = *u.GrupoID
	}
	var q string
	switch t {
	case "setores":
		q = `INSERT INTO setores (nome, sigla, grupo_id) VALUES (?, NULLIF(?,''), ?)`
	case "tags":
		q = `INSERT INTO tags (nome, cor, grupo_id) VALUES (?, NULLIF(?,''), ?)`
	case "funcoes":
		q = `INSERT INTO funcoes (nome, grupo_id, tipo) VALUES (?, ?, ?)`
	default:
		q = `INSERT INTO ` + t + ` (nome, grupo_id) VALUES (?,?)`
	}
	var res interface {
		LastInsertId() (int64, error)
	}
	if t == "setores" || t == "tags" {
		res, err = a.st.db.Exec(q, nome, map[bool]string{true: req.Sigla, false: req.Cor}[t == "setores"], grupoID)
	} else if t == "funcoes" {
		res, err = a.st.db.Exec(q, nome, grupoID, tipo)
	} else {
		res, err = a.st.db.Exec(q, nome, grupoID)
	}
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não inserido (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "criar", t, &id, nome, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "nome": nome})
}

func (a *App) hCatalogoDel(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err2 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err2 != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	// ordem 06/10: gerente/encarregado/auxiliar gerenciam catálogos (o escopo
	// específico do item vem logo abaixo — herdados são somente leitura)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin" && !a.podeGestaoPessoal(u)) {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
		return
	}
	if t == "funcoes" {
		var itemTipo string
		_ = a.st.db.QueryRow(`SELECT COALESCE(tipo, 'antiguidade') FROM funcoes WHERE id = ?`, id).Scan(&itemTipo)
		// v39 (ordem Diretor): funções de grupo são HARDCODED — ninguém exclui
		if itemTipo == "grupo" {
			jsonErro(w, http.StatusForbidden, "funções de grupo são fixas do sistema e não podem ser excluídas")
			return
		}
	}
	if esc := escopoDoUsuario(u); esc > 0 {
		var donoGrupo int64
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "item não encontrado")
			return
		}
		subordinados := a.gruposSubordinadosAtivos(esc)
		ehSubordinado := false
		for _, sub := range subordinados {
			if sub == donoGrupo {
				ehSubordinado = true
				break
			}
		}
		if donoGrupo != esc && !ehSubordinado {
			jsonErro(w, http.StatusForbidden, "catálogo herdado de grupo superior ou global — somente leitura")
			return
		}
	}
	// padrão: EXCLUSÃO real (ordem do Tenente 28/09). Em uso → 409 (FK).
	if r.URL.Query().Get("modo") != "desativar" {
		if _, err = a.st.db.Exec(`DELETE FROM `+t+` WHERE id = ?`, id); err != nil {
			jsonErro(w, http.StatusConflict, "item em uso por lançamentos/cadastros — pode desativar em vez de excluir")
			return
		}
		a.st.Auditoria(&u.ID, "excluir", t, &id, "", ipDe(r))
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	// alternativa conservadora: desativação (histórico preservado)
	if _, err = a.st.db.Exec(`UPDATE `+t+` SET ativo = 0 WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "desativar", t, &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hOperadoresDoSetor(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "somente chefe de setor opera este endpoint")
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "chefe de setor sem grupo definido")
		return
	}
	var setorID *int64
	_ = a.st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, u.ID).Scan(&setorID)
	if setorID == nil {
		jsonErro(w, http.StatusBadRequest, "chefe de setor sem setor vinculado")
		return
	}

	if r.Method == http.MethodGet {
		rows, err := a.st.db.Query(`
			SELECT u.id, u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
			       COALESCE(u.funcao_id,0), COALESCE(f.nome,''),
			       EXISTS(SELECT 1 FROM usuario_papeis up WHERE up.usuario_id = u.id AND up.grupo_id = ? AND up.papel = 'operador')
			FROM usuarios u
			LEFT JOIN funcoes f ON f.id = u.funcao_id
			WHERE u.grupo_id = ? AND u.setor_id = ? AND u.ativo = 1
			ORDER BY u.login`, *u.GrupoID, *u.GrupoID, *setorID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, fid int64
			var login, ng, nc, fnome string
			var ehOp int
			if rows.Scan(&id, &login, &ng, &nc, &fid, &fnome, &ehOp) == nil {
				out = append(out, map[string]any{
					"id": id, "login": login, "nome_guerra": ng, "nome_completo": nc,
					"funcao_id": fid, "funcao_nome": fnome, "eh_operador": ehOp == 1,
				})
			}
		}
		jsonOK(w, out)
		return
	}

	// POST: designar {login} (conta do setor) como operador
	var req struct {
		Login string `json:"login"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" {
		jsonErro(w, http.StatusBadRequest, "login obrigatório")
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.Login))
	var id int64
	var grupo, setor *int64
	err := a.st.db.QueryRow(`SELECT id, grupo_id, setor_id FROM usuarios WHERE login = ? AND ativo = 1`, login).
		Scan(&id, &grupo, &setor)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "conta não encontrada")
		return
	}
	if grupo == nil || *grupo != *u.GrupoID || setor == nil || *setor != *setorID {
		jsonErro(w, http.StatusForbidden, "conta não pertence ao seu setor")
		return
	}
	if _, err := a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'operador')`, id, *u.GrupoID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "designar_operador", "usuarios", &id, login, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id})
}

func (a *App) iniciarWatchdogSLA() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		for range ticker.C {
			a.verificarAtrasosSLA()
		}
	}()
}

func (a *App) verificarAtrasosSLA() {
	var webhookURL, cfgPrazo string
	_ = a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'WEBHOOK_ATRASOS_URL'`).Scan(&webhookURL)
	// Fix P1-5: webhook é comando de SAÍDA — URL só http/https com host; falhas
	// LOGADAS (antes engolidas: o alerta prometido nunca disparava sem pista).
	if webhookURL != "" {
		if u, err := url.Parse(webhookURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			log.Printf("SLA webhook: WEBHOOK_ATRASOS_URL inválida (%q) — alerta não disparado", webhookURL)
			webhookURL = ""
		}
	}
	if webhookURL == "" {
		return
	}

	prazoHoras := 24
	_ = a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'CAUTELA_PRAZO_PADRAO_HORAS'`).Scan(&cfgPrazo)
	if p, err := strconv.Atoi(cfgPrazo); err == nil && p > 0 {
		prazoHoras = p
	}

	q := `SELECT mc.id, mi.nome, mi.codigo_patrimonio, p.nome_guerra, mc.data_saida,
	             ROUND((strftime('%s', 'now') - strftime('%s', mc.data_saida)) / 3600.0, 1) as horas_fora
	      FROM material_cautelas mc
	      JOIN material_itens mi ON mi.id = mc.item_id
	      JOIN pessoas p ON p.id = mc.pessoa_id
	      WHERE mc.status = 'ativa'
	        AND (strftime('%s', 'now') - strftime('%s', mc.data_saida)) > (? * 3600)
	      ORDER BY mc.data_saida ASC LIMIT 50`

	rows, err := a.st.db.Query(q, prazoHoras)
	if err != nil {
		return
	}
	defer rows.Close()

	var itens []map[string]any
	for rows.Next() {
		var cid int64
		var iNome, iCod, pGuerra, dts string
		var horas float64
		if rows.Scan(&cid, &iNome, &iCod, &pGuerra, &dts, &horas) == nil {
			itens = append(itens, map[string]any{
				"cautela_id":        cid,
				"item":              iNome,
				"codigo_patrimonio": iCod,
				"responsavel":       pGuerra,
				"saida":             dts,
				"horas_em_aberto":   horas,
			})
		}
	}
	rows.Close() // FIXED: Explicitly close rows to free the single DB connection before HTTP call

	if len(itens) == 0 {
		return
	}

	payload := map[string]any{
		"sistema":        "SCI",
		"evento":         "ALERTA_CAUTELAS_ATRASADAS",
		"total_atrasos":  len(itens),
		"prazo_config_h": prazoHoras,
		"disparado_em":   time.Now().UTC().Format(time.RFC3339),
		"cautelas":       itens,
	}
	corpo, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(corpo))
	if err != nil {
		log.Printf("SLA webhook: falha ao montar requisição: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, errPost := client.Do(req)
	if errPost != nil {
		log.Printf("SLA webhook: POST falhou para %s: %v", webhookURL, errPost)
		return
	}
	if resp != nil {
		if resp.StatusCode >= 400 {
			log.Printf("SLA webhook: alvo respondeu %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func (a *App) hSetorSugestoesList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 && u.Papel != "admin" {
		jsonErro(w, http.StatusForbidden, "usuário sem grupo definido")
		return
	}

	setorFiltro := strings.TrimSpace(r.URL.Query().Get("setor"))
	statusFiltro := strings.TrimSpace(r.URL.Query().Get("status"))

	q := `SELECT s.id, s.grupo_id, COALESCE(g.nome, ''), s.setor_tipo, s.autor_id,
	             COALESCE(NULLIF(u_aut.nome_guerra,''), NULLIF(u_aut.nome_completo,''), '—'), s.tipo_acao, s.dados_json,
	             s.status, s.aprovado_por, COALESCE(NULLIF(u_apr.nome_guerra,''), NULLIF(u_apr.nome_completo,''), '—'),
	             COALESCE(s.aprovado_em, ''), COALESCE(s.justificativa, ''), s.criado_em
	      FROM setor_sugestoes s
	      JOIN grupos g ON g.id = s.grupo_id
	      JOIN usuarios u_aut ON u_aut.id = s.autor_id
	      LEFT JOIN usuarios u_apr ON u_apr.id = s.aprovado_por
	      WHERE 1=1`
	var args []any

	if escopo > 0 {
		q += ` AND s.grupo_id = ?`
		args = append(args, escopo)
	}
	if setorFiltro != "" {
		q += ` AND s.setor_tipo = ?`
		args = append(args, setorFiltro)
	}
	if statusFiltro != "" {
		q += ` AND s.status = ?`
		args = append(args, statusFiltro)
	}
	q += ` ORDER BY s.id DESC LIMIT 100`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var lista []map[string]any
	for rows.Next() {
		var id, gid, autorID int64
		var gNome, sTipo, autorNome, tipoAcao, dadosJSON, status, aprovadorNome, aprovadoEm, just, criadoEm string
		var aprovadorID *int64
		if errScan := rows.Scan(&id, &gid, &gNome, &sTipo, &autorID, &autorNome, &tipoAcao,
			&dadosJSON, &status, &aprovadorID, &aprovadorNome, &aprovadoEm, &just, &criadoEm); errScan == nil {
			var dados map[string]any
			_ = json.Unmarshal([]byte(dadosJSON), &dados)
			lista = append(lista, map[string]any{
				"id":             id,
				"grupo_id":       gid,
				"grupo_nome":     gNome,
				"setor_tipo":     sTipo,
				"autor_id":       autorID,
				"autor_nome":     autorNome,
				"tipo_acao":      tipoAcao,
				"dados":          dados,
				"dados_json":     dadosJSON,
				"status":         status,
				"aprovado_por":   aprovadorID,
				"aprovador_nome": aprovadorNome,
				"aprovado_em":    aprovadoEm,
				"justificativa":  just,
				"criado_em":      criadoEm,
			})
		}
	}
	if lista == nil {
		lista = []map[string]any{}
	}
	jsonOK(w, map[string]any{"sugestoes": lista})
}

func (a *App) hSetorSugestoesAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "usuário sem grupo operacional")
		return
	}

	var req struct {
		SetorTipo string         `json:"setor_tipo"` // 'comando', 'pessoal', 'material'
		TipoAcao  string         `json:"tipo_acao"`
		Dados     map[string]any `json:"dados"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	req.SetorTipo = strings.ToLower(strings.TrimSpace(req.SetorTipo))
	if req.SetorTipo != "comando" && req.SetorTipo != "pessoal" && req.SetorTipo != "material" {
		jsonErro(w, http.StatusBadRequest, "setor_tipo inválido (deve ser 'comando', 'pessoal' ou 'material')")
		return
	}
	if strings.TrimSpace(req.TipoAcao) == "" {
		jsonErro(w, http.StatusBadRequest, "tipo_acao obrigatório")
		return
	}
	// Fix P1-2 (rodada 04/10): allowlist de tipo_acao na ENTRADA — a sugestão guarda
	// dados_json autoral que vira escrita no banco na aprovação; só os dois tipos
	// conhecidos por aplicarEfeitoSugestao podem entrar na fila.
	req.TipoAcao = strings.ToLower(strings.TrimSpace(req.TipoAcao))
	if req.TipoAcao != "alterar_status_militar" && req.TipoAcao != "atualizar_item_material" {
		jsonErro(w, http.StatusBadRequest, "tipo_acao não permitido (deve ser 'alterar_status_militar' ou 'atualizar_item_material')")
		return
	}

	dj, err := json.Marshal(req.Dados)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "falha ao serializar dados")
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO setor_sugestoes (grupo_id, setor_tipo, autor_id, tipo_acao, dados_json, status)
		VALUES (?, ?, ?, ?, ?, 'pendente')`,
		escopo, req.SetorTipo, u.ID, req.TipoAcao, string(dj))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao salvar sugestão: "+err.Error())
		return
	}

	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "sugestao_criar", "setor_sugestoes", &id, fmt.Sprintf("setor=%s acao=%s", req.SetorTipo, req.TipoAcao), ipDe(r))

	jsonOK(w, map[string]any{
		"ok":         true,
		"id":         id,
		"mensagem":   "Sugestão encaminhada com sucesso para apreciação do Chefe de Setor",
		"status":     "pendente",
		"setor_tipo": req.SetorTipo,
	})
}

func idDeJSON(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0
		}
		return i
	}
	return 0
}

func (a *App) aplicarEfeitoSugestao(tipoAcao string, dados map[string]any, escopo int64) (int64, string, error) {
	return aplicarEfeitoSugestaoTx(a.st.db, tipoAcao, dados, escopo)
}

func aplicarEfeitoSugestaoTx(ex executorSQL, tipoAcao string, dados map[string]any, escopo int64) (int64, string, error) {
	if tipoAcao == "alterar_status_militar" {
		pID := idDeJSON(dados["pessoa_id"])
		novoStatus, _ := dados["novo_status"].(string)
		novoStatus = strings.ToLower(strings.TrimSpace(novoStatus))
		if pID <= 0 || novoStatus == "" {
			return 0, "", fmt.Errorf("dados incompletos (pessoa_id/novo_status)")
		}
		if novoStatus != "ativo" && novoStatus != "inativo" {
			return 0, "", fmt.Errorf("status de militar não permitido: %q", novoStatus)
		}
		var gid int64
		if err := ex.QueryRow(`SELECT COALESCE(grupo_id,0) FROM pessoas WHERE id = ?`, pID).Scan(&gid); err != nil {
			return 0, "", fmt.Errorf("militar %d não encontrado", pID)
		}
		if escopo > 0 && gid != escopo {
			return 0, "", fmt.Errorf("militar %d fora do grupo da sugestão", pID)
		}
		if escopo <= 0 {
			return 0, "", fmt.Errorf("sem grupo operacional para aplicar alteração de militar")
		}
		if _, err := ex.Exec(`UPDATE pessoas SET status = ? WHERE id = ?`, novoStatus, pID); err != nil {
			return 0, "", err
		}
		return pID, "militar", nil
	}
	if tipoAcao == "atualizar_item_material" {
		itemID := idDeJSON(dados["item_id"])
		novoStatus, _ := dados["status"].(string)
		novoStatus = strings.ToLower(strings.TrimSpace(novoStatus))
		if itemID <= 0 || novoStatus == "" {
			return 0, "", fmt.Errorf("dados incompletos (item_id/status)")
		}
		if novoStatus != "disponivel" && novoStatus != "acautelado" && novoStatus != "manutencao" && novoStatus != "baixado" {
			return 0, "", fmt.Errorf("status de item não permitido: %q", novoStatus)
		}
		var gid int64
		if err := ex.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&gid); err != nil {
			return 0, "", fmt.Errorf("item %d não encontrado", itemID)
		}
		if escopo > 0 && gid != escopo {
			return 0, "", fmt.Errorf("item %d fora do grupo da sugestão", itemID)
		}
		if escopo <= 0 {
			return 0, "", fmt.Errorf("sem grupo operacional para aplicar alteração de item")
		}
		if _, err := ex.Exec(`UPDATE material_itens SET status = ? WHERE id = ?`, novoStatus, itemID); err != nil {
			return 0, "", err
		}
		return itemID, "item", nil
	}
	return 0, "", fmt.Errorf("tipo de ação desconhecido: %q", tipoAcao)
}

func (a *App) hSetorSugestoesAvaliar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	// Apenas chefes (gerente, chefe_setor ou admin) podem avaliar sugestões
	if u.Papel != "admin" && u.Papel != "gerente" && u.Papel != "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "Apenas o Chefe de Setor ou Gerente pode aprovar/rejeitar sugestões")
		return
	}

	sugID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || sugID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID da sugestão inválido")
		return
	}

	var req struct {
		Acao          string `json:"acao"` // 'aprovar' ou 'rejeitar'
		Justificativa string `json:"justificativa"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	req.Acao = strings.ToLower(strings.TrimSpace(req.Acao))
	if req.Acao != "aprovar" && req.Acao != "rejeitar" {
		jsonErro(w, http.StatusBadRequest, "ação inválida (deve ser 'aprovar' ou 'rejeitar')")
		return
	}

	var gid int64
	var statusAtual, sTipo, tipoAcao, dadosJSON string
	err = a.st.db.QueryRow(`
		SELECT grupo_id, status, setor_tipo, tipo_acao, dados_json
		FROM setor_sugestoes WHERE id = ?`, sugID).Scan(&gid, &statusAtual, &sTipo, &tipoAcao, &dadosJSON)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "sugestão não encontrada")
		return
	}

	if escopo > 0 && gid != escopo && u.Papel != "admin" {
		jsonErro(w, http.StatusForbidden, "sugestão fora do seu grupo")
		return
	}
	if statusAtual != "pendente" {
		jsonErro(w, http.StatusConflict, "sugestão já foi avaliada anteriormente ("+statusAtual+")")
		return
	}

	agora := time.Now().UTC().Format(time.RFC3339)
	novoStatus := "rejeitado"
	if req.Acao == "aprovar" {
		novoStatus = "aprovado"
	}

	// Fix P1-2 (rodada 04/10): transação única — efeito + chancela nascem e morrem juntos.
	// Em erro de efeito a sugestão NÃO vira 'aprovada' (fica pendente) e o motivo volta
	// ao chefe (409). Guard TOCTOU: WHERE status='pendente' + RowsAffected==0 → 409
	// 'já avaliada' (avaliação concorrente não reaplica efeito nem sobrescreve chancela).
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Se aprovado, o resultado oficial fica em nome do Chefe de Setor que aprovou!
	// WHERE status='pendente' mata o TOCTOU: se avaliada concorrentemente, RowsAffected=0.
	ra, err := tx.Exec(`
		UPDATE setor_sugestoes
		SET status = ?, aprovado_por = ?, aprovado_em = ?, justificativa = ?
		WHERE id = ? AND status = 'pendente'`, novoStatus, u.ID, agora, req.Justificativa, sugID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao atualizar status: "+err.Error())
		return
	}
	if n, _ := ra.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusConflict, "sugestão já avaliada")
		return
	}

	// Se aprovado, aplicar a ação correspondente no banco com autoria do Chefe.
	// Escopo do alvo + allowlist revalidados AQUI (momento da aplicação) — não confiar
	// no checkpoint da criação. Falha de efeito → rollback: sugestão segue pendente.
	if req.Acao == "aprovar" {
		var dados map[string]any
		_ = json.Unmarshal([]byte(dadosJSON), &dados)
		if _, _, err := aplicarEfeitoSugestaoTx(tx, tipoAcao, dados, escopo); err != nil {
			jsonErro(w, http.StatusConflict, "falha ao aplicar efeito da sugestão: "+err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao concluir avaliação: "+err.Error())
		return
	}

	chefeNome := u.NomeGuerra
	if chefeNome == "" {
		chefeNome = u.NomeCompleto
	}
	if chefeNome == "" {
		chefeNome = u.Login
	}

	a.st.Auditoria(&u.ID, "sugestao_"+novoStatus, "setor_sugestoes", &sugID,
		fmt.Sprintf("avaliado por chefe=%s justificativa=%s", chefeNome, req.Justificativa), ipDe(r))

	jsonOK(w, map[string]any{
		"ok":             true,
		"id":             sugID,
		"status":         novoStatus,
		"aprovador_id":   u.ID,
		"aprovador_nome": chefeNome,
		"aprovado_em":    agora,
		"mensagem":       fmt.Sprintf("Sugestão %s com sucesso com chancela oficial de %s", novoStatus, chefeNome),
	})
}

// ---------- rotas rotasCatalogo ----------
func (a *App) rotasCatalogo() {
	m := a.mux

	m.Handle("GET /api/catalogo/{t}", a.auth(false, a.hCatalogoList))
	m.Handle("GET /api/setores/agregado", a.auth(false, a.hSetoresAgregado))
	m.Handle("POST /api/catalogo/{t}", a.guardaGestaoPessoal(a.hCatalogoAdd))
	m.Handle("DELETE /api/catalogo/{t}/{id}", a.guardaGestaoPessoal(a.hCatalogoDel))
	m.Handle("PATCH /api/catalogo/{t}/{id}/pai", a.guardaGestaoPessoal(a.hCatalogoReparentar)) // hierarquia (v9.16)
	m.Handle("PATCH /api/catalogo/{t}/{id}", a.guardaGestaoPessoal(a.hCatalogoEditar))
	m.Handle("DELETE /api/setores/{id}", a.auth(false, a.hSetorExcluir)) // ordem 06/10 item 8c: exclusão com remanejamento
	m.Handle("GET /api/operadores-do-setor", a.auth(false, a.hOperadoresDoSetor))
	m.Handle("POST /api/operadores-do-setor", a.auth(false, a.hOperadoresDoSetor))

	// Workflow Setorial (v1.5) — Sugestões e Aprovações por Chefe de Setor
	m.Handle("GET /api/setores/sugestoes", a.auth(false, a.hSetorSugestoesList))
	m.Handle("POST /api/setores/sugestoes", a.auth(false, a.hSetorSugestoesAdd))
	m.Handle("POST /api/setores/sugestoes/{id}/avaliar", a.auth(false, a.hSetorSugestoesAvaliar))
}
