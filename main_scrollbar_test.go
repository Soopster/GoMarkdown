package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderWithScrollbarRowsClampsOverflow(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)

	rows := []string{strings.Repeat("─", 40)}
	rowWidths := []int{40}
	outRows, outWidths := renderWithScrollbarRows(rows, rowWidths, 12, 1, 40, 0, styles, 0, false, nil)

	if len(outRows) != 1 || len(outWidths) != 1 {
		t.Fatalf("unexpected output sizes: rows=%d widths=%d", len(outRows), len(outWidths))
	}
	if outWidths[0] != 13 {
		t.Fatalf("expected output width 13, got %d", outWidths[0])
	}
	if got := lipgloss.Width(outRows[0]); got != 13 {
		t.Fatalf("expected rendered row width 13, got %d", got)
	}
}

func TestRenderWithScrollbarRowsIgnoresStaleCachedWidth(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)

	rows := []string{"short"}
	rowWidths := []int{999}
	outRows, _ := renderWithScrollbarRows(rows, rowWidths, 12, 1, 20, 0, styles, 0, false, nil)
	if len(outRows) != 1 {
		t.Fatalf("unexpected output row count: %d", len(outRows))
	}
	if !strings.Contains(outRows[0], "short") {
		t.Fatalf("expected original row content to be preserved, got %q", outRows[0])
	}
}

func TestRenderWithScrollbarRowsIgnoresUnderreportedCachedWidth(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)

	rows := []string{strings.Repeat("─", 40)}
	rowWidths := []int{1}
	outRows, outWidths := renderWithScrollbarRows(rows, rowWidths, 12, 1, 40, 0, styles, 0, false, nil)
	if len(outRows) != 1 || len(outWidths) != 1 {
		t.Fatalf("unexpected output sizes: rows=%d widths=%d", len(outRows), len(outWidths))
	}
	if got := lipgloss.Width(outRows[0]); got != 13 {
		t.Fatalf("expected rendered row width 13, got %d", got)
	}
}

func TestRenderWithScrollbarRowsRestoresBackgroundAfterReset(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)
	bg := hexToBgSGR(palette.bg)

	outRows, _ := renderWithScrollbarRows([]string{"\x1b[31mred\x1b[0m tail"}, nil, 12, 1, 20, 0, styles, 0, false, nil)
	if len(outRows) != 1 {
		t.Fatalf("unexpected output row count: %d", len(outRows))
	}
	if strings.Contains(outRows[0], "\x1b[0m tail") {
		t.Fatalf("expected app background after reset, got %q", outRows[0])
	}
	if !strings.Contains(outRows[0], "\x1b[0m"+bg+" tail") {
		t.Fatalf("expected reset to restore app background, got %q", outRows[0])
	}
}

func TestRenderWithScrollbarRowsInsetsThumbAtBottomForBorderedPanes(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)

	rows := []string{"a", "b", "c", "d", "e"}
	outRows, _ := renderWithScrollbarRows(rows, nil, 4, len(rows), 25, 1, styles, 0, true, nil)
	thumb := xansi.Strip(styles.scrollThumbGlyph)
	track := xansi.Strip(styles.scrollTrackGlyph)

	if !strings.HasSuffix(xansi.Strip(outRows[len(outRows)-2]), thumb) {
		t.Fatalf("expected thumb on penultimate row, got %q", xansi.Strip(outRows[len(outRows)-2]))
	}
	if !strings.HasSuffix(xansi.Strip(outRows[len(outRows)-1]), track) {
		t.Fatalf("expected bottom row to remain track, got %q", xansi.Strip(outRows[len(outRows)-1]))
	}
}

func TestCalculateScrollbarGeometryUsesProportionalThumb(t *testing.T) {
	top := calculateScrollbarGeometry(12, 40, 10, 0, true)
	if !top.scrollable {
		t.Fatal("expected scrollbar to be active")
	}
	if got := top.thumbEnd - top.thumbStart; got != 3 {
		t.Fatalf("expected three-row thumb, got %d", got)
	}
	if top.thumbStart != 1 {
		t.Fatalf("expected top thumb to start at inset row 1, got %d", top.thumbStart)
	}

	bottom := calculateScrollbarGeometry(12, 40, 10, 1, true)
	if bottom.thumbEnd != 11 {
		t.Fatalf("expected bottom thumb to end before inset row 11, got %d", bottom.thumbEnd)
	}

	nearlyFull := calculateScrollbarGeometry(5, 6, 5, 0, true)
	if got := nearlyFull.thumbEnd - nearlyFull.thumbStart; got != 2 {
		t.Fatalf("expected nearly-full document to retain one row of thumb travel, got thumb length %d", got)
	}
}

