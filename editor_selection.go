package main

import (
	"slices"
	"strings"
)

func (m *model) editMoveWordLeft(selecting bool) {
	if !selecting && m.editHasSelection() {
		m.editCollapseSelection(false)
		return
	}
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	contentRunes := []rune(m.textarea.Value())
	offset := m.editCursorOffset()
	m.editMoveCursorToOffset(editPrevWordOffset(contentRunes, offset))
	m.editClearPreferredColumn()
}

func (m *model) editMoveWordRight(selecting bool) {
	if !selecting && m.editHasSelection() {
		m.editCollapseSelection(true)
		return
	}
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	contentRunes := []rune(m.textarea.Value())
	offset := m.editCursorOffset()
	m.editMoveCursorToOffset(editNextWordOffset(contentRunes, offset))
	m.editClearPreferredColumn()
}

func (m *model) editExpandSelectionLine(delta int) {
	if delta == 0 {
		return
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return
	}
	if !m.editSelActive {
		m.editStartSelection()
	}
	anchorOffset := m.editSelAnchorOffset
	if anchorOffset < 0 {
		anchorOffset = m.editCursorOffset()
	}
	row := m.textarea.Line()
	row += delta
	row = max(0, min(row, len(lines)-1))
	col := 0
	if delta > 0 {
		col = len([]rune(lines[row]))
	}
	cursorOffset := editRowColToRuneOffset(lines, row, col)
	m.editSetSelectionOffsets(anchorOffset, cursorOffset)
	m.editClearPreferredColumn()
}

func (m *model) editMoveHorizontal(delta int, selecting bool) {
	if delta == 0 {
		return
	}
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	row := m.textarea.Line()
	col := m.editCursorCol()
	if delta < 0 {
		if col > 0 {
			m.textarea.SetCursorColumn(col - 1)
		} else if row > 0 {
			m.textarea.CursorUp()
			m.textarea.CursorEnd()
		}
		return
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	if row < len(lines) {
		if col < len([]rune(lines[row])) {
			m.textarea.SetCursorColumn(col + 1)
		} else if row < len(lines)-1 {
			m.textarea.CursorDown()
			m.textarea.SetCursorColumn(0)
		}
	}
}

func (m *model) editMoveDocBoundary(toEnd, selecting bool) {
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	if toEnd {
		lines := strings.Split(m.textarea.Value(), "\n")
		lastRow := max(0, len(lines)-1)
		lastCol := 0
		if len(lines) > 0 {
			lastCol = len([]rune(lines[lastRow]))
		}
		m.editSetCursor(lastRow, lastCol)
		m.editClearPreferredColumn()
		return
	}
	m.editSetCursor(0, 0)
	m.editClearPreferredColumn()
}

func (m *model) editClearPreferredColumn() {
	m.editPreferredColSet = false
}

func (m *model) editSetPreferredColumnFromCursor() {
	m.editPreferredCol = m.editCursorCol()
	m.editPreferredColSet = true
}

func (m *model) editMoveVertical(delta int, selecting bool) {
	if delta == 0 {
		return
	}
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	steps := delta
	if steps < 0 {
		steps = -steps
	}
	for range steps {
		prevRow, prevCol := m.textarea.Line(), m.editCursorCol()
		if delta > 0 {
			m.textarea.CursorDown()
		} else {
			m.textarea.CursorUp()
		}
		if m.textarea.Line() == prevRow && m.editCursorCol() == prevCol {
			break
		}
	}
	if !selecting {
		m.editSetPreferredColumnFromCursor()
	}
}

func (m *model) editMoveLineBoundary(toEnd, selecting bool) {
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	if toEnd {
		m.textarea.CursorEnd()
	} else {
		m.textarea.CursorStart()
	}
	m.editClearPreferredColumn()
}

func (m *model) editMoveVisualLineBoundary(toEnd, selecting bool) {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return
	}
	row := m.textarea.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	width := m.textarea.Width()
	if width <= 0 {
		width = 1
	}
	lineRunes := []rune(lines[row])
	wrapped := editWrapLine(lineRunes, width)
	if len(wrapped) == 0 {
		wrapped = [][]rune{{}}
	}
	col := m.editCursorCol()
	if col < 0 {
		col = 0
	}
	if col > len(lineRunes) {
		col = len(lineRunes)
	}
	segmentStart := 0
	segmentEnd := len(lineRunes)
	cursor := 0
	for i, seg := range wrapped {
		segLen := len(seg)
		next := cursor + segLen
		last := i == len(wrapped)-1
		if col < next || (col == next && last) {
			segmentStart = cursor
			segmentEnd = next
			break
		}
		cursor = next
	}
	targetCol := segmentStart
	if toEnd {
		targetCol = segmentEnd
	}
	if selecting {
		if !m.editSelActive {
			m.editStartSelection()
		}
	} else {
		m.editClearSelection()
	}
	m.textarea.SetCursorColumn(targetCol)
	m.editClearPreferredColumn()
}

