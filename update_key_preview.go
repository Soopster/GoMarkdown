package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) handlePreviewModeKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	if (m.mode == modeRaw || m.mode == modeSplit) && m.focusRight {
		return m, nil, false
	}
	if !m.focusRight && !m.showOutline && !m.fullScreen {
		if next, cmd, ok := m.handleNavigatorKeyPress(msg); ok {
			return next, cmd, true
		}
	}
	switch msg.String() {
	case "ctrl+c", "q":
		return m, m.saveAndQuitCmd(), true
	case "p":
		m.previewYOffset = m.viewport.YOffset()
		m.mode = modePreview
		m.textarea.Blur()
		m.focusRight = true
		m.status = "Preview mode"
		return m, m.renderCurrentContent(), true
	case "v", "V":
		m.previewYOffset = m.viewport.YOffset()
		m.richPreview = !m.richPreview
		m.status = m.previewStatus()
		return m, m.renderCurrentContent(), true
	case "f":
		m.fullScreen = !m.fullScreen
		m.focusRight = true
		if m.fullScreen {
			m.showOutline = false
			m.status = "Full screen"
		} else {
			m.status = "Split view"
		}
		m.resizeViews()
		return m, m.renderCurrentContent(), true
	case "g":
		if m.mode == modePreview {
			m.cancelMomentum()
			m.viewport.SetYOffset(0)
			m.previewYOffset = 0
			m.status = "Top"
			return m, nil, true
		}
	case "G":
		if m.mode == modePreview {
			m.cancelMomentum()
			bottom := bottomOffsetFromRendered(m.rendered, m.viewport.Height())
			m.viewport.SetYOffset(bottom)
			m.previewYOffset = bottom
			m.status = "Bottom"
			return m, nil, true
		}
	case "j", "down":
		if m.mode == modePreview && m.focusRight {
			return m.handlePreviewScrollKey(msg, 1)
		}
	case "k", "up":
		if m.mode == modePreview && m.focusRight {
			return m.handlePreviewScrollKey(msg, -1)
		}
	case "=":
		m.adjustZoom(0.1)
		return m, m.renderCurrentContent(), true
	case "-":
		m.adjustZoom(-0.1)
		return m, m.renderCurrentContent(), true
	case "0":
		m.zoom = 1.0
		m.status = "Zoom 100%"
		return m, m.renderCurrentContent(), true
	case "y":
		m.codePlain = !m.codePlain
		if m.codePlain {
			m.status = "Code: plain"
		} else {
			m.status = "Code: syntax"
		}
		return m, m.renderCurrentContent(), true
	case "Y":
		path := m.currentPathForClipboard()
		if path == "" {
			return m, nil, true
		}
		m.status = "Copied path to clipboard"
		return m, tea.SetClipboard(path), true
	case "ctrl+y":
		link := m.currentHeadingLinkForClipboard()
		if link == "" {
			path := m.currentPathForClipboard()
			if path == "" {
				return m, nil, true
			}
			m.status = "Copied path to clipboard"
			return m, tea.SetClipboard(path), true
		}
		m.status = "Copied heading link"
		return m, tea.SetClipboard(link), true
	case "ctrl+shift+y":
		out := strings.TrimSpace(m.status)
		if out == "" {
			return m, nil, true
		}
		m.status = "Copied command output"
		return m, tea.SetClipboard(out), true
	case "t":
		m.previewYOffset = m.viewport.YOffset()
		themeCmd := m.applyTheme(nextStyle(m.styleName))
		m.status = fmt.Sprintf("Preview style: %s", m.styleName)
		return m, tea.Batch(themeCmd, m.renderCurrentContent()), true
	case "T":
		m.showThemePicker = true
		m.status = "Theme picker"
		return m, nil, true
	case "/":
		return m, m.openSearch(), true
	case "ctrl+k":
		m.showCmdPalette = true
		m.cmdFilter = ""
		m.cmdIdx = 0
		m.filteredCommands = m.filterCommands("")
		m.status = "Command palette"
		return m, nil, true
	case "m":
		m.reducedMotion = !m.reducedMotion
		if m.reducedMotion {
			m.status = "Reduced motion on"
		} else {
			m.status = "Reduced motion off"
		}
		return m, nil, true
	case "u":
		m.showGauge = !m.showGauge
		m.status = fmt.Sprintf("Section gauge %v", onOff(m.showGauge))
		m.resizeViews()
		return m, m.renderCurrentContent(), true
	case "x":
		m.toggleFocusMode()
		return m, m.renderCurrentContent(), true
	case "H":
		m.cyclePerfVisualMode()
		m.resizeViews()
		return m, m.renderCurrentContent(), true
	case "P":
		m.togglePerfOverlay()
		return m, nil, true
	case "z":
		m.toggleReadingMode()
		return m, m.renderCurrentContent(), true
	case "?":
		m.showHelp = true
		m.status = "Help"
		return m, nil, true
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if m.mode == modePreview {
			n := int(msg.String()[0] - '1')
			if n >= 0 && n < len(m.headings) {
				m.setCurrentHeading(n)
				cmd := m.jumpToHeading(m.headings[n].title)
				return m, cmd, true
			}
		}
	case "tab", "shift+tab", "ctrl+tab":
		m.focusRight = !m.focusRight
		if m.mode == modeRaw {
			if m.focusRight {
				m.textarea.Focus()
			} else {
				m.textarea.Blur()
			}
		}
		target := "preview"
		if !m.focusRight {
			if m.showOutline {
				target = "outline"
			} else {
				target = "files"
			}
		}
		m.status = fmt.Sprintf("Focus: %s", target)
		return m, nil, true
	case "[":
		m.adjustListRatio(-0.03)
		m.status = "Adjusted split"
		m.resizeViews()
		return m, nil, true
	case "]":
		m.adjustListRatio(0.03)
		m.status = "Adjusted split"
		m.resizeViews()
		return m, nil, true
	case "C", "c":
		if !m.showOutline && m.mode == modePreview {
			m.startCreate()
			return m, nil, true
		}
	case "R", "r":
		if !m.showOutline && m.mode == modePreview {
			m.startRename()
			return m, nil, true
		}
	case "D", "d":
		if !m.showOutline && m.mode == modePreview {
			m.startDelete()
			return m, nil, true
		}
	case "n":
		m.showLineNums = !m.showLineNums
		m.textarea.ShowLineNumbers = m.showLineNums
		m.status = fmt.Sprintf("Line numbers %v", onOff(m.showLineNums))
		if m.mode == modePreview {
			m.previewYOffset = m.viewport.YOffset()
			return m, m.renderCurrentContent(), true
		}
		return m, nil, true
	case "o":
		if len(m.outline.Items()) > 0 {
			m.showOutline = !m.showOutline
			if m.showOutline {
				m.status = "Outline mode"
				m.ensureOutlineSelection()
			} else {
				m.status = "File list"
			}
		}
		return m, nil, true
	case "esc":
		if m.mode == modeSplit || m.mode == modeRaw {
			wasSplit := m.mode == modeSplit
			m.mode = modePreview
			m.textarea.Blur()
			m.focusRight = true
			if wasSplit {
				m.resizeViews()
			}
			m.status = "Preview mode"
			return m, m.renderCurrentContent(), true
		}
		return m, nil, true
	case "e":
		if m.currentPath != "" {
			m.previewYOffset = m.viewport.YOffset()
			m.mode = modeRaw
			m.textarea.Focus()
			m.focusRight = true
			m.status = "Raw edit mode"
			m.positionEditorCursor()
		}
		return m, nil, true
	case "ctrl+r":
		return m, m.refreshFilesAndCurrentFileCmd(), true
	case "ctrl+p":
		return m.handledAction("find_file")
	case "ctrl+shift+f":
		return m.handledAction("content_search")
	case "alt+left":
		return m.handledAction("nav_back")
	case "ctrl+g":
		m.status = "Requested terminal capabilities/colors"
		m.capProbeRequested = true
		cmds := []tea.Cmd{requestTerminalColorsCmd(), requestTermcapProbeCmd()}
		return m, tea.Batch(cmds...), true
	case "ctrl+s":
		if m.mode == modeRaw {
			return m, m.saveCurrentFileCmd(), true
		}
	}
	return m, nil, false
}

