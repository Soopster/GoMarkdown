package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

const externalReloadDelay = 120 * time.Millisecond

func (m model) handleAsyncUpdate(msg tea.Msg) (model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case filesRefreshedMsg:
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		selectedPath := m.selectedNavigatorPath()
		current := m.currentPath
		m.fileTree = msg.root
		if msg.root != nil {
			m.expandNavigatorAncestors(current)
		}
		m.rebuildNavigatorItems(selectedPath)
		if len(m.fileList.Items()) == 0 {
			m.currentPath = ""
			m.loadedPath = ""
			m.setViewportContent("No markdown files found.")
			m.textarea.SetValue("")
			m.sourceContent = ""
			m.setOutline(nil, true)
			m.headingRenderLines = nil
			m.headingRenderIndices = nil
			m.showOutline = false
			m.highlightLine = -1
			m.status = "No files"
			return m, nil, true
		}
		items := m.navigatorItems()
		selected, ok := m.selectedNavigatorItem()
		targetPath := ""
		if current != "" && indexForPath(items, current) >= 0 {
			targetPath = current
		} else if ok && selected.kind == navigatorFileNode {
			targetPath = selected.path
		} else {
			for _, item := range items {
				if item.kind == navigatorFileNode {
					targetPath = item.path
					break
				}
			}
		}
		if targetPath == "" {
			m.status = "No files"
			return m, nil, true
		}
		restorePreview := current != "" && sameFile(targetPath, current) && (m.restoreSession || m.mode == modePreview || m.mode == modeSplit)
		shouldLoad := msg.reloadCurrent || !sameFile(targetPath, m.loadedPath)
		m.currentPath = targetPath
		if !restorePreview {
			m.previewYOffset = 0
		}
		m.restoreSession = false
		m.highlightLine = -1
		m.highlightLine = -1
		m.updateBreadcrumb()
		if shouldLoad {
			return m, m.loadFileCmd(m.currentPath), true
		}
		return m, nil, true

	case fileLoadedMsg:
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		if !sameFile(msg.path, m.currentPath) {
			return m, nil, true
		}
		preserveEditorPosition := false
		editorCursorOffset := 0
		if m.mode == modeRaw || m.mode == modeSplit {
			editorCursorOffset = m.editCursorOffset()
			preserveEditorPosition = m.textarea.Value() != "" ||
				m.textarea.Line() != 0 ||
				m.editCursorCol() != 0 ||
				m.textarea.ScrollYOffset() != 0 ||
				m.editHasSelection()
		}
		m.textarea.SetValue(msg.content)
		if preserveEditorPosition {
			maxOffset := len([]rune(msg.content))
			if editorCursorOffset > maxOffset {
				editorCursorOffset = maxOffset
			}
			m.editMoveCursorToOffset(editorCursorOffset)
			m.editClearSelection()
			if m.mode == modeRaw {
				m.ensureEditCursorVisibleSoft(false)
			}
		}
		m.sourceContent = msg.content
		m.loadedPath = msg.path
		m.syncNavigatorSelection(msg.path)
		m.headings = parseHeadings(msg.content)
		m.setOutline(m.headings, true)
		if m.restoreHeading >= 0 && m.restoreHeading < len(m.headings) {
			m.setCurrentHeading(m.restoreHeading)
		}
		m.restoreHeading = -1
		m.highlightLine = -1
		m.editDirty = false
		m.docStats = computeDocStats(msg.content, len(m.headings))
		m.frontmatter, m.frontmatterLines = parseFrontmatter(msg.content)
		m.ensureWatcherForPath(msg.path)
		m.trackRecentFile(msg.path)
		m.updateBreadcrumb()
		m.resizeViews()
		m.status = fmt.Sprintf("Loaded %s", filepath.Base(msg.path))
		if m.mode == modeRaw {
			m.textarea.Focus()
			m.focusRight = true
			if !preserveEditorPosition {
				m.positionEditorCursor()
			}
			return m, nil, true
		}
		m.invalidateRenderCaches()
		return m, m.renderCurrentContent(), true

	case renderedMsg:
		if m.mode != modePreview && m.mode != modeSplit {
			return m, nil, true
		}
		if msg.generation != 0 && msg.generation != m.renderGeneration {
			return m, nil, true
		}
		if m.perf != nil && msg.renderMs > 0 {
			m.perf.recordDuration("render", time.Duration(msg.renderMs*float64(time.Millisecond)))
		}
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		m.rendered = msg.content
		m.renderedLineCache = strings.Split(msg.content, "\n")
		m.renderedLineCount = len(m.renderedLineCache)
		autoDisabledNow := m.autoDisableHeavyVisuals()
		perfVisualFlip := m.perfVisualMode == perfVisualAuto && autoDisabledNow != m.lastPerfAutoDisable
		m.lastPerfAutoDisable = autoDisabledNow
		if perfVisualFlip {
			m.resizeViews()
		}
		// Update render output cache
		m.lastRenderCacheKey = msg.cacheKey
		m.lastRenderWidth = msg.width
		m.lastRenderLineNums = msg.lineNums
		m.lastRenderReading = msg.readingMode
		m.lastRenderRich = msg.richPreview
		m.lastRenderCode = msg.codePlain
		m.lastRenderStyle = msg.styleName
		m.lastRenderOutput = msg.content
		if strings.TrimSpace(m.searchInput.Value()) != "" {
			m.refreshSearchMatches()
		}
		m.updateHeadingOffsets()
		searchLine := -1
		if m.showSearch && len(m.searchMatches) > 0 && m.searchIndex >= 0 && m.searchIndex < len(m.searchMatches) {
			searchLine = m.searchMatches[m.searchIndex].line
		}
		if searchLine >= 0 {
			m.highlightLine = searchLine
		} else {
			m.syncCurrentHeading(msg.yOffset)
		}
		// Check if post-processing inputs are unchanged — skip SetContent if so
		searchCnt := len(m.searchMatches)
		if msg.content == m.lastVPRendered &&
			m.highlightLine == m.lastVPHighlight &&
			m.focusMode == m.lastVPFocus &&
			m.readingMode == m.lastVPReading &&
			m.currentHeading == m.lastVPHeading &&
			searchCnt == m.lastVPSearchCnt &&
			m.searchIndex == m.lastVPSearchIdx {
			m.viewport.SetYOffset(msg.yOffset)
			if perfVisualFlip {
				return m, m.renderCurrentContent(), true
			}
			return m, nil, true
		}
		content := msg.content
		postProcessNeeded := (m.focusMode && m.currentHeading >= 0) || searchCnt > 0 || m.highlightLine >= 0
		if postProcessNeeded {
			lines := slices.Clone(m.renderedLineCache)
			if m.focusMode && m.currentHeading >= 0 {
				prefix := m.styles.sgrDimPrefix
				if m.readingMode {
					prefix = m.styles.sgrReadingDim
				}
				applyFocusDimLines(lines, m.headings, m.currentHeading, prefix)
			}
			if searchCnt > 0 {
				applySearchHighlightsLines(lines, m.searchMatches, m.searchIndex, m.searchQueryLen, m.styles.sgrSearchPri, m.styles.sgrSearchSec)
			}
			if m.highlightLine >= 0 {
				applyHighlightLines(lines, m.highlightLine, m.styles.sgrHighlight)
			}
			content = strings.Join(lines, "\n")
			m.setViewportContentLines(content, lines)
		} else {
			m.setViewportContentLines(content, m.renderedLineCache)
		}
		m.viewport.SetYOffset(msg.yOffset)
		if m.pendingLinkAnchor != "" {
			anchor := m.pendingLinkAnchor
			m.pendingLinkAnchor = ""
			if idx := headingIndexForSlug(m.headings, anchor); idx >= 0 {
				m.setCurrentHeading(idx)
				m.updateBreadcrumb()
				return m, m.jumpToHeading(m.headings[idx].title), true
			}
			m.status = fmt.Sprintf("Anchor not found: #%s", anchor)
		}
		// Update viewport post-processing cache
		m.lastVPRendered = msg.content
		m.lastVPHighlight = m.highlightLine
		m.lastVPFocus = m.focusMode
		m.lastVPReading = m.readingMode
		m.lastVPHeading = m.currentHeading
		m.lastVPSearchCnt = searchCnt
		m.lastVPSearchIdx = m.searchIndex
		if perfVisualFlip {
			return m, m.renderCurrentContent(), true
		}
		return m, nil, true

	case scrollTickMsg:
		// Ignore stale ticks from previous momentum sessions
		if msg.generation != m.scrollMomentumGen || !m.scrollMomentum {
			return m, nil, true
		}
		m.decayInferredScrollHold()
		selecting := m.editSelectExtendActive && m.editSelActive
		return m, m.handleMomentumTick(selecting), true

	case keyboardHoldTickMsg:
		if msg.generation != m.scrollHoldTickGen || !m.scrollHoldTickActive || !m.scrollKeyHeld || m.scrollHoldDir == 0 {
			return m, nil, true
		}
		m.decayInferredScrollHold()
		if !m.scrollKeyHeld || m.scrollHoldDir == 0 {
			m.scrollHoldTickActive = false
			return m, nil, true
		}
		isUp := m.scrollHoldDir < 0
		holdDuration := time.Since(m.scrollHoldSince)
		axis := momentumAxisPreviewVertical
		var scrollCmd tea.Cmd
		if m.mode == modeRaw || m.mode == modeSplit {
			selecting := m.editSelectExtendActive && m.editSelActive && m.editSelectExtendDir == m.scrollHoldDir
			if m.scrollHoldDir == -2 || m.scrollHoldDir == 2 {
				axis = momentumAxisEditHorizontal
				impulse, scrollDelta, maxVel := momentumParamsForAxis(axis, true, holdDuration)
				scrollCmd = m.applyTextareaHorizontalMomentumScroll(isUp, impulse, scrollDelta, maxVel, selecting)
			} else {
				axis = momentumAxisEditVertical
				impulse, scrollDelta, maxVel := momentumParamsForAxis(axis, true, holdDuration)
				scrollCmd = m.applyTextareaMomentumScroll(isUp, impulse, scrollDelta, maxVel, selecting)
			}
		} else {
			impulse, scrollDelta, maxVel := momentumParamsForAxis(axis, true, holdDuration)
			scrollCmd = m.applyMomentumScroll(isUp, impulse, scrollDelta, maxVel)
		}
		if m.sawNativeKeyRepeat {
			m.scrollHoldTickActive = false
			return m, scrollCmd, true
		}
		nextCmd := keyboardHoldTickCmd(m.scrollHoldTickGen, keyboardHoldTickEvery)
		return m, tea.Batch(scrollCmd, nextCmd), true

	case boundaryFlashClearMsg:
		if msg.generation == m.boundaryFlashGen {
			m.boundaryFlash = 0
		}
		return m, nil, true

	case editFocusLineClearMsg:
		if msg.generation == m.editFocusLineGen {
			m.editFocusLine = -1
		}
		return m, nil, true

	case searchDebounceMsg:
		// Only render if this is still the current search generation
		if msg.generation == m.searchGeneration {
			return m, m.renderCurrentContent(), true
		}
		return m, nil, true

	case fileSavedMsg:
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		m.editDirty = false
		m.headings = parseHeadings(m.sourceContent)
		m.setOutline(m.headings, true)
		m.status = fmt.Sprintf("Saved %s", filepath.Base(msg.path))
		cmds := []tea.Cmd{m.renderCurrentContent()}
		cmds = append(cmds, m.showToast("Saved"))
		return m, tea.Batch(cmds...), true

	case fsEventMsg:
		cmds := []tea.Cmd{}
		if m.watcher != nil {
			cmds = append(cmds, watchFilesCmd(m.watcher))
		}
		// Only react to write/create/remove/rename events, not chmod
		if msg.event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
			return m, tea.Batch(cmds...), true
		}
		currentFileEvent := m.currentPath != "" && sameFile(msg.event.Name, m.currentPath)
		treeChanged := msg.event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
		currentPreviewReload := false
		if currentFileEvent && m.editDirty {
			if treeChanged {
				cmds = append(cmds, m.refreshFilesCmd())
			}
			cmds = append(cmds, m.showToast("External change detected (unsaved edits)"))
			m.status = "⚠ External change — unsaved edits"
			return m, tea.Batch(cmds...), true
		}
		if currentFileEvent && (m.mode == modePreview || m.mode == modeSplit) && !m.editDirty && msg.event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
			m.externalReloadGen++
			cmds = append(cmds, externalReloadCmd(m.externalReloadGen, m.currentPath, externalReloadDelay))
			currentPreviewReload = true
		}
		if treeChanged || !currentFileEvent {
			cmds = append(cmds, m.refreshFilesCmd())
		}
		if currentPreviewReload {
			cmds = append(cmds, autoReloadClearCmd(4*time.Second))
		}
		return m, tea.Batch(cmds...), true

	case externalReloadMsg:
		if msg.generation != m.externalReloadGen || msg.path == "" || !sameFile(msg.path, m.currentPath) {
			return m, nil, true
		}
		content, err := os.ReadFile(msg.path)
		if err != nil {
			return m, nil, true
		}
		if string(content) == m.sourceContent {
			return m, nil, true
		}
		m.previewYOffset = m.viewport.YOffset()
		m.status = "File changed, refreshing…"
		m.showAutoReload = true
		m.autoReloadAt = time.Now()
		m.invalidateRenderCaches()
		return m, tea.Batch(
			autoReloadClearCmd(4*time.Second),
			m.loadFileCmd(msg.path),
		), true

	case fileCreatedMsg:
		m.clearOp()
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		m.status = fmt.Sprintf("Created %s", filepath.Base(msg.path))
		m.currentPath = msg.path
		m.previewYOffset = 0
		cmds := []tea.Cmd{m.refreshFilesCmd(), m.loadFileCmd(msg.path)}
		return m, tea.Batch(cmds...), true

	case fileRenamedMsg:
		m.clearOp()
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		m.status = fmt.Sprintf("Renamed to %s", filepath.Base(msg.newPath))
		m.currentPath = msg.newPath
		cmds := []tea.Cmd{m.refreshFilesCmd()}
		if m.mode == modePreview {
			m.previewYOffset = 0
			cmds = append(cmds, m.loadFileCmd(msg.newPath))
		}
		return m, tea.Batch(cmds...), true

	case fileDeletedMsg:
		m.clearOp()
		if msg.err != nil {
			m.status = m.styles.statusWarn.Render(msg.err.Error())
			return m, nil, true
		}
		m.status = fmt.Sprintf("Deleted %s", filepath.Base(msg.path))
		if sameFile(m.currentPath, msg.path) {
			m.currentPath = ""
			m.loadedPath = ""
			m.textarea.SetValue("")
			m.sourceContent = ""
			m.setViewportContent("")
			m.headings = nil
			m.headingRenderLines = nil
			m.headingRenderIndices = nil
			m.setOutline(nil, true)
		}
		return m, m.refreshFilesCmd(), true

	case fsWatchErrMsg:
		m.status = m.styles.statusWarn.Render(msg.err.Error())
		if m.watcher != nil {
			return m, watchFilesCmd(m.watcher), true
		}
		return m, nil, true

	case autoReloadClearMsg:
		m.showAutoReload = false
		return m, nil, true

	case toastClearMsg:
		m.toast = ""
		m.toastUntil = time.Time{}
		return m, nil, true

	case autoSaveTickMsg:
		if msg.generation == m.autoSaveGen && m.editDirty && m.autoSave && m.currentPath != "" {
			m.editDirty = false
			return m, tea.Batch(m.saveFileCmd(m.currentPath, m.sourceContent), m.showToast("Auto-saved")), true
		}
		return m, nil, true

	case splitRenderDebounceMsg:
		if msg.generation == m.splitRenderGen && m.mode == modeSplit {
			return m, m.renderCurrentContent(), true
		}
		return m, nil, true

	case walkDirResultMsg:
		m.fileFinderAll = msg.files
		if m.showFileFinder {
			m.fileFinderResults = fuzzyMatch(m.fileFinderAll, m.fileFinderInput.Value())
		}
		return m, nil, true

	case contentSearchResultMsg:
		if msg.generation == m.contentSearchGen {
			m.contentSearchResults = msg.hits
			m.contentSearchIdx = 0
		}
		return m, nil, true

	case gitStatusMsg:
		m.gitFileStatus = msg.status
		return m, nil, true

	case contentSearchDebounceMsg:
		if msg.generation == m.contentSearchGen && m.showContentSearch {
			query := m.contentSearchInput.Value()
			if query != "" {
				return m, m.searchAcrossFilesCmd(query, m.contentSearchGen), true
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}
