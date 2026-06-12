package main

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

func editWrapLine(runes []rune, width int) [][]rune {
	if width <= 0 {
		width = 1
	}
	var (
		lines  = [][]rune{{}}
		word   = []rune{}
		row    int
		spaces int
	)

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}

		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces))...)
				spaces = 0
				word = nil
			} else {
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces))...)
				spaces = 0
				word = nil
			}
		} else {
			lastCharLen := rw.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastCharLen > width {
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}

	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		lines[row+1] = append(lines[row+1], word...)
		spaces++
		lines[row+1] = append(lines[row+1], []rune(strings.Repeat(" ", spaces))...)
	} else {
		lines[row] = append(lines[row], word...)
		spaces++
		lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces))...)
	}

	return lines
}

func editDisplayPoint(lines []string, width, row, col int) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	if row < 0 {
		row = 0
	} else if row >= len(lines) {
		row = len(lines) - 1
	}

	displayRow := 0
	for i := range row {
		displayRow += len(editWrapLine([]rune(lines[i]), width))
	}

	lineRunes := []rune(lines[row])
	if col < 0 {
		col = 0
	} else if col > len(lineRunes) {
		col = len(lineRunes)
	}
	wrapped := editWrapLine(lineRunes, width)
	counter := 0
	for i, segment := range wrapped {
		segmentLen := len(segment)
		if counter+segmentLen == col && i+1 < len(wrapped) {
			return displayRow + i + 1, 0
		}
		if counter+segmentLen >= col {
			return displayRow + i, col - counter
		}
		counter += segmentLen
	}
	last := len(wrapped) - 1
	return displayRow + last, len(wrapped[last])
}

func (m *model) editSelectionDisplayRange() (int, int, int, int, bool) {
	if !m.editHasSelection() {
		return 0, 0, 0, 0, false
	}
	width := m.textarea.Width()
	if width <= 0 {
		return 0, 0, 0, 0, false
	}
	sourceLines := strings.Split(m.textarea.Value(), "\n")
	if len(sourceLines) == 0 {
		return 0, 0, 0, 0, false
	}
	sR, sC, eR, eC := m.editNormalizedSel()
	sDispRow, sDispCol := editDisplayPoint(sourceLines, width, sR, sC)
	eDispRow, eDispCol := editDisplayPoint(sourceLines, width, eR, eC)
	if sDispRow == eDispRow && sDispCol == eDispCol {
		return 0, 0, 0, 0, false
	}
	return sDispRow, sDispCol, eDispRow, eDispCol, true
}

func (m *model) editSelectionVisibleRange(viewRows int) (int, int, int, int, bool) {
	if viewRows <= 0 {
		return 0, 0, 0, 0, false
	}
	sDispRow, sDispCol, eDispRow, eDispCol, ok := m.editSelectionDisplayRange()
	if !ok {
		return 0, 0, 0, 0, false
	}
	scroll := m.textarea.ScrollYOffset()
	sVisRow := sDispRow - scroll
	eVisRow := eDispRow - scroll
	if eVisRow < 0 || sVisRow >= viewRows {
		return 0, 0, 0, 0, false
	}
	if sVisRow < 0 {
		sVisRow = 0
		sDispCol = 0
	}
	if eVisRow >= viewRows {
		eVisRow = viewRows - 1
		eDispCol = int(^uint(0) >> 1)
	}
	return sVisRow, sDispCol, eVisRow, eDispCol, true
}

func (m *model) editRenderedPrefixWidth() int {
	prefixW := lipgloss.Width(xansi.Strip(m.textarea.Prompt))
	if prefixW <= 0 {
		prefixW = 2
	}
	if m.textarea.ShowLineNumbers {
		digits := len(strconv.Itoa(max(1, m.textarea.MaxHeight)))
		prefixW += digits + 2
	}
	return prefixW
}

func (m *model) applyEditSelectionHighlight(lines []string) []string {
	sR, sC, eR, eC, ok := m.editSelectionVisibleRange(len(lines))
	if !ok {
		return lines
	}
	prefixW := m.editRenderedPrefixWidth()
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color(m.palette.highlight))
	result := make([]string, len(lines))
	copy(result, lines)
	for row := sR; row <= eR && row < len(lines); row++ {
		stripped := []rune(xansi.Strip(lines[row]))
		lo := prefixW
		if row == sR {
			lo = prefixW + sC
		}
		hi := len(stripped)
		if row == eR {
			hi = prefixW + eC
		}
		lo = max(lo, prefixW)
		lo = min(lo, len(stripped))
		hi = max(hi, lo)
		hi = min(hi, len(stripped))
		var b strings.Builder
		b.WriteString(string(stripped[:lo]))
		if lo < hi {
			b.WriteString(selStyle.Render(string(stripped[lo:hi])))
		}
		b.WriteString(string(stripped[hi:]))
		result[row] = b.String()
	}
	return result
}

