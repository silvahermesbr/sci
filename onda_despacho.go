package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type confIniciarReq struct {
	Local       string  `json:"local"`
	Nome        string  `json:"nome"`
	PrazoFinal  string  `json:"prazo_final"`
	Encarregado *int64  `json:"encarregado_usuario_id"`
	Setores     []int64 `json:"setores"`
	FuncaoIDs   []int64 `json:"funcao_ids"`
}

// validarSetoresDoGrupo verifica se cada setor existe, está ativo e pertence ao grupo (ou é global).
// Remove duplicados preservando a ordem original.
func (a *App) validarSetoresDoGrupo(grupoID int64, setores []int64) ([]int64, error) {
	if len(setores) == 0 {
		return nil, errors.New("selecione ao menos um setor")
	}
	vistos := make(map[int64]bool)
	var unicos []int64
	for _, sid := range setores {
		if sid <= 0 {
			return nil, errors.New("setor inválido ou inexistente")
		}
		if vistos[sid] {
			continue
		}
		var achou int64
		err := a.st.db.QueryRow(`SELECT id FROM setores WHERE id = ? AND ativo = 1 AND (grupo_id = ? OR grupo_id IS NULL)`, sid, grupoID).Scan(&achou)
		if err != nil {
			return nil, fmt.Errorf("setor %d inválido ou inexistente", sid)
		}
		vistos[sid] = true
		unicos = append(unicos, sid)
	}
	return unicos, nil
}

