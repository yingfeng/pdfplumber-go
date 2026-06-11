# pdfplumber-go

Pure Go pdfplumber-compatible PDF library, backed by pdf_oxide engine.
Supports all pdfplumber Char fields and RAGFlow pdf_parser.py utility functions.

No Rust compilation, no local pdf_oxide checkout, no Python bindings.

## Prerequisites

- Go 1.21+
- CGo enabled (`CGO_ENABLED=1`)

## Installation

```bash
# 1. Install the native pdf_oxide library
#    Network may need a SOCKS5 proxy:
https_proxy=socks5h://127.0.0.1:7890 \
CGO_ENABLED=1 go run github.com/yfedoseev/pdf_oxide/go/cmd/install@v0.3.63

# 2. The installer prints environment variables to export, e.g.:
export CGO_CFLAGS="-I$HOME/.cache/pdf_oxide/v0.3.63/include"
export CGO_LDFLAGS="$HOME/.cache/pdf_oxide/v0.3.63/lib/linux_amd64/libpdf_oxide.a -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"

# 3. Add to your project:
#    go get github.com/yfedoseev/pdf_oxide/go
```

## RAGFlow pdf_parser.py API mapping

When porting RAGFlow's `pdf_parser.py` to Go, use the following mapping
to replace pdfplumber and pypdf.

### 1. Document lifecycle

| Python (pdfplumber / pypdf) | Go (pdfplumber-go) |
|---|---|
| `pdfplumber.open(path)` | `doc, err := pdfplumber.Open(path)` |
| `pdfplumber.open(BytesIO(data))` | `doc, err := pdfplumber.OpenBytes(data)` |
| `pdf2_read(path)` | `doc, err := pdfplumber.Open(path)` |
| `pdf2_read(BytesIO(data))` | `doc, err := pdfplumber.OpenBytes(data)` |
| `len(pdf.pages)` | `doc.PageCount()` |
| `pdf.close()` | `doc.Close()` |

### 2. Character extraction & rendering

| Python | Go |
|---|---|
| `page.dedupe_chars().chars` | `chars, _ := doc.GetDedupePageChars(pageIdx, 1.0)` |
| `page.chars` (raw) | `chars, _ := doc.GetPageChars(pageIdx)` |
| `page.to_image(resolution, antialias=True).annotated` | `res, _ := pdfplumber.RenderPage(pdfBytes, pageIdx, dpi)` → `img := res.ToImage()` |
| `page.extract_text()` (pypdf PlainParser) | `text, _ := doc.GetPageText(pageIdx)` |

### 3. Char field access (dict key → struct field)

| Python `c["..."]` | Go `c....` | Type |
|---|---|---|
| `c["text"]` | `c.Text` | `string` |
| `c["fontname"]` | `c.Fontname` | `string` |
| `c["x0"]`, `c["x1"]` | `c.X0`, `c.X1` | `float64` |
| `c["top"]`, `c["bottom"]` | `c.Top`, `c.Bottom` | `float64` |
| `c["width"]`, `c["height"]` | `c.Width`, `c.Height` | `float64` |
| `c["ncs"]` | `c.Ncs` | `string` |
| `c["stroking_color"]` | `c.StrokingColor` | `string` |
| `c["non_stroking_color"]` | `c.NonStrokingColor` | `string` |
| `c["page_number"]` | `c.PageNumber` | `int` |
| `c["size"]` | `c.Size` | `float64` |
| `c.get("text", "")` | `if c.Text != ""` | |
| `c.get("fontname", "")` | `if c.Fontname != ""` | |

### 4. RAGFlow utility functions (1:1 match)

| pdf_parser.py | pdfplumber-go | Status |
|---|---|---|
| `_has_color(o)` | `pdfplumber.HasColor(&char)` | ✅ |
| `_is_garbled_char(ch)` | `pdfplumber.IsGarbledChar(rune)` | ✅ (includes Cn/Cs) |
| `_is_garbled_text(text, threshold)` | `pdfplumber.IsGarbledText(text, threshold)` | ✅ |
| `_has_subset_font_prefix(fontname)` | `pdfplumber.HasSubsetFontPrefix(fontname)` | ✅ |
| `_is_garbled_by_font_encoding(chars, min)` | `pdfplumber.IsGarbledByFontEncoding(chars, min)` | ✅ |
| `__char_width(c)` | `pdfplumber.CharWidth(&char)` | ✅ |
| `__height(c)` | `pdfplumber.CharHeight(&char)` | ✅ |
| `_x_dis(a, b)` | `pdfplumber.XDis(&a, &b)` | ✅ |
| `_y_dis(a, b)` | `pdfplumber.YDis(&a, &b)` | ✅ |
| `sort_X_by_page(arr, threshold)` | `pdfplumber.SortXByPage(chars, threshold)` | ✅ |
| `total_page_number(fnm, binary)` | `pdfplumber.TotalPageNumber(path, data)` | ✅ |

### 5. RAGFlow pipeline steps (pseudocode)

