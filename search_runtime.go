package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func applySearchHighlights(content string, matches []searchHit, current int, queryLen int, primaryPrefix, secondaryPrefix string) string {
	if len(matches) == 0 || queryLen <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	applySearchHighlightsLines(lines, matches, current, queryLen, primaryPrefix, secondaryPrefix)
	return strings.Join(lines, "\n")
}

func applySearchHighlightsLines(lines []string, matches []searchHit, current int, queryLen int, primaryPrefix, secondaryPrefix string) {
	if len(matches) == 0 || queryLen <= 0 || len(lines) == 0 {
		return
	}
	activeLine := -1
	if current >= 0 && current < len(matches) {
		activeLine = matches[current].line
	}
	reset := "\x1b[0m"

	for i := 0; i < len(matches); {
		line := matches[i].line
		j := i + 1
		for j < len(matches) && matches[j].line == line {
			j++
		}
		if line >= 0 && line < len(lines) {
			prefix := secondaryPrefix
			if line == activeLine {
				prefix = primaryPrefix
			}
			lines[line] = highlightLineMatches(lines[line], matches, i, j, queryLen, prefix, reset)
		}
		i = j
	}
}

func highlightLineMatches(line string, matches []searchHit, start, end int, queryLen int, prefix, reset string) string {
	if start < 0 || end <= start || queryLen <= 0 {
		return line
	}
	// ASCII fast-path: check if line has no bytes >= 128 outside ANSI sequences
	isASCII := true
	for i := 0; i < len(line); i++ {
		if line[i] == 0x1b {
			// Skip ANSI sequence
			if i+1 < len(line) && line[i+1] == '[' {
				j := i + 2
				for j < len(line) && ((line[j] >= '0' && line[j] <= '9') || line[j] == ';') {
					j++
				}
				if j < len(line) {
					j++
				}
				i = j - 1 // -1 because loop will i++
				continue
			}
		}
		if line[i] >= 128 {
			isASCII = false
			break
		}
	}

	var b strings.Builder
	b.Grow(len(line) + (end-start)*(len(prefix)+len(reset)+1))
	plainIdx := 0
	spanIdx := start
	currentStart := matches[spanIdx].start
	currentEnd := currentStart + queryLen

	if isASCII {
		// Fast path: byte == character for non-ANSI content
		for i := 0; i < len(line); {
			if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
				j := i + 2
				for j < len(line) && ((line[j] >= '0' && line[j] <= '9') || line[j] == ';') {
					j++
				}
				if j < len(line) {
					j++
				}
				b.WriteString(line[i:j])
				i = j
				continue
			}
			if plainIdx >= currentStart && plainIdx < currentEnd {
				b.WriteString(prefix)
				b.WriteByte(line[i])
				b.WriteString(reset)
			} else {
				b.WriteByte(line[i])
			}
			plainIdx++
			i++
			if plainIdx >= currentEnd && spanIdx < end-1 {
				spanIdx++
				currentStart = matches[spanIdx].start
				currentEnd = currentStart + queryLen
			}
		}
	} else {
		// Unicode fallback: use rune decoding
		for i := 0; i < len(line); {
			if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
				j := i + 2
				for j < len(line) && ((line[j] >= '0' && line[j] <= '9') || line[j] == ';') {
					j++
				}
				if j < len(line) {
					j++
				}
				b.WriteString(line[i:j])
				i = j
				continue
			}
			r, size := utf8.DecodeRuneInString(line[i:])
			if plainIdx >= currentStart && plainIdx < currentEnd {
				b.WriteString(prefix)
				b.WriteRune(r)
				b.WriteString(reset)
			} else {
				b.WriteRune(r)
			}
			plainIdx++
			i += size
			if plainIdx >= currentEnd && spanIdx < end-1 {
				spanIdx++
				currentStart = matches[spanIdx].start
				currentEnd = currentStart + queryLen
			}
		}
	}
	return b.String()
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';') {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func stripLineNumberPrefix(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	startDigits := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == startDigits {
		return s
	}
	if i >= len(s) || (s[i] != ' ' && s[i] != '\t') {
		return s
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

func findHeadingOffset(rendered string, title string, lineNums bool) int {
	titleLower := strings.ToLower(strings.TrimSpace(title))
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		line = stripANSI(line)
		line = stripLineNumberPrefix(line)
		lineLower := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(lineLower, titleLower) {
			return i
		}
	}
	return -1
}

func (m *model) refreshSearchMatches() {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.searchMatches = nil
		m.searchQueryLen = 0
		return
	}
	opts := m.activeSearchOptions()
	if m.mode == modeRaw {
		hits, qlen := findSearchMatchesWithOptions(strings.Split(m.textarea.Value(), "\n"), query, opts)
		m.searchMatches = hits
		m.searchQueryLen = qlen
		if m.searchIndex >= len(m.searchMatches) {
			m.searchIndex = 0
		}
		return
	}
	if m.rendered == "" {
		m.searchMatches = nil
		m.searchQueryLen = 0
		return
	}
	hits, qlen := findSearchMatchesWithOptions(m.renderedLineCache, query, opts)
	m.searchMatches = hits
	m.searchQueryLen = qlen
	if m.searchIndex >= len(m.searchMatches) {
		m.searchIndex = 0
	}
}

func (m model) activeSearchOptions() searchOptions {
	if m.showReplace {
		return searchOptions{
			caseSensitive: m.replaceCaseSensitive,
			wholeWord:     m.replaceWholeWord,
		}
	}
	return searchOptions{}
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 128 {
			return false
		}
	}
	return true
}

