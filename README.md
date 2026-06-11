# pdfplumber-go

Pure Go pdfplumber-compatible PDF library backed by pdf_oxide engine.

Support the same Char fields and RAGFlow utility functions as pdfplumber.
No Rust compilation required.

## Prerequisites

- Go 1.21+
- CGo must be enabled (`CGO_ENABLED=1`)
- Native libraries: download via the pdf_oxide installer

## Installation

```bash
# 1. Install native libraries (libpdf_oxide.a, headers)
#    Network may need a proxy for GitHub Releases:
https_proxy=socks5h://127.0.0.1:7890 \
CGO_ENABLED=1 go run github.com/yfedoseev/pdf_oxide/go/cmd/install@v0.3.63

# 2. The installer prints environment variables to export, e.g.:
export CGO_CFLAGS="-I$HOME/.cache/pdf_oxide/v0.3.63/include"
export CGO_LDFLAGS="$HOME/.cache/pdf_oxide/v0.3.63/lib/linux_amd64/libpdf_oxide.a -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"

# 3. Add to your project:
#    go get github.com/yfedoseev/pdf_oxide/go
```

## Quick start

```go
package main

import (
    "fmt"
    "github.com/infominer/pdfplumber-go/pdfplumber"
)

func main() {
    doc, _ := pdfplumber.Open("document.pdf")
    defer doc.Close()

    // Character extraction
    chars, _ := doc.GetPageChars(0)
    fmt.Printf("Page 0: %d chars\n", len(chars))

    // Deduplicated characters
    chars, _ = doc.GetDedupePageChars(0, 1.0)

    // Page rendering
    pdfBytes, _ := os.ReadFile("document.pdf")
    res, _ := pdfplumber.RenderPage(pdfBytes, 0, 216.0)
    img := res.ToImage()  // *image.RGBA

    // RAGFlow utilities
    pdfplumber.HasColor(&char)
    pdfplumber.IsGarbledChar('A')
    pdfplumber.IsGarbledText(text, 0.5)
    pdfplumber.HasSubsetFontPrefix("DY1+FontName")
    pdfplumber.IsGarbledByFontEncoding(chars, 20)
}
```

## Build & test

```bash
cd go/pdfplumber

# Set up native libraries first (see Installation above).
# Then:

CGO_CFLAGS="-I$HOME/.cache/pdf_oxide/v0.3.63/include" \
CGO_LDFLAGS="$HOME/.cache/pdf_oxide/v0.3.63/lib/linux_amd64/libpdf_oxide.a -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc" \
go test -v -count=1
```

## Char fields

| Field | Type | Description |
|-------|------|-------------|
| `text` | string | Character text |
| `fontname` | string | PDF font name |
| `x0`, `x1` | float64 | Horizontal bounds |
| `top`, `bottom` | float64 | Vertical bounds |
| `width`, `height` | float64 | Character dimensions |
| `ncs` | string | Color space ("DeviceGray", "DeviceRGB") |
| `stroking_color` | string | Stroke color value |
| `non_stroking_color` | string | Non-stroke color value |
| `page_number` | int | 1-based page number |
| `size` | float64 | Font size |
| `matrix` | [6]float64 | Text matrix |
| `upright` | bool | Whether character is upright |
| `adv` | float64 | Character advance |
| `doctop` | float64 | Document-level top position |

## License

MIT
