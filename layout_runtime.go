package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"
)

const editUndoMaxEntries = 100

func (m *model) resizeViews() {
	availHeight := m.availableContentHeight()
	innerHeight := max(1, availHeight-2) // account for borders
	if m.fullScreen {
		innerHeight = max(1, availHeight)
	}

	listWidthTotal := m.listWidth()
	listContentWidth := max(1, listWidthTotal-2)
	m.fileList.SetSize(listContentWidth, innerHeight)
	m.outline.SetSize(listContentWidth, innerHeight)

	rightWidthTotal := m.rightWidth()
	if rightWidthTotal < 10 {
		rightWidthTotal = 10
	}
	rightContentWidth := max(10, rightWidthTotal-2)
	if m.fullScreen {
		rightContentWidth = max(10, rightWidthTotal)
	}
	gaugeWidth := 0
	if m.showSectionGauge() {
		gaugeWidth = m.sectionGaugeWidth() + 1 // separator
	}
	if m.mode == modeSplit {
		// Split mode: divide right pane between editor and preview
		splitAvailable := max(20, rightContentWidth-1) // reserve one column for the middle divider
		splitEditW := max(10, splitAvailable/2)
		splitPreviewW := max(10, splitAvailable-splitEditW)
		m.textarea.SetWidth(max(1, splitEditW-1))
		m.textarea.SetHeight(innerHeight)
		m.viewport.SetWidth(max(10, splitPreviewW-1))
		m.viewport.SetHeight(innerHeight)
		m.splitEditWidth = splitEditW
		m.splitPreviewWidth = splitPreviewW
	} else {
		m.splitEditWidth = 0
		m.splitPreviewWidth = 0
		m.viewport.SetWidth(max(10, rightContentWidth-1-gaugeWidth)) // leave space for scrollbar and gauge
		m.viewport.SetHeight(innerHeight)
		taWidth := rightContentWidth - 1
		if !m.editSoftWrap && (m.mode == modeRaw || m.mode == modeSplit) {
			taWidth = 10000 // wide for horizontal scrolling
		}
		m.textarea.SetWidth(max(1, taWidth)) // reserve 1 col for scrollbar
		m.textarea.SetHeight(innerHeight)
	}
}

func (m model) refreshFilesCmd() tea.Cmd {
	return m.refreshFilesCmdWithCurrentReload(false)
}

func (m model) refreshFilesAndCurrentFileCmd() tea.Cmd {
	return m.refreshFilesCmdWithCurrentReload(true)
}

func (m model) refreshFilesCmdWithCurrentReload(reloadCurrent bool) tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		root, err := buildNavigatorTree(dir)
		return filesRefreshedMsg{root: root, err: err, reloadCurrent: reloadCurrent}
	}
}

func (m model) loadFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		return fileLoadedMsg{path: path, content: string(content), err: err}
	}
}

func (m model) saveFileCmd(path, content string) tea.Cmd {
	return func() tea.Msg {
		err := os.WriteFile(path, []byte(content), 0644)
		return fileSavedMsg{path: path, err: err}
	}
}

func (m *model) getOrCreateRenderer(width int, styleName string, codePlain bool, rich bool) (*glamour.TermRenderer, error) {
	// Check if cached renderer is still valid
	if m.cachedRenderer != nil &&
		m.rendererWidth == width &&
		m.rendererStyle == styleName &&
		m.rendererCodePlain == codePlain &&
		m.rendererRich == rich {
		return m.cachedRenderer, nil
	}

	// Need to create a new renderer
	styleCfg, useDefault := selectStyle(styleName, rich, m.palette)
	opts := []glamour.TermRendererOption{
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	}
	if useDefault {
		opts = append(opts, glamour.WithAutoStyle())
	} else {
		if codePlain && rich {
			styleCfg.CodeBlock.Chroma = nil
		}
		opts = append(opts, glamour.WithStyles(*styleCfg))
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return nil, err
	}

	// Cache the renderer
	m.cachedRenderer = renderer
	m.rendererWidth = width
	m.rendererStyle = styleName
	m.rendererCodePlain = codePlain
	m.rendererRich = rich

	return renderer, nil
}

