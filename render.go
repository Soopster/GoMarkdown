package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func (m model) View() tea.View {
	var started time.Time
	if m.perf != nil && m.perf.enabled {
		started = time.Now()
	}
	if m.width == 0 {
		if !started.IsZero() {
			m.perf.recordDuration("view", time.Since(started))
		}
		v := tea.NewView("Loading...")
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.ReportFocus = true
		v.WindowTitle = m.windowTitle()
		v.KeyboardEnhancements.ReportEventTypes = true
		return v
	}

	if lc := m.layoutCache; m.reuseLastFrame && lc != nil && lc.frameContent != "" {
		if !started.IsZero() {
			m.perf.recordDuration("view", time.Since(started))
		}
		v := tea.NewView(lc.frameContent)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.ReportFocus = true
		v.WindowTitle = m.windowTitle()
		v.KeyboardEnhancements.ReportEventTypes = true
		v.OnMouse = lc.frameOnMouse
		return v
	}

	ui := m.renderLayout()
	framed := padToFrame(ui, m.width, m.height, m.styles.sgrFrameClear)
	if !started.IsZero() {
		m.perf.recordDuration("view", time.Since(started))
	}
	content, onMouse := m.layeredViewContent(framed)
	if lc := m.layoutCache; lc != nil {
		lc.frameContent = content
		lc.frameOnMouse = onMouse
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true
	v.WindowTitle = m.windowTitle()
	v.KeyboardEnhancements.ReportEventTypes = true
	v.OnMouse = onMouse
	return v
}

func (m model) renderLayout() string {
	var started time.Time
	if m.perf != nil && m.perf.enabled {
		started = time.Now()
	}
	if m.width == 0 {
		out := "Loading..."
		if !started.IsZero() {
			m.perf.recordDuration("layout", time.Since(started))
		}
		return out
	}

	if m.showHelp {
		out := m.renderHelpOverlay()
		if !started.IsZero() {
			m.perf.recordDuration("layout", time.Since(started))
		}
		return out
	}

	if m.showThemePicker {
		out := m.renderThemePicker()
		if !started.IsZero() {
			m.perf.recordDuration("layout", time.Since(started))
		}
		return out
	}

	if m.showCmdPalette {
		out := m.renderCmdPalette()
		if !started.IsZero() {
			m.perf.recordDuration("layout", time.Since(started))
		}
		return out
	}

	if m.showFileFinder {
		out := m.renderFileFinder()
		if !started.IsZero() {
			m.perf.recordDuration("layout", time.Since(started))
		}
		return out
	}

	if m.showContentSearch {
		out := m.renderContentSearch()
		if !started.IsZero() {
			m.perf.recordDuration("layout", time.Since(started))
		}
		return out
	}

	lc := m.layoutCache

	var rightRows []string
	var rightWidths []int
	if m.mode == modeSplit {
		// Split mode: editor left, preview right
		splitEditW := max(10, m.splitEditWidth)
		splitPreviewW := max(10, m.splitPreviewWidth)
		innerHeight := max(1, m.availableContentHeight()-2)
		editContentW := m.textarea.Width()
		if editContentW <= 0 {
			editContentW = max(1, splitEditW-1)
		}
		editHeight := m.textarea.Height()
		if editHeight <= 0 {
			editHeight = innerHeight
		}
		previewContentW := m.viewport.Width()
		if previewContentW <= 0 {
			previewContentW = max(10, splitPreviewW-1)
		}
		previewHeight := m.viewport.Height()
		if previewHeight <= 0 {
			previewHeight = innerHeight
		}

		// Editor half
		textLines := strings.Split(m.textarea.View(), "\n")
		textLines = m.applyEditSearchHighlights(textLines)
		if m.editSelActive {
			textLines = m.applyEditSelectionHighlight(textLines)
		}
		textLines = m.applyEditFocusLineHighlight(textLines)
		editRows, editWidths := renderWithScrollbarRows(textLines, nil, editContentW, editHeight, m.editScrollbarPercent(), m.styles, 0, true, nil)

		// Preview half
		pvRows, pvWidths := m.viewportVisibleRows()
		previewRows, _ := renderWithScrollbarRows(pvRows, pvWidths, previewContentW, previewHeight, m.viewport.ScrollPercent(), m.styles, m.boundaryFlash, true, lc)

		// Join with divider
		divider := m.styles.dividerVert
		rightRows, rightWidths = joinRowsHorizontal(editRows, editWidths, divider, 1, previewRows, splitPreviewW, innerHeight)
	} else if m.mode == modeRaw {
		textLines := strings.Split(m.textarea.View(), "\n")
		textLines = m.applyEditSearchHighlights(textLines)
		if m.editSelActive {
			textLines = m.applyEditSelectionHighlight(textLines)
		}
		textLines = m.applyEditFocusLineHighlight(textLines)
		textH := m.textarea.Height()
		if textH <= 0 {
			textH = 1
		}
		rightRows, rightWidths = renderWithScrollbarRows(textLines, nil, m.textarea.Width(), textH, m.editScrollbarPercent(), m.styles, 0, !m.fullScreen, nil)
	} else {
		rows, rowWidths := m.viewportVisibleRows()
		rightRows, rightWidths = renderWithScrollbarRows(rows, rowWidths, m.viewport.Width(), m.viewport.Height(), m.viewport.ScrollPercent(), m.styles, m.boundaryFlash, !m.fullScreen, lc)
		if m.showSectionGauge() {
			if gaugeRows := m.renderSectionGaugeRows(); len(gaugeRows) > 0 {
				sep := m.styles.dividerVert
				rightRows, rightWidths = joinRowsHorizontal(rightRows, rightWidths, sep, 1, gaugeRows, m.sectionGaugeWidth(), m.viewport.Height())
			}
		}
	}

	availHeight := m.availableContentHeight()
	innerHeight := max(1, availHeight-2)
	if m.fullScreen {
		innerHeight = max(1, availHeight)
	}
	lw := m.listWidth()

	// Cache left panel box — unchanged during pure scroll
	var leftBox string
	if !m.fullScreen {
		fileIdx := m.fileList.Index()
		fileCnt := len(m.fileList.Items())
		outIdx := m.outline.Index()
		if lc != nil &&
			lc.leftOutline == m.showOutline &&
			lc.leftFocusRight == m.focusRight &&
			lc.leftOutIdx == outIdx &&
			lc.leftHeadingCnt == len(m.headings) &&
			lc.leftFileIdx == fileIdx &&
			lc.leftFileCnt == fileCnt &&
			lc.leftWidth == lw &&
			lc.leftHeight == availHeight &&
			lc.leftBg == m.palette.bg &&
			lc.leftPanel != "" {
			leftBox = lc.leftPanel
		} else {
			var left string
			if m.showOutline {
				left = m.outlineTreeView()
			} else {
				left = m.fileList.View()
			}
			leftRows := strings.Split(left, "\n")
			leftInnerWidth := max(1, lw-2)
			leftTitle := ""
			if !m.showOutline {
				leftTitle = "Files"
			}
			leftBox = renderPreviewPaneBox(leftRows, nil, leftInnerWidth, innerHeight, !m.focusRight, false, m.styles, nil, leftTitle)
			if lc != nil {
				lc.leftPanel = leftBox
				lc.leftOutline = m.showOutline
				lc.leftFocusRight = m.focusRight
				lc.leftOutIdx = outIdx
				lc.leftHeadingCnt = len(m.headings)
				lc.leftFileIdx = fileIdx
				lc.leftFileCnt = fileCnt
				lc.leftWidth = lw
				lc.leftHeight = availHeight
				lc.leftBg = m.palette.bg
			}
		}
	}

	rightInnerWidth := max(1, m.rightWidth()-2)
	if m.fullScreen {
		rightInnerWidth = max(1, m.rightWidth())
	}
	rightTitle := ""
	if m.currentPath != "" && !m.fullScreen {
		rightTitle = filepath.Base(m.currentPath)
	}
	rightBox := renderPreviewPaneBox(rightRows, rightWidths, rightInnerWidth, innerHeight, m.focusRight, m.fullScreen, m.styles, lc, rightTitle)

	var body string
	if m.fullScreen {
		body = rightBox
	} else {
		var divStr string
		if lc != nil && lc.dividerBg == m.palette.bg && lc.dividerStr != "" {
			divStr = lc.dividerStr
		} else {
			divStr = m.styles.divider.Render("│")
			if m.heavyVisualsEnabled() {
				divStr = applyGlobalBackground(divStr, m.palette.bg)
			}
			if lc != nil {
				lc.dividerStr = divStr
				lc.dividerBg = m.palette.bg
			}
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, divStr, rightBox)
	}
	if m.heavyVisualsEnabled() && !(m.fullScreen && m.mode == modePreview) {
		// JoinHorizontal introduces bare resets; raw-mode rightBox has lipgloss
		// resets. In fullscreen+preview the preview pane box already uses
		// sgrResetBg so this is unnecessary.
		body = applyGlobalBackground(body, m.palette.bg)
	}
	if !(m.fullScreen && m.mode == modePreview) {
		bodyTargetHeight := m.availableContentHeight()
		body = clampHeight(body, bodyTargetHeight, m.styles.sgrFrameClear)
	}

	// Cache breadcrumb bar
	var outlineBar string
	hdgCnt := len(m.headings)
	if lc != nil &&
		lc.breadcrumbHdg == m.currentHeading &&
		lc.breadcrumbHdgCnt == hdgCnt &&
		lc.breadcrumbWidth == m.width &&
		lc.breadcrumbPath == m.currentPath &&
		lc.breadcrumbBg == m.palette.bg {
		outlineBar = lc.breadcrumbStr
	} else {
		outlineBar = m.renderBreadcrumbOutline()
		if outlineBar != "" && m.heavyVisualsEnabled() {
			outlineBar = applyGlobalBackground(outlineBar, m.palette.bg)
		}
		if lc != nil {
			lc.breadcrumbStr = outlineBar
			lc.breadcrumbHdg = m.currentHeading
			lc.breadcrumbHdgCnt = hdgCnt
			lc.breadcrumbWidth = m.width
			lc.breadcrumbPath = m.currentPath
			lc.breadcrumbBg = m.palette.bg
		}
	}
	// Cache status bar (bypass cache entirely when perf overlay is active)
	toastStr := ""
	if time.Now().Before(m.toastUntil) {
		toastStr = m.toast
	}
	perfOverlayOn := m.perf != nil && m.perf.enabled && m.perf.overlay
	statusCrumb := m.breadcrumb
	statusEditRow, statusEditCol, statusSelCh, statusSelLn := m.editStatusMetrics()
	var statusBar string
	if !perfOverlayOn && lc != nil &&
		lc.statusMsg == m.status &&
		lc.statusCrumb == statusCrumb &&
		lc.statusMode == m.mode &&
		lc.statusOutline == m.showOutline &&
		lc.statusFocusR == m.focusRight &&
		lc.statusFullScr == m.fullScreen &&
		lc.statusToast == toastStr &&
		lc.statusAutoRld == m.showAutoReload &&
		lc.statusWidth == m.width &&
		lc.statusBg == m.palette.bg &&
		lc.statusEditRow == statusEditRow &&
		lc.statusEditCol == statusEditCol &&
		lc.statusSelCh == statusSelCh &&
		lc.statusSelLn == statusSelLn &&
		lc.statusStr != "" {
		statusBar = lc.statusStr
	} else {
		statusBar = m.renderStatusBar()
		if m.heavyVisualsEnabled() {
			statusBar = applyGlobalBackground(statusBar, m.palette.bg)
		}
		if !perfOverlayOn && lc != nil {
			lc.statusStr = statusBar
			lc.statusMsg = m.status
			lc.statusCrumb = statusCrumb
			lc.statusMode = m.mode
			lc.statusOutline = m.showOutline
			lc.statusFocusR = m.focusRight
			lc.statusFullScr = m.fullScreen
			lc.statusToast = toastStr
			lc.statusAutoRld = m.showAutoReload
			lc.statusWidth = m.width
			lc.statusBg = m.palette.bg
			lc.statusEditRow = statusEditRow
			lc.statusEditCol = statusEditCol
			lc.statusSelCh = statusSelCh
			lc.statusSelLn = statusSelLn
		}
	}
	var extras []string
	if m.showSearch {
		searchBar := m.renderSearchBar()
		if m.heavyVisualsEnabled() {
			searchBar = applyGlobalBackground(searchBar, m.palette.bg)
		}
		extras = append(extras, searchBar)
	}

	parts := []string{}
	if outlineBar != "" {
		parts = append(parts, outlineBar)
	}
	promptBar := m.renderPromptBar()
	if promptBar != "" {
		if m.heavyVisualsEnabled() {
			promptBar = applyGlobalBackground(promptBar, m.palette.bg)
		}
		parts = append(parts, promptBar)
	}
	parts = append(parts, body)
	parts = append(parts, extras...)
	parts = append(parts, statusBar)
	out := strings.Join(parts, "\n")
	if !started.IsZero() {
		m.perf.recordDuration("layout", time.Since(started))
	}
	return out
}

func (m model) renderThemePicker() string {
	var rows []string
	for i, name := range themeOrder {
		pal := paletteForStyle(name)
		row := lipgloss.JoinHorizontal(
			lipgloss.Top,
			themePreview(pal),
			" ",
			lipgloss.NewStyle().
				Foreground(lipgloss.Color(m.palette.text)).
				Background(lipgloss.Color(m.palette.bg)).
				Render(themeLabel(name)),
		)
		if i == m.themePickerIdx && !m.followSystem {
			row = lipgloss.NewStyle().
				Background(lipgloss.Color(pal.highlight)).
				Foreground(lipgloss.Color(pal.text)).
				Padding(0, 1).
				Render(row)
		} else {
			row = lipgloss.NewStyle().
				Padding(0, 1).
				Background(lipgloss.Color(m.palette.bg)).
				Render(row)
		}
		rows = append(rows, row)
	}

	check := " "
	if m.followSystem {
		check = "×"
	}
	followRow := lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.bg)).
		Foreground(lipgloss.Color(m.palette.text)).
		Padding(0, 1).
		Render(fmt.Sprintf("[%s] Follow system theme (s)", check))
	rows = append(rows, followRow)

	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.palette.muted)).
		Background(lipgloss.Color(m.palette.bg)).
		Padding(0, 1).
		Render("↑/↓ select • enter apply • s follow system • esc close")
	rows = append(rows, hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.palette.border)).
		Background(lipgloss.Color(m.palette.bg)).
		Padding(0, 1).
		Render(strings.Join(rows, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(m.palette.bg))))
}

