package pdfplumber

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"regexp"
	"strings"

	pdfoxide "github.com/yfedoseev/pdf_oxide/go"
)

type Char struct {
	Text              string    `json:"text"`
	Fontname          string    `json:"fontname"`
	Size              float64   `json:"size"`
	X0                float64   `json:"x0"`
	X1                float64   `json:"x1"`
	Top               float64   `json:"top"`
	Bottom            float64   `json:"bottom"`
	Width             float64   `json:"width"`
	Height            float64   `json:"height"`
	Doctop            float64   `json:"doctop"`
	Matrix            [6]float64 `json:"matrix"`
	Upright           bool      `json:"upright"`
	StrokingColor     string    `json:"stroking_color"`
	NonStrokingColor  string    `json:"non_stroking_color"`
	Ncs               string    `json:"ncs"`
	Adv               float64   `json:"adv"`
	PageNumber        int       `json:"page_number"`
}

type Document struct {
	inner *pdfoxide.PdfDocument
}

func Open(path string) (*Document, error) {
	doc, err := pdfoxide.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: open %s: %w", path, err)
	}
	return &Document{inner: doc}, nil
}

func OpenBytes(data []byte) (*Document, error) {
	doc, err := pdfoxide.OpenFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: open from bytes: %w", err)
	}
	return &Document{inner: doc}, nil
}

func (d *Document) Close() {
	if d.inner != nil {
		d.inner.Close()
		d.inner = nil
	}
}

func (d *Document) PageCount() int {
	n, err := d.inner.PageCount()
	if err != nil {
		return 0
	}
	return n
}

func (d *Document) GetPageChars(pageIdx int) ([]Char, error) {
	n, err := d.inner.PageCount()
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: page count: %w", err)
	}
	if pageIdx < 0 || pageIdx >= n {
		return nil, fmt.Errorf("pdfplumber: page index %d out of range (pages: %d)", pageIdx, n)
	}
	raw, err := d.inner.ExtractChars(pageIdx)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: extract chars page %d: %w", pageIdx, err)
	}
	chars := make([]Char, len(raw))
	for i, c := range raw {
		x0 := float64(c.X)
		top := float64(c.Y)
		w := float64(c.Width)
		h := float64(c.Height)
		fs := float64(c.FontSize)
		chars[i] = Char{
			Text:             string(c.Char),
			Fontname:         c.FontName,
			Size:             fs,
			X0:               x0,
			X1:               x0 + w,
			Top:              top,
			Bottom:           top + h,
			Width:            w,
			Height:           h,
			Doctop:           top,
			Matrix:           [6]float64{fs, 0, 0, fs, x0, top},
			Upright:          true,
			StrokingColor:    "",
			NonStrokingColor: "",
			Ncs:              "",
			Adv:              fs * 0.5,
			PageNumber:       pageIdx + 1,
		}
	}
	return chars, nil
}

func (d *Document) GetDedupePageChars(pageIdx int, tolerance float64) ([]Char, error) {
	chars, err := d.GetPageChars(pageIdx)
	if err != nil {
		return nil, err
	}
	return dedupeChars(chars, tolerance), nil
}

func dedupeChars(chars []Char, tolerance float64) []Char {
	if len(chars) == 0 {
		return nil
	}
	result := make([]Char, 0, len(chars))
	for _, ch := range chars {
		dup := false
		for _, existing := range result {
			ox := math.Max(0, math.Min(ch.X1, existing.X1)-math.Max(ch.X0, existing.X0))
			oy := math.Max(0, math.Min(ch.Bottom, existing.Bottom)-math.Max(ch.Top, existing.Top))
			oa := ox * oy
			if oa <= 0 {
				continue
			}
			ca := (ch.X1 - ch.X0) * (ch.Bottom - ch.Top)
			ea := (existing.X1 - existing.X0) * (existing.Bottom - existing.Top)
			maxA := math.Max(ca, ea)
			ratio := oa / maxA
			sameFont := ch.Fontname == existing.Fontname
			sameSize := math.Abs(ch.Size-existing.Size) <= tolerance
			if ratio > 0.5 && sameFont && sameSize {
				dup = true
				break
			}
		}
		if !dup {
			result = append(result, ch)
		}
	}
	return result
}

type RenderResult struct {
	Data     []byte
	Width    int
	Height   int
	Channels int
}