func (m *model) renderCurrentContent() tea.Cmd {
	if m.mode != modePreview && m.mode != modeSplit {
		return nil
	}
	m.renderGeneration++
	requestGen := m.renderGeneration
	styleName := m.styleName
	codePlain := m.codePlain
	richPreview := m.effectiveRichPreview()
	perfEnabled := m.perf != nil && m.perf.enabled

	// Get or create cached renderer
	baseWidth := m.effectiveReadingWidth()
	effectiveWidth := int(float64(baseWidth) * m.zoom)
	if effectiveWidth < 10 {
		effectiveWidth = 10
	}
	glamourWidth := effectiveWidth
	renderer, err := m.getOrCreateRenderer(glamourWidth, styleName, codePlain, richPreview)
	if err != nil {
		return func() tea.Msg {
			renderMs := 0.0
			if perfEnabled {
				started := time.Now()
				renderMs = float64(time.Since(started)) / float64(time.Millisecond)
			}
			return renderedMsg{
				err:        err,
				renderMs:   renderMs,
				generation: requestGen,
			}
		}
	}

	content := m.sourceContent
	// Strip frontmatter before rendering (Glamour renders it as broken table)
	content = stripFrontmatter(content, m.frontmatterLines)
	// Apply section folding
	content = m.applyFolding(content)
	content, markdownFeatures := extractMarkdownFeatures(content)
	content, mermaidBlocks := extractMermaidBlocks(content)
	content, previewCodeBlocks := extractPreviewCodeBlocks(content)
	renderCacheKey := buildRenderCacheKey(content, markdownFeatures, mermaidBlocks, previewCodeBlocks)
	// Check annotateCodeBlocks cache
	var annotated string
	if content == m.lastAnnotateContent && glamourWidth == m.lastAnnotateWidth {
		annotated = m.lastAnnotateOutput
	} else {
		annotated = content
		m.lastAnnotateContent = content
		m.lastAnnotateWidth = glamourWidth
		m.lastAnnotateOutput = annotated
	}

	// Check render output cache — skip Glamour if inputs unchanged
	lineNums := m.showLineNums
	readingMode := m.readingMode
	if renderCacheKey == m.lastRenderCacheKey &&
		glamourWidth == m.lastRenderWidth &&
		lineNums == m.lastRenderLineNums &&
		readingMode == m.lastRenderReading &&
		richPreview == m.lastRenderRich &&
		codePlain == m.lastRenderCode &&
		styleName == m.lastRenderStyle &&
		m.lastRenderOutput != "" {
		yOffset := m.previewYOffset
		cachedOutput := m.lastRenderOutput
		return func() tea.Msg {
			renderMs := 0.0
			if perfEnabled {
				started := time.Now()
				renderMs = float64(time.Since(started)) / float64(time.Millisecond)
			}
			return renderedMsg{
				content:     cachedOutput,
				yOffset:     yOffset,
				width:       glamourWidth,
				renderMs:    renderMs,
				generation:  requestGen,
				annotated:   annotated,
				cacheKey:    renderCacheKey,
				lineNums:    lineNums,
				readingMode: readingMode,
				richPreview: richPreview,
				codePlain:   codePlain,
				styleName:   styleName,
			}
		}
	}

	yOffset := m.previewYOffset
	vpWidth := m.viewport.Width()
	palette := m.palette
	lineNumPrefix := m.styles.lineNumFmt
	lineNumBg := m.styles.lineNumBgWrap
	uiStyles := m.styles
	return func() tea.Msg {
		var started time.Time
		if perfEnabled {
			started = time.Now()
		}
		out, err := renderer.Render(annotated)
		if err == nil {
			blockReplacements, inlineReplacements, footnotes := renderMarkdownFeatures(renderer, markdownFeatures, glamourWidth, palette, m.currentPath)
			if len(blockReplacements) > 0 {
				out = replaceMarkdownBlockPlaceholders(out, blockReplacements)
			}
			if replacements := m.renderMermaidReplacements(mermaidBlocks, glamourWidth); len(replacements) > 0 {
				out = replaceMermaidPlaceholders(out, replacements)
			}
			if replacements := renderPreviewCodeBlocks(renderer, previewCodeBlocks, glamourWidth, uiStyles, m.fullScreen, codePlain); len(replacements) > 0 {
				out = replaceMarkdownBlockPlaceholders(out, replacements)
			}
			if len(inlineReplacements) > 0 {
				out = replaceMarkdownInlinePlaceholders(out, inlineReplacements)
			}
			out = appendMarkdownFootnotes(out, footnotes)
			if lineNums {
				out = addLineNumbersSGR(out, lineNumPrefix, lineNumBg, palette)
			}
		}
		if readingMode {
			out = addLineSpacing(out)
			out = centerContent(out, vpWidth, effectiveWidth)
		}
		renderMs := 0.0
		if !started.IsZero() {
			renderMs = float64(time.Since(started)) / float64(time.Millisecond)
		}
		return renderedMsg{
			content:     out,
			err:         err,
			yOffset:     yOffset,
			width:       glamourWidth,
			renderMs:    renderMs,
			generation:  requestGen,
			annotated:   annotated,
			cacheKey:    renderCacheKey,
			lineNums:    lineNums,
			readingMode: readingMode,
			richPreview: richPreview,
			codePlain:   codePlain,
			styleName:   styleName,
		}
	}
}

func watchFilesCmd(w *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			return fsEventMsg{event: ev}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			return fsWatchErrMsg{err: err}
		}
	}
}

func autoReloadClearCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return autoReloadClearMsg{}
	})
}

func externalReloadCmd(generation int, path string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return externalReloadMsg{generation: generation, path: path}
	})
}

func toastClearCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return toastClearMsg{}
	})
}

func (m *model) showToast(msg string) tea.Cmd {
	if msg == "" {
		m.toast = ""
		m.toastUntil = time.Time{}
		return nil
	}
	m.toast = msg
	duration := 3 * time.Second
	m.toastUntil = time.Now().Add(duration)
	return toastClearCmd(duration)
}

func (m *model) ensureWatcherForPath(path string) {
	if m.watcher == nil || path == "" {
		return
	}
	dir := filepath.Dir(path)
	if dir == "" {
		return
	}
	for _, watched := range m.watcher.WatchList() {
		if sameFile(watched, dir) {
			return
		}
	}
	_ = m.watcher.Add(dir)
}

func (m model) listWidth() int {
	if m.fullScreen {
		return 0
	}
	if m.width == 0 {
		return 30
	}
	minLeft := 16
	minRight := 20
	if m.listWidthRatio <= 0 {
		m.listWidthRatio = 0.33
	}
	left := int(float64(m.width) * m.listWidthRatio)
	if left < minLeft {
		left = minLeft
	}
	if left > m.width-minRight {
		left = m.width - minRight
	}
	return left
}

func (m model) rightWidth() int {
	divider := 1
	if m.fullScreen {
		return max(10, m.width)
	}
	return max(10, m.width-m.listWidth()-divider)
}

