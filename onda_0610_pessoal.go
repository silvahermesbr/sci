package main

// onda_0610_pessoal.go — itens 3 e 15 da ordem do Diretor 06/10:
//   • ENCARREGADO DE PESSOAL (função cujo nome contém "encarregado", mesmo sem
//     papel do sistema) passa a GERENCIAR PESSOAL no ESCOPO do próprio grupo:
//     listar/criar/editar contas do grupo, banco de pessoal (pessoas) e
//     catálogos de setores/funções do grupo (POST/PATCH).
//     AUXILIAR DE PESSOAL (item 15) tem os MESMOS poderes e as MESMAS
//     restrições (detecção: nome da função contém "auxiliar").
//   • NÃO ganha poderes de admin: NUKE/MODO_RESERVA/backup/import, excluir
//     grupo/conferência arquivada, criar gerente. Criação de conta fica
//     limitada a operador/chefe_setor do PRÓPRIO grupo (mesma regra forçada
//     do gerente). Senha de conta (hUsuarioSenha) continua gerente/admin.
//   • SEM migração de schema (zero tabela nova): detecção é pelo NOME da
//     função (funcao do usuário → fallback função da pessoa vinculada).
//
// Detecção do encarregado REUSA ehEncarregado (onda_0510_conf_escopo.go) —
// nada reinventado. Admin segue 403-papel onde já é hoje: o cheque explícito
// de admin de cada handler continua ANTES deste guarda.

import (
	"net/http"
	"strings"
)

// containsNomeFuncao: nome normalizado (minúsculas, sem acento) contém agulha.
func containsNomeFuncao(nome, agulha string) bool {
	return strings.Contains(normSemAcento(nome), agulha)
}

// ehAuxiliarDePessoal (item 15): MESMA regra de detecção do encarregado, com
// "auxiliar". Função do cadastro do usuário tem precedência; em último caso
// consulta a função da pessoa vinculada. Sozinha não dá poder nenhum — só
// alimenta podeGestaoPessoal e o ramo de nomeação de função.
func (a *App) ehAuxiliarDePessoal(u *Usuario) bool {
	if u == nil {
		return false
	}
	if containsNomeFuncao(u.FuncaoNome, "auxiliar") {
		return true
	}
	if u.FuncaoID != nil && *u.FuncaoID > 0 {
		var nome string
		if err := a.st.db.QueryRow(`SELECT nome FROM funcoes WHERE id = ?`, *u.FuncaoID).Scan(&nome); err == nil && containsNomeFuncao(nome, "auxiliar") {
			return true
		}
	}
	if u.PessoaID != nil && *u.PessoaID > 0 {
		var nome string
		if err := a.st.db.QueryRow(`SELECT nome FROM funcoes WHERE id = (SELECT funcao_id FROM pessoas WHERE id = ?)`, *u.PessoaID).Scan(&nome); err == nil && containsNomeFuncao(nome, "auxiliar") {
			return true
		}
	}
	return false
}

// funcaoIDPorNome: id da função do catálogo do GRUPO (ou global) cujo nome
// normalizado contém a agulha — grupo específico tem precedência sobre global.
// Nil = o grupo não tem função que casa (nada a designar).
func (a *App) funcaoIDPorNome(grupoID *int64, agulha string) *int64 {
	if grupoID == nil || *grupoID <= 0 {
		return nil
	}
	rows, err := a.st.db.Query(`SELECT id, nome FROM funcoes
		 WHERE grupo_id = ? OR grupo_id IS NULL
		 ORDER BY CASE WHEN grupo_id IS NULL THEN 1 ELSE 0 END`, *grupoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var nome string
		if rows.Scan(&id, &nome) == nil && containsNomeFuncao(nome, agulha) {
			fid := id
			return &fid
		}
	}
	return nil
}

// podeGestaoPessoal: GERENTE (qualquer escopo — os handlers validam o próprio),
// OU encarregado/auxiliar de pessoal COM grupo na sessão. Admin devolve false
// de propósito: onde o admin já era aceito, o cheque de admin roda antes.
func (a *App) podeGestaoPessoal(u *Usuario) bool {
	if u == nil {
		return false
	}
	if u.Papel == "gerente" {
		return true
	}
	if u.Papel == "admin" {
		return false
	}
	// papel do sistema MANDA: operador/chefe_setor não ganham poderes de
	// gestão por função renomeada no catálogo (anti-escalação de privilégio).
	if u.Papel == "operador" || u.Papel == "chefe_setor" {
		return false
	}
	if esc := escopoDoUsuario(u); esc <= 0 {
		return false
	}
	return a.ehEncarregado(u) || a.ehAuxiliarDePessoal(u)
}

// guardaGestaoPessoal: middleware das rotas de pessoal que hoje exigem
// gerente/admin — admin continua 403-papel (regra da ordem), operador/comum
// também. Encarregado/auxiliar passam e o handler aplica o escopo do grupo.
func (a *App) guardaGestaoPessoal(prox http.HandlerFunc) http.Handler {
	return a.auth(false, func(w http.ResponseWriter, r *http.Request) {
		u := usuarioDoCtx(r)
		if !a.podeGestaoPessoal(u) {
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
	auxID := a.funcaoIDPorNome(grupoDaPessoa, "auxiliar")
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
