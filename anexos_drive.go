package main

// Anexos com integração ao Drive (ordem Diretor 04/10):
//   - Anexo por REFERÊNCIA: {drive_arquivo_id, nome, tamanho, tipo} — aponta
//     para o arquivo que já está no drive do grupo; nada é duplicado.
//   - Anexo do COMPUTADOR: {dados_base64, ...} — legado mantido; o front envia
//     uma cópia para o drive e passa a referenciar (ver views).
// validarAnexoDrive confere existência e permissão de leitura do arquivo
// referenciado; devolve os metadados normalizados do anexo.

import (
	"net/http"
	"strconv"
)

// anexoDriveMeta: metadados normalizados de um anexo por referência.
func (a *App) anexoDriveMeta(u *Usuario, driveArquivoID int64) (map[string]any, bool) {
	ok, meta, err := a.checarAcessoArquivo(u, driveArquivoID, false)
	if err != nil || !ok || meta == nil {
		return nil, false
	}
	nome, _ := meta["nome_original"].(string)
	tipo, _ := meta["tipo"].(string)
	var tamanho int64
	switch v := meta["tamanho"].(type) {
	case int64:
		tamanho = v
	case int:
		tamanho = int64(v)
	}
	return map[string]any{
		"drive_arquivo_id": driveArquivoID,
		"nome":             nome,
		"tipo":             tipo,
		"tamanho":          tamanho,
	}, true
}

// normalizaAnexosDrive: para cada anexo do pedido, se trouxer drive_arquivo_id,
// valida permissão e substitui pelo objeto limpo (sem dados_base64). Anexos só
// com dados_base64 passam intactos (fluxo legado). Anexo de referência sem
// permissão = erro (400), nunca silencioso.
func (a *App) normalizaAnexosDrive(u *Usuario, anexos []map[string]any) ([]map[string]any, *erroAPI) {
	if len(anexos) == 0 {
		return anexos, nil
	}
	out := make([]map[string]any, 0, len(anexos))
	for _, an := range anexos {
		if an == nil {
			continue
		}
		if idv, tem := an["drive_arquivo_id"]; tem && idv != nil {
			var id int64
			switch v := idv.(type) {
			case float64:
				id = int64(v)
			case int64:
				id = v
			case string:
				id, _ = strconv.ParseInt(v, 10, 64)
			}
			if id <= 0 {
				return nil, &erroAPI{codigo: http.StatusBadRequest, mensagem: "drive_arquivo_id inválido"}
			}
			meta, ok := a.anexoDriveMeta(u, id)
			if !ok {
				return nil, &erroAPI{codigo: http.StatusBadRequest, mensagem: "anexo do drive inacessível ou inexistente"}
			}
			out = append(out, meta)
			continue
		}
		out = append(out, an)
	}
	return out, nil
}

// erroAPI: erro de negócio com código HTTP (usada pelos normalizadores).
type erroAPI struct {
	codigo   int
	mensagem string
}

// anexosDeRequest: converte o campo "anexos" (any do JSON) em lista tipada e
// aplica a normalização por referência ao drive. nil = sem anexos.
func anexosDeRequest(a *App, u *Usuario, raw any) ([]map[string]any, *erroAPI) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []any:
		if len(v) == 0 {
			return nil, nil
		}
		lista := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				lista = append(lista, m)
			}
		}
		return a.normalizaAnexosDrive(u, lista)
	case []map[string]any:
		if len(v) == 0 {
			return nil, nil
		}
		return a.normalizaAnexosDrive(u, v)
	default:
		return nil, nil
	}
}