func findSearchMatches(lines []string, query string) ([]searchHit, int) {
	return findSearchMatchesWithOptions(lines, query, searchOptions{})
}

func findSearchMatchesWithOptions(lines []string, query string, opts searchOptions) ([]searchHit, int) {
	qRunes := []rune(query)
	qLen := len(qRunes)
	if qLen == 0 {
		return nil, 0
	}
	needle := string(qRunes)
	if !opts.caseSensitive {
		needle = strings.ToLower(needle)
	}
	var matches []searchHit
	for i, line := range lines {
		plain := stripANSI(line)
		plain = stripLineNumberPrefix(plain)
		hay := []rune(plain)
		for _, idx := range runeSearchStarts(hay, needle, opts) {
			matches = append(matches, searchHit{line: i, start: idx})
		}
	}
	return matches, qLen
}

func runeSearchStarts(hay []rune, needle string, opts searchOptions) []int {
	needleRunes := []rune(needle)
	if len(needleRunes) == 0 || len(hay) < len(needleRunes) {
		return nil
	}
	compareHay := string(hay)
	compareNeedle := needle
	if !opts.caseSensitive {
		compareHay = strings.ToLower(compareHay)
		compareNeedle = strings.ToLower(compareNeedle)
	}
	hayRunes := []rune(compareHay)
	needleRunes = []rune(compareNeedle)
	matches := make([]int, 0)
	qlen := len(needleRunes)
	for idx := 0; idx+qlen <= len(hayRunes); idx++ {
		if !equalRunes(hayRunes[idx:idx+qlen], needleRunes) {
			continue
		}
		if opts.wholeWord && !isWholeWordMatch(hay, idx, idx+qlen) {
			continue
		}
		matches = append(matches, idx)
		idx += qlen - 1
	}
	return matches
}

func isWholeWordMatch(hay []rune, start, end int) bool {
	if start < 0 || end > len(hay) || start >= end {
		return false
	}
	firstWord := editIsWordRune(hay[start])
	lastWord := editIsWordRune(hay[end-1])
	if firstWord && start > 0 && editIsWordRune(hay[start-1]) {
		return false
	}
	if lastWord && end < len(hay) && editIsWordRune(hay[end]) {
		return false
	}
	return true
}

func (m *model) editSelectionOffsets() (start, end int, ok bool) {
	if !m.editHasSelection() {
		return 0, 0, false
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	sR, sC, eR, eC := m.editNormalizedSel()
	start = editRowColToRuneOffset(lines, sR, sC)
	end = editRowColToRuneOffset(lines, eR, eC)
	if end < start {
		start, end = end, start
	}
	return start, end, end > start
}

func (m *model) replaceMatchesInRange(start, end int, replacement string, opts searchOptions) int {
	contentRunes := []rune(m.textarea.Value())
	if start < 0 {
		start = 0
	}
	if end > len(contentRunes) {
		end = len(contentRunes)
	}
	if start >= end {
		return 0
	}
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return 0
	}
	segment := contentRunes[start:end]
	matches := runeSearchStarts(segment, query, opts)
	if len(matches) == 0 {
		return 0
	}
	qLen := len([]rune(query))
	replRunes := []rune(replacement)
	updatedSegment := slices.Clone(segment)
	for i := len(matches) - 1; i >= 0; i-- {
		matchStart := matches[i]
		matchEnd := matchStart + qLen
		updatedSegment = slices.Replace(updatedSegment, matchStart, matchEnd, replRunes...)
	}
	updated := slices.Concat(slices.Clone(contentRunes[:start]), updatedSegment, contentRunes[end:])
	m.textarea.SetValue(string(updated))
	cursor := start + len(updatedSegment)
	m.editMoveCursorToOffset(cursor)
	if m.replaceScopeSelection {
		m.editSetSelectionOffsets(start, cursor)
	} else {
		m.editClearSelection()
	}
	m.editClearPreferredColumn()
	m.ensureEditCursorVisibleSoft(m.replaceScopeSelection && m.editHasSelection())
	m.sourceContent = m.textarea.Value()
	m.updateOutlineFromEditor()
	return len(matches)
}

