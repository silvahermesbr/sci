package main

// Testes da Fase E da ordem 04/10 — anexos com integração ao Drive:
//   E1. Mensagem anexando arquivo DO DRIVE por referência (sem dados_base64):
//       gravado com metadados do drive; anexo inacessível é recusado.
//   E2. Comentário de aviso com anexo (referência) e leitura nos detalhes.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// criaDriveArquivoDe: cadastra arquivo de drive com autor específico.
func criaDriveArquivoDe(t *testing.T, st *Store, grupoID int64, nome, autorLogin string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`
		INSERT INTO drive_arquivos (pasta_id, grupo_id, nome_original, nome_armazenado, tipo, tamanho, autor_usuario_id, autor_papel_id)
		VALUES (NULL, ?, ?, ?, 'application/pdf', 1234,
		        (SELECT id FROM usuarios WHERE login = ?),
		        (SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?))
		RETURNING id`, grupoID, nome, "arq_"+nome, autorLogin, autorLogin).Scan(&id); err != nil {
		t.Fatalf("criar arquivo de drive: %v", err)
	}
	return id
}

func criaDriveArquivo(t *testing.T, st *Store, grupoID int64, nome string) int64 {
	t.Helper()
	return criaDriveArquivoDe(t, st, grupoID, nome, "gerdr")
}

func ctxDrive(t *testing.T, app *App, st *Store) (int64, *http.Cookie, *http.Cookie) {
	t.Helper()
	ckGer := loginAsPapel(t, app, st, "gerdr", "gerente")
	ckOp := loginAsPapel(t, app, st, "opdr", "operador")
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Drive') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	for _, lg := range []string{"gerdr", "opdr"} {
		if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, gid, lg); err != nil {
			t.Fatalf("vincular %s: %v", lg, err)
		}
		if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login = ?)`, gid, lg); err != nil {
			t.Fatalf("papel de %s no grupo: %v", lg, err)
		}
	}
	return gid, ckGer, ckOp
}

func TestAnexoMensagemPorReferenciaDrive(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer, _ := ctxDrive(t, app, st)
	arqID := criaDriveArquivo(t, st, gid, "diretriz.pdf")

	var papelOpID int64
	_ = st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login='opdr'`).Scan(&papelOpID)

	// E1 feliz: anexo por referência
	rr, res := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelOpID},
		"assunto":                "Com anexo do drive",
		"corpo":                  "<p>veja o anexo</p>",
		"tipo":                   "comum",
		"anexos":                 []map[string]any{{"drive_arquivo_id": arqID}},
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("envio com anexo de drive deve 200, veio %d: %v", rr.Code, res)
	}

	var anexos string
	if err := st.db.QueryRow(`SELECT COALESCE(anexos,'[]') FROM mensagens ORDER BY id DESC LIMIT 1`).Scan(&anexos); err != nil {
		t.Fatalf("ler anexos: %v", err)
	}
	if !strings.Contains(anexos, fmt.Sprintf(`"drive_arquivo_id":%d`, arqID)) {
		t.Fatalf("anexo deve referenciar o arquivo do drive (%d), veio %s", arqID, anexos)
	}
	if strings.Contains(anexos, "dados_base64") {
		t.Fatalf("anexo por referência NÃO deve carregar dados_base64 (sem duplicação): %s", anexos)
	}

	// E1 triste: anexo de OUTRO grupo sem compartilhamento = recusado
	var gidOutro int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Alheio') RETURNING id`).Scan(&gidOutro); err != nil {
		t.Fatalf("criar grupo alheio: %v", err)
	}
	arqAlheio := criaDriveArquivoDe(t, st, gidOutro, "secreto.pdf", "admin")

	rr2, _ := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelOpID},
		"assunto":                "Com anexo alheio",
		"corpo":                  "<p>x</p>",
		"tipo":                   "comum",
		"anexos":                 []map[string]any{{"drive_arquivo_id": arqAlheio}},
	}, ckGer)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("anexo de drive sem permissão deve 400, veio %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestAnexoComentarioAviso(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer, _ := ctxDrive(t, app, st)
	arqID := criaDriveArquivo(t, st, gid, "ata.pdf")

	// aviso do grupo (via SQL direto: superfície de lista não importa aqui)
	var avisoID int64
	if err := st.db.QueryRow(`
		INSERT INTO avisos (titulo, conteudo, grupo_id, autor_usuario_id, autor_papel_id)
		VALUES ('Aviso E', 'conteudo', ?, (SELECT id FROM usuarios WHERE login='gerdr'),
		        (SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login='gerdr'))
		RETURNING id`, gid).Scan(&avisoID); err != nil {
		t.Fatalf("criar aviso: %v", err)
	}

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/avisos/%d/comentar", avisoID), map[string]any{
		"texto":  "Segue a ata.",
		"anexos": []map[string]any{{"drive_arquivo_id": arqID}},
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("comentar com anexo deve 200, veio %d: %v", rr.Code, res)
	}

	rrD, resD := doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", avisoID), nil, ckGer)
	if rrD.Code != http.StatusOK {
		t.Fatalf("detalhes: %d", rrD.Code)
	}
	det, _ := resD["comentarios"].([]any)
	if len(det) != 1 {
		t.Fatalf("esperado 1 comentário, veio %v", resD)
	}
	com := det[0].(map[string]any)
	anx, _ := com["anexos"].([]any)
	if len(anx) != 1 {
		t.Fatalf("comentário deve devolver 1 anexo, veio %v", com)
	}
	first := anx[0].(map[string]any)
	if idv, _ := first["drive_arquivo_id"].(float64); int64(idv) != arqID {
		t.Fatalf("anexo devolvido deve referenciar drive_arquivo_id %d, veio %v", arqID, first)
	}
}