func (m model) renderCmdPalette() string {
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	muted := lipgloss.Color(m.palette.muted)
	highlight := lipgloss.Color(m.palette.highlight)
	innerWidth := max(40, min(88, max(40, m.width-10)))
	contentWidth := max(20, innerWidth-2)

	var rows []string

	// Filter input
	prompt := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Render("> ")
	filter := m.cmdFilter
	if filter == "" {
		filter = lipgloss.NewStyle().
			Background(bg).
			Foreground(muted).
			Render("Type to filter...")
	}
	inputText := xansi.Truncate(prompt+filter, contentWidth, "")
	inputRow := lipgloss.NewStyle().
		Background(bg).
		Width(innerWidth).
		Padding(0, 1).
		Render(inputText)
	rows = append(rows, inputRow)

	// Separator
	sep := lipgloss.NewStyle().
		Background(bg).
		Foreground(muted).
		Width(innerWidth).
		Render(strings.Repeat("─", innerWidth))
	rows = append(rows, sep)

	// Commands list (show up to 10)
	maxShow := 10
	cmds := m.filteredCommands
	selected := m.cmdIdx
	if selected < 0 {
		selected = 0
	}
	if len(cmds) > 0 && selected >= len(cmds) {
		selected = len(cmds) - 1
	}
	start, end := commandPaletteWindow(len(cmds), selected, maxShow)
	cmds = cmds[start:end]
	maxKeyWidth := 6
	for _, cmd := range cmds {
		w := lipgloss.Width("[" + cmd.keys + "]")
		if w > maxKeyWidth {
			maxKeyWidth = w
		}
	}
	if maxKeyWidth > 18 {
		maxKeyWidth = 18
	}
	nameWidth := max(10, contentWidth-3-maxKeyWidth)
	for i, cmd := range cmds {
		marker := "  "
		if start+i == selected {
			marker = "▸ "
		}
		name := xansi.Truncate(cmd.name, nameWidth, "")
		keys := lipgloss.NewStyle().
			Foreground(muted).
			Width(maxKeyWidth).
			Render(xansi.Truncate("["+cmd.keys+"]", maxKeyWidth, ""))
		nameStyle := lipgloss.NewStyle().
			Foreground(text).
			Width(nameWidth)
		line := marker + nameStyle.Render(name) + " " + keys
		line = xansi.Truncate(line, contentWidth, "")
		rowStyle := lipgloss.NewStyle().Width(innerWidth).Padding(0, 1)
		if start+i == selected {
			rowStyle = rowStyle.
				Background(highlight).
				Foreground(text)
		} else {
			rowStyle = rowStyle.
				Background(bg).
				Foreground(text)
		}
		row := rowStyle.Render(line)
		rows = append(rows, row)
	}

	if len(m.filteredCommands) == 0 {
		noMatch := lipgloss.NewStyle().
			Background(bg).
			Foreground(muted).
			Width(innerWidth).
			Padding(0, 1).
			Render("  No matching commands")
		rows = append(rows, noMatch)
	}

	// Hint
	hintText := "↑/↓ select • enter run • esc close"
	if len(m.filteredCommands) > maxShow {
		hintText = "↑/↓ select • enter run • esc close • pgup/pgdn jump"
	}
	hint := lipgloss.NewStyle().
		Foreground(muted).
		Background(bg).
		Width(innerWidth).
		Padding(0, 1).
		Render(hintText)
	rows = append(rows, hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.palette.border)).
		Background(bg).
		Padding(0, 1).
		Render(strings.Join(rows, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(bg).Foreground(text)))
}