// validarFuncoesAntiguidadeDoGrupo (correção 09/10 — ordem do dono): valida
// que cada id existe, está ativo, é função de posto/graduação (tipo
// 'antiguidade' ou legado NULL) e pertence ao GRUPO da conferência
// (funcoes.grupo_id = grupoID). A seed global (grupo_id NULL) NÃO vale mais
// para conferências novas — antiguidade é a cadastrada pelo gerente do grupo.
// Chame ANTES de criar a conferência — erro aqui é 400, nunca conferência órfã.
func (a *App) validarFuncoesAntiguidadeDoGrupo(grupoID int64, ids []int64) error {
	for _, fid := range ids {
		var n int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE id = ? AND ativo = 1 AND (tipo = 'antiguidade' OR tipo IS NULL) AND grupo_id = ?`, fid, grupoID).Scan(&n)
		if n == 0 {
			return fmt.Errorf("funcao_id %d invalida, inativa ou nao pertence ao grupo", fid)
		}
	}
	return nil
}

// criarConferenciaBase cria a conferência com tipo padrão, aplica carry-over e escalas,
// e inicializa conferencias setoriais. Refatorado de hConferenciaIniciar para zero duplicação.
func (a *App) criarConferenciaBase(u *Usuario, req confIniciarReq) (int64, error) {
	if u.GrupoID == nil {
		return 0, errors.New("conta sem grupo definido")
	}
	grupoID := *u.GrupoID
	data := time.Now().In(a.horaLocal).Format("2006-01-02")
	if strings.TrimSpace(req.Nome) == "" {
		req.Nome = tipoConferenciaPadrao + " — " + data
	}
	tipoID, _, err := a.tipoPadraoID()
	if err != nil {
		return 0, fmt.Errorf("tipo '%s' inexistente", tipoConferenciaPadrao)
	}
	criadoEm := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, e := a.st.db.Exec(
		`INSERT INTO conferencias (data, tipo_id, local, grupo_id, criado_por, criado_em, nome, prazo_final, encarregado_usuario_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		data, tipoID, req.Local, grupoID, u.ID, criadoEm,
		strings.TrimSpace(req.Nome), strings.TrimSpace(req.PrazoFinal), req.Encarregado)
	if e != nil {
		return 0, e
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Carry-over de conferências fechadas
	if _, err := a.st.db.Exec(`INSERT INTO presencas
		(conferencia_id, pessoa_id, situacao, destino_id, observacao, marcado_por, marcado_em, verificado)
		SELECT ?, pr.pessoa_id, pr.situacao, pr.destino_id, pr.observacao, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 0
		FROM presencas pr
		JOIN conferencias c ON c.id = pr.conferencia_id AND c.status = 'fechada'
		JOIN (SELECT pessoa_id, conferencia_id FROM (
		      SELECT p2.pessoa_id, p2.conferencia_id,
		             ROW_NUMBER() OVER (PARTITION BY p2.pessoa_id ORDER BY c2.data DESC, c2.id DESC) rn
		      FROM presencas p2 JOIN conferencias c2 ON c2.id = p2.conferencia_id AND c2.status='fechada'
		      ) WHERE rn = 1) u2 ON u2.pessoa_id = pr.pessoa_id AND u2.conferencia_id = c.id
		WHERE julianday(?) - julianday(c.data) <= 1
		  AND pr.pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ? AND status = 'ativo')`,
		id, u.ID, data, grupoID); err != nil {
		log.Printf("sci carry-over conf %d: %v", id, err)
	}

	// Motor Inteligente de Escalas
	var destServicoID int64
	_ = a.st.db.QueryRow(`SELECT id FROM destinos WHERE (grupo_id = ? OR grupo_id IS NULL) AND LOWER(nome) LIKE '%serviço%' AND ativo = 1 ORDER BY grupo_id DESC LIMIT 1`, grupoID).Scan(&destServicoID)
	if destServicoID == 0 {
		resD, errD := a.st.db.Exec(`INSERT INTO destinos (grupo_id, nome, ativo) VALUES (?, 'Serviço de Escala', 1)`, grupoID)
		if errD == nil {
			destServicoID, _ = resD.LastInsertId()
		}
	}
	if destServicoID > 0 {
		if _, err := a.st.db.Exec(`
			INSERT INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, observacao, marcado_por, marcado_em, verificado)
			SELECT ?, ep.pessoa_id, 'justificada', ?, 'Escala: ' || etp.nome, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 0
			FROM escala_pessoas ep
			JOIN escala_turnos et ON et.id = ep.turno_id
			JOIN escala_tipos etp ON etp.id = et.tipo_id
			JOIN pessoas pes ON pes.id = ep.pessoa_id AND pes.status = 'ativo' AND pes.grupo_id = ?
			WHERE substr(et.data_inicio, 1, 10) <= ? AND substr(COALESCE(NULLIF(et.data_fim, ''), et.data_inicio), 1, 10) >= ?
			ON CONFLICT(conferencia_id, pessoa_id) DO UPDATE SET
			  situacao = 'justificada',
			  destino_id = excluded.destino_id,
			  observacao = excluded.observacao,
			  alterado_por = excluded.marcado_por,
			  alterado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		`, id, destServicoID, u.ID, grupoID, data, data); err != nil {
			log.Printf("sci motor-escalas conf %d: %v", id, err)
		}
	}

	// Inicialização das conferências setoriais individuais
	_, _ = a.st.db.Exec(`
		INSERT OR IGNORE INTO conferencia_setores (conferencia_id, setor_id, status)
		SELECT ?, s.id, 'nao_iniciada'
		FROM setores s
		WHERE (s.grupo_id = ? OR s.grupo_id IS NULL)
		  AND s.ativo = 1
		  AND s.id IN (SELECT DISTINCT setor_id FROM pessoas WHERE grupo_id = ? AND status = 'ativo' AND setor_id IS NOT NULL)
	`, id, grupoID, grupoID)

	// FuncaoIDs: filtro de antiguidade na conferencia (já validados nos handlers
	// ANTES da criação — aqui só semea; onda 09/10)
	if len(req.FuncaoIDs) > 0 {
		for _, fid := range req.FuncaoIDs {
			_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO conferencia_funcoes (conferencia_id, funcao_id) VALUES (?, ?)`, id, fid)
		}
	}

	return id, nil
}

// POST /api/conferencia/despachar
func (a *App) hConferenciaDespachar(w http.ResponseWriter, r *http.Request) {
	a.ensureTabelaDespachos()
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "o admin não inicia conferências — quem inicia é o gerente/operador de um grupo")
		return
	}
	if u.Papel != "gerente" && !a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u) {
		jsonErro(w, http.StatusForbidden, "a conferência é despachada pelo gerente ou pelo encarregado/auxiliar de pessoal")
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	var req confIniciarReq
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if len(req.Setores) == 0 {
		jsonErro(w, http.StatusBadRequest, "selecione ao menos um setor")
		return
	}

	setoresUnicos, err := a.validarSetoresDoGrupo(*u.GrupoID, req.Setores)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "setor inválido ou inexistente")
		return
	}

	// onda 09/10: funcao_ids inválido → 400 ANTES de criar a conferência
	// (correção 09/10: só vale função de antiguidade DO GRUPO da conferência)
	if len(req.FuncaoIDs) > 0 {
		if err := a.validarFuncoesAntiguidadeDoGrupo(*u.GrupoID, req.FuncaoIDs); err != nil {
			jsonErro(w, http.StatusBadRequest, "posto/graduação inválido no filtro (precisa ser tag de antiguidade do grupo)")
			return
		}
	}

	cid, err := a.criarConferenciaBase(u, req)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, sid := range setoresUnicos {
		_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO conferencia_despachos (conferencia_id, setor_id, criado_por) VALUES (?, ?, ?)`, cid, sid, u.ID)
	}

	a.st.Auditoria(&u.ID, "despachar", "conferencias", &cid, fmt.Sprintf("setores=%v", req.Setores), ipDe(r))

	resp := map[string]any{
		"ok":             true,
		"conferencia_id": cid,
		"despachados":    len(setoresUnicos),
	}
	// v1.5.4-D3 (R-7): mesmo aviso do iniciar — quem ficou fora do filtro de
	// antiguidade por não ter a tag em nenhuma das 3 fontes (recorte = setores
	// despachados).
	if len(req.FuncaoIDs) > 0 {
		resp["sem_tag"] = a.militaresSemTagAntiguidade(*u.GrupoID, cid, setoresUnicos)
	}
	jsonOK(w, resp)
}
