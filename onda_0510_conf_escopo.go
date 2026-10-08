package main

// onda_0510_conf_escopo.go — onda conferências (ordem Diretor 05/10):
//   • ENCARREGADO DE PESSOAL (função cujo nome contém "encarregado", mesmo sem
//     papel do sistema) passa a REALIZAR conferências com escopo de GRUPO —
//     mesmo poder do gerente na conferência (inicia, marca em qualquer setor, fecha).
//   • chefe_setor E operador passam a ver/lançar SÓ militares do PRÓPRIO setor.
//   • Todos atuam na MESMA conferência aberta do grupo.
//
// Regra de detecção do encarregado: nome da função NORMALIZADO (minúsculas, sem
// acentos) contém "encarregado". Um único helper no Go (ehEncarregado) e um no
// front (window.ehEncarregado em core.js) — nada de lógica duplicada.
// Sem dependências novas: normalização própria (mapa de acentos), zero go get.

import (
	"net/http"
	"strings"
)

// normSemAcento: minúsculas + remove diacríticos (mapa próprio, sem x/text).
func normSemAcento(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	repl := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
	)
	return repl.Replace(s)
}

// ehEncarregado: usuário DESIGNADO (funcao_membros) a função cujo nome contém
// "encarregado" (normalizado) — consulta funcao_membros no escopo do grupo.
// NADA de consulta a usuarios.funcao_id / pessoas.funcao_id (R3: designação).
func (a *App) ehEncarregado(u *Usuario) bool {
	if u == nil {
		return false
	}
	esc := escopoDoUsuario(u)
	if esc <= 0 {
		return false
	}
	rows, err := a.st.db.Query(`SELECT f.nome FROM funcao_membros fm JOIN funcoes f ON f.id=fm.funcao_id WHERE fm.usuario_id=? AND fm.grupo_id=?`, u.ID, esc)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var nome string
		if err := rows.Scan(&nome); err == nil && containsNomeFuncao(nome, "encarregado") {
			return true
		}
	}
	return false
}

// setorDoUsuario: setor de atuação — u.SetorID com fallback pessoa.setor_id
// (mesma resolução já usada pelos guardas de chefe_setor).
func setorDoUsuario(a *App, u *Usuario) *int64 {
	if u.SetorID != nil && *u.SetorID > 0 {
		return u.SetorID
	}
	if u.PessoaID != nil && *u.PessoaID > 0 {
		var s *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&s)
		return s
	}
	return nil
}

// podeVerTodoSetor: gerente, ENCARREGADO e AUXILIAR de pessoal enxergam/lançam
// o grupo inteiro.
func podeVerTodoSetor(a *App, u *Usuario) bool {
	return u != nil && (u.Papel == "gerente" || a.ehEncarregado(u) || a.ehAuxiliarDePessoal(u))
}

// papelConfAutorizado: conjunto que passa nos middlewares confAuth/confMarcarAuth
// — papeis do sistema + encarregado de pessoal (sem papel). Admin segue proibido.
func (a *App) papelConfAutorizado(u *Usuario) bool {
	if u == nil {
		return false
	}
	switch u.Papel {
	case "gerente", "operador", "chefe_setor":
		return true
	}
	// ordem 06/10 (item 15): auxiliar de pessoal espelha o encarregado.
	return a.ehEncarregado(u) || a.ehAuxiliarDePessoal(u)
}

// authConfCom: middleware das áreas de conferência — papeis permitidos + encarregado
// de pessoal (sempre). Admin segue proibido. Mesma resposta 403 do authPapeis.
func (a *App) authConfCom(papeis []string, prox http.HandlerFunc) http.Handler {
	return a.auth(false, func(w http.ResponseWriter, r *http.Request) {
		u := usuarioDoCtx(r)
		permitido := false
		for _, p := range papeis {
			if u.Papel == p {
				permitido = true
				break
			}
		}
		if !permitido && u.Papel != "admin" {
			// ordem 06/10: auxiliar de pessoal espelha o encarregado
			permitido = a.ehEncarregado(u) || a.ehAuxiliarDePessoal(u)
		}
		if !permitido {
			http.Error(w, `{"erro":"papel sem acesso a esta área"}`, http.StatusForbidden)
			return
		}
		prox(w, r)
	})
}

// authConf: início/fechamento/arquivo de conferência — GERENTE, OPERADOR e
// ENCARREGADO (chefe_setor fica fora, como no confAuth original).
func (a *App) authConf(prox http.HandlerFunc) http.Handler {
	return a.authConfCom([]string{"gerente", "operador"}, prox)
}

// chefeComandaSetor: o papel chefe_setor comanda o setor S? Fonte da verdade =
// chefe_setores (multi-chefia 06/10). Fallbacks da CONTA LEGADA (chefe criado
// antes da v35, sem linha em chefe_setores): u.SetorID — o MESMO setor corrente
// que hMe devolve ao front — e o setor da pessoa vinculada. Sem os fallbacks o
// chefe legado via o botão "FECHAR MEU SETOR" liberado (hMe usa u.SetorID) e
// tomava 403 do próprio servidor (regressão 06/10, S4b/TestV15).
func (a *App) chefeComandaSetor(u *Usuario, setorID int64) bool {
	if u == nil || setorID <= 0 {
		return false
	}
	var comandos int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND setor_id = ?`, u.ID, setorID).Scan(&comandos); e == nil && comandos > 0 {
		return true
	}
	if u.SetorID != nil && *u.SetorID == setorID {
		return true
	}
	if u.PessoaID != nil && *u.PessoaID > 0 {
		var s *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&s)
		if s != nil && *s == setorID {
			return true
		}
	}
	return false
}

// guardaSetorNaMarcar: regra de setor para hConferenciaMarcar — chefe_setor E
// operador só militares do PRÓPRIO setor (chefe: linhas em chefe_setores —
// multi-chefia 06/10; operador: u.SetorID → fallback pessoa.setor_id).
// Gerente e encarregado não têm filtro (grupo inteiro). 403 com mensagem clara.
func (a *App) guardaSetorNaMarcar(w http.ResponseWriter, u *Usuario, pessoaID int64) bool {
	if podeVerTodoSetor(a, u) {
		return true
	}
	switch u.Papel {
	case "chefe_setor":
		// Multi-chefia: comanda TODOS os setores onde tem linha; fallbacks da
		// conta legada = u.SetorID (mesma resolução do hMe) e pessoa vinculada.
		var pSetor *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, pessoaID).Scan(&pSetor)
		if pSetor == nil {
			jsonErro(w, http.StatusForbidden, "militar sem setor — não é de comando do chefe")
			return false
		}
		if a.chefeComandaSetor(u, *pSetor) {
			return true
		}
		jsonErro(w, http.StatusForbidden, "você só pode lançar presença para militares do seu próprio setor")
		return false
	case "operador":
		setorUsuario := setorDoUsuario(a, u)
		if setorUsuario == nil {
			jsonErro(w, http.StatusForbidden, "sua conta não tem setor atribuído no cadastro — solicite ao gerente")
			return false
		}
		var setorPessoa *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, pessoaID).Scan(&setorPessoa)
		if setorPessoa == nil || *setorPessoa != *setorUsuario {
			jsonErro(w, http.StatusForbidden, "você só pode lançar presença para militares do seu próprio setor")
			return false
		}
		return true
	default:
		return true // outros papeis autorizados sem filtro de setor
	}
}
