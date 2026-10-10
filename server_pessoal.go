// server_pessoal.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func validarFotoBase64(s string) (mime string, data []byte, err error) {
	const maxFotoBase64 = 512 * 1024 // 512 KB
	if len(s) > maxFotoBase64 {
		return "", nil, errors.New("foto excede o limite de 512 KB")
	}
	var prefix string
	if strings.HasPrefix(s, "data:image/png;base64,") {
		mime = "image/png"
		prefix = "data:image/png;base64,"
	} else if strings.HasPrefix(s, "data:image/jpeg;base64,") {
		mime = "image/jpeg"
		prefix = "data:image/jpeg;base64,"
	} else if strings.HasPrefix(s, "data:image/webp;base64,") {
		mime = "image/webp"
		prefix = "data:image/webp;base64,"
	} else {
		return "", nil, errors.New("formato de imagem inválido; prefixo deve ser data:image/(png|jpeg|webp);base64,")
	}

	payload := s[len(prefix):]
	b, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, fmt.Errorf("decodificação base64 inválida: %w", err)
	}

	switch mime {
	case "image/png":
		if len(b) < 8 || !bytes.Equal(b[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
			return "", nil, errors.New("conteúdo não corresponde a uma imagem PNG válida")
		}
	case "image/jpeg":
		if len(b) < 3 || !bytes.Equal(b[:3], []byte{0xFF, 0xD8, 0xFF}) {
			return "", nil, errors.New("conteúdo não corresponde a uma imagem JPEG válida")
		}
	case "image/webp":
		if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
			return "", nil, errors.New("conteúdo não corresponde a uma imagem WebP válida")
		}
	}

	return mime, b, nil
}

