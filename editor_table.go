package main

import "strings"

func parseTableAtCursor(content string, cursorLine int) (grid [][]string, startLine, endLine int) {
	lines := strings.Split(content, "\n")
	if cursorLine < 0 || cursorLine >= len(lines) {
		return nil, -1, -1
	}
	// Check if current line looks like a table row
	if !strings.Contains(lines[cursorLine], "|") {
		return nil, -1, -1
	}
	// Find table bounds (scan up and down)
	start := cursorLine
	for start > 0 && strings.Contains(lines[start-1], "|") {
		start--
	}
	end := cursorLine
	for end < len(lines)-1 && strings.Contains(lines[end+1], "|") {
		end++
	}
	// Parse table rows
	for i := start; i <= end; i++ {
		row := parseTableRow(lines[i])
		if len(row) == 0 {
			continue
		}
		// Skip separator rows (|---|---|)
		isSep := true
		for _, cell := range row {
			trimmed := strings.TrimSpace(cell)
			trimmed = strings.Trim(trimmed, "-: ")
			if trimmed != "" {
				isSep = false
				break
			}
		}
		if isSep {
			continue
		}
		grid = append(grid, row)
	}
	if len(grid) < 1 {
		return nil, -1, -1
	}
	return grid, start, end + 1
}

func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	// Strip leading/trailing pipes
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func formatTable(grid [][]string) string {
	if len(grid) == 0 {
		return ""
	}
	// Determine column count and max widths
	cols := 0
	for _, row := range grid {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return ""
	}
	widths := make([]int, cols)
	for _, row := range grid {
		for i, cell := range row {
			if i < cols && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	// Minimum width 3 for separator
	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
	}
	var out strings.Builder
	for rowIdx, row := range grid {
		out.WriteByte('|')
		for i := range cols {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			out.WriteByte(' ')
			out.WriteString(cell)
			padding := widths[i] - len(cell)
			for p := 0; p < padding; p++ {
				out.WriteByte(' ')
			}
			out.WriteString(" |")
		}
		out.WriteByte('\n')
		// Add separator after first row (header)
		if rowIdx == 0 {
			out.WriteByte('|')
			for i := range cols {
				out.WriteByte(' ')
				for w := 0; w < widths[i]; w++ {
					out.WriteByte('-')
				}
				out.WriteString(" |")
			}
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// editCheckTableMode detects whether the cursor is in a markdown table
// and enters/exits table editing sub-mode automatically.
func (m *model) editCheckTableMode() {
	line := m.textarea.Line()
	lines := strings.Split(m.textarea.Value(), "\n")
	if line < 0 || line >= len(lines) {
		if m.editTableMode {
			m.editExitTableMode()
		}
		return
	}
	if !strings.Contains(lines[line], "|") {
		if m.editTableMode {
			m.editExitTableMode()
		}
		return
	}
	// Find table bounds
	start := line
	for start > 0 && strings.Contains(lines[start-1], "|") {
		start--
	}
	end := line
	for end < len(lines)-1 && strings.Contains(lines[end+1], "|") {
		end++
	}
	// Need at least 2 rows (header + separator) to be a real table
	if end-start < 1 {
		if m.editTableMode {
			m.editExitTableMode()
		}
		return
	}
	if !m.editTableMode {
		m.editTableMode = true
		m.tableStartLine = start
		m.tableEndLine = end + 1
		m.status = "Table mode"
	} else {
		m.tableStartLine = start
		m.tableEndLine = end + 1
	}
}

// editExitTableMode auto-formats the table and exits table sub-mode.
func (m *model) editExitTableMode() {
	if !m.editTableMode {
		return
	}
	// Parse and reformat the table for alignment
	content := m.textarea.Value()
	grid, startLine, endLine := parseTableAtCursor(content, m.tableStartLine)
	if grid != nil && len(grid) > 0 {
		lines := strings.Split(content, "\n")
		formatted := formatTable(grid)
		formattedLines := strings.Split(strings.TrimSuffix(formatted, "\n"), "\n")
		// Replace lines from startLine to endLine-1 with formatted lines
		var result []string
		result = append(result, lines[:startLine]...)
		result = append(result, formattedLines...)
		if endLine < len(lines) {
			result = append(result, lines[endLine:]...)
		}
		newContent := strings.Join(result, "\n")
		if newContent != content {
			m.editPushUndo()
			curRow := m.textarea.Line()
			curCol := m.editCursorCol()
			m.textarea.SetValue(newContent)
			m.editSetCursor(curRow, curCol)
			m.sourceContent = newContent
			m.updateOutlineFromEditor()
		}
	}
	m.editTableMode = false
	m.tableGrid = nil
	m.status = ""
}

// editTableNextCell moves the cursor to the next cell in the table.
// Wraps to the first cell of the next row if at end of row.
func (m *model) editTableNextCell() {
	lines := strings.Split(m.textarea.Value(), "\n")
	row := m.textarea.Line()
	col := m.editCursorCol()
	if row < 0 || row >= len(lines) {
		return
	}
	line := lines[row]
	lineRunes := []rune(line)
	// Find next | after current position
	nextPipe := -1
	for i := col + 1; i < len(lineRunes); i++ {
		if lineRunes[i] == '|' {
			nextPipe = i
			break
		}
	}
	if nextPipe >= 0 && nextPipe < len(lineRunes)-1 {
		// Move past the pipe and any leading space
		target := nextPipe + 1
		for target < len(lineRunes) && lineRunes[target] == ' ' {
			target++
		}
		m.textarea.SetCursorColumn(target)
		return
	}
	// At end of row — try to move to next data row
	nextRow := row + 1
	// Skip separator rows
	for nextRow < len(lines) && nextRow < m.tableEndLine {
		if isTableSeparatorRow(lines[nextRow]) {
			nextRow++
			continue
		}
		break
	}
	if nextRow < m.tableEndLine && nextRow < len(lines) {
		m.editSetCursor(nextRow, 0)
		// Move to first cell content (past leading |)
		lr := []rune(lines[nextRow])
		target := 0
		for target < len(lr) && lr[target] == '|' {
			target++
		}
		for target < len(lr) && lr[target] == ' ' {
			target++
		}
		m.textarea.SetCursorColumn(target)
	}
}

// editTablePrevCell moves the cursor to the previous cell in the table.
func (m *model) editTablePrevCell() {
	lines := strings.Split(m.textarea.Value(), "\n")
	row := m.textarea.Line()
	col := m.editCursorCol()
	if row < 0 || row >= len(lines) {
		return
	}
	line := lines[row]
	lineRunes := []rune(line)
	// Find pipe before current position (skip the one we might be right next to)
	searchFrom := col - 1
	// Skip backwards past spaces to find the pipe
	for searchFrom >= 0 && lineRunes[searchFrom] == ' ' {
		searchFrom--
	}
	if searchFrom >= 0 && lineRunes[searchFrom] == '|' {
		searchFrom--
	}
	// Find previous pipe
	prevPipe := -1
	for i := searchFrom; i >= 0; i-- {
		if lineRunes[i] == '|' {
			prevPipe = i
			break
		}
	}
	if prevPipe >= 0 {
		// Move past the pipe and any leading space
		target := prevPipe + 1
		for target < len(lineRunes) && lineRunes[target] == ' ' {
			target++
		}
		m.textarea.SetCursorColumn(target)
		return
	}
	// At start of row — try to move to prev data row
	prevRow := row - 1
	for prevRow >= m.tableStartLine {
		if isTableSeparatorRow(lines[prevRow]) {
			prevRow--
			continue
		}
		break
	}
	if prevRow >= m.tableStartLine {
		lr := []rune(lines[prevRow])
		// Find last cell: find the last pipe that's not at end
		lastPipe := -1
		for i := len(lr) - 2; i >= 0; i-- {
			if lr[i] == '|' {
				lastPipe = i
				break
			}
		}
		if lastPipe >= 0 {
			target := lastPipe + 1
			for target < len(lr) && lr[target] == ' ' {
				target++
			}
			m.editSetCursor(prevRow, target)
		} else {
			m.editSetCursor(prevRow, 0)
		}
	}
}

// editTableAddRow adds a new empty row at the end of the table.
func (m *model) editTableAddRow() {
	lines := strings.Split(m.textarea.Value(), "\n")
	if m.tableEndLine <= 0 || m.tableEndLine > len(lines) {
		return
	}
	// Count columns from header row
	headerRow := parseTableRow(lines[m.tableStartLine])
	cols := len(headerRow)
	if cols == 0 {
		cols = 1
	}
	// Build empty row
	var row strings.Builder
	row.WriteByte('|')
	for range cols {
		row.WriteString("   |")
	}
	newRow := row.String()
	// Insert after last table line
	var result []string
	result = append(result, lines[:m.tableEndLine]...)
	result = append(result, newRow)
	if m.tableEndLine < len(lines) {
		result = append(result, lines[m.tableEndLine:]...)
	}
	m.editPushUndo()
	newContent := strings.Join(result, "\n")
	m.textarea.SetValue(newContent)
	m.sourceContent = newContent
	m.tableEndLine++
	// Move cursor to new row, first cell
	m.editSetCursor(m.tableEndLine-1, 2)
	m.updateOutlineFromEditor()
}

func isTableSeparatorRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return false
	}
	trimmed = strings.Trim(trimmed, "|")
	for _, cell := range strings.Split(trimmed, "|") {
		cell = strings.TrimSpace(cell)
		cell = strings.Trim(cell, "-: ")
		if cell != "" {
			return false
		}
	}
	return true
}