func RenderPage(pdfData []byte, pageIdx int, dpi float64) (*RenderResult, error) {
	if len(pdfData) == 0 {
		return nil, fmt.Errorf("pdfplumber: empty PDF data for rendering")
	}
	doc, err := pdfoxide.OpenFromBytes(pdfData)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: open for render: %w", err)
	}
	defer doc.Close()

	pixmap, err := doc.RenderPageRaw(pageIdx, int(math.Round(dpi)))
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: render page %d: %w", pageIdx, err)
	}

	data := make([]byte, len(pixmap.Data))
	for i := 0; i < len(pixmap.Data); i += 4 {
		a := pixmap.Data[i+3]
		if a == 0 {
			data[i] = 0; data[i+1] = 0; data[i+2] = 0; data[i+3] = 0
		} else {
			data[i] = uint8(math.Min(255, float64(pixmap.Data[i])*255/float64(a)))
			data[i+1] = uint8(math.Min(255, float64(pixmap.Data[i+1])*255/float64(a)))
			data[i+2] = uint8(math.Min(255, float64(pixmap.Data[i+2])*255/float64(a)))
			data[i+3] = a
		}
	}
	return &RenderResult{Data: data, Width: pixmap.Width, Height: pixmap.Height, Channels: 4}, nil
}

func (r *RenderResult) ToImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.Width, r.Height))
	copy(img.Pix, r.Data)
	return img
}

func (r *RenderResult) ColorModel() color.Model { return color.RGBAModel }
func (r *RenderResult) Bounds() image.Rectangle { return image.Rect(0, 0, r.Width, r.Height) }
func (r *RenderResult) At(x, y int) color.Color {
	if x < 0 || x >= r.Width || y < 0 || y >= r.Height {
		return color.RGBA{}
	}
	idx := (y*r.Width + x) * r.Channels
	if r.Channels >= 4 {
		return color.RGBA{R: r.Data[idx], G: r.Data[idx+1], B: r.Data[idx+2], A: r.Data[idx+3]}
	}
	return color.RGBA{R: r.Data[idx], G: r.Data[idx+1], B: r.Data[idx+2], A: 255}
}

func InitRenderer(path string) error { return nil }

var noisePattern = regexp.MustCompile(`^[a-zT_\[\]\(\\)-]+$`)

func HasColor(c *Char) bool {
	if c.Ncs == "DeviceGray" {
		if len(c.StrokingColor) > 0 && c.StrokingColor[0] == '1' &&
			len(c.NonStrokingColor) > 0 && c.NonStrokingColor[0] == '1' {
			if noisePattern.MatchString(c.Text) {
				return false
			}
		}
	}
	return true
}

func IsGarbledChar(ch rune) bool {
	cp := int(ch)
	if cp >= 0xE000 && cp <= 0xF8FF { return true }
	if cp >= 0xF0000 && cp <= 0xFFFFF { return true }
	if cp >= 0x100000 && cp <= 0x10FFFF { return true }
	if cp == 0xFFFD { return true }
	if cp < 0x20 && ch != '\t' && ch != '\n' && ch != '\r' { return true }
	if cp >= 0x80 && cp <= 0x9F { return true }
	// Surrogate (Cs category): Unicode surrogates D800-DFFF
	if cp >= 0xD800 && cp <= 0xDFFF { return true }
	// Unassigned (Cn category): noncharacters and reserved
	if cp == 0xFFFE || cp == 0xFFFF { return true }
	if cp >= 0x1FFFE && cp <= 0x1FFFF { return true }
	if cp >= 0x2FFFE && cp <= 0x2FFFF { return true }
	if cp >= 0x3FFFE && cp <= 0x3FFFF { return true }
	if cp >= 0x4FFFE && cp <= 0x4FFFF { return true }
	if cp >= 0x5FFFE && cp <= 0x5FFFF { return true }
	if cp >= 0x6FFFE && cp <= 0x6FFFF { return true }
	if cp >= 0x7FFFE && cp <= 0x7FFFF { return true }
	if cp >= 0x8FFFE && cp <= 0x8FFFF { return true }
	if cp >= 0x9FFFE && cp <= 0x9FFFF { return true }
	if cp >= 0xAFFFE && cp <= 0xAFFFF { return true }
	if cp >= 0xBFFFE && cp <= 0xBFFFF { return true }
	if cp >= 0xCFFFE && cp <= 0xCFFFF { return true }
	if cp >= 0xDFFFE && cp <= 0xDFFFF { return true }
	if cp >= 0xEFFFE && cp <= 0xEFFFF { return true }
	if cp >= 0xFFFFE && cp <= 0xFFFFF { return true }
	if cp >= 0x10FFFE && cp <= 0x10FFFF { return true }
	return false
}