func (a *App) hLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login string `json:"login"`
		Senha string `json:"senha"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" || req.Senha == "" {
		jsonErro(w, http.StatusBadRequest, "login e senha obrigatórios")
		return
	}
	u, err := a.validarCredenciais(req.Login, req.Senha, ipDe(r))
	if err != nil {
		jsonErro(w, http.StatusUnauthorized, err.Error())
		return
	}
	tok, expira, err := a.st.CriarSessao(u.ID, ttlSessao)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao criar sessão")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieSessao, Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(ttlSessao.Seconds()),
	})

	sessaoU, err := a.st.UsuarioDaSessao(tok)
	if err != nil || sessaoU == nil {
		sessaoU = u
	}
	setorID := sessaoU.SetorID
	if setorID == nil && sessaoU.PessoaID != nil && *sessaoU.PessoaID > 0 {
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *sessaoU.PessoaID).Scan(&setorID)
	}
	setoresChefiados := []int64{}
	if sessaoU.Papel == "chefe_setor" {
		rows, e := a.st.db.Query(`SELECT setor_id FROM chefe_setores WHERE usuario_id = ? ORDER BY setor_id`, sessaoU.ID)
		if e == nil {
			for rows.Next() {
				var sid int64
				if rows.Scan(&sid) == nil {
					setoresChefiados = append(setoresChefiados, sid)
				}
			}
			rows.Close()
		}
	}
	jsonOK(w, map[string]any{
		"usuario":           sessaoU,
		"setor_id":          setorID,
		"setor_nome":        sessaoU.SetorNome,
		"setores_chefiados": setoresChefiados,
		"grupo_nome":        sessaoU.GrupoNome,
		"expira":            expira,
	})
}

func (a *App) hAuthSetup(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if !u.PrecisaSetup {
		jsonErro(w, http.StatusBadRequest, "usuário não precisa de setup")
		return
	}
	var req struct {
		NovoLogin string `json:"novo_login"`
		NovaSenha string `json:"nova_senha"`
	}
	if err := decodificar(r, &req); err != nil || req.NovoLogin == "" || req.NovaSenha == "" {
		jsonErro(w, http.StatusBadRequest, "identificação e nova senha são obrigatórios")
		return
	}
	if len(req.NovaSenha) < 8 {
		jsonErro(w, http.StatusBadRequest, "a nova senha deve ter no mínimo 8 caracteres")
		return
	}

	hash, err := hashSenha(req.NovaSenha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao processar senha")
		return
	}

	req.NovoLogin = strings.ToLower(strings.TrimSpace(req.NovoLogin))

	_, err = a.st.db.Exec(`UPDATE usuarios SET login = ?, senha_hash = ?, precisa_setup = 0 WHERE id = ?`, req.NovoLogin, hash, u.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			jsonErro(w, http.StatusConflict, "este número de identificação já está em uso por outro usuário")
			return
		}
		jsonErro(w, http.StatusInternalServerError, "falha ao salvar novos dados")
		return
	}
	a.st.Auditoria(&u.ID, "setup_concluido", "usuarios", &u.ID, "login atualizado", ipDe(r))
	jsonOK(w, map[string]string{"msg": "Cadastro atualizado com sucesso. Faça o login novamente."})
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	// Onda 05/10: setor_id resolvido (usuarios.setor_id → pessoas.setor_id) —
	// o front usa para o escopo de setor de chefe/operador na conferência.
	setorID := u.SetorID
	if setorID == nil && u.PessoaID != nil && *u.PessoaID > 0 {
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&setorID)
	}
	// Multi-chefia (ordem 06/10 item 14): setores COMANDADOS pelo usuário
	// (fonte da verdade do comando p/ front). Vazio quando não é chefe.
	setoresChefiados := []int64{}
	if u.Papel == "chefe_setor" {
		rows, e := a.st.db.Query(`SELECT setor_id FROM chefe_setores WHERE usuario_id = ? ORDER BY setor_id`, u.ID)
		if e == nil {
			for rows.Next() {
				var sid int64
				if rows.Scan(&sid) == nil {
					setoresChefiados = append(setoresChefiados, sid)
				}
			}
			rows.Close()
		}
	}
	// ordem 06/10 (P3): nome do GRUPO da sessão — o dropdown de contexto do
	// front usa como título da linha; PapeisDoUsuario já traz grupo_nome por
	// papel, aqui é o do papel ATIVO (última leitura, sem custo extra).
	jsonOK(w, map[string]any{"usuario": u, "setor_id": setorID, "setor_nome": u.SetorNome, "setores_chefiados": setoresChefiados, "grupo_nome": u.GrupoNome})
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieSessao); err == nil {
		a.st.EncerrarSessao(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSessao, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hTrocarSenha(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Atual string `json:"atual"`
		Nova  string `json:"nova"`
	}
	if err := decodificar(r, &req); err != nil || req.Atual == "" || req.Nova == "" {
		jsonErro(w, http.StatusBadRequest, "atual e nova obrigatórias")
		return
	}
	if len(req.Nova) < 8 {
		jsonErro(w, http.StatusBadRequest, "nova senha: mínimo 8 caracteres")
		return
	}
	u := usuarioDoCtx(r)
	var hash string
	if err := a.st.db.QueryRow(`SELECT senha_hash FROM usuarios WHERE id = ?`, u.ID).Scan(&hash); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !verificaSenha(req.Atual, hash) {
		jsonErro(w, http.StatusUnauthorized, "senha atual incorreta")
		return
	}
	novo, err := hashSenha(req.Nova)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, novo, u.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	// v1.5.4-E1 (R-16a): a troca de senha invalida TODAS as sessões da conta —
	// quem segura a senha antiga (outra sessão, cookie vazado) cai na hora.
	// DECISÃO: a sessão CORRENTE é preservada — a troca é pela própria conta
	// autenticada; derrubar quem acabou de trocar só destrói o fluxo em uso,
	// sem ganho de segurança (o segredo novo já está com o legítimo dono).
	// Redefinição por OUTREM (hUsuarioSenha) não preserva sessão nenhuma.
	// Documentado no ARQUITETURA §3.C.
	sessaoCorrente := ""
	if c, errC := r.Cookie(cookieSessao); errC == nil {
		sessaoCorrente = c.Value
	}
	a.invalidarSessoesDeSenha(u.ID, &sessaoCorrente)
	a.st.Auditoria(&u.ID, "trocar_senha", "usuarios", &u.ID, "sessões anteriores invalidadas (corrente preservada)", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hUsuarioSenha(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Senha string `json:"senha"`
	}
	if err = decodificar(r, &req); err != nil || len(req.Senha) < 8 {
		jsonErro(w, http.StatusBadRequest, "senha: mínimo 8 caracteres")
		return
	}
	solicitante := usuarioDoCtx(r)
	// ordem 06/10: senha de conta é poder credencial — SÓ admin e gerente
	// (antes: qualquer conta autenticada passava; buraco de escalação fechado).
	if solicitante.Papel != "admin" && solicitante.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "senha de conta é redefinida pelo gerente ou administrador")
		return
	}
	if solicitante.Papel == "gerente" {
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || solicitante.GrupoID == nil || alvoGrupo != *solicitante.GrupoID {
			jsonErro(w, http.StatusForbidden, "gerente só redefine senha de membros do próprio grupo")
			return
		}
	}
	var hashAtual string
	if err := a.st.db.QueryRow(`SELECT senha_hash FROM usuarios WHERE id = ?`, id).Scan(&hashAtual); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = hashAtual // leitura apenas para validar existência da conta
	novoHash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, novoHash, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	// v1.5.4-E1 (R-16a): redefinição por OUTREM (gerente/admin) derruba TODAS
	// as sessões da conta afetada — quem tinha a senha antiga perde acesso já
	// (o oposto da troca pela própria conta, que preserva a corrente).
	a.invalidarSessoesDeSenha(id, nil)
	a.st.Auditoria(&solicitante.ID, "redefinir_senha", "usuarios", &id, "sessões da conta invalidadas", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hUsuarioEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		NomeGuerra     *string `json:"nome_guerra"`
		NomeCompleto   *string `json:"nome_completo"`
		DataNascimento *string `json:"data_nascimento"`
		TipoSanguineo  *string `json:"tipo_sanguineo"`
		Telefone       *string `json:"telefone"`
		Email          *string `json:"email"`
		Endereco       *string `json:"endereco"`
		FotoBase64     *string `json:"foto_base64"`
		GrupoID        *int64  `json:"grupo_id"`
		FuncaoID       *int64  `json:"funcao_id"`
		SetorID        *int64  `json:"setor_id"`
		Ativo          *bool   `json:"ativo"`
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	solicitante := usuarioDoCtx(r)
	// ordem 06/10: encarregado/auxiliar de pessoal editam contas — mas SÓ as de
	// operador/chefe_setor do PRÓPRIO grupo (nunca gerente/admin). Fix ordem
	// 06/10: conta comum (sem papel) NÃO passa mais reto — era escalação.
	if solicitante.Papel != "admin" && solicitante.Papel != "gerente" {
		if !a.podeGestaoPessoal(solicitante) {
			jsonErro(w, http.StatusForbidden, "usuário sem permissão para editar outros usuários")
			return
		}
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || solicitante.GrupoID == nil || alvoGrupo != *solicitante.GrupoID {
			jsonErro(w, http.StatusForbidden, "encarregado/auxiliar só edita membros do próprio grupo")
			return
		}
		// ordem 06/10: função de pessoal NÃO move conta de grupo (só admin)
		if req.GrupoID != nil {
			jsonErro(w, http.StatusForbidden, "encarregado/auxiliar não altera o grupo da conta")
			return
		}
	}
	// v367: SEM trava por papel aqui — o bloco acima já devolveu 403 para
	// não-admin/gerente SEM designação (fail-closed); operador/chefe_setor
	// designados na cadeira enc_pessoal editam dentro das restrições de alvo
	// e grupo impostas acima (doutrina: designação manda, qualquer papel).
	if solicitante.Papel == "gerente" {
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || solicitante.GrupoID == nil || alvoGrupo != *solicitante.GrupoID {
			jsonErro(w, http.StatusForbidden, "gerente só edita membros do próprio grupo")
			return
		}
		// ordem 06/10 (paridade com o encarregado): gerente não move conta de
		// grupo — regra já implícita no escopo; agora explícita (só admin).
		if req.GrupoID != nil {
			jsonErro(w, http.StatusForbidden, "gerente não altera o grupo da conta")
			return
		}
	}

	if req.NomeGuerra != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET nome_guerra = ? WHERE id = ?`, strings.TrimSpace(*req.NomeGuerra), id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET nome_guerra = ? WHERE id = ?`, strings.TrimSpace(*req.NomeGuerra), *pID)
		}
	}
	if req.NomeCompleto != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET nome_completo = ? WHERE id = ?`, strings.TrimSpace(*req.NomeCompleto), id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET nome_completo = ? WHERE id = ?`, strings.TrimSpace(*req.NomeCompleto), *pID)
		}
	}
	if req.DataNascimento != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET data_nascimento = ? WHERE id = ?`, strings.TrimSpace(*req.DataNascimento), id)
	}
	if req.TipoSanguineo != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET tipo_sanguineo = ? WHERE id = ?`, strings.TrimSpace(*req.TipoSanguineo), id)
	}
	if req.Telefone != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET telefone = ? WHERE id = ?`, strings.TrimSpace(*req.Telefone), id)
	}
	if req.Email != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET email = ? WHERE id = ?`, strings.TrimSpace(*req.Email), id)
	}
	if req.Endereco != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET endereco = ? WHERE id = ?`, strings.TrimSpace(*req.Endereco), id)
	}
	if req.FotoBase64 != nil {
		foto := strings.TrimSpace(*req.FotoBase64)
		if foto != "" {
			if _, _, err := validarFotoBase64(foto); err != nil {
				jsonErro(w, http.StatusBadRequest, "foto_base64 inválida: "+err.Error())
				return
			}
		}
		_, _ = a.st.db.Exec(`UPDATE usuarios SET foto_base64 = ? WHERE id = ?`, foto, id)
	}
	if req.GrupoID != nil {
		gid := *req.GrupoID
		if gid <= 0 {
			_, _ = a.st.db.Exec(`UPDATE usuarios SET grupo_id = NULL WHERE id = ?`, id)
		} else {
			_, _ = a.st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, id)
			// Adiciona ou preserva papel no grupo
			_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'operador')`, id, gid)
		}
	}
	if req.FuncaoID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET funcao_id = ? WHERE id = ?`, req.FuncaoID, id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET funcao_id = ? WHERE id = ?`, req.FuncaoID, *pID)
		}
	}
	if req.SetorID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, req.SetorID, id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET setor_id = ? WHERE id = ?`, req.SetorID, *pID)
		}
	}
	if req.Ativo != nil {
		at := 0
		if *req.Ativo {
			at = 1
		}
		_, _ = a.st.db.Exec(`UPDATE usuarios SET ativo = ? WHERE id = ?`, at, id)
	}
	a.st.Auditoria(&solicitante.ID, "editar_usuario", "usuarios", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) tipoPadraoID() (int64, string, error) {
	var id int64
	var nome string
	err := a.st.db.QueryRow(
		`SELECT id, nome FROM conferencia_tipos WHERE nome = ? AND ativo = 1`, tipoConferenciaPadrao).
		Scan(&id, &nome)
	return id, nome, err
}

