// server_grupos.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (a *App) gruposSubordinadosAtivos(gid int64) []int64 {
	visitado := map[int64]bool{gid: true}
	fila := []int64{gid}
	out := []int64{}
	for len(fila) > 0 {
		g := fila[0]
		fila = fila[1:]
		rows, err := a.st.db.Query(`SELECT subordinado_id FROM grupo_vinculos
			WHERE superior_id = ? AND criado_por_superior = 1 AND criado_por_subordinado = 1`, g)
		if err != nil {
			break
		}
		for rows.Next() {
			var s int64
			if rows.Scan(&s) == nil && !visitado[s] {
				visitado[s] = true
				out = append(out, s)
				fila = append(fila, s)
			}
		}
		rows.Close()
	}
	return out
}

func (a *App) argsArvore(escopo int64) []any {
	if escopo <= 0 {
		return nil
	}
	return a.filtroArvore(escopo, "f").args
}

func (a *App) gruposSuperioresAtivos(gid int64) []int64 {
	// v9.16.8: TODA a cadeia de superiores (recursivo) — herança vale para qualquer vínculo
	// ativo, em todos os níveis (ex.: 3º Pel → Cia → Bde).
	visita := map[int64]bool{gid: true}
	fila := []int64{gid}
	var out []int64
	for len(fila) > 0 {
		atual := fila[0]
		fila = fila[1:]
		rows, err := a.st.db.Query(`SELECT superior_id FROM grupo_vinculos WHERE subordinado_id = ?`, atual)
		if err != nil {
			continue
		}
		for rows.Next() {
			var sup int64
			if rows.Scan(&sup) == nil && !visita[sup] {
				visita[sup] = true
				out = append(out, sup)
				fila = append(fila, sup)
			}
		}
		rows.Close()
	}
	return out
}

