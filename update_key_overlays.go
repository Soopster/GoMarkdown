package main

import (
	"fmt"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m model) handleHelpKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if !m.showHelp {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "?":
		m.showHelp = false
		return m, nil, true
	case "q", "ctrl+c":
		return m, m.saveAndQuitCmd(), true
	default:
		return m, nil, true
	}
}

func (m model) handleThemePickerKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if !m.showThemePicker {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "T":
		m.showThemePicker = false
		m.status = "Closed theme picker"
		return m, nil, true
	case "up", "k":
		m.moveThemeSelection(-1)
		return m, nil, true
	case "down", "j":
		m.moveThemeSelection(1)
		return m, nil, true
	case "enter":
		m.followSystem = false
		cmd := m.applyTheme(themeOrder[m.themePickerIdx])
		m.status = fmt.Sprintf("Theme: %s", m.styleName)
		m.showThemePicker = false
		return m, tea.Batch(cmd, m.renderCurrentContent()), true
	case "s":
		m.followSystem = !m.followSystem
		if m.followSystem {
			style := m.preferredFollowStyle()
			m.themePickerIdx = themeIndex(style)
			cmd := m.applyTheme(style)
			m.status = "Theme: follow system"
			m.showThemePicker = false
			return m, tea.Batch(cmd, m.renderCurrentContent()), true
		}
		m.status = "Theme: manual"
		return m, nil, true
	default:
		return m, nil, true
	}
}

func (m model) handleSearchKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if !m.showSearch {
		return m, nil, false
	}
	switch msg.String() {
	case "esc":
		if m.showReplace {
			m.closeReplace()
			return m, nil, true
		}
		m.closeSearch(true)
		return m, nil, true
	case "ctrl+h":
		if m.mode != modeRaw {
			return m, nil, true
		}
		if m.showReplace {
			m.closeReplace()
			return m, nil, true
		}
		return m, m.openReplace(), true
	case "tab", "shift+tab":
		if m.showReplace {
			return m, m.setReplaceInputFocus(!m.replaceInputFocus), true
		}
	case "alt+c":
		if m.showReplace {
			m.replaceCaseSensitive = !m.replaceCaseSensitive
			m.searchIndex = -1
			m.refreshSearchMatches()
			m.status = fmt.Sprintf("Case sensitive %v", onOff(m.replaceCaseSensitive))
			return m, nil, true
		}
	case "alt+w":
		if m.showReplace {
			m.replaceWholeWord = !m.replaceWholeWord
			m.searchIndex = -1
			m.refreshSearchMatches()
			m.status = fmt.Sprintf("Whole word %v", onOff(m.replaceWholeWord))
			return m, nil, true
		}
	case "alt+s":
		if m.showReplace {
			if !m.editHasSelection() && !m.replaceScopeSelection {
				m.status = m.styles.statusWarn.Render("No selection")
				return m, nil, true
			}
			m.replaceScopeSelection = !m.replaceScopeSelection
			m.status = fmt.Sprintf("Selection scope %v", onOff(m.replaceScopeSelection))
			return m, nil, true
		}
	case "ctrl+enter":
		if m.showReplace {
			m.replaceAllSearchMatches()
			return m, nil, true
		}
	case "enter":
		if m.showReplace {
			scrollCmd := m.replaceNextSearchMatch()
			return m, scrollCmd, true
		}
		fallthrough
	case "ctrl+n", "f3":
		m.refreshSearchMatches()
		nextIdx := m.searchIndex + 1
		if m.searchIndex < 0 {
			nextIdx = m.searchInitialMatchIndex(1)
		}
		scrollCmd := m.jumpToSearchMatch(nextIdx)
		if m.mode == modePreview {
			return m, tea.Batch(scrollCmd, m.renderCurrentContent()), true
		}
		return m, scrollCmd, true
	case "shift+enter", "ctrl+p", "shift+f3":
		m.refreshSearchMatches()
		prevIdx := m.searchIndex - 1
		if m.searchIndex < 0 {
			prevIdx = m.searchInitialMatchIndex(-1)
		}
		scrollCmd := m.jumpToSearchMatch(prevIdx)
		if m.mode == modePreview {
			return m, tea.Batch(scrollCmd, m.renderCurrentContent()), true
		}
		return m, scrollCmd, true
	}
	if m.showReplace && m.replaceInputFocus {
		var cmd tea.Cmd
		m.replaceInput, cmd = m.replaceInput.Update(msg)
		return m, cmd, true
	}
	var cmd tea.Cmd
	prev := m.searchInput.Value()
	m.searchInput, cmd = m.searchInput.Update(msg)
	if m.searchInput.Value() != prev {
		m.searchIndex = -1
		m.refreshSearchMatches()
		if m.mode == modePreview {
			m.searchGeneration++
			gen := m.searchGeneration
			debounceCmd := tea.Tick(searchDebounceDelay, func(time.Time) tea.Msg {
				return searchDebounceMsg{generation: gen}
			})
			return m, tea.Batch(cmd, debounceCmd), true
		}
	}
	return m, cmd, true
}

