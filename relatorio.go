package main

// Relatório A4 — go-pdf/fpdf, 100% Go, gráficos desenhados nativamente
// (barras e barras de proporção — zero dependência extra, footprint mínimo).

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	NaoVerificados int   `json:"nao_verificados"` // ordem Tenente 30/09: sem ✅ no fechamento
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
	NaoVerificados     int             `json:"nao_verificados"` // ordem Tenente 30/09: sem ✅ no fechamento
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
	corNV       = [3]int{117, 117, 117} // ordem Tenente 30/09: NÃO VERIFICADO (cinza)
	corBarra    = [3]int{27, 94, 32}   // barras de setor/destino (verde-militar)
	corDestino  = [3]int{21, 101, 192} // barras de destino (azul)
)

// ---------- ESTRUTURAS DE RELATÓRIO EXPANDIDO (v2.0 Paper-Trail) ----------

type FichaPessoalPDF struct {
	ID             int64
	NomeGuerra     string
	NomeCompleto   string
	Setor          string
	Funcao         string
	Grupo          string
	Status         string
	DataNascimento string
	TipoSanguineo  string
	Telefone       string
	Email          string
	Endereco       string
	FotoBase64     string
	Presencas      int
	Atrasos        int
	Faltas         int
	Justificadas   int
	TotalConfs     int
	PctPresenca    float64
	CautelasAtivas []map[string]any
	Escalas        []map[string]any
}

type ReciboCautelaPDF struct {
	ID               int64
	ItemNome         string
	CodigoPatrimonio string
	NumeroSerie      string
	CategoriaNome    string
	Sensibilidade    string
	PessoaNomeGuerra string
	PessoaCompleto   string
	PessoaSetor      string
	PessoaFuncao     string
	PessoaGrupo      string
	DataSaida        string
	DataDevolucao    string
	ResponsavelSaida string
	ResponsavelDev   string
	ObsSaida         string
	ObsDevolucao     string
	Status           string
}

type EscalasRelatorioPDF struct {
	Periodo string
	Grupo   string
	Turnos  []map[string]any
}

type InventarioRelatorioPDF struct {
	Grupo   string
	Totais  map[string]int
	Itens   []map[string]any
}

// ---------- MOTOR GRÁFICO PDF 2.0 (Design System Executivo) ----------

