package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tanq16/expenseowl/internal/api"
)

func main() {
	outDir := flag.String("out", "email-previews", "Directory to write rendered email previews")
	baseURL := flag.String("base-url", "https://www.expenselog.com.ar", "Base URL used in preview links")
	flag.Parse()

	previews, err := api.BuildEmailPreviews(*baseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build previews: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	keys := make([]string, 0, len(previews))
	for key := range previews {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		preview := previews[key]
		htmlPath := filepath.Join(*outDir, key+".html")
		txtPath := filepath.Join(*outDir, key+".txt")
		subjectPath := filepath.Join(*outDir, key+".subject.txt")

		if err := os.WriteFile(htmlPath, []byte(preview.HTML), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "failed writing %s: %v\n", htmlPath, err)
			os.Exit(1)
		}
		if err := os.WriteFile(txtPath, []byte(preview.Text), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "failed writing %s: %v\n", txtPath, err)
			os.Exit(1)
		}
		if err := os.WriteFile(subjectPath, []byte(preview.Subject+"\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "failed writing %s: %v\n", subjectPath, err)
			os.Exit(1)
		}
	}

	fmt.Printf("Rendered %d email previews in %s\n", len(previews), *outDir)
}