func IsGarbledText(text string, threshold float64) bool {
	if len(text) == 0 {
		return false
	}
	re := regexp.MustCompile(`\(cid\s*:\s*\d+\s*\)`)
	if re.MatchString(text) {
		return true
	}
	garbledCount := 0
	total := 0
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		total++
		if IsGarbledChar(r) {
			garbledCount++
		}
	}
	if total == 0 {
		return false
	}
	return float64(garbledCount)/float64(total) >= threshold
}

func HasSubsetFontPrefix(fontname string) bool {
	if len(fontname) < 3 {
		return false
	}
	re := regexp.MustCompile(`^[A-Z0-9]{2,6}\+`)
	return re.MatchString(fontname)
}

func IsGarbledByFontEncoding(chars []Char, minChars int) bool {
	if len(chars) < minChars {
		return false
	}
	subsetFontCount := 0
	totalNonSpace := 0
	asciiPunctSym := 0
	cjkLike := 0

	for _, c := range chars {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		totalNonSpace++
		if HasSubsetFontPrefix(c.Fontname) {
			subsetFontCount++
		}
		cp := int([]rune(text)[0])
		if (cp >= 0x2E80 && cp <= 0x9FFF) || (cp >= 0xF900 && cp <= 0xFAFF) ||
			(cp >= 0x20000 && cp <= 0x2FA1F) ||
			(cp >= 0xAC00 && cp <= 0xD7AF) ||
			(cp >= 0x3040 && cp <= 0x30FF) {
			cjkLike++
		} else if (cp >= 0x21 && cp <= 0x2F) || (cp >= 0x3A && cp <= 0x40) ||
			(cp >= 0x5B && cp <= 0x60) || (cp >= 0x7B && cp <= 0x7E) {
			asciiPunctSym++
		}
	}
	if totalNonSpace < minChars {
		return false
	}
	subsetRatio := float64(subsetFontCount) / float64(totalNonSpace)
	if subsetRatio < 0.3 {
		return false
	}
	cjkRatio := float64(cjkLike) / float64(totalNonSpace)
	punctRatio := float64(asciiPunctSym) / float64(totalNonSpace)
	if cjkRatio < 0.05 && punctRatio > 0.4 {
		return true
	}
	return false
}

// ── RAGFlow char utility functions ──────────────────────────────────

// CharWidth computes (x1-x0) / max(len(text), 1), matching RAGFlow's __char_width.
func CharWidth(c *Char) float64 {
	l := len(c.Text)
	if l == 0 {
		l = 1
	}
	return (c.X1 - c.X0) / float64(l)
}

// CharHeight returns bottom - top, matching RAGFlow's __height.
func CharHeight(c *Char) float64 {
	return c.Bottom - c.Top
}

// XDis computes the minimum horizontal distance between two chars,
// matching RAGFlow's _x_dis.
func XDis(a, b *Char) float64 {
	d1 := math.Abs(a.X1 - b.X0)
	d2 := math.Abs(a.X0 - b.X1)
	d3 := math.Abs(a.X0+a.X1-b.X0-b.X1) / 2.0
	return math.Min(d1, math.Min(d2, d3))
}

// YDis computes the vertical distance between two chars (center to center),
// matching RAGFlow's _y_dis.
func YDis(a, b *Char) float64 {
	return (b.Top + b.Bottom - a.Top - a.Bottom) / 2.0
}

// TotalPageNumber opens a PDF and returns the page count.
func TotalPageNumber(path string, data []byte) (int, error) {
	var doc *Document
	var err error
	if data != nil {
		doc, err = OpenBytes(data)
	} else {
		doc, err = Open(path)
	}
	if err != nil {
		return 0, err
	}
	defer doc.Close()
	return doc.PageCount(), nil
}

// SortXByPage sorts chars by page_number, x0, top and restores order
// when x-difference is small, matching RAGFlow's sort_X_by_page.
func SortXByPage(chars []Char, threshold float64) []Char {
	sorted := make([]Char, len(chars))
	copy(sorted, chars)
	// Sort by page_number, x0, top
	// Bubble sort to match RAGFlow's insertion-style algorithm
	for i := 0; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			a, b := sorted[j-1], sorted[j]
			if a.PageNumber != b.PageNumber {
				break
			}
			if math.Abs(b.X0-a.X0) < threshold && b.Top < a.Top {
				sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
			} else {
				break
			}
		}
	}
	return sorted
}