func (m *model) editMovePage(down bool, selecting bool) {
	height := m.textarea.Height()
	if height <= 0 {
		height = 1
	}
	step := max(1, height-1)
	delta := -step
	if down {
		delta = step
	}
	m.editMoveVertical(delta, selecting)
	m.ensureEditCursorVisibleSoft(selecting)
}

func (m *model) editCursorDisplayRow() int {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return 0
	}
	return editCursorDisplayRow(lines, max(1, m.textarea.Width()), m.textarea.Line(), m.editCursorCol())
}

func editCursorDisplayRow(lines []string, width, row, col int) int {
	if len(lines) == 0 {
		return 0
	}
	if width <= 0 {
		width = 1
	}
	row = max(0, min(row, len(lines)-1))
	col = max(0, col)
	displayRow := 0
	for i := range row {
		displayRow += len(editWrapLine([]rune(lines[i]), width))
	}
	lineRunes := []rune(lines[row])
	if col > len(lineRunes) {
		col = len(lineRunes)
	}
	segments := editWrapLine(lineRunes, width)
	if len(segments) == 0 {
		return displayRow
	}
	remaining := col
	for i, seg := range segments {
		segLen := len(seg)
		if remaining <= segLen {
			return displayRow + i
		}
		remaining -= segLen
	}
	return displayRow + len(segments) - 1
}

func (m *model) ensureEditCursorInBand(low, high int) {
	if m.mode != modeRaw {
		return
	}
	height := m.textarea.Height()
	if height <= 0 {
		return
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return
	}
	if low < 0 {
		low = 0
	}
	if high >= height {
		high = height - 1
	}
	if low > high {
		low = 0
		high = height - 1
	}
	targetRow := m.textarea.Line()
	targetCol := m.editCursorCol()
	visibleRow := m.editCursorDisplayRow() - m.textarea.ScrollYOffset()
	var adjust int
	if visibleRow < low {
		adjust = visibleRow - low
	} else if visibleRow > high {
		adjust = visibleRow - high
	}
	if adjust == 0 {
		return
	}
	step := 1
	if adjust < 0 {
		step = -1
		adjust = -adjust
	}
	moved := 0
	for range adjust {
		prevRow := m.textarea.Line()
		prevCol := m.editCursorCol()
		if step > 0 {
			m.textarea.CursorDown()
		} else {
			m.textarea.CursorUp()
		}
		if m.textarea.Line() == prevRow && m.editCursorCol() == prevCol {
			break
		}
		moved++
	}
	for range moved {
		prevRow := m.textarea.Line()
		prevCol := m.editCursorCol()
		if step > 0 {
			m.textarea.CursorUp()
		} else {
			m.textarea.CursorDown()
		}
		if m.textarea.Line() == prevRow && m.editCursorCol() == prevCol {
			break
		}
	}
	targetRow = max(0, min(targetRow, len(lines)-1))
	targetCol = max(0, min(targetCol, len([]rune(lines[targetRow]))))
	m.editMoveCursorRowDelta(targetRow - m.textarea.Line())
	m.textarea.SetCursorColumn(targetCol)
}

func (m *model) ensureEditCursorComfort() {
	height := m.textarea.Height()
	if height < 4 {
		return
	}
	bandPad := max(1, height/5)
	low := bandPad
	high := max(low+1, height-bandPad-1)
	m.ensureEditCursorInBand(low, high)
}

