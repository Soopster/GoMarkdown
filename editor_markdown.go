package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type markdownListContext struct {
	indent     string
	prefix     string
	content    string
	ordered    bool
	orderNum   int
	orderDelim string
	bullet     string
	task       bool
}

func parseMarkdownListContext(line string) (markdownListContext, bool) {
	if m := reListTask.FindStringSubmatch(line); len(m) == 5 {
		indent, bullet, content := m[1], m[2], m[4]
		prefix := indent + bullet + " [ ] "
		return markdownListContext{
			indent:  indent,
			prefix:  prefix,
			content: content,
			bullet:  bullet,
			task:    true,
			ordered: false,
		}, true
	}
	if m := reListOrdered.FindStringSubmatch(line); len(m) == 5 {
		indent, numText, delim, content := m[1], m[2], m[3], m[4]
		n, err := strconv.Atoi(numText)
		if err != nil {
			return markdownListContext{}, false
		}
		prefix := fmt.Sprintf("%s%d%s ", indent, n, delim)
		return markdownListContext{
			indent:     indent,
			prefix:     prefix,
			content:    content,
			ordered:    true,
			orderNum:   n,
			orderDelim: delim,
		}, true
	}
	if m := reListUnordered.FindStringSubmatch(line); len(m) == 4 {
		indent, bullet, content := m[1], m[2], m[3]
		prefix := indent + bullet + " "
		return markdownListContext{
			indent:  indent,
			prefix:  prefix,
			content: content,
			bullet:  bullet,
		}, true
	}
	return markdownListContext{}, false
}

func (ctx markdownListContext) continuationPrefix() string {
	if ctx.task {
		return ctx.indent + ctx.bullet + " [ ] "
	}
	if ctx.ordered {
		return fmt.Sprintf("%s%d%s ", ctx.indent, ctx.orderNum+1, ctx.orderDelim)
	}
	return ctx.indent + ctx.bullet + " "
}

func (m *model) editHandleSmartEnter() bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return false
	}
	row := max(0, min(m.textarea.Line(), len(lines)-1))
	lineRunes := []rune(lines[row])
	col := max(0, min(m.editCursorCol(), len(lineRunes)))
	ctx, ok := parseMarkdownListContext(lines[row])
	if !ok {
		return false
	}
	contentEmpty := strings.TrimSpace(ctx.content) == ""
	prefixLen := len([]rune(ctx.prefix))
	if contentEmpty && col >= prefixLen {
		indentLen := len([]rune(ctx.indent))
		lineLen := len(lineRunes)
		m.textarea.SetCursorColumn(lineLen)
		wasFocused := m.textarea.Focused()
		if !wasFocused {
			m.textarea.Focus()
		}
		for m.editCursorCol() > indentLen {
			prevCol := m.editCursorCol()
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
			_ = cmd
			if m.editCursorCol() >= prevCol {
				break
			}
		}
		if !wasFocused {
			m.textarea.Blur()
		}
		return true
	}
	insert := ctx.continuationPrefix()
	// Avoid SetValue() here; it resets textarea viewport state and can cause a
	// large visual jump while editing long lists near the bottom of the screen.
	m.textarea.InsertString("\n" + insert)
	return true
}

func normalizeMarkdownIndent(indent string) string {
	if indent == "" {
		return ""
	}
	count := 0
	for _, r := range indent {
		switch r {
		case '\t':
			count += 4
		case ' ':
			count++
		}
	}
	if count <= 0 {
		return ""
	}
	if count%2 != 0 {
		count--
	}
	return strings.Repeat(" ", count)
}

func normalizeMarkdownListLine(line string) string {
	if m := reListTask.FindStringSubmatch(line); len(m) == 5 {
		indent := normalizeMarkdownIndent(m[1])
		bullet := m[2]
		state := m[3]
		content := strings.TrimLeft(m[4], " \t")
		return fmt.Sprintf("%s%s [%s] %s", indent, bullet, state, content)
	}
	if m := reListOrdered.FindStringSubmatch(line); len(m) == 5 {
		indent := normalizeMarkdownIndent(m[1])
		number := m[2]
		delim := m[3]
		content := strings.TrimLeft(m[4], " \t")
		return fmt.Sprintf("%s%s%s %s", indent, number, delim, content)
	}
	if m := reListUnordered.FindStringSubmatch(line); len(m) == 4 {
		indent := normalizeMarkdownIndent(m[1])
		bullet := m[2]
		content := strings.TrimLeft(m[3], " \t")
		return fmt.Sprintf("%s%s %s", indent, bullet, content)
	}
	return line
}

func isMarkdownFenceLine(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) < 3 {
		return false
	}
	if strings.HasPrefix(line, "```") {
		return true
	}
	if strings.HasPrefix(line, "~~~") {
		return true
	}
	return false
}

func normalizeMarkdownForSave(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines)+8)
	inFence := false
	for i, line := range lines {
		normalized := line
		if !inFence {
			if m := reHeadingTight.FindStringSubmatch(normalized); len(m) == 3 {
				normalized = m[1] + " " + m[2]
			}
			normalized = normalizeMarkdownListLine(normalized)
		}
		if isMarkdownFenceLine(normalized) {
			if !inFence {
				if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
					out = append(out, "")
				}
				out = append(out, normalized)
				inFence = true
				continue
			}
			out = append(out, normalized)
			inFence = false
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, normalized)
	}
	return strings.Join(out, "\n")
}

