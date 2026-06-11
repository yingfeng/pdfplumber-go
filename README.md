# pdfplumber-go

Pure Rust reimplementation of pdfplumber, exposed via C FFI for Go (cgo) usage.

Backed by **pdf_oxide** (v0.3 from crates.io) as the underlying PDF engine — a complete,
production-grade PDF parser with 100% pass rate across 3,830 test PDFs, full Adobe CMap
support, TrueType cmap direct reading, and built-in tiny-skia rendering.

## Architecture

```
Go SDK (go/pdfplumber/) — cgo → libpdfplumber_go.so — pdf_oxide (crates.io)
```

- **Rust engine** (`src/`): thin wrapper around pdf_oxide, provides pdfplumber-style
  Char extraction and page rendering
- **C FFI** (`ffi.rs`): 8 stable `extern "C"` functions for Go cgo integration
- **Go SDK** (`go/pdfplumber/`): pdfplumber-compatible API with RAGFlow utility functions

## Build

```bash
cargo build --release
```

Output: `target/release/libpdfplumber_go.so`

## Go usage

```bash
cd go/pdfplumber
CGO_LDFLAGS="-L../../target/release" \
LD_LIBRARY_PATH=../../target/release \
go test -v -count=1
```

```go
import "github.com/infominer/pdfplumber-go/pdfplumber"

doc, _ := pdfplumber.Open("document.pdf")
defer doc.Close()

chars, _ := doc.GetPageChars(0)
chars, _ = doc.GetDedupePageChars(0, 1.0)

res, _ := pdfplumber.RenderPage(pdfBytes, 0, 216.0)
img := res.ToImage()

// RAGFlow utilities
pdfplumber.HasColor(&char)
pdfplumber.IsGarbledChar('A')
pdfplumber.IsGarbledText(text, 0.5)
pdfplumber.HasSubsetFontPrefix("DY1+FontName")
pdfplumber.IsGarbledByFontEncoding(chars, 20)
```

## Python usage

```python
from pdfplumber_rs_client import open as pdfplumber_open

with pdfplumber_open("document.pdf") as pdf:
    for page in pdf.pages:
        chars = page.chars
        chars_deduped = page.dedupe_chars(tolerance=1.0)
```

## Test

```bash
# Python tests (77 tests)
python3 tests/test_ragflow_compat.py

# Go tests (12 tests)
cd go/pdfplumber
CGO_LDFLAGS="-L../../target/release" LD_LIBRARY_PATH=../../target/release go test -v -count=1
```
