package main

// Relatório A4 — go-pdf/fpdf, 100% Go, gráficos desenhados nativamente
// (barras e barras de proporção — zero dependência extra, footprint mínimo).

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

type ConferenciaPDF struct {
	ID          int64            `json:"id"`
	Data        string           `json:"data"`
	Status      string           `json:"status"`
	Hora        *string          `json:"hora"`
	CriadaEm    string           `json:"criada_em"`
	FechadaEm   *string          `json:"fechada_em"`
	CriadoPor   string           `json:"criado_por"`
	GeradoPor   string           `json:"gerado_por"`
	Resumo      map[string]int   `json:"resumo"`
	Lancamentos []map[string]any `json:"lancamentos"`
}

// cp1252Traduz converte UTF-8 PT-BR para bytes cp1252 (fontes core do fpdf).
var cp1252Traduz = strings.NewReplacer(
	"à", "\xe0", "á", "\xe1", "â", "\xe2", "ã", "\xe3", "ç", "\xe7",
	"é", "\xe9", "ê", "\xea", "í", "\xed", "ó", "\xf3", "ô", "\xf4",
	"õ", "\xf5", "ú", "\xfa", "º", "\xba", "ª", "\xaa", "°", "\xb0",
	"ü", "\xfc", "ö", "\xf6", "ä", "\xe4", "ï", "\xef", "ì", "\xec",
	"ò", "\xf2", "ù", "\xf9", "ñ", "\xf1", "ÿ", "\xff",
	"À", "\xc0", "Á", "\xc1", "Â", "\xc2", "Ã", "\xc3", "Ç", "\xc7",
	"É", "\xc9", "Ê", "\xca", "Í", "\xcd", "Ó", "\xd3", "Ô", "\xd4",
	"Õ", "\xd5", "Ú", "\xda", "Ü", "\xdc", "Ñ", "\xf1",
	"–", "-", "—", "-", "·", "\xb7", "“", "\"", "”", "\"", "’", "'", "‘", "'",
)

type PessoaStat struct {
	ID           int64   `json:"id"`
	NomeGuerra   string  `json:"nome_guerra"`
	Setor        string  `json:"setor"`
	Funcao       string  `json:"funcao"`
	FuncaoID     *int64  `json:"funcao_id"`
	Antiguidade  int     `json:"antiguidade"`
	Grupo        string  `json:"grupo"`
	Presencas    int     `json:"presencas"`
	Atrasos      int     `json:"atrasos"`
	Faltas       int     `json:"faltas"`
	Justificadas int     `json:"justificadas"`
	Lancados     int     `json:"lancados"`
	Pct          float64 `json:"pct"`
}

type SetorStat struct {
	Setor        string  `json:"setor"`
	Presencas    int     `json:"presencas"`
	Faltas       int     `json:"faltas"`
	Justificadas int     `json:"justificadas"`
	Lancados     int     `json:"lancados"`
	Pct          float64 `json:"pct"`
}

type DestinoStat struct {
	Destino    string `json:"destino"`
	Quantidade int    `json:"quantidade"`
}

type FormaturaStat struct {
	Data      string `json:"data"`
	Tipo      string `json:"tipo"`
	Hora      string `json:"hora"`
	Status    string `json:"status"`
	Presentes int    `json:"presentes"`
	Faltas    int    `json:"faltas"`
}

type Bundle struct {
	De, Ate            string          `json:"-"`
	Convocacoes        int             `json:"convocacoes"`
	EfetivoAtivo       int             `json:"efetivo_ativo"`
	TotalLanc          int             `json:"total_lancamentos"`
	Presentes          int             `json:"presentes"`
	PresentesPuros     int             `json:"presentes_puros"` // presentes SEM ressalva (ordem Tenente 28/09)
	Atrasos            int             `json:"atrasos"`
	Faltas             int             `json:"falta"`
	Justificadas       int             `json:"justificadas"`
	PctGeral           float64         `json:"pct_geral"`
	PctPronto          float64         `json:"pct_pronto"`           // % do efetivo pronto = presentes puros / convocações
	PctPresencaEstrita float64         `json:"pct_presenca_estrita"` // decisão 28/09: justificada = falta
	TotalFaltas        int             `json:"total_faltas"`         // falta + justificada (decisão 28/09)
	PorSetor           []SetorStat     `json:"por_setor"`
	PorDestino         []DestinoStat   `json:"por_destino"`
	Pessoas            []PessoaStat    `json:"pessoas"`
	Formaturas         []FormaturaStat `json:"formaturas"`
}