func (a *App) hGrupoExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var nome string
	if err := a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, id).Scan(&nome); err != nil {
		jsonErro(w, http.StatusNotFound, "grupo inexistente")
		return
	}
	forcar := r.URL.Query().Get("forcar") == "1"
	nuke := r.URL.Query().Get("nuke") == "1"
	if forcar || nuke {
		var req struct {
			Senha string `json:"senha"`
		}
		if err := decodificar(r, &req); err != nil || req.Senha == "" {
			jsonErro(w, http.StatusBadRequest, "senha de admin obrigatória para exclusão forçada")
			return
		}
		var hash string
		if err := a.st.db.QueryRow(`SELECT senha_hash FROM usuarios WHERE id = ?`, u.ID).Scan(&hash); err != nil ||
			!verificaSenha(req.Senha, hash) {
			modo := "forcar"
			if nuke {
				modo = "nuke"
			}
			a.st.Auditoria(&u.ID, "excluir_grupo_negado", "grupos", &id, "senha incorreta ("+modo+")", ipDe(r))
			jsonErro(w, http.StatusUnauthorized, "senha de admin incorreta — exclusão negada")
			return
		}
	}
	var pessoas, contas, vinculos, confs int
	_ = a.st.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM pessoas WHERE grupo_id = ?),
		(SELECT COUNT(*) FROM usuarios WHERE grupo_id = ? AND ativo = 1),
		(SELECT COUNT(*) FROM grupo_vinculos WHERE superior_id = ? OR subordinado_id = ?),
		(SELECT COUNT(*) FROM conferencias WHERE grupo_id = ?)`,
		id, id, id, id, id).Scan(&pessoas, &contas, &vinculos, &confs)
	if confs > 0 && !nuke {
		jsonErro(w, http.StatusConflict, fmt.Sprintf("grupo tem %d conferência(s) no histórico — exclusão negada (imutabilidade histórica); exclusão TOTAL exige o MODO NUKE com dupla confirmação", confs))
		return
	}
	if !forcar && !nuke {
		if pessoas > 0 {
			jsonErro(w, http.StatusConflict, fmt.Sprintf("grupo tem %d pessoa(s) no banco de pessoal — mova ou exclua antes (ou use exclusão forçada com senha)", pessoas))
			return
		}
		if contas > 0 {
			jsonErro(w, http.StatusConflict, fmt.Sprintf("grupo tem %d conta(s) ativa(s) — mova ou exclua as contas antes (ou use exclusão forçada com senha)", contas))
			return
		}
		if vinculos > 0 {
			jsonErro(w, http.StatusConflict, "grupo tem vínculos de subordinação — remova-os antes (painel de grupos)")
			return
		}
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM grupo_vinculos WHERE superior_id = ? OR subordinado_id = ?`, id, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if nuke {
		// MODO NUKE (ordem Tenente 30/09): apaga TODO o rastro do grupo. Ordem respeita
		// FKs: histórico → vínculos com módulos → catálogos do grupo. Backup automático
		// é gravado logo após o commit (backupAssincrono no fim do handler).
		for _, q := range []string{
			// 0) auditoria dos usuários do grupo: vínculo anulado (rastro forense
			// preservado — auditoria.usuario_id não tem CASCADE)
			`UPDATE auditoria SET usuario_id = NULL WHERE usuario_id IN (SELECT id FROM usuarios WHERE grupo_id = ?)`,
			// 1) histórico de conferências do grupo (presenças/comentários caem por CASCADE)
			`DELETE FROM presencas WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
			`DELETE FROM comentarios WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
			`DELETE FROM conferencias WHERE grupo_id = ?`,
			// 2) módulos em reserva (escala/material) ligados ao grupo ou ao pessoal dele
			`DELETE FROM escala_pessoas WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
			`DELETE FROM escala_turnos WHERE grupo_id = ?`,
			`DELETE FROM escala_tipos WHERE grupo_id = ?`,
			`DELETE FROM material_cautelas WHERE item_id IN (SELECT id FROM material_itens WHERE grupo_id = ?)`,
			`DELETE FROM material_itens WHERE grupo_id = ?`,
			`DELETE FROM material_categorias WHERE grupo_id = ?`,
			// 3) catálogos de organização do grupo (referências já apagadas acima).
			// setores/funções: 2 passes — filho (pai_id) antes do pai, self-FK exige
			`DELETE FROM setores WHERE pai_id IS NOT NULL AND grupo_id = ?`,
			`DELETE FROM funcoes WHERE pai_id IS NOT NULL AND grupo_id = ?`,
			`DELETE FROM tags WHERE grupo_id = ?`,
			`DELETE FROM destinos WHERE grupo_id = ?`,
			`DELETE FROM setores WHERE grupo_id = ?`,
			`DELETE FROM funcoes WHERE grupo_id = ?`,
			`DELETE FROM conferencia_tipos WHERE grupo_id = ?`,
		} {
			if _, err = tx.Exec(q, id); err != nil {
				jsonErro(w, http.StatusInternalServerError, "nuke: "+err.Error())
				return
			}
		}
	}
	if _, err = tx.Exec(`DELETE FROM usuarios WHERE grupo_id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err = tx.Exec(`DELETE FROM pessoas WHERE grupo_id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err = tx.Exec(`DELETE FROM grupos WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	detalhe := "vazio"
	if nuke {
		detalhe = fmt.Sprintf("NUKE: %d conta(s), %d pessoa(s) e %d conferência(s) APAGADAS", contas, pessoas, confs)
	} else if forcar {
		detalhe = fmt.Sprintf("FORÇADA: %d conta(s) e %d pessoa(s) removidas", contas, pessoas)
	}
	a.st.Auditoria(&u.ID, "excluir", "grupos", &id, nome+" ["+detalhe+"]", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "forçada": forcar || nuke, "nuke": nuke,
		"contas_removidas": contas, "pessoas_removidas": pessoas, "conferencias_removidas": confs})
}

func escopoDoUsuario(u *Usuario) int64 {
	if u == nil {
		return -1
	}
	if u.Papel == "admin" {
		return 0
	}
	if u.GrupoID == nil {
		return -1 // gerente/operador sem grupo = sem acesso a dados de grupo
	}
	return *u.GrupoID
}

func podeAdministrar(u *Usuario) bool {
	return u != nil && (u.Papel == "admin" || u.Papel == "gerente")
}

func (a *App) hGruposList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		if errors.Is(err, ErrContaSemGrupo) {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		jsonErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}
	jsonOK(w, a.gruposComCodigo(escopo))
}

