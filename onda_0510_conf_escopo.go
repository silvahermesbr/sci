package main

// onda_0510_conf_escopo.go — onda conferências (ordem Diretor 05/10):
//   • ENCARREGADO DE PESSOAL (função cujo nome contém "encarregado", mesmo sem
//     papel do sistema) passa a REALIZAR conferências com escopo de GRUPO —
//     mesmo poder do gerente na conferência (inicia, marca em qualquer setor, fecha).
//   • chefe_setor E operador passam a ver/lançar SÓ militares do PRÓPRIO setor.
//   • Todos atuam na MESMA conferência aberta do grupo.
//
// Regra de detecção do encarregado (v1.6.0 FASE 2 — o PODER segue o CONTEXTO
// ATIVO da sessão): u.Papel é o papel da linha APONTADA por
// sessoes.papel_ativo_id (UsuarioDaSessao sobrepõe a cada leitura). A cadeira
// 'enc_pessoal'/'enc_material' MATERIALIZOU linha em usuario_papeis na migração
// v45 (legado) e sincroniza no ato da designação (hFuncaoMembrosSet, Fase 3) —
// a linha materializada É o espelho da designação; a query por request em
// funcao_membros SAIU. Efeito em cadeia automático pelos wrappers (ehEncarregado
// e ehAuxiliarDePessoal delegam para ehEncarregadoDePessoal; nada nos chamadores
// muda): podeVerTodoSetor, authConfCom/confAuth/confMarcarAuth, papelConfAutorizado,
// guardaSetorNaMarcar, podeGestaoPessoal, confPDFAuth, authMaterial, podeGerirPapelAlvo
// e a trava do encarregado — todos viram CONTEXT-BOUND: no contexto operador o
// usuário é operador (setor); no contexto enc_*, é encarregado. Quem tem os dois
// contextos troca no dropdown (POST /api/sessao/contexto).
// normSemAcento segue viva para as migrações legadas (store.go) — detecção por
// NOME não autoriza mais nada.
// Sem dependências novas: zero go get.

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

// ehEncarregadoDePessoal: o CONTEXTO ATIVO da sessão é 'enc_pessoal' (titular
// ou auxiliar — a titularidade é detalhe da designação, o poder é da cadeira).
// Fonte: linha materializada em usuario_papeis (migração v45 materializou o
// legado; a designação nova sincroniza a linha no hFuncaoMembrosSet — Fase 3).
// A linha tem grupo_id → u.GrupoID resolve o escopo; sem contexto enc → false.
func (a *App) ehEncarregadoDePessoal(u *Usuario) bool {
	return u != nil && u.Papel == "enc_pessoal"
}

// ehEncarregadoDeMaterial: o CONTEXTO ATIVO da sessão é 'enc_material' (mesma
// doutrina do ehEncarregadoDePessoal — linha materializada em usuario_papeis
// pela v45/Fase 3; detection por chave imutável do PAPEL, nunca por nome).
func (a *App) ehEncarregadoDeMaterial(u *Usuario) bool {
	return u != nil && u.Papel == "enc_material"
}

