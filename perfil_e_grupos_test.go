package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
)

func TestPerfilUsuarioCompletoEFoto(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar um usuário operador de teste
	_, respAdd := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "militar.silva",
		"senha": "password123",
		"papel": "operador",
	}, adminCookie)
	uid := int64(respAdd["id"].(float64))

	userCookie := loginAs(t, app, "militar.silva", "password123")

	// 2. Atualizar perfil próprio com todos os novos campos e foto 1x1
	dummyJPEG := base64.StdEncoding.EncodeToString([]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01\x01\x01\x00`\x00`\x00\x00\xFF\xDB\x00C\x00\xFF\xD9"))
	fotoDataURI := "data:image/jpeg;base64," + dummyJPEG

	rrPatch, respPatch := doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":     "SILVA",
		"nome_completo":   "Carlos Silva de Oliveira",
		"data_nascimento": "1994-08-25",
		"tipo_sanguineo":  "O+",
		"telefone":        "(11) 98765-4321",
		"email":           "silva@eb.mil.br",
		"endereco":        "Av. Duque de Caxias, 1000 - Vila Militar",
		"foto_base64":     fotoDataURI,
	}, userCookie)

	if rrPatch.Code != http.StatusOK || respPatch["ok"] != true {
		t.Fatalf("falha ao atualizar perfil: status %d resp: %v", rrPatch.Code, respPatch)
	}

	// 3. Consultar /api/perfil e validar integridade dos dados
	rrGet, respGet := doJSONReq(app, "GET", "/api/perfil", nil, userCookie)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("falha ao obter perfil: status %d", rrGet.Code)
	}

	uMap, ok := respGet["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("campo usuario ausente na resposta de /api/perfil")
	}

	if uMap["nome_guerra"] != "SILVA" || uMap["nome_completo"] != "Carlos Silva de Oliveira" {
		t.Errorf("nomes não conferem: %v", uMap)
	}
	if uMap["data_nascimento"] != "1994-08-25" {
		t.Errorf("data_nascimento esperada 1994-08-25, obtida: %v", uMap["data_nascimento"])
	}
	if uMap["tipo_sanguineo"] != "O+" {
		t.Errorf("tipo_sanguineo esperado O+, obtido: %v", uMap["tipo_sanguineo"])
	}
	if uMap["telefone"] != "(11) 98765-4321" {
		t.Errorf("telefone esperado (11) 98765-4321, obtido: %v", uMap["telefone"])
	}
	if uMap["email"] != "silva@eb.mil.br" {
		t.Errorf("email esperado silva@eb.mil.br, obtido: %v", uMap["email"])
	}
	if uMap["endereco"] != "Av. Duque de Caxias, 1000 - Vila Militar" {
		t.Errorf("endereco esperado Av. Duque de Caxias, 1000 - Vila Militar, obtido: %v", uMap["endereco"])
	}
	if uMap["foto_base64"] != fotoDataURI {
		t.Errorf("foto_base64 não confere")
	}

	// 4. Testar endpoint de foto direta /api/usuarios/{id}/foto
	rrFoto, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uid), nil, userCookie)
	if rrFoto.Code != http.StatusOK {
		t.Fatalf("status inesperado para /api/usuarios/%d/foto: %d", uid, rrFoto.Code)
	}
	if ct := rrFoto.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("esperado Content-Type image/jpeg, obtido: %s", ct)
	}
}

func TestCriacaoGrupoSimplificadaEAtribuicaoGerente(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo simplificado apenas com o nome (sem login/senha/gerente)
	rrGrupo, respGrupo := doJSONReq(app, "POST", "/api/grupos", map[string]string{
		"nome": "Pelotão Especial Alpha",
	}, adminCookie)

	if rrGrupo.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo simplificado: code %d resp %v", rrGrupo.Code, respGrupo)
	}
	gid := int64(respGrupo["id"].(float64))
	cod := respGrupo["codigo"].(string)
	if gid <= 0 || len(cod) != 6 {
		t.Fatalf("id ou código de grupo inválidos: id=%d cod=%s", gid, cod)
	}

	// 2. Verificar que o grupo nasce sem gerente
	rrGer0, respGer0 := doJSONReq(app, "GET", fmt.Sprintf("/api/grupos/%d/gerente", gid), nil, adminCookie)
	if rrGer0.Code != http.StatusOK || respGer0["gerente"] != "" {
		t.Fatalf("grupo deveria nascer sem gerente: %v", respGer0)
	}

	// 3. Criar usuário e movê-lo para o novo grupo
	_, respU1 := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "militar.alpha1",
		"senha": "password123",
		"papel": "operador",
	}, adminCookie)
	uid1 := int64(respU1["id"].(float64))

	_, _ = doJSONReq(app, "PATCH", fmt.Sprintf("/api/usuarios/%d/mover", uid1), map[string]any{
		"grupo_id": gid,
	}, adminCookie)

	// 4. Promover militar.alpha1 a Gerente do grupo (herança de função)
	rrTroca1, respTroca1 := doJSONReq(app, "POST", fmt.Sprintf("/api/grupos/%d/trocar-gerente", gid), map[string]string{
		"login": "militar.alpha1",
	}, adminCookie)

	if rrTroca1.Code != http.StatusOK || respTroca1["gerente"] != "militar.alpha1" {
		t.Fatalf("falha ao promover militar.alpha1 a gerente: code %d resp %v", rrTroca1.Code, respTroca1)
	}

	// Verificar se grupo agora tem gerente
	_, respGer1 := doJSONReq(app, "GET", fmt.Sprintf("/api/grupos/%d/gerente", gid), nil, adminCookie)
	if respGer1["gerente"] != "militar.alpha1" {
		t.Errorf("gerente esperado militar.alpha1, obtido: %v", respGer1["gerente"])
	}

	// 5. Adicionar segundo usuário ao grupo e promovê-lo a Gerente
	_, respU2 := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "militar.alpha2",
		"senha": "password123",
		"papel": "operador",
	}, adminCookie)
	uid2 := int64(respU2["id"].(float64))

	_, _ = doJSONReq(app, "PATCH", fmt.Sprintf("/api/usuarios/%d/mover", uid2), map[string]any{
		"grupo_id": gid,
	}, adminCookie)

	rrTroca2, respTroca2 := doJSONReq(app, "POST", fmt.Sprintf("/api/grupos/%d/trocar-gerente", gid), map[string]string{
		"login": "militar.alpha2",
	}, adminCookie)
	if rrTroca2.Code != http.StatusOK || respTroca2["gerente"] != "militar.alpha2" {
		t.Fatalf("falha ao promover militar.alpha2 a gerente: %v", respTroca2)
	}

	// 6. Validar que militar.alpha1 virou operador (regra de 1 gerente titular)
	var papel1, papel2 string
	_ = app.st.db.QueryRow(`SELECT papel FROM usuarios WHERE id = ?`, uid1).Scan(&papel1)
	_ = app.st.db.QueryRow(`SELECT papel FROM usuarios WHERE id = ?`, uid2).Scan(&papel2)

	if papel1 != "operador" {
		t.Errorf("militar.alpha1 deveria ter sido rebaixado a operador, mas está com papel: %s", papel1)
	}
	if papel2 != "gerente" {
		t.Errorf("militar.alpha2 deveria ser gerente, mas está com papel: %s", papel2)
	}
}
