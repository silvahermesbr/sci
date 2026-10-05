package main

// ordem_diretor_pdf_test.go — extrairTextoPDF: extrator de texto para PDFs
// gerados pelo go-pdf/fpdf (fontes core, cp1252) usado nos testes da rodada
// "Alterações SCI" (ordem Diretor 04/10/26). Sem dependência nova: junta os
// streams FlateDecode, varre os operadores de texto (Tj/TJ/'/") e decodifica
// cp1252 → texto sem acentos, para asserções Contains estáveis.

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strings"
	"testing"
)

// cp1252ParaRuna decodifica um byte cp1252 (Latin-1 + glifos 0x80–0x9F Windows).
func cp1252ParaRuna(b byte) rune {
	switch b {
	case 0x80:
		return '€'
	case 0x82:
		return '‚'
	case 0x83:
		return 'ƒ'
	case 0x84:
		return '„'
	case 0x85:
		return '…'
	case 0x86:
		return '†'
	case 0x87:
		return '‡'
	case 0x88:
		return 'ˆ'
	case 0x89:
		return '‰'
	case 0x8A:
		return 'Š'
	case 0x8B:
		return '‹'
	case 0x8C:
		return 'Œ'
	case 0x8E:
		return 'Ž'
	case 0x91:
		return '\u2018'
	case 0x92:
		return '\u2019'
	case 0x93:
		return '\u201C'
	case 0x94:
		return '\u201D'
	case 0x95:
		return '•'
	case 0x96:
		return '–'
	case 0x97:
		return '—'
	case 0x98:
		return '˜'
	case 0x99:
		return '™'
	case 0x9A:
		return 'š'
	case 0x9B:
		return '›'
	case 0x9C:
		return 'œ'
	case 0x9E:
		return 'ž'
	case 0x9F:
		return 'Ÿ'
	default:
		return rune(b) // ASCII e Latin-1
	}
}

// semAcento: remove diacríticos PT-BR p/ comparação estável do texto extraído.
var semAcento = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"è", "e", "é", "e", "ê", "e", "ë", "e",
	"ì", "i", "í", "i", "î", "i", "ï", "i",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o",
	"ù", "u", "ú", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "ý", "y", "ÿ", "y",
	"À", "A", "Á", "A", "Â", "A", "Ã", "A", "Ä", "A", "Å", "A",
	"È", "E", "É", "E", "Ê", "E", "Ë", "E",
	"Ì", "I", "Í", "I", "Î", "I", "Ï", "I",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ù", "U", "Ú", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N", "Ý", "Y",
	"€", "EUR", "‘", "'", "’", "'", "“", "\"", "”", "\"",
	"–", "-", "—", "-", "•", "*", "Š", "S", "š", "s",
	"Œ", "OE", "œ", "oe", "Ž", "Z", "ž", "z", "Ÿ", "Y", "ƒ", "f",
)

// reStream localiza blocos stream..endstream (o dicionário anterior é ignorado —
// tentamos zlib e, se falhar, tratamos o bruto como texto plano).
var reStream = regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`)

// extrairTextoPDF devolve o texto visível do PDF, sem acentos.
func extrairTextoPDF(t *testing.T, pdf []byte) string {
	t.Helper()
	if len(pdf) == 0 || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("conteúdo não parece um PDF (%d bytes): %.40q", len(pdf), pdf)
	}
	var conteudo []byte
	for _, m := range reStream.FindAllSubmatch(pdf, -1) {
		bruto := m[1]
		zr, err := zlib.NewReader(bytes.NewReader(bruto))
		if err == nil {
			dados, errZ := io.ReadAll(zr)
			zr.Close()
			if errZ == nil {
				conteudo = append(conteudo, dados...)
				continue
			}
		}
		conteudo = append(conteudo, bruto...)
	}
	var sb strings.Builder
	for i := 0; i < len(conteudo); {
		c := conteudo[i]
		if c != '(' {
			i++
			continue
		}
		// string literal do PDF: parênteses balanceados com escapes
		prof := 1
		i++
		for i < len(conteudo) && prof > 0 {
			ch := conteudo[i]
			switch ch {
			case '\\':
				if i+1 >= len(conteudo) {
					i++
					break
				}
				esc := conteudo[i+1]
				switch esc {
				case 'n':
					sb.WriteByte('\n')
				case 'r':
					sb.WriteByte('\r')
				case 't':
					sb.WriteByte('\t')
				case 'b':
					sb.WriteByte('\b')
				case 'f':
					sb.WriteByte('\f')
				case '(', ')', '\\':
					sb.WriteRune(cp1252ParaRuna(esc))
				case '0', '1', '2', '3', '4', '5', '6', '7':
					oct := 0
					j := i + 1
					for j < len(conteudo) && j <= i+3 && conteudo[j] >= '0' && conteudo[j] <= '7' {
						oct = oct*8 + int(conteudo[j]-'0')
						j++
					}
					sb.WriteRune(cp1252ParaRuna(byte(oct)))
					i = j
					continue
				default:
					sb.WriteRune(cp1252ParaRuna(esc))
				}
				i += 2
			case '(':
				prof++
				i++
			case ')':
				prof--
				if prof > 0 {
					sb.WriteRune('(') // parêntese interno literal
				}
				i++
			default:
				sb.WriteRune(cp1252ParaRuna(ch))
				i++
			}
		}
		sb.WriteByte(' ') // separa células adjacentes
	}
	return semAcento.Replace(sb.String())
}
