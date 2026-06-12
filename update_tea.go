package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

func (m model) handleTeaSystemUpdate(msg tea.Msg) (model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.ColorProfileMsg:
		prevSupportsAdvanced := m.supportsAdvancedRendering()
		m.colorProfile = msg.Profile
		m.colorProfileKnown = true
		if !m.capProbeRequested && msg.Profile != colorprofile.TrueColor {
			m.capProbeRequested = true
			return m, requestTermcapProbeCmd(), true
		}
		if prevSupportsAdvanced != m.supportsAdvancedRendering() {
			m.invalidateRenderCaches()
			if m.mode == modePreview {
				return m, m.renderCurrentContent(), true
			}
		}
		return m, nil, true

	case tea.CapabilityMsg:
		switch strings.TrimSpace(msg.String()) {
		case "RGB":
			m.termCapRGB = true
		case "Tc":
			m.termCapTc = true
		}
		return m, nil, true

	case tea.BackgroundColorMsg:
		m.terminalBgKnown = true
		m.terminalBgDark = msg.IsDark()
		m.terminalBgColor = msg.String()
		if m.followSystem {
			style := m.preferredFollowStyle()
			if style != m.styleName {
				m.themePickerIdx = themeIndex(style)
				themeCmd := m.applyTheme(style)
				m.status = "Theme: follow system"
				if m.mode == modePreview {
					return m, tea.Batch(themeCmd, m.renderCurrentContent()), true
				}
				return m, themeCmd, true
			}
		}
		return m, nil, true

	case tea.ForegroundColorMsg:
		m.terminalFgColor = msg.String()
		return m, nil, true

	case tea.CursorColorMsg:
		m.terminalCursorColor = msg.String()
		return m, nil, true

	case tea.ResumeMsg:
		m.status = "Resumed from shell"
		m.capProbeRequested = true
		cmds := []tea.Cmd{
			requestTerminalColorsCmd(),
			requestTermcapProbeCmd(),
		}
		if m.mode == modePreview {
			cmds = append(cmds, m.renderCurrentContent())
		}
		return m, tea.Batch(cmds...), true

	case overlayLayerHitMsg:
		if click, ok := msg.mouse.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
			switch msg.id {
			case layerIDHelpOverlay:
				m.showHelp = false
				m.status = "Help closed"
				return m, nil, true
			case layerIDThemeOverlay:
				m.showThemePicker = false
				m.status = "Closed theme picker"
				return m, nil, true
			case layerIDCmdOverlay:
				m.showCmdPalette = false
				m.cmdFilter = ""
				m.cmdIdx = 0
				m.filteredCommands = nil
				m.status = ""
				return m, nil, true
			}
		}
		return m, nil, true

	case tea.PasteStartMsg:
		return m, nil, true

	case tea.PasteEndMsg:
		return m, nil, true

	case tea.PasteMsg:
		content := msg.String()
		if content == "" {
			return m, nil, true
		}
		if m.opMode == opCreate || m.opMode == opRename || m.opMode == opGoToLine || m.opMode == opGoToHeading {
			line := firstPasteLine(content)
			if line == "" {
				return m, nil, true
			}
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(tea.PasteMsg{Content: line})
			return m, cmd, true
		}
		if m.showSearch {
			line := collapsePasteToLine(content)
			if line == "" {
				return m, nil, true
			}
			if m.showReplace && m.replaceInputFocus {
				var cmd tea.Cmd
				m.replaceInput, cmd = m.replaceInput.Update(tea.PasteMsg{Content: line})
				return m, cmd, true
			}
			prev := m.searchInput.Value()
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(tea.PasteMsg{Content: line})
			if m.searchInput.Value() != prev {
				m.searchIndex = -1
				m.refreshSearchMatches()
				if m.mode == modePreview {
					m.searchGeneration++
					gen := m.searchGeneration
					debounceCmd := tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
						return searchDebounceMsg{generation: gen}
					})
					return m, tea.Batch(cmd, debounceCmd), true
				}
			}
			return m, cmd, true
		}
		if m.showCmdPalette {
			line := collapsePasteToLine(content)
			if line == "" {
				return m, nil, true
			}
			m.cmdFilter += line
			m.filteredCommands = m.filterCommands(m.cmdFilter)
			m.cmdIdx = 0
			return m, nil, true
		}
		if (m.mode == modeRaw || m.mode == modeSplit) && m.focusRight {
			m.editPushUndo()
			if m.editHasSelection() {
				m.editDeleteSelection()
			}
			m.editClearSelection()
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.editCheckTableMode()
			cmds := []tea.Cmd{cmd}
			if m.autoSave {
				cmds = append(cmds, m.scheduleAutoSave())
			}
			if m.mode == modeSplit {
				cmds = append(cmds, m.scheduleSplitRender())
			}
			return m, tea.Batch(cmds...), true
		}
		return m, nil, true

	case tea.ClipboardMsg:
		target := m.pendingClipboardTarget
		m.pendingClipboardTarget = clipboardTargetNone
		if target == clipboardTargetNone {
			return m, nil, true
		}
		content := msg.String()
		if content == "" {
			m.status = m.styles.statusWarn.Render("Clipboard empty or unavailable")
			return m, nil, true
		}
		switch target {
		case clipboardTargetPrompt:
			line := firstPasteLine(content)
			if line == "" {
				return m, nil, true
			}
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(tea.PasteMsg{Content: line})
			m.status = "Pasted from clipboard"
			return m, cmd, true
		case clipboardTargetSearch:
			line := collapsePasteToLine(content)
			if line == "" {
				return m, nil, true
			}
			prev := m.searchInput.Value()
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(tea.PasteMsg{Content: line})
			if m.searchInput.Value() != prev {
				m.searchIndex = -1
				m.refreshSearchMatches()
				if m.mode == modePreview {
					m.searchGeneration++
					gen := m.searchGeneration
					debounceCmd := tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
						return searchDebounceMsg{generation: gen}
					})
					m.status = "Pasted from clipboard"
					return m, tea.Batch(cmd, debounceCmd), true
				}
			}
			m.status = "Pasted from clipboard"
			return m, cmd, true
		case clipboardTargetReplace:
			line := collapsePasteToLine(content)
			if line == "" {
				return m, nil, true
			}
			var cmd tea.Cmd
			m.replaceInput, cmd = m.replaceInput.Update(tea.PasteMsg{Content: line})
			m.status = "Pasted from clipboard"
			return m, cmd, true
		case clipboardTargetCmdPalette:
			line := collapsePasteToLine(content)
			if line == "" {
				return m, nil, true
			}
			m.cmdFilter += line
			m.filteredCommands = m.filterCommands(m.cmdFilter)
			m.cmdIdx = 0
			m.status = "Pasted from clipboard"
			return m, nil, true
		case clipboardTargetEditor:
			if (m.mode != modeRaw && m.mode != modeSplit) || !m.focusRight {
				m.status = m.styles.statusWarn.Render("Editor not focused")
				return m, nil, true
			}
			m.editPushUndo()
			if m.editHasSelection() {
				m.editDeleteSelection()
			}
			m.editClearSelection()
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(tea.PasteMsg{Content: content})
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			cmds := []tea.Cmd{cmd}
			if m.mode == modeSplit {
				cmds = append(cmds, m.scheduleSplitRender())
				m.splitSyncScroll()
			}
			m.status = "Pasted from clipboard"
			return m, tea.Batch(cmds...), true
		default:
			return m, nil, true
		}

	case tea.KeyboardEnhancementsMsg:
		m.keyboardEventTypes = msg.SupportsEventTypes()
		return m, nil, true

	case tea.BlurMsg:
		if m.terminalFocused {
			m.terminalFocused = false
			m.isDragging = false
			m.scrollbarDrag = scrollbarDragNone
			m.cancelMomentum()
			m.clearEditSelectionExtend()
			m.clearKeyboardScrollHold()
			m.lastScrollKeyDir = 0
			m.lastScrollKeyAt = time.Time{}
			m.status = "Terminal unfocused"
		}
		return m, nil, true

	case tea.FocusMsg:
		if !m.terminalFocused {
			m.terminalFocused = true
			m.status = "Terminal focused"
		}
		return m, nil, true

	case tea.KeyReleaseMsg:
		m.keyboardEventTypes = true
		switch msg.String() {
		case "j", "k", "up", "down", "left", "right":
			m.clearKeyboardScrollHold()
			m.clearEditSelectionExtend()
			m.lastScrollKeyDir = 0
			m.lastScrollKeyAt = time.Time{}
			return m, nil, true
		}

	case tea.MouseClickMsg:
		if m.mode == modeRaw && m.focusRight {
			m.editResetUndoCoalescing()
			m.editClearPreferredColumn()
			m.clearEditSelectionExtend()
		}
		if msg.Button == tea.MouseLeft {
			if m.onDivider(msg.X) {
				m.isDragging = true
				m.setListWidthFromColumn(msg.X)
				m.resizeViews()
				return m, nil, true
			}
			if m.handlePreviewScrollbarMouseClick(msg) {
				return m, nil, true
			}
			if m.handleRawScrollbarMouseClick(msg) {
				return m, nil, true
			}
			if m.handlePreviewGaugeMouseClick(msg) {
				return m, nil, true
			}
			if m.handleRawEditorMouseClick(msg) {
				return m, nil, true
			}
		}

	case tea.MouseReleaseMsg:
		if m.mode == modeRaw && m.focusRight {
			m.editResetUndoCoalescing()
			m.editClearPreferredColumn()
			m.clearEditSelectionExtend()
		}
		m.isDragging = false
		if m.handlePreviewScrollbarMouseRelease(msg) {
			return m, nil, true
		}
		if m.handleRawScrollbarMouseRelease(msg) {
			return m, nil, true
		}
		if m.handleRawEditorMouseRelease(msg) {
			return m, nil, true
		}

	case tea.MouseMotionMsg:
		if m.mode == modeRaw && m.focusRight {
			m.editResetUndoCoalescing()
			m.editClearPreferredColumn()
			m.clearEditSelectionExtend()
		}
		if m.isDragging {
			m.setListWidthFromColumn(msg.X)
			m.resizeViews()
			return m, nil, true
		}
		if m.handlePreviewScrollbarMouseDrag(msg) {
			return m, nil, true
		}
		if m.handleRawScrollbarMouseDrag(msg) {
			return m, nil, true
		}
		if m.handleRawEditorMouseDrag(msg) {
			return m, nil, true
		}
		// Discard non-drag motion — noise during scroll
		return m, nil, true

	case tea.MouseWheelMsg:
		if (m.mode == modeRaw || m.mode == modeSplit) && m.focusRight {
			m.editResetUndoCoalescing()
			m.editClearPreferredColumn()
			m.clearEditSelectionExtend()
		}
		// Keep wheel input deterministic and bounded; touchpads can emit bursts
		// of wheel events that feel far too fast when momentum compounds.
		inViewport := m.fullScreen || msg.X >= m.listWidth()
		if inViewport && m.mode == modePreview {
			isUp := msg.Button == tea.MouseWheelUp
			m.applyMouseWheelPreviewScroll(isUp)
			return m, nil, true
		}
		if inViewport && m.mode == modeRaw {
			isUp := msg.Button == tea.MouseWheelUp
			cmd := m.applyMouseWheelTextareaScroll(isUp, false)
			return m, cmd, true
		}
		if inViewport && m.mode == modeSplit && m.focusRight {
			isUp := msg.Button == tea.MouseWheelUp
			cmd := m.applyMouseWheelTextareaScroll(isUp, false)
			return m, cmd, true
		}
		return m, nil, true
	default:
		return m, nil, false
	}
	return m, nil, false
}