func TestScrollbarTargetOffsetPreservesGrabPoint(t *testing.T) {
	geometry := calculateScrollbarGeometry(12, 40, 10, 0, true)
	for row := geometry.thumbStart; row < geometry.thumbEnd; row++ {
		grab := scrollbarGrabOffset(row, geometry)
		if got := scrollbarTargetOffset(row, grab, 12, 40, 10, 30, true); got != 0 {
			t.Fatalf("expected clicking top thumb row %d to preserve offset 0, got %d", row, got)
		}
	}

	grab := scrollbarGrabOffset(geometry.thumbStart, geometry)
	if got := scrollbarTargetOffset(11, grab, 12, 40, 10, 30, true); got != 30 {
		t.Fatalf("expected dragging to bottom to reach offset 30, got %d", got)
	}
}

func TestRenderWithScrollbarRowsLeavesEmptyRailWhenContentFits(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)
	rows := []string{"a", "b", "c"}

	outRows, _ := renderWithScrollbarRows(rows, nil, 4, len(rows), len(rows), 1, styles, 0, false, nil)
	for i, row := range outRows {
		plain := []rune(xansi.Strip(row))
		if len(plain) != 5 || plain[len(plain)-1] != ' ' {
			t.Fatalf("expected empty rail on row %d, got %q", i, string(plain))
		}
	}
}

func TestRenderWithScrollbarRowsShowsBoundaryFlashOverThumb(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)
	rows := []string{"a", "b", "c", "d", "e"}

	topRows, _ := renderWithScrollbarRows(rows, nil, 4, len(rows), 20, 0, styles, 1, false, nil)
	if !strings.HasSuffix(xansi.Strip(topRows[0]), xansi.Strip(styles.scrollFlashTop)) {
		t.Fatalf("expected top boundary flash, got %q", xansi.Strip(topRows[0]))
	}

	bottomRows, _ := renderWithScrollbarRows(rows, nil, 4, len(rows), 20, 1, styles, -1, false, nil)
	if !strings.HasSuffix(xansi.Strip(bottomRows[len(bottomRows)-1]), xansi.Strip(styles.scrollFlashBottom)) {
		t.Fatalf("expected bottom boundary flash, got %q", xansi.Strip(bottomRows[len(bottomRows)-1]))
	}
}

func TestRenderPreviewPaneBoxClampsOverflowRows(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)

	box := renderPreviewPaneBox([]string{strings.Repeat("x", 30)}, []int{30}, 12, 1, true, false, styles, nil, "")
	lines := strings.Split(box, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines in boxed output, got %d", len(lines))
	}
	if got := lipgloss.Width(lines[1]); got != 14 {
		t.Fatalf("expected middle row width 14 (inner 12 + borders), got %d", got)
	}
}

func TestRenderPreviewPaneBoxRestoresBackgroundAfterReset(t *testing.T) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)
	bg := hexToBgSGR(palette.bg)

	box := renderPreviewPaneBox([]string{"\x1b[31mred\x1b[0m tail"}, nil, 12, 1, true, false, styles, nil, "")
	if strings.Contains(box, "\x1b[0m tail") {
		t.Fatalf("expected app background after reset, got %q", box)
	}
	if !strings.Contains(box, "\x1b[0m"+bg+" tail") {
		t.Fatalf("expected reset to restore app background, got %q", box)
	}
}

func TestRenderLayoutPaneFocusBordersUpdateWithCache(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.fullScreen = false
	m.currentPath = ""
	m.focusRight = true
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()
	m.setViewportContent("alpha\nbeta\ngamma")

	firstLine := paneBorderTopLine(t, m.renderLayout())
	assertPaneTopLeftBorders(t, firstLine, '╭', '┏')

	m.focusRight = false
	secondLine := paneBorderTopLine(t, m.renderLayout())
	assertPaneTopLeftBorders(t, secondLine, '┏', '╭')
}

