// Package agentassets owns the single bundled copy of Pi's Exhibit extensions.
package agentassets

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed ext/exhibit.ts ext/edit.ts ext/guard.ts ext/source-context.ts ext/questions.ts
var files embed.FS

// Materialize writes the extension and its relative imports beside each other.
func Materialize(root string) error {
	for _, name := range []string{"exhibit.ts", "edit.ts", "guard.ts", "source-context.ts", "questions.ts"} {
		data, err := files.ReadFile("ext/" + name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			return fmt.Errorf("materialize %s: %w", name, err)
		}
	}
	return nil
}
