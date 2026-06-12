package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

func (m *model) positionEditorCursor() {
	targetLine, targetCol := m.targetEditPosition()
	val := m.sourceContent
	m.textarea.SetValue(val)
	lines := strings.Split(val, "\n")
	if targetLine < 0 {
		targetLine = 0
	}
	if targetLine >= len(lines) {
		targetLine = len(lines) - 1
	}
	// SetValue leaves the cursor at the end; jump directly to the start.
	m.textarea.MoveToBegin()
	for range targetLine {
		m.textarea.CursorDown()
	}
	// The cursor is now at targetLine, but the textarea viewport has been
	// scrolling it into view from the bottom — so targetLine sits at the
	// bottom of the visible editor area, showing content above the preview
	// position. Fix: overshoot by one viewport-height then walk back up.
	// repositionView() only scrolls when the cursor leaves the visible range,
	// so the walk-back doesn't move the viewport — leaving targetLine at top.
	height := m.textarea.Height()
	if height <= 0 {
		height = 1
	}
	extra := min(height-1, len(lines)-1-targetLine)
	for range extra {
		m.textarea.CursorDown()
	}
	for range extra {
		m.textarea.CursorUp()
	}
	m.textarea.SetCursorColumn(targetCol)
	m.updateOutlineFromEditor()
}

func editRowColToRuneOffset(lines []string, row, col int) int {
	if len(lines) == 0 {
		return 0
	}
	if row < 0 {
		row = 0
	} else if row >= len(lines) {
		row = len(lines) - 1
	}
	offset := 0
	for i := range row {
		offset += len([]rune(lines[i])) + 1
	}
	lineRunes := []rune(lines[row])
	if col < 0 {
		col = 0
	} else if col > len(lineRunes) {
		col = len(lineRunes)
	}
	return offset + col
}

func editRuneOffsetToRowCol(lines []string, offset int) (row, col int) {
	if len(lines) == 0 {
		return 0, 0
	}
	if offset < 0 {
		offset = 0
	}
	for i, line := range lines {
		lineLen := len([]rune(line))
		if offset <= lineLen {
			return i, offset
		}
		offset -= lineLen
		if i < len(lines)-1 {
			if offset == 0 {
				return i + 1, 0
			}
			offset--
		}
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last]))
}

func equalRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *model) editCursorOffset() int {
	lines := strings.Split(m.textarea.Value(), "\n")
	return editRowColToRuneOffset(lines, m.textarea.Line(), m.editCursorCol())
}

func (m *model) editMoveCursorToOffset(offset int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	row, col := editRuneOffsetToRowCol(lines, offset)
	m.editSetCursor(row, col)
}

func editIsWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func editRuneClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if editIsWordRune(r) {
		return 1
	}
	return 2
}

func editPrevWordOffset(runes []rune, offset int) int {
	if offset <= 0 {
		return 0
	}
	i := offset - 1
	for i >= 0 && unicode.IsSpace(runes[i]) {
		i--
	}
	if i < 0 {
		return 0
	}
	class := editRuneClass(runes[i])
	for i > 0 && editRuneClass(runes[i-1]) == class {
		i--
	}
	return i
}

func editNextWordOffset(runes []rune, offset int) int {
	if offset >= len(runes) {
		return len(runes)
	}
	i := offset
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	if i >= len(runes) {
		return len(runes)
	}
	class := editRuneClass(runes[i])
	for i < len(runes) && editRuneClass(runes[i]) == class {
		i++
	}
	return i
}

func editIsVerticalEditorKey(key string) bool {
	switch key {
	case "up", "down", "shift+up", "shift+down":
		return true
	default:
		return false
	}
}

func (m *model) flashEditFocusLine(line int) tea.Cmd {
	if m.mode != modeRaw {
		return nil
	}
	m.editFocusLine = line
	m.editFocusLineGen++
	gen := m.editFocusLineGen
	return tea.Tick(editFocusFlashDuration, func(time.Time) tea.Msg {
		return editFocusLineClearMsg{generation: gen}
	})
}

func (m *model) editJumpToPosition(row, col int) tea.Cmd {
	m.cancelMomentum()
	m.editClearSelection()
	m.editSetCursor(row, col)
	m.editClearPreferredColumn()
	m.ensureEditCursorVisibleSoft(false)
	idx := headingIndexForSourceLine(m.headings, m.textarea.Line())
	m.setCurrentHeading(idx)
	m.updateBreadcrumb()
	return m.flashEditFocusLine(m.textarea.Line())
}

func (m *model) editJumpParagraph(next bool) tea.Cmd {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return nil
	}
	isBlank := func(s string) bool { return strings.TrimSpace(s) == "" }
	row := max(0, min(m.textarea.Line(), len(lines)-1))
	if next {
		end := row
		for end+1 < len(lines) && !isBlank(lines[end+1]) {
			end++
		}
		i := end + 1
		for i < len(lines) && isBlank(lines[i]) {
			i++
		}
		if i >= len(lines) {
			return nil
		}
		m.status = "Next paragraph"
		return m.editJumpToPosition(i, 0)
	}
	start := row
	for start > 0 && !isBlank(lines[start-1]) {
		start--
	}
	i := start - 1
	for i >= 0 && isBlank(lines[i]) {
		i--
	}
	if i < 0 {
		return nil
	}
	for i > 0 && !isBlank(lines[i-1]) {
		i--
	}
	m.status = "Previous paragraph"
	return m.editJumpToPosition(i, 0)
}

func (m *model) editJumpHeading(next bool) tea.Cmd {
	if len(m.headings) == 0 {
		return nil
	}
	line := m.textarea.Line()
	target := -1
	if next {
		for i, h := range m.headings {
			if h.line > line {
				target = i
				break
			}
		}
	} else {
		for i := len(m.headings) - 1; i >= 0; i-- {
			if m.headings[i].line < line {
				target = i
				break
			}
		}
	}
	if target < 0 || target >= len(m.headings) {
		return nil
	}
	m.status = fmt.Sprintf("Heading: %s", m.headings[target].title)
	return m.editJumpToPosition(m.headings[target].line, 0)
}