func (a *App) hVinculoAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		Codigo string `json:"codigo"`
		Lado   string `json:"lado"`  // "superior" = declaro MEU código a um inferior; "subordinado" = o contrário
		Outro  string `json:"outro"` // admin: vincula DOIS códigos de uma vez (fecha o bilateral)
	}
	if err := decodificar(r, &req); err != nil || req.Codigo == "" || (req.Lado != "superior" && req.Lado != "subordinado") {
		jsonErro(w, http.StatusBadRequest, "codigo e lado (superior|subordinado) obrigatórios")
		return
	}
	// ADMIN vincula diretamente dois grupos pelos códigos (ordem Tenente 28/09 noite)
	if u.Papel == "admin" && strings.TrimSpace(req.Outro) != "" {
		var idA, idB int64
		if a.st.db.QueryRow(`SELECT id FROM grupos WHERE codigo = ?`, strings.ToUpper(strings.TrimSpace(req.Codigo))).Scan(&idA) != nil ||
			a.st.db.QueryRow(`SELECT id FROM grupos WHERE codigo = ?`, strings.ToUpper(strings.TrimSpace(req.Outro))).Scan(&idB) != nil {
			jsonErro(w, http.StatusNotFound, "código de grupo inexistente")
			return
		}
		if idA == idB {
			jsonErro(w, http.StatusBadRequest, "um grupo não se vincula a si mesmo")
			return
		}
		if _, err := a.st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
			VALUES (?, ?, 1, 1) ON CONFLICT(superior_id, subordinado_id)
			DO UPDATE SET criado_por_superior = 1, criado_por_subordinado = 1`, idA, idB); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "vincular", "grupo_vinculos", nil,
			fmt.Sprintf("admin: sup=%d sub=%d", idA, idB), ipDe(r))
		jsonOK(w, map[string]any{"vinculado": true})
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo não registra vínculos")
		return
	}
	var outroID int64
	err := a.st.db.QueryRow(`SELECT id FROM grupos WHERE codigo = ?`, strings.ToUpper(strings.TrimSpace(req.Codigo))).Scan(&outroID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "código de grupo inexistente")
		return
	}
	if outroID == *u.GrupoID {
		jsonErro(w, http.StatusBadRequest, "um grupo não se vincula a si mesmo")
		return
	}
	var supID, subID int64
	if req.Lado == "superior" {
		supID, subID = *u.GrupoID, outroID
	} else {
		supID, subID = outroID, *u.GrupoID
	}
	// proteção contra ciclo imediato A->B->A (v1): se o "subordinado" já é superior do solicitante
	var ciclo int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM grupo_vinculos WHERE superior_id = ? AND subordinado_id = ?`,
		subID, supID).Scan(&ciclo)
	if ciclo > 0 {
		jsonErro(w, http.StatusConflict, "vínculo reverso já existe — não é possível inverter")
		return
	}
	_, err = a.st.db.Exec(`INSERT INTO grupo_vinculos
		(superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?,?,?,?)
		ON CONFLICT(superior_id, subordinado_id) DO UPDATE SET
		  criado_por_superior = criado_por_superior | ?,
		  criado_por_subordinado = criado_por_subordinado | ?`,
		supID, subID,
		map[bool]int{true: 1, false: 0}[req.Lado == "superior"],
		map[bool]int{true: 1, false: 0}[req.Lado == "subordinado"],
		map[bool]int{true: 1, false: 0}[req.Lado == "superior"],
		map[bool]int{true: 1, false: 0}[req.Lado == "subordinado"])
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	var pS, pSb int
	_ = a.st.db.QueryRow(`SELECT criado_por_superior, criado_por_subordinado FROM grupo_vinculos
		WHERE superior_id = ? AND subordinado_id = ?`, supID, subID).Scan(&pS, &pSb)
	a.st.Auditoria(&u.ID, "vincular", "grupo_vinculos", nil,
		fmt.Sprintf("sup=%d sub=%d completo=%v", supID, subID, pS == 1 && pSb == 1), ipDe(r))
	if pS == 1 && pSb == 1 {
		jsonOK(w, map[string]any{"vinculado": true})
	} else {
		jsonOK(w, map[string]any{"vinculado": false,
			"aguardando": "aguardando a confirmação do outro grupo"})
	}
}