type editSearchHighlightSegment struct {
	start   int
	end     int
	current bool
}

func (m *model) applyEditSearchHighlights(lines []string) []string {
	if !m.showSearch || m.mode != modeRaw || len(lines) == 0 || len(m.searchMatches) == 0 || m.searchQueryLen <= 0 {
		return lines
	}
	width := m.textarea.Width()
	if width <= 0 {
		return lines
	}
	sourceLines := strings.Split(m.textarea.Value(), "\n")
	if len(sourceLines) == 0 {
		return lines
	}
	scroll := m.textarea.ScrollYOffset()
	prefixW := m.editRenderedPrefixWidth()
	rowSegs := map[int][]editSearchHighlightSegment{}
	for i, hit := range m.searchMatches {
		if hit.line < 0 || hit.line >= len(sourceLines) {
			continue
		}
		startRow, startCol := editDisplayPoint(sourceLines, width, hit.line, hit.start)
		endRow, endCol := editDisplayPoint(sourceLines, width, hit.line, hit.start+m.searchQueryLen)
		startVis := startRow - scroll
		endVis := endRow - scroll
		if endVis < 0 || startVis >= len(lines) {
			continue
		}
		if startVis < 0 {
			startVis = 0
		}
		if endVis >= len(lines) {
			endVis = len(lines) - 1
		}
		for row := startVis; row <= endVis; row++ {
			segStart := 0
			segEnd := int(^uint(0) >> 1)
			if row == startRow-scroll {
				segStart = startCol
			}
			if row == endRow-scroll {
				segEnd = endCol
			}
			rowSegs[row] = append(rowSegs[row], editSearchHighlightSegment{
				start:   segStart,
				end:     segEnd,
				current: i == m.searchIndex,
			})
		}
	}
	if len(rowSegs) == 0 {
		return lines
	}

	primaryStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.warn)).
		Foreground(lipgloss.Color(m.palette.bg)).
		Bold(true)
	secondaryStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.highlight)).
		Foreground(lipgloss.Color(m.palette.text))

	result := make([]string, len(lines))
	copy(result, lines)
	for row, segs := range rowSegs {
		if row < 0 || row >= len(result) || len(segs) == 0 {
			continue
		}
		runes := []rune(xansi.Strip(result[row]))
		lineLen := len(runes)
		if lineLen == 0 {
			continue
		}
		for i := range segs {
			absStart := prefixW + segs[i].start
			absEnd := prefixW + segs[i].end
			if absStart < prefixW {
				absStart = prefixW
			}
			if absStart > lineLen {
				absStart = lineLen
			}
			if absEnd < absStart {
				absEnd = absStart
			}
			if absEnd > lineLen {
				absEnd = lineLen
			}
			segs[i].start = absStart
			segs[i].end = absEnd
		}
		sort.SliceStable(segs, func(i, j int) bool {
			if segs[i].start == segs[j].start {
				if segs[i].current == segs[j].current {
					return segs[i].end < segs[j].end
				}
				return segs[i].current && !segs[j].current
			}
			return segs[i].start < segs[j].start
		})
		var b strings.Builder
		cursor := 0
		for _, seg := range segs {
			if seg.end <= seg.start {
				continue
			}
			if seg.start < cursor {
				seg.start = cursor
			}
			if seg.start > lineLen {
				break
			}
			if seg.end > lineLen {
				seg.end = lineLen
			}
			if seg.start > cursor {
				b.WriteString(string(runes[cursor:seg.start]))
			}
			if seg.end > seg.start {
				style := secondaryStyle
				if seg.current {
					style = primaryStyle
				}
				b.WriteString(style.Render(string(runes[seg.start:seg.end])))
			}
			cursor = seg.end
		}
		if cursor < lineLen {
			b.WriteString(string(runes[cursor:]))
		}
		result[row] = b.String()
	}
	return result
}

func (m *model) applyEditFocusLineHighlight(lines []string) []string {
	if m.editFocusLine < 0 || len(lines) == 0 {
		return lines
	}
	source := strings.Split(m.textarea.Value(), "\n")
	if len(source) == 0 {
		return lines
	}
	line := m.editFocusLine
	if line < 0 || line >= len(source) {
		return lines
	}
	width := m.textarea.Width()
	if width <= 0 {
		width = 1
	}
	displayStart := 0
	for i := range line {
		displayStart += len(editWrapLine([]rune(source[i]), width))
	}
	rowCount := len(editWrapLine([]rune(source[line]), width))
	if rowCount <= 0 {
		rowCount = 1
	}
	displayEnd := displayStart + rowCount - 1
	scroll := m.textarea.ScrollYOffset()
	visStart := displayStart - scroll
	visEnd := displayEnd - scroll
	if visEnd < 0 || visStart >= len(lines) {
		return lines
	}
	if visStart < 0 {
		visStart = 0
	}
	if visEnd >= len(lines) {
		visEnd = len(lines) - 1
	}
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.highlight)).
		Foreground(lipgloss.Color(m.palette.text))
	out := make([]string, len(lines))
	copy(out, lines)
	for i := visStart; i <= visEnd; i++ {
		out[i] = style.Render(xansi.Strip(out[i]))
	}
	return out
}