func (a *App) hPessoaFicha(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	q := `SELECT p.id, p.nome_guerra, COALESCE(p.nome_completo,''),
		COALESCE(s.nome,'INDEFINIDO'), COALESCE(fu.nome,'INDEFINIDO'), COALESCE(g.nome,'—'),
		p.status
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN grupos g ON g.id = p.grupo_id
		WHERE p.id = ?`
	if escopo > 0 {
		q += ` AND p.grupo_id = ?`
	}
	args := []any{id}
	if escopo > 0 {
		args = append(args, escopo)
	}
	var fid int64
	var ng, nc, setor, funcao, grupo, status string
	if e := a.st.db.QueryRow(q, args...).Scan(&fid, &ng, &nc, &setor, &funcao, &grupo, &status); e != nil {
		jsonErro(w, http.StatusNotFound, "pessoa não encontrada no seu escopo")
		return
	}
	jsonOK(w, map[string]any{"id": fid, "nome_guerra": ng, "nome_completo": nc,
		"setor": setor, "funcao": funcao, "grupo": grupo, "status": status})
}

func (a *App) hPessoaComentarios(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	// Validação de escopo: o usuário só vê comentários de quem ele enxerga na ficha
	if escopo > 0 {
		var check int
		if e := a.st.db.QueryRow(`SELECT 1 FROM pessoas WHERE id = ? AND grupo_id = ?`, id, escopo).Scan(&check); e != nil {
			jsonErro(w, http.StatusNotFound, "pessoa não encontrada no seu escopo")
			return
		}
	}
	q := `
		SELECT c.id, c.conferencia_id, COALESCE(t.nome,''), c.comentario,
		       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'), c.criado_em
		FROM comentarios c
		LEFT JOIN tags t ON t.id = c.tag_id
		LEFT JOIN usuarios u ON u.id = c.operador_id
		WHERE c.pessoa_id = ?
		ORDER BY c.criado_em DESC, c.ordem DESC`
	rows, err := a.st.db.Query(q, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var cid, confID int64
		var tag, comentario, op, data string
		if rows.Scan(&cid, &confID, &tag, &comentario, &op, &data) == nil {
			out = append(out, map[string]any{
				"id": cid, "conferencia_id": confID, "tag": tag,
				"comentario": comentario, "operador": op, "datahora": data,
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) hPessoaPDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	var f FichaPessoalPDF
	f.ID = id

	var gid *int64
	var setor, funcao, grupo string
	qP := `SELECT p.nome_guerra, p.nome_completo, p.status, p.grupo_id,
	              COALESCE(s.nome, 'Indefinido'), COALESCE(fu.nome, 'Indefinida'), COALESCE(g.nome, 'Geral'),
	              COALESCE(u.data_nascimento, ''), COALESCE(u.tipo_sanguineo, ''),
	              COALESCE(u.telefone, ''), COALESCE(u.email, ''), COALESCE(u.endereco, ''),
	              COALESCE(u.foto_base64, '')
	       FROM pessoas p
	       LEFT JOIN setores s ON s.id = p.setor_id
	       LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	       LEFT JOIN grupos g ON g.id = p.grupo_id
	       LEFT JOIN usuarios u ON u.pessoa_id = p.id
	       WHERE p.id = ?`

	err = a.st.db.QueryRow(qP, id).Scan(
		&f.NomeGuerra, &f.NomeCompleto, &f.Status, &gid,
		&setor, &funcao, &grupo,
		&f.DataNascimento, &f.TipoSanguineo,
		&f.Telefone, &f.Email, &f.Endereco, &f.FotoBase64,
	)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "pessoa não encontrada")
		return
	}

	if escopo > 0 && (gid == nil || *gid != escopo) {
		jsonErro(w, http.StatusForbidden, "pessoa fora do seu escopo")
		return
	}

	f.Setor = setor
	f.Funcao = funcao
	f.Grupo = grupo

	_ = a.st.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(situacao = 'presente'), 0),
		       COALESCE(SUM(situacao = 'atraso'), 0),
		       COALESCE(SUM(situacao = 'falta'), 0),
		       COALESCE(SUM(situacao = 'justificada'), 0)
		FROM presencas WHERE pessoa_id = ?`, id).Scan(
		&f.TotalConfs, &f.Presencas, &f.Atrasos, &f.Faltas, &f.Justificadas,
	)
	if f.TotalConfs > 0 {
		f.PctPresenca = 100.0 * float64(f.Presencas+f.Atrasos) / float64(f.TotalConfs)
	}

	rowsC, errC := a.st.db.Query(`
		SELECT mc.id, mi.nome, mi.codigo_patrimonio, mc.data_saida, COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—')
		FROM material_cautelas mc
		JOIN material_itens mi ON mi.id = mc.item_id
		JOIN usuarios u ON u.id = mc.responsavel_entrega_id
		WHERE mc.pessoa_id = ? AND mc.status = 'ativa'
		ORDER BY mc.id DESC LIMIT 10`, id)
	if errC == nil {
		for rowsC.Next() {
			var cid int64
			var itemNome, codPat, dtSaida, resp string
			if rowsC.Scan(&cid, &itemNome, &codPat, &dtSaida, &resp) == nil {
				f.CautelasAtivas = append(f.CautelasAtivas, map[string]any{
					"id":                  cid,
					"item_nome":           itemNome,
					"codigo_patrimonio":   codPat,
					"data_saida":          dtSaida,
					"responsavel_entrega": resp,
				})
			}
		}
		rowsC.Close()
	}

	rowsE, errE := a.st.db.Query(`
		SELECT et.data_inicio, et.data_fim, etp.nome, COALESCE(ep.funcao_escala, '')
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		WHERE ep.pessoa_id = ?
		ORDER BY et.data_inicio DESC LIMIT 8`, id)
	if errE == nil {
		for rowsE.Next() {
			var dtIni, dtFim, tNome, fEsc string
			if rowsE.Scan(&dtIni, &dtFim, &tNome, &fEsc) == nil {
				f.Escalas = append(f.Escalas, map[string]any{
					"data_inicio":   dtIni,
					"data_fim":      dtFim,
					"tipo_nome":     tNome,
					"funcao_escala": fEsc,
				})
			}
		}
		rowsE.Close()
	}

	pdf, err := a.gerarFichaPessoalPDF(f, u.Login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar ficha em PDF: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar", "ficha_pessoal", &id, f.NomeGuerra, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=ficha_%s_%d.pdf", f.NomeGuerra, id))
	_, _ = w.Write(pdf)
}

func (a *App) hPessoasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	pessoas := a.pessoasTodas(escopo)
	if len(pessoas) == 0 {
		jsonOK(w, map[string]any{"pessoas": pessoas})
		return
	}
	type ultModInfo struct {
		em   string
		quem string
	}
	ultMods := make(map[int64]ultModInfo)
	q := `SELECT ad.registro_id, ad.em, COALESCE(NULLIF(u.nome_guerra, ''), COALESCE(u.login, ''))
	      FROM auditoria ad
	      JOIN (
	          SELECT registro_id, MAX(em) AS max_em, MAX(id) AS max_id
	          FROM auditoria
	          WHERE entidade = 'pessoas' AND registro_id IS NOT NULL
	          GROUP BY registro_id
	      ) ult ON ad.id = ult.max_id
	      LEFT JOIN usuarios u ON u.id = ad.usuario_id
	      WHERE ad.entidade = 'pessoas'`
	if rows, err := a.st.db.Query(q); err == nil {
		defer rows.Close()
		for rows.Next() {
			var regID int64
			var em, quem string
			if err := rows.Scan(&regID, &em, &quem); err == nil {
				ultMods[regID] = ultModInfo{em: em, quem: quem}
			}
		}
	}
	for _, p := range pessoas {
		var idVal int64
		switch v := p["id"].(type) {
		case int64:
			idVal = v
		case int:
			idVal = int64(v)
		case float64:
			idVal = int64(v)
		}
		if idVal > 0 {
			if m, achou := ultMods[idVal]; achou {
				p["ultima_mod_por"] = m.quem
				p["ultima_mod_em"] = m.em
				continue
			}
		}
		p["ultima_mod_por"] = nil
		p["ultima_mod_em"] = nil
	}
	jsonOK(w, map[string]any{"pessoas": pessoas})
}

func (a *App) hPessoaExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var grupoID *int64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&grupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "pessoa inexistente")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if u.Papel != "admin" {
		// ordem 06/10: gerente/encarregado/auxiliar excluem SÓ do próprio grupo
		if grupoID == nil || *grupoID != esc {
			jsonErro(w, http.StatusForbidden, "pessoa de outro grupo")
			return
		}
	}
	var lanc int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE pessoa_id = ?`, id).Scan(&lanc)
	if lanc > 0 {
		if _, err = a.st.db.Exec(`UPDATE pessoas SET status='inativo',
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "desativar", "pessoas", &id, "histórico preservado (lançamentos)", ipDe(r))
		jsonOK(w, map[string]bool{"desativado": true})
		return
	}
	if _, err = a.st.db.Exec(`DELETE FROM pessoas WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir", "pessoas", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hPessoasAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NomeGuerra   string `json:"nome_guerra"`
		NomeCompleto string `json:"nome_completo"`
		SetorID      *int64 `json:"setor_id"`
		FuncaoID     *int64 `json:"funcao_id"`
		Status       string `json:"status"`
		GrupoID      *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.NomeGuerra == "" || req.NomeCompleto == "" {
		jsonErro(w, http.StatusBadRequest, "nome de guerra e nome completo obrigatórios")
		return
	}
	if req.Status == "" {
		req.Status = "ativo"
	}
	// v9.4 (ordem Tenente): status de pessoal só ATIVO ou INATIVO
	if req.Status != "ativo" && req.Status != "inativo" {
		jsonErro(w, http.StatusBadRequest, "status só pode ser ativo ou inativo")
		return
	}
	u := usuarioDoCtx(r)
	// REGRA (28/09, estendida ordem 06/10): admin/gerente/encarregado/auxiliar
	// de pessoal cadastram; gerente/encarregado/auxiliar SEMPRE no próprio grupo
	var grupoID *int64
	switch {
	case u.Papel == "admin":
		grupoID = req.GrupoID // admin escolhe o grupo (ou NULL = sem grupo)
	case u.Papel == "gerente":
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		grupoID = u.GrupoID
	case a.podeGestaoPessoal(u): // encarregado OU auxiliar de pessoal
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		grupoID = u.GrupoID
	default:
		jsonErro(w, http.StatusForbidden, "somente admin, gerente e encarregado/auxiliar de pessoal cadastram pessoal")
		return
	}
	// ordem 04/10 (Fase G): função informada deve pertencer ao catálogo do grupo
	// de vinculação (global = grupo NULL serve a todos).
	if req.FuncaoID != nil && *req.FuncaoID <= 0 {
		req.FuncaoID = nil
	}
	if req.FuncaoID != nil && !a.funcaoValidaParaPessoa(*req.FuncaoID, grupoID) {
		jsonErro(w, http.StatusBadRequest, "função não pertence ao grupo da pessoa")
		return
	}
	// v9.7: safeguard anti-duplicata — mesma pessoa (guerra+completo, case-insensitive,
	// trim) não nasce duas vezes no mesmo grupo (índice único do schema v7 é a 2ª barreira)
	var dup int
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas
		WHERE grupo_id IS ? AND LOWER(TRIM(nome_guerra)) = LOWER(TRIM(?)) AND LOWER(TRIM(nome_completo)) = LOWER(TRIM(?))`,
		grupoID, strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeCompleto)).Scan(&dup); err == nil && dup > 0 {
		jsonErro(w, http.StatusConflict, "pessoa já cadastrada neste grupo (mesmo nome de guerra e nome completo)")
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, funcao_id, grupo_id, status) VALUES (?,?,?,?,?,?)`,
		req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, grupoID, req.Status)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "criar", "pessoas", &id, req.NomeGuerra, ipDe(r))
	jsonOK(w, map[string]any{"id": id})
}