func (a *App) hVinculoList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.GrupoID == nil {
		jsonOK(w, map[string]any{"pendentes": []map[string]any{}, "ativos": []map[string]any{}})
		return
	}
	gid := *u.GrupoID
	rows, err := a.st.db.Query(`
		SELECT gv.superior_id, gs.nome, gv.subordinado_id, gsub.nome,
		       gv.criado_por_superior, gv.criado_por_subordinado
		FROM grupo_vinculos gv
		JOIN grupos gs ON gs.id = gv.superior_id
		JOIN grupos gsub ON gsub.id = gv.subordinado_id
		WHERE gv.superior_id = ? OR gv.subordinado_id = ?`, gid, gid)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	pend, ativos := []map[string]any{}, []map[string]any{}
	for rows.Next() {
		var supID, subID, pS, pSb int64
		var supNome, subNome string
		if rows.Scan(&supID, &supNome, &subID, &subNome, &pS, &pSb) == nil {
			role := "subordinado"
			if supID == gid {
				role = "superior"
			}
			item := map[string]any{"superior": supNome, "subordinado": subNome, "meu_papel": role}
			if pS == 1 && pSb == 1 {
				ativos = append(ativos, item)
			} else {
				pend = append(pend, item)
			}
		}
	}
	jsonOK(w, map[string]any{"pendentes": pend, "ativos": ativos})
}

func (a *App) gruposComCodigo(escopo int64) []map[string]any {
	if escopo < 0 {
		return []map[string]any{}
	}
	rows, err := a.st.db.Query(`
		SELECT g.id, g.nome, COALESCE(g.codigo,''), g.criado_em,
		       (SELECT COUNT(*) FROM usuarios u WHERE u.grupo_id = g.id) AS contas,
		       (SELECT COUNT(*) FROM pessoas p WHERE p.grupo_id = g.id) AS efetivo,
		       (SELECT GROUP_CONCAT(g2.nome) FROM grupo_vinculos gv
		          JOIN grupos g2 ON g2.id = gv.subordinado_id
		          WHERE gv.superior_id = g.id AND gv.criado_por_superior = 1 AND gv.criado_por_subordinado = 1),
		       (SELECT GROUP_CONCAT(g3.nome) FROM grupo_vinculos gv2
		          JOIN grupos g3 ON g3.id = gv2.superior_id
		          WHERE gv2.subordinado_id = g.id AND gv2.criado_por_superior = 1 AND gv2.criado_por_subordinado = 1),
		       (SELECT GROUP_CONCAT(g2.id) FROM grupo_vinculos gv
		          JOIN grupos g2 ON g2.id = gv.subordinado_id
		          WHERE gv.superior_id = g.id AND gv.criado_por_superior = 1 AND gv.criado_por_subordinado = 1),
		       (SELECT GROUP_CONCAT(g3.id) FROM grupo_vinculos gv2
		          JOIN grupos g3 ON g3.id = gv2.superior_id
		          WHERE gv2.subordinado_id = g.id AND gv2.criado_por_superior = 1 AND gv2.criado_por_subordinado = 1)
		FROM grupos g ORDER BY g.nome`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome, cod, criado string
		var contas, efetivo int
		var sub, sup, subIDs, supIDs sql.NullString
		if rows.Scan(&id, &nome, &cod, &criado, &contas, &efetivo, &sub, &sup, &subIDs, &supIDs) == nil {
			if escopo > 0 && id != escopo {
				// gerente/operador só enxerga o próprio grupo (sub/superiores chegam pelas colunas)
				continue
			}
			out = append(out, map[string]any{
				"id": id, "nome": nome, "codigo": cod, "criado_em": criado,
				"contas": contas, "efetivo": efetivo,
				"subordinados": nilToSlice(sub), "superiores": nilToSlice(sup),
				"subordinados_ids": nilToInts(subIDs), "superiores_ids": nilToInts(supIDs),
			})
		}
	}
	return out
}