func (m *model) ensureEditCursorVisibleSoft(selecting bool) {
	if !selecting {
		m.ensureEditCursorComfort()
		return
	}
	if m.mode != modeRaw {
		return
	}
	height := m.textarea.Height()
	if height <= 1 {
		return
	}
	low := 1
	high := height - 2
	if low > high {
		low = 0
		high = height - 1
	}
	visibleRow := m.editCursorDisplayRow() - m.textarea.ScrollYOffset()
	if visibleRow < low || visibleRow > high {
		m.ensureEditCursorInBand(low, high)
	}
}

func (m *model) editDeleteRuneRange(start, end int) bool {
	runes := []rune(m.textarea.Value())
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return false
	}
	updated := slices.Concat(slices.Clone(runes[:start]), runes[end:])
	updatedContent := string(updated)
	m.textarea.SetValue(updatedContent)
	m.editMoveCursorToOffset(start)
	m.editClearSelection()
	return true
}

func (m *model) editDeleteWordLeft() bool {
	if m.editHasSelection() {
		m.editDeleteSelection()
		return true
	}
	offset := m.editCursorOffset()
	start := editPrevWordOffset([]rune(m.textarea.Value()), offset)
	return m.editDeleteRuneRange(start, offset)
}

func (m *model) editDeleteWordRight() bool {
	if m.editHasSelection() {
		m.editDeleteSelection()
		return true
	}
	offset := m.editCursorOffset()
	end := editNextWordOffset([]rune(m.textarea.Value()), offset)
	return m.editDeleteRuneRange(offset, end)
}

func (m *model) editSelectionLineRange() (start, end int, hasSelection bool) {
	if !m.editHasSelection() {
		line := m.textarea.Line()
		return line, line, false
	}
	sR, _, eR, eC := m.editNormalizedSel()
	start, end = sR, eR
	if eC == 0 && end > start {
		end--
	}
	if end < start {
		end = start
	}
	return start, end, true
}

func (m *model) editCanMoveLines(delta int) bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) <= 1 {
		return false
	}
	start, end, _ := m.editSelectionLineRange()
	if delta < 0 {
		return start > 0
	}
	if delta > 0 {
		return end < len(lines)-1
	}
	return false
}

func (m *model) editAnchorRowCol() (int, int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	if m.editSelAnchorOffset >= 0 {
		return editRuneOffsetToRowCol(lines, m.editSelAnchorOffset)
	}
	row := max(0, min(m.editSelAnchorRow, len(lines)-1))
	col := max(0, min(m.editSelAnchorCol, len([]rune(lines[row]))))
	return row, col
}

func (m *model) editSetSelectionRowCols(anchorRow, anchorCol, cursorRow, cursorCol int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		m.editClearSelection()
		return
	}
	anchorRow = max(0, min(anchorRow, len(lines)-1))
	cursorRow = max(0, min(cursorRow, len(lines)-1))
	anchorCol = max(0, min(anchorCol, len([]rune(lines[anchorRow]))))
	cursorCol = max(0, min(cursorCol, len([]rune(lines[cursorRow]))))
	anchorOffset := editRowColToRuneOffset(lines, anchorRow, anchorCol)
	cursorOffset := editRowColToRuneOffset(lines, cursorRow, cursorCol)
	m.editSetSelectionOffsets(anchorOffset, cursorOffset)
}

func (m *model) editIndentLines() bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return false
	}
	start, end, hasSelection := m.editSelectionLineRange()
	start = max(0, min(start, len(lines)-1))
	end = max(start, min(end, len(lines)-1))

	curRow := m.textarea.Line()
	curCol := m.editCursorCol()
	anchorRow, anchorCol := 0, 0
	if hasSelection {
		anchorRow, anchorCol = m.editAnchorRowCol()
	}

	for i := start; i <= end; i++ {
		lines[i] = "    " + lines[i]
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))

	if curRow >= start && curRow <= end {
		curCol += 4
	}
	if hasSelection {
		if anchorRow >= start && anchorRow <= end {
			anchorCol += 4
		}
		m.editSetSelectionRowCols(anchorRow, anchorCol, curRow, curCol)
	} else {
		m.editSetCursor(curRow, curCol)
		m.editClearSelection()
	}
	m.editClearPreferredColumn()
	m.ensureEditCursorVisibleSoft(hasSelection)
	return true
}