func (a *App) hPessoasEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		NomeGuerra   string `json:"nome_guerra"`
		NomeCompleto string `json:"nome_completo"`
		Status       string `json:"status"`
		SetorID      *int64 `json:"setor_id"`
		FuncaoID     *int64 `json:"funcao_id"`
		GrupoID      *int64 `json:"grupo_id"` // admin pode mover pessoa de grupo
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Status == "" {
		req.Status = "ativo"
	}
	// v9.4 (ordem Tenente): status de pessoal só ATIVO ou INATIVO
	if req.Status != "ativo" && req.Status != "inativo" {
		jsonErro(w, http.StatusBadRequest, "status só pode ser ativo ou inativo")
		return
	}
	u := usuarioDoCtx(r)
	// ordem 06/10: gerente/encarregado/auxiliar só editam pessoal DO PRÓPRIO
	// grupo (admin edita tudo) — escopo explícito também para quem entra pelo
	// guardaGestaoPessoal (conta sem papel do sistema).
	var gidEscopo *int64 // grupo da pessoa (nil = sem grupo)
	if u.Papel != "admin" {
		esc, err := a.exigeEscopo(u)
		if err != nil {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		var gid *int64
		if err = a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&gid); err != nil {
			jsonErro(w, http.StatusNotFound, "pessoa inexistente")
			return
		}
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "pessoa de outro grupo")
			return
		}
		gidEscopo = gid
	} else {
		_ = a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&gidEscopo)
	}
	// função atual da pessoa — base da regra de nomeação (item 15)
	var funcaoAntiga *int64
	_ = a.st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, id).Scan(&funcaoAntiga)
	// ordem 06/10 (item 15): QUEM NOMEIA função — gerente/admin qualquer;
	// encarregado/auxiliar só definir/desfazer a "auxiliar de pessoal" do
	// próprio grupo (reenvio da mesma função passa: edição de campos).
	if req.FuncaoID != nil && *req.FuncaoID <= 0 {
		req.FuncaoID = nil
	}
	if permitido, motivo := a.atribuiFuncaoPessoal(u, gidEscopo, req.FuncaoID, funcaoAntiga); !permitido {
		jsonErro(w, http.StatusForbidden, motivo)
		return
	}
	// ordem 04/10 (Fase G): função informada deve pertencer ao catálogo do grupo
	// FINAL da pessoa (admin pode mover o grupo; gerente não).
	if req.FuncaoID != nil {
		grupoAlvo := gidEscopo
		if u.Papel == "admin" && req.GrupoID != nil {
			grupoAlvo = req.GrupoID
		}
		if !a.funcaoValidaParaPessoa(*req.FuncaoID, grupoAlvo) {
			jsonErro(w, http.StatusBadRequest, "função não pertence ao grupo da pessoa")
			return
		}
	}
	// admin pode mover a pessoa de grupo na edição; gerente nunca altera grupo
	if u.Papel == "admin" {
		if _, err = a.st.db.Exec(`UPDATE pessoas SET nome_guerra=?, nome_completo=?, setor_id=?, funcao_id=?, status=?, grupo_id=?,
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`,
			req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, req.Status, req.GrupoID, id); err != nil {
			jsonErro(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if _, err = a.st.db.Exec(
			`UPDATE pessoas SET nome_guerra=?, nome_completo=?, setor_id=?, funcao_id=?, status=?,
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`,
			req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, req.Status, id); err != nil {
			jsonErro(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.FuncaoID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET funcao_id = ? WHERE pessoa_id = ?`, req.FuncaoID, id)
	}
	if req.SetorID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE pessoa_id = ?`, req.SetorID, id)
	}
	a.st.Auditoria(&u.ID, "alterar", "pessoas", &id, req.NomeGuerra, ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// funcaoValidaParaPessoa: função informada deve pertencer ao catálogo do grupo
// da pessoa (global = grupo NULL serve a todos). Reuso da regra de hUsuariosAdd.
func (a *App) funcaoValidaParaPessoa(funcaoID int64, gidPessoa *int64) bool {
	var fGrupo *int64
	_ = a.st.db.QueryRow(`SELECT grupo_id FROM funcoes WHERE id = ?`, funcaoID).Scan(&fGrupo)
	// função global (grupo NULL) serve a todos
	if fGrupo == nil {
		return true
	}
	// função de grupo específico: pessoa precisa estar no mesmo grupo
	if gidPessoa == nil || *fGrupo != *gidPessoa {
		return false
	}
	return true
}

func (a *App) hUsuariosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	// ordem 04/10 (Grupos): gerente vê o PRÓPRIO grupo + GRUPOS SUBORDINADOS
	// (admin continua vendo todos). Operador/chefe seguem só no próprio grupo.
	var subordinados []int64
	if u.Papel == "gerente" && escopo > 0 {
		subordinados = a.gruposSubordinadosAtivos(escopo)
	}
	rows, err := a.st.db.Query(
		`SELECT id, login, papel, pessoa_id, COALESCE(grupo_id,0), ativo, criado_em, senhas,
		        COALESCE(nome_guerra,''), COALESCE(nome_completo,''),
		        COALESCE(data_nascimento,''), COALESCE(tipo_sanguineo,''),
		        COALESCE(telefone,''), COALESCE(email,''),
		        COALESCE(endereco,''), COALESCE(foto_base64,''), COALESCE(setor_id,0)
		 FROM usuarios ORDER BY id`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, grupoID, setorID int64
		var login, papel, criado, senhas, nomeGuerra, nomeCompleto string
		var dataNasc, tipoSang, tel, email, endereco, foto string
		var pessoaID *int64
		var ativo int
		if rows.Scan(&id, &login, &papel, &pessoaID, &grupoID, &ativo, &criado, &senhas, &nomeGuerra, &nomeCompleto,
			&dataNasc, &tipoSang, &tel, &email, &endereco, &foto, &setorID) == nil {
			// escopo: admin vê tudo; gerente vê o PRÓPRIO grupo + subordinados
			// (ordem 04/10); operador/chefe só contas do próprio grupo, SEM
			// dados de sessão/senha (v9.4).
			mostrar := escopo <= 0 || int64(escopo) == grupoID
			if !mostrar && len(subordinados) > 0 {
				for _, sid := range subordinados {
					if sid == grupoID {
						mostrar = true
						break
					}
				}
			}
			if !mostrar {
				continue
			}
			item := map[string]any{
				"id": id, "login": login, "papel": papel, "pessoa_id": pessoaID,
				"grupo_id": grupoID, "setor_id": setorID, "ativo": ativo == 1, "criado_em": criado,
				"nome_guerra": nomeGuerra, "nome_completo": nomeCompleto,
				"data_nascimento": dataNasc, "tipo_sanguineo": tipoSang,
				"telefone": tel, "email": email, "endereco": endereco, "foto_base64": foto,
			}
			// ordem 06/10: hash de senhas só para papeis do sistema (operador e
			// contas sem papel — encarregado/auxiliar/comum — não recebem)
			if u.Papel == "admin" || u.Papel == "gerente" || u.Papel == "chefe_setor" {
				item["senhas"] = json.RawMessage(senhas)
			}
			out = append(out, item)
		}
	}
	rows.Close()

	for i := range out {
		id := out[i]["id"].(int64)
		papeis, _ := a.st.PapeisDoUsuario(id)
		out[i]["papeis"] = papeis
	}
	jsonOK(w, out)
}

func (a *App) hPerfilGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	_ = a.st.db.QueryRow(`SELECT COALESCE(nome_guerra,''), COALESCE(nome_completo,''),
		COALESCE(data_nascimento,''), COALESCE(tipo_sanguineo,''), COALESCE(telefone,''),
		COALESCE(email,''), COALESCE(endereco,''), COALESCE(foto_base64,'')
		FROM usuarios WHERE id = ?`, u.ID).Scan(
		&u.NomeGuerra, &u.NomeCompleto,
		&u.DataNascimento, &u.TipoSanguineo, &u.Telefone,
		&u.Email, &u.Endereco, &u.FotoBase64)

	var grupo string
	if u.GrupoID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, *u.GrupoID).Scan(&grupo)
	}
	var setor, funcao string
	if u.SetorID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM setores WHERE id = ?`, *u.SetorID).Scan(&setor)
	}
	if u.FuncaoID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM funcoes WHERE id = ?`, *u.FuncaoID).Scan(&funcao)
	}
	jsonOK(w, map[string]any{"usuario": u, "grupo": grupo, "setor": setor, "funcao": funcao})
}