func (m *model) applyFormatOnSaveIfEnabled() bool {
	if !m.formatOnSave || m.mode != modeRaw {
		return false
	}
	before := m.textarea.Value()
	after := normalizeMarkdownForSave(before)
	if after == before {
		return false
	}
	cursorOffset := m.editCursorOffset()
	m.editPushUndo()
	m.textarea.SetValue(after)
	maxOffset := len([]rune(after))
	if cursorOffset > maxOffset {
		cursorOffset = maxOffset
	}
	m.editMoveCursorToOffset(cursorOffset)
	m.editClearSelection()
	m.ensureEditCursorVisibleSoft(false)
	m.sourceContent = after
	m.updateOutlineFromEditor()
	return true
}

func (m *model) saveCurrentFileCmd() tea.Cmd {
	if m.currentPath == "" {
		return nil
	}
	if m.applyFormatOnSaveIfEnabled() {
		m.status = "Formatted"
	}
	return m.saveFileCmd(m.currentPath, m.sourceContent)
}

func (m *model) editApplyTransform(fn func() bool) bool {
	m.editBeginUndoCoalesced(editUndoCoalesceTransform)
	changed := fn()
	if changed {
		m.sourceContent = m.textarea.Value()
		m.updateOutlineFromEditor()
		m.ensureEditCursorVisibleSoft(m.editHasSelection())
	}
	m.editResetUndoCoalescing()
	return changed
}

func isMarkdownFenceBoundaryLine(line string) bool {
	return strings.TrimSpace(line) == "```"
}

func (m *model) editCanAutoPair(opener rune) bool {
	if opener != '`' {
		return true
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	row := m.textarea.Line()
	if row < 0 || row >= len(lines) {
		return true
	}
	return !isMarkdownFenceBoundaryLine(lines[row])
}

func (m *model) editHandleAutoPairBackspace() bool {
	if m.editHasSelection() {
		return false
	}
	offset := m.editCursorOffset()
	runes := []rune(m.textarea.Value())
	if offset <= 0 || offset >= len(runes) {
		return false
	}
	left := runes[offset-1]
	right := runes[offset]
	closer, ok := autoPairClosers[left]
	if !ok || closer != right {
		return false
	}
	updated := slices.Concat(slices.Clone(runes[:offset-1]), runes[offset+1:])
	m.textarea.SetValue(string(updated))
	m.editMoveCursorToOffset(offset - 1)
	m.editClearSelection()
	m.editClearPreferredColumn()
	m.sourceContent = m.textarea.Value()
	m.updateOutlineFromEditor()
	m.ensureEditCursorVisibleSoft(false)
	return true
}

func (m *model) editHandleAutoPairInput(text string) bool {
	runes := []rune(text)
	if len(runes) != 1 {
		return false
	}
	typed := runes[0]
	isCloserRune := false
	for _, c := range autoPairClosers {
		if typed == c {
			isCloserRune = true
			break
		}
	}
	// Skip over an existing closer instead of inserting duplicate closers.
	if isCloserRune && !m.editHasSelection() {
		content := strings.Split(m.textarea.Value(), "\n")
		row := m.textarea.Line()
		col := m.editCursorCol()
		if row >= 0 && row < len(content) {
			lineRunes := []rune(content[row])
			if col >= 0 && col < len(lineRunes) && lineRunes[col] == typed {
				m.editMoveHorizontal(1, false)
				return true
			}
		}
	}
	closer, isOpener := autoPairClosers[typed]
	if !isOpener || !m.editCanAutoPair(typed) {
		return false
	}
	if m.editHasSelection() {
		start, _, ok := m.editSelectionOffsets()
		if !ok {
			return false
		}
		selected := m.editSelectedText()
		m.editDeleteSelection()
		m.textarea.InsertString(string(typed) + selected + string(closer))
		m.editSetSelectionOffsets(start+1, start+1+len([]rune(selected)))
		m.editClearPreferredColumn()
		m.sourceContent = m.textarea.Value()
		m.updateOutlineFromEditor()
		m.ensureEditCursorVisibleSoft(true)
		return true
	}
	col := m.editCursorCol()
	m.textarea.InsertString(string(typed) + string(closer))
	m.textarea.SetCursorColumn(col + 1)
	m.editClearSelection()
	m.editClearPreferredColumn()
	m.sourceContent = m.textarea.Value()
	m.updateOutlineFromEditor()
	m.ensureEditCursorVisibleSoft(false)
	return true
}

func (m *model) updateOutlineFromEditor() {
	m.headings = parseHeadings(m.sourceContent)
	m.setOutline(m.headings, false)
	line := m.textarea.Line()
	idx := headingIndexForSourceLine(m.headings, line)
	m.setCurrentHeading(idx)
	m.updateBreadcrumb()
	m.editDirty = true
	m.docStats = computeDocStats(m.sourceContent, len(m.headings))
}
