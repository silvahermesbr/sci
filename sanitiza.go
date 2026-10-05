package main

// Sanitizador server-side de rich text (fix P0 XSS da onda v1.3).
// O editor (montarRichEditor, web/core.js) emite SOMENTE: b, i, u, h2, h3, p, div,
// ul, ol, li, br e o atributo style="text-align:...". Este sanitizador aceita
// exatamente esse conjunto (allowlist) e remove tudo o mais; texto puro entre
// tags é SEMPRE re-escapado. Entrada pode conter entidades (o editor produz
// &amp; etc.): decodifica antes e re-escapa na saída — idempotente e estável.

import (
	"html"
	"regexp"
	"strings"
)

var espacosRe = regexp.MustCompile(`\s+`)

var tagRe = regexp.MustCompile(`(?i)<(/?)([a-zA-Z0-9]+)((?:[^>])*?)>`)
var styleAlignRe = regexp.MustCompile(`(?i)^\s*(?:style\s*=\s*")?\s*text-align\s*:\s*(left|center|right|justify)\s*;?\s*"?\s*$`)
var corRe = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3}([0-9a-fA-F]{2})?)?$`)

var tagsPermitidas = map[string]bool{
	"b": true, "i": true, "u": true, "br": true, "p": true, "div": true,
	"h2": true, "h3": true, "ul": true, "ol": true, "li": true,
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// corValida: allowlist de cor CSS (#rgb, #rrggbb, #rrggbbaa) — fix P2-3.
func corValida(c string) bool { return corRe.MatchString(strings.TrimSpace(c)) }

// sanitizaRichText devolve HTML seguro conforme a allowlist do editor.
func sanitizaRichText(in string) string {
	in = html.UnescapeString(in)
	var sb strings.Builder
	last := 0
	for _, loc := range tagRe.FindAllStringSubmatchIndex(in, -1) {
		sb.WriteString(escapeHTML(in[last:loc[0]]))
		last = loc[1]
		fecha := in[loc[2]:loc[3]] == "/"
		tag := strings.ToLower(in[loc[4]:loc[5]])
		attrs := strings.TrimSpace(in[loc[6]:loc[7]])
		if !tagsPermitidas[tag] {
			continue // tag fora da allowlist: some; o texto interno já foi escapado
		}
		if tag == "br" {
			if !fecha {
				sb.WriteString("<br>")
			}
			continue
		}
		if fecha {
			sb.WriteString("</" + tag + ">")
			continue
		}
		if m := styleAlignRe.FindStringSubmatch(attrs); m != nil {
			sb.WriteString("<" + tag + ` style="text-align:` + strings.ToLower(m[1]) + `">`)
			continue
		}
		sb.WriteString("<" + tag + ">")
	}
	sb.WriteString(escapeHTML(in[last:]))
	return sb.String()
}
// remediarAssunto (ordem Diretor 04/10): o título da mensagem às vezes chega
// com marcações do editor ("<strong>assunto</strong>", entidades, quebras).
// Assunto é TÍTULO: vira texto puro — tags removidas (o texto interno fica),
// entidades decodificadas, espaços/quebras colapsados.
func remediarAssunto(in string) string {
	out := tagRe.ReplaceAllString(in, " ")
	out = html.UnescapeString(out)
	out = espacosRe.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}
