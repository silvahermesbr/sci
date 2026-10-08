package main

import (
	"database/sql"
	"sync"
)

var (
	apresentacaoSchemaMu   sync.Mutex
	apresentacaoSchemaOnce = make(map[*sql.DB]*sync.Once)
)

// ensureTabelaApresentacao garante de forma idempotente a criação da tabela
// pessoas_apresentacao utilizando padrão sync.Once por conexão com banco.
func (a *App) ensureTabelaApresentacao() error {
	if a == nil || a.st == nil || a.st.db == nil {
		return nil
	}
	db := a.st.db
	apresentacaoSchemaMu.Lock()
	once, ok := apresentacaoSchemaOnce[db]
	if !ok {
		once = &sync.Once{}
		apresentacaoSchemaOnce[db] = once
	}
	apresentacaoSchemaMu.Unlock()

	var execErr error
	once.Do(func() {
		q := `CREATE TABLE IF NOT EXISTS pessoas_apresentacao (
    pessoa_id INTEGER PRIMARY KEY REFERENCES pessoas(id) ON DELETE CASCADE,
    estado TEXT NOT NULL DEFAULT 'presente' CHECK (estado IN ('presente','dispensado','descompensado','a serviço externo','atrasado','falta')),
    motivo TEXT,
    definido_por INTEGER REFERENCES usuarios(id),
    definido_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
)`
		_, execErr = db.Exec(q)
	})
	return execErr
}
