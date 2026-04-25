package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const outputPath = "data/k8s-supported-versions.json"

func main() {
	token := flag.String("token", "", "GitHub personal access token (increases API rate limit)")
	flag.Parse()

	fmt.Println("Fetching Kubernetes release tags from GitHub...")

	supported, err := detectSupportedVersions(*token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, v := range supported.Versions {
		fmt.Printf("  minor %s: %d patch release(s)\n", v.Minor, len(v.Patches))
	}

	if err := writeJSON(outputPath, supported); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Written to %s\n", outputPath)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling JSON: %w", err)
	}

	// Append a trailing newline for POSIX compliance.
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}