func (a *App) hArvoreGrupos(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		if errors.Is(err, ErrContaSemGrupo) {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		jsonErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}
	rows, err := a.st.db.Query(`
		SELECT g.id, g.nome, COALESCE(g.codigo,''),
		       (SELECT COUNT(*) FROM pessoas p WHERE p.grupo_id = g.id AND p.status = 'ativo'),
		       (SELECT COUNT(*) FROM usuarios us WHERE us.grupo_id = g.id AND us.ativo = 1)
		FROM grupos g ORDER BY g.nome`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	type GrupoN struct {
		ID             int64     `json:"id"`
		Nome           string    `json:"nome"`
		Codigo         string    `json:"codigo"`
		Efetivo        int       `json:"efetivo"`         // próprio (sem subordinados)
		EfetivoTotal   int       `json:"efetivo_total"`   // recursivo: próprio + subárvore
		Contas         int       `json:"contas"`          // próprias (sem subordinados)
		ContasTotal    int       `json:"contas_total"`    // recursivo: próprias + subárvore
		SubgruposTotal int       `json:"subgrupos_total"` // total de grupos subordinados na subárvore
		Filhos         []*GrupoN `json:"filhos"`
	}
	nos := map[int64]*GrupoN{}
	filhosDe := map[int64][]int64{}
	paiDe := map[int64]int64{}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var nome, cod string
		var efetivo, contas int
		if err := rows.Scan(&id, &nome, &cod, &efetivo, &contas); err != nil {
			continue
		}
		nos[id] = &GrupoN{ID: id, Nome: nome, Codigo: cod, Efetivo: efetivo, Contas: contas, Filhos: []*GrupoN{}}
	}
	linhas, err := a.st.db.Query(`SELECT superior_id, subordinado_id FROM grupo_vinculos
		WHERE criado_por_superior = 1 AND criado_por_subordinado = 1`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer linhas.Close()
	for linhas.Next() {
		var sup, sub int64
		if linhas.Scan(&sup, &sub) == nil {
			filhosDe[sup] = append(filhosDe[sup], sub)
			paiDe[sub] = sup
		}
	}
	var montar func(id int64, profundidade int) *GrupoN
	montar = func(id int64, profundidade int) *GrupoN {
		no := nos[id]
		if no == nil || profundidade > 32 {
			return nil
		}
		no.EfetivoTotal = no.Efetivo
		no.ContasTotal = no.Contas
		no.SubgruposTotal = 0
		for _, filho := range filhosDe[id] {
			if f := montar(filho, profundidade+1); f != nil {
				no.Filhos = append(no.Filhos, f)
				no.EfetivoTotal += f.EfetivoTotal
				no.ContasTotal += f.ContasTotal
				no.SubgruposTotal += 1 + f.SubgruposTotal
			}
		}
		return no
	}
	raizes := []int64{}
	for id := range nos {
		if paiDe[id] == 0 {
			raizes = append(raizes, id)
		}
	}
	if escopo > 0 {
		// gerente/operador: árvore enraizada no PRÓPRIO grupo
		if raiz := montar(escopo, 0); raiz != nil {
			jsonOK(w, []*GrupoN{raiz})
			return
		}
		jsonOK(w, []*GrupoN{})
		return
	}
	out := []*GrupoN{}
	for _, id := range raizes {
		if no := montar(id, 0); no != nil {
			out = append(out, no)
		}
	}
	jsonOK(w, out)
}

func (a *App) hGruposAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome       string `json:"nome"`
		Login      string `json:"login"`
		Senha      string `json:"senha"`
		NomeGuerra string `json:"nome_guerra"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome do grupo obrigatório")
		return
	}

	cod := gerarCodigoGrupo()
	for tenta := 0; tenta < 8; tenta++ {
		var existe int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE codigo = ?`, cod).Scan(&existe)
		if existe == 0 {
			break
		}
		cod = gerarCodigoGrupo()
	}

	req.Login = strings.ToLower(strings.TrimSpace(req.Login))
	criarGerente := req.Login != "" && len(req.Senha) >= 8

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO grupos (nome, codigo) VALUES (?,?)`, strings.TrimSpace(req.Nome), cod)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "grupo não criado (duplicado?): "+err.Error())
		return
	}
	gid, _ := res.LastInsertId()

	var uid int64
	if criarGerente {
		var existeLogin int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE login = ?`, req.Login).Scan(&existeLogin)
		if existeLogin > 0 {
			jsonErro(w, http.StatusBadRequest, "login já existe")
			return
		}
		hash, err := hashSenha(req.Senha)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		ng := strings.TrimSpace(req.NomeGuerra)
		if ng == "" {
			ng = req.Login
		}
		precisaSetup := 1
		if req.Senha != "" && req.Senha != "sci" {
			precisaSetup = 0
		}
		res2, err := tx.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, nome_guerra, nome_completo, precisa_setup)
			VALUES (?,?,?,?,?,?,?)`,
			req.Login, hash, "gerente", gid, ng, ng, precisaSetup)
		if err != nil {
			jsonErro(w, http.StatusBadRequest, "gerente não criado: "+err.Error())
			return
		}
		uid, _ = res2.LastInsertId()
		_, _ = tx.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?,?)`,
			uid, gid, "gerente")
	}

	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	det := req.Nome + " [" + cod + "]"
	if criarGerente {
		det += " gerente=" + req.Login
	}
	a.st.Auditoria(&u.ID, "criar", "grupos", &gid, det, ipDe(r))
	resp := map[string]any{"id": gid, "codigo": cod}
	if uid > 0 {
		resp["gerente_id"] = uid
	}
	jsonOK(w, resp)
}