func (m model) contentWidth() int {
	if m.mode == modeSplit {
		w := m.splitPreviewWidth
		if w <= 0 {
			w = m.viewport.Width() + 1
		}
		if w > 1 {
			w = w - 1 // reserve one column for preview scrollbar in split pane
		}
		if w < 10 {
			w = 10
		}
		return w
	}

	w := m.rightWidth()
	if m.fullScreen {
		if w > 1 {
			w = w - 1 // reserve one column for preview scrollbar in fullscreen
		}
	} else if w > 3 {
		w = w - 3 // border (2) + scrollbar (1)
	}
	if m.showSectionGauge() {
		w = w - (m.sectionGaugeWidth() + 1)
	}
	if w < 10 {
		w = 10
	}
	return w
}

func (m model) effectiveReadingWidth() int {
	baseWidth := m.contentWidth()
	if m.readingMode {
		const maxReadingWidth = 80
		if baseWidth > maxReadingWidth {
			return maxReadingWidth
		}
	}
	return baseWidth
}

func (m model) editScrollMetrics() (height, totalRows, maxOffset int) {
	height = m.textarea.Height()
	if height <= 0 {
		return height, 0, 0
	}
	width := max(1, m.textarea.Width())
	lines := strings.Split(m.textarea.Value(), "\n")
	for _, line := range lines {
		totalRows += len(editWrapLine([]rune(line), width))
	}
	maxOffset = max(0, totalRows-height)
	return height, totalRows, maxOffset
}

func (m *model) setRawOffsetFromScrollbarRow(row int) {
	height, totalRows, maxOffset := m.editScrollMetrics()
	if height <= 0 || totalRows <= 0 {
		return
	}
	target := scrollbarTargetOffset(row, m.scrollbarDragGrab, height, totalRows, height, maxOffset, !m.fullScreen)
	targetCursorDisplayRow := min(totalRows-1, target+height-1)
	lines := strings.Split(m.textarea.Value(), "\n")
	rowIdx, col, ok := editDisplayToSourcePoint(lines, max(1, m.textarea.Width()), targetCursorDisplayRow, 0)
	if !ok {
		return
	}
	m.cancelMomentum()
	m.focusRight = true
	_ = m.textarea.Focus()
	m.editClearSelection()
	m.editMouseSelecting = false
	m.editMouseAnchorOff = -1
	m.editClearPreferredColumn()
	m.editSetCursor(rowIdx, col)
}

func (m model) editScrollbarPercent() float64 {
	if m.mode != modeRaw && m.mode != modeSplit {
		return 0
	}
	_, _, maxOffset := m.editScrollMetrics()
	return scrollbarPercent(m.textarea.ScrollYOffset(), maxOffset)
}

func scrollbarPercent(offset, maxOffset int) float64 {
	if maxOffset <= 0 {
		return 1
	}
	if offset <= 0 {
		return 0
	}
	if offset >= maxOffset {
		return 1
	}
	return float64(offset) / float64(maxOffset)
}

type scrollbarGeometry struct {
	trackStart int
	trackEnd   int
	thumbStart int
	thumbEnd   int
	scrollable bool
}

func calculateScrollbarGeometry(height, totalRows, visibleRows int, percent float64, insetEnds bool) scrollbarGeometry {
	if height <= 0 {
		return scrollbarGeometry{}
	}
	trackStart := 0
	trackEnd := height
	if insetEnds && height > 2 {
		trackStart++
		trackEnd--
	}
	trackLen := trackEnd - trackStart
	if trackLen <= 0 || totalRows <= visibleRows || totalRows <= 0 {
		return scrollbarGeometry{
			trackStart: trackStart,
			trackEnd:   trackEnd,
			thumbStart: trackStart,
			thumbEnd:   trackEnd,
		}
	}

	visibleRows = max(1, min(visibleRows, totalRows))
	thumbLen := int(math.Round(float64(trackLen) * float64(visibleRows) / float64(totalRows)))
	thumbLen = max(1, min(thumbLen, trackLen))
	if trackLen > 1 {
		thumbLen = min(thumbLen, trackLen-1)
	}
	percent = math.Max(0, math.Min(1, percent))
	travel := trackLen - thumbLen
	thumbStart := trackStart
	if travel > 0 {
		thumbStart += int(math.Round(percent * float64(travel)))
	}
	return scrollbarGeometry{
		trackStart: trackStart,
		trackEnd:   trackEnd,
		thumbStart: thumbStart,
		thumbEnd:   thumbStart + thumbLen,
		scrollable: true,
	}
}

func (m model) chromeRows() int {
	rows := 1 // status bar
	if m.showSearch {
		rows += m.searchChromeRows()
	}
	if m.opMode != opNone {
		rows++
	}
	return rows
}

func (m model) searchChromeRows() int {
	if !m.showSearch {
		return 0
	}
	if m.showReplace {
		return 2
	}
	return 1
}

func (m model) availableContentHeight() int {
	rows := m.height - m.chromeRows() - m.breadcrumbRows() - m.stickyHeaderRows()
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m model) breadcrumbRows() int {
	if len(m.headings) == 0 && m.currentPath == "" {
		return 0
	}
	return 1
}

func (m model) promptRows() int {
	if m.opMode == opNone {
		return 0
	}
	return 1
}

func (m model) stickyHeaderRows() int {
	return 0
}

func (m *model) currentList() *list.Model {
	if m.showOutline {
		return &m.outline
	}
	return &m.fileList
}

func (m *model) handleSelectionChange(prevIndex int) tea.Cmd {
	l := m.currentList()
	if l.Index() == prevIndex {
		return nil
	}
	if m.showOutline {
		if m.mode == modePreview {
			if h, ok := l.SelectedItem().(headingItem); ok {
				m.setCurrentHeading(l.Index())
				cmd := m.jumpToHeading(h.title)
				m.updateBreadcrumb()
				return cmd
			}
		}
		return nil
	}
	if selected := m.selectedPath(); selected != "" {
		m.currentPath = selected
		m.previewYOffset = 0
		m.updateBreadcrumb()
		return m.loadFileCmd(selected)
	}
	return nil
}