func (a *App) hPerfilSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NomeGuerra     string `json:"nome_guerra"`
		NomeCompleto   string `json:"nome_completo"`
		DataNascimento string `json:"data_nascimento"`
		TipoSanguineo  string `json:"tipo_sanguineo"`
		Telefone       string `json:"telefone"`
		Email          string `json:"email"`
		Endereco       string `json:"endereco"`
		FotoBase64     string `json:"foto_base64"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.NomeGuerra) == "" || strings.TrimSpace(req.NomeCompleto) == "" {
		jsonErro(w, http.StatusBadRequest, "nome de guerra e nome completo obrigatórios")
		return
	}
	u := usuarioDoCtx(r)
	foto := strings.TrimSpace(req.FotoBase64)
	if foto != "" {
		if _, _, err := validarFotoBase64(foto); err != nil {
			jsonErro(w, http.StatusBadRequest, "foto_base64 inválida: "+err.Error())
			return
		}
	}
	if _, err := a.st.db.Exec(`UPDATE usuarios SET
		nome_guerra = ?, nome_completo = ?,
		data_nascimento = ?, tipo_sanguineo = ?,
		telefone = ?, email = ?, endereco = ?,
		foto_base64 = ?
		WHERE id = ?`,
		strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeCompleto),
		strings.TrimSpace(req.DataNascimento), strings.TrimSpace(req.TipoSanguineo),
		strings.TrimSpace(req.Telefone), strings.TrimSpace(req.Email), strings.TrimSpace(req.Endereco),
		foto, u.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if u.PessoaID != nil {
		_, _ = a.st.db.Exec(`UPDATE pessoas SET nome_guerra = ?, nome_completo = ? WHERE id = ?`,
			strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeCompleto), *u.PessoaID)
	}
	a.st.Auditoria(&u.ID, "editar", "usuarios", &u.ID, "perfil próprio", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hUsuarioFotoGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var foto string
	err = a.st.db.QueryRow(`SELECT COALESCE(foto_base64,'') FROM usuarios WHERE id = ?`, id).Scan(&foto)
	if err != nil || foto == "" {
		http.NotFound(w, r)
		return
	}
	partes := strings.Split(foto, ",")
	b64 := foto
	mime := "image/jpeg"
	if len(partes) == 2 {
		b64 = partes[1]
		if strings.Contains(partes[0], "image/png") {
			mime = "image/png"
		} else if strings.Contains(partes[0], "image/webp") {
			mime = "image/webp"
		}
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (a *App) hMoverConta(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		GrupoID int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.GrupoID == 0 {
		jsonErro(w, http.StatusBadRequest, "grupo_id obrigatório")
		return
	}
	u := usuarioDoCtx(r)
	var alvoPapel string
	var origem int64
	if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &origem); err != nil {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	if alvoPapel == "admin" {
		jsonErro(w, http.StatusForbidden, "conta admin não é movida")
		return
	}
	dentroDaArvore := func(g int64) bool {
		return u.GrupoID != nil && (g == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), g))
	}
	// Fix ordem 06/10: só ADMIN e GERENTE movem contas — antes, operador/comum
	// passavam reto aqui e moviam qualquer conta para qualquer grupo.
	if u.Papel != "admin" {
		if u.Papel != "gerente" || u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "somente admin e gerente movem contas")
			return
		}
		if !dentroDaArvore(origem) || !dentroDaArvore(req.GrupoID) {
			jsonErro(w, http.StatusForbidden, "mover só dentro da sua hierarquia (próprio grupo + subordinados)")
			return
		}
	}
	// destino precisa existir
	var existe int
	if a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE id = ?`, req.GrupoID).Scan(&existe) != nil || existe == 0 {
		jsonErro(w, http.StatusNotFound, "grupo de destino inexistente")
		return
	}
	if origem == req.GrupoID {
		jsonOK(w, map[string]bool{"ok": true}) // nada a fazer
		return
	}
	// gerente que se move: se for o único do grupo de origem, rebaixa a operador no destino
	vaiRebaixar := false
	if alvoPapel == "gerente" {
		var outros int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE grupo_id = ? AND papel = 'gerente' AND id <> ? AND ativo = 1`,
			origem, id).Scan(&outros)
		if outros == 0 {
			vaiRebaixar = true
		}
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if vaiRebaixar {
		if _, err = tx.Exec(`UPDATE usuarios SET grupo_id = ?, papel = 'operador' WHERE id = ?`, req.GrupoID, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if _, err = tx.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, req.GrupoID, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "mover", "usuarios", &id,
		fmt.Sprintf("origem=%d destino=%d rebaixado=%v", origem, req.GrupoID, vaiRebaixar), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "rebaixado": vaiRebaixar})
}

func (a *App) hUsuariosAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Senha    string `json:"senha"`
		Papel    string `json:"papel"`
		PessoaID *int64 `json:"pessoa_id"`
		GrupoID  *int64 `json:"grupo_id"`
		FuncaoID *int64 `json:"funcao_id"`
		SetorID  *int64 `json:"setor_id"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" {
		jsonErro(w, http.StatusBadRequest, "identificação obrigatória")
		return
	}
	precisaSetup := 1
	if req.Senha == "" {
		req.Senha = "sci"
	} else if req.Senha != "sci" {
		precisaSetup = 0
	}
	u := usuarioDoCtx(r)
	papel := strings.ToLower(strings.TrimSpace(req.Papel))
	switch papel {
	case "admin", "gerente", "operador", "chefe_setor":
	default:
		jsonErro(w, http.StatusBadRequest, "papel inválido (admin | gerente | operador | chefe_setor)")
		return
	}
	// Hierarquia de criação (ordem Diretor 04/10 — "Criação de usuários" e
	// "Sistema de operadores"):
	// - ADMIN cria qualquer papel; usuários criados pelo admin NÃO têm grupo
	//   (exceto gerente, que exige unidade).
	// - GERENTE cria CHEFE_SETOR no próprio grupo; gerente NÃO cria operador —
	//   ele seleciona os chefes de setor do grupo, e os chefes selecionam os
	//   operadores dentre os usuários do seu setor.
	// - CHEFE_SETOR promove a OPERADOR somente usuário DO SEU SETOR.
	// - v367: designado na cadeira 'enc_pessoal' cria contas MESMO COM papel de
	//   sistema (operador/chefe_setor) — o cargo manda; sem designação segue sem
	//   criar. Gerente/admin inalterados.
	// A liberação do menu de gestão no front é atrelada à FUNÇÃO de encarregado
	// de pessoal (catálogo do grupo); a defesa dura aqui é pela DESIGNAÇÃO.
	// ordem 06/10: ENCARREGADO/AUXILIAR DE PESSOAL criam contas, mas só
	// operador/chefe_setor do PRÓPRIO grupo — criar gerente/admin é poder de
	// gerente/admin.
	if u.Papel != "admin" && u.Papel != "gerente" && a.podeGestaoPessoal(u) {
		if papel != "operador" && papel != "chefe_setor" {
			jsonErro(w, http.StatusForbidden, "encarregado/auxiliar de pessoal só designa operador ou chefe de setor do próprio grupo")
			return
		}
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "encarregado/auxiliar sem grupo definido")
			return
		}
		req.GrupoID = u.GrupoID // força o próprio grupo, ignore o que vier no corpo
	}
	if u.Papel != "admin" {
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo definido")
			return
		}
		switch {
		case u.Papel == "gerente":
			if papel != "chefe_setor" {
				jsonErro(w, http.StatusForbidden, "gerente seleciona chefes de setor; operadores são designados pelos chefes")
				return
			}
			req.GrupoID = u.GrupoID // força o próprio grupo, ignore o que vier no corpo
		// v367: o ramo do DESIGNADO vem ANTES dos papéis de sistema — operador/
		// chefe_setor com cadeira 'enc_pessoal' criam operador/chefe_setor do
		// próprio grupo (grupo já forçado acima); sem designação caem nos ramos
		// de papel abaixo.
		case a.podeGestaoPessoal(u):
		case u.Papel == "chefe_setor":
			if papel != "operador" {
				jsonErro(w, http.StatusForbidden, "chefe de setor só designa operadores")
				return
			}
			req.GrupoID = u.GrupoID
		default:
			jsonErro(w, http.StatusForbidden, "sem permissão para criar contas")
			return
		}
	}
	if papel == "admin" {
		req.GrupoID = nil // admin é global
	}
	// ordem 04/10: DEFAULT dos usuários criados pelo admin é SEM grupo — o
	// modal do admin não envia grupo. Se o corpo trouxer grupo explícito
	// (provisionamento/seed), o admin pode vinculá-lo deliberadamente.
	// CHEFE_SETOR designa OPERADOR: a conta herda o SETOR do chefe (é o
	// vínculo que define o escopo de atuação); gerente/admin podem informar
	// funcao_id/setor_id explícitos (catálogos do grupo) na criação.
	if u.Papel == "chefe_setor" && papel == "operador" && req.SetorID == nil {
		var setorChefe *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, u.ID).Scan(&setorChefe)
		req.SetorID = setorChefe
	}
	if req.SetorID != nil && *req.SetorID <= 0 {
		req.SetorID = nil
	}
	if req.FuncaoID != nil && *req.FuncaoID <= 0 {
		req.FuncaoID = nil
	}
	// ordem 04/10 (Fase G): função informada deve pertencer ao catálogo do
	// grupo de vinculação (global = grupo NULL serve a todos). Gerente não
	// atravessa grupos atribuindo função alheia.
	if req.FuncaoID != nil {
		var fGrupo *int64
		_ = a.st.db.QueryRow(`SELECT grupo_id FROM funcoes WHERE id = ?`, *req.FuncaoID).Scan(&fGrupo)
		alvo := req.GrupoID
		if fGrupo != nil && (alvo == nil || *fGrupo != *alvo) {
			jsonErro(w, http.StatusForbidden, "função não pertence ao catálogo do grupo")
			return
		}
	}
	if papel == "gerente" && (req.GrupoID == nil || *req.GrupoID <= 0) {
		jsonErro(w, http.StatusBadRequest, "gerente deve obrigatoriamente estar vinculado a uma unidade")
		return
	}
	hash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO usuarios (login, senha_hash, papel, pessoa_id, grupo_id, funcao_id, setor_id, precisa_setup) VALUES (?,?,?,?,?,?,?,?)`,
		strings.ToLower(strings.TrimSpace(req.Login)), hash, papel, req.PessoaID, req.GrupoID, req.FuncaoID, req.SetorID, precisaSetup)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não criado (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	var funcaoID *int64
	if req.FuncaoID != nil {
		funcaoID = req.FuncaoID // função informada no create (ordem 04/10)
	} else if req.PessoaID != nil {
		_ = a.st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, *req.PessoaID).Scan(&funcaoID)
	}
	_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id) VALUES (?,?,?,?)`,
		id, req.GrupoID, papel, funcaoID)

	a.st.Auditoria(&u.ID, "criar", "usuarios", &id, req.Login+" ("+papel+")", ipDe(r))
	jsonOK(w, map[string]any{"id": id})
}