```go
// Corresponds to RAGFlow __images__ (L1537-1544):
doc, _ := pdfplumber.Open(fnm)
defer doc.Close()

for pageIdx := fromPage; pageIdx < toPage && pageIdx < doc.PageCount(); pageIdx++ {
    // 1. Page rendering
    dpi := 72 * zoomin
    res, _ := pdfplumber.RenderPage(pdfBytes, pageIdx, float64(dpi))
    pageImg := res.ToImage()  // *image.RGBA → feed to LayoutRecognizer

    // 2. Character extraction + dedupe + _has_color filter
    chars, _ := doc.GetDedupePageChars(pageIdx, 1.0)
    filtered := make([]pdfplumber.Char, 0, len(chars))
    for _, c := range chars {
        if pdfplumber.HasColor(&c) {
            filtered = append(filtered, c)
        }
    }
    pageChars = filtered

    // 3. Garbled text detection (L1557-1565)
    sampleText := extractSampleText(pageChars)
    if pdfplumber.IsGarbledText(sampleText, 0.3) {
        pageChars = nil  // fall back to OCR
        continue
    }

    // 4. Font-encoding garbling (L1567-1575)
    if pdfplumber.IsGarbledByFontEncoding(pageChars) {
        pageChars = nil  // fall back to OCR
    }

    // 5. OCR step (L1617-1618): median height/width from chars
    if len(pageChars) > 0 {
        heights := sortedCharHeights(pageChars)
        meanHeight := median(heights)
        widths := sortedCharWidths(pageChars)
        meanWidth := median(widths)
    }
}
```

## Go API reference

```go
import "github.com/infominer/pdfplumber-go/pdfplumber"

// Open a PDF
doc, _ := pdfplumber.Open("document.pdf")
defer doc.Close()

// Or from bytes
data, _ := os.ReadFile("document.pdf")
doc, _ = pdfplumber.OpenBytes(data)

// Page count
n := doc.PageCount()  // int

// Character extraction
chars, _ := doc.GetPageChars(0)             // []Char, raw
chars, _ = doc.GetDedupePageChars(0, 1.0)   // []Char, deduplicated

// Plain text extraction (pypdf replacement)
text, _ := doc.GetPageText(0)  // string

// Page rendering (RGBA pixels)
res, _ := pdfplumber.RenderPage(pdfBytes, 0, 216.0)
img := res.ToImage()  // *image.RGBA
```

### Char fields

| Field | Type | Description |
|---|---|---|
| `Text` | string | Character text |
| `Fontname` | string | PDF font name |
| `X0`, `X1` | float64 | Horizontal bounds |
| `Top`, `Bottom` | float64 | Vertical bounds |
| `Width`, `Height` | float64 | Character dimensions |
| `Ncs` | string | Color space |
| `StrokingColor` | string | Stroke color value |
| `NonStrokingColor` | string | Fill color value |
| `PageNumber` | int | 1-based page number |
| `Size` | float64 | Font size |
| `Matrix` | [6]float64 | Text matrix |
| `Upright` | bool | Whether character is upright |
| `Adv` | float64 | Character advance |
| `Doctop` | float64 | Document-level top position |

## Build & test

```bash
cd go/pdfplumber

# Build (download deps)
go mod tidy

# Run Go tests
CGO_CFLAGS="-I$HOME/.cache/pdf_oxide/v0.3.63/include" \
CGO_LDFLAGS="$HOME/.cache/pdf_oxide/v0.3.63/lib/linux_amd64/libpdf_oxide.a -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc" \
go test -v -count=1

# Build the dumpchars CLI tool (used by compare.py)
go build ./cmd/dumpchars/
```

## Compare against pdfplumber (Python)

The `tests/compare.py` script compares pdfplumber-go output with real Python
pdfplumber for a given PDF. It tests all interfaces used by RAGFlow's
`pdf_parser.py`:

```bash
# Compare a single page (verbose)
python3 tests/compare.py document.pdf --page 0 -v

# Compare all pages
python3 tests/compare.py document.pdf --all-pages

# Machine-readable JSON
python3 tests/compare.py document.pdf --json | jq .

# Exit code: 0 = all pass, 1 = some fail
```

Comparison covers: open (file + bytes), page count, Char extraction (all 16
fields), `dedupe_chars`, `_has_color`, `_is_garbled_char`, `_is_garbled_text`,
`_has_subset_font_prefix`, `_is_garbled_by_font_encoding`, `__char_width`,
`__height`, `_x_dis`, `_y_dis`, `sort_X_by_page`, `to_image` rendering,
plain text extraction (pypdf replacement).

## Architecture

```
                              ┌──────────────────────┐
                              │   pdf_oxide (Rust)    │
                              │  crates.io v0.3.63    │
                              │  +rendering feature   │
                              └──────────┬───────────┘
                                         │ cgo / static link
                              ┌──────────▼───────────┐
                              │  pdf_oxide Go binding │
                              │  github.com/.../go   │
                              └──────────┬───────────┘
                                         │ Go import
                              ┌──────────▼───────────┐
                              │   pdfplumber-go       │
                              │   go/pdfplumber/      │
                              │   Char, Document,     │
                              │   RAGFlow utils       │
                              └──────────────────────┘
```

**Zero Rust code in the calling project.** The Rust layer is inside
`pdf_oxide` (from crates.io), accessed through its official Go binding.

## Project structure

```
pdfplumber-go/
├── go/pdfplumber/
│   ├── go.mod
│   ├── pdfplumber.go       # Core API + RAGFlow utility functions
│   ├── pdfplumber_test.go  # 13 Go tests
│   └── cmd/dumpchars/      # CLI tool for Python comparison
└── tests/
    └── compare.py           # pdfplumber-go vs pdfplumber comparison
```

## License

MIT
