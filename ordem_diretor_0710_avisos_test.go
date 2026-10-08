package main

// Testes da ordem Diretor 07/10 — avisos notificam TODO o grupo:
// gerente publica aviso → cada usuário ativo do grupo (exceto o autor) ganha
// linha em mensagem_destinatarios (fonte do sino/badge) e o contador do
// /api/notificacoes (avisos_pendentes) sobe para quem ainda não deu ciente.

import (
	"net/http"
	"testing"
)

func TestAvisoNotificaTodoGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo Aviso 0710') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	criaUsuarioTeste(t, st, "ger_av", "senha-g", "gerente")
	criaUsuarioTeste(t, st, "op_av1", "senha-o1", "operador")
	criaUsuarioTeste(t, st, "op_av2", "senha-o2", "operador")
	criaUsuarioTeste(t, st, "chefe_av", "senha-c", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('ger_av','op_av1','op_av2','chefe_av')`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}

	ckGer := loginAs(t, app, "ger_av", "senha-g")

	// 1. Publicar aviso
	rr, res := doJSONReq(app, "POST", "/api/avisos", map[string]any{
		"titulo": "Comunicado 0710", "conteudo": "<p>Ordem do dia</p>",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("publicar aviso esperado 200, veio %d (%v)", rr.Code, res)
	}
	if nf, ok := res["notificados"].(float64); !ok || int(nf) != 3 {
		t.Fatalf("esperado notificados=3 (op1, op2, chefe), veio %v", res["notificados"])
	}

	// 2. Cada destinatário (op1, op2, chefe) deve ter linha não lida — o sino conta daí
	// (a notificação é uma mensagem-sistema "📢 Aviso publicado: ..." — ver mensagens.go)
	var naoLidas int64
	for _, login := range []string{"op_av1", "op_av2", "chefe_av"} {
		var papelID int64
		if err := st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?`, login).Scan(&papelID); err != nil {
			t.Fatalf("papel de %s: %v", login, err)
		}
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM mensagem_destinatarios md JOIN mensagens m ON m.id = md.mensagem_id WHERE md.destinatario_papel_id = ? AND md.lida_em IS NULL AND m.assunto LIKE '📢 %'`, papelID).Scan(&naoLidas); err != nil {
			t.Fatalf("contar não lidas de %s: %v", login, err)
		}
		if naoLidas != 1 {
			t.Fatalf("%s deveria ter 1 notificação de aviso, veio %d", login, naoLidas)
		}
	}

	// 3. O AUTOR não se notifica
	var papelGer int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'ger_av')`).Scan(&papelGer); err != nil {
		t.Fatalf("papel do gerente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM mensagem_destinatarios WHERE destinatario_papel_id = ?`, papelGer).Scan(&naoLidas); err != nil {
		t.Fatalf("contar do autor: %v", err)
	}
	if naoLidas != 0 {
		t.Fatalf("autor não deveria ser notificado, veio %d", naoLidas)
	}

	// 4. aviso_pendentes do /api/notificacoes para o op1 = 1 (não deu ciente)
	ckOp := loginAs(t, app, "op_av1", "senha-o1")
	rrN, resN := doJSONReq(app, "GET", "/api/notificacoes", nil, ckOp)
	if rrN.Code != http.StatusOK {
		t.Fatalf("/api/notificacoes esperado 200, veio %d", rrN.Code)
	}
	if av, ok := resN["avisos_pendentes"].(float64); !ok || int(av) != 1 {
		t.Fatalf("avisos_pendentes esperado 1, veio %v", resN["avisos_pendentes"])
	}

	// 5. Dar ciente → zera para o op1
	rrC, resC := doJSONReq(app, "POST", "/api/avisos/1/ciente", map[string]any{}, ckOp)
	if rrC.Code != http.StatusOK {
		t.Fatalf("dar ciente esperado 200, veio %d (%v)", rrC.Code, resC)
	}
	rrN2, resN2 := doJSONReq(app, "GET", "/api/notificacoes", nil, ckOp)
	if rrN2.Code != http.StatusOK {
		t.Fatalf("/api/notificacoes (2ª) esperado 200, veio %d", rrN2.Code)
	}
	if av, ok := resN2["avisos_pendentes"].(float64); !ok || int(av) != 0 {
		t.Fatalf("após ciente, avisos_pendentes esperado 0, veio %v", resN2["avisos_pendentes"])
	}
}
