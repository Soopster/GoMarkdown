package main

import (
	tea "charm.land/bubbletea/v2"
)

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.refreshFilesCmd(),
		requestTerminalColorsCmd(),
		m.walkDirCmd(),
		m.gitStatusCmd(),
	}
	if m.watcher != nil {
		cmds = append(cmds, watchFilesCmd(m.watcher))
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.reuseLastFrame = false
	if keyMsg, ok := keyPressFromMsg(msg); ok {
		return m.handleKeyPress(keyMsg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.listWidthRatio <= 0 {
			m.listWidthRatio = defaultListRatio
		}
		m.resizeViews()
		return m, nil
	default:
		if next, cmd, ok := m.handleAsyncUpdate(msg); ok {
			return next, cmd
		}
		if next, cmd, ok := m.handleTeaSystemUpdate(msg); ok {
			return next, cmd
		}
		return m.handleBubbleTeaUpdate(msg)
	}
}

func keyPressFromMsg(msg tea.Msg) (tea.KeyPressMsg, bool) {
	switch keyMsg := msg.(type) {
	case tea.KeyPressMsg:
		return keyMsg, true
	case tea.KeyReleaseMsg:
		return tea.KeyPressMsg{}, false
	case tea.KeyMsg:
		// Some wrappers surface key events through the KeyMsg interface instead
		// of the concrete KeyPressMsg type. Treat those as presses so app-level
		// shortcuts still route through the main hotkey handlers.
		return tea.KeyPressMsg(keyMsg.Key()), true
	default:
		return tea.KeyPressMsg{}, false
	}
}
