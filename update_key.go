package main

import tea "charm.land/bubbletea/v2"

func (m model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.editMouseSelecting = false
	m.editMouseAnchorOff = -1
	if msg.String() == "ctrl+v" {
		target := m.activeClipboardTarget()
		if target != clipboardTargetNone {
			m.pendingClipboardTarget = target
			m.status = "Reading clipboard…"
			return m, readClipboardCmd()
		}
	}
	if msg.String() == "ctrl+o" {
		return m, tea.Suspend
	}
	if m.opMode != opNone {
		return m.handleOpKey(msg)
	}

	handlers := []func(tea.KeyPressMsg) (model, tea.Cmd, bool){
		m.handleHelpKeyPress,
		m.handleThemePickerKeyPress,
		m.handleSearchKeyPress,
		m.handleBookmarkKeyPress,
		m.handleFileFinderKeyPress,
		m.handleContentSearchKeyPress,
		m.handleCommandPaletteKeyPress,
		m.handleEditModeKeyPress,
		m.handlePreviewModeKeyPress,
	}
	for _, handler := range handlers {
		if next, cmd, ok := handler(msg); ok {
			return next, cmd
		}
	}

	return m.handleBubbleTeaUpdate(msg)
}

func (m model) handledAction(action string) (model, tea.Cmd, bool) {
	next, cmd := m.runCommandAction(action)
	return next.(model), cmd, true
}
