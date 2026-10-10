package main

// onda_v160_material_setor.go — v1.6.0 Fase 7 ("setor em tudo" para o
// OPERADOR): o módulo Material ganha a dimensão de SETOR para o contexto
// operador. Gerente e CONTEXTO enc_material seguem com escopo de GRUPO
// (catálogo, responsáveis e itens de carga geral incluídos); admin continua
// PROIBIDO pelo authMaterial. mensagens/mural/drive NÃO mudam (comunicação
// por papel — interpretação registrada no plano) e escalas seguem dormentes
// com reservaAuth escopo grupo (D-3 documentada no gate).
//
// O recorte é aplicado handler a handler (leituras com filtro SQL, escritas
// com exigência de objeto do próprio setor), porque o authMaterial é porta de
// PAPEL — e o helper do guarda central (exigeSetorOperador, onda paralela do
// Agente B) não existe nesta base. Na integração as duas camadas COEXISTEM:
// este arquivo NÃO toca o authMaterial.

import (
	"database/sql"
	"net/http"
)

// msgSemSetorOperador: mensagem ÚNICA da conta operador sem setor — a MESMA
// do guarda central (exigeSetorOperador, middleware do Agente B dentro do
// authMaterial). Recusas redundantes na integração falam a mesma língua.
const msgSemSetorOperador = "conta sem setor atribuído — solicite ao gerente/encarregado"

// setorEscopoMaterial: recorte de setor do módulo para o CONTEXTO ativo.
//   - gerente / enc_material / admin → nil (escopo GRUPO: nada recortado);
//   - operador → o setor de atuação (u.SetorID → fallback pessoa vinculada,
//     mesma resolução dos guardas de conferência).
//
// nil NÃO significa "sem recorte aplicável" para o operador: operador SEM
// setor é recusa (recortaSetorMaterial), nunca escopo de grupo.
func setorEscopoMaterial(a *App, u *Usuario) *int64 {
	if u == nil {
		return nil
	}
	if u.Papel == "operador" {
		return setorDoUsuario(a, u)
	}
	return nil
}

// recortaSetorMaterial: resolve o recorte e RECUSA o operador sem setor.
// Devolve (recorte, true) para seguir; (nil, false) depois de escrever o 403.
// Chamar ANTES de abrir transação (consulta o pool — pool=1: query no pool com
// tx aberta é deadlock, lição bd6a7af).
func recortaSetorMaterial(w http.ResponseWriter, a *App, u *Usuario) (*int64, bool) {
	corte := setorEscopoMaterial(a, u)
	if u != nil && u.Papel == "operador" && corte == nil {
		jsonErro(w, http.StatusForbidden, msgSemSetorOperador)
		return nil, false
	}
	return corte, true
}

// setorNoCorte: o objeto (item/cautela/conferência de material) passa no
// recorte? recorte nil (escopo grupo) → tudo passa; operador → o objeto PRECISA
// ter setor e ser o DELE (setor NULL = Carga Geral, fora do recorte).
func setorNoCorte(corte *int64, obj *int64) bool {
	if corte == nil {
		return true
	}
	return obj != nil && *obj == *corte
}

// itemSetorID: setor do item de material (nil = Carga Geral). Pool apenas.
func itemSetorID(db *sql.DB, itemID int64) *int64 {
	var s *int64
	_ = db.QueryRow(`SELECT setor_id FROM material_itens WHERE id = ?`, itemID).Scan(&s)
	return s
}

// cautelaItemSetor: setor do ITEM da cautela — o recorte da cautela é o setor
// do seu item (cautela não tem setor próprio). Pool apenas.
func cautelaItemSetor(db *sql.DB, cautelaID int64) *int64 {
	var s *int64
	_ = db.QueryRow(`SELECT mi.setor_id
		FROM material_cautelas mc JOIN material_itens mi ON mi.id = mc.item_id
		WHERE mc.id = ?`, cautelaID).Scan(&s)
	return s
}

// materialConfSetor: setor da conferência de material (nil = Carga Geral).
// Pool apenas.
func materialConfSetor(db *sql.DB, confID int64) *int64 {
	var s *int64
	_ = db.QueryRow(`SELECT setor_id FROM material_conferencias WHERE id = ?`, confID).Scan(&s)
	return s
}
