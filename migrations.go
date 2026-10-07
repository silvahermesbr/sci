// migrations.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"fmt"
	"strings"
)

func (s *Store) migrarV2() error {
	rows, err := s.db.Query(`PRAGMA table_info(usuarios)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	tem := false
	for rows.Next() {
		var cid int
		var nome, tipo string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &nome, &tipo, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if nome == "senhas" {
			tem = true
		}
	}
	rows.Close()
	if tem {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE usuarios ADD COLUMN senhas TEXT NOT NULL DEFAULT '[]'`)
	return err
}

func (s *Store) migrarV18() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 18`).Scan(&v)
	if v == 18 {
		return nil
	}
	if !s.colunaExiste("conferencias", "arquivada_em") {
		if _, err := s.db.Exec(`ALTER TABLE conferencias ADD COLUMN arquivada_em TEXT`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migração v18: %w", err)
			}
		}
	}
	if !s.colunaExiste("comentarios", "tag_id") {
		if _, err := s.db.Exec(`ALTER TABLE comentarios ADD COLUMN tag_id INTEGER REFERENCES tags(id)`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migração v18 comentarios: %w", err)
			}
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_conferencias_arq ON conferencias(arquivada_em)`); err != nil {
		return fmt.Errorf("migração v18 idx: %w", err)
	}
	return s.marcarVersao(18)
}