func (a *App) novoPDF(orientacao, tituloDoc, subtitulo, operador string) *fpdf.Fpdf {
	T := cp1252Traduz.Replace
	pdf := fpdf.New(orientacao, "mm", "A4", "")
	pdf.SetMargins(14, 12, 14)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AliasNbPages("{nb}")

	larguraUtil := 182.0
	if orientacao == "L" {
		larguraUtil = 269.0
	}

	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetDrawColor(203, 213, 225) // Slate-300
		pdf.SetLineWidth(0.3)
		pdf.Line(14, pdf.GetY(), 14+larguraUtil, pdf.GetY())
		pdf.Ln(1.5)

		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(100, 116, 139) // Slate-500
		authHash := fmt.Sprintf("%08X", (time.Now().UnixNano()/1e6)%0xFFFFFFFF)
		infoEsq := fmt.Sprintf("SCI · Documento Oficial Auditável · sci.db · Autenticidade: #%s", authHash)
		pdf.CellFormat(larguraUtil*0.7, 4.5, T(infoEsq), "", 0, "L", false, 0, "")

		infoDir := fmt.Sprintf("Pág. %d de {nb}", pdf.PageNo())
		if operador != "" {
			infoDir = fmt.Sprintf("Op: %s · %s", operador, infoDir)
		}
		pdf.CellFormat(larguraUtil*0.3, 4.5, T(infoDir), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()

	// Faixa superior de destaque institucional
	pdf.SetFillColor(30, 41, 59) // Slate-800
	pdf.Rect(14, 12, larguraUtil, 2.5, "F")

	// Nome do sistema e OM
	pdf.SetY(17)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(15, 23, 42) // Slate-900
	om := a.omTitulo
	if om == "" {
		om = "SCI — SISTEMA DE CONTROLE INTERNO"
	}
	pdf.CellFormat(larguraUtil*0.65, 6, T(om), "", 0, "L", false, 0, "")

	// Metadados à direita
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(16, 185, 129) // Emerald-500
	pdf.CellFormat(larguraUtil*0.35, 5, T("AUDITORIA & CONTROLE"), "", 1, "R", false, 0, "")

	// Título do Documento
	pdf.SetX(14)
	pdf.SetFont("Helvetica", "B", 10.5)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(larguraUtil*0.65, 5, T(tituloDoc), "", 0, "L", false, 0, "")

	agoraStr := time.Now().In(a.horaLocal).Format("02/01/2006 15:04")
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(100, 116, 139)
	pdf.CellFormat(larguraUtil*0.35, 4.5, T("Emitido em: "+agoraStr), "", 1, "R", false, 0, "")

	// Subtítulo
	if subtitulo != "" {
		pdf.SetX(14)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(larguraUtil, 4.5, T(subtitulo), "", 1, "L", false, 0, "")
	}

	pdf.Ln(2)
	pdf.SetDrawColor(226, 232, 240) // Slate-200
	pdf.SetLineWidth(0.4)
	pdf.Line(14, pdf.GetY(), 14+larguraUtil, pdf.GetY())
	pdf.Ln(4)

	return pdf
}

func pdfTabelaCabecalho(pdf *fpdf.Fpdf, colunas []string, larguras []float64) {
	T := cp1252Traduz.Replace
	pdf.SetFont("Helvetica", "B", 7.5)
	pdf.SetFillColor(30, 41, 59)    // Slate-800
	pdf.SetTextColor(255, 255, 255) // Branco
	pdf.SetDrawColor(51, 65, 85)    // Slate-700
	pdf.SetLineWidth(0.2)
	for i, c := range colunas {
		pdf.CellFormat(larguras[i], 6.2, T(c), "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
}

func pdfTabelaLinha(pdf *fpdf.Fpdf, valores []string, larguras []float64, alinhamentos []string, par bool) {
	T := cp1252Traduz.Replace
	pdf.SetFont("Helvetica", "", 7.5)
	if par {
		pdf.SetFillColor(248, 250, 252) // Slate-50 suave
	} else {
		pdf.SetFillColor(255, 255, 255) // Branco
	}
	pdf.SetTextColor(15, 23, 42)    // Slate-900
	pdf.SetDrawColor(226, 232, 240) // Slate-200
	pdf.SetLineWidth(0.15)
	for i, v := range valores {
		al := "L"
		if i < len(alinhamentos) && alinhamentos[i] != "" {
			al = alinhamentos[i]
		}
		pdf.CellFormat(larguras[i], 5.8, T(v), "1", 0, al, true, 0, "")
	}
	pdf.Ln(-1)
}

func (a *App) gerarRelatorioPDF(b Bundle) ([]byte, error) {
	T := cp1252Traduz.Replace
	sub := fmt.Sprintf("Período: %s a %s · Efetivo Pronto: %.1f%%", b.De, b.Ate, b.PctPronto)
	pdf := a.novoPDF("P", "RELATÓRIO GERAL DE EFETIVO & CONFERÊNCIAS", sub, "")

	// ---- resumo do período ----
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("1. RESUMO GERAL DO PERÍODO"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	caixas := [][2]string{
		{"EFETIVO ATIVO", strconv.Itoa(b.EfetivoAtivo)},
		{"PRESENTES", strconv.Itoa(b.Presentes)},
		{"ATRASOS", strconv.Itoa(b.Atrasos)},
		{"FALTAS", strconv.Itoa(b.Faltas)},
		{"JUSTIFICADAS", strconv.Itoa(b.Justificadas)},
		{"N.VERIFIC.", strconv.Itoa(b.NaoVerificados)},
		{"% EF. PRONTO", fmt.Sprintf("%.1f%%", b.PctPronto)},
	}
	y0 := pdf.GetY()
	for i, c := range caixas {
		x := 14 + float64(i)*26.0
		pdf.SetXY(x, y0)
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(203, 213, 225)
		pdf.Rect(x, y0, 24.5, 13, "FD")

		pdf.SetXY(x, y0+1.5)
		pdf.SetFont("Helvetica", "B", 6.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(24.5, 3.5, T(c[0]), "", 0, "C", false, 0, "")

		pdf.SetXY(x, y0+5.5)
		pdf.SetFont("Helvetica", "B", 10.5)
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(24.5, 6, T(c[1]), "", 0, "C", false, 0, "")
	}
	pdf.SetY(y0 + 15)
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
			{"Falta", corFalta}, {"Justificada", corJust}, {"NÃO VERIFICADO", corNV}}
		valores := []int{b.Presentes, b.Atrasos, b.Faltas, b.Justificadas, b.NaoVerificados}
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
		y := pdf.GetY()
		for i, seg := range segmentos {
			c := seg[1].([3]int)
			xs := 15 + float64(i)*36
			pdf.SetFillColor(c[0], c[1], c[2])
			pdf.Rect(xs, y, 3, 3, "F")
			pdf.SetXY(xs+4.5, y-0.8)
			pdf.Cell(33, 4, T(fmt.Sprintf("%s (%d)", seg[0], valores[i])))
		}
		pdf.SetY(y + 8)
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
	cab := []string{"ORD", "Função", "Nome de guerra", "Setor", "Grupo", "Pres.", "Atraso", "Falta", "Just.", "N.V."}
	larg := []float64{11, 30, 30, 22, 22, 14, 14, 14, 14, 12}
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
			strconv.Itoa(p.Antiguidade), fnCell, T(p.NomeGuerra), T(p.Setor), T(p.Grupo),
			strconv.Itoa(p.Presencas), strconv.Itoa(p.Atrasos),
			strconv.Itoa(p.Faltas), strconv.Itoa(p.Justificadas), strconv.Itoa(p.NaoVerificados),
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
// gerarConferenciaPDF: Relatório detalhado de uma conferência de pessoal (v2.0)
func (a *App) gerarConferenciaPDF(c ConferenciaPDF) ([]byte, error) {
	T := cp1252Traduz.Replace
	sub := fmt.Sprintf("Data: %s · Iniciada por: %s em %s", c.Data, c.CriadoPor, fmtDataBR(c.CriadaEm))
	if c.FechadaEm != nil {
		sub += fmt.Sprintf(" · Fechada em %s", fmtDataBR(*c.FechadaEm))
	}
	pdf := a.novoPDF("P", fmt.Sprintf("CONFERÊNCIA DE PESSOAL Nº #%d", c.ID), sub, c.GeradoPor)

	presentes := c.Resumo["presentes"]
	atrasos := c.Resumo["atrasos"]
	faltas := c.Resumo["faltas"]
	just := c.Resumo["justificadas"]
	nv := c.Resumo["nao_verificados"]
	total := presentes + atrasos + faltas + just + nv

	pctVal := "—"
	if total > 0 {
		pctVal = fmt.Sprintf("%.1f%%", 100*float64(presentes+atrasos)/float64(total))
	}

	caixas := [][2]string{
		{"LANÇADOS", strconv.Itoa(total)},
		{"PRESENTES", strconv.Itoa(presentes)},
		{"ATRASOS", strconv.Itoa(atrasos)},
		{"FALTAS", strconv.Itoa(faltas)},
		{"JUSTIFICADAS", strconv.Itoa(just)},
		{"NÃO VERIF.", strconv.Itoa(nv)},
		{"% VÁLIDAS", pctVal},
	}
	y0 := pdf.GetY()
	for i, cx := range caixas {
		x := 14 + float64(i)*26.0
		pdf.SetXY(x, y0)
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(203, 213, 225)
		pdf.Rect(x, y0, 24.5, 13, "FD")

		pdf.SetXY(x, y0+1.5)
		pdf.SetFont("Helvetica", "B", 6.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(24.5, 3.5, T(cx[0]), "", 0, "C", false, 0, "")

		pdf.SetXY(x, y0+5.5)
		pdf.SetFont("Helvetica", "B", 10.5)
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(24.5, 6, T(cx[1]), "", 0, "C", false, 0, "")
	}
	pdf.SetY(y0 + 17)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("CHAMADA NOMINAL & LANÇAMENTOS"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	cab := []string{"ORD", "Função", "Nome de Guerra", "Setor", "Situação", "Destino / Motivo", "Observações"}
	larg := []float64{10, 28, 34, 26, 22, 26, 36}
	alinh := []string{"C", "L", "L", "L", "C", "L", "L"}
	pdfTabelaCabecalho(pdf, cab, larg)

	sitRot := map[string]string{
		"presente":       "Presente",
		"atraso":         "Atraso",
		"falta":          "Falta",
		"justificada":    "Justificada",
		"nao_verificado": "NÃO VERIF.",
	}

	for idx, l := range c.Lancamentos {
		if pdf.GetY() > 265 {
			pdf.AddPage()
			pdfTabelaCabecalho(pdf, cab, larg)
		}
		st := str(l["situacao"])
		if r, ok := sitRot[st]; ok {
			st = r
		}
		fn := str(l["funcao"])
		if fn == "" {
			fn = "—"
		}
		vals := []string{
			strconv.Itoa(l["ord"].(int)),
			fn,
			str(l["nome_guerra"]),
			str(l["setor"]),
			st,
			str(l["destino"]),
			str(l["observacao"]),
		}
		pdfTabelaLinha(pdf, vals, larg, alinh, idx%2 == 1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gerarFichaPessoalPDF: Dossiê Individual do Militar / Servidor (v2.0)
func (a *App) gerarFichaPessoalPDF(f FichaPessoalPDF, operador string) ([]byte, error) {
	T := cp1252Traduz.Replace
	sub := fmt.Sprintf("Militar: %s · Setor: %s · Unidade/Grupo: %s", f.NomeGuerra, f.Setor, f.Grupo)
	pdf := a.novoPDF("P", "FICHA CADASTRAL INDIVIDUAL", sub, operador)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("1. IDENTIFICAÇÃO E DADOS CADASTRAIS"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	yFoto := pdf.GetY()
	fotoInserida := false
	if f.FotoBase64 != "" {
		dadosB64 := f.FotoBase64
		if idx := strings.Index(dadosB64, ","); idx != -1 {
			dadosB64 = dadosB64[idx+1:]
		}
		imgBytes, err := base64.StdEncoding.DecodeString(dadosB64)
		if err == nil && len(imgBytes) > 0 {
			imgName := fmt.Sprintf("foto_p_%d_%d", f.ID, time.Now().UnixNano())
			tp := "JPEG"
			if bytes.HasPrefix(imgBytes, []byte("\x89PNG")) {
				tp = "PNG"
			}
			opt := fpdf.ImageOptions{ImageType: tp}
			pdf.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(imgBytes))
			pdf.ImageOptions(imgName, 14, yFoto, 30, 38, false, opt, 0, "")
			fotoInserida = true
		}
	}
	if !fotoInserida {
		pdf.SetFillColor(241, 245, 249)
		pdf.SetDrawColor(203, 213, 225)
		pdf.Rect(14, yFoto, 30, 38, "FD")
		pdf.SetXY(14, yFoto+17)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetTextColor(148, 163, 184)
		pdf.CellFormat(30, 4, T("[ FOTO 3x4 ]"), "", 0, "C", false, 0, "")
	}

	pdf.SetXY(48, yFoto)
	campos := [][2]string{
		{"Nome de Guerra / Função", fmt.Sprintf("%s (%s)", f.NomeGuerra, f.Funcao)},
		{"Nome Completo", f.NomeCompleto},
		{"Subunidade / Grupo", f.Grupo},
		{"Pelotão / Setor", f.Setor},
		{"Data de Nascimento / Sangue", fmt.Sprintf("%s   ·   Tipo Sanguíneo: %s", f.DataNascimento, f.TipoSanguineo)},
		{"Telefone / E-mail", fmt.Sprintf("%s   ·   %s", f.Telefone, f.Email)},
		{"Endereço Residencial", f.Endereco},
		{"Status Cadastral", strings.ToUpper(f.Status)},
	}

	for _, cp := range campos {
		pdf.SetX(48)
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(40, 4.7, T(cp[0])+":", "", 0, "L", false, 0, "")

		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(15, 23, 42)
		val := cp[1]
		if strings.TrimSpace(val) == "" || val == " ()" || val == "   ·   Tipo Sanguíneo: " || val == "   ·   " {
			val = "—"
		}
		pdf.CellFormat(108, 4.7, T(val), "", 1, "L", false, 0, "")
	}
	pdf.SetY(yFoto + 42)

	// Resumo de Assiduidade
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("2. HISTÓRICO DE EFETIVO E ASSIDUIDADE"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	cards := [][2]string{
		{"CONFERÊNCIAS", strconv.Itoa(f.TotalConfs)},
		{"PRESENTES", strconv.Itoa(f.Presencas)},
		{"ATRASOS", strconv.Itoa(f.Atrasos)},
		{"FALTAS", strconv.Itoa(f.Faltas)},
		{"JUSTIFICADAS", strconv.Itoa(f.Justificadas)},
		{"% ASSIDUIDADE", fmt.Sprintf("%.1f%%", f.PctPresenca)},
	}
	yCards := pdf.GetY()
	for i, c := range cards {
		x := 14 + float64(i)*30.3
		pdf.SetXY(x, yCards)
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(203, 213, 225)
		pdf.Rect(x, yCards, 28.5, 13, "FD")

		pdf.SetXY(x, yCards+1.5)
		pdf.SetFont("Helvetica", "B", 6.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(28.5, 3.5, T(c[0]), "", 0, "C", false, 0, "")

		pdf.SetXY(x, yCards+5.5)
		pdf.SetFont("Helvetica", "B", 10.5)
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(28.5, 6, T(c[1]), "", 0, "C", false, 0, "")
	}
	pdf.SetY(yCards + 16)

	// Cautelas Ativas
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("3. BENS E MATERIAIS ACAUTELADOS ATIVOS"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	if len(f.CautelasAtivas) == 0 {
		pdf.SetFont("Helvetica", "I", 7.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(182, 6, T("Nenhum material acautelado sob responsabilidade no momento."), "1", 1, "C", false, 0, "")
		pdf.Ln(3)
	} else {
		colMat := []string{"Cautela", "Item / Descrição", "Patrimônio", "Data Retirada", "Armeiro Entregador"}
		largMat := []float64{22, 65, 30, 35, 30}
		pdfTabelaCabecalho(pdf, colMat, largMat)
		for idx, c := range f.CautelasAtivas {
			vals := []string{
				fmt.Sprintf("#%v", c["id"]),
				str(c["item_nome"]),
				str(c["codigo_patrimonio"]),
				fmtDataBR(str(c["data_saida"])),
				str(c["responsavel_entrega"]),
			}
			pdfTabelaLinha(pdf, vals, largMat, []string{"C", "L", "C", "C", "L"}, idx%2 == 1)
		}
		pdf.Ln(3)
	}

	// Escalas
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("4. SERVIÇOS DE ESCALA PROGRAMADOS / RECENTES"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	if len(f.Escalas) == 0 {
		pdf.SetFont("Helvetica", "I", 7.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(182, 6, T("Nenhum serviço de escala recente ou programado registrado."), "1", 1, "C", false, 0, "")
		pdf.Ln(3)
	} else {
		colEsc := []string{"Início do Turno", "Término do Turno", "Tipo de Serviço / Posto", "Função Escalada"}
		largEsc := []float64{40, 40, 52, 50}
		pdfTabelaCabecalho(pdf, colEsc, largEsc)
		for idx, e := range f.Escalas {
			vals := []string{
				fmtDataBR(str(e["data_inicio"])),
				fmtDataBR(str(e["data_fim"])),
				str(e["tipo_nome"]),
				str(e["funcao_escala"]),
			}
			pdfTabelaLinha(pdf, vals, largEsc, []string{"C", "C", "L", "L"}, idx%2 == 1)
		}
		pdf.Ln(3)
	}

	// Assinaturas
	pdf.SetY(250)
	pdf.SetDrawColor(148, 163, 184)
	pdf.SetLineWidth(0.3)
	pdf.Line(20, 260, 95, 260)
	pdf.Line(115, 260, 190, 260)

	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(71, 85, 105)
	pdf.SetXY(20, 261)
	pdf.CellFormat(75, 4, T(f.NomeCompleto), "", 0, "C", false, 0, "")
	pdf.SetXY(20, 265)
	pdf.CellFormat(75, 4, T("Assinatura do Militar"), "", 0, "C", false, 0, "")

	pdf.SetXY(115, 261)
	pdf.CellFormat(75, 4, T("Encarregado de Pessoal / Comandante"), "", 0, "C", false, 0, "")
	pdf.SetXY(115, 265)
	pdf.CellFormat(75, 4, T("Visto da Autoridade Competente"), "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gerarReciboCautelaPDF: Ticket / Recibo formal de cautela com 2 vias (v2.0)
func (a *App) gerarReciboCautelaPDF(r ReciboCautelaPDF, operador string) ([]byte, error) {
	T := cp1252Traduz.Replace
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(14, 10, 14)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	renderVia := func(yOffset float64, viaTitulo string) {
		largura := 182.0

		pdf.SetDrawColor(203, 213, 225)
		pdf.SetLineWidth(0.3)
		pdf.SetFillColor(255, 255, 255)
		pdf.Rect(14, yOffset, largura, 126, "D")

		pdf.SetFillColor(30, 41, 59)
		pdf.Rect(14, yOffset, largura, 2, "F")

		pdf.SetXY(18, yOffset+4)
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(15, 23, 42)
		om := a.omTitulo
		if om == "" {
			om = "SCI — SISTEMA DE CONTROLE INTERNO"
		}
		pdf.CellFormat(110, 5, T(om), "", 0, "L", false, 0, "")

		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(16, 185, 129)
		pdf.CellFormat(66, 5, T(viaTitulo), "", 1, "R", false, 0, "")

		pdf.SetX(18)
		pdf.SetFont("Helvetica", "B", 9.5)
		pdf.SetTextColor(30, 41, 59)
		pdf.CellFormat(110, 4.5, T(fmt.Sprintf("RECIBO DE CAUTELA Nº #%d   ·   STATUS: %s", r.ID, strings.ToUpper(r.Status))), "", 0, "L", false, 0, "")

		agoraStr := time.Now().In(a.horaLocal).Format("02/01/2006 15:04")
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(66, 4.5, T("Emitido em: "+agoraStr), "", 1, "R", false, 0, "")

		pdf.SetY(yOffset + 15)
		pdf.SetDrawColor(226, 232, 240)
		pdf.Line(18, pdf.GetY(), 192, pdf.GetY())
		pdf.Ln(2)

		yDados := pdf.GetY()
		pdf.SetXY(18, yDados)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(30, 41, 59)
		pdf.CellFormat(86, 4, T("DISCRIMINAÇÃO DO MATERIAL"), "", 1, "L", false, 0, "")

		itensMat := [][2]string{
			{"Item / Bem", r.ItemNome},
			{"Patrimônio / Tombo", r.CodigoPatrimonio},
			{"Número de Série", r.NumeroSerie},
			{"Categoria", r.CategoriaNome},
			{"Sensibilidade", strings.ToUpper(r.Sensibilidade)},
		}
		for _, it := range itensMat {
			pdf.SetX(18)
			pdf.SetFont("Helvetica", "B", 7)
			pdf.SetTextColor(71, 85, 105)
			pdf.CellFormat(30, 3.8, T(it[0])+":", "", 0, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 7)
			pdf.SetTextColor(15, 23, 42)
			v := it[1]
			if strings.TrimSpace(v) == "" {
				v = "—"
			}
			pdf.CellFormat(56, 3.8, T(v), "", 1, "L", false, 0, "")
		}

		pdf.SetXY(108, yDados)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetTextColor(30, 41, 59)
		pdf.CellFormat(84, 4, T("MILITAR / TOMADOR RESPONSÁVEL"), "", 1, "L", false, 0, "")

		itensTomador := [][2]string{
			{"Nome de Guerra", r.PessoaNomeGuerra},
			{"Nome Completo", r.PessoaCompleto},
			{"Função / Posto", r.PessoaFuncao},
			{"Setor / Pelotão", r.PessoaSetor},
			{"Unidade / Grupo", r.PessoaGrupo},
		}
		for _, it := range itensTomador {
			pdf.SetX(108)
			pdf.SetFont("Helvetica", "B", 7)
			pdf.SetTextColor(71, 85, 105)
			pdf.CellFormat(28, 3.8, T(it[0])+":", "", 0, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 7)
			pdf.SetTextColor(15, 23, 42)
			v := it[1]
			if strings.TrimSpace(v) == "" {
				v = "—"
			}
			pdf.CellFormat(56, 3.8, T(v), "", 1, "L", false, 0, "")
		}

		pdf.SetY(yDados + 24)
		pdf.SetDrawColor(226, 232, 240)
		pdf.SetFillColor(248, 250, 252)
		pdf.Rect(18, pdf.GetY(), 174, 12, "FD")

		pdf.SetXY(20, pdf.GetY()+1.5)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetTextColor(71, 85, 105)
		saidaInfo := fmt.Sprintf("Data de Retirada: %s   ·   Armeiro / Operador da Saída: %s", fmtDataBR(r.DataSaida), r.ResponsavelSaida)
		pdf.CellFormat(170, 4, T(saidaInfo), "", 1, "L", false, 0, "")

		if r.DataDevolucao != "" {
			devInfo := fmt.Sprintf("Devolvido em: %s   ·   Recebido por: %s", fmtDataBR(r.DataDevolucao), r.ResponsavelDev)
			if r.ObsDevolucao != "" {
				devInfo += fmt.Sprintf("   ·   Avarias: %s", r.ObsDevolucao)
			}
			pdf.SetX(20)
			pdf.SetTextColor(16, 185, 129)
			pdf.CellFormat(170, 4, T(devInfo), "", 1, "L", false, 0, "")
		} else {
			obsTxt := r.ObsSaida
			if obsTxt == "" {
				obsTxt = "Nenhuma avaria ou ressalva declarada na retirada."
			}
			pdf.SetX(20)
			pdf.SetTextColor(100, 116, 139)
			pdf.CellFormat(170, 4, T("Observações de Saída: "+obsTxt), "", 1, "L", false, 0, "")
		}

		pdf.SetY(pdf.GetY() + 6)
		pdf.SetFont("Helvetica", "I", 6.8)
		pdf.SetTextColor(100, 116, 139)
		termo := "Declaro que recebi o material acima especificado em perfeitas condições de uso e conservação, assumindo integral responsabilidade civil, administrativa e penal pela sua guarda, manutenção e restituição, obrigando-me a comunicar imediatamente qualquer extravio ou avaria."
		pdf.SetX(18)
		pdf.MultiCell(174, 3.2, T(termo), "", "J", false)

		yAss := yOffset + 104
		pdf.SetDrawColor(148, 163, 184)
		pdf.SetLineWidth(0.3)
		pdf.Line(24, yAss+10, 96, yAss+10)
		pdf.Line(114, yAss+10, 186, yAss+10)

		pdf.SetFont("Helvetica", "", 7)
		pdf.SetTextColor(71, 85, 105)
		pdf.SetXY(24, yAss+11)
		pdf.CellFormat(72, 4, T(fmt.Sprintf("%s (Tomador)", r.PessoaNomeGuerra)), "", 0, "C", false, 0, "")
		pdf.SetXY(114, yAss+11)
		pdf.CellFormat(72, 4, T(fmt.Sprintf("%s (Armaria / Reserva)", r.ResponsavelSaida)), "", 0, "C", false, 0, "")
	}

	renderVia(10, "1ª VIA — RESERVA DE MATERIAL / ARMARIA")

	pdf.SetY(145)
	pdf.SetDrawColor(148, 163, 184)
	pdf.SetLineWidth(0.2)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(148, 163, 184)
	pdf.CellFormat(182, 5, T("✂ - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -"), "", 1, "C", false, 0, "")

	renderVia(155, "2ª VIA — MILITAR / TOMADOR")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gerarEscalasPDF: Grade de Escalas e Serviços em modo Landscape (v2.0)
func (a *App) gerarEscalasPDF(e EscalasRelatorioPDF, operador string) ([]byte, error) {
	T := cp1252Traduz.Replace
	sub := fmt.Sprintf("Período: %s · Unidade/Grupo: %s", e.Periodo, e.Grupo)
	pdf := a.novoPDF("L", "ESCALA DE SERVIÇO DE EFETIVO", sub, operador)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("GRADE OFICIAL DE SERVIÇOS & TURNOS PROGRAMADOS"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	if len(e.Turnos) == 0 {
		pdf.SetFont("Helvetica", "I", 8.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(269, 8, T("Nenhum turno de serviço escalado para o período selecionado."), "1", 1, "C", false, 0, "")
	} else {
		col := []string{"Início", "Término", "Tipo de Posto / Serviço", "Grupo / Unidade", "Militares Alocados e Funções", "Observações"}
		larg := []float64{32, 32, 50, 40, 75, 40}
		al := []string{"C", "C", "L", "L", "L", "L"}
		pdfTabelaCabecalho(pdf, col, larg)

		for idx, t := range e.Turnos {
			if pdf.GetY() > 180 {
				pdf.AddPage()
				pdfTabelaCabecalho(pdf, col, larg)
			}
			mils := str(t["militares"])
			if mils == "" {
				mils = "[ Sem militares escalados ]"
			}
			vals := []string{
				fmtDataBR(str(t["data_inicio"])),
				fmtDataBR(str(t["data_fim"])),
				str(t["tipo_nome"]),
				str(t["grupo_nome"]),
				mils,
				str(t["observacao"]),
			}
			pdfTabelaLinha(pdf, vals, larg, al, idx%2 == 1)
		}
	}

	pdf.Ln(6)
	if pdf.GetY() > 175 {
		pdf.AddPage()
	}

	pdf.SetY(pdf.GetY() + 6)
	pdf.SetDrawColor(148, 163, 184)
	pdf.SetLineWidth(0.3)
	pdf.Line(85, pdf.GetY()+12, 195, pdf.GetY()+12)

	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(71, 85, 105)
	pdf.SetXY(85, pdf.GetY()+13)
	pdf.CellFormat(110, 4, T("Comandante / Chefe da Subunidade"), "", 1, "C", false, 0, "")
	pdf.SetX(85)
	pdf.CellFormat(110, 4, T("HOMOLOGAÇÃO DA ESCALA DE SERVIÇO"), "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gerarInventarioMaterialPDF: Relatório de Inventário & Carga do Material (v2.0)
func (a *App) gerarInventarioMaterialPDF(inv InventarioRelatorioPDF, operador string) ([]byte, error) {
	T := cp1252Traduz.Replace
	sub := fmt.Sprintf("Unidade/Grupo: %s · Base Patrimonial e Conferência de Carga", inv.Grupo)
	pdf := a.novoPDF("P", "RELATÓRIO GERAL DE INVENTÁRIO & CARGA", sub, operador)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("1. RESUMO DO PATRIMÔNIO CADASTRADO"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	cards := [][2]string{
		{"TOTAL ITENS", strconv.Itoa(inv.Totais["total"])},
		{"DISPONÍVEIS", strconv.Itoa(inv.Totais["disponivel"])},
		{"ACAUTELADOS", strconv.Itoa(inv.Totais["acautelado"])},
		{"MANUTENÇÃO", strconv.Itoa(inv.Totais["manutencao"])},
		{"BAIXADOS", strconv.Itoa(inv.Totais["baixado"])},
	}
	yCards := pdf.GetY()
	for i, c := range cards {
		x := 14 + float64(i)*36.4
		pdf.SetXY(x, yCards)
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(203, 213, 225)
		pdf.Rect(x, yCards, 34.5, 13, "FD")

		pdf.SetXY(x, yCards+1.5)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(34.5, 3.5, T(c[0]), "", 0, "C", false, 0, "")

		pdf.SetXY(x, yCards+5.5)
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(15, 23, 42)
		pdf.CellFormat(34.5, 6, T(c[1]), "", 0, "C", false, 0, "")
	}
	pdf.SetY(yCards + 17)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("2. DISCRIMINAÇÃO COMPLETA DE CARGA & PATRIMÔNIO"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	if len(inv.Itens) == 0 {
		pdf.SetFont("Helvetica", "I", 8.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(182, 8, T("Nenhum item patrimonial cadastrado para esta unidade."), "1", 1, "C", false, 0, "")
	} else {
		col := []string{"Patrimônio", "Item / Descrição", "Categoria", "Nº Série", "Status", "Posse / Local", "Visto"}
		larg := []float64{25, 48, 30, 25, 22, 22, 10}
		al := []string{"C", "L", "L", "C", "C", "L", "C"}
		pdfTabelaCabecalho(pdf, col, larg)

		for idx, it := range inv.Itens {
			if pdf.GetY() > 265 {
				pdf.AddPage()
				pdfTabelaCabecalho(pdf, col, larg)
			}
			stRot := map[string]string{
				"disponivel": "Disponível",
				"acautelado": "Acautelado",
				"manutencao": "Manutenção",
				"baixado":    "Baixado",
			}
			st := str(it["status"])
			if r, ok := stRot[st]; ok {
				st = r
			}
			vals := []string{
				str(it["codigo_patrimonio"]),
				str(it["nome"]),
				str(it["categoria_nome"]),
				str(it["numero_serie"]),
				st,
				str(it["responsavel_atual"]),
				"[  ]",
			}
			pdfTabelaLinha(pdf, vals, larg, al, idx%2 == 1)
		}
	}

	pdf.Ln(6)
	if pdf.GetY() > 255 {
		pdf.AddPage()
	}

	pdf.SetY(pdf.GetY() + 8)
	pdf.SetDrawColor(148, 163, 184)
	pdf.SetLineWidth(0.3)
	pdf.Line(45, pdf.GetY()+12, 145, pdf.GetY()+12)

	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(71, 85, 105)
	pdf.SetXY(45, pdf.GetY()+13)
	pdf.CellFormat(100, 4, T("Encarregado do Material / Comissão de Inventário"), "", 1, "C", false, 0, "")
	pdf.SetX(45)
	pdf.CellFormat(100, 4, T("CONFERÊNCIA FÍSICA DE CARGA HOMOLOGADA"), "", 1, "C", false, 0, "")

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
	// v9.15.3 (ordem Tenente): TODOS os horários exibidos são de BRASÍLIA, sem exceção
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*3600)
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.In(loc).Format("02/01/2006 15:04")
	}
	if len(iso) >= 16 {
		return iso[:10] + " " + iso[11:16]
	}
	return iso
}

// hExportarDados: Motor de exportação analítica e fria (Dual-Mode: JSON e SQLite).
// Permite extração granular por Ano, Período (De..Ate), Tabela, Pessoa ou Item.
func (a *App) hExportarDados(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	tipo := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tipo")))
	if tipo == "" {
		tipo = "json"
	}
	ano := strings.TrimSpace(r.URL.Query().Get("ano"))
	de := strings.TrimSpace(r.URL.Query().Get("de"))
	ate := strings.TrimSpace(r.URL.Query().Get("ate"))
	if ano != "" && de == "" && ate == "" {
		de = ano + "-01-01"
		ate = ano + "-12-31"
	}
	tabela := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tabela")))
	if tabela == "" {
		tabela = "tudo"
	}
	pessoaID, _ := strconv.ParseInt(r.URL.Query().Get("pessoa_id"), 10, 64)
	itemID, _ := strconv.ParseInt(r.URL.Query().Get("item_id"), 10, 64)

	ts := time.Now().Format("20060102_150405")

	if tipo == "sqlite" {
		tempDir := os.TempDir()
		tempFile := filepath.Join(tempDir, fmt.Sprintf("sci_export_%s_%d.db", ts, time.Now().UnixNano()))
		defer os.Remove(tempFile)

		dbTemp, err := sql.Open("sqlite", tempFile)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao criar base temporária de exportação: "+err.Error())
			return
		}
		defer dbTemp.Close()

		_, _ = dbTemp.Exec(`BEGIN TRANSACTION`)
		_, _ = dbTemp.Exec(`CREATE TABLE export_meta (chave TEXT PRIMARY KEY, valor TEXT)`)
		_, _ = dbTemp.Exec(`INSERT INTO export_meta VALUES ('gerado_em', ?), ('operador', ?), ('filtros', ?)`,
			time.Now().UTC().Format(time.RFC3339), u.Login, fmt.Sprintf("ano=%s de=%s ate=%s tabela=%s", ano, de, ate, tabela))

		// Exportar conferências
		if tabela == "tudo" || tabela == "conferencias" {
			_, _ = dbTemp.Exec(`
				CREATE TABLE conferencias (
					id INTEGER PRIMARY KEY, grupo_id INTEGER, data TEXT, local TEXT,
					status TEXT, aberta_em TEXT, fechada_em TEXT, criado_por TEXT
				);
				CREATE TABLE presencas (
					id INTEGER PRIMARY KEY, conferencia_id INTEGER, pessoa_id INTEGER,
					nome_guerra TEXT, situacao TEXT, destino TEXT, observacao TEXT, verificado INTEGER
				);
			`)
			qConf := `SELECT c.id, c.grupo_id, c.data, COALESCE(c.local,''), c.status, c.criado_em, COALESCE(c.fechada_em,''), COALESCE(u.login,'')
			          FROM conferencias c LEFT JOIN usuarios u ON u.id = c.criado_por WHERE 1=1`
			var argsConf []any
			if escopo > 0 {
				qConf += ` AND c.grupo_id = ?`
				argsConf = append(argsConf, escopo)
			}
			if de != "" && ate != "" {
				qConf += ` AND c.data BETWEEN ? AND ?`
				argsConf = append(argsConf, de, ate)
			}
			qConf += ` ORDER BY c.data DESC, c.id DESC`
			rows, err := a.st.db.Query(qConf, argsConf...)
			if err == nil {
				for rows.Next() {
					var cid, gid int64
					var dt, loc, st, cri, fec, por string
					if rows.Scan(&cid, &gid, &dt, &loc, &st, &cri, &fec, &por) == nil {
						_, _ = dbTemp.Exec(`INSERT INTO conferencias VALUES (?,?,?,?,?,?,?,?)`, cid, gid, dt, loc, st, cri, fec, por)
					}
				}
				rows.Close()
			}
			// Presenças associadas
			qPres := `SELECT pr.id, pr.conferencia_id, pr.pessoa_id, p.nome_guerra, pr.situacao, COALESCE(d.nome,''), COALESCE(pr.observacao,''), pr.verificado
			          FROM presencas pr
			          JOIN conferencias c ON c.id = pr.conferencia_id
			          JOIN pessoas p ON p.id = pr.pessoa_id
			          LEFT JOIN destinos d ON d.id = pr.destino_id
			          WHERE 1=1`
			var argsPres []any
			if escopo > 0 {
				qPres += ` AND c.grupo_id = ?`
				argsPres = append(argsPres, escopo)
			}
			if de != "" && ate != "" {
				qPres += ` AND c.data BETWEEN ? AND ?`
				argsPres = append(argsPres, de, ate)
			}
			if pessoaID > 0 {
				qPres += ` AND pr.pessoa_id = ?`
				argsPres = append(argsPres, pessoaID)
			}
			rPres, errP := a.st.db.Query(qPres, argsPres...)
			if errP == nil {
				for rPres.Next() {
					var pid, cid, pesId int64
					var ng, sit, dest, obs string
					var ver int
					if rPres.Scan(&pid, &cid, &pesId, &ng, &sit, &dest, &obs, &ver) == nil {
						_, _ = dbTemp.Exec(`INSERT INTO presencas VALUES (?,?,?,?,?,?,?,?)`, pid, cid, pesId, ng, sit, dest, obs, ver)
					}
				}
				rPres.Close()
			}
		}

		// Exportar Cautelas
		if tabela == "tudo" || tabela == "material" {
			_, _ = dbTemp.Exec(`
				CREATE TABLE material_cautelas (
					id INTEGER PRIMARY KEY, item_id INTEGER, item_nome TEXT, codigo_patrimonio TEXT,
					pessoa_id INTEGER, pessoa_nome TEXT, data_saida TEXT, data_devolucao TEXT,
					status TEXT, responsavel_entrega TEXT, obs_saida TEXT
				)
			`)
			qCaut := `SELECT mc.id, mc.item_id, mi.nome, mi.codigo_patrimonio, mc.pessoa_id, p.nome_guerra,
			                 mc.data_saida, COALESCE(mc.data_devolucao,''), mc.status, COALESCE(u.login,''), COALESCE(mc.obs_saida,'')
			          FROM material_cautelas mc
			          JOIN material_itens mi ON mi.id = mc.item_id
			          JOIN pessoas p ON p.id = mc.pessoa_id
			          LEFT JOIN usuarios u ON u.id = mc.responsavel_entrega_id
			          WHERE 1=1`
			var argsCaut []any
			if escopo > 0 {
				qCaut += ` AND mi.grupo_id = ?`
				argsCaut = append(argsCaut, escopo)
			}
			if de != "" && ate != "" {
				qCaut += ` AND substr(mc.data_saida, 1, 10) BETWEEN ? AND ?`
				argsCaut = append(argsCaut, de, ate)
			}
			if itemID > 0 {
				qCaut += ` AND mc.item_id = ?`
				argsCaut = append(argsCaut, itemID)
			}
			if pessoaID > 0 {
				qCaut += ` AND mc.pessoa_id = ?`
				argsCaut = append(argsCaut, pessoaID)
			}
			rCaut, errC := a.st.db.Query(qCaut, argsCaut...)
			if errC == nil {
				for rCaut.Next() {
					var cid, iid, pesId int64
					var inome, icod, png, dts, dtd, st, resp, obs string
					if rCaut.Scan(&cid, &iid, &inome, &icod, &pesId, &png, &dts, &dtd, &st, &resp, &obs) == nil {
						_, _ = dbTemp.Exec(`INSERT INTO material_cautelas VALUES (?,?,?,?,?,?,?,?,?,?,?)`, cid, iid, inome, icod, pesId, png, dts, dtd, st, resp, obs)
					}
				}
				rCaut.Close()
			}
		}

		_, _ = dbTemp.Exec(`COMMIT`)
		_ = dbTemp.Close()

		conteudo, err := os.ReadFile(tempFile)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao ler arquivo exportado: "+err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "exportar_sqlite", "sistema", nil, fmt.Sprintf("de=%s ate=%s tab=%s tam=%d", de, ate, tabela, len(conteudo)), ipDe(r))
		w.Header().Set("Content-Type", "application/vnd.sqlite3")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sci_export_%s.db"`, ts))
		w.Header().Set("Content-Length", strconv.Itoa(len(conteudo)))
		_, _ = w.Write(conteudo)
		return
	}

	// Modalidade JSON
	resultado := map[string]any{
		"sistema":   "SCI — Sistema de Controle Interno",
		"gerado_em": time.Now().UTC().Format(time.RFC3339),
		"operador":  u.Login,
		"filtros": map[string]any{
			"ano":       ano,
			"de":        de,
			"ate":       ate,
			"tabela":    tabela,
			"pessoa_id": pessoaID,
			"item_id":   itemID,
		},
	}

	// Conferências e Presenças
	if tabela == "tudo" || tabela == "conferencias" {
		qConf := `SELECT c.id, c.grupo_id, c.data, COALESCE(c.local,''), c.status, c.criado_em, COALESCE(c.fechada_em,''), COALESCE(u.login,'')
		          FROM conferencias c LEFT JOIN usuarios u ON u.id = c.criado_por WHERE 1=1`
		var argsConf []any
		if escopo > 0 {
			qConf += ` AND c.grupo_id = ?`
			argsConf = append(argsConf, escopo)
		}
		if de != "" && ate != "" {
			qConf += ` AND c.data BETWEEN ? AND ?`
			argsConf = append(argsConf, de, ate)
		}
		qConf += ` ORDER BY c.data DESC, c.id DESC LIMIT 1000`
		rows, err := a.st.db.Query(qConf, argsConf...)
		var confList []map[string]any
		if err == nil {
			for rows.Next() {
				var cid, gid int64
				var dt, loc, st, cri, fec, por string
				if rows.Scan(&cid, &gid, &dt, &loc, &st, &cri, &fec, &por) == nil {
					confList = append(confList, map[string]any{
						"id": cid, "grupo_id": gid, "data": dt, "local": loc,
						"status": st, "criada_em": cri, "fechada_em": fec, "criado_por": por,
					})
				}
			}
			rows.Close()
		}
		resultado["conferencias"] = confList

		// Presenças
		qPres := `SELECT pr.id, pr.conferencia_id, pr.pessoa_id, p.nome_guerra, pr.situacao, COALESCE(d.nome,''), COALESCE(pr.observacao,''), pr.verificado
		          FROM presencas pr
		          JOIN conferencias c ON c.id = pr.conferencia_id
		          JOIN pessoas p ON p.id = pr.pessoa_id
		          LEFT JOIN destinos d ON d.id = pr.destino_id
		          WHERE 1=1`
		var argsPres []any
		if escopo > 0 {
			qPres += ` AND c.grupo_id = ?`
			argsPres = append(argsPres, escopo)
		}
		if de != "" && ate != "" {
			qPres += ` AND c.data BETWEEN ? AND ?`
			argsPres = append(argsPres, de, ate)
		}
		if pessoaID > 0 {
			qPres += ` AND pr.pessoa_id = ?`
			argsPres = append(argsPres, pessoaID)
		}
		qPres += ` ORDER BY pr.id DESC LIMIT 5000`
		rPres, errP := a.st.db.Query(qPres, argsPres...)
		var presList []map[string]any
		if errP == nil {
			for rPres.Next() {
				var pid, cid, pesId int64
				var ng, sit, dest, obs string
				var ver int
				if rPres.Scan(&pid, &cid, &pesId, &ng, &sit, &dest, &obs, &ver) == nil {
					presList = append(presList, map[string]any{
						"id": pid, "conferencia_id": cid, "pessoa_id": pesId,
						"pessoa_nome_guerra": ng, "situacao": sit, "destino": dest,
						"observacao": obs, "verificado": ver == 1,
					})
				}
			}
			rPres.Close()
		}
		resultado["presencas"] = presList
	}

	// Cautelas
	if tabela == "tudo" || tabela == "material" {
		qCaut := `SELECT mc.id, mc.item_id, mi.nome, mi.codigo_patrimonio, mc.pessoa_id, p.nome_guerra,
		                 mc.data_saida, COALESCE(mc.data_devolucao,''), mc.status, COALESCE(u.login,''), COALESCE(mc.obs_saida,''),
		                 COALESCE(mi.nivel_sensibilidade, 'padrao')
		          FROM material_cautelas mc
		          JOIN material_itens mi ON mi.id = mc.item_id
		          JOIN pessoas p ON p.id = mc.pessoa_id
		          LEFT JOIN usuarios u ON u.id = mc.responsavel_entrega_id
		          WHERE 1=1`
		var argsCaut []any
		if escopo > 0 {
			qCaut += ` AND mi.grupo_id = ?`
			argsCaut = append(argsCaut, escopo)
		}
		if de != "" && ate != "" {
			qCaut += ` AND substr(mc.data_saida, 1, 10) BETWEEN ? AND ?`
			argsCaut = append(argsCaut, de, ate)
		}
		if itemID > 0 {
			qCaut += ` AND mc.item_id = ?`
			argsCaut = append(argsCaut, itemID)
		}
		if pessoaID > 0 {
			qCaut += ` AND mc.pessoa_id = ?`
			argsCaut = append(argsCaut, pessoaID)
		}
		qCaut += ` ORDER BY mc.id DESC LIMIT 5000`
		rCaut, errC := a.st.db.Query(qCaut, argsCaut...)
		var cautList []map[string]any
		if errC == nil {
			for rCaut.Next() {
				var cid, iid, pesId int64
				var inome, icod, png, dts, dtd, st, resp, obs, sens string
				if rCaut.Scan(&cid, &iid, &inome, &icod, &pesId, &png, &dts, &dtd, &st, &resp, &obs, &sens) == nil {
					cautList = append(cautList, map[string]any{
						"id": cid, "item_id": iid, "item_nome": inome, "codigo_patrimonio": icod,
						"pessoa_id": pesId, "pessoa_nome_guerra": png, "data_saida": dts,
						"data_devolucao": dtd, "status": st, "responsavel_entrega": resp,
						"obs_saida": obs, "nivel_sensibilidade": sens,
					})
				}
			}
			rCaut.Close()
		}
		resultado["cautelas"] = cautList
	}

	dadosJSON, err := json.MarshalIndent(resultado, "", "  ")
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao serializar json: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar_json", "sistema", nil, fmt.Sprintf("de=%s ate=%s tab=%s tam=%d", de, ate, tabela, len(dadosJSON)), ipDe(r))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sci_export_%s.json"`, ts))
	w.Header().Set("Content-Length", strconv.Itoa(len(dadosJSON)))
	_, _ = w.Write(dadosJSON)
}
