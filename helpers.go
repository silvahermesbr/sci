// helpers.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErro(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"erro": msg})
}

func decodificar(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func (a *App) pessoasAtivas(escopo int64) []map[string]any {
	return a.pessoasAtivasOpt(escopo, nil)
}

// pessoasAtivasOpt: pessoasAtivas com filtro opcional de funções (onda 09/10,
// conferência por antiguidade). funcoes não vazio → só entram pessoas cuja
// função (p.funcao_id, u2.funcao_id ou up2.funcao_id) esteja na lista.
func (a *App) pessoasAtivasOpt(escopo int64, funcoes []int64) []map[string]any {
	q := `
		SELECT p.id, p.nome_guerra, p.nome_completo, COALESCE(s.nome,''),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))),
		       p.setor_id
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		WHERE p.status = 'ativo'`
	funcoesArgs := []any{}
	if len(funcoes) > 0 {
		ph := ""
		for i := range funcoes {
			if i > 0 {
				ph += ","
			}
			ph += "?"
		}
		q += ` AND (p.funcao_id IN (` + ph + `) OR u2.funcao_id IN (` + ph + `) OR up2.funcao_id IN (` + ph + `))`
		for i := 0; i < 3; i++ {
			for _, fid := range funcoes {
				funcoesArgs = append(funcoesArgs, fid)
			}
		}
	}
	var rows *sql.Rows
	var err error
	// v1.5.4-D3 (R-7): no modo antiguidade (filtro de funções ativo) a listagem
	// segue a antiguidade UNIFICADA das 3 fontes da tag — antes ordenava por
	// setor/nome e a escada quebrava para quem só tem tag na conta ou no papel.
	ordem := `ORDER BY COALESCE(s.nome,''), p.nome_guerra, p.id`
	if len(funcoes) > 0 {
		ordem = `ORDER BY ` + ordemAntiguidadeTresFontes("p.nome_guerra")
	}
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` AND p.grupo_id = ?
			`+ordem, append(funcoesArgs, escopo)...)
	} else if escopo == 0 {
		rows, err = a.st.db.Query(q+`
			`+ordem, funcoesArgs...)
	} else {
		return []map[string]any{}
	}
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ng, nc, setor, funcao string
		var setorID *int64
		if rows.Scan(&id, &ng, &nc, &setor, &funcao, &setorID) == nil {
			item := map[string]any{
				"id": id, "nome_guerra": ng, "nome_completo": nc, "setor": setor, "funcao": funcao,
			}
			if setorID != nil {
				item["setor_id"] = *setorID
			} else {
				item["setor_id"] = nil
			}
			out = append(out, item)
		}
	}
	return out
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

var (
	ErrContaSemGrupo  = errors.New("conta sem grupo definido")
	ErrNaoAutenticado = errors.New("não autenticado")
)

// papeisComEscopoDeDados (v1.6.0 Fase 5 — conta SEM FUNÇÃO): a LISTA ÚNICA de
// papeis que têm escopo de dados. Papel fora dela — 'sem_funcao', vazio
// (legado sem cadeira) ou qualquer valor não reconhecido — NÃO tem escopo,
// MESMO COM grupo no cadastro: exigeEscopo/escopoDoUsuario devolvem -1 e a
// guarda central da v1.5.4-A (403 + filtroGrupoSQL AND 1=0) barra o resto.
// 'sem_funcao' é ausência de contexto (não é papel de linha — fica FORA do
// CHECK de usuario_papeis por design): a conta nasce por hUsuariosAdd sem
// linha em usuario_papeis e volta a valer quando o gerente/enc designa uma
// cadeira (linha materializada → próximo login resolve o contexto).
// Drive segue por ACL de grant (exceção aceita, registrada no mapa da onda).
var papeisComEscopoDeDados = map[string]bool{
	"admin":        true,
	"gerente":      true,
	"operador":     true,
	"chefe_setor":  true,
	"enc_pessoal":  true,
	"enc_material": true,
}

// papelTemEscopoDeDados: o papel ativo dá acesso a dados de grupo?
func papelTemEscopoDeDados(papel string) bool {
	return papeisComEscopoDeDados[papel]
}

func (a *App) exigeEscopo(u *Usuario) (int64, error) {
	if u == nil {
		return -1, ErrNaoAutenticado
	}
	// v1.6.0 Fase 5: papel fora da lista → -1 (ErrContaSemGrupo deixa o
	// mapeamento central intacto: 403 em todos os handlers, nunca 401).
	if !papelTemEscopoDeDados(u.Papel) {
		return -1, ErrContaSemGrupo
	}
	if u.Papel == "admin" {
		return 0, nil
	}
	if u.GrupoID == nil {
		return -1, ErrContaSemGrupo
	}
	return *u.GrupoID, nil
}

func filtroGrupoSQL(escopo int64, alias string) string {
	if escopo < 0 {
		return " AND 1 = 0"
	}
	if escopo == 0 {
		return ""
	}
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return ` AND ` + prefix + `grupo_id = ?`
}

func filtroGrupoArgs(escopo int64) []any {
	if escopo <= 0 {
		return nil
	}
	return []any{escopo}
}

func int64Contem(lista []int64, v int64) bool {
	for _, x := range lista {
		if x == v {
			return true
		}
	}
	return false
}

func (a *App) periodoPadrao(r *http.Request) (string, string) {
	hoje := time.Now().In(a.horaLocal)
	de := r.URL.Query().Get("de")
	ate := r.URL.Query().Get("ate")
	if ate == "" {
		ate = hoje.Format("2006-01-02")
	}
	if de == "" {
		de = hoje.AddDate(0, 0, -6).Format("2006-01-02")
	}
	return de, ate
}

func (a *App) pessoasTodas(escopo int64) []map[string]any {
	q := `
		SELECT p.id, p.nome_guerra, p.nome_completo, p.setor_id,
		       COALESCE(p.funcao_id, u2.funcao_id, up2.funcao_id), p.status,
		       COALESCE(s.nome,''),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))),
		       COALESCE(g.nome,'')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN grupos g ON g.id = p.grupo_id`
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` WHERE p.grupo_id = ?
			ORDER BY p.status, COALESCE(s.nome,''), p.nome_guerra`, escopo)
	} else if escopo == 0 {
		rows, err = a.st.db.Query(q + `
			ORDER BY p.status, COALESCE(s.nome,''), p.nome_guerra`)
	} else {
		return []map[string]any{}
	}
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var setorID, funcaoID *int64 // NULL = sem setor/função (não descartar a linha!)
		var ng, nc, status, setor, funcao, grupo string
		if rows.Scan(&id, &ng, &nc, &setorID, &funcaoID, &status, &setor, &funcao, &grupo) == nil {
			out = append(out, map[string]any{
				"id": id, "nome_guerra": ng, "nome_completo": nc,
				"setor_id": setorID, "funcao_id": funcaoID,
				"setor": setor, "funcao": funcao, "status": status, "grupo": grupo,
			})
		}
	}
	return out
}

func nilToSlice(ns sql.NullString) []string {
	if !ns.Valid || ns.String == "" {
		return []string{}
	}
	return strings.Split(ns.String, ",")
}

func nilToInts(ns sql.NullString) []int64 {
	out := []int64{}
	for _, p := range nilToSlice(ns) {
		if v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}
