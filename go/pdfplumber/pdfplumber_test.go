package pdfplumber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var fixtureDir = func() string {
	candidates := []string{
		"../../../pdfsink-rs/tests/fixtures",
		"../../pdfsink-rs/tests/fixtures",
		"../pdfsink-rs/tests/fixtures",
	}
	for _, c := range candidates {
		p := filepath.Join(c)
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}()

func TestOpenFile(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	path := filepath.Join(fixtureDir, "simple_text.pdf")
	doc, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer doc.Close()
	if doc.PageCount() != 1 {
		t.Fatalf("expected 1 page, got %d", doc.PageCount())
	}
}

func TestOpenBytes(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	data, err := os.ReadFile(filepath.Join(fixtureDir, "simple_text.pdf"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	doc, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}
	defer doc.Close()
	if doc.PageCount() != 1 {
		t.Fatalf("expected 1 page, got %d", doc.PageCount())
	}
}

func TestGetPageChars(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	doc, err := Open(filepath.Join(fixtureDir, "simple_text.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer doc.Close()

	chars, err := doc.GetPageChars(0)
	if err != nil {
		t.Fatalf("GetPageChars failed: %v", err)
	}
	if len(chars) == 0 {
		t.Fatal("expected non-empty chars")
	}

	c := chars[0]
	if c.Text == "" {
		t.Error("expected non-empty text")
	}
	if c.Fontname == "" {
		t.Error("expected non-empty fontname")
	}
	if c.X0 >= c.X1 {
		t.Errorf("expected x0 < x1, got %f >= %f", c.X0, c.X1)
	}
	if c.Top >= c.Bottom {
		t.Errorf("expected top < bottom, got %f >= %f", c.Top, c.Bottom)
	}
	if c.PageNumber < 1 {
		t.Errorf("expected page_number >= 1, got %d", c.PageNumber)
	}
}

func TestDedupeChars(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	doc, err := Open(filepath.Join(fixtureDir, "rotated_and_duplicates.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer doc.Close()

	raw, err := doc.GetPageChars(0)
	if err != nil {
		t.Fatalf("GetPageChars failed: %v", err)
	}
	deduped, err := doc.GetDedupePageChars(0, 1.0)
	if err != nil {
		t.Fatalf("GetDedupePageChars failed: %v", err)
	}
	if len(deduped) > len(raw) {
		t.Errorf("expected deduped <= raw (%d > %d)", len(deduped), len(raw))
	}
	if len(deduped) == 0 && len(raw) > 0 {
		t.Error("expected non-empty deduped")
	}
}

func TestMultiPage(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	doc, err := Open(filepath.Join(fixtureDir, "multipage.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer doc.Close()

	if doc.PageCount() < 2 {
		t.Fatalf("expected at least 2 pages, got %d", doc.PageCount())
	}
	for i := 0; i < doc.PageCount(); i++ {
		chars, err := doc.GetPageChars(i)
		if err != nil {
			t.Fatalf("GetPageChars(%d) failed: %v", i, err)
		}
		if len(chars) == 0 {
			t.Errorf("page %d: expected non-empty chars", i)
		}
		for _, c := range chars {
			if c.PageNumber != i+1 {
				t.Errorf("expected page_number=%d, got %d", i+1, c.PageNumber)
				break
			}
		}
	}
}

func TestHasColor(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	doc, err := Open(filepath.Join(fixtureDir, "simple_text.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer doc.Close()
	chars, err := doc.GetPageChars(0)
	if err != nil {
		t.Fatalf("GetPageChars failed: %v", err)
	}
	hasColor := 0
	for _, c := range chars {
		if HasColor(&c) {
			hasColor++
		}
	}
	if hasColor != len(chars) {
		t.Errorf("HasColor filtered out %d/%d chars", len(chars)-hasColor, len(chars))
	}
}

func TestMultiplePDFs(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".pdf" {
			doc, err := Open(filepath.Join(fixtureDir, e.Name()))
			if err != nil {
				t.Errorf("Open(%s) failed: %v", e.Name(), err)
				continue
			}
			for i := 0; i < doc.PageCount(); i++ {
				if _, err := doc.GetPageChars(i); err != nil {
					t.Errorf("GetPageChars(%s, %d) failed: %v", e.Name(), i, err)
				}
			}
			doc.Close()
			count++
		}
	}
	if count == 0 {
		t.Error("no PDFs found in fixture directory")
	}
}

func TestHasSubsetFontPrefix(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  bool
	}{
		{"DY1+ZLQDm1-1", true},
		{"ABCDEF+FontName", true},
		{"Helvetica", false},
		{"+Plus", false},
		{"", false},
		{"A+", false},
		{"ABCDEFG+Long", false},
	} {
		got := HasSubsetFontPrefix(tt.input)
		if got != tt.want {
			t.Errorf("HasSubsetFontPrefix(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsGarbledChar(t *testing.T) {
	for _, tt := range []struct {
		r    rune
		want bool
	}{
		{0xE000, true}, {0xF8FF, true}, {0xFFFD, true},
		{0x00, false}, {'A', false}, {'中', false}, {'\n', false}, {0x7F, false},
	} {
		got := IsGarbledChar(tt.r)
		if got != tt.want {
			t.Errorf("IsGarbledChar(%U) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestIsGarbledText(t *testing.T) {
	if IsGarbledText("Hello World", 0.5) {
		t.Error("normal text should not be garbled")
	}
	if !IsGarbledText("\uFFFD\uFFFD\uFFFD", 0.3) {
		t.Error("replacement chars should be garbled")
	}
}

func TestIsGarbledByFontEncoding(t *testing.T) {
	normal := []Char{
		{Text: "H", Fontname: "Helvetica"},
		{Text: "e", Fontname: "Helvetica"},
	}
	if IsGarbledByFontEncoding(normal, 20) {
		t.Error("normal chars should not be garbled by font encoding")
	}
}

func TestRenderPage(t *testing.T) {
	if fixtureDir == "" {
		t.Skip("fixture directory not found")
	}
	data, err := os.ReadFile(filepath.Join(fixtureDir, "simple_text.pdf"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	res, err := RenderPage(data, 0, 216.0)
	if err != nil {
		if strings.Contains(err.Error(), "code 8") || strings.Contains(err.Error(), "unsupported") {
			t.Skipf("rendering not available: %v", err)
		}
		t.Fatalf("RenderPage failed: %v", err)
	}
	if res.Width <= 0 || res.Height <= 0 {
		t.Errorf("invalid dimensions: %dx%d", res.Width, res.Height)
	}
	if res.Channels != 4 {
		t.Errorf("expected 4 channels, got %d", res.Channels)
	}
	if len(res.Data) != res.Width*res.Height*res.Channels {
		t.Errorf("data size mismatch: %d != %d", len(res.Data), res.Width*res.Height*res.Channels)
	}
	img := res.ToImage()
	if img.Bounds().Dx() != res.Width || img.Bounds().Dy() != res.Height {
		t.Errorf("image size mismatch: %v != %dx%d", img.Bounds(), res.Width, res.Height)
	}
}