func (a *App) hGrupoGerenteGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var login string
	var tem int
	err = a.st.db.QueryRow(`SELECT login, 1 FROM usuarios WHERE grupo_id = ? AND papel = 'gerente' AND ativo = 1 LIMIT 1`, id).Scan(&login, &tem)
	if err != nil {
		jsonOK(w, map[string]any{"gerente": ""})
		return
	}
	jsonOK(w, map[string]any{"gerente": login})
}

func (a *App) hGrupoTrocarGerente(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Login string `json:"login"`
	}
	if err = decodificar(r, &req); err != nil || strings.TrimSpace(req.Login) == "" {
		jsonErro(w, http.StatusBadRequest, "login do novo gerente obrigatório (ou __REMOVE__ para vagar)")
		return
	}
	req.Login = strings.ToLower(strings.TrimSpace(req.Login))

	removeGerente := req.Login == "__remove__"

	var uid int64
	var papel string
	var uGrupoID *int64

	if !removeGerente {
		if err := a.st.db.QueryRow(`SELECT id, papel, grupo_id FROM usuarios WHERE login = ? AND ativo = 1`,
			req.Login).Scan(&uid, &papel, &uGrupoID); err != nil {
			jsonErro(w, http.StatusNotFound, "conta não encontrada")
			return
		}
		if papel == "gerente" && uGrupoID != nil && *uGrupoID == gid {
			jsonErro(w, http.StatusBadRequest, "esta conta já é o gerente deste grupo")
			return
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Rebaixa qualquer gerente anterior do grupo para operador
	if removeGerente {
		_, _ = tx.Exec(`UPDATE usuarios SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND ativo = 1`, gid)
		_, _ = tx.Exec(`UPDATE usuario_papeis SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente'`, gid)
	} else {
		_, _ = tx.Exec(`UPDATE usuarios SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND id <> ? AND ativo = 1`, gid, uid)
		_, _ = tx.Exec(`UPDATE usuario_papeis SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND usuario_id <> ?`, gid, uid)

		// Promove o usuário selecionado a Gerente e vincula ao grupo
		if _, err = tx.Exec(`UPDATE usuarios SET papel = 'gerente', grupo_id = ? WHERE id = ?`, gid, uid); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Atualiza ou insere o papel em usuario_papeis (herança de função)
		var papelID int64
		errP := tx.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ?`, uid, gid).Scan(&papelID)
		if errP == nil {
			_, _ = tx.Exec(`UPDATE usuario_papeis SET papel = 'gerente' WHERE id = ?`, papelID)
		} else {
			_, _ = tx.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, uid, gid)
		}
	}

	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "definir_gerente", "grupos", &gid, "novo="+req.Login, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "gerente": req.Login})
}

func (a *App) hGrupoUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	if u == nil {
		jsonErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}
	if u.Papel != "admin" {
		if u.Papel != "gerente" || u.GrupoID == nil || !a.st.EhSubordinado(*u.GrupoID, id) {
			jsonErro(w, http.StatusForbidden, "sem permissão para alterar este grupo")
			return
		}
	}

	var req struct {
		Nome   *string `json:"nome"`
		Codigo *string `json:"codigo"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "payload inválido: "+err.Error())
		return
	}

	var nomeAtual, codigoAtual string
	if err := a.st.db.QueryRow(`SELECT nome, COALESCE(codigo,'') FROM grupos WHERE id = ?`, id).Scan(&nomeAtual, &codigoAtual); err != nil {
		jsonErro(w, http.StatusNotFound, "grupo inexistente")
		return
	}

	novoNome := nomeAtual
	if req.Nome != nil {
		trimmed := strings.TrimSpace(*req.Nome)
		if trimmed == "" {
			jsonErro(w, http.StatusBadRequest, "nome do grupo não pode ser vazio")
			return
		}
		var existenteID int64
		if err := a.st.db.QueryRow(`SELECT id FROM grupos WHERE LOWER(nome) = LOWER(?) AND id <> ?`, trimmed, id).Scan(&existenteID); err == nil {
			jsonErro(w, http.StatusBadRequest, "já existe outro grupo com este nome")
			return
		}
		novoNome = trimmed
	}

	novoCodigo := codigoAtual
	if req.Codigo != nil {
		trimmedCod := strings.ToUpper(strings.TrimSpace(*req.Codigo))
		if trimmedCod != "" {
			var existenteID int64
			if err := a.st.db.QueryRow(`SELECT id FROM grupos WHERE UPPER(codigo) = ? AND id <> ?`, trimmedCod, id).Scan(&existenteID); err == nil {
				jsonErro(w, http.StatusBadRequest, "já existe outro grupo com este código")
				return
			}
		}
		novoCodigo = trimmedCod
	}

	if _, err := a.st.db.Exec(`UPDATE grupos SET nome = ?, codigo = ? WHERE id = ?`, novoNome, novoCodigo, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao atualizar grupo: "+err.Error())
		return
	}

	detalhes := fmt.Sprintf("nome=%s codigo=%s", novoNome, novoCodigo)
	a.st.Auditoria(&u.ID, "editar", "grupos", &id, detalhes, ipDe(r))

	jsonOK(w, map[string]any{
		"ok":     true,
		"id":     id,
		"nome":   novoNome,
		"codigo": novoCodigo,
	})
}

