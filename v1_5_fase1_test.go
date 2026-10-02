package main

import (
	"testing"
)

// TestMigracaoV27ValidaSchema: Valida criação das tabelas e colunas da v1.5 (Schema v27)
func TestMigracaoV27ValidaSchema(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Verificar versão do schema_migrations
	var v int
	if err := st.db.QueryRow(`SELECT MAX(versao) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatalf("ler schema_migrations: %v", err)
	}
	if v < 28 {
		t.Fatalf("esperava schema versão >= 28, obteve %d", v)
	}

	// 2. Verificar tabelas de modelos de escala
	tabelas := []string{"escala_modelos", "escala_modelo_postos", "escala_modelo_aptos", "setor_sugestoes"}
	for _, tab := range tabelas {
		var n int
		if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?`, tab).Scan(&n); err != nil || n == 0 {
			t.Fatalf("tabela %s não encontrada no banco", tab)
		}
	}

	// 3. Verificar colunas novas em escala_turnos e escala_modelo_postos
	colunasTurnos := []string{"fase", "modelo_id", "grupo_delegado_id", "status_delegacao", "posto_grad_min_id", "posto_grad_max_id"}
	for _, col := range colunasTurnos {
		var n int
		if err := st.db.QueryRow(`SELECT count(*) FROM pragma_table_info('escala_turnos') WHERE name = ?`, col).Scan(&n); err != nil || n == 0 {
			t.Fatalf("coluna %s não encontrada em escala_turnos", col)
		}
	}
	colunasPostos := []string{"posto_grad_min_id", "posto_grad_max_id"}
	for _, col := range colunasPostos {
		var n int
		if err := st.db.QueryRow(`SELECT count(*) FROM pragma_table_info('escala_modelo_postos') WHERE name = ?`, col).Scan(&n); err != nil || n == 0 {
			t.Fatalf("coluna %s não encontrada em escala_modelo_postos", col)
		}
	}

	// 4. Testar idempotência chamando migrarV27 e migrarV28 novamente
	if err := st.migrarV27(); err != nil {
		t.Fatalf("migrarV27 idempotente falhou: %v", err)
	}
	if err := st.migrarV28(); err != nil {
		t.Fatalf("migrarV28 idempotente falhou: %v", err)
	}

	_ = app
}