func (m model) handleBookmarkKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if m.markMode == 0 {
		return m, nil, false
	}
	r := rune(0)
	if msg.Text != "" {
		runes := []rune(msg.Text)
		if len(runes) == 1 {
			r = runes[0]
		}
	}
	if msg.String() == "esc" || r == 0 {
		m.markMode = 0
		m.status = ""
		return m, nil, true
	}
	if m.markMode == 'b' {
		m.bookmarks[r] = bookmarkEntry{Path: m.currentPath, YOffset: m.viewport.YOffset(), Line: 0, Heading: m.currentHeading}
		m.markMode = 0
		m.status = fmt.Sprintf("Bookmark '%c' set", r)
		return m, nil, true
	}
	if m.markMode == '\'' {
		if bm, ok := m.bookmarks[r]; ok {
			m.markMode = 0
			if bm.Path != m.currentPath && bm.Path != "" {
				m.navStack = append(m.navStack, navEntry{path: m.currentPath, yOffset: m.viewport.YOffset(), heading: m.currentHeading})
				m.currentPath = bm.Path
				m.previewYOffset = bm.YOffset
				m.restoreHeading = bm.Heading
				return m, m.loadFileCmd(bm.Path), true
			}
			m.viewport.SetYOffset(bm.YOffset)
			if bm.Heading >= 0 && bm.Heading < len(m.headings) {
				m.setCurrentHeading(bm.Heading)
			}
			m.status = fmt.Sprintf("Jumped to bookmark '%c'", r)
		} else {
			m.markMode = 0
			m.status = fmt.Sprintf("No bookmark '%c'", r)
		}
		return m, nil, true
	}
	m.markMode = 0
	return m, nil, true
}

func (m model) handleFileFinderKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if !m.showFileFinder {
		return m, nil, false
	}
	switch msg.String() {
	case "esc":
		m.showFileFinder = false
		m.fileFinderInput.Blur()
		m.status = ""
		return m, nil, true
	case "up":
		if m.fileFinderIdx > 0 {
			m.fileFinderIdx--
		}
		return m, nil, true
	case "down":
		if m.fileFinderIdx < len(m.fileFinderResults)-1 {
			m.fileFinderIdx++
		}
		return m, nil, true
	case "enter":
		if m.fileFinderIdx >= 0 && m.fileFinderIdx < len(m.fileFinderResults) {
			selected := m.fileFinderResults[m.fileFinderIdx]
			m.showFileFinder = false
			m.fileFinderInput.Blur()
			fullPath := filepath.Join(m.dir, selected.path)
			m.currentPath = fullPath
			m.previewYOffset = 0
			m.mode = modePreview
			return m, m.loadFileCmd(fullPath), true
		}
		return m, nil, true
	case "backspace":
		v := m.fileFinderInput.Value()
		if len(v) > 0 {
			m.fileFinderInput.SetValue(v[:len(v)-1])
			m.fileFinderResults = fuzzyMatch(m.fileFinderAll, m.fileFinderInput.Value())
			m.fileFinderIdx = 0
		}
		return m, nil, true
	default:
		t := msg.Text
		if len(t) >= 1 && t[0] >= 32 {
			m.fileFinderInput.SetValue(m.fileFinderInput.Value() + t)
			m.fileFinderResults = fuzzyMatch(m.fileFinderAll, m.fileFinderInput.Value())
			m.fileFinderIdx = 0
		}
		return m, nil, true
	}
}