func (m *model) jumpToHeading(title string) tea.Cmd {
	offset := findHeadingOffset(m.rendered, title, m.showLineNums)
	if offset >= 0 {
		cmd := m.scrollTo(offset)
		m.highlightLine = offset
		if idx := headingIndexForTitle(m.headings, title); idx >= 0 {
			m.setCurrentHeading(idx)
		}
		m.updateBreadcrumb()
		return cmd
	}
	return nil
}

func (m *model) setListWidthFromColumn(col int) {
	minLeft := 16
	minRight := 20
	if col < minLeft {
		col = minLeft
	}
	if col > m.width-minRight {
		col = m.width - minRight
	}
	m.listWidthRatio = float64(col) / float64(max(1, m.width))
}

func (m *model) adjustListRatio(delta float64) {
	m.setListWidthFromColumn(int(float64(m.width) * (m.listWidthRatio + delta)))
}

func (m *model) toggleFocusMode() {
	m.focusMode = !m.focusMode
	if m.focusMode {
		m.status = "Focus mode on"
	} else {
		m.status = "Focus mode off"
	}
}

func (m *model) toggleReadingMode() {
	m.readingMode = !m.readingMode
	if m.readingMode {
		m.status = "Reading mode on (80 char width, enhanced spacing)"
		if len(m.headings) == 0 {
			m.status = "Reading mode on (tip: add headings for section focus)"
		}
	} else {
		m.status = "Reading mode off"
	}
}

func (m model) autoDisableHeavyVisuals() bool {
	return m.perfVisualMode == perfVisualAuto && m.renderedLineCount >= largeDocLineThreshold
}

func (m model) heavyVisualsEnabled() bool {
	switch m.perfVisualMode {
	case perfVisualForceOn:
		return true
	case perfVisualForceOff:
		return false
	default:
		return !m.autoDisableHeavyVisuals()
	}
}

func (m *model) cyclePerfVisualMode() {
	switch m.perfVisualMode {
	case perfVisualAuto:
		m.perfVisualMode = perfVisualForceOn
	case perfVisualForceOn:
		m.perfVisualMode = perfVisualForceOff
	default:
		m.perfVisualMode = perfVisualAuto
	}
	m.lastPerfAutoDisable = m.autoDisableHeavyVisuals()
	switch m.perfVisualMode {
	case perfVisualForceOn:
		m.status = "Performance visuals: on"
	case perfVisualForceOff:
		m.status = "Performance visuals: off"
	default:
		if m.lastPerfAutoDisable {
			m.status = fmt.Sprintf("Performance visuals: auto (off for %d+ lines)", largeDocLineThreshold)
		} else {
			m.status = "Performance visuals: auto"
		}
	}
}

func (m *model) adjustZoom(delta float64) {
	if m.zoom <= 0 {
		m.zoom = 1.0
	}
	m.zoom += delta
	if m.zoom < 0.5 {
		m.zoom = 0.5
	}
	if m.zoom > 2.5 {
		m.zoom = 2.5
	}
	m.status = fmt.Sprintf("Zoom %.0f%%", m.zoom*100)
}

func (m model) onDivider(x int) bool {
	dividerX := m.listWidth()
	return x == dividerX || x == dividerX+1
}

func (m *model) moveThemeSelection(delta int) {
	m.themePickerIdx += delta
	if m.themePickerIdx < 0 {
		m.themePickerIdx = len(themeOrder) - 1
	}
	if m.themePickerIdx >= len(themeOrder) {
		m.themePickerIdx = 0
	}
}

func (m model) previewStatus() string {
	if m.richPreview && !m.supportsAdvancedRendering() {
		profile := "limited"
		if m.colorProfileKnown {
			profile = strings.ToLower(m.colorProfile.String())
		}
		return fmt.Sprintf("Plain preview (terminal profile: %s)", profile)
	}
	if m.richPreview {
		label := m.styleName
		if m.styleName == defaultStyleName {
			label = "default"
		}
		if m.followSystem {
			label = "system"
		}
		return fmt.Sprintf("Rich preview (%s)", label)
	}
	return "Plain preview (ASCII style)"
}

func nextStyle(current string) string {
	for i, name := range themeOrder {
		if name == current {
			return themeOrder[(i+1)%len(themeOrder)]
		}
	}
	return themeOrder[0]
}

func themeIndex(name string) int {
	for i, n := range themeOrder {
		if n == name {
			return i
		}
	}
	return 0
}

func selectStyle(name string, rich bool, palette colorPalette) (*ansi.StyleConfig, bool) {
	background := palette.bg
	if !rich {
		cfg := styles.ASCIIStyleConfig
		emphasizeHeadings(&cfg, "#ffffff", "#a0aec0", "#94a3b8")
		applySurfaceBackground(&cfg, background, palette.surface)
		setBaseTextColor(&cfg, palette.text)
		cfg.CodeBlock.Margin = ptr(uint(0))
		return &cfg, false
	}
	var cfg ansi.StyleConfig
	switch name {
	case defaultStyleName:
		cfg = styles.DarkStyleConfig
		emphasizeHeadings(&cfg, "#7dd3fc", "#93c5fd", "#c084fc")
	case styles.DarkStyle:
		cfg = styles.DarkStyleConfig
		emphasizeHeadings(&cfg, "#ffd166", "#06d6a0", "#4dabf7")
	case styles.LightStyle:
		cfg = styles.LightStyleConfig
		emphasizeHeadings(&cfg, "#7f5af0", "#ff6b6b", "#227c9d")
	case styles.TokyoNightStyle:
		cfg = styles.TokyoNightStyleConfig
		emphasizeHeadings(&cfg, "#82aaff", "#c792ea", "#89ddff")
	case styles.PinkStyle:
		cfg = styles.PinkStyleConfig
		emphasizeHeadings(&cfg, "#ff6ad5", "#ffc0cb", "#ff9ecd")
	case styles.DraculaStyle:
		cfg = styles.DraculaStyleConfig
		emphasizeHeadings(&cfg, "#ffb86c", "#8be9fd", "#bd93f9")
	case catppuccinStyle:
		cfg = styles.DarkStyleConfig
		emphasizeHeadings(&cfg, "#cba6f7", "#89b4fa", "#94e2d5")
	case nordStyle:
		cfg = styles.DarkStyleConfig
		emphasizeHeadings(&cfg, "#88c0d0", "#81a1c1", "#a3be8c")
	case gruvboxStyle:
		cfg = styles.DarkStyleConfig
		emphasizeHeadings(&cfg, "#fabd2f", "#fe8019", "#b8bb26")
	default:
		cfg = styles.DarkStyleConfig
		emphasizeHeadings(&cfg, "#ffd166", "#06d6a0", "#4dabf7")
	}
	applySurfaceBackground(&cfg, background, palette.surface)
	setBaseTextColor(&cfg, palette.text)
	cfg.CodeBlock.Margin = ptr(uint(0))
	return &cfg, false
}