func commandPaletteWindow(total, selected, maxShow int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if maxShow <= 0 || maxShow >= total {
		return 0, total
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	start = selected - maxShow/2
	if start < 0 {
		start = 0
	}
	end = start + maxShow
	if end > total {
		end = total
		start = end - maxShow
	}
	return start, end
}

func (m model) renderSearchBar() string {
	bg := lipgloss.Color(m.palette.bg)
	muted := lipgloss.Color(m.palette.muted)
	text := lipgloss.Color(m.palette.text)

	var count string
	if strings.TrimSpace(m.searchInput.Value()) == "" {
		count = ""
	} else {
		total := len(m.searchMatches)
		if total == 0 {
			count = "No matches"
		} else if m.searchIndex < 0 || m.searchIndex >= total {
			if total == 1 {
				count = "1 match"
			} else {
				count = fmt.Sprintf("%d matches", total)
			}
		} else if total == 1 {
			count = "1 of 1 match"
		} else {
			current := m.searchIndex + 1
			count = fmt.Sprintf("%d of %d matches", current, total)
		}
	}

	inputView := m.searchInput.View()
	countView := lipgloss.NewStyle().
		Background(bg).
		Foreground(muted).
		Render(" " + count)
	searchLabel := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Render("Find ")
	searchRow := lipgloss.JoinHorizontal(lipgloss.Top,
		searchLabel,
		lipgloss.NewStyle().Background(bg).Render(inputView),
		countView,
	)
	searchRow = padLineToWidth(searchRow, m.width, m.palette.bg)
	if !m.showReplace {
		return searchRow
	}

	replaceInputView := m.replaceInput.View()
	replaceLabel := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Render("Repl ")
	flagOn := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color(m.palette.warn)).
		Bold(true)
	flagOff := lipgloss.NewStyle().
		Background(bg).
		Foreground(muted)
	renderFlag := func(enabled bool, text string) string {
		if enabled {
			return flagOn.Render(text)
		}
		return flagOff.Render(text)
	}
	scopeEnabled := m.replaceScopeSelection && m.editHasSelection()
	flags := strings.Join([]string{
		renderFlag(m.replaceCaseSensitive, "[Aa]"),
		renderFlag(m.replaceWholeWord, "[W]"),
		renderFlag(scopeEnabled, "[Sel]"),
	}, " ")
	replaceRow := lipgloss.JoinHorizontal(lipgloss.Top,
		replaceLabel,
		lipgloss.NewStyle().Background(bg).Render(replaceInputView),
		lipgloss.NewStyle().Background(bg).Foreground(muted).Render(" "+flags),
	)
	replaceRow = padLineToWidth(replaceRow, m.width, m.palette.bg)
	return searchRow + "\n" + replaceRow
}

