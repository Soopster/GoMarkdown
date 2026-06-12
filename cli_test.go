package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCLIArgsDefaultsToWorkingDirectorySession(t *testing.T) {
	action, cfg, err := parseCLIArgs(nil)
	if err != nil {
		t.Fatalf("parseCLIArgs returned error: %v", err)
	}
	if action != cliActionRun {
		t.Fatalf("expected run action, got %v", action)
	}
	if cfg.dir == "" {
		t.Fatal("expected default directory")
	}
	if cfg.sessionPath != sessionFilePath(cfg.dir) {
		t.Fatalf("expected default session path %q, got %q", sessionFilePath(cfg.dir), cfg.sessionPath)
	}
}

func TestParseCLIArgsHelp(t *testing.T) {
	action, _, err := parseCLIArgs([]string{"--help"})
	if err != nil {
		t.Fatalf("parseCLIArgs returned error: %v", err)
	}
	if action != cliActionHelp {
		t.Fatalf("expected help action, got %v", action)
	}
}

func TestParseCLIArgsVersion(t *testing.T) {
	action, _, err := parseCLIArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("parseCLIArgs returned error: %v", err)
	}
	if action != cliActionVersion {
		t.Fatalf("expected version action, got %v", action)
	}
}

func TestParseCLIArgsNoSession(t *testing.T) {
	_, cfg, err := parseCLIArgs([]string{"--no-session"})
	if err != nil {
		t.Fatalf("parseCLIArgs returned error: %v", err)
	}
	if cfg.sessionPath != "" {
		t.Fatalf("expected empty session path, got %q", cfg.sessionPath)
	}
}

func TestParseCLIArgsRejectsSessionConflict(t *testing.T) {
	_, _, err := parseCLIArgs([]string{"--no-session", "--session-file", "state.json"})
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestParseCLIArgsResolvesFileTarget(t *testing.T) {
	path := filepath.Join("testdata", "sample.md")
	action, cfg, err := parseCLIArgs([]string{path})
	if err != nil {
		t.Fatalf("parseCLIArgs returned error: %v", err)
	}
	if action != cliActionRun {
		t.Fatalf("expected run action, got %v", action)
	}
	if !strings.HasSuffix(cfg.initialPath, filepath.FromSlash(path)) {
		t.Fatalf("expected initial path to end with %q, got %q", path, cfg.initialPath)
	}
	if cfg.dir != filepath.Dir(cfg.initialPath) {
		t.Fatalf("expected dir %q, got %q", filepath.Dir(cfg.initialPath), cfg.dir)
	}
}

func TestParseCLIArgsRejectsNonMarkdownFile(t *testing.T) {
	_, _, err := parseCLIArgs([]string{"go.mod"})
	if err == nil {
		t.Fatal("expected markdown validation error")
	}
}

func TestRenderCLIHelpIncludesInstallAndFlags(t *testing.T) {
	out := renderCLIHelp("markdownviewer")
	for _, needle := range []string{
		"go install github.com/lukeryan/markdownviewer@latest",
		"--no-session",
		"--session-file",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected help output to contain %q", needle)
		}
	}
}
