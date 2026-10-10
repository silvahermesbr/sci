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
	// Assinatura (ordem Diretor 04/10): nome completo em negrito + função
	// específica do grupo logo abaixo (quando houver).
	FuncaoGeradoPor string `json:"funcao_gerado_por,omitempty"`
	Resumo      map[string]int   `json:"resumo"`
	Lancamentos []map[string]any `json:"lancamentos"`
	Filtro      string           `json:"filtro,omitempty"`
	FechadoPorNome string        `json:"fechado_por_nome,omitempty"`
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
	sub := fmt.Sprintf("Período: %s a %s · Efetivo Pronto: %.1f%%", b.De, b.Ate, b.PctPronto)
	pdf := a.novoPDF("P", "RELATÓRIO GERAL DE EFETIVO & CONFERÊNCIAS", sub, "")
	a.relatorioSimplesRender(pdf, b)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// relatorioSimplesRender: conteúdo da página 1 do relatório por período
// (resumo, gráficos e tabela por antiguidade) — compartilhado pelo modo
// SIMPLES (gerarRelatorioPDF) e pela 1ª folha do DETALHADO (relatorioDetalhado),
// garantindo página 1 IDÊNTICA nos dois modos (ordem SCI 06/10 — Frente D).
func (a *App) relatorioSimplesRender(pdf *fpdf.Fpdf, b Bundle) {
	T := cp1252Traduz.Replace

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
	pdf.Cell(0, 6, T("EFETIVO — POR ORDEM DE ANTIGUIDADE (ID menor = mais antigo)"))
	pdf.Ln(7)
	cab := []string{"ORD", "Antiguidade", "Nome de guerra", "Setor", "Grupo", "Pres.", "Atraso", "Falta", "Just.", "N.V."}
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
	titulo := fmt.Sprintf("CONFERÊNCIA DE PESSOAL Nº #%d", c.ID)
	if c.Filtro == "faltas" {
		titulo += " (SÓ FALTAS)"
	} else if c.Filtro == "justificados" {
		titulo += " (SÓ JUSTIFICADOS)"
	} else if c.Filtro == "atrasos" {
		titulo += " (SÓ ATRASOS)"
	}
	pdf := a.novoPDF("P", titulo, sub, c.GeradoPor)

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

	cab := []string{"ORD", "Antiguidade", "Nome de Guerra", "Setor", "Situação", "Destino / Motivo", "Observações"}
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

	// Bloco centralizado para assinatura física (v1.5)
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(0, 4, "________________________________________________________", "", 1, "C", false, 0, "")
	// Assinatura por NOME COMPLETO em negrito (ordem Diretor 04/10): nunca o
	// login. Função específica do grupo logo abaixo, quando houver.
	nomeResp := c.GeradoPor
	if strings.TrimSpace(nomeResp) == "" {
		nomeResp = c.FechadoPorNome
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 5, T(nomeResp), "", 1, "C", false, 0, "")
	if strings.TrimSpace(c.FuncaoGeradoPor) != "" {
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(71, 85, 105)
		pdf.CellFormat(0, 4, T(c.FuncaoGeradoPor), "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(100, 116, 139)
	pdf.CellFormat(0, 4, T("Responsável pela Conferência"), "", 1, "C", false, 0, "")

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
		{"Nome de Guerra / Antiguidade", fmt.Sprintf("%s (%s)", f.NomeGuerra, f.Funcao)},
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
		colEsc := []string{"Início do Turno", "Término do Turno", "Tipo de Serviço / Posto", "Antiguidade Escalada"}
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
			{"Antiguidade", r.PessoaFuncao},
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

// ProntoMaterialItemPDF representa um item dentro do relatório de conferência diária de material.
type ProntoMaterialItemPDF struct {
	ItemID              int64  `json:"item_id"`
	Nome                string `json:"nome"`
	CodigoPatrimonio    string `json:"codigo_patrimonio"`
	CategoriaNome       string `json:"categoria_nome"`
	QuantidadeEsperada  int64  `json:"quantidade_esperada"`
	QuantidadeConferida int64  `json:"quantidade_conferida"`
	Status              string `json:"status"`
	ConferidoPorNome    string `json:"conferido_por_nome"`
	Observacao          string `json:"observacao"`
}

// ProntoMaterialPDF contém os dados estruturados para emissão do Pronto Diário de Material.
type ProntoMaterialPDF struct {
	ID              int64                   `json:"id"`
	GrupoNome       string                  `json:"grupo_nome"`
	SetorNome       string                  `json:"setor_nome"`
	Data            string                  `json:"data"`
	Status          string                  `json:"status"`
	AbertaPorNome   string                  `json:"aberta_por_nome"`
	AbertaEm        string                  `json:"aberta_em"`
	FechadaPorNome  string                  `json:"fechada_por_nome"`
	FechadaEm       string                  `json:"fechada_em"`
	EncarregadoNome string                  `json:"encarregado_nome"`
	AuxiliarNome    string                  `json:"auxiliar_nome"`
	Observacao      string                  `json:"observacao"`
	Totais          map[string]int          `json:"totais"`
	Itens           []ProntoMaterialItemPDF `json:"itens"`
}

// gerarProntoMaterialPDF: Relatório oficial de Conferência Diária / Pronto de Material (v2.0)
func (a *App) gerarProntoMaterialPDF(p ProntoMaterialPDF, operador string) ([]byte, error) {
	T := cp1252Traduz.Replace
	sub := fmt.Sprintf("Unidade/Grupo: %s · Setor: %s · Data: %s", p.GrupoNome, p.SetorNome, p.Data)
	pdf := a.novoPDF("P", "PRONTO DIÁRIO DE MATERIAL", sub, operador)

	// 1. Resumo da Conferência Diária
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("1. RESUMO DA CONFERÊNCIA DIÁRIA"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	cards := [][2]string{
		{"TOTAL ITENS", strconv.Itoa(p.Totais["total"])},
		{"PRESENTES", strconv.Itoa(p.Totais["presente"])},
		{"ACAUTELADOS", strconv.Itoa(p.Totais["acautelado"])},
		{"MANUTENÇÃO", strconv.Itoa(p.Totais["manutencao"])},
		{"NÃO CONFERIDOS", strconv.Itoa(p.Totais["nao_conferido"])},
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

	// 2. Tabela de Itens
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(0, 5, T("2. DISCRIMINAÇÃO DOS ITENS & CONFERÊNCIA"), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	if len(p.Itens) == 0 {
		pdf.SetFont("Helvetica", "I", 8.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(182, 8, T("Nenhum item relacionado nesta conferência."), "1", 1, "C", false, 0, "")
	} else {
		col := []string{"Patrimônio", "Item / Descrição", "Esperado", "Conferido", "Situação", "Conferido por", "Observação"}
		larg := []float64{24, 48, 16, 16, 22, 26, 30}
		al := []string{"C", "L", "C", "C", "C", "L", "L"}
		pdfTabelaCabecalho(pdf, col, larg)

		for idx, it := range p.Itens {
			if pdf.GetY() > 260 {
				pdf.AddPage()
				pdfTabelaCabecalho(pdf, col, larg)
			}
			qtdConfStr := "—"
			if it.QuantidadeConferida > 0 {
				qtdConfStr = strconv.FormatInt(it.QuantidadeConferida, 10)
			}
			stRot := map[string]string{
				"presente":   "Presente",
				"acautelado": "Acautelado",
				"ausente":    "Ausente / Falta",
				"manutencao": "Manutenção",
				"baixado":    "Baixado",
			}
			stTxt := it.Status
			if r, ok := stRot[stTxt]; ok {
				stTxt = r
			}
			vals := []string{
				it.CodigoPatrimonio,
				it.Nome,
				strconv.FormatInt(it.QuantidadeEsperada, 10),
				qtdConfStr,
				stTxt,
				it.ConferidoPorNome,
				it.Observacao,
			}
			pdfTabelaLinha(pdf, vals, larg, al, idx%2 == 1)
		}
	}

	pdf.Ln(6)
	if pdf.GetY() > 250 {
		pdf.AddPage()
	}

	yAss := pdf.GetY() + 10
	pdf.SetDrawColor(148, 163, 184)
	pdf.SetLineWidth(0.3)
	pdf.Line(20, yAss, 95, yAss)
	pdf.Line(105, yAss, 180, yAss)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(30, 41, 59)
	pdf.SetXY(20, yAss+2)
	pdf.CellFormat(75, 4, T("Encarregado de Material"), "", 0, "C", false, 0, "")
	pdf.SetXY(105, yAss+2)
	pdf.CellFormat(75, 4, T("Gerente / Chefe de Setor"), "", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(100, 116, 139)
	pdf.SetXY(20, yAss+6)
	pdf.CellFormat(75, 3.5, T("Conferência Física Executada"), "", 0, "C", false, 0, "")
	pdf.SetXY(105, yAss+6)
	pdf.CellFormat(75, 3.5, T("Visto da Autoridade Responsável"), "", 1, "C", false, 0, "")

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
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

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

// MaterialItemEtiqueta representa os metadados do material para confecção de etiquetas.
type MaterialItemEtiqueta struct {
	ID                 int64  `json:"id"`
	Nome               string `json:"nome"`
	CodigoPatrimonio   string `json:"codigo_patrimonio"`
	CategoriaNome      string `json:"categoria_nome"`
	NumeroSerie        string `json:"numero_serie"`
	NivelSensibilidade string `json:"nivel_sensibilidade"`
	TipoMaterial       string `json:"tipo_material"`
	ClasseMaterial     string `json:"classe_material"`
}

// gerarEtiquetasLotePDF gera uma grade padronizada de 10 etiquetas por folha A4 (2 colunas x 5 linhas)
// com código QR de alta precisão e identificação completa do bem.
func (a *App) gerarEtiquetasLotePDF(itens []MaterialItemEtiqueta) ([]byte, error) {
	T := cp1252Traduz.Replace
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(7.5, 12, 7.5)
	pdf.SetAutoPageBreak(false, 0)

	etiquetaW := 95.0
	etiquetaH := 52.0
	colGap := 5.0
	rowGap := 3.0
	leftMargin := 7.5
	topMargin := 12.0

	for idx, it := range itens {
		posNaPagina := idx % 10
		if posNaPagina == 0 {
			pdf.AddPage()
		}
		col := posNaPagina % 2
		lin := posNaPagina / 2

		x := leftMargin + float64(col)*(etiquetaW+colGap)
		y := topMargin + float64(lin)*(etiquetaH+rowGap)

		// Moldura com cantos arredondados
		pdf.SetDrawColor(148, 163, 184)
		pdf.SetLineWidth(0.3)
		pdf.RoundedRect(x, y, etiquetaW, etiquetaH, 2.5, "1234", "D")

		// Faixa superior de cabeçalho
		pdf.SetFillColor(241, 245, 249)
		pdf.RoundedRect(x, y, etiquetaW, 7.0, 2.5, "12", "F")
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetTextColor(51, 65, 85)
		pdf.SetXY(x, y+1.5)
		pdf.CellFormat(etiquetaW, 4.0, "SCI - CONTROLE PATRIMONIAL", "", 0, "C", false, 0, "")

		// QR Code à direita (34x34 mm)
		payload := fmt.Sprintf("sci://m:%d:%s", it.ID, it.CodigoPatrimonio)
		qrObj, err := GerarQRCode(payload)
		if err == nil {
			pngBytes, errPng := qrObj.RenderPNG(4, 2)
			if errPng == nil {
				imgName := fmt.Sprintf("qr_etq_%d_%d", it.ID, idx)
				opt := fpdf.ImageOptions{ImageType: "PNG"}
				pdf.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(pngBytes))
				pdf.ImageOptions(imgName, x+etiquetaW-34.0-3.0, y+8.5, 34.0, 34.0, false, opt, 0, "")
			}
		}

		// Textos à esquerda (largura 54 mm)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetTextColor(15, 23, 42)
		pdf.SetXY(x+3.5, y+8.5)
		nomeExibir := it.Nome
		if len(nomeExibir) > 28 {
			nomeExibir = nomeExibir[:26] + "..."
		}
		pdf.CellFormat(54.0, 4.5, T(nomeExibir), "", 1, "L", false, 0, "")

		// Patrimônio
		pdf.SetFont("Helvetica", "B", 8.5)
		pdf.SetTextColor(2, 132, 199)
		pdf.SetXY(x+3.5, y+14.0)
		pdf.CellFormat(54.0, 4.0, T("PAT: #"+it.CodigoPatrimonio), "", 1, "L", false, 0, "")

		// Categoria
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(71, 85, 105)
		pdf.SetXY(x+3.5, y+19.0)
		catTxt := it.CategoriaNome
		if catTxt == "" {
			catTxt = "Geral"
		}
		if it.TipoMaterial != "" {
			catTxt += " · " + it.TipoMaterial
		}
		if len(catTxt) > 30 {
			catTxt = catTxt[:28] + "..."
		}
		pdf.CellFormat(54.0, 3.5, T("Cat: "+catTxt), "", 1, "L", false, 0, "")

		// Nº de Série
		pdf.SetXY(x+3.5, y+23.0)
		numSerie := it.NumeroSerie
		if numSerie == "" {
			numSerie = "—"
		}
		pdf.CellFormat(54.0, 3.5, T("Série: "+numSerie), "", 1, "L", false, 0, "")

		// Classe / Sensibilidade
		pdf.SetXY(x+3.5, y+27.0)
		classeTxt := it.ClasseMaterial
		if classeTxt == "" {
			classeTxt = it.NivelSensibilidade
		}
		if classeTxt == "" {
			classeTxt = "Padrão"
		}
		pdf.CellFormat(54.0, 3.5, T("Classe: "+strings.ToUpper(classeTxt)), "", 1, "L", false, 0, "")

		// Rodapé da etiqueta
		pdf.SetFont("Helvetica", "I", 6.5)
		pdf.SetTextColor(148, 163, 184)
		pdf.SetXY(x+2.0, y+45.5)
		pdf.CellFormat(etiquetaW-4.0, 4.0, T("Exército Brasileiro · Material Identificado"), "", 0, "C", false, 0, "")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EscalaDiaPDF contém os dados estruturados para emissão do relatório diário de escala de serviço.
type EscalaDiaPDF struct {
	Data          string           `json:"data"`
	GrupoNome     string           `json:"grupo_nome"`
	Fase          string           `json:"fase"`
	GeradoPor     string           `json:"gerado_por"`
	HomologadoPor string           `json:"homologado_por,omitempty"`
	Turnos        []EscalaTurnoPDF `json:"turnos"`
}

// EscalaTurnoPDF representa uma linha de posto na escala diária.
type EscalaTurnoPDF struct {
	ID            int64  `json:"id"`
	PostoNome     string `json:"posto_nome"`
	Horario       string `json:"horario"`
	MilitarNome   string `json:"militar_nome"`
	MilitarGuerra string `json:"militar_guerra"`
	SetorOuOrigem string `json:"setor_ou_origem"`
	Status        string `json:"status"`
}

// gerarEscalaDiaPDF produz o relatório oficial em PDF da escala diária de serviço.
func (a *App) gerarEscalaDiaPDF(d EscalaDiaPDF) ([]byte, error) {
	T := cp1252Traduz.Replace
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(14, 14, 14)
	pdf.AddPage()

	// Cabeçalho institucional
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(0, 7, T("ESCALA DIÁRIA DE SERVIÇO"), "", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(71, 85, 105)
	pdf.CellFormat(0, 5, T("UNIDADE: "+strings.ToUpper(d.GrupoNome)), "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 5, T("DATA DE SERVIÇO: "+fmtDataBR(d.Data)), "", 1, "C", false, 0, "")
	pdf.Ln(3)

	// Barra de metadados e fase
	pdf.SetFillColor(241, 245, 249)
	pdf.SetDrawColor(203, 213, 225)
	pdf.Rect(14, pdf.GetY(), 182, 10, "FD")
	pdf.SetXY(17, pdf.GetY()+2.5)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetTextColor(30, 41, 59)
	pdf.CellFormat(70, 5, T("FASE ATUAL: "+strings.ToUpper(d.Fase)), "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(100, 116, 139)
	pdf.CellFormat(105, 5, T("Emitido por: "+d.GeradoPor+" em "+time.Now().Format("02/01/2006 15:04")), "", 1, "R", false, 0, "")
	pdf.Ln(5)

	// Tabela de postos e escalados
	colunas := []string{"POSTO / SERVIÇO", "HORÁRIO", "MILITAR ESCALADO", "ORIGEM / SETOR", "SITUAÇÃO"}
	larguras := []float64{45, 28, 48, 36, 25}
	pdfTabelaCabecalho(pdf, colunas, larguras)

	if len(d.Turnos) == 0 {
		pdf.SetFont("Helvetica", "I", 8.5)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(182, 8, T("Nenhum posto registrado nesta escala."), "1", 1, "C", false, 0, "")
	} else {
		for i, t := range d.Turnos {
			militar := t.MilitarGuerra
			if militar == "" {
				militar = t.MilitarNome
			}
			if militar == "" {
				militar = "[AGUARDANDO ESCALA]"
			}
			vals := []string{
				t.PostoNome,
				t.Horario,
				militar,
				t.SetorOuOrigem,
				strings.ToUpper(t.Status),
			}
			alinh := []string{"L", "C", "L", "L", "C"}
			pdfTabelaLinha(pdf, vals, larguras, alinh, i%2 == 1)
		}
	}

	// Assinatura física centralizada
	pdf.Ln(14)
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(50, 50, 50)
	pdf.CellFormat(0, 4, "________________________________________________________", "", 1, "C", false, 0, "")
	nomeResp := d.GeradoPor
	if d.HomologadoPor != "" {
		nomeResp = d.HomologadoPor
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(0, 5, T(nomeResp), "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(100, 116, 139)
	pdf.CellFormat(0, 4, T("Responsável pela Escala de Serviço"), "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------- RELATÓRIO DETALHADO (ordem SCI 06/10 — Frente D) ----------
// SIMPLES = o mesmo conteúdo do relatório geral (gerarRelatorioPDF).
// DETALHADO = página 1 idêntica ao simples + UMA FOLHA NOVA POR GRUPO
// (AddPage por grupo) com o registro linha a linha: data/hora, militar,
// situação, destino, observação, tag e — para pessoal do PRÓPRIO grupo — a
// coluna SETOR (seção). Cada grupo SUBORDINADO começa em folha própria.

// sitRotuloDetalhado: rótulo legível da situação no registro detalhado.
func sitRotuloDetalhado(s string) string {
	switch s {
	case "presente":
		return "Presente"
	case "atraso":
		return "Atraso"
	case "falta":
		return "Falta"
	case "justificada":
		return "Justificada"
	case "nao_verificado":
		return "NAO VERIF."
	}
	return s
}

// detalhadoRegistro: uma linha do registro detalhado já pronta para a tabela.
type detalhadoRegistro struct {
	Data       string // dd/mm/aaaa
	Hora       string // hh:mm (In(loc), nunca Format cru)
	Nome       string // nome de guerra (ou completo, conforme cadastro)
	Setor      string // seção — só exibida para pessoal do PRÓPRIO grupo
	Situacao   string
	Destino    string
	Observacao string
	Tag        string
}

// detalhadoGrupo: folha(s) de um grupo no modo detalhado.
type detalhadoGrupo struct {
	Nome      string
	Proprio   bool // grupo do escopo → exibe coluna SETOR
	Registros []detalhadoRegistro
}

// registrosDetalhadoGrupo: lançamentos do período para UM conjunto de grupos
// (grupo próprio +, em chamadas separadas, cada subordinado), um registro por
// linha de presença. Consulta AUXILIAR única — pool de 1 conexão: sem join na
// view agregada, subselect correlacionado no ORDER BY, rows drenadas p/ slice
// antes de qualquer outra query.
func (a *App) registrosDetalhadoGrupo(de, ate string, grupoIDs []int64) ([]detalhadoRegistro, error) {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(grupoIDs)), ",")
	args := make([]any, 0, len(grupoIDs)+2)
	for _, id := range grupoIDs {
		args = append(args, id)
	}
	args = append(args, de, ate)
	q := `
SELECT f.data,
       COALESCE(f.hora,''),
       COALESCE(NULLIF(p.nome_completo,''), p.nome_guerra),
       COALESCE(NULLIF(s.sigla,''), s.nome, 'INDEFINIDO'),
       pr.situacao,
       CASE WHEN pr.situacao = 'justificada' THEN COALESCE(d.nome,'') ELSE '' END,
       COALESCE(pr.observacao,''),
       COALESCE(t.nome,'')
FROM presencas pr
JOIN conferencias f ON f.id = pr.conferencia_id
JOIN pessoas p ON p.id = pr.pessoa_id
LEFT JOIN setores s ON s.id = p.setor_id
LEFT JOIN destinos d ON d.id = pr.destino_id
LEFT JOIN tags t ON t.id = pr.tag_id
WHERE p.grupo_id IN (` + ph + `) AND f.data BETWEEN ? AND ?
ORDER BY f.data, f.hora,
         (SELECT COUNT(*) FROM presencas pr2
            JOIN conferencias f2 ON f2.id = pr2.conferencia_id
           WHERE pr2.pessoa_id = pr.pessoa_id
             AND f2.data <= f.data),
         p.nome_guerra COLLATE NOCASE`
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []detalhadoRegistro{}
	for rows.Next() {
		var r detalhadoRegistro
		var hora string
		if err := rows.Scan(&r.Data, &hora, &r.Nome, &r.Setor, &r.Situacao,
			&r.Destino, &r.Observacao, &r.Tag); err != nil {
			return nil, err
		}
		r.Data = fmtDataBR(r.Data + "T12:00:00Z")[:10] // dd/mm/aaaa — meio-dia UTC = mesmo dia em BRT
		if hora != "" {
			// hora é "time-of-day" local da conferência (não timestamp): exibe
			// HH:MM sem operação de fuso — nunca um Format cru de timestamp.
			if partes := strings.Split(hora, ":"); len(partes) >= 2 {
				r.Hora = partes[0] + ":" + partes[1]
			} else {
				r.Hora = hora
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// folhasDetalhado: monta as folhas do modo detalhado — grupo do escopo primeiro
// (com coluna SETOR), depois cada subordinado ATIVO (transitivo, vínculo
// bilateral) em folha própria. Erro de consulta PROPAGA — relatório
// silenciosamente incompleto é falha grave.
func (a *App) folhasDetalhado(de, ate string, escopo int64) ([]detalhadoGrupo, error) {
	folhas := []detalhadoGrupo{}
	ids := []int64{escopo}
	nomeProprio := ""
	_ = a.st.db.QueryRow(`SELECT COALESCE(nome,'') FROM grupos WHERE id = ?`, escopo).Scan(&nomeProprio)
	folhas = append(folhas, detalhadoGrupo{Nome: nomeProprio, Proprio: true})

	// subordinados: nome + registros por grupo, um bloco de queries por vez
	subs := a.gruposSubordinadosAtivos(escopo)
	nomes := map[int64]string{}
	for _, sid := range subs {
		var n string
		_ = a.st.db.QueryRow(`SELECT COALESCE(nome,'') FROM grupos WHERE id = ?`, sid).Scan(&n)
		nomes[sid] = n
	}

	// 1º bloco: grupo próprio — pool de 1 conexão: rows drenadas p/ slice
	// ANTES do próximo bloco de queries (QueryRow de nome já consumido acima).
	reg, err := a.registrosDetalhadoGrupo(de, ate, ids)
	if err != nil {
		return nil, err
	}
	folhas[0].Registros = reg
	for _, sid := range subs {
		regSub, errSub := a.registrosDetalhadoGrupo(de, ate, []int64{sid})
		if errSub != nil {
			return nil, errSub
		}
		folhas = append(folhas, detalhadoGrupo{Nome: nomes[sid], Proprio: false, Registros: regSub})
	}
	return folhas, nil
}

// detalhadoTabelaGrupo: desenha cabeçalho + linhas de UM grupo, repetindo o
// cabeçalho nas folhas de continuação. Tabelas SEMPRE P&B (ordem 28/09).
func (a *App) detalhadoTabelaGrupo(pdf *fpdf.Fpdf, g detalhadoGrupo) {
	T := cp1252Traduz.Replace
	rotulo := "GRUPO: " + g.Nome
	if g.Proprio {
		rotulo += "  (GRUPO DO ESCOPO)"
	}
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetTextColor(30, 41, 59)
	pdf.Cell(0, 6, T(rotulo))
	pdf.Ln(7)

	cab := []string{"Data", "Hora", "Militar", "Situação", "Destino", "Observação", "Tag"}
	larg := []float64{20, 13, 42, 22, 24, 45, 16}
	if g.Proprio {
		// pessoal do próprio grupo: coluna SETOR (seção) cabível
		cab = []string{"Data", "Hora", "Militar", "Setor", "Situação", "Destino", "Observação", "Tag"}
		larg = []float64{18, 12, 36, 22, 20, 22, 36, 16}
	}
	alinh := []string{"C", "C", "L", "C", "C", "L", "L", "C"}
	if g.Proprio {
		alinh = []string{"C", "C", "L", "C", "C", "L", "L", "C"}
	}
	pdfTabelaCabecalho(pdf, cab, larg)
	if len(g.Registros) == 0 {
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(100, 116, 139)
		msg := "Sem lançamentos no período."
		if g.Proprio {
			msg = "Sem lançamentos no período para o grupo do escopo."
		}
		pdf.CellFormat(0, 6, T(msg), "1", 1, "C", false, 0, "")
		return
	}
	for i, r := range g.Registros {
		if pdf.GetY() > 262 {
			pdf.AddPage()
			// repetição do cabeçalho do grupo + da tabela nas folhas de continuação
			pdf.SetFont("Helvetica", "B", 9.5)
			pdf.SetTextColor(30, 41, 59)
			pdf.Cell(0, 6, T(rotulo+" (continuação)"))
			pdf.Ln(7)
			pdfTabelaCabecalho(pdf, cab, larg)
		}
		vals := []string{r.Data, r.Hora, T(r.Nome), sitRotuloDetalhado(r.Situacao),
			T(r.Destino), T(r.Observacao), T(r.Tag)}
		if g.Proprio {
			vals = []string{r.Data, r.Hora, T(r.Nome), T(r.Setor),
				sitRotuloDetalhado(r.Situacao), T(r.Destino), T(r.Observacao), T(r.Tag)}
		}
		pdfTabelaLinha(pdf, vals, larg, alinh, i%2 == 1)
	}
}

// relatorioDetalhadoMulti: variante para o ADMIN (escopo global) — mesmas
// páginas de resumo, folha PRÓPRIA para cada grupo ativo informado (um por
// folha, modelo do Diretor), sem coluna SETOR (nenhum é "grupo do escopo").
func (a *App) relatorioDetalhadoMulti(de, ate, modo string, ids []int64) ([]byte, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("nenhum grupo informado")
	}
	b := a.montarBundle(de, ate, 0) // 0 = visão global (todas as grupos)
	sub := fmt.Sprintf("Período: %s a %s · Efetivo Pronto: %.1f%%", b.De, b.Ate, b.PctPronto)
	pdf := a.novoPDF("P", "RELATÓRIO GERAL DE EFETIVO & CONFERÊNCIAS", sub, "")
	a.relatorioSimplesRender(pdf, b) // página 1 = resumo idêntico ao SIMPLES
	if modo != "detalhado" {
		var buf bytes.Buffer
		if err := pdf.Output(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	for _, gid := range ids {
		reg, err := a.registrosDetalhadoGrupo(de, ate, []int64{gid})
		if err != nil {
			return nil, err
		}
		var nome string
		_ = a.st.db.QueryRow(`SELECT COALESCE(nome,'') FROM grupos WHERE id = ?`, gid).Scan(&nome)
		pdf.AddPage() // UMA FOLHA NOVA POR GRUPO — sempre
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(15, 23, 42)
		pdf.Cell(0, 6, cp1252Traduz.Replace(fmt.Sprintf("%s — PERIODO: %s A %s", nome, fmtDataBR(de+"T12:00:00Z")[:10], fmtDataBR(ate+"T12:00:00Z")[:10])))
		pdf.Ln(8)
		a.detalhadoTabelaGrupo(pdf, detalhadoGrupo{Nome: nome, Proprio: false, Registros: reg})
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// relatorioDetalhado: ponto de entrada do PDF por período com modos.
func (a *App) relatorioDetalhado(de, ate, modo string, escopo int64) ([]byte, error) {
	if escopo <= 0 {
		return nil, fmt.Errorf("modo detalhado exige grupo do escopo")
	}
	b := a.montarBundle(de, ate, escopo)
	sub := fmt.Sprintf("Período: %s a %s · Efetivo Pronto: %.1f%%", b.De, b.Ate, b.PctPronto)
	pdf := a.novoPDF("P", "RELATÓRIO GERAL DE EFETIVO & CONFERÊNCIAS", sub, "")
	a.relatorioSimplesRender(pdf, b) // página 1 = resumo idêntico ao SIMPLES
	if modo != "detalhado" {
		var buf bytes.Buffer
		if err := pdf.Output(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	folhas, err := a.folhasDetalhado(de, ate, escopo)
	if err != nil {
		return nil, err
	}
	for _, g := range folhas {
		pdf.AddPage() // UMA FOLHA NOVA POR GRUPO — sempre, mesmo grupo sem lançamentos
		// Cabeçalho de folha: nome do grupo + período
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(15, 23, 42)
		pdf.Cell(0, 6, cp1252Traduz.Replace(fmt.Sprintf("%s — PERIODO: %s A %s", g.Nome, fmtDataBR(de + "T12:00:00Z")[:10], fmtDataBR(ate + "T12:00:00Z")[:10])))
		pdf.Ln(8)
		a.detalhadoTabelaGrupo(pdf, g)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
