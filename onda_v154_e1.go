package main

// onda_v154_e1.go — v1.5.4-E1 (defeitos R-9, R-10, R-14, R-16 do ARQUITETURA
// §14): guardas de autenticação/escopo nas rotas de contexto de sessão,
// gestão de papéis, envio de mensagens e senha.
//
//   R-9  — hMudarContexto: setor precisa pertencer ao grupo do PAPEL ATIVO;
//          chefe_setor só setores que AINDA comanda (chefe_setores — fonte
//          única desde a v1.5.4-D1); admin global. O `UPDATE usuarios SET
//          setor_id` global foi REMOVIDO: o contexto da sessão vive em
//          sessoes.setor_ativo_id (UsuarioDaSessao o sobrepõe);
//          usuarios.setor_id passa a ser cadastro/exibição (proposta D-5,
//          decisão pendente de comando).
//   R-10 — hUsuarioPapelAdd/Del: allowlist de solicitantes fail-closed
//          (podeGerirPapelAlvo) + escopo do alvo. O Del era fail-open: fora
//          do ramo gerente não havia checagem — chefe, operador nominal e
//          qualquer conta removiam papéis de qualquer um e RE-CHAVEAVAM as
//          sessões do alvo. O re-chaveio agora só ocorre após autorização.
//   R-14 — hMensagensEnviar: ninguém envia para fora do escopo do papel
//          ativo (próprio grupo + subordinados ativos — gruposSubordinados-
//          Ativos, doutrina de árvore do sistema); conta sem grupo não
//          envia a ninguém (403); admin segue global; resposta (pai_id)
//          alcança o remetente da mensagem pai.
//   R-16 — invalidarSessoesDeSenha: troca/redefinição de senha mata as
//          sessões da conta (server_pessoal.go); admin semeado com a senha
//          PADRÃO nasce com troca obrigatória (precisa_setup — SeedIfEmpty).

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashDeTokenCru: sessões guardam o SHA-256 do cookie (mesma transformação
// usada em CriarSessao/EncerrarSessao/UsuarioDaSessao — nada de armazenar o
// token cru).
func hashDeTokenCru(cru string) string {
	h := sha256.Sum256([]byte(cru))
	return hex.EncodeToString(h[:])
}

// invalidarSessoesDeSenha (R-16a): troca/redefinição de senha invalida TODAS
// as sessões da conta afetada. Quando a troca é PELA PRÓPRIA conta, a sessão
// corrente é preservada (manterTokenCorrente = cookie da requisição): quem
// acabou de autenticar e trocar a própria senha não pode cair do fluxo em
// uso — e o segredo novo já está em posse do legítimo dono. Redefinição por
// outrem (gerente/admin) passa nil e derruba tudo.
func (a *App) invalidarSessoesDeSenha(usuarioID int64, manterTokenCorrente *string) {
	if manterTokenCorrente != nil && *manterTokenCorrente != "" {
		_, _ = a.st.db.Exec(`DELETE FROM sessoes WHERE usuario_id = ? AND token_hash != ?`,
			usuarioID, hashDeTokenCru(*manterTokenCorrente))
		return
	}
	_, _ = a.st.db.Exec(`DELETE FROM sessoes WHERE usuario_id = ?`, usuarioID)
}

// podeGerirPapelAlvo (R-10): allowlist de solicitantes + escopo do alvo para
// gerir (atribuir/remover) papéis de sistema em hUsuarioPapelAdd/Del.
// Doutrina do hUsuariosAdd (v367/0610):
//   - admin é global (qualquer alvo, qualquer papel);
//   - gerente opera no PRÓPRIO grupo e subordinados ativos, mas só
//     operador/chefe_setor (nunca gerente/admin);
//   - designado na cadeira 'enc_pessoal' (titular/auxiliar — o cargo manda,
//     qualquer papel de sistema) só no PRÓPRIO grupo e só operador/chefe_setor.
// Fail-closed: operador comum, chefe_setor sem designação, conta sem grupo
// e qualquer papel não reconhecido NÃO gerem papéis.
func (a *App) podeGerirPapelAlvo(u *Usuario, papelAlvo string, grupoAlvo *int64) bool {
	if u == nil {
		return false
	}
	switch u.Papel {
	case "admin":
		return true
	case "gerente":
		if u.GrupoID == nil || grupoAlvo == nil {
			return false
		}
		if papelAlvo != "operador" && papelAlvo != "chefe_setor" {
			return false
		}
		return *grupoAlvo == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), *grupoAlvo)
	}
	// Designado na cadeira enc_pessoal (a checagem já nega admin/gerente
	// aqui — eles foram tratados acima — e exige grupo na sessão).
	if a.podeGestaoPessoal(u) && u.GrupoID != nil {
		if papelAlvo != "operador" && papelAlvo != "chefe_setor" {
			return false
		}
		return grupoAlvo != nil && *grupoAlvo == *u.GrupoID
	}
	return false
}

// chefeComandaSetorNoGrupo (R-9): o papel chefe_setor comanda o setor S
// DENTRO do grupo do papel ativo — fonte única chefe_setores (v1.5.4-D1) com
// o grupo amarrado (multi-chefia em grupos distintos não cruza contexto).
func (a *App) chefeComandaSetorNoGrupo(u *Usuario, setorID, grupoID int64) bool {
	if u == nil || setorID <= 0 || grupoID <= 0 {
		return false
	}
	var n int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores
		WHERE usuario_id = ? AND setor_id = ? AND grupo_id = ?`, u.ID, setorID, grupoID).Scan(&n); e != nil {
		return false
	}
	return n > 0
}