// ---------- rotas rotasGrupos ----------
func (a *App) rotasGrupos() {
	m := a.mux

	// Onda C2 (05/10): Aba Funções — designação de membros por função (gerente).
	m.Handle("GET /api/grupo/funcoes/membros", a.auth(false, a.hFuncaoMembrosGet))
	m.Handle("POST /api/grupo/funcoes/membros", a.auth(false, a.hFuncaoMembrosSet))
	m.Handle("DELETE /api/grupo/funcoes/membros/{id}", a.auth(false, a.hFuncaoMembrosDel))
	m.Handle("POST /api/grupos/{id}/nomear_chefe", a.auth(false, a.hGrupoNomearChefe))
	m.Handle("POST /api/grupos/{id}/destituir_chefe", a.auth(false, a.hGrupoDestituirChefe))
	m.Handle("DELETE /api/grupos/{id}", a.auth(true, a.hGrupoExcluir)) // v9.7: só admin, só grupo vazio
	m.Handle("GET /api/grupos", a.auth(false, a.hGruposList))
	m.Handle("GET /api/grupos/arvore", a.auth(false, a.hArvoreGrupos)) // v9.4: árvore nested (admin: floresta; gerente: do próprio)
	m.Handle("POST /api/grupos", a.auth(true, a.hGruposAdd))           // R7: exige gerente no ato
	m.Handle("GET /api/grupos/{id}/gerente", a.auth(true, a.hGrupoGerenteGet))
	m.Handle("POST /api/grupos/{id}/trocar-gerente", a.auth(true, a.hGrupoTrocarGerente))
	m.Handle("PATCH /api/grupos/{id}", a.auth(false, a.hGrupoUpdate))
	m.Handle("POST /api/admin/grupos/vinculo", a.auth(true, a.hAdminVinculoSet))   // R8
	m.Handle("DELETE /api/admin/grupos/vinculo", a.auth(true, a.hAdminVinculoRem)) // R8
	m.Handle("GET /api/vinculos", a.auth(false, a.hVinculoList))
	m.Handle("POST /api/vinculos", a.auth(false, a.hVinculoAdd)) // legado: fora da UI (R12)
}
