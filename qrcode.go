package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
)

// QR Code Encoder leve, autocontido e estritamente offline (sem dependências externas).
// Suporta versões 1 a 4 (até 78 bytes no nível L / 62 bytes no M), ideal para payloads sci://...

type QRECCLevel int

const (
	QRECC_L QRECCLevel = 0 // ~7% recuperação
	QRECC_M QRECCLevel = 1 // ~15% recuperação
)

type qrConfig struct {
	version    int
	ecc        QRECCLevel
	size       int // matriz size: 17 + 4*version
	dataBytes  int
	eccBytes   int
	alignPos   int // centro do alignment pattern (0 para v1)
	formatBits uint16
}

var qrConfigs = map[int]map[QRECCLevel]qrConfig{
	1: {
		QRECC_L: {version: 1, ecc: QRECC_L, size: 21, dataBytes: 19, eccBytes: 7, alignPos: 0, formatBits: 0x77c4},
		QRECC_M: {version: 1, ecc: QRECC_M, size: 21, dataBytes: 16, eccBytes: 10, alignPos: 0, formatBits: 0x5412},
	},
	2: {
		QRECC_L: {version: 2, ecc: QRECC_L, size: 25, dataBytes: 34, eccBytes: 10, alignPos: 18, formatBits: 0x77c4},
		QRECC_M: {version: 2, ecc: QRECC_M, size: 25, dataBytes: 28, eccBytes: 16, alignPos: 18, formatBits: 0x5412},
	},
	3: {
		QRECC_L: {version: 3, ecc: QRECC_L, size: 29, dataBytes: 55, eccBytes: 15, alignPos: 22, formatBits: 0x77c4},
		QRECC_M: {version: 3, ecc: QRECC_M, size: 29, dataBytes: 44, eccBytes: 26, alignPos: 22, formatBits: 0x5412},
	},
	4: {
		QRECC_L: {version: 4, ecc: QRECC_L, size: 33, dataBytes: 80, eccBytes: 20, alignPos: 26, formatBits: 0x77c4},
		QRECC_M: {version: 4, ecc: QRECC_M, size: 33, dataBytes: 64, eccBytes: 36, alignPos: 26, formatBits: 0x5412},
	},
}