func (m model) handleNavigatorKeyPress(msg tea.KeyPressMsg) (model, tea.Cmd, bool) {
	item, ok := m.selectedNavigatorItem()
	if !ok {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		if item.kind == navigatorDirNode {
			m.toggleNavigatorDir(item.path)
			if updated, ok := m.selectedNavigatorItem(); ok && updated.kind == navigatorDirNode && updated.expanded {
				m.status = fmt.Sprintf("Expanded %s", updated.name)
			} else {
				m.status = fmt.Sprintf("Collapsed %s", item.name)
			}
			return m, nil, true
		}
		if item.kind == navigatorFileNode && !sameFile(item.path, m.currentPath) {
			m.currentPath = item.path
			m.previewYOffset = 0
			m.updateBreadcrumb()
			return m, m.loadFileCmd(item.path), true
		}
		return m, nil, true
	case "right", "l":
		if item.kind != navigatorDirNode {
			return m, nil, false
		}
		if !item.expanded {
			m.toggleNavigatorDir(item.path)
			m.status = fmt.Sprintf("Expanded %s", item.name)
			return m, nil, true
		}
		items := m.navigatorItems()
		idx := m.fileList.Index()
		if idx+1 < len(items) && items[idx+1].depth > item.depth {
			m.fileList.Select(idx + 1)
			if selCmd := m.handleSelectionChange(idx); selCmd != nil {
				return m, selCmd, true
			}
			return m, nil, true
		}
		return m, nil, true
	case "left", "h":
		if item.kind == navigatorDirNode && item.expanded {
			m.toggleNavigatorDir(item.path)
			m.status = fmt.Sprintf("Collapsed %s", item.name)
			return m, nil, true
		}
		items := m.navigatorItems()
		idx := m.fileList.Index()
		for i := idx - 1; i >= 0; i-- {
			if items[i].kind == navigatorDirNode && items[i].depth < item.depth {
				m.fileList.Select(i)
				return m, nil, true
			}
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) handlePreviewScrollKey(msg tea.KeyPressMsg, dir int) (model, tea.Cmd, bool) {
	if m.reducedMotion {
		m.viewport.SetYOffset(m.viewport.YOffset() + dir)
		m.previewYOffset = m.viewport.YOffset()
		m.syncCurrentHeading(m.viewport.YOffset())
		return m, nil, true
	}
	if msg.IsRepeat {
		m.sawNativeKeyRepeat = true
		if m.scrollHoldTickActive {
			m.scrollHoldTickActive = false
			m.scrollHoldTickGen++
		}
	}
	repeat := m.isKeyboardScrollRepeat(dir, msg.IsRepeat)
	holdDuration := m.updateKeyboardScrollHold(dir)
	impulse, scrollDelta, maxVel := momentumParamsForAxis(momentumAxisPreviewVertical, repeat, holdDuration)
	scrollCmd := m.applyMomentumScroll(dir < 0, impulse, scrollDelta, maxVel)
	holdCmd := m.ensureKeyboardHoldTickLoop()
	return m, tea.Batch(scrollCmd, holdCmd), true
}
