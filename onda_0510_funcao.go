package main

// onda_0510_funcao.go — Onda 05/10 "Drive/Email POR FUNÇÃO" (DELIB-0010 B +
// decisões Q1/Q3 do Diretor).
//
// PRINCÍPIO: a FUNÇÃO é a dona do registro no drive e no email; usuários mudam,
// a função permanece. O sucessor na função HERDA o que a função possui (Q3) e
// o auxiliar (quando existir — hoje em stand-by, Q1) herda igual, pois a
// resolução olha funcao_membros sem filtrar titularidade.
//
// ACESSO (dupla chave, ADITIVA — ninguém perde acesso):
//   pasta/arquivo = (autor ∪ gerência ∪ grants ∪ herança de pasta) OU
//                   (funcao_id = função que o usuário EXERCE em funcao_membros).
//   O ramo da função é SÓ leitura: edição continua exigindo o caminho clássico
//   (autor/gerência/grant pode_editar). NULL = legado de usuário → regra antiga.
//
// EMAIL: caixa da função — mensagem com mensagens.funcao_id = função que o
// usuário exerce entra no inbox dele (UNION lógico com a caixa pessoal).

import (
	"net/http"
)

// funcoesExercidas: ids das funções que o usuário exerce (titular OU auxiliar —
// Q1: mesmo acesso; auxiliar em stand-by) no grupo do papel ativo da sessão.
// Sem papel/grupo ativo → vazio. Cache de sessão não: query pontual, barata
// (índice idx_funcao_membros_grupo).
func (a *App) funcoesExercidas(u *Usuario) []int64 {
	if u == nil || u.GrupoID == nil {
		return nil
	}
	rows, err := a.st.db.Query(`SELECT funcao_id FROM funcao_membros WHERE usuario_id = ? AND grupo_id = ?`,
		u.ID, *u.GrupoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var f int64
		if rows.Scan(&f) == nil {
			out = append(out, f)
		}
	}
	return out
}

// funcaoExercidaByID: a função de id fID está entre as que o usuário exerce?
func (a *App) funcaoExercidaByID(u *Usuario, fID int64) bool {
	if fID <= 0 {
		return false
	}
	for _, f := range a.funcoesExercidas(u) {
		if f == fID {
			return true
		}
	}
	return false
}

// funcaoTitularNoGrupo: função em que o usuário é TITULAR no grupo gID — mesma
// resolução do backflow v34 (MIN(funcao_id), titularidade='titular'): 1 titular
// por função, mas o usuário pode titularizar mais de uma, então MIN amarra o
// determinismo. nil (NULL) quando não exerce → registro nasce legado de usuário.
// Usado no CARIMBO DA ESCRITA (pasta/upload/cópia/mensagem).
func (a *App) funcaoTitularNoGrupo(u *Usuario, grupoID int64) *int64 {
	if u == nil || grupoID <= 0 {
		return nil
	}
	var fID *int64
	_ = a.st.db.QueryRow(`SELECT MIN(funcao_id) FROM funcao_membros
		WHERE usuario_id = ? AND grupo_id = ? AND titularidade = 'titular'`,
		u.ID, grupoID).Scan(&fID)
	return fID
}

// grupoInt64De: meta["grupo_id"] (checarAcessoArquivo) vem como int64 do Scan —
// tolera any por segurança e devolve 0 quando ausente (funcaoTitularNoGrupo
// trata 0 como "sem grupo" → NULL).
func grupoInt64De(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	}
	return 0
}

// acessoViaFuncaoPasta: braço ADITIVO da função para pastas. true só quando a
// pasta é POSSESSO da função (funcao_id exercida pelo usuário) e não pede edição.
func (a *App) acessoViaFuncaoPasta(u *Usuario, pastaID int64, precisaEdicao bool) bool {
	if precisaEdicao || pastaID <= 0 {
		return false
	}
	var fID *int64
	if err := a.st.db.QueryRow(`SELECT funcao_id FROM drive_pastas WHERE id = ?`, pastaID).Scan(&fID); err != nil {
		return false
	}
	return fID != nil && a.funcaoExercidaByID(u, *fID)
}

// acessoViaFuncaoArquivo: braço ADITIVO da função para arquivos. true só quando
// o arquivo é POSSESSO da função (funcao_id exercida) e não pede edição.
func (a *App) acessoViaFuncaoArquivo(u *Usuario, arquivoID int64, precisaEdicao bool) bool {
	if precisaEdicao || arquivoID <= 0 {
		return false
	}
	var fID *int64
	if err := a.st.db.QueryRow(`SELECT funcao_id FROM drive_arquivos WHERE id = ?`, arquivoID).Scan(&fID); err != nil {
		return false
	}
	return fID != nil && a.funcaoExercidaByID(u, *fID)
}

// hDriveItensDaFuncao: GET /api/drive/da_funcao — itens POSSESSO da(s) função(s)
// que o usuário exerce (badge "da função" no front usa o mesmo critério).
func (a *App) hDriveItensDaFuncao(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	funs := a.funcoesExercidas(u)
	if len(funs) == 0 {
		jsonOK(w, map[string]any{"pastas": []any{}, "arquivos": []any{}, "funcao_ids": []any{}})
		return
	}
	qArgs := []any{}
	marcas := ""
	for i, f := range funs {
		if i > 0 {
			marcas += ","
		}
		marcas += "?"
		qArgs = append(qArgs, f)
	}

	pastas := []map[string]any{}
	pRows, err := a.st.db.Query(`
		SELECT dp.id, dp.nome, COALESCE(NULLIF(u2.nome_guerra,''), NULLIF(u2.nome_completo,''), '—')
		FROM drive_pastas dp
		LEFT JOIN usuarios u2 ON u2.id = dp.autor_usuario_id
		WHERE dp.funcao_id IN (`+marcas+`)
		ORDER BY dp.nome ASC`, qArgs...)
	if err == nil {
		for pRows.Next() {
			var id int64
			var nome, autor string
			if pRows.Scan(&id, &nome, &autor) == nil {
				pastas = append(pastas, map[string]any{"id": id, "nome": nome, "autor_nome": autor})
			}
		}
		pRows.Close()
	}

	arquivos := []map[string]any{}
	aRows, errA := a.st.db.Query(`
		SELECT da.id, da.pasta_id, da.nome_original, COALESCE(NULLIF(u2.nome_guerra,''), NULLIF(u2.nome_completo,''), '—')
		FROM drive_arquivos da
		LEFT JOIN usuarios u2 ON u2.id = da.autor_usuario_id
		WHERE da.funcao_id IN (`+marcas+`)
		ORDER BY da.criado_em DESC`, qArgs...)
	if errA == nil {
		for aRows.Next() {
			var id int64
			var nome, autor string
			var pastaID *int64
			if aRows.Scan(&id, &pastaID, &nome, &autor) == nil {
				arquivos = append(arquivos, map[string]any{"id": id, "pasta_id": pastaID, "nome_original": nome, "autor_nome": autor})
			}
		}
		aRows.Close()
	}

	funsAny := []any{}
	for _, f := range funs {
		funsAny = append(funsAny, f)
	}
	jsonOK(w, map[string]any{"pastas": pastas, "arquivos": arquivos, "funcao_ids": funsAny})
}