func TestRenderLayoutFileExplorerShowsFilesHeading(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.fullScreen = false
	m.currentPath = ""
	m.perfVisualMode = perfVisualForceOff
	m.showOutline = false
	m.resizeViews()
	m.setViewportContent("alpha")

	layout := xansi.Strip(m.renderLayout())
	if !strings.Contains(layout, "Files") {
		t.Fatalf("expected file explorer heading in layout, got %q", layout)
	}

	m.showOutline = true
	m.resizeViews()
	outlineLayout := xansi.Strip(m.renderLayout())
	if strings.Contains(outlineLayout, "Files") {
		t.Fatalf("did not expect file explorer heading in outline layout, got %q", outlineLayout)
	}
}

func TestRenderLayoutPreviewBorderShowsCurrentFile(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.fullScreen = false
	m.perfVisualMode = perfVisualForceOff
	m.currentPath = "/tmp/note.md"
	m.resizeViews()
	m.setViewportContent("alpha")

	topLine := paneBorderTopLine(t, m.renderLayout())
	if !strings.Contains(topLine, " note.md ") {
		t.Fatalf("expected preview border to include current file label, got %q", topLine)
	}
}

func TestResizeViewsFileExplorerKeepsFullInnerHeight(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.fullScreen = false
	m.showOutline = false
	m.resizeViews()

	want := max(1, m.availableContentHeight()-2)
	if got := m.fileList.Height(); got != want {
		t.Fatalf("expected file explorer list height %d, got %d", want, got)
	}
}

func TestResizeViewsSplitPreviewUsesFullRightPaneWidthWithoutGaugeGap(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 120
	m.height = 28
	m.mode = modeSplit
	m.fullScreen = false
	m.showGauge = false
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()

	rightContentWidth := max(10, m.rightWidth()-2)
	if got := m.splitEditWidth + 1 + m.splitPreviewWidth; got != rightContentWidth {
		t.Fatalf("expected split widths + divider to fill right pane content width %d, got %d", rightContentWidth, got)
	}
	if got := m.viewport.Width(); got != m.splitPreviewWidth-1 {
		t.Fatalf("expected preview viewport width %d, got %d", m.splitPreviewWidth-1, got)
	}
}

func TestRenderSectionGaugeRowsPreservesActiveMarkerOnCollapsedRows(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.showGauge = true
	m.perfVisualMode = perfVisualForceOn
	m.viewport.SetHeight(2)
	m.renderedLineCount = 100
	m.headings = []headingItem{
		{title: "A", renderLine: 0},
		{title: "B", renderLine: 1},
		{title: "C", renderLine: 2},
	}
	m.currentHeading = 0

	rows := m.renderSectionGaugeRows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 gauge rows, got %d", len(rows))
	}
	if got := rows[0]; !strings.HasSuffix(got, m.styles.gaugeMark) {
		t.Fatalf("expected active gauge marker suffix at collapsed row, got %q", got)
	}
}

func TestSectionGaugeWidthAdaptsToPaneWidth(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.showGauge = true
	m.perfVisualMode = perfVisualForceOn
	m.headings = []headingItem{{title: "H1", renderLine: 0, level: 1}}

	m.width = 70
	m.height = 24
	m.resizeViews()
	if got := m.sectionGaugeWidth(); got != 2 {
		t.Fatalf("expected compact gauge width 2 for narrow pane, got %d", got)
	}

	m.width = 140
	m.resizeViews()
	if got := m.sectionGaugeWidth(); got != 3 {
		t.Fatalf("expected expanded gauge width 3 for wide pane, got %d", got)
	}
}

func TestPreviewGaugeMouseClickJumpsToHeading(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 120
	m.height = 26
	m.fullScreen = false
	m.showGauge = true
	m.perfVisualMode = perfVisualForceOn
	m.focusRight = false
	m.resizeViews()

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	m.setViewportContent(strings.Join(lines, "\n"))
	m.headings = []headingItem{
		{title: "One", line: 10, renderLine: 10, level: 1},
		{title: "Two", line: 50, renderLine: 50, level: 2},
		{title: "Three", line: 90, renderLine: 90, level: 3},
	}
	m.headingRenderLines = []int{10, 50, 90}
	m.headingRenderIndices = []int{0, 1, 2}
	m.currentHeading = 0
	m.previewYOffset = m.viewport.YOffset()

	targetRow := rowForRenderLine(50, m.renderedLineCount, m.viewport.Height())
	paneX, paneY, _, _, ok := m.previewPaneRect()
	if !ok {
		t.Fatal("expected preview pane rect")
	}
	contentX := paneX + 1
	gaugeX := contentX + m.viewport.Width() + 2
	clickY := paneY + 1 + targetRow

	updatedAny, _ := m.Update(tea.MouseClickMsg{X: gaugeX, Y: clickY, Button: tea.MouseLeft})
	updated := updatedAny.(model)
	if !updated.focusRight {
		t.Fatal("expected gauge click to focus right pane")
	}
	if updated.currentHeading != 1 {
		t.Fatalf("expected gauge click to jump to second heading, got %d", updated.currentHeading)
	}
	if updated.previewYOffset == 0 {
		t.Fatal("expected gauge click to move preview offset")
	}
}