func (m *model) replaceCurrentSearchMatch(replacement string) bool {
	if m.mode != modeRaw {
		return false
	}
	m.refreshSearchMatches()
	if len(m.searchMatches) == 0 || m.searchQueryLen <= 0 {
		return false
	}
	idx := m.searchIndex
	if idx < 0 || idx >= len(m.searchMatches) {
		idx = m.searchInitialMatchIndex(1)
		if idx < 0 {
			return false
		}
	}
	hit := m.searchMatches[idx]
	lines := strings.Split(m.textarea.Value(), "\n")
	start := editRowColToRuneOffset(lines, hit.line, hit.start)
	end := start + m.searchQueryLen
	runes := []rune(m.textarea.Value())
	if start < 0 || end > len(runes) || start >= end {
		return false
	}
	replRunes := []rune(replacement)
	updated := slices.Concat(slices.Clone(runes[:start]), replRunes, runes[end:])
	m.textarea.SetValue(string(updated))
	cursor := start + len(replRunes)
	m.editMoveCursorToOffset(cursor)
	m.editClearSelection()
	m.editClearPreferredColumn()
	m.ensureEditCursorVisibleSoft(false)
	m.sourceContent = m.textarea.Value()
	m.updateOutlineFromEditor()
	m.searchReturnRow = m.textarea.Line()
	m.searchReturnCol = m.editCursorCol()
	m.searchReturnSet = true
	return true
}

func (m *model) replaceNextSearchMatch() tea.Cmd {
	if m.mode != modeRaw || !m.showReplace {
		return nil
	}
	m.editBeginUndoCoalesced(editUndoCoalesceTransform)
	if !m.replaceCurrentSearchMatch(m.replaceInput.Value()) {
		m.status = "No matches"
		m.editResetUndoCoalescing()
		return nil
	}
	m.refreshSearchMatches()
	next := m.searchInitialMatchIndex(1)
	if next >= 0 {
		cmd := m.jumpToSearchMatch(next)
		m.status = "Replaced 1 match"
		m.editResetUndoCoalescing()
		return cmd
	}
	m.searchIndex = -1
	m.status = "Replaced last match"
	m.editResetUndoCoalescing()
	return nil
}

func (m *model) replaceAllSearchMatches() int {
	if m.mode != modeRaw || !m.showReplace {
		return 0
	}
	m.editBeginUndoCoalesced(editUndoCoalesceTransform)
	opts := m.activeSearchOptions()
	total := 0
	if m.replaceScopeSelection {
		start, end, ok := m.editSelectionOffsets()
		if ok {
			total = m.replaceMatchesInRange(start, end, m.replaceInput.Value(), opts)
		}
	} else {
		total = m.replaceMatchesInRange(0, len([]rune(m.textarea.Value())), m.replaceInput.Value(), opts)
	}
	m.refreshSearchMatches()
	if total > 0 {
		m.status = fmt.Sprintf("Replaced %d matches", total)
	} else {
		m.status = "No matches"
	}
	m.editResetUndoCoalescing()
	return total
}

func (m *model) jumpToSearchMatch(idx int) tea.Cmd {
	if len(m.searchMatches) == 0 {
		return nil
	}
	var wrapped bool
	if idx < 0 {
		idx = len(m.searchMatches) - 1
		wrapped = true
	}
	if idx >= len(m.searchMatches) {
		idx = 0
		wrapped = true
	}
	m.searchIndex = idx
	hit := m.searchMatches[idx]
	var cmd tea.Cmd
	if m.mode == modeRaw {
		m.editClearSelection()
		m.editSetCursor(hit.line, hit.start)
		m.editClearPreferredColumn()
		m.ensureEditCursorVisibleSoft(false)
		cmd = m.flashEditFocusLine(hit.line)
	} else {
		line := hit.line
		m.highlightLine = line
		cmd = m.scrollTo(line)
	}
	total := len(m.searchMatches)
	if total == 1 {
		m.status = "1 of 1 match"
	} else {
		m.status = fmt.Sprintf("%d of %d matches", idx+1, total)
	}
	if wrapped && total > 1 {
		return tea.Batch(cmd, m.showToast("Search wrapped"))
	}
	return cmd
}

func (m *model) searchInitialMatchIndex(direction int) int {
	if len(m.searchMatches) == 0 {
		return -1
	}
	if m.mode != modeRaw {
		if direction < 0 {
			return len(m.searchMatches) - 1
		}
		return 0
	}
	line := m.textarea.Line()
	col := m.editCursorCol()
	if m.showSearch && m.searchReturnSet {
		line = m.searchReturnRow
		col = m.searchReturnCol
	}
	if direction < 0 {
		for i := len(m.searchMatches) - 1; i >= 0; i-- {
			hit := m.searchMatches[i]
			if hit.line < line || (hit.line == line && hit.start <= col) {
				return i
			}
		}
		return len(m.searchMatches) - 1
	}
	for i, hit := range m.searchMatches {
		if hit.line > line || (hit.line == line && hit.start >= col) {
			return i
		}
	}
	return 0
}

func (m *model) nextSearchMatch(delta int) {
	if len(m.searchMatches) == 0 {
		return
	}
	m.jumpToSearchMatch(m.searchIndex + delta)
}
