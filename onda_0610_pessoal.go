package main

// onda_0610_pessoal.go — item 3 da ordem do Diretor 06/10 (SCI-BUGS-0610):
//   • ENCARREGADO DE PESSOAL (função cujo nome contém "encarregado", mesmo sem
//     papel do sistema) e AUXILIAR DE PESSOAL (item 15, nome contém "auxiliar")
//     GEREM PESSOAL no ESCOPO do próprio grupo: listar/criar/editar contas do
//     grupo, banco de pessoal (pessoas), catálogos de setores/funções do grupo
//     (POST/PATCH/DELETE/reparentar) e designação de membros de função.
//   • NÃO ganham poderes de admin: NUKE/MODO_RESERVA/backup/import, excluir
//     grupo/conferência arquivada, criar gerente/admin. Criação de conta fica
//     limitada a operador/chefe_setor do PRÓPRIO grupo. Senha de conta
//     (hUsuarioSenha) continua gerente/admin.
//   • SEM migração de schema (zero tabela nova): detecção é pelo NOME da
//     função (funcao do usuário → fallback função da pessoa vinculada).
//
// ADMIN entra em tudo onde já entrava: podeGestaoPessoal(admin) = true — o
// guarda das rotas roda ANTES do handler, então negar admin aqui é barrar
// quem sempre foi dono dessas rotas (regressão corrigida nesta versão).
// Detecção reusa ehEncarregado (onda_0510_conf_escopo.go) — nada reinventado.

import (
	"net/http"
)

// ehAuxiliarDePessoal: wrapper de compatibilidade delegando para ehEncarregadoDePessoal.
// Detecção por chave, nunca por nome (anti-escalação).
func (a *App) ehAuxiliarDePessoal(u *Usuario) bool {
	return a.ehEncarregadoDePessoal(u)
}

// funcaoIDPorChave: id da função com a chave imutável pedida; função do GRUPO
// ganha da global. Nil = cadeira não resolvida.
func (a *App) funcaoIDPorChave(grupoID *int64, chave string) *int64 {
	var id int64
	var err error
	if grupoID != nil && *grupoID > 0 {
		err = a.st.db.QueryRow(`SELECT id FROM funcoes
			WHERE chave = ? AND ativo = 1 AND (grupo_id = ? OR grupo_id IS NULL)
			ORDER BY (grupo_id IS NULL) ASC LIMIT 1`, chave, *grupoID).Scan(&id)
	} else {
		err = a.st.db.QueryRow(`SELECT id FROM funcoes
			WHERE chave = ? AND ativo = 1 AND grupo_id IS NULL
			LIMIT 1`, chave).Scan(&id)
	}
	if err != nil {
		return nil
	}
	return &id
}

// podeGestaoPessoal: quem gere pessoal — ADMIN e GERENTE sempre (os handlers
// validam o escopo de cada um) ou designado (titular/auxiliar) na cadeira
// 'enc_pessoal' COM grupo na sessão — v367: QUALQUER papel (v366 revoga o
// paliativo anti-fantasia: o sync não fabrica mais linha em usuario_papeis,
// então papel de sistema + designação legítima é o caso real; o trava que
// negava ANTES de consultar a designação só impedia o encarregado legítimo).
// Fail-closed mantido: SEM designação ativa na cadeira → sem poder, qualquer papel.
// Detecção por chave imutável 'enc_pessoal', nunca por nome (anti-escalação).
func (a *App) podeGestaoPessoal(u *Usuario) bool {
	if u == nil {
		return false
	}
	if u.Papel == "admin" || u.Papel == "gerente" {
		return true
	}
	if esc := escopoDoUsuario(u); esc <= 0 {
		return false
	}
	return a.ehEncarregadoDePessoal(u)
}

// guardaGestaoPessoal: middleware das rotas de gestão de pessoal — admin e
// gerente passam sempre; encarregado/auxiliar de pessoal passam e o handler
// aplica o escopo do grupo; operador/chefe_setor/comum → 403.
// ATENÇÃO: NÃO aplicar em GET /api/usuarios — a listagem de contas é lida por
// TODOS os papeis (v9.4: operador/chefe lêem o próprio grupo; as telas de
// designação de conferência, calendário e drive dependem disso).
func (a *App) guardaGestaoPessoal(prox http.HandlerFunc) http.Handler {
	return a.auth(false, func(w http.ResponseWriter, r *http.Request) {
		if !a.podeGestaoPessoal(usuarioDoCtx(r)) {
			jsonErro(w, http.StatusForbidden, "gestão de pessoal é do gerente ou do encarregado/auxiliar de pessoal")
			return
		}
		prox(w, r)
	})
}

// atribuiFuncaoPessoal (item 15): QUEM NOMEIA função no PATCH de pessoa.
// GERENTE/ADMIN: qualquer função. Encarregado/auxiliar: só MUDAR função quando
// a mudança é definir/desfazer a "auxiliar" do próprio grupo — reenvio da
// MESMA função (edição de nome/setor com o formulário completo) passa.
// funcaoNova = req.FuncaoID do PATCH (nil = limpar); funcaoAntiga = atual.
func (a *App) atribuiFuncaoPessoal(u *Usuario, grupoDaPessoa *int64, funcaoNova, funcaoAntiga *int64) (bool, string) {
	if u == nil {
		return false, "sem usuário na sessão"
	}
	if u.Papel == "admin" || u.Papel == "gerente" {
		return true, ""
	}
	if !a.podeGestaoPessoal(u) {
		return false, "sem permissão para editar pessoal"
	}
	auxID := a.funcaoIDPorChave(grupoDaPessoa, "enc_pessoal")
	mesmoValor := func(a, b *int64) bool {
		return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
	}
	if mesmoValor(funcaoNova, funcaoAntiga) {
		return true, "" // função não mudou — só edição dos demais campos
	}
	if funcaoNova != nil {
		// definindo função nova: só a auxiliar do grupo
		if auxID == nil || *funcaoNova != *auxID {
			return false, "encarregado/auxiliar só designa a função auxiliar de pessoal do próprio grupo"
		}
		return true, ""
	}
	// funcaoNova == nil: limpando a função — só se a antiga era a auxiliar
	if funcaoAntiga != nil && (auxID == nil || *funcaoAntiga != *auxID) {
		return false, "encarregado/auxiliar só remove a função auxiliar de pessoal"
	}
	return true, ""
}