func TestPreviewScrollbarMouseClickAndDrag(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 120
	m.height = 26
	m.fullScreen = true
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()

	lines := make([]string, 300)
	for i := range lines {
		lines[i] = "line"
	}
	m.setViewportContent(strings.Join(lines, "\n"))
	m.renderedLineCount = len(lines)
	m.previewYOffset = 0
	m.viewport.SetYOffset(0)

	paneX, paneY, _, _, ok := m.previewPaneRect()
	if !ok {
		t.Fatal("expected preview pane rect")
	}
	scrollbarX := paneX + m.viewport.Width()
	topY := paneY
	bottomY := topY + m.viewport.Height() - 1

	clickedAny, _ := m.Update(tea.MouseClickMsg{X: scrollbarX, Y: topY, Button: tea.MouseLeft})
	clicked := clickedAny.(model)
	if clicked.scrollbarDrag != scrollbarDragPreview {
		t.Fatalf("expected preview scrollbar drag to start, got %v", clicked.scrollbarDrag)
	}
	if clicked.previewYOffset != 0 {
		t.Fatalf("expected top scrollbar click to keep top offset, got %d", clicked.previewYOffset)
	}

	draggedAny, _ := clicked.Update(tea.MouseMotionMsg{X: scrollbarX, Y: bottomY, Button: tea.MouseLeft})
	dragged := draggedAny.(model)
	maxOffset := max(0, dragged.renderedLineCount-dragged.viewport.Height())
	if dragged.previewYOffset != maxOffset {
		t.Fatalf("expected preview scrollbar drag to reach bottom offset %d, got %d", maxOffset, dragged.previewYOffset)
	}

	releasedAny, _ := dragged.Update(tea.MouseReleaseMsg{X: scrollbarX, Y: bottomY, Button: tea.MouseLeft})
	released := releasedAny.(model)
	if released.scrollbarDrag != scrollbarDragNone {
		t.Fatalf("expected preview scrollbar drag to end on release, got %v", released.scrollbarDrag)
	}
}

func TestPreviewScrollbarDoesNotCaptureMouseWhenContentFits(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 120
	m.height = 26
	m.fullScreen = true
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()
	m.setViewportContent("one\ntwo\nthree")

	paneX, paneY, _, _, ok := m.previewPaneRect()
	if !ok {
		t.Fatal("expected preview pane rect")
	}
	scrollbarX := paneX + m.viewport.Width()
	updatedAny, _ := m.Update(tea.MouseClickMsg{X: scrollbarX, Y: paneY, Button: tea.MouseLeft})
	updated := updatedAny.(model)
	if updated.scrollbarDrag != scrollbarDragNone {
		t.Fatalf("expected no preview scrollbar drag, got %v", updated.scrollbarDrag)
	}
}

func TestRenderLayoutFullScreenPreviewOmitsPaneBorders(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 80
	m.height = 20
	m.fullScreen = true
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()
	m.setViewportContent("alpha\nbeta\ngamma")

	layout := xansi.Strip(m.renderLayout())
	for _, line := range strings.Split(layout, "\n") {
		if strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "┏") || strings.HasSuffix(line, "╮") || strings.HasSuffix(line, "┓") {
			t.Fatalf("expected fullscreen preview to omit pane borders, got %q", line)
		}
	}
}

func TestEditScrollbarPercentReachesBottomAtDocumentEnd(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 100
	m.height = 22
	m.currentPath = "note.md"
	m.resizeViews()

	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))
	_ = m.textarea.View()
	m.textarea.MoveToBegin()
	m.editSetCursor(len(lines)-1, 0)
	_ = m.textarea.View()

	if got := m.editScrollbarPercent(); got != 1 {
		t.Fatalf("expected edit scrollbar percent to be 1 at document end, got %.4f", got)
	}
	if got := m.textarea.ScrollPercent(); got >= 1 {
		t.Fatalf("expected textarea scroll percent to remain below 1 due to buffer padding, got %.4f", got)
	}
}