func (m model) hasQuickActions() bool {
	if m.showHelp || m.showThemePicker || m.showCmdPalette {
		return false
	}
	return true
}

func (m model) quickActions() []quickAction {
	if m.showHelp || m.showThemePicker || m.showCmdPalette {
		return nil
	}
	switch m.opMode {
	case opCreate:
		return []quickAction{
			{key: "enter", desc: "Create"},
			{key: "ctrl+v", desc: "Paste"},
			{key: "esc", desc: "Cancel"},
		}
	case opRename:
		return []quickAction{
			{key: "enter", desc: "Rename"},
			{key: "ctrl+v", desc: "Paste"},
			{key: "esc", desc: "Cancel"},
		}
	case opDeleteConfirm:
		return []quickAction{
			{key: "y", desc: "Delete"},
			{key: "n", desc: "Cancel"},
		}
	case opGoToLine:
		return []quickAction{
			{key: "enter", desc: "Jump"},
			{key: "ctrl+v", desc: "Paste"},
			{key: "esc", desc: "Cancel"},
		}
	case opGoToHeading:
		return []quickAction{
			{key: "enter", desc: "Jump"},
			{key: "ctrl+v", desc: "Paste"},
			{key: "esc", desc: "Cancel"},
		}
	}
	if m.showSearch {
		if m.showReplace {
			return []quickAction{
				{key: "enter", desc: "Replace"},
				{key: "ctrl+enter", desc: "Replace all"},
				{key: "tab", desc: "Swap field"},
				{key: "alt+c/w/s", desc: "Case/word/scope"},
				{key: "esc", desc: "Close replace"},
			}
		}
		return []quickAction{
			{key: "enter", desc: "Next"},
			{key: "shift+enter", desc: "Prev"},
			{key: "ctrl+n", desc: "Next"},
			{key: "ctrl+p", desc: "Prev"},
			{key: "ctrl+v", desc: "Paste"},
			{key: "esc", desc: "Close"},
		}
	}
	if m.mode == modeRaw {
		target := "editor"
		if m.focusRight {
			if m.showOutline {
				target = "outline"
			} else {
				target = "files"
			}
		}
		return []quickAction{
			{key: "ctrl+s", desc: "Save"},
			{key: "ctrl+y", desc: "Redo"},
			{key: "ctrl+v", desc: "Paste"},
			{key: "alt+y", desc: "Copy line"},
			{key: "esc", desc: "Preview"},
			{key: "ctrl+tab", desc: fmt.Sprintf("Focus %s", target)},
			{key: "?", desc: "Help"},
		}
	}

	actions := []quickAction{
		{key: "/", desc: "Search"},
	}
	if len(m.headings) > 0 {
		label := "Outline"
		if m.showOutline {
			label = "Files"
		}
		actions = append(actions, quickAction{key: "o", desc: label})
	}
	if m.currentPath != "" {
		actions = append(actions, quickAction{key: "e", desc: "Edit"})
		actions = append(actions, quickAction{key: "Y", desc: "Copy path"})
	} else {
		actions = append(actions, quickAction{key: "C", desc: "New file"})
	}
	actions = append(actions, quickAction{key: "f", desc: "Full"})
	actions = append(actions, quickAction{key: "ctrl+o", desc: "Shell"})
	actions = append(actions, quickAction{key: "H", desc: "Perf"})
	actions = append(actions, quickAction{key: "P", desc: "Overlay"})
	actions = append(actions, quickAction{key: "?", desc: "Help"})
	return actions
}