func emphasizeHeadings(cfg *ansi.StyleConfig, h1Color, h2Color, h3Color string) {
	apply := func(block *ansi.StyleBlock, color string, underline bool) {
		bold := true
		block.StylePrimitive.Bold = &bold
		if underline {
			block.StylePrimitive.Underline = &underline
		}
		block.StylePrimitive.Color = &color
		block.StylePrimitive.Prefix = ""
		if block.StylePrimitive.BlockPrefix == "" {
			block.StylePrimitive.BlockPrefix = "\n"
		}
		if block.StylePrimitive.BlockSuffix == "" {
			block.StylePrimitive.BlockSuffix = "\n"
		}
	}
	apply(&cfg.H1, h1Color, false)
	apply(&cfg.H2, h2Color, true)
	apply(&cfg.H3, h3Color, false)
}

func applySurfaceBackground(cfg *ansi.StyleConfig, background string, codeBackground string) {
	if codeBackground == "" {
		codeBackground = background
	}

	setBlockBackground := func(b *ansi.StyleBlock, bg string) {
		b.StylePrimitive.BackgroundColor = ptr(bg)
	}
	setPrimitiveBackground := func(p *ansi.StylePrimitive, bg string) {
		p.BackgroundColor = ptr(bg)
	}

	blocks := []*ansi.StyleBlock{
		&cfg.Document,
		&cfg.BlockQuote,
		&cfg.Paragraph,
		&cfg.List.StyleBlock,
		&cfg.Heading,
		&cfg.H1,
		&cfg.H2,
		&cfg.H3,
		&cfg.H4,
		&cfg.H5,
		&cfg.H6,
		&cfg.Table.StyleBlock,
		&cfg.DefinitionList,
		&cfg.HTMLBlock,
		&cfg.HTMLSpan,
	}
	for _, block := range blocks {
		setBlockBackground(block, background)
	}

	codeBlocks := []*ansi.StyleBlock{
		&cfg.Code,
		&cfg.CodeBlock.StyleBlock,
	}
	for _, block := range codeBlocks {
		setBlockBackground(block, codeBackground)
	}

	primitives := []*ansi.StylePrimitive{
		&cfg.Text,
		&cfg.Strikethrough,
		&cfg.Emph,
		&cfg.Strong,
		&cfg.HorizontalRule,
		&cfg.Item,
		&cfg.Enumeration,
		&cfg.Task.StylePrimitive,
		&cfg.Link,
		&cfg.LinkText,
		&cfg.Image,
		&cfg.ImageText,
		&cfg.DefinitionTerm,
		&cfg.DefinitionDescription,
	}
	for _, primitive := range primitives {
		setPrimitiveBackground(primitive, background)
	}

	if cfg.CodeBlock.Chroma != nil {
		setPrimitiveBackground(&cfg.CodeBlock.Chroma.Background, codeBackground)
	}
}

func setBaseTextColor(cfg *ansi.StyleConfig, textColor string) {
	cfg.Document.StylePrimitive.Color = ptr(textColor)
	cfg.Text.Color = ptr(textColor)
}

func ptr[T any](v T) *T {
	return &v
}

func applyGlobalBackground(s string, bg string) string {
	rw := getBackgroundRewriter(bg)
	out := rw.replacer.Replace(s)
	if !strings.HasSuffix(out, rw.seq) {
		out += rw.seq
	}
	return out
}