var (
	// Preto e branco no A4 (ordem Tenente 28/09) — v9.11: tabelas e textos P&B;
	// gráficos/proporções COLORIDOS (ordem Tenente 29/09).
	verdeR, verdeG, verdeB = 15, 15, 15
	verdeClaro             = [3]int{235, 235, 235}
	// v9.11 (ordem Tenente 29/09): GRÁFICOS E PROPORÇÕES COLORIDOS; tabelas e textos P&B.
	corPresente = [3]int{67, 160, 71}  // verde
	corAtraso   = [3]int{251, 176, 52} // âmbar
	corFalta    = [3]int{198, 40, 40}  // vermelho
	corJust     = [3]int{69, 90, 100}  // azul-acinzentado
	corBarra    = [3]int{27, 94, 32}   // barras de setor/destino (verde-militar)
	corDestino  = [3]int{21, 101, 192} // barras de destino (azul)
)

func (a *App) gerarRelatorioPDF(b Bundle) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 14, 15)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()
	T := cp1252Traduz.Replace

	// ---- cabeçalho ----
	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetTextColor(verdeR, verdeG, verdeB)
	pdf.Cell(0, 9, T(a.omTitulo))
	pdf.Ln(9)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 90, 90)
	pdf.Cell(0, 5, T(fmt.Sprintf("Período: %s a %s  ·  Emitido em %s",
		b.De, b.Ate, time.Now().In(a.horaLocal).Format("02/01/2006 15:04"))))
	// % EFETIVO PRONTO no lado direito (ordem Tenente 28/09): só presentes sem ressalva
	pdf.SetXY(122, pdf.GetY()-0.8)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(73, 6.5, T(fmt.Sprintf("EFETIVO PRONTO: %.1f%%", b.PctPronto)))
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(90, 90, 90)
	pdf.Cell(0, 4.5, T("USO INTERNO"))
	pdf.Ln(7.5)
	pdf.SetDrawColor(verdeR, verdeG, verdeB)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(5)

	// ---- resumo do período ----
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetTextColor(verdeR, verdeG, verdeB)
	pdf.Cell(0, 6, T("RESUMO DO PERÍODO"))
	pdf.Ln(7)
	pctStr := fmt.Sprintf("%.1f%%", b.PctGeral)
	pctProntoStr := fmt.Sprintf("%.1f%%", b.PctPronto)
	caixas := [][2]string{
		{"CONVOCACOES", strconv.Itoa(b.Convocacoes)},
		{"EFETIVO ATIVO", strconv.Itoa(b.EfetivoAtivo)},
		{"PRESENTE", strconv.Itoa(b.Presentes)},
		{"ATRASO", strconv.Itoa(b.Atrasos)},
		{"FALTA", strconv.Itoa(b.Faltas)},
		{"JUSTIFICADA", strconv.Itoa(b.Justificadas)},
		{"% VÁLIDAS", pctStr},
		{"% EF.PRONTO", pctProntoStr},
	}
	pdf.SetFont("Helvetica", "", 7)
	y0 := pdf.GetY()
	for i, c := range caixas {
		x := 15 + float64(i)*22
		rot := T(c[0])
		if len(rot) > 13 {
			rot = rot[:13]
		}
		pdf.SetXY(x, y0)
		pdf.CellFormat(22, 5, rot, "1", 0, "C", false, 0, "")
		pdf.SetXY(x, y0+5)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(22, 8, c[1], "1", 0, "C", false, 0, "")
		pdf.SetFont("Helvetica", "", 7)
	}
	pdf.SetY(y0 + 13)
	// decisão Tenente 28/09: justificada = falta — nota com o total
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(60, 60, 60)
	pdf.Cell(0, 4.5, T(fmt.Sprintf("Justificada conta como falta — total de faltas (justificadas + não justificadas): %d", b.TotalFaltas)))
	pdf.Ln(7)

	// ---- proporção de situações (barra segmentada) ----
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetTextColor(verdeR, verdeG, verdeB)
	pdf.Cell(0, 6, T("PROPORÇÃO DE SITUAÇÕES"))
	pdf.Ln(7)
	if b.TotalLanc > 0 {
		segmentos := [][2]any{{"Presente", corPresente}, {"Atraso", corAtraso},
			{"Falta", corFalta}, {"Justificada", corJust}}
		valores := []int{b.Presentes, b.Atrasos, b.Faltas, b.Justificadas}
		x := 15.0
		for i, v := range valores {
			larg := 180 * float64(v) / float64(b.TotalLanc)
			if larg <= 0 {
				continue
			}
			c := segmentos[i][1].([3]int)
			pdf.SetFillColor(c[0], c[1], c[2])
			pdf.Rect(x, pdf.GetY(), larg, 6, "F")
			x += larg
		}
		pdf.Ln(9)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(60, 60, 60)
		for i, seg := range segmentos {
			c := seg[1].([3]int)
			xs := 15 + float64(i)*45
			pdf.SetFillColor(c[0], c[1], c[2])
			pdf.Rect(xs, pdf.GetY(), 3, 3, "F")
			pdf.SetXY(xs+4.5, pdf.GetY()-0.8)
			pdf.Cell(40, 4, T(fmt.Sprintf("%s (%d)", seg[0], valores[i])))
		}
		pdf.Ln(8)
	} else {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.Cell(0, 6, T("Sem lançamentos no período."))
		pdf.Ln(9)
	}

	// ---- barras: presença por setor ----
	if len(b.PorSetor) > 0 {
		pdf.SetFont("Helvetica", "B", 9.5)
		pdf.SetTextColor(verdeR, verdeG, verdeB)
		pdf.Cell(0, 6, T("VALIDEZ POR SETOR (P+A)"))
		pdf.Ln(7)
		pdf.SetFont("Helvetica", "", 8)
		for _, s := range b.PorSetor {
			if pdf.GetY() > 250 {
				pdf.AddPage()
			}
			pdf.SetTextColor(40, 40, 40)
			nome := s.Setor
			if len(nome) > 28 {
				nome = nome[:28]
			}
			pdf.Cell(40, 6, T(nome))
			pdf.SetFillColor(verdeR, verdeG, verdeB)
			larg := 110 * s.Pct / 100
			if larg < 0.4 && s.Lancados > 0 {
				larg = 0.4
			}
			pdf.Rect(57, pdf.GetY(), larg, 4.4, "F")
			pdf.SetTextColor(40, 40, 40)
			pdf.SetXY(170, pdf.GetY())
			pdf.Cell(25, 6, T(fmt.Sprintf("%.1f%% (%d/%d)", s.Pct, s.Presencas, s.Lancados)))
			pdf.Ln(7)
		}
		pdf.Ln(3)
	}

	// ---- faltas por destino ----
	if len(b.PorDestino) > 0 {
		if pdf.GetY() > 215 {
			pdf.AddPage()
		}
		pdf.SetFont("Helvetica", "B", 9.5)
		pdf.SetTextColor(verdeR, verdeG, verdeB)
		pdf.Cell(0, 6, T("FALTAS/JUSTIFICADAS POR DESTINO"))
		pdf.Ln(7)
		max := b.PorDestino[0].Quantidade
		if max == 0 {
			max = 1
		}
		pdf.SetFont("Helvetica", "", 8)
		for _, d := range b.PorDestino {
			if pdf.GetY() > 255 {
				pdf.AddPage()
			}
			pdf.SetTextColor(40, 40, 40)
			nome := d.Destino
			if len(nome) > 28 {
				nome = nome[:28]
			}
			pdf.Cell(40, 6, T(nome))
			pdf.SetFillColor(corFalta[0], corFalta[1], corFalta[2])
			pdf.Rect(57, pdf.GetY(), 110*float64(d.Quantidade)/float64(max), 4.4, "F")
			pdf.SetXY(170, pdf.GetY())
			pdf.Cell(25, 6, strconv.Itoa(d.Quantidade))
			pdf.Ln(7)
		}
		pdf.Ln(3)
	}

	// ---- tabela por pessoa ----
	if pdf.GetY() > 190 {
		pdf.AddPage()
	}
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetTextColor(verdeR, verdeG, verdeB)
	pdf.Cell(0, 6, T("EFETIVO — POR ANTIGUIDADE DE FUNÇÃO (ID menor = mais antigo)"))
	pdf.Ln(7)
	cab := []string{"Nome de guerra", "Função (ID)", "Setor", "Grupo", "Pres.", "Atraso", "Falta", "Just."}
	larg := []float64{36, 38, 26, 28, 15, 15, 15, 15}
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(verdeR, verdeG, verdeB)
	pdf.SetTextColor(255, 255, 255)
	for i, h := range cab {
		align := "L"
		if i >= 2 {
			align = "C"
		}
		pdf.CellFormat(larg[i], 6, T(h), "1", 0, align, true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 8)
	zebra := false
	for _, p := range b.Pessoas {
		if pdf.GetY() > 275 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "B", 8)
			pdf.SetFillColor(verdeR, verdeG, verdeB)
			pdf.SetTextColor(255, 255, 255)
			for i, h := range cab {
				align := "L"
				if i >= 2 {
					align = "C"
				}
				pdf.CellFormat(larg[i], 6, T(h), "1", 0, align, true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Helvetica", "", 8)
		}
		// zebra: alterna e SEMPRE define a cor antes de desenhar
		zebra = !zebra
		if zebra {
			pdf.SetFillColor(242, 247, 244)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		pdf.SetTextColor(30, 30, 30)
		fnCell := "—"
		if p.Funcao != "" {
			fnCell = T(p.Funcao) + " (#" + strconv.FormatInt(*p.FuncaoID, 10) + ")"
		} else if p.FuncaoID != nil {
			fnCell = "(#" + strconv.FormatInt(*p.FuncaoID, 10) + ")"
		}
		vals := []string{
			T(p.NomeGuerra), fnCell, T(p.Setor), T(p.Grupo),
			strconv.Itoa(p.Presencas), strconv.Itoa(p.Atrasos),
			strconv.Itoa(p.Faltas), strconv.Itoa(p.Justificadas),
		}
		for i, v := range vals {
			align := "L"
			if i >= 2 {
				align = "C"
			}
			fill := zebra
			pdf.CellFormat(larg[i], 5.6, v, "1", 0, align, fill, 0, "")
		}
		pdf.Ln(-1)
	}

	// ---- rodapé ----
	pdf.SetY(-14)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(120, 120, 120)
	pdf.Cell(0, 5, T("SCI — Sistema de Controle Interno · documento gerado automaticamente · dados-fonte: sci.db"))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gerarConferenciaPDF: relatório PRÓPRIO de uma conferência — só para conferências FECHADAS.
// Contém: horário de fechamento da conferência e horário de geração + operador.
func (a *App) gerarConferenciaPDF(c ConferenciaPDF) ([]byte, error) {
	T := cp1252Traduz.Replace
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 14, 15)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetTextColor(15, 15, 15)
	pdf.Cell(0, 9, T(a.omTitulo))
	pdf.Ln(9)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 90, 90)
	pdf.Cell(0, 5, T(fmt.Sprintf("Relatório de conferência de pessoal · %s · %s", c.Data, a.omTitulo)))
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(40, 40, 40)
	pdf.Cell(0, 5, T(fmt.Sprintf("Iniciada por %s em %s", c.CriadoPor, fmtDataBR(c.CriadaEm))))
	if c.FechadaEm != nil {
		pdf.SetX(95)
		pdf.Cell(0, 5, T(fmt.Sprintf("Finalizada em %s", fmtDataBR(*c.FechadaEm))))
	}
	pdf.Ln(5.5)
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(40, 40, 40)
	pdf.Cell(0, 5, T(fmt.Sprintf("Relatório gerado por %s em %s", c.GeradoPor,
		time.Now().In(a.horaLocal).Format("02/01/2006 15:04:05"))))
	pdf.Ln(7)
	pdf.SetDrawColor(15, 15, 15)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(5)

	presentes := c.Resumo["presentes"]
	atrasos := c.Resumo["atrasos"]
	faltas := c.Resumo["faltas"]
	just := c.Resumo["justificadas"]
	total := presentes + atrasos + faltas + just
	caixas := [][2]string{
		{"LANÇADOS", strconv.Itoa(total)},
		{"PRESENTES", strconv.Itoa(presentes)},
		{"ATRASOS", strconv.Itoa(atrasos)},
		{"FALTAS", strconv.Itoa(faltas)},
		{"JUSTIFICADAS", strconv.Itoa(just)},
	}
	if total > 0 {
		// FIX S4-P2 (verif5): uniformiza métrica — "presença" em TODO o documento
		// conta presentes puros; presente+atraso vira "% VÁLIDAS (presente+atraso)".
		caixas = append(caixas, [2]string{"% VÁLIDAS (P+A)", fmt.Sprintf("%.1f%%", 100*float64(presentes+atrasos)/float64(total))})
	} else {
		caixas = append(caixas, [2]string{"% VÁLIDAS (P+A)", "—"})
	}
	pdf.SetFont("Helvetica", "", 7)
	y0 := pdf.GetY()
	for i, cx := range caixas {
		x := 15 + float64(i)*26
		pdf.SetXY(x, y0)
		pdf.CellFormat(25, 5, T(cx[0]), "1", 0, "C", false, 0, "")
		pdf.SetXY(x, y0+5)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(25, 8, cx[1], "1", 0, "C", false, 0, "")
		pdf.SetFont("Helvetica", "", 7)
	}
	pdf.SetY(y0 + 13)

	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetTextColor(15, 15, 15)
	pdf.Cell(0, 6, T("LANÇAMENTOS DA CONFERÊNCIA"))
	pdf.Ln(7)
	cab := []string{"Nome de guerra", "Setor", "Situação", "Destino", "Observação", "Por"}
	larg := []float64{34, 30, 24, 30, 51, 21}
	pdf.SetFont("Helvetica", "B", 7.6)
	pdf.SetFillColor(15, 15, 15)
	pdf.SetTextColor(255, 255, 255)
	for i, hh := range cab {
		pdf.CellFormat(larg[i], 5.6, T(hh), "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 7.6)
	zebra := false
	for _, l := range c.Lancamentos {
		if pdf.GetY() > 272 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "B", 7.6)
			pdf.SetFillColor(15, 15, 15)
			pdf.SetTextColor(255, 255, 255)
			for i, hh := range cab {
				pdf.CellFormat(larg[i], 5.6, T(hh), "1", 0, "L", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Helvetica", "", 7.6)
		}
		zebra = !zebra
		if zebra {
			pdf.SetFillColor(240, 240, 240)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		pdf.SetTextColor(30, 30, 30)
		vals := []string{
			T(str(l["nome_guerra"])), T(str(l["setor"])), T(str(l["situacao"])),
			T(str(l["destino"])), T(str(l["observacao"])), T(str(l["marcado_por"])),
		}
		for i, v := range vals {
			pdf.CellFormat(larg[i], 5.2, v, "1", 0, "L", zebra, 0, "")
		}
		pdf.Ln(-1)
	}
	pdf.SetY(-14)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(120, 120, 120)
	pdf.Cell(0, 5, T("SCI — relatório de conferência de pessoal · documento gerado automaticamente"))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// fmtDataBR: 2026-09-28T15:04:05Z (ou local) -> 28/09/2026 15:04
func fmtDataBR(iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.Format("02/01/2006 15:04")
	}
	if len(iso) >= 16 {
		return iso[:10] + " " + iso[11:16]
	}
	return iso
}