// GF(256) com polinômio primitivo 0x11D (285)
var gfExp [512]byte
var gfLog [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfExp[i+255] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x >= 256 {
			x ^= 0x11D
		}
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func rsGenPoly(numECC int) []byte {
	g := []byte{1}
	for i := 0; i < numECC; i++ {
		next := make([]byte, len(g)+1)
		for j, coef := range g {
			next[j] ^= gfMul(coef, gfExp[i])
			next[j+1] ^= coef
		}
		g = next
	}
	return g
}

func rsEncode(data []byte, numECC int) []byte {
	gen := rsGenPoly(numECC)
	msg := make([]byte, len(data)+numECC)
	copy(msg, data)
	for i := 0; i < len(data); i++ {
		coef := msg[i]
		if coef != 0 {
			for j := 0; j < len(gen); j++ {
				msg[i+j] ^= gfMul(gen[j], coef)
			}
		}
	}
	return msg[len(data):]
}

type QRCode struct {
	Matrix [][]bool
	Size   int
}

func GerarQRCode(texto string) (*QRCode, error) {
	bytesTexto := []byte(texto)
	tam := len(bytesTexto)
	// Escolhe menor versão que suporte
	var cfg qrConfig
	encontrou := false
	for v := 1; v <= 4; v++ {
		c := qrConfigs[v][QRECC_M]
		// Byte mode header: 4 bits mode (0100) + 8 bits char count + data
		capBytes := c.dataBytes - 2
		if tam <= capBytes {
			cfg = c
			encontrou = true
			break
		}
	}
	if !encontrou {
		// Tenta nível L se não couber no M
		for v := 1; v <= 4; v++ {
			c := qrConfigs[v][QRECC_L]
			capBytes := c.dataBytes - 2
			if tam <= capBytes {
				cfg = c
				encontrou = true
				break
			}
		}
	}
	if !encontrou {
		return nil, fmt.Errorf("payload excede a capacidade suportada (%d bytes)", tam)
	}

	// 1. Bitstream com Byte Mode
	var bits []bool
	addBits := func(val uint32, count int) {
		for i := count - 1; i >= 0; i-- {
			bits = append(bits, (val>>i)&1 == 1)
		}
	}

	// Modo byte = 0100
	addBits(0b0100, 4)
	// Char count = 8 bits para v1-v9
	addBits(uint32(tam), 8)
	// Dados
	for _, b := range bytesTexto {
		addBits(uint32(b), 8)
	}
	// Terminator (até 4 zeros)
	restantes := cfg.dataBytes*8 - len(bits)
	if restantes > 4 {
		restantes = 4
	}
	addBits(0, restantes)

	// Alinhar a byte
	if len(bits)%8 != 0 {
		addBits(0, 8-(len(bits)%8))
	}

	// Pad bytes alternando 0xEC e 0x11
	data := make([]byte, len(bits)/8)
	for i := 0; i < len(data); i++ {
		var b byte
		for j := 0; j < 8; j++ {
			if bits[i*8+j] {
				b |= 1 << (7 - j)
			}
		}
		data[i] = b
	}

	pad := []byte{0xEC, 0x11}
	padIdx := 0
	for len(data) < cfg.dataBytes {
		data = append(data, pad[padIdx%2])
		padIdx++
	}

	// 2. Reed-Solomon ECC
	ecc := rsEncode(data, cfg.eccBytes)
	codewordTotal := append(data, ecc...)

	// 3. Matriz e Funções Fixas
	N := cfg.size
	mat := make([][]bool, N)
	reservado := make([][]bool, N)
	for i := range mat {
		mat[i] = make([]bool, N)
		reservado[i] = make([]bool, N)
	}

	setModule := func(r, c int, val bool) {
		mat[r][c] = val
		reservado[r][c] = true
	}

	// Finder patterns (7x7) + separators (8x8)
	drawFinder := func(r0, c0 int) {
		for dr := -1; dr <= 7; dr++ {
			for dc := -1; dc <= 7; dc++ {
				r := r0 + dr
				c := c0 + dc
				if r >= 0 && r < N && c >= 0 && c < N {
					val := false
					if dr >= 0 && dr <= 6 && dc >= 0 && dc <= 6 {
						if dr == 0 || dr == 6 || dc == 0 || dc == 6 || (dr >= 2 && dr <= 4 && dc >= 2 && dc <= 4) {
							val = true
						}
					}
					setModule(r, c, val)
				}
			}
		}
	}
	drawFinder(0, 0)
	drawFinder(0, N-7)
	drawFinder(N-7, 0)

	// Alignment pattern para versão >= 2
	if cfg.alignPos > 0 {
		ap := cfg.alignPos
		for dr := -2; dr <= 2; dr++ {
			for dc := -2; dc <= 2; dc++ {
				val := (dr == -2 || dr == 2 || dc == -2 || dc == 2 || (dr == 0 && dc == 0))
				setModule(ap+dr, ap+dc, val)
			}
		}
	}

	// Timing patterns
	for i := 8; i < N-8; i++ {
		val := (i % 2 == 0)
		if !reservado[6][i] {
			setModule(6, i, val)
		}
		if !reservado[i][6] {
			setModule(i, 6, val)
		}
	}

	// Dark module
	setModule(4*cfg.version+9, 8, true)

	// Reservar áreas de Formato (ao redor dos finders)
	for i := 0; i <= 8; i++ {
		reservado[8][i] = true
		reservado[i][8] = true
		reservado[8][N-1-i] = true
		reservado[N-1-i][8] = true
	}

	// 4. Inserção de dados em zig-zag
	allBits := make([]bool, len(codewordTotal)*8)
	for i, b := range codewordTotal {
		for j := 0; j < 8; j++ {
			allBits[i*8+j] = ((b >> (7 - j)) & 1) == 1
		}
	}

	bitIdx := 0
	subindo := true
	for c := N - 1; c > 0; c -= 2 {
		if c == 6 {
			c-- // Pula timing pattern vertical
		}
		var rStart, rEnd, rStep int
		if subindo {
			rStart, rEnd, rStep = N-1, -1, -1
		} else {
			rStart, rEnd, rStep = 0, N, 1
		}
		for r := rStart; r != rEnd; r += rStep {
			for dc := 0; dc < 2; dc++ {
				col := c - dc
				if !reservado[r][col] {
					bit := false
					if bitIdx < len(allBits) {
						bit = allBits[bitIdx]
						bitIdx++
					}
					// Aplica Mask 0: (row + col) % 2 == 0
					if (r+col)%2 == 0 {
						bit = !bit
					}
					mat[r][col] = bit
				}
			}
		}
		subindo = !subindo
	}

	// 5. Escrever Format Info (Mask 0)
	fbits := cfg.formatBits
	// Formato em torno do finder do canto superior esquerdo
	// bits 0..5 no (8, 0..5)
	for i := 0; i <= 5; i++ {
		mat[8][i] = ((fbits >> (14 - i)) & 1) == 1
	}
	mat[8][7] = ((fbits >> 8) & 1) == 1
	mat[8][8] = ((fbits >> 7) & 1) == 1
	mat[7][8] = ((fbits >> 6) & 1) == 1
	for i := 0; i <= 5; i++ {
		mat[5-i][8] = ((fbits >> (5 - i)) & 1) == 1
	}

	// Segundo conjunto de formato (canto superior direito e inferior esquerdo)
	for i := 0; i <= 7; i++ {
		mat[8][N-1-i] = ((fbits >> i) & 1) == 1
	}
	for i := 0; i <= 6; i++ {
		mat[N-7+i][8] = ((fbits >> (8 + i)) & 1) == 1
	}

	return &QRCode{Matrix: mat, Size: N}, nil
}

// RenderPNG desenha o QRCode com margem (quiet zone) e escala especificada
func (qr *QRCode) RenderPNG(escala int, quietZone int) ([]byte, error) {
	if escala <= 0 {
		escala = 8
	}
	if quietZone < 0 {
		quietZone = 4
	}
	totalSize := (qr.Size + 2*quietZone) * escala
	img := image.NewRGBA(image.Rect(0, 0, totalSize, totalSize))
	branco := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	preto := color.RGBA{R: 15, G: 23, B: 42, A: 255} // Slate-900 militar

	// Preencher branco
	for y := 0; y < totalSize; y++ {
		for x := 0; x < totalSize; x++ {
			img.Set(x, y, branco)
		}
	}

	// Desenhar módulos
	for r := 0; r < qr.Size; r++ {
		for c := 0; c < qr.Size; c++ {
			if qr.Matrix[r][c] {
				x0 := (c + quietZone) * escala
				y0 := (r + quietZone) * escala
				for dy := 0; dy < escala; dy++ {
					for dx := 0; dx < escala; dx++ {
						img.Set(x0+dx, y0+dy, preto)
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderSVG gera um vetor SVG leve e escalável inline
func (qr *QRCode) RenderSVG(w io.Writer, tamanhoPx int) error {
	quietZone := 4
	totalM := qr.Size + 2*quietZone
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, totalM, totalM, tamanhoPx, tamanhoPx)
	fmt.Fprint(w, `<rect width="100%" height="100%" fill="#ffffff"/>`)
	fmt.Fprint(w, `<path fill="#0f172a" d="`)
	for r := 0; r < qr.Size; r++ {
		for c := 0; c < qr.Size; c++ {
			if qr.Matrix[r][c] {
				fmt.Fprintf(w, "M%d %dh1v1h-1z ", c+quietZone, r+quietZone)
			}
		}
	}
	fmt.Fprint(w, `"/>`)
	fmt.Fprint(w, `</svg>`)
	return nil
}