func (m model) handleBubbleTeaUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.showHelp || m.showThemePicker || m.showCmdPalette {
		if _, ok := msg.(tea.MouseMsg); ok {
			return m, nil
		}
	}

	cmds := []tea.Cmd{}

	activeList := m.currentList()
	if m.mode == modeRaw || m.mode == modeSplit {
		sendToList := !isKeyMsg(msg) || !m.focusRight
		sendToTextarea := !isKeyMsg(msg) || m.focusRight
		if mm, ok := msg.(tea.MouseMsg); ok {
			if m.fullScreen {
				sendToList = false
			} else if mm.Mouse().X < m.listWidth() {
				sendToTextarea = false
			} else {
				sendToList = false
			}
		}

		if sendToList {
			prevIndex := activeList.Index()
			var cmd tea.Cmd
			*activeList, cmd = activeList.Update(msg)
			cmds = append(cmds, cmd)
			if selCmd := m.handleSelectionChange(prevIndex); selCmd != nil {
				cmds = append(cmds, selCmd)
			}
		}

		if sendToTextarea {
			var cmd tea.Cmd
			prevValue := m.textarea.Value()
			prevLine := m.textarea.Line()
			prevCol := m.editCursorCol()
			prevScrollY := m.textarea.ScrollYOffset()
			m.textarea, cmd = m.textarea.Update(msg)
			cmds = append(cmds, cmd)

			nextValue := m.textarea.Value()
			valueChanged := nextValue != prevValue
			cursorMoved := m.textarea.Line() != prevLine ||
				m.editCursorCol() != prevCol ||
				m.textarea.ScrollYOffset() != prevScrollY

			if valueChanged {
				m.sourceContent = nextValue
				m.updateOutlineFromEditor()
			}
			if m.mode == modeSplit {
				if valueChanged {
					cmds = append(cmds, m.scheduleSplitRender())
				}
				if valueChanged || cursorMoved {
					m.splitSyncScroll()
				}
			}
		}
	} else {
		sendToList := !isKeyMsg(msg) || !m.focusRight
		sendToViewport := !isKeyMsg(msg) || m.focusRight
		if mm, ok := msg.(tea.MouseMsg); ok {
			if m.fullScreen {
				sendToList = false
			} else if mm.Mouse().X < m.listWidth() {
				sendToViewport = false
			} else {
				sendToList = false
			}
		}

		if sendToList {
			prevIndex := activeList.Index()
			var cmd tea.Cmd
			*activeList, cmd = activeList.Update(msg)
			cmds = append(cmds, cmd)
			if selCmd := m.handleSelectionChange(prevIndex); selCmd != nil {
				cmds = append(cmds, selCmd)
			}
		}

		if sendToViewport {
			prevOffset := m.previewYOffset
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
			m.previewYOffset = m.viewport.YOffset()
			if m.previewYOffset != prevOffset {
				m.cancelMomentum()
				m.syncCurrentHeading(m.viewport.YOffset())
			}
		}
	}

	return m, tea.Batch(cmds...)
}
