package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) handleEditModeKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if (m.mode != modeRaw && m.mode != modeSplit) || !m.focusRight {
		return m, nil, false
	}

	keyCoalesceKind := editUndoKeyCoalesceKind(msg)
	if keyCoalesceKind == editUndoCoalesceNone {
		m.editResetUndoCoalescing()
	}
	if !editIsVerticalEditorKey(msg.String()) {
		m.editClearPreferredColumn()
	}
	if !editIsSelectionArrowKey(msg) {
		m.clearEditSelectionExtend()
	}
	if msg.Code == tea.KeyHome || msg.Code == tea.KeyEnd {
		m.cancelMomentum()
		toEnd := msg.Code == tea.KeyEnd
		ctrl := msg.Mod&tea.ModCtrl != 0
		selecting := msg.Mod&tea.ModShift != 0
		if ctrl {
			m.editMoveDocBoundary(toEnd, selecting)
		} else if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(toEnd, selecting)
		} else {
			m.editMoveLineBoundary(toEnd, selecting)
		}
		m.ensureEditCursorVisibleSoft(selecting)
		return m, nil, true
	}
	if m.editTableMode {
		switch msg.String() {
		case "tab":
			m.editTableNextCell()
			return m, nil, true
		case "shift+tab":
			m.editTablePrevCell()
			return m, nil, true
		case "enter":
			row := m.textarea.Line()
			if row >= m.tableEndLine-1 {
				m.editTableAddRow()
			} else {
				nextRow := row + 1
				lines := strings.Split(m.textarea.Value(), "\n")
				for nextRow < m.tableEndLine && isTableSeparatorRow(lines[nextRow]) {
					nextRow++
				}
				if nextRow < m.tableEndLine {
					m.editSetCursor(nextRow, m.editCursorCol())
				}
			}
			return m, nil, true
		case "esc":
			m.editExitTableMode()
			return m, nil, true
		}
	}

	switch msg.String() {
	case "ctrl+c":
		if m.editHasSelection() {
			text := m.editSelectedText()
			m.status = "Copied"
			return m, tea.SetClipboard(text), true
		}
		return m, m.saveAndQuitCmd(), true
	case "q":
		return m, m.saveAndQuitCmd(), true
	case "esc":
		m.cancelMomentum()
		m.mode = modePreview
		m.textarea.Blur()
		m.focusRight = true
		m.resizeViews()
		m.status = "Preview mode"
		return m, m.renderCurrentContent(), true
	case "enter":
		m.editBeginUndoCoalesced(editUndoCoalesceInsert)
		if m.editHasSelection() {
			m.editDeleteSelection()
		}
		m.editClearSelection()
		if !m.editHandleSmartEnter() {
			m.textarea.InsertString("\n")
		}
		m.sourceContent = m.textarea.Value()
		m.updateOutlineFromEditor()
		return m, nil, true
	case "tab":
		m.cancelMomentum()
		if m.editHasSelection() {
			m.editPushUndo()
			if m.editIndentLines() {
				m.sourceContent = m.textarea.Value()
				m.updateOutlineFromEditor()
			}
			return m, nil, true
		}
		m.editBeginUndoCoalesced(editUndoCoalesceInsert)
		m.editClearSelection()
		m.textarea.InsertString("    ")
		m.sourceContent = m.textarea.Value()
		m.updateOutlineFromEditor()
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "shift+tab":
		m.cancelMomentum()
		if !m.editCanOutdentLines() {
			return m, nil, true
		}
		m.editPushUndo()
		if m.editOutdentLines() {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
		}
		return m, nil, true
	case "ctrl+s":
		return m, m.saveCurrentFileCmd(), true
	case "ctrl+shift+s":
		m.formatOnSave = !m.formatOnSave
		m.status = fmt.Sprintf("Format on save %v", onOff(m.formatOnSave))
		return m, nil, true
	case "ctrl+shift+h":
		m.editHomeEndWrapped = !m.editHomeEndWrapped
		if m.editHomeEndWrapped {
			m.status = "Home/end: wrapped"
		} else {
			m.status = "Home/end: logical"
		}
		return m, nil, true
	case "ctrl+l":
		m.startGoToLine()
		return m, nil, true
	case "ctrl+shift+l":
		m.startGoToHeading()
		return m, nil, true
	case "alt+y":
		line := m.currentEditorLineForClipboard()
		if line == "" {
			return m, nil, true
		}
		m.status = "Copied selected text"
		return m, tea.SetClipboard(line), true
	case "ctrl+y", "ctrl+shift+z":
		m.editClearSelection()
		if m.editRedo() {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.status = "Redone"
		}
		return m, nil, true
	case "ctrl+g":
		m.status = "Requested terminal capabilities/colors"
		m.capProbeRequested = true
		cmds := []tea.Cmd{requestTerminalColorsCmd(), requestTermcapProbeCmd()}
		return m, tea.Batch(cmds...), true
	case "ctrl+f":
		return m, m.openSearch(), true
	case "ctrl+p":
		return m.handledAction("find_file")
	case "ctrl+shift+f":
		return m.handledAction("content_search")
	case "alt+z":
		return m.handledAction("toggle_soft_wrap")
	case "ctrl+h":
		return m, m.openReplace(), true
	case "ctrl+tab":
		m.focusRight = !m.focusRight
		if m.focusRight {
			m.textarea.Focus()
		} else {
			m.textarea.Blur()
		}
		target := "preview"
		if !m.focusRight {
			if m.showOutline {
				target = "outline"
			} else {
				target = "files"
			}
		}
		m.status = fmt.Sprintf("Focus: %s", target)
		return m, nil, true
	case "ctrl+left":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMoveWordLeft(false)
		return m, nil, true
	case "ctrl+right":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMoveWordRight(false)
		return m, nil, true
	case "ctrl+shift+left", "shift+ctrl+left":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMoveWordLeft(true)
		return m, nil, true
	case "ctrl+shift+right", "shift+ctrl+right":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMoveWordRight(true)
		return m, nil, true
	case "ctrl+home":
		m.cancelMomentum()
		m.editMoveDocBoundary(false, false)
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "ctrl+end":
		m.cancelMomentum()
		m.editMoveDocBoundary(true, false)
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "ctrl+shift+home", "shift+ctrl+home":
		m.cancelMomentum()
		m.editMoveDocBoundary(false, true)
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "ctrl+shift+end", "shift+ctrl+end":
		m.cancelMomentum()
		m.editMoveDocBoundary(true, true)
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "left", "shift+left":
		return m.handleEditHorizontalKey(msg, -2, true)
	case "right", "shift+right":
		return m.handleEditHorizontalKey(msg, 2, false)
	case "down", "shift+down":
		return m.handleEditVerticalKey(msg, 1)
	case "up", "shift+up":
		return m.handleEditVerticalKey(msg, -1)
	case "pgup":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		if m.editHasSelection() {
			m.editCollapseSelection(false)
		}
		m.editMovePage(false, false)
		return m, nil, true
	case "pgdown":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		if m.editHasSelection() {
			m.editCollapseSelection(true)
		}
		m.editMovePage(true, false)
		return m, nil, true
	case "shift+pgup":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMovePage(false, true)
		return m, nil, true
	case "shift+pgdown":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMovePage(true, true)
		return m, nil, true
	case "ctrl+k":
		m.showCmdPalette = true
		m.cmdFilter = ""
		m.cmdIdx = 0
		m.filteredCommands = m.filterCommands("")
		m.status = "Command palette"
		return m, nil, true
	case "ctrl+b":
		m.editApplyTransform(func() bool { m.insertInlineMarkdown("**", "bold text", "**"); return true })
		return m, nil, true
	case "ctrl+i":
		m.editApplyTransform(func() bool { m.insertInlineMarkdown("_", "italic text", "_"); return true })
		return m, nil, true
	case "shift+home":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(false, true)
		} else {
			m.editMoveLineBoundary(false, true)
		}
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "shift+end":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(true, true)
		} else {
			m.editMoveLineBoundary(true, true)
		}
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "ctrl+a":
		m.cancelMomentum()
		if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(false, false)
		} else {
			m.editMoveLineBoundary(false, false)
		}
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "ctrl+e":
		m.cancelMomentum()
		if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(true, false)
		} else {
			m.editMoveLineBoundary(true, false)
		}
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "ctrl+shift+a", "shift+ctrl+a":
		m.editSetSelectionOffsets(0, len([]rune(m.textarea.Value())))
		return m, nil, true
	case "ctrl+x":
		if m.editHasSelection() {
			text := m.editSelectedText()
			m.editPushUndo()
			m.editDeleteSelection()
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.ensureEditCursorVisibleSoft(false)
			m.status = "Cut"
			return m, tea.SetClipboard(text), true
		}
		return m, nil, true
	case "ctrl+v":
		m.cancelMomentum()
		next, cmd := m.handleBubbleTeaUpdate(msg)
		return next.(model), cmd, true
	case "ctrl+z":
		m.editClearSelection()
		if m.editUndo() {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.status = "Undone"
		}
		return m, nil, true
	case "ctrl+backspace":
		m.cancelMomentum()
		if !m.editHasSelection() {
			offset := m.editCursorOffset()
			if editPrevWordOffset([]rune(m.textarea.Value()), offset) == offset {
				return m, nil, true
			}
		}
		m.editPushUndo()
		if m.editDeleteWordLeft() {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.ensureEditCursorVisibleSoft(false)
		}
		return m, nil, true
	case "ctrl+delete":
		m.cancelMomentum()
		if !m.editHasSelection() {
			offset := m.editCursorOffset()
			if editNextWordOffset([]rune(m.textarea.Value()), offset) == offset {
				return m, nil, true
			}
		}
		m.editPushUndo()
		if m.editDeleteWordRight() {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.ensureEditCursorVisibleSoft(false)
		}
		return m, nil, true
	case "alt+up":
		if !m.editCanMoveLines(-1) {
			return m, nil, true
		}
		m.editPushUndo()
		if m.editMoveLines(-1) {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
		}
		return m, nil, true
	case "alt+down":
		if !m.editCanMoveLines(1) {
			return m, nil, true
		}
		m.editPushUndo()
		if m.editMoveLines(1) {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
		}
		return m, nil, true
	case "ctrl+alt+up", "alt+ctrl+up":
		m.editPushUndo()
		if m.editDuplicateLines(-1) {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
		}
		return m, nil, true
	case "ctrl+alt+down", "alt+ctrl+down":
		m.editPushUndo()
		if m.editDuplicateLines(1) {
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
		}
		return m, nil, true
	case "alt+shift+left", "shift+alt+left":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMoveWordLeft(true)
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "alt+shift+right", "shift+alt+right":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editMoveWordRight(true)
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "alt+shift+up", "shift+alt+up":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editExpandSelectionLine(-1)
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "alt+shift+down", "shift+alt+down":
		m.cancelMomentum()
		m.clearEditSelectionExtend()
		m.editExpandSelectionLine(1)
		m.ensureEditCursorVisibleSoft(true)
		return m, nil, true
	case "alt+[":
		m.cancelMomentum()
		return m, m.editJumpParagraph(false), true
	case "alt+]":
		m.cancelMomentum()
		return m, m.editJumpParagraph(true), true
	case "alt+h":
		m.cancelMomentum()
		return m, m.editJumpHeading(false), true
	case "alt+l":
		m.cancelMomentum()
		return m, m.editJumpHeading(true), true
	case "home":
		m.cancelMomentum()
		if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(false, false)
		} else {
			m.editMoveLineBoundary(false, false)
		}
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "end":
		m.cancelMomentum()
		if m.editHomeEndWrapped {
			m.editMoveVisualLineBoundary(true, false)
		} else {
			m.editMoveLineBoundary(true, false)
		}
		m.ensureEditCursorVisibleSoft(false)
		return m, nil, true
	case "backspace", "delete":
		if m.editHasSelection() {
			m.editBeginUndoCoalesced(keyCoalesceKind)
			m.editDeleteSelection()
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
			m.ensureEditCursorVisibleSoft(false)
			return m, nil, true
		}
		if msg.String() == "backspace" {
			m.editBeginUndoCoalesced(keyCoalesceKind)
			if m.editHandleAutoPairBackspace() {
				return m, nil, true
			}
		}
		offset := m.editCursorOffset()
		if msg.String() == "backspace" && offset == 0 {
			return m, nil, true
		}
		if msg.String() == "delete" && offset >= len([]rune(m.textarea.Value())) {
			return m, nil, true
		}
		m.editBeginUndoCoalesced(keyCoalesceKind)
		m.editClearSelection()
		m.cancelMomentum()
		next, cmd := m.handleBubbleTeaUpdate(msg)
		return next.(model), cmd, true
	default:
		m.cancelMomentum()
		if msg.Text != "" {
			m.editBeginUndoCoalesced(keyCoalesceKind)
			if m.editHandleAutoPairInput(msg.Text) {
				return m, nil, true
			}
		}
		if m.editHasSelection() && msg.Text != "" {
			m.editDeleteSelection()
			m.sourceContent = m.textarea.Value()
			m.updateOutlineFromEditor()
		}
		next, cmd := m.handleBubbleTeaUpdate(msg)
		return next.(model), cmd, true
	}
}

