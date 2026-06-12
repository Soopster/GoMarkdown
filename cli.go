package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type cliAction int

const (
	cliActionRun cliAction = iota
	cliActionHelp
	cliActionVersion
)

type appConfig struct {
	dir         string
	initialPath string
	sessionPath string
}

func defaultAppConfig() appConfig {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	return appConfig{
		dir:         dir,
		sessionPath: sessionFilePath(dir),
	}
}

func parseCLIArgs(args []string) (cliAction, appConfig, error) {
	cfg := defaultAppConfig()
	fs := flag.NewFlagSet("markdownviewer", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))

	var (
		showVersion bool
		noSession   bool
		sessionFile string
	)

	fs.BoolVar(&showVersion, "version", false, "print version information and exit")
	fs.BoolVar(&showVersion, "v", false, "print version information and exit")
	fs.BoolVar(&noSession, "no-session", false, "disable session restore/save for this run")
	fs.StringVar(&sessionFile, "session-file", "", "use an explicit session file path")
	fs.Usage = func() {}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cliActionHelp, cfg, nil
		}
		return cliActionRun, cfg, err
	}
	if showVersion {
		return cliActionVersion, cfg, nil
	}
	if noSession && sessionFile != "" {
		return cliActionRun, cfg, errors.New("--no-session and --session-file cannot be used together")
	}

	rest := fs.Args()
	if len(rest) > 1 {
		return cliActionRun, cfg, fmt.Errorf("expected at most 1 path argument, got %d", len(rest))
	}
	if sessionFile != "" {
		abs, err := filepath.Abs(sessionFile)
		if err != nil {
			return cliActionRun, cfg, fmt.Errorf("resolve session file: %w", err)
		}
		cfg.sessionPath = abs
	}
	if noSession {
		cfg.sessionPath = ""
	}
	if len(rest) == 0 {
		return cliActionRun, cfg, nil
	}

	resolved, err := filepath.Abs(rest[0])
	if err != nil {
		return cliActionRun, cfg, fmt.Errorf("resolve path %q: %w", rest[0], err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return cliActionRun, cfg, fmt.Errorf("stat path %q: %w", rest[0], err)
	}
	if info.IsDir() {
		cfg.dir = resolved
		if cfg.sessionPath == "" {
			return cliActionRun, cfg, nil
		}
		if sessionFile == "" {
			cfg.sessionPath = sessionFilePath(cfg.dir)
		}
		return cliActionRun, cfg, nil
	}
	if !isMarkdownPath(resolved) {
		return cliActionRun, cfg, fmt.Errorf("path %q is not a supported markdown file", rest[0])
	}

	cfg.dir = filepath.Dir(resolved)
	cfg.initialPath = resolved
	if sessionFile == "" && cfg.sessionPath != "" {
		cfg.sessionPath = sessionFilePath(cfg.dir)
	}
	return cliActionRun, cfg, nil
}

func renderCLIHelp(progName string) string {
	return fmt.Sprintf(`%s is a terminal Markdown viewer and editor.

Usage:
  %s [flags] [file-or-directory]

Flags:
  -h, --help                 Show this help message and exit
  -v, --version              Print version information and exit
      --no-session           Disable session restore/save for this run
      --session-file string  Use an explicit session file path

Examples:
  %s
  %s README.md
  %s docs/

Install:
  go install github.com/Soopster/GoMarkdown@latest
`, progName, progName, progName, progName, progName)
}

func isMarkdownPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := markdownExts[ext]
	return ok
}