func (m model) renderQuickActionsBar() string {
	actions := m.quickActions()
	if len(actions) == 0 || m.width == 0 {
		return ""
	}
	bg := lipgloss.Color(m.palette.bg)
	keyStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.highlight)).
		Foreground(lipgloss.Color(m.palette.text)).
		Bold(true).
		Padding(0, 1)
	descStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color(m.palette.muted)).
		Padding(0, 1)
	gap := lipgloss.NewStyle().Background(bg).Render(" ")

	var parts []string
	for _, a := range actions {
		token := lipgloss.JoinHorizontal(lipgloss.Top, keyStyle.Render(a.key), descStyle.Render(a.desc))
		parts = append(parts, token)
	}
	line := strings.Join(parts, gap)
	return padLineToWidth(line, m.width, m.palette.bg)
}

func (m model) editSelectionMetrics() (chars, lines int) {
	if m.mode != modeRaw || !m.editHasSelection() {
		return 0, 0
	}
	sR, sC, eR, eC := m.editNormalizedSel()
	allLines := strings.Split(m.textarea.Value(), "\n")
	start := editRowColToRuneOffset(allLines, sR, sC)
	end := editRowColToRuneOffset(allLines, eR, eC)
	if end <= start {
		return 0, 0
	}
	return end - start, (eR - sR + 1)
}