func backgroundSGR(color string) string {
	if strings.HasPrefix(color, "#") && len(color) == 7 {
		r, _ := strconv.ParseInt(color[1:3], 16, 64)
		g, _ := strconv.ParseInt(color[3:5], 16, 64)
		b, _ := strconv.ParseInt(color[5:7], 16, 64)
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\x1b[48;5;%sm", color)
}

func foregroundSGR(color string) string {
	if strings.HasPrefix(color, "#") && len(color) == 7 {
		r, _ := strconv.ParseInt(color[1:3], 16, 64)
		g, _ := strconv.ParseInt(color[3:5], 16, 64)
		b, _ := strconv.ParseInt(color[5:7], 16, 64)
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\x1b[38;5;%sm", color)
}

func (m model) outlineTreeView() string {
	content := renderHeadingTree(m.headings, m.outline.Index(), m.styles)
	if content == "" {
		content = lipgloss.NewStyle().
			Background(lipgloss.Color(m.palette.bg)).
			Foreground(lipgloss.Color(m.palette.muted)).
			Render("No outline")
	}
	return content
}

func renderHeadingTree(headings []headingItem, selected int, s uiStyles) string {
	if len(headings) == 0 {
		return ""
	}

	lines := make([]string, 0, len(headings))
	for i, h := range headings {
		display := h.display
		prefix := ""
		title := h.title
		if len(display) > len(h.title) {
			prefix = display[:max(0, len(display)-len(h.title))]
		}
		line := lipgloss.JoinHorizontal(lipgloss.Top,
			s.treePrefix.Render(prefix),
			s.treeTitle.Render(title),
		)
		if i == selected {
			line = lipgloss.JoinHorizontal(lipgloss.Top, s.treeMarkerActive, s.treeActive.Render(line))
		} else {
			line = lipgloss.JoinHorizontal(lipgloss.Top, s.treeMarkerInactive, s.treeNormal.Render(line))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func padLineToWidth(line string, _ int, bg string) string {
	bgSeq := getBackgroundRewriter(bg).seq
	return line + bgSeq + "\x1b[K\x1b[0m"
}

func padToFrame(content string, width int, height int, filler string) string {
	_ = width
	if height <= 0 {
		return content
	}
	var buf strings.Builder
	buf.Grow(len(content) + height*(len(filler)+1))
	lineCount := 0
	start := 0
	for lineCount < height {
		var line string
		if start <= len(content) {
			rel := strings.IndexByte(content[start:], '\n')
			if rel >= 0 {
				line = content[start : start+rel]
				start += rel + 1
			} else {
				line = content[start:]
				start = len(content) + 1
			}
		}
		if lineCount > 0 {
			buf.WriteByte('\n')
		}
		if line != "" {
			buf.WriteString(line)
		}
		buf.WriteString(filler)
		lineCount++
		if start > len(content) && line == "" {
			break
		}
	}
	for lineCount < height {
		if lineCount > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(filler)
		lineCount++
	}
	return buf.String()
}

func clampHeight(content string, height int, filler string) string {
	if height <= 0 {
		return ""
	}
	var buf strings.Builder
	buf.Grow(len(content))
	lineCount := 0
	start := 0
	for i := 0; i <= len(content) && lineCount < height; i++ {
		if i == len(content) || content[i] == '\n' {
			if lineCount > 0 {
				buf.WriteByte('\n')
			}
			buf.WriteString(content[start:i])
			lineCount++
			start = i + 1
		}
	}
	for lineCount < height {
		if lineCount > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(filler)
		lineCount++
	}
	return buf.String()
}

func renderWithScrollbarRows(rows []string, rowWidths []int, width int, height int, totalRows int, percent float64, s uiStyles, boundaryFlash int, insetEnds bool, lc *layoutCache) ([]string, []int) {
	if height <= 0 {
		return nil, nil
	}
	if width <= 0 {
		for i, row := range rows {
			w := 0
			if i < len(rowWidths) {
				w = rowWidths[i]
			} else {
				w = lipgloss.Width(row)
			}
			if w > width {
				width = w
			}
		}
	}
	geometry := calculateScrollbarGeometry(height, totalRows, height, percent, insetEnds)
	styledTrack := s.scrollTrackGlyph
	styledThumb := s.scrollThumbGlyph
	styledEmpty := s.scrollEmptyGlyph
	// Boundary flash arrows
	var flashTop, flashBottom string
	if boundaryFlash == 1 {
		flashTop = s.scrollFlashTop
	}
	if boundaryFlash == -1 {
		flashBottom = s.scrollFlashBottom
	}
	bgSGR := s.scrollBgSGR
	reset := s.scrollReset
	// Reuse slices from layoutCache when possible
	var outRows []string
	var outWidths []int
	if lc != nil && cap(lc.scrollRows) >= height {
		outRows = lc.scrollRows[:height]
		outWidths = lc.scrollWidths[:height]
	} else {
		outRows = make([]string, height)
		outWidths = make([]int, height)
	}
	if lc != nil {
		lc.scrollRows = outRows
		lc.scrollWidths = outWidths
	}
	trustRowWidths := lc != nil
	for i := range height {
		line := ""
		lineW := widthUnknown
		if i < len(rows) {
			line = rows[i]
			if i < len(rowWidths) {
				lineW = rowWidths[i]
			}
			if lineW == widthUnknown || !trustRowWidths {
				lineW = lipgloss.Width(line)
			}
			if lineW > width {
				// Guard against render rows that exceed viewport width (e.g. code
				// block decoration lines). If we don't clamp, the scrollbar glyph
				// is appended after the overflow and shifts right outside the pane.
				line = xansi.Truncate(line, width, "")
				lineW = width
			}
			line = applyRowBackground(line, s)
		}
		b := builderPool.Get().(*strings.Builder)
		b.Reset()
		b.Grow(len(line) + width + len(styledTrack) + 32)
		b.WriteString(line)
		// Direct padding: measure visible width, pad with spaces in bg color
		if pad := width - lineW; pad > 0 {
			b.WriteString(bgSGR)
			for j := 0; j < pad; j++ {
				b.WriteByte(' ')
			}
			b.WriteString(reset)
		}
		if !geometry.scrollable {
			b.WriteString(styledEmpty)
		} else if i == 0 && flashTop != "" {
			b.WriteString(flashTop)
		} else if i == height-1 && flashBottom != "" {
			b.WriteString(flashBottom)
		} else if i >= geometry.thumbStart && i < geometry.thumbEnd {
			b.WriteString(styledThumb)
		} else {
			b.WriteString(styledTrack)
		}
		outRows[i] = b.String()
		builderPool.Put(b)
		outWidths[i] = width + 1
	}
	return outRows, outWidths
}

func renderWithScrollbar(rows []string, rowWidths []int, width int, height int, totalRows int, percent float64, s uiStyles, boundaryFlash int, insetEnds bool) string {
	outRows, _ := renderWithScrollbarRows(rows, rowWidths, width, height, totalRows, percent, s, boundaryFlash, insetEnds, nil)
	if len(outRows) == 0 {
		return ""
	}
	return strings.Join(outRows, "\n")
}

func applyRowBackground(row string, s uiStyles) string {
	if row == "" {
		return ""
	}
	row = strings.ReplaceAll(row, "\x1b[0m", s.sgrResetBg)
	row = strings.ReplaceAll(row, "\x1b[m", s.sgrResetBg)
	row = strings.ReplaceAll(row, "\x1b[49m", s.sgrBgMain)
	return s.sgrBgMain + row
}

func joinRowsHorizontal(leftRows []string, leftWidths []int, sep string, sepWidth int, rightRows []string, rightWidth int, height int) ([]string, []int) {
	if height <= 0 {
		return nil, nil
	}
	outRows := make([]string, height)
	outWidths := make([]int, height)
	for i := range height {
		left := ""
		leftW := 0
		if i < len(leftRows) {
			left = leftRows[i]
			if i < len(leftWidths) {
				leftW = leftWidths[i]
			} else {
				leftW = lipgloss.Width(left)
			}
		}
		right := ""
		if i < len(rightRows) {
			right = rightRows[i]
		}
		outRows[i] = left + sep + right
		outWidths[i] = leftW + sepWidth + rightWidth
	}
	return outRows, outWidths
}

func renderPreviewPaneBox(rows []string, rowWidths []int, innerWidth int, innerHeight int, active bool, borderless bool, s uiStyles, lc *layoutCache, topLabel string) string {
	if innerWidth < 1 {
		innerWidth = 1
	}
	if innerHeight < 1 {
		innerHeight = 1
	}
	bgSGR := s.scrollBgSGR
	resetBg := s.sgrResetBg
	if borderless {
		var b strings.Builder
		b.Grow(innerHeight * (innerWidth + 16))
		for i := range innerHeight {
			row := ""
			rowWidth := 0
			if i < len(rows) {
				row = rows[i]
				if i < len(rowWidths) && rowWidths[i] > 0 {
					rowWidth = rowWidths[i]
				} else {
					rowWidth = lipgloss.Width(row)
				}
				if rowWidth > innerWidth {
					row = xansi.Truncate(row, innerWidth, "")
					rowWidth = innerWidth
				}
				row = applyRowBackground(row, s)
			}
			b.WriteString(row)
			if rowWidth < innerWidth {
				b.WriteString(bgSGR)
				b.WriteString(strings.Repeat(" ", innerWidth-rowWidth))
				b.WriteString(resetBg)
			}
			if i < innerHeight-1 {
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	var leftBorder, rightBorder, topBorder, bottomBorder string
	if lc != nil &&
		lc.rightBorderWidth == innerWidth &&
		lc.rightBorderActive == active &&
		lc.rightBorderBg == s.sgrResetBg &&
		lc.rightBorderLabel == topLabel {
		leftBorder = lc.rightBorderLeft
		rightBorder = lc.rightBorderRight
		topBorder = lc.rightBorderTop
		bottomBorder = lc.rightBorderBottom
	} else {
		var borderChars lipgloss.Border
		var borderPrefix string
		if active {
			borderChars = lipgloss.ThickBorder()
			borderPrefix = s.sgrActiveBorder
		} else {
			borderChars = lipgloss.RoundedBorder()
			borderPrefix = s.sgrBorderNormal
		}
		resetBg := s.sgrResetBg
		leftBorder = borderPrefix + borderChars.Left + resetBg
		rightBorder = borderPrefix + borderChars.Right + resetBg
		topInner := strings.Repeat(borderChars.Top, innerWidth)
		if title := strings.TrimSpace(topLabel); title != "" {
			label := " " + title + " "
			if lipgloss.Width(label) > innerWidth {
				label = xansi.Truncate(label, innerWidth, "")
			}
			if rem := innerWidth - lipgloss.Width(label); rem > 0 {
				topInner = label + strings.Repeat(borderChars.Top, rem)
			} else {
				topInner = label
			}
		}
		topBorder = borderPrefix + borderChars.TopLeft + topInner + borderChars.TopRight + resetBg
		bottomBorder = borderPrefix + borderChars.BottomLeft + strings.Repeat(borderChars.Bottom, innerWidth) + borderChars.BottomRight + resetBg
		if lc != nil {
			lc.rightBorderLeft = leftBorder
			lc.rightBorderRight = rightBorder
			lc.rightBorderTop = topBorder
			lc.rightBorderBottom = bottomBorder
			lc.rightBorderActive = active
			lc.rightBorderWidth = innerWidth
			lc.rightBorderBg = s.sgrResetBg
			lc.rightBorderLabel = topLabel
		}
	}
	var b strings.Builder
	b.Grow((innerHeight + 2) * (innerWidth + 32))
	b.WriteString(topBorder)
	b.WriteByte('\n')
	for i := range innerHeight {
		row := ""
		rowWidth := 0
		if i < len(rows) {
			row = rows[i]
			if i < len(rowWidths) && rowWidths[i] > 0 {
				rowWidth = rowWidths[i]
			} else {
				rowWidth = lipgloss.Width(row)
			}
			if rowWidth > innerWidth {
				row = xansi.Truncate(row, innerWidth, "")
				rowWidth = innerWidth
			}
			row = applyRowBackground(row, s)
		}
		b.WriteString(leftBorder)
		b.WriteString(row)
		if pad := innerWidth - rowWidth; pad > 0 {
			b.WriteString(bgSGR)
			for j := 0; j < pad; j++ {
				b.WriteByte(' ')
			}
			b.WriteString(resetBg)
		}
		b.WriteString(rightBorder)
		if i < innerHeight-1 {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	b.WriteString(bottomBorder)
	return b.String()
}

func colorCodeBorders(content string, styledBorder string) string {
	const sentinel = "\uE001"
	if !strings.Contains(content, sentinel) {
		return content
	}
	return strings.ReplaceAll(content, sentinel, styledBorder)
}

func addLineNumbers(content string, palette colorPalette) string {
	numPrefix := foregroundSGR(palette.muted) + backgroundSGR(palette.bg)
	bgPrefix := backgroundSGR(palette.bg)
	return addLineNumbersSGR(content, numPrefix, bgPrefix, palette)
}

func addLineNumbersSGR(content string, numPrefix string, bgPrefix string, _ colorPalette) string {
	lines := strings.Split(content, "\n")
	width := len(strconv.Itoa(len(lines)))
	reset := "\x1b[0m"
	var buf strings.Builder
	buf.Grow(len(content) + len(lines)*(width+len(numPrefix)+len(bgPrefix)+len(reset)*2+4))
	numScratch := make([]byte, 0, width+1)
	for i, line := range lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(bgPrefix)
		buf.WriteString(numPrefix)
		// Format line number with left-padding using a reused scratch buffer.
		numScratch = strconv.AppendInt(numScratch[:0], int64(i+1), 10)
		for k := len(numScratch); k < width; k++ {
			buf.WriteByte(' ')
		}
		buf.Write(numScratch)
		buf.WriteByte(' ')
		buf.WriteString(reset)
		buf.WriteString(bgPrefix)
		buf.WriteString(line)
		buf.WriteString(reset)
	}
	return buf.String()
}

func (m *model) updateHeadingOffsets() {
	m.headingRenderLines = m.headingRenderLines[:0]
	m.headingRenderIndices = m.headingRenderIndices[:0]
	if len(m.headings) == 0 {
		return
	}
	// Reset all offsets
	for i := range m.headings {
		m.headings[i].renderLine = -1
	}
	// Build a map of lowercase heading titles to their indices (first unmatched)
	type target struct {
		lower   string
		indices []int
	}
	targets := make([]target, 0, len(m.headings))
	seen := make(map[string]int) // lower title -> index in targets
	for i, h := range m.headings {
		key := strings.ToLower(strings.TrimSpace(h.title))
		if idx, ok := seen[key]; ok {
			targets[idx].indices = append(targets[idx].indices, i)
		} else {
			seen[key] = len(targets)
			targets = append(targets, target{lower: key, indices: []int{i}})
		}
	}

	targetByTitle := make(map[string]*target, len(targets))
	for i := range targets {
		targetByTitle[targets[i].lower] = &targets[i]
	}

	lines := m.renderedLineCache
	if len(lines) == 0 {
		return
	}
	remaining := len(m.headings)
	// Fast path: exact text matches are common for rendered headings.
	for lineNum, line := range lines {
		plain := stripLineNumberPrefix(stripANSI(line))
		plainLower := strings.ToLower(strings.TrimSpace(plain))
		t := targetByTitle[plainLower]
		if t == nil || len(t.indices) == 0 {
			continue
		}
		idx := t.indices[0]
		m.headings[idx].renderLine = lineNum
		t.indices = t.indices[1:]
		remaining--
		if remaining <= 0 {
			break
		}
	}
	// Fallback for styled lines where heading text is embedded in a larger string.
	if remaining > 0 {
		for lineNum, line := range lines {
			plain := stripLineNumberPrefix(stripANSI(line))
			plainLower := strings.ToLower(strings.TrimSpace(plain))
			for ti := range targets {
				t := &targets[ti]
				if len(t.indices) == 0 {
					continue
				}
				if strings.Contains(plainLower, t.lower) {
					idx := t.indices[0]
					m.headings[idx].renderLine = lineNum
					t.indices = t.indices[1:]
					remaining--
				}
			}
			if remaining <= 0 {
				break
			}
		}
	}
	for i, h := range m.headings {
		if h.renderLine >= 0 {
			m.headingRenderLines = append(m.headingRenderLines, h.renderLine)
			m.headingRenderIndices = append(m.headingRenderIndices, i)
		}
	}
}

func (m *model) syncCurrentHeading(y int) {
	if len(m.headings) == 0 {
		m.highlightLine = y
		if m.currentHeading != -1 {
			m.setCurrentHeading(-1)
			m.updateBreadcrumb()
		}
		return
	}
	idx := headingIndexAtOffset(m.headingRenderLines, m.headingRenderIndices, y)
	if idx < 0 {
		idx = currentHeadingIndex(m.headings, y)
	}
	if idx < 0 {
		m.highlightLine = y
		if m.currentHeading != -1 {
			m.setCurrentHeading(-1)
			m.updateBreadcrumb()
		}
		return
	}
	hl := m.headings[idx].renderLine
	if idx == m.currentHeading && hl == m.highlightLine {
		return // nothing changed — skip outline.Select + breadcrumb rebuild
	}
	m.highlightLine = hl
	m.setCurrentHeading(idx)
	m.updateBreadcrumb()
}

func applyHighlight(content string, line int, hlPrefix string) string {
	if line < 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	applyHighlightLines(lines, line, hlPrefix)
	return strings.Join(lines, "\n")
}

func applyHighlightLines(lines []string, line int, hlPrefix string) {
	if line < 0 || line >= len(lines) {
		return
	}
	const reset = "\x1b[0m"
	raw := lines[line]
	if raw == "" {
		return
	}
	lead := 0
	for lead < len(raw) {
		ch := raw[lead]
		if ch != ' ' && ch != '\t' {
			break
		}
		lead++
	}
	if lead >= len(raw) {
		lines[line] = hlPrefix + raw + reset
		return
	}
	lines[line] = raw[:lead] + hlPrefix + raw[lead:] + reset
}