func (m model) handleEditHorizontalKey(msg tea.KeyPressMsg, dir int, left bool) (model, tea.Cmd, bool) {
	repeat := m.isKeyboardScrollRepeat(dir, msg.IsRepeat)
	selecting := m.editShouldExtendSelection(dir, msg)
	if selecting {
		if m.reducedMotion {
			m.cancelMomentum()
			m.editMoveHorizontal(map[bool]int{true: -1, false: 1}[left], true)
			m.armEditSelectionExtend(dir)
			return m, nil, true
		}
		if msg.IsRepeat {
			m.sawNativeKeyRepeat = true
			if m.scrollHoldTickActive {
				m.scrollHoldTickActive = false
				m.scrollHoldTickGen++
			}
		}
		holdDuration := m.updateKeyboardScrollHold(dir)
		impulse, scrollDelta, maxVel := momentumParamsForAxis(momentumAxisEditHorizontal, repeat, holdDuration)
		scrollCmd := m.applyTextareaHorizontalMomentumScroll(left, impulse, scrollDelta, maxVel, true)
		holdCmd := m.ensureKeyboardHoldTickLoop()
		m.armEditSelectionExtend(dir)
		return m, tea.Batch(scrollCmd, holdCmd), true
	}
	m.clearEditSelectionExtend()
	if m.editHasSelection() {
		m.cancelMomentum()
		m.editCollapseSelection(!left)
		return m, nil, true
	}
	m.editClearSelection()
	if m.reducedMotion {
		m.cancelMomentum()
		m.scrollTextareaColumns(map[bool]int{true: -1, false: 1}[left], false)
		return m, nil, true
	}
	if msg.IsRepeat {
		m.sawNativeKeyRepeat = true
		if m.scrollHoldTickActive {
			m.scrollHoldTickActive = false
			m.scrollHoldTickGen++
		}
	}
	holdDuration := m.updateKeyboardScrollHold(dir)
	impulse, scrollDelta, maxVel := momentumParamsForAxis(momentumAxisEditHorizontal, repeat, holdDuration)
	scrollCmd := m.applyTextareaHorizontalMomentumScroll(left, impulse, scrollDelta, maxVel, false)
	holdCmd := m.ensureKeyboardHoldTickLoop()
	return m, tea.Batch(scrollCmd, holdCmd), true
}