// ehEncarregado: wrapper de compatibilidade delegando para ehEncarregadoDePessoal.
// v1.6.0 Fase 2: com o contexto ativo como fonte, TODOS os chamadores deste
// wrapper viram context-bound sem mudança nenhuma neles.
func (a *App) ehEncarregado(u *Usuario) bool {
	return a.ehEncarregadoDePessoal(u)
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

// podeVerTodoSetor: gerente ou contexto 'enc_pessoal' ATIVO enxergam/lançam
// o grupo inteiro (v1.6.0 Fase 2: no contexto operador/chefe, o mesmo usuário
// com cadeira volta ao corte do próprio setor).
func podeVerTodoSetor(a *App, u *Usuario) bool {
	return u != nil && (u.Papel == "gerente" || a.ehEncarregado(u) || a.ehAuxiliarDePessoal(u))
}

// papelConfAutorizado: conjunto que passa nos middlewares confAuth/confMarcarAuth
// — papeis do sistema + CONTEXTO 'enc_pessoal' ativo (era "encarregado sem
// papel"; agora é a linha materializada). Admin segue proibido.
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

// exigeSetorOperador (v1.6.0 FASE 4 — extinção do operador de grupo): o
// operador é de SETOR. Conta com contexto ativo 'operador' SEM setor resolvido
// (usuarios.setor_id → pessoas.setor_id) não atua nos módulos operacionais:
// 403 com a mensagem de reparo. A criação já exige setor (hUsuariosAdd e
// hUsuarioPapelAdd); contas legadas ficam BLOQUEADAS até atribuição — sem
// auto-adivinhação (decisão do comando). Ficam FORA, por doutrina: leituras
// de conferência (hoje/estado/lista/{id} — o corte vazio já protege) e
// mensagens/mural/drive (comunicação por papel — interpretação registrada).
// Chamado nos 4 portões de módulo: authConfCom, authMaterial, reservaAuth e
// os handlers de dados de relatório (escopoRelatorio é helper sem ResponseWriter).
func (a *App) exigeSetorOperador(w http.ResponseWriter, u *Usuario) bool {
	if u == nil || u.Papel != "operador" {
		return true
	}
	if setorDoUsuario(a, u) != nil {
		return true
	}
	jsonErro(w, http.StatusForbidden, "conta sem setor atribuído — solicite ao gerente/encarregado")
	return false
}

// authConfCom: middleware das áreas de conferência — papeis permitidos +
// CONTEXTO 'enc_pessoal' ativo (o encarregado de pessoal de verdade só existe
// no seu contexto; no contexto operador/chefe ele é operador/chefe). Admin
// segue proibido. Mesma resposta 403 do authPapeis.
// v1.6.0 Fase 4: operador SEM setor (legado) é barrado aqui — exigeSetorOperador.
func (a *App) authConfCom(papeis []string, prox http.HandlerFunc) http.Handler {
	return a.auth(false, func(w http.ResponseWriter, r *http.Request) {
		u := usuarioDoCtx(r)
		if !a.exigeSetorOperador(w, u) {
			return
		}
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

// chefeComandaSetor: o papel chefe_setor comanda o setor S? Fonte ÚNICA de
// verdade = chefe_setores (multi-chefia 06/10). Os fallbacks da CONTA LEGADA
// (u.SetorID e pessoas.setor_id) foram REMOVIDOS na v1.5.4-D1 (R-12,
// chefe-zumbi): eles devolviam poder ao chefe destituído cujo usuarios.setor_id
// ficou órfão da chefia anterior. Legados pendentes são MATERIALIZADOS na
// migração v44 antes da troca (decisão D-2). Consulte setorAtivoComandado para
// o guarda do contexto da sessão.
func (a *App) chefeComandaSetor(u *Usuario, setorID int64) bool {
	if u == nil || setorID <= 0 {
		return false
	}
	var comandos int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND setor_id = ?`, u.ID, setorID).Scan(&comandos); e == nil && comandos > 0 {
		return true
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
		// ordem 08/10 — contexto-govena: o chefe só lança no setor ATIVO do
		// contexto da sessão (u.SetorID → fallback pessoa vinculada). Antes,
		// chefeComandaSetor autorizava lançamento em TODOS os setores que
		// comanda — cruzava dados entre setores.
		var pSetor *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, pessoaID).Scan(&pSetor)
		if pSetor == nil {
			jsonErro(w, http.StatusForbidden, "militar sem setor — não é de comando do chefe")
			return false
		}
		// v1.5.4-D1 (R-12): o contexto só vale se o chefe AINDA comanda o setor
		// (setorAtivoComandado) — contexto órfão de chefia anterior não marca.
		sAtivo := a.setorAtivoComandado(u)
		if sAtivo != nil && *sAtivo == *pSetor {
			return true
		}
		jsonErro(w, http.StatusForbidden, "setor ativo no seu contexto é outro — troque a função no menu de contexto para lançar neste setor")
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