func (m model) editStatusMetrics() (row, col, selChars, selLines int) {
	if m.mode != modeRaw {
		return 0, 0, 0, 0
	}
	row = m.textarea.Line() + 1
	col = m.editCursorCol() + 1
	selChars, selLines = m.editSelectionMetrics()
	return row, col, selChars, selLines
}

func (m model) editStatusText() string {
	row, col, selChars, selLines := m.editStatusMetrics()
	if row <= 0 {
		return ""
	}
	info := fmt.Sprintf("Ln %d, Col %d", row, col)
	if selChars > 0 {
		info += fmt.Sprintf("  Sel %dch/%dln", selChars, selLines)
	}
	return info
}

func (m model) renderStatusBar() string {
	bg := lipgloss.Color(m.palette.bg)
	muted := lipgloss.Color(m.palette.muted)
	text := lipgloss.Color(m.palette.text)

	crumb := m.breadcrumb
	if crumb == "" && m.dir != "" {
		crumb = filepath.Base(m.dir)
	}
	crumbView := lipgloss.NewStyle().Background(bg).Foreground(muted).Render(crumb)

	modeToken := "P"
	if m.mode == modeRaw {
		modeToken = "E"
	} else if m.mode == modeSplit {
		modeToken = "S"
	}
	panelToken := "F"
	if m.showOutline {
		panelToken = "O"
	}
	focusToken := "R"
	if !m.focusRight {
		focusToken = "L"
	}
	state := modeToken + "/" + panelToken + "/" + focusToken
	if m.fullScreen {
		state += " FS"
	}
	stateView := m.styles.statusChip.Render(state)

	msg := m.status
	activityChip := ""
	if m.toast != "" && time.Now().Before(m.toastUntil) {
		msg = m.toast
		activityChip = m.styles.statusChip.Render("✔")
	} else if m.showAutoReload {
		activityChip = m.styles.statusChip.Render("↻")
	}
	msgView := lipgloss.NewStyle().Background(bg).Foreground(text).Render(msg)

	parts := []string{crumbView, stateView}
	if m.colorProfileKnown {
		profile := strings.ToUpper(m.colorProfile.String())
		parts = append(parts, m.styles.statusChip.Render(profile))
	}
	if m.termCapRGB || m.termCapTc {
		var caps []string
		if m.termCapRGB {
			caps = append(caps, "RGB")
		}
		if m.termCapTc {
			caps = append(caps, "Tc")
		}
		parts = append(parts, m.styles.statusChip.Render(strings.Join(caps, "+")))
	}
	if !m.terminalFocused {
		parts = append(parts, m.styles.statusChipWarn.Render("UNFOCUSED"))
	}
	if activityChip != "" {
		parts = append(parts, activityChip)
	}
	if perf := m.perfStatusSummary(); perf != "" {
		perfView := lipgloss.NewStyle().Background(bg).Foreground(muted).Render(perf)
		parts = append(parts, perfView)
	}
	if editInfo := m.editStatusText(); editInfo != "" {
		editInfoView := lipgloss.NewStyle().Background(bg).Foreground(muted).Render(editInfo)
		parts = append(parts, editInfoView)
	}
	if m.editTableMode {
		parts = append(parts, m.styles.statusChip.Render("TABLE"))
	}
	if m.showStats && m.currentPath != "" {
		statsChip := m.docStatsChip()
		if statsChip != "" {
			parts = append(parts, lipgloss.NewStyle().Background(bg).Foreground(muted).Render(statsChip))
		}
	}
	if m.editDirty {
		parts = append(parts, m.styles.statusChipWarn.Render("DIRTY"))
	}
	if m.autoSave {
		parts = append(parts, m.styles.statusChip.Render("AUTO"))
	}
	parts = append(parts, msgView)

	sep := lipgloss.NewStyle().Background(bg).Render(" ")
	line := parts[0]
	for _, p := range parts[1:] {
		line = lipgloss.JoinHorizontal(lipgloss.Top, line, sep, p)
	}

	return lipgloss.NewStyle().
		Background(bg).
		Width(m.width).
		MaxWidth(m.width).
		Render(line)
}

func (m model) docStatsChip() string {
	s := m.docStats
	if s.words == 0 {
		return ""
	}
	readTime := ""
	if s.readingMins < 1 {
		readTime = "<1 min"
	} else {
		readTime = fmt.Sprintf("%.0f min", math.Ceil(s.readingMins))
	}
	return fmt.Sprintf("%dw %dc %dp %dh %s", s.words, s.chars, s.paragraphs, s.headings, readTime)
}

func (m model) frontmatterChips() string {
	if len(m.frontmatter) == 0 {
		return ""
	}
	var chips []string
	// Show title, date, tags in order if present
	for _, key := range []string{"title", "date", "tags", "author", "category"} {
		if val, ok := m.frontmatter[key]; ok && val != "" {
			chips = append(chips, key+":"+val)
		}
	}
	if len(chips) == 0 {
		return ""
	}
	return strings.Join(chips, " | ")
}

func (m model) autoReloadIndicator() string {
	if !m.showAutoReload {
		return ""
	}
	label := "↻ Auto-reloaded"
	if !m.autoReloadAt.IsZero() {
		label = fmt.Sprintf("↻ Auto-reloaded %s", m.autoReloadAt.Format("15:04:05"))
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.highlight)).
		Foreground(lipgloss.Color(m.palette.text)).
		Padding(0, 1).
		Render(label)
}