func (m *model) editOutdentLines() bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return false
	}
	start, end, hasSelection := m.editSelectionLineRange()
	start = max(0, min(start, len(lines)-1))
	end = max(start, min(end, len(lines)-1))

	curRow := m.textarea.Line()
	curCol := m.editCursorCol()
	anchorRow, anchorCol := 0, 0
	if hasSelection {
		anchorRow, anchorCol = m.editAnchorRowCol()
	}

	removed := make([]int, len(lines))
	changed := false
	for i := start; i <= end; i++ {
		line := lines[i]
		if strings.HasPrefix(line, "\t") {
			lines[i] = strings.TrimPrefix(line, "\t")
			removed[i] = 1
			changed = true
			continue
		}
		cut := 0
		for cut < 4 && cut < len(line) && line[cut] == ' ' {
			cut++
		}
		if cut > 0 {
			lines[i] = line[cut:]
			removed[i] = cut
			changed = true
		}
	}
	if !changed {
		return false
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))

	adjustCol := func(row, col int) int {
		if row < start || row > end {
			return col
		}
		delta := removed[row]
		if delta <= 0 {
			return col
		}
		if col <= delta {
			return 0
		}
		return col - delta
	}

	curCol = adjustCol(curRow, curCol)
	if hasSelection {
		anchorCol = adjustCol(anchorRow, anchorCol)
		m.editSetSelectionRowCols(anchorRow, anchorCol, curRow, curCol)
	} else {
		m.editSetCursor(curRow, curCol)
		m.editClearSelection()
	}
	m.editClearPreferredColumn()
	m.ensureEditCursorVisibleSoft(hasSelection)
	return true
}

func (m *model) editCanOutdentLines() bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return false
	}
	start, end, _ := m.editSelectionLineRange()
	start = max(0, min(start, len(lines)-1))
	end = max(start, min(end, len(lines)-1))
	for i := start; i <= end; i++ {
		line := lines[i]
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") {
			return true
		}
	}
	return false
}

func (m *model) editSetSelectionOffsets(anchorOffset, cursorOffset int) {
	runes := []rune(m.textarea.Value())
	if anchorOffset < 0 {
		anchorOffset = 0
	}
	if cursorOffset < 0 {
		cursorOffset = 0
	}
	if anchorOffset > len(runes) {
		anchorOffset = len(runes)
	}
	if cursorOffset > len(runes) {
		cursorOffset = len(runes)
	}
	lines := strings.Split(string(runes), "\n")
	aRow, aCol := editRuneOffsetToRowCol(lines, anchorOffset)
	cRow, cCol := editRuneOffsetToRowCol(lines, cursorOffset)
	m.editSelActive = true
	m.editSelAnchorRow = aRow
	m.editSelAnchorCol = aCol
	m.editSelAnchorOffset = anchorOffset
	m.editSetCursor(cRow, cCol)
}

func (m *model) editSetSelectionLines(startLine, endLine int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		m.editClearSelection()
		return
	}
	startLine = max(0, min(startLine, len(lines)-1))
	endLine = max(0, min(endLine, len(lines)-1))
	anchor := editRowColToRuneOffset(lines, startLine, 0)
	cursor := editRowColToRuneOffset(lines, endLine, len([]rune(lines[endLine])))
	m.editSetSelectionOffsets(anchor, cursor)
}

func (m *model) editMoveLines(delta int) bool {
	if delta == 0 || !m.editCanMoveLines(delta) {
		return false
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	start, end, hasSelection := m.editSelectionLineRange()
	oldRow := m.textarea.Line()
	oldCol := m.editCursorCol()
	block := slices.Clone(lines[start : end+1])
	var updated []string
	newStart := start
	newEnd := end
	if delta < 0 {
		updated = slices.Concat(lines[:start-1], block, []string{lines[start-1]}, lines[end+1:])
		newStart--
		newEnd--
	} else {
		updated = slices.Concat(lines[:start], []string{lines[end+1]}, block, lines[end+2:])
		newStart++
		newEnd++
	}
	m.textarea.SetValue(strings.Join(updated, "\n"))
	if hasSelection {
		m.editSetSelectionLines(newStart, newEnd)
	} else {
		m.editClearSelection()
		m.editSetCursor(oldRow+delta, oldCol)
	}
	m.ensureEditCursorVisibleSoft(hasSelection)
	return true
}

func (m *model) editDuplicateLines(direction int) bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return false
	}
	start, end, hasSelection := m.editSelectionLineRange()
	oldRow := m.textarea.Line()
	oldCol := m.editCursorCol()
	block := slices.Clone(lines[start : end+1])

	insertAt := start
	if direction > 0 {
		insertAt = end + 1
	}
	updated := slices.Concat(lines[:insertAt], block, lines[insertAt:])
	m.textarea.SetValue(strings.Join(updated, "\n"))

	newStart := insertAt
	newEnd := insertAt + len(block) - 1
	if hasSelection {
		m.editSetSelectionLines(newStart, newEnd)
	} else {
		m.editClearSelection()
		newRow := oldRow
		if direction > 0 {
			newRow = oldRow + len(block)
		}
		m.editSetCursor(newRow, oldCol)
	}
	m.ensureEditCursorVisibleSoft(hasSelection)
	return true
}