func (m model) previewAnchorRenderLine() int {
	if m.previewYOffset < 0 {
		return 0
	}
	return m.previewYOffset
}

func (m model) previewAnchorText() string {
	if len(m.viewportLines) == 0 {
		return ""
	}
	y := m.previewAnchorRenderLine()
	if y < 0 {
		return ""
	}
	// Scan forward a few lines from the viewport top to find the first line
	// with enough text for a reliable substring match. The top line is often
	// a blank separator or a continuation of a word-wrapped line.
	limit := min(y+8, len(m.viewportLines))
	for scan := y; scan < limit; scan++ {
		plain := stripANSI(m.viewportLines[scan])
		plain = stripLineNumberPrefix(plain)
		plain = strings.TrimSpace(plain)
		if len(plain) >= 4 {
			return plain
		}
	}
	return ""
}

func bestSourceLineMatch(lines []string, snippet string, approx int) (line int, col int, ok bool) {
	if len(lines) == 0 {
		return 0, 0, false
	}
	snippet = strings.ToLower(strings.TrimSpace(snippet))
	if snippet == "" {
		return 0, 0, false
	}
	bestLine := -1
	bestCol := 0
	bestDist := int(^uint(0) >> 1)
	for i, src := range lines {
		lower := strings.ToLower(src)
		idx := strings.Index(lower, snippet)
		if idx < 0 {
			continue
		}
		dist := abs(i - approx)
		if bestLine < 0 || dist < bestDist {
			bestLine = i
			bestCol = utf8.RuneCountInString(src[:idx])
			bestDist = dist
		}
	}
	if bestLine < 0 {
		return 0, 0, false
	}
	return bestLine, bestCol, true
}

func (m model) targetEditLine() int {
	lines := strings.Split(m.sourceContent, "\n")
	if len(lines) == 0 {
		return 0
	}
	maxLine := len(lines) - 1
	y := m.previewAnchorRenderLine()
	if y < 0 {
		y = 0
	}
	if idx := headingIndexAtOffset(m.headingRenderLines, m.headingRenderIndices, y); idx >= 0 && idx < len(m.headings) {
		cur := m.headings[idx]
		curLine := max(0, min(maxLine, cur.line))
		if cur.renderLine >= 0 {
			if idx+1 < len(m.headings) {
				next := m.headings[idx+1]
				if next.renderLine > cur.renderLine && next.line > cur.line {
					spanRender := next.renderLine - cur.renderLine
					deltaRender := y - cur.renderLine
					if deltaRender < 0 {
						deltaRender = 0
					}
					if deltaRender > spanRender {
						deltaRender = spanRender
					}
					spanSource := next.line - cur.line
					mapped := cur.line + (deltaRender*spanSource)/spanRender
					return max(0, min(maxLine, mapped))
				}
			}
			projected := cur.line + (y - cur.renderLine)
			return max(0, min(maxLine, projected))
		}
		return curLine
	}
	if y > maxLine {
		y = maxLine
	}
	return y
}

func (m model) renderLineForSourceLine(line int) int {
	lines := strings.Split(m.sourceContent, "\n")
	if len(lines) == 0 {
		return 0
	}
	maxLine := len(lines) - 1
	line = max(0, min(maxLine, line))

	prevIdx := -1
	nextIdx := -1
	for i, h := range m.headings {
		if h.renderLine < 0 {
			continue
		}
		if h.line <= line {
			prevIdx = i
			continue
		}
		nextIdx = i
		break
	}

	switch {
	case prevIdx >= 0:
		prev := m.headings[prevIdx]
		if nextIdx >= 0 {
			next := m.headings[nextIdx]
			if next.line > prev.line && next.renderLine > prev.renderLine {
				spanSource := next.line - prev.line
				spanRender := next.renderLine - prev.renderLine
				deltaSource := line - prev.line
				return prev.renderLine + (deltaSource*spanRender)/spanSource
			}
		}
		return max(0, prev.renderLine+(line-prev.line))
	case nextIdx >= 0:
		next := m.headings[nextIdx]
		if next.line > 0 && next.renderLine > 0 {
			return (line * next.renderLine) / next.line
		}
	}

	return line
}

func (m model) targetEditPosition() (line int, col int) {
	line = m.targetEditLine()
	col = 0
	lines := strings.Split(m.sourceContent, "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	if anchor := m.previewAnchorText(); anchor != "" {
		if matchedLine, matchedCol, ok := bestSourceLineMatch(lines, anchor, line); ok {
			return matchedLine, matchedCol
		}
	}
	return line, 0
}