func (m model) renderBreadcrumbOutline() string {
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	muted := lipgloss.Color(m.palette.muted)
	if m.width == 0 {
		return ""
	}
	if len(m.headings) == 0 {
		if m.currentPath == "" {
			return ""
		}
		label := fmt.Sprintf("File: %s", filepath.Base(m.currentPath))
		if gitInd := m.gitStatusIndicator(m.currentPath); gitInd != "" {
			label += " [" + gitInd + "]"
		}
		fmChips := m.frontmatterChips()
		if fmChips != "" {
			label += "  " + fmChips
		}
		return padLineToWidth(lipgloss.NewStyle().
			Background(bg).
			Foreground(text).
			Bold(true).
			Padding(0, 1).
			Render(label), m.width, m.palette.bg)
	}
	active := lipgloss.NewStyle().
		Background(lipgloss.Color(m.palette.highlight)).
		Foreground(text).
		Padding(0, 1)
	inactive := lipgloss.NewStyle().
		Background(bg).
		Foreground(muted).
		Padding(0, 1)
	ellipsis := inactive.Render("…")
	ellipsisW := lipgloss.Width(ellipsis)

	maxWidth := m.width
	n := len(m.headings)
	current := m.currentHeading
	if current < 0 {
		current = 0
	}
	if current >= n {
		current = n - 1
	}
	tokenWidths := make([]int, n)
	for i, h := range m.headings {
		label := fmt.Sprintf("%d. %s", i+1, h.title)
		tokenWidths[i] = lipgloss.Width(inactive.Render(label))
	}

	start, end := current, current
	rangeW := tokenWidths[current]
	totalWidth := func(start, end, rangeW int) int {
		total := rangeW
		if start > 0 {
			total += ellipsisW
		}
		if end < n-1 {
			total += ellipsisW
		}
		return total
	}
	if totalWidth(start, end, rangeW) > maxWidth {
		only := active.Render(fmt.Sprintf("%d. %s", current+1, m.headings[current].title))
		only = xansi.Truncate(only, max(1, maxWidth), "")
		return padLineToWidth(only, m.width, m.palette.bg)
	}
	for {
		bestSide := 0 // -1 left, +1 right
		bestBalance := int(^uint(0) >> 1)
		if start > 0 {
			newStart := start - 1
			newRangeW := rangeW + tokenWidths[newStart]
			if totalWidth(newStart, end, newRangeW) <= maxWidth {
				leftHidden := newStart
				rightHidden := (n - 1) - end
				balance := abs(leftHidden - rightHidden)
				bestSide = -1
				bestBalance = balance
			}
		}
		if end < n-1 {
			newEnd := end + 1
			newRangeW := rangeW + tokenWidths[newEnd]
			if totalWidth(start, newEnd, newRangeW) <= maxWidth {
				leftHidden := start
				rightHidden := (n - 1) - newEnd
				balance := abs(leftHidden - rightHidden)
				if bestSide == 0 || balance < bestBalance {
					bestSide = 1
					bestBalance = balance
				}
			}
		}
		if bestSide == 0 {
			break
		}
		if bestSide < 0 {
			start--
			rangeW += tokenWidths[start]
		} else {
			end++
			rangeW += tokenWidths[end]
		}
	}

	var parts []string
	if start > 0 {
		parts = append(parts, ellipsis)
	}
	for i := start; i <= end; i++ {
		label := fmt.Sprintf("%d. %s", i+1, m.headings[i].title)
		rendered := inactive.Render(label)
		if i == current {
			rendered = active.Render(label)
		}
		parts = append(parts, rendered)
	}
	if end < n-1 {
		parts = append(parts, ellipsis)
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return padLineToWidth(line, m.width, m.palette.bg)
}

func (m model) renderPromptBar() string {
	switch m.opMode {
	case opCreate:
		return m.renderPrompt("Create file", m.promptInput.View())
	case opRename:
		return m.renderPrompt(fmt.Sprintf("Rename %s", filepath.Base(m.opTarget)), m.promptInput.View())
	case opDeleteConfirm:
		msg := fmt.Sprintf("Delete %s? (y/n)", filepath.Base(m.opTarget))
		return m.renderPrompt(msg, "")
	case opGoToLine:
		return m.renderPrompt("Go to line[:col]", m.promptInput.View())
	case opGoToHeading:
		return m.renderPrompt("Go to heading (# or text)", m.promptInput.View())
	default:
		return ""
	}
}

func (m model) renderPrompt(label, content string) string {
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	bar := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Padding(0, 1).
		Render(label)
	if content != "" {
		bar = lipgloss.JoinHorizontal(lipgloss.Top, bar, content)
	}
	return padLineToWidth(bar, m.width, m.palette.bg)
}

func (m model) renderFileFinder() string {
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	muted := lipgloss.Color(m.palette.muted)
	highlight := lipgloss.Color(m.palette.highlight)
	innerWidth := max(40, min(88, max(40, m.width-10)))
	contentWidth := max(20, innerWidth-2)

	var rows []string

	// Input
	prompt := lipgloss.NewStyle().Background(bg).Foreground(text).Bold(true).Render("🔍 ")
	filter := m.fileFinderInput.Value()
	if filter == "" {
		filter = lipgloss.NewStyle().Background(bg).Foreground(muted).Render("Type to search files...")
	}
	inputRow := lipgloss.NewStyle().Background(bg).Width(innerWidth).Padding(0, 1).Render(
		xansi.Truncate(prompt+filter, contentWidth, ""))
	rows = append(rows, inputRow)

	// Separator
	sep := lipgloss.NewStyle().Background(bg).Foreground(muted).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
	rows = append(rows, sep)

	// Results
	maxShow := 12
	results := m.fileFinderResults
	selected := m.fileFinderIdx
	if selected < 0 {
		selected = 0
	}
	if len(results) > 0 && selected >= len(results) {
		selected = len(results) - 1
	}
	start := 0
	if selected >= maxShow {
		start = selected - maxShow + 1
	}
	end := min(start+maxShow, len(results))
	for i := start; i < end; i++ {
		marker := "  "
		if i == selected {
			marker = "▸ "
		}
		name := xansi.Truncate(results[i].path, contentWidth-4, "…")
		rowStyle := lipgloss.NewStyle().Width(innerWidth).Padding(0, 1)
		if i == selected {
			rowStyle = rowStyle.Background(highlight).Foreground(text)
		} else {
			rowStyle = rowStyle.Background(bg).Foreground(text)
		}
		rows = append(rows, rowStyle.Render(marker+name))
	}
	if len(results) == 0 {
		noMatch := lipgloss.NewStyle().Background(bg).Foreground(muted).Width(innerWidth).Padding(0, 1).Render("No matching files")
		rows = append(rows, noMatch)
	}

	// Count
	countStr := fmt.Sprintf("%d files", len(results))
	countRow := lipgloss.NewStyle().Background(bg).Foreground(muted).Width(innerWidth).Padding(0, 1).Render(countStr)
	rows = append(rows, countRow)

	body := strings.Join(rows, "\n")
	box := lipgloss.NewStyle().
		Background(bg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(muted).
		Width(innerWidth).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(lipgloss.Color(m.palette.bg))))
}