func (m model) handleContentSearchKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if !m.showContentSearch {
		return m, nil, false
	}
	switch msg.String() {
	case "esc":
		m.showContentSearch = false
		m.contentSearchInput.Blur()
		m.status = ""
		return m, nil, true
	case "up":
		if m.contentSearchIdx > 0 {
			m.contentSearchIdx--
		}
		return m, nil, true
	case "down":
		if m.contentSearchIdx < len(m.contentSearchResults)-1 {
			m.contentSearchIdx++
		}
		return m, nil, true
	case "enter":
		if m.contentSearchIdx >= 0 && m.contentSearchIdx < len(m.contentSearchResults) {
			hit := m.contentSearchResults[m.contentSearchIdx]
			m.showContentSearch = false
			m.contentSearchInput.Blur()
			fullPath := filepath.Join(m.dir, hit.path)
			m.currentPath = fullPath
			m.previewYOffset = 0
			m.mode = modePreview
			m.highlightLine = hit.line
			return m, m.loadFileCmd(fullPath), true
		}
		return m, nil, true
	case "backspace":
		v := m.contentSearchInput.Value()
		if len(v) > 0 {
			m.contentSearchInput.SetValue(v[:len(v)-1])
			m.contentSearchGen++
			gen := m.contentSearchGen
			return m, tea.Tick(contentSearchDebounce, func(time.Time) tea.Msg {
				return contentSearchDebounceMsg{generation: gen}
			}), true
		}
		return m, nil, true
	default:
		t := msg.Text
		if len(t) >= 1 && t[0] >= 32 {
			m.contentSearchInput.SetValue(m.contentSearchInput.Value() + t)
			m.contentSearchGen++
			gen := m.contentSearchGen
			return m, tea.Tick(contentSearchDebounce, func(time.Time) tea.Msg {
				return contentSearchDebounceMsg{generation: gen}
			}), true
		}
		return m, nil, true
	}
}

func (m model) handleCommandPaletteKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if !m.showCmdPalette {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "ctrl+k":
		m.showCmdPalette = false
		m.cmdFilter = ""
		m.cmdIdx = 0
		m.status = ""
		return m, nil, true
	case "up", "k":
		if m.cmdIdx > 0 {
			m.cmdIdx--
		}
		return m, nil, true
	case "down", "j":
		if m.cmdIdx < len(m.filteredCommands)-1 {
			m.cmdIdx++
		}
		return m, nil, true
	case "pgup":
		m.cmdIdx = max(0, m.cmdIdx-10)
		return m, nil, true
	case "pgdown":
		m.cmdIdx = min(max(0, len(m.filteredCommands)-1), m.cmdIdx+10)
		return m, nil, true
	case "enter":
		if m.cmdIdx >= 0 && m.cmdIdx < len(m.filteredCommands) {
			selected := m.filteredCommands[m.cmdIdx]
			m.showCmdPalette = false
			m.cmdFilter = ""
			m.cmdIdx = 0
			return m.handledAction(selected.action)
		}
		return m, nil, true
	case "backspace":
		if len(m.cmdFilter) > 0 {
			m.cmdFilter = m.cmdFilter[:len(m.cmdFilter)-1]
			m.filteredCommands = m.filterCommands(m.cmdFilter)
			m.cmdIdx = 0
		}
		return m, nil, true
	default:
		t := msg.Text
		if len(t) == 1 && t[0] >= 32 && t[0] < 127 {
			m.cmdFilter += t
			m.filteredCommands = m.filterCommands(m.cmdFilter)
			m.cmdIdx = 0
		}
		return m, nil, true
	}
}
