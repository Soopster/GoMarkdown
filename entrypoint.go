package main

import (
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

func run(args []string, progName string, stdout, stderr io.Writer) error {
	action, cfg, err := parseCLIArgs(args)
	if err != nil {
		return err
	}

	switch action {
	case cliActionHelp:
		_, err = io.WriteString(stdout, renderCLIHelp(progName))
		return err
	case cliActionVersion:
		_, err = io.WriteString(stdout, versionDetails()+"\n")
		return err
	default:
	}

	// This app does enough cell-width work during rendering that the small LUT
	// memory cost is worth paying once before the TUI starts.
	runewidth.CreateLUT()

	m := newModelWithConfig(cfg)
	p := tea.NewProgram(
		m,
		tea.WithFilter(programMsgFilter),
		tea.WithFPS(programFPSFromEnv()),
	)
	finalModel, err := p.Run()
	if final, ok := finalModel.(model); ok {
		if final.watcher != nil {
			_ = final.watcher.Close()
		}
		if final.perf != nil && final.perf.logFile != nil {
			_ = final.perf.logFile.Close()
		}
	}
	return err
}