func TestRawScrollbarMouseClickAndDrag(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 100
	m.height = 22
	m.currentPath = "note.md"
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()

	lines := make([]string, 240)
	for i := range lines {
		lines[i] = "line"
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))
	_ = m.textarea.View()
	m.textarea.MoveToBegin()

	contentX, contentY, _, contentH, ok := m.editRawContentRect()
	if !ok {
		t.Fatal("expected raw editor content rect")
	}
	scrollbarX := contentX + m.textarea.Width()
	topY := contentY
	bottomY := contentY + contentH - 1

	clickedAny, _ := m.Update(tea.MouseClickMsg{X: scrollbarX, Y: topY, Button: tea.MouseLeft})
	clicked := clickedAny.(model)
	if clicked.scrollbarDrag != scrollbarDragRaw {
		t.Fatalf("expected raw scrollbar drag to start, got %v", clicked.scrollbarDrag)
	}
	if clicked.textarea.ScrollYOffset() != 0 {
		t.Fatalf("expected top scrollbar click to keep raw editor at top, got %d", clicked.textarea.ScrollYOffset())
	}

	draggedAny, _ := clicked.Update(tea.MouseMotionMsg{X: scrollbarX, Y: bottomY, Button: tea.MouseLeft})
	dragged := draggedAny.(model)
	_, _, maxOffset := dragged.editScrollMetrics()
	if dragged.textarea.ScrollYOffset() != maxOffset {
		t.Fatalf("expected raw scrollbar drag to reach bottom offset %d, got %d", maxOffset, dragged.textarea.ScrollYOffset())
	}

	releasedAny, _ := dragged.Update(tea.MouseReleaseMsg{X: scrollbarX, Y: bottomY, Button: tea.MouseLeft})
	released := releasedAny.(model)
	if released.scrollbarDrag != scrollbarDragNone {
		t.Fatalf("expected raw scrollbar drag to end on release, got %v", released.scrollbarDrag)
	}
}

func TestRawScrollbarDoesNotCaptureMouseWhenContentFits(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 100
	m.height = 22
	m.currentPath = "note.md"
	m.perfVisualMode = perfVisualForceOff
	m.resizeViews()
	m.textarea.SetValue("one\ntwo\nthree")
	_ = m.textarea.View()

	contentX, contentY, _, _, ok := m.editRawContentRect()
	if !ok {
		t.Fatal("expected raw editor content rect")
	}
	scrollbarX := contentX + m.textarea.Width()
	updatedAny, _ := m.Update(tea.MouseClickMsg{X: scrollbarX, Y: contentY, Button: tea.MouseLeft})
	updated := updatedAny.(model)
	if updated.scrollbarDrag != scrollbarDragNone {
		t.Fatalf("expected no raw scrollbar drag, got %v", updated.scrollbarDrag)
	}
}

func paneBorderTopLine(t *testing.T, layout string) string {
	t.Helper()
	for _, line := range strings.Split(xansi.Strip(layout), "\n") {
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) == 0 {
			continue
		}
		if r[0] == '╭' || r[0] == '┏' {
			return line
		}
	}
	t.Fatal("missing pane border top line in layout output")
	return ""
}

func assertPaneTopLeftBorders(t *testing.T, line string, wantLeft, wantRight rune) {
	t.Helper()
	r := []rune(line)
	if len(r) == 0 {
		t.Fatalf("empty line for pane border check")
	}
	if got := r[0]; got != wantLeft {
		t.Fatalf("unexpected left pane border rune: got %q want %q line=%q", got, wantLeft, line)
	}
	dividerCol := -1
	for i, ch := range r {
		if ch == '│' {
			dividerCol = i
			break
		}
	}
	if dividerCol < 0 || dividerCol+1 >= len(r) {
		t.Fatalf("missing pane divider in border line: %q", line)
	}
	rightPaneCol := dividerCol + 1
	if got := r[rightPaneCol]; got != wantRight {
		t.Fatalf("unexpected right pane border rune: got %q want %q line=%q", got, wantRight, line)
	}
}
