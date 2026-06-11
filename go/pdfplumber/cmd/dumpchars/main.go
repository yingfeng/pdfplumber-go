package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/infominer/pdfplumber-go/pdfplumber"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: dumpchars <pdf-path> [page]")
		os.Exit(1)
	}
	path := os.Args[1]
	page := 0
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &page)
	}

	doc, err := pdfplumber.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer doc.Close()

	chars, err := doc.GetPageChars(page)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(chars)
}
