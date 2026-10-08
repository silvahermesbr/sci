package main

import (
	"database/sql"
	"log"
	"sync"
)

var (
	despachoOnce sync.Once
	despachoMu   sync.Mutex
	despachoDbs  = make(map[*sql.DB]*sync.Once)
)

func (a *App) ensureTabelaDespachos() {
	if a == nil || a.st == nil || a.st.db == nil {
		return
	}
	db := a.st.db
	despachoMu.Lock()
	once, ok := despachoDbs[db]
	if !ok {
		once = &sync.Once{}
		despachoDbs[db] = once
	}
	despachoMu.Unlock()

	despachoOnce.Do(func() {})

	once.Do(func() {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS conferencia_despachos (
			id INTEGER PRIMARY KEY,
			conferencia_id INTEGER NOT NULL REFERENCES conferencias(id) ON DELETE CASCADE,
			setor_id INTEGER NOT NULL REFERENCES setores(id),
			criado_por INTEGER NOT NULL REFERENCES usuarios(id),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			UNIQUE (conferencia_id, setor_id)
		)`)
		if err != nil {
			log.Printf("sci despachos ensure: %v", err)
		}
	})
}