// insertInlineMarkdown wraps the cursor position with opener+placeholder+closer
// and positions the cursor at the start of the placeholder.
func (m *model) insertInlineMarkdown(opener, placeholder, closer string) {
	if m.editHasSelection() {
		sR, sC, eR, eC := m.editNormalizedSel()
		lines := strings.Split(m.textarea.Value(), "\n")
		startOffset := editRowColToRuneOffset(lines, sR, sC)
		endOffset := editRowColToRuneOffset(lines, eR, eC)

		allRunes := []rune(m.textarea.Value())
		openRunes := []rune(opener)
		closeRunes := []rune(closer)

		canUnwrap := startOffset >= len(openRunes) &&
			endOffset+len(closeRunes) <= len(allRunes) &&
			equalRunes(allRunes[startOffset-len(openRunes):startOffset], openRunes) &&
			equalRunes(allRunes[endOffset:endOffset+len(closeRunes)], closeRunes)

		var updated []rune
		var selStart, selEnd int
		if canUnwrap {
			updated = append(updated, allRunes[:startOffset-len(openRunes)]...)
			updated = append(updated, allRunes[startOffset:endOffset]...)
			updated = append(updated, allRunes[endOffset+len(closeRunes):]...)
			selStart = startOffset - len(openRunes)
			selEnd = selStart + (endOffset - startOffset)
		} else {
			updated = append(updated, allRunes[:startOffset]...)
			updated = append(updated, openRunes...)
			updated = append(updated, allRunes[startOffset:endOffset]...)
			updated = append(updated, closeRunes...)
			updated = append(updated, allRunes[endOffset:]...)
			selStart = startOffset + len(openRunes)
			selEnd = selStart + (endOffset - startOffset)
		}

		updatedContent := string(updated)
		m.textarea.SetValue(updatedContent)
		m.editSetSelectionOffsets(selStart, selEnd)
		return
	}
	m.editClearSelection()
	info := m.textarea.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	m.textarea.InsertString(opener + placeholder + closer)
	m.textarea.SetCursorColumn(col + len([]rune(opener)))
}

// toggleLinePrefix toggles a line prefix (e.g. "# ", "> ", "- ") on the
// current line, then restores the cursor to the equivalent text position.
func (m *model) toggleLinePrefix(prefix string) {
	line := m.textarea.Line()
	info := m.textarea.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	content := m.textarea.Value()
	lines := strings.Split(content, "\n")
	prefixRunes := len([]rune(prefix))
	newCol := col
	if line >= 0 && line < len(lines) {
		if strings.HasPrefix(lines[line], prefix) {
			lines[line] = strings.TrimPrefix(lines[line], prefix)
			newCol = max(0, col-prefixRunes)
		} else {
			lines[line] = prefix + lines[line]
			newCol = col + prefixRunes
		}
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))
	m.textarea.MoveToBegin()
	for range line {
		m.textarea.CursorDown()
	}
	m.textarea.SetCursorColumn(newCol)
}

// insertBlockAtCursor appends a newline + block after the current line and
// positions the cursor at (innerLineOffset, innerCol) within the inserted block.
func (m *model) insertBlockAtCursor(block string, innerLineOffset, innerCol int) {
	m.textarea.CursorEnd()
	m.textarea.InsertString("\n" + block)
	numBlockLines := strings.Count(block, "\n") + 1
	upCount := numBlockLines - 1 - innerLineOffset
	for range upCount {
		m.textarea.CursorUp()
	}
	m.textarea.SetCursorColumn(innerCol)
}