func (a *App) hUsuarioExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	eu := usuarioDoCtx(r)
	if id == eu.ID {
		jsonErro(w, http.StatusBadRequest, "não é possível excluir a própria conta")
		return
	}
	var papel string
	if err := a.st.db.QueryRow(`SELECT papel FROM usuarios WHERE id = ?`, id).Scan(&papel); err != nil {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	if papel == "admin" {
		jsonErro(w, http.StatusForbidden, "conta admin não é excluída")
		return
	}
	// v9.4: GERENTE exclui OPERADOR/CHEFE_SETOR do próprio grupo; operador/chefe não exclui ninguém.
	if eu.Papel == "operador" || eu.Papel == "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "somente admin e gerente excluem contas")
		return
	}
	if eu.Papel == "gerente" {
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || eu.GrupoID == nil || alvoGrupo != *eu.GrupoID {
			jsonErro(w, http.StatusForbidden, "gerente só exclui membros do próprio grupo")
			return
		}
	}
	if papel == "gerente" {
		var gid int64
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&gid); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if gid == 0 {
			jsonErro(w, http.StatusConflict, "gerente sem grupo — mova a conta antes de excluir")
			return
		}

	}
	var marcou, criou, comentou int
	_ = a.st.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM presencas WHERE marcado_por = ?),
		(SELECT COUNT(*) FROM conferencias WHERE criado_por = ?),
		(SELECT COUNT(*) FROM comentarios WHERE operador_id = ?)`, id, id, id).Scan(&marcou, &criou, &comentou)
	if marcou+criou+comentou > 0 {
		if _, err := a.st.db.Exec(`UPDATE usuarios SET ativo = 0 WHERE id = ?`, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&eu.ID, "desativar", "usuarios", &id, "histórico preservado (FK)", ipDe(r))
		jsonOK(w, map[string]bool{"desativado": true})
		return
	}
	if _, err := a.st.db.Exec(`DELETE FROM usuarios WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&eu.ID, "excluir", "usuarios", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hPessoaQRCode(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID de militar inválido")
		return
	}
	var nomeGuerra string
	var gid *int64
	err = a.st.db.QueryRow(`SELECT nome_guerra, grupo_id FROM pessoas WHERE id = ?`, id).Scan(&nomeGuerra, &gid)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Militar não encontrado")
		return
	}
	if esc > 0 && (gid == nil || *gid != esc) {
		jsonErro(w, http.StatusForbidden, "Acesso restrito ao grupo")
		return
	}
	payload := fmt.Sprintf("sci://p:%d:%s", id, nomeGuerra)
	qr, err := GerarQRCode(payload)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Erro ao gerar QR Code: "+err.Error())
		return
	}
	if r.URL.Query().Get("format") == "svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
		_ = qr.RenderSVG(w, 256)
		return
	}
	pngData, err := qr.RenderPNG(8, 4)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Erro ao renderizar PNG: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(pngData)))
	_, _ = w.Write(pngData)
}