func (m model) handleEditVerticalKey(msg tea.KeyPressMsg, dir int) (model, tea.Cmd, bool) {
	if m.mode == modeSplit {
		defer m.splitSyncScroll()
	}
	repeat := m.isKeyboardScrollRepeat(dir, msg.IsRepeat)
	if m.editShouldExtendSelection(dir, msg) {
		if m.reducedMotion {
			m.cancelMomentum()
			m.editMoveVertical(dir, true)
			m.armEditSelectionExtend(dir)
			return m, nil, true
		}
		if msg.IsRepeat {
			m.sawNativeKeyRepeat = true
			if m.scrollHoldTickActive {
				m.scrollHoldTickActive = false
				m.scrollHoldTickGen++
			}
		}
		holdDuration := m.updateKeyboardScrollHold(dir)
		impulse, scrollDelta, maxVel := momentumParamsForAxis(momentumAxisEditVertical, repeat, holdDuration)
		scrollCmd := m.applyTextareaMomentumScroll(dir < 0, impulse, scrollDelta, maxVel, true)
		holdCmd := m.ensureKeyboardHoldTickLoop()
		m.armEditSelectionExtend(dir)
		m.editSetPreferredColumnFromCursor()
		return m, tea.Batch(scrollCmd, holdCmd), true
	}
	m.clearEditSelectionExtend()
	if m.editHasSelection() {
		m.cancelMomentum()
		m.editCollapseSelection(dir > 0)
		m.editSetPreferredColumnFromCursor()
		return m, nil, true
	}
	m.editClearSelection()
	if m.reducedMotion {
		if dir > 0 {
			m.textarea.CursorDown()
		} else {
			m.textarea.CursorUp()
		}
		m.editSetPreferredColumnFromCursor()
		return m, nil, true
	}
	if msg.IsRepeat {
		m.sawNativeKeyRepeat = true
		if m.scrollHoldTickActive {
			m.scrollHoldTickActive = false
			m.scrollHoldTickGen++
		}
	}
	holdDuration := m.updateKeyboardScrollHold(dir)
	impulse, scrollDelta, maxVel := momentumParamsForAxis(momentumAxisEditVertical, repeat, holdDuration)
	scrollCmd := m.applyTextareaMomentumScroll(dir < 0, impulse, scrollDelta, maxVel, false)
	holdCmd := m.ensureKeyboardHoldTickLoop()
	m.editSetPreferredColumnFromCursor()
	return m, tea.Batch(scrollCmd, holdCmd), true
}
