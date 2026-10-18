// Command run writes the generated files from catalog/catalog.json. It is
// what go generate runs, from the module root:
//
//	go generate ./...
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyz-is/arandu-directd/internal/gen"
)

func main() {
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	c, err := gen.Load(root)
	if err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(root, gen.ReadmeFile))
	if err != nil {
		return err
	}
	files, err := gen.Generate(c, readme)
	if err != nil {
		return err
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