func (m *model) editCursorCol() int {
	info := m.textarea.LineInfo()
	return info.StartColumn + info.ColumnOffset
}

func (m *model) editStartSelection() {
	m.editSetSelectionOffsets(m.editCursorOffset(), m.editCursorOffset())
}

func (m *model) editClearSelection() {
	m.editSelActive = false
	m.editSelAnchorOffset = -1
}

func (m *model) editHasSelection() bool {
	if !m.editSelActive {
		return false
	}
	sR, sC, eR, eC := m.editNormalizedSel()
	return sR != eR || sC != eC
}

func (m *model) editSetCursor(row, col int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		row = 0
		col = 0
	} else {
		row = max(0, min(row, len(lines)-1))
		col = max(0, min(col, len([]rune(lines[row]))))
	}
	curRow := m.textarea.Line()
	if curRow < 0 {
		curRow = 0
	}
	if curRow < row {
		m.editMoveCursorRowDelta(row - curRow)
	} else if curRow > row {
		m.editMoveCursorRowDelta(-(curRow - row))
	}
	m.textarea.SetCursorColumn(col)
}

func (m *model) editMoveCursorRowDelta(delta int) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		for range delta {
			prev := m.textarea.Line()
			m.textarea.CursorDown()
			if m.textarea.Line() == prev {
				break
			}
		}
		return
	}
	for range -delta {
		prev := m.textarea.Line()
		m.textarea.CursorUp()
		if m.textarea.Line() == prev {
			break
		}
	}
}

func (m *model) editCollapseSelection(toEnd bool) {
	if !m.editSelActive {
		return
	}
	if !m.editHasSelection() {
		m.editClearSelection()
		return
	}
	sR, sC, eR, eC := m.editNormalizedSel()
	row, col := sR, sC
	if toEnd {
		row, col = eR, eC
	}
	m.editSetCursor(row, col)
	m.editClearSelection()
}

func (m *model) editNormalizedSel() (int, int, int, int) {
	lines := strings.Split(m.textarea.Value(), "\n")
	curOffset := editRowColToRuneOffset(lines, m.textarea.Line(), m.editCursorCol())
	anchorOffset := m.editSelAnchorOffset
	if anchorOffset < 0 {
		anchorOffset = editRowColToRuneOffset(lines, m.editSelAnchorRow, m.editSelAnchorCol)
	}
	aRow, aCol := editRuneOffsetToRowCol(lines, anchorOffset)
	curRow, curCol := editRuneOffsetToRowCol(lines, curOffset)
	if anchorOffset <= curOffset {
		return aRow, aCol, curRow, curCol
	}
	return curRow, curCol, aRow, aCol
}

func (m *model) editSelectedText() string {
	sR, sC, eR, eC := m.editNormalizedSel()
	lines := strings.Split(m.textarea.Value(), "\n")
	if sR == eR {
		r := []rune(lines[sR])
		return string(r[sC:min(eC, len(r))])
	}
	var parts []string
	parts = append(parts, string([]rune(lines[sR])[sC:]))
	for i := range eR - sR - 1 {
		parts = append(parts, lines[sR+1+i])
	}
	r := []rune(lines[eR])
	parts = append(parts, string(r[:min(eC, len(r))]))
	return strings.Join(parts, "\n")
}

func (m *model) editDeleteSelection() {
	if !m.editSelActive {
		return
	}
	if !m.editHasSelection() {
		m.editClearSelection()
		return
	}
	sR, sC, eR, eC := m.editNormalizedSel()
	lines := strings.Split(m.textarea.Value(), "\n")
	if sR == eR {
		r := []rune(lines[sR])
		lines[sR] = string(r[:sC]) + string(r[min(eC, len(r)):])
	} else {
		sRunes, eRunes := []rune(lines[sR]), []rune(lines[eR])
		merged := string(sRunes[:sC]) + string(eRunes[min(eC, len(eRunes)):])
		newLines := slices.Concat(lines[:sR], []string{merged}, lines[eR+1:])
		lines = newLines
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))
	m.textarea.MoveToBegin()
	for range sR {
		m.textarea.CursorDown()
	}
	m.textarea.SetCursorColumn(sC)
	m.editClearSelection()
}