func (a *App) hConscienciaResumo(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	// Buscar dados do próprio grupo e subordinados imediatos/recursivos
	var grupoPrincipalNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoPrincipalNome)
	} else {
		grupoPrincipalNome = "Comando Geral / Todas as Unidades"
	}

	hoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	ontem := time.Now().In(a.horaLocal).AddDate(0, 0, -1).Format("2006-01-02")

	// Determinar grupos no escopo
	var gruposIDs []int64
	if escopo > 0 {
		gruposIDs = append([]int64{escopo}, a.gruposSubordinadosAtivos(escopo)...)
	} else {
		rows, _ := a.st.db.Query(`SELECT id FROM grupos ORDER BY nome`)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var gid int64
				if rows.Scan(&gid) == nil {
					gruposIDs = append(gruposIDs, gid)
				}
			}
		}
	}

	// Métricas agregadas
	var totalEfetivo, totalPresentes, totalEscaladosHoje, totalMateriais, totalCautelasAbertas, totalConferenciasFechadas int

	type SubordinadoResumo struct {
		ID                   int64  `json:"id"`
		Nome                 string `json:"nome"`
		Efetivo              int    `json:"efetivo"`
		PresentesHoje        int    `json:"presentes_hoje"`
		FaltasHoje           int    `json:"faltas_hoje"`
		StatusConferencia    string `json:"status_conferencia"` // 'fechada', 'aberta', 'pendente'
		ConferenciaID        *int64 `json:"conferencia_id,omitempty"`
		MateriaisAcautelados int    `json:"materiais_acautelados"`
	}

	var listaSub []SubordinadoResumo

	for _, gid := range gruposIDs {
		var gNome string
		var ef int
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, gid).Scan(&gNome)
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE grupo_id = ? AND status = 'ativo'`, gid).Scan(&ef)
		totalEfetivo += ef

		// Conferência de hoje do grupo
		var confID int64
		var confStatus string
		var presentes, faltas int
		errConf := a.st.db.QueryRow(`
			SELECT id, status FROM conferencias
			WHERE grupo_id = ? AND data = ?
			ORDER BY id DESC LIMIT 1`, gid, hoje).Scan(&confID, &confStatus)

		statusConf := "pendente"
		var pConfID *int64
		if errConf == nil {
			statusConf = confStatus
			pConfID = &confID
			if confStatus == "fechada" {
				totalConferenciasFechadas++
			}
			_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND situacao = 'presente'`, confID).Scan(&presentes)
			_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND situacao IN ('falta', 'justificada')`, confID).Scan(&faltas)
		}
		totalPresentes += presentes

		// Materiais acautelados no grupo
		var acautelados int
		_ = a.st.db.QueryRow(`
			SELECT COUNT(*) FROM material_cautelas mc
			JOIN material_itens mi ON mi.id = mc.item_id
			WHERE mi.grupo_id = ? AND mc.status = 'ativa'`, gid).Scan(&acautelados)
		totalCautelasAbertas += acautelados

		var totalMatGrupo int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM material_itens WHERE grupo_id = ?`, gid).Scan(&totalMatGrupo)
		totalMateriais += totalMatGrupo

		listaSub = append(listaSub, SubordinadoResumo{
			ID:                   gid,
			Nome:                 gNome,
			Efetivo:              ef,
			PresentesHoje:        presentes,
			FaltasHoje:           faltas,
			StatusConferencia:    statusConf,
			ConferenciaID:        pConfID,
			MateriaisAcautelados: acautelados,
		})
	}

	// Escalados de hoje em todas as unidades do escopo
	if len(gruposIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(gruposIDs)), ",")
		args := make([]any, len(gruposIDs)+1)
		for i, g := range gruposIDs {
			args[i] = g
		}
		args[len(gruposIDs)] = hoje
		_ = a.st.db.QueryRow(`
			SELECT COUNT(DISTINCT ep.pessoa_id)
			FROM escala_pessoas ep
			JOIN escala_turnos et ON et.id = ep.turno_id
			WHERE et.grupo_id IN (`+ph+`) AND substr(et.data_inicio, 1, 10) <= ? AND substr(et.data_fim, 1, 10) >= ?`,
			append(args, hoje)...).Scan(&totalEscaladosHoje)
	}

	// Sugestões pendentes
	var sugestoesPendentes int
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM setor_sugestoes WHERE grupo_id = ? AND status = 'pendente'`, escopo).Scan(&sugestoesPendentes)
	} else {
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM setor_sugestoes WHERE status = 'pendente'`).Scan(&sugestoesPendentes)
	}

	jsonOK(w, map[string]any{
		"grupo_id":              escopo,
		"grupo_nome":            grupoPrincipalNome,
		"data_hoje":             hoje,
		"data_ontem":            ontem,
		"total_efetivo":         totalEfetivo,
		"total_presentes_hoje":  totalPresentes,
		"total_escalados_hoje":  totalEscaladosHoje,
		"total_materiais":       totalMateriais,
		"total_cautelas_ativas": totalCautelasAbertas,
		"conferencias_fechadas": totalConferenciasFechadas,
		"sugestoes_pendentes":   sugestoesPendentes,
		"subordinados":          listaSub,
	})
}

