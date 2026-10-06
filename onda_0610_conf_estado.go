package main

// Onda 06/10 — P4 (item 4 do email do Diretor): SINCRONIZAÇÃO da conferência
// com HASH DE ESTADO POR SETOR. Fonte de verdade = servidor; o cliente faz
// tick de 2s em GET /api/conferencia/estado?id=N e só age quando o hash muda:
// hash igual = ZERO mutação de DOM; mudou = aplica SÓ o setor alterado.
// Contrato: {hash_geral, setores:[{setor_id, hash}]} — SHA-1 (crypto/sha1)
// sobre estados/contagens POR SETOR, UMA query com subselects correlacionados
// (pool SQLite de 1 conexão — NUNCA QueryRow dentro de rows.Next()).

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strconv"
)

// hConferenciaEstado: hash do estado da conferência aberta, por setor.
// Mesmos guardas do pooling atual (GET /api/conferencia/hoje, a.auth(false)):
// escopo do usuário; admin (sem grupo) recebe hash vazio; ?id=N ou a aberta
// mais recente do grupo. Sem conferência aberta: hash_geral "" (o front trata
// como "estado sumiu" e recarrega a view, igual ao pooling de hoje).
func (a *App) hConferenciaEstado(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	setoresVazio := []map[string]string{}
	if escopo == 0 && u.Papel == "admin" {
		jsonOK(w, map[string]any{"hash_geral": "", "setores": setoresVazio})
		return
	}
	idQ := r.URL.Query().Get("id")
	qConf := `SELECT id FROM conferencias WHERE status = 'aberta' AND grupo_id = ?`
	args := []any{escopo}
	if idQ != "" {
		if cid, e := strconv.ParseInt(idQ, 10, 64); e == nil {
			qConf += ` AND id = ?`
			args = append(args, cid)
		}
	}
	qConf += ` ORDER BY id DESC LIMIT 1`
	var cid int64
	if err := a.st.db.QueryRow(qConf, args...).Scan(&cid); err != nil {
		jsonOK(w, map[string]any{"hash_geral": "", "setores": setoresVazio})
		return
	}

	// UMA query: para cada setor ativo do escopo, concatenação determinística
	// do estado de CADA militar (pessoa sem lançamento contribui token
	// constante '-') + status setorial + contagens. A ordem é p.id (estável
	// entre chamadas — pré-condição do hash como impressão digital).
	rows, err := a.st.db.Query(`
		SELECT s.id,
		       COALESCE((SELECT cs.status FROM conferencia_setores cs
		                 WHERE cs.conferencia_id = ? AND cs.setor_id = s.id), 'nao_iniciada'),
		       (SELECT group_concat(x.tok, '')
		          FROM (SELECT COALESCE((
		                 SELECT pr.pessoa_id || ':' || COALESCE(pr.situacao, '') || ':'
		                        || COALESCE(pr.destino_id, 0) || ':' || COALESCE(pr.observacao, '') || ':'
		                        || COALESCE(pr.verificado, 0)
		                 FROM presencas pr
		                 WHERE pr.conferencia_id = ? AND pr.pessoa_id = p.id), '-')
		                 AS tok
		              FROM pessoas p
		              WHERE p.setor_id = s.id AND p.status = 'ativo'
		                AND (? <= 0 OR p.grupo_id = ?)
		              ORDER BY p.id) x),
		       (SELECT COUNT(*) FROM pessoas p3
		         WHERE p3.setor_id = s.id AND p3.status = 'ativo' AND (? <= 0 OR p3.grupo_id = ?)),
		       (SELECT COUNT(*) FROM pessoas p4
		         JOIN presencas pr2 ON pr2.conferencia_id = ? AND pr2.pessoa_id = p4.id AND pr2.verificado = 1
		         WHERE p4.setor_id = s.id AND p4.status = 'ativo' AND (? <= 0 OR p4.grupo_id = ?))
		FROM setores s
		WHERE s.ativo = 1 AND (? <= 0 OR s.grupo_id = ? OR s.grupo_id IS NULL)
		ORDER BY s.id
	`, cid, cid, escopo, escopo, escopo, escopo, cid, escopo, escopo, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	geral := sha1.New()
	setores := []map[string]string{}
	for rows.Next() {
		var sid int64
		var status, estadosConcat string
		var total, verificados int
		if err := rows.Scan(&sid, &status, &estadosConcat, &total, &verificados); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		h := sha1.New()
		h.Write([]byte(status))
		h.Write([]byte{'|'})
		h.Write([]byte(estadosConcat))
		h.Write([]byte{'|'})
		h.Write([]byte(strconv.Itoa(total)))
		h.Write([]byte{'|'})
		h.Write([]byte(strconv.Itoa(verificados)))
		hashSetor := hex.EncodeToString(h.Sum(nil))
		geral.Write([]byte(hashSetor))
		setores = append(setores, map[string]string{"setor_id": strconv.FormatInt(sid, 10), "hash": hashSetor})
	}
	if err := rows.Err(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"hash_geral": hex.EncodeToString(geral.Sum(nil)), "setores": setores})
}
