// markdownviewer is a terminal-based Markdown viewer and editor.
// Usage: markdownviewer [file-or-directory]
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	err := run(os.Args[1:], filepath.Base(os.Args[0]), os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