func (m model) renderContentSearch() string {
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	muted := lipgloss.Color(m.palette.muted)
	highlight := lipgloss.Color(m.palette.highlight)
	innerWidth := max(50, min(100, max(50, m.width-8)))
	contentWidth := max(30, innerWidth-2)

	var rows []string

	// Input
	prompt := lipgloss.NewStyle().Background(bg).Foreground(text).Bold(true).Render("grep: ")
	filter := m.contentSearchInput.Value()
	if filter == "" {
		filter = lipgloss.NewStyle().Background(bg).Foreground(muted).Render("Type to search across files...")
	}
	inputRow := lipgloss.NewStyle().Background(bg).Width(innerWidth).Padding(0, 1).Render(
		xansi.Truncate(prompt+filter, contentWidth, ""))
	rows = append(rows, inputRow)

	sep := lipgloss.NewStyle().Background(bg).Foreground(muted).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
	rows = append(rows, sep)

	// Results
	maxShow := 15
	results := m.contentSearchResults
	selected := m.contentSearchIdx
	if selected < 0 {
		selected = 0
	}
	if len(results) > 0 && selected >= len(results) {
		selected = len(results) - 1
	}
	start := 0
	if selected >= maxShow {
		start = selected - maxShow + 1
	}
	end := min(start+maxShow, len(results))
	for i := start; i < end; i++ {
		hit := results[i]
		marker := "  "
		if i == selected {
			marker = "▸ "
		}
		loc := fmt.Sprintf("%s:%d", hit.path, hit.line+1)
		ctx := hit.context
		maxCtx := contentWidth - lipgloss.Width(loc) - 5
		if maxCtx < 10 {
			maxCtx = 10
		}
		if len(ctx) > maxCtx {
			ctx = ctx[:maxCtx] + "…"
		}
		line := fmt.Sprintf("%s%s  %s", marker, loc, ctx)
		line = xansi.Truncate(line, contentWidth, "")
		rowStyle := lipgloss.NewStyle().Width(innerWidth).Padding(0, 1)
		if i == selected {
			rowStyle = rowStyle.Background(highlight).Foreground(text)
		} else {
			rowStyle = rowStyle.Background(bg).Foreground(text)
		}
		rows = append(rows, rowStyle.Render(line))
	}
	if len(results) == 0 && m.contentSearchInput.Value() != "" {
		noMatch := lipgloss.NewStyle().Background(bg).Foreground(muted).Width(innerWidth).Padding(0, 1).Render("No matches found")
		rows = append(rows, noMatch)
	}

	countStr := fmt.Sprintf("%d matches", len(results))
	countRow := lipgloss.NewStyle().Background(bg).Foreground(muted).Width(innerWidth).Padding(0, 1).Render(countStr)
	rows = append(rows, countRow)

	body := strings.Join(rows, "\n")
	box := lipgloss.NewStyle().
		Background(bg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(muted).
		Width(innerWidth).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(lipgloss.Color(m.palette.bg))))
}
