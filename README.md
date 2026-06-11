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

## Go API

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

// Character extraction (returns []pdfplumber.Char with all fields)
chars, _ := doc.GetPageChars(0)

// Deduplicated characters
chars, _ = doc.GetDedupePageChars(0, 1.0)

// Page rendering (RGBA pixels)
res, _ := pdfplumber.RenderPage(pdfBytes, 0, 216.0)
img := res.ToImage()  // *image.RGBA
```

### Char fields

| Field | Type | Description |
|-------|------|-------------|
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

### RAGFlow utility functions

```go
// Character filtering (matches RAGFlow _has_color exactly)
pdfplumber.HasColor(&char)  // bool

// Garbled character detection (PUA, surrogate, noncharacter, control)
pdfplumber.IsGarbledChar('A')  // bool — matches RAGFlow _is_garbled_char

// Text-level garbled detection (CID pattern + threshold)
pdfplumber.IsGarbledText(text, 0.5)  // bool

// Subset font prefix detection (e.g. "DY1+FontName")
pdfplumber.HasSubsetFontPrefix(fontname)  // bool

// Font-encoding garbling detection (CJK mapped to ASCII)
pdfplumber.IsGarbledByFontEncoding(chars, 20)  // bool

// Char dimension utilities
pdfplumber.CharWidth(&char)   // float64 — (x1-x0)/len(text)
pdfplumber.CharHeight(&char)  // float64 — bottom-top

// Distance utilities
pdfplumber.XDis(&a, &b)  // float64 — horizontal distance
pdfplumber.YDis(&a, &b)  // float64 — vertical distance

// Sorting
pdfplumber.SortXByPage(chars, threshold)  // []Char

// Page count utility
n, _ := pdfplumber.TotalPageNumber(path, data)  // (int, error)
```

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

# Machine-readable JSON output
python3 tests/compare.py document.pdf --json | jq .

# Exit code: 0 = all pass, 1 = some fail
```

The comparison covers:
- open (file + bytes), page count, close
- Char extraction (all 16 fields)
- `dedupe_chars` equivalence
- `_has_color` filter logic
- `_is_garbled_char` with Unicode category (Cn/Cs)
- `_is_garbled_text` with CID pattern detection
- `_has_subset_font_prefix`
- `_is_garbled_by_font_encoding`
- `__char_width`, `__height`
- `_x_dis`, `_y_dis`
- `sort_X_by_page`
- `to_image` rendering

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
│   ├── pdfplumber_test.go  # 12 Go tests
│   └── cmd/dumpchars/      # CLI tool for Python comparison
└── tests/
    └── compare.py           # pdfplumber-go vs pdfplumber comparison
```

## License

MIT