// ---------- rotas rotasPessoal ----------
func (a *App) rotasPessoal() {
	m := a.mux
	m.HandleFunc("POST /api/login", a.hLogin)
	m.Handle("POST /api/setup", a.auth(false, a.hAuthSetup))
	m.Handle("GET /api/me", a.auth(false, a.hMe))
	m.Handle("POST /api/logout", a.auth(false, a.hLogout))
	m.Handle("POST /api/senha", a.auth(false, a.hTrocarSenha))

	// Contexto de Sessão (Multi-Funções)
	m.Handle("POST /api/sessao/contexto", a.auth(false, a.hMudarContexto))

	// Gestão de Papéis Múltiplos
	m.Handle("POST /api/usuarios/{id}/papeis", a.auth(false, a.hUsuarioPapelAdd))
	m.Handle("DELETE /api/usuarios/{id}/papeis/{papel_id}", a.auth(false, a.hUsuarioPapelDel))

	// Módulo de Mensageria Interna por Função & Despachos (v1.2 Fase 2)
	m.Handle("GET /api/mensagens/inbox", a.auth(false, a.hMensagensInbox))
	m.Handle("GET /api/mensagens/enviadas", a.auth(false, a.hMensagensEnviadas))
	m.Handle("POST /api/mensagens", a.auth(false, a.hMensagensEnviar))
	m.Handle("POST /api/mensagens/{id}/ler", a.auth(false, a.hMensagensMarcarLida))
	m.Handle("POST /api/mensagens/{id}/excluir", a.auth(false, a.hMensagensExcluir))
	m.Handle("POST /api/mensagens/{id}/arquivar", a.auth(false, a.hMensagensArquivar))
	m.Handle("POST /api/mensagens/{id}/desarquivar", a.auth(false, a.hMensagensDesarquivar))
	m.Handle("GET /api/mensagens/pastas", a.auth(false, a.hMensagensPastasList))
	m.Handle("POST /api/mensagens/pastas", a.auth(false, a.hMensagensPastasAdd))
	m.Handle("DELETE /api/mensagens/pastas/{id}", a.auth(false, a.hMensagensPastasDel))
	m.Handle("POST /api/mensagens/{id}/mover-pasta", a.auth(false, a.hMensagensMoverPasta))
	m.Handle("GET /api/mensagens/{id}/thread", a.auth(false, a.hMensagensThread))
	m.Handle("POST /api/mensagens/{id}/responder", a.auth(false, a.hMensagensResponderThread))
	m.Handle("POST /api/mensagens/{id}/finalizar", a.auth(false, a.hMensagensFinalizar))
	m.Handle("GET /api/mensagens/contador", a.auth(false, a.hMensagensContador))
	m.Handle("GET /api/mensagens/destinatarios", a.auth(false, a.hMensagensDestinatarios))
	m.Handle("GET /api/pessoas/{id}/ficha", a.auth(false, a.hPessoaFicha))

	// módulo Pessoal (f2): escrita de pessoas é do guarda — admin/gerente/enc/aux.
	// DELETE entra no guarda: era só auth e um OPERADOR do grupo excluía pessoa
	// (checagem interna cobre grupo, mas não papel). GET continua aberto (leitura).
	m.Handle("GET /api/pessoas", a.auth(false, a.hPessoasList))
	m.Handle("GET /api/pessoas/apresentacao", a.auth(false, a.hPessoaApresentacaoGet))
	m.Handle("POST /api/pessoas", a.guardaGestaoPessoal(a.hPessoasAdd))
	m.Handle("PATCH /api/pessoas/{id}", a.guardaGestaoPessoal(a.hPessoasEdit))
	m.Handle("DELETE /api/pessoas/{id}", a.guardaGestaoPessoal(a.hPessoaExcluir))
	m.Handle("POST /api/pessoas/{id}/apresentacao", a.guardaGestaoPessoal(a.hPessoaApresentacaoSet))
	m.Handle("GET /api/pessoas/{id}/modificacoes", a.auth(false, a.hPessoaModificacoes))
	m.Handle("GET /api/pessoas/{id}/qr", a.auth(false, a.hPessoaQRCode))
	m.Handle("GET /api/pessoas/{id}/pdf", a.auth(false, a.hPessoaPDF))

	m.Handle("GET /api/usuarios", a.auth(false, a.hUsuariosList)) // admin: todas; gerente: próprio+subordinados (04/10); operador/chefe: próprio grupo (v9.4) — leitura p/ TODOS os papeis (telas de designação); escrever é do guarda
	// criação é validada DENTRO do handler (admin cria qualquer; gerente cria
	// chefe_setor; chefe_setor promove operador; encarregado/auxiliar criam
	// operador/chefe do próprio grupo) — guarda de rota aqui quebraria o chefe.
	m.Handle("POST /api/usuarios", a.auth(false, a.hUsuariosAdd))
	m.Handle("PATCH /api/usuarios/{id}", a.guardaGestaoPessoal(a.hUsuarioEdit))
	m.Handle("DELETE /api/usuarios/{id}", a.auth(false, a.hUsuarioExcluir))   // R6; gerente só operador do próprio grupo (v9.4)
	m.Handle("POST /api/usuarios/{id}/senha", a.auth(false, a.hUsuarioSenha)) // admin: qualquer; gerente: operador do próprio grupo (v9.4)
	m.Handle("GET /api/usuarios/{id}/foto", a.auth(false, a.hUsuarioFotoGet))
	m.Handle("GET /api/perfil", a.auth(false, a.hPerfilGet))
	m.Handle("PATCH /api/perfil", a.auth(false, a.hPerfilSet))
	m.Handle("PATCH /api/usuarios/{id}/mover", a.auth(false, a.hMoverConta)) // v9.5: admin qualquer; gerente dentro da própria árvore

	// Consciência Situacional (v1.5) — Comando e Visão Geral Consolidada
	// Consciência Situacional: módulo em ACERVO para versão futura (ordem Diretor
	// 05/10/26: acesso removido across the board). Rota DESLIGADA (404 — mesmo
	// tratamento de rota inexistente); handler e código preservados para o religamento.
	m.HandleFunc("GET /api/consciencia/resumo", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
}
