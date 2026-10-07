// helpers.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"database/sql"
	"encoding/json"
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
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` AND p.grupo_id = ?
			ORDER BY COALESCE(s.nome,''), p.nome_guerra`, escopo)
	} else if escopo == 0 {
		rows, err = a.st.db.Query(q + `
			ORDER BY COALESCE(s.nome,''), p.nome_guerra`)
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

func filtroGrupoSQL(escopo int64, alias string) string {
	if escopo <= 0 {
		return ""
	}
	return ` AND ` + alias + `.grupo_id = ?`
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
	} else {
		rows, err = a.st.db.Query(q + `
			ORDER BY p.status, COALESCE(s.nome,''), p.nome_guerra`)
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
