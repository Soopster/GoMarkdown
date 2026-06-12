package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/glamour/styles"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"
)

func testModelNoWatcher() model {
	m := newModel()
	if m.watcher != nil {
		_ = m.watcher.Close()
		m.watcher = nil
	}
	// Keep tests deterministic regardless of local session state.
	m.mode = modePreview
	m.showSearch = false
	m.showCmdPalette = false
	m.showThemePicker = false
	m.showHelp = false
	m.opMode = opNone
	m.showLineNums = false
	m.textarea.ShowLineNumbers = false
	m.formatOnSave = false
	return m
}

func setupRawMouseTestModel() model {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.currentPath = "note.md"
	m.sourceContent = "alpha beta\ngamma"
	m.resizeViews()
	m.textarea.SetValue(m.sourceContent)
	m.textarea.MoveToBegin()
	return m
}

func TestFsEventCurrentPreviewFileShowsRefreshingStatusAndReloads(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")

	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.currentPath = path
	m.resizeViews()
	m.setViewportContent(strings.Repeat("line\n", 40))
	m.viewport.SetYOffset(7)
	m.previewYOffset = 0

	updated, cmd, handled := m.handleAsyncUpdate(fsEventMsg{
		event: fsnotify.Event{Name: path, Op: fsnotify.Write},
	})
	if !handled {
		t.Fatal("expected fs event to be handled")
	}
	if cmd == nil {
		t.Fatal("expected current preview file change to schedule reload commands")
	}
	if updated.toast != "" {
		t.Fatalf("expected no auto-reload toast before reload probe, got %q", updated.toast)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batched reload command, got %T", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("expected delayed reload probe and clear commands, got %d", len(batch))
	}
	if updated.previewYOffset != 0 {
		t.Fatalf("expected viewport anchor unchanged until reload executes, got %d", updated.previewYOffset)
	}
}

func TestFsEventCurrentSplitFileShowsRefreshingStatusAndReloads(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")

	m := testModelNoWatcher()
	m.mode = modeSplit
	m.width = 100
	m.height = 24
	m.currentPath = path
	m.resizeViews()
	m.setViewportContent(strings.Repeat("line\n", 40))
	m.viewport.SetYOffset(5)

	updated, cmd, handled := m.handleAsyncUpdate(fsEventMsg{
		event: fsnotify.Event{Name: path, Op: fsnotify.Write},
	})
	if !handled {
		t.Fatal("expected fs event to be handled")
	}
	if cmd == nil {
		t.Fatal("expected current split file change to schedule reload commands")
	}
	if updated.toast != "" {
		t.Fatalf("expected no auto-reload toast in split mode before reload probe, got %q", updated.toast)
	}
}

func TestFsEventNonCurrentPreviewFileRefreshesNavigatorWithoutReloadProbe(t *testing.T) {
	tmp := t.TempDir()
	currentPath := filepath.Join(tmp, "current.md")
	otherPath := filepath.Join(tmp, "other.md")
	if err := os.WriteFile(currentPath, []byte("# Current\n"), 0o644); err != nil {
		t.Fatalf("write current fixture: %v", err)
	}
	if err := os.WriteFile(otherPath, []byte("# Other\n"), 0o644); err != nil {
		t.Fatalf("write other fixture: %v", err)
	}

	m := testModelNoWatcher()
	m.mode = modePreview
	m.dir = tmp
	m.currentPath = currentPath
	m.loadedPath = currentPath
	m.externalReloadGen = 4

	updated, cmd, handled := m.handleAsyncUpdate(fsEventMsg{
		event: fsnotify.Event{Name: otherPath, Op: fsnotify.Write},
	})
	if !handled {
		t.Fatal("expected fs event to be handled")
	}
	if cmd == nil {
		t.Fatal("expected non-current file change to refresh the navigator")
	}
	if updated.externalReloadGen != 4 {
		t.Fatalf("expected non-current file change to avoid current-file reload probe, got generation %d", updated.externalReloadGen)
	}
	msg := cmd()
	switch got := msg.(type) {
	case filesRefreshedMsg:
		if got.reloadCurrent {
			t.Fatal("expected navigator refresh without current-file reload")
		}
	case tea.BatchMsg:
		if len(got) != 1 {
			t.Fatalf("expected one navigator refresh command, got %d", len(got))
		}
		refreshed, ok := got[0]().(filesRefreshedMsg)
		if !ok {
			t.Fatalf("expected navigator refresh message, got %T", got[0]())
		}
		if refreshed.reloadCurrent {
			t.Fatal("expected navigator refresh without current-file reload")
		}
	default:
		t.Fatalf("expected navigator refresh message, got %T", msg)
	}
}

func TestExternalReloadMsgSchedulesRefreshAndReloadForCurrentFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")
	if err := os.WriteFile(path, []byte("# changed\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.currentPath = path
	m.resizeViews()
	m.setViewportContent(strings.Repeat("line\n", 40))
	m.viewport.SetYOffset(6)
	m.sourceContent = "# before\n"
	m.externalReloadGen = 3

	updated, cmd, handled := m.handleAsyncUpdate(externalReloadMsg{
		generation: 3,
		path:       path,
	})
	if !handled {
		t.Fatal("expected external reload message to be handled")
	}
	if cmd == nil {
		t.Fatal("expected external reload to schedule refresh and load commands")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batched refresh/load command, got %T", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("expected clear and load commands, got %d", len(batch))
	}
	if updated.currentPath != path {
		t.Fatalf("expected current path unchanged, got %q", updated.currentPath)
	}
	if updated.status != "File changed, refreshing…" {
		t.Fatalf("expected refreshing status once modtime changed, got %q", updated.status)
	}
	if updated.previewYOffset != 6 {
		t.Fatalf("expected viewport offset preserved at reload time, got %d", updated.previewYOffset)
	}
	if !updated.showAutoReload {
		t.Fatal("expected auto-reload indicator enabled after confirmed change")
	}
}

func TestExternalReloadMsgIgnoresStaleGeneration(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.currentPath = "/tmp/note.md"
	m.externalReloadGen = 4

	_, cmd, handled := m.handleAsyncUpdate(externalReloadMsg{
		generation: 3,
		path:       m.currentPath,
	})
	if !handled {
		t.Fatal("expected external reload message to be handled")
	}
	if cmd != nil {
		t.Fatal("expected stale external reload to be ignored")
	}
}

func TestExternalReloadMsgIgnoresUnchangedCurrentFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")
	if err := os.WriteFile(path, []byte("# same\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	m := testModelNoWatcher()
	m.mode = modePreview
	m.currentPath = path
	m.externalReloadGen = 2
	m.sourceContent = "# same\n"

	_, cmd, handled := m.handleAsyncUpdate(externalReloadMsg{
		generation: 2,
		path:       path,
	})
	if !handled {
		t.Fatal("expected external reload message to be handled")
	}
	if cmd != nil {
		t.Fatal("expected unchanged current file to skip reload")
	}
}

func TestFileLoadedAfterFsEventReturnsToLoadedStatus(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")

	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.currentPath = path
	m.resizeViews()

	updated, _, handled := m.handleAsyncUpdate(fsEventMsg{
		event: fsnotify.Event{Name: path, Op: fsnotify.Write},
	})
	if !handled {
		t.Fatal("expected fs event to be handled")
	}
	updated, _, handled = updated.handleAsyncUpdate(fileLoadedMsg{
		path:    path,
		content: "# Changed\n",
	})
	if !handled {
		t.Fatal("expected file load to be handled")
	}
	if updated.status != "Loaded note.md" {
		t.Fatalf("expected normal loaded status after refresh, got %q", updated.status)
	}
}

func TestFsEventCurrentFileWhileDirtyWarnsWithoutAutoRefresh(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")

	m := testModelNoWatcher()
	m.mode = modePreview
	m.currentPath = path
	m.editDirty = true

	updated, cmd, handled := m.handleAsyncUpdate(fsEventMsg{
		event: fsnotify.Event{Name: path, Op: fsnotify.Write},
	})
	if !handled {
		t.Fatal("expected fs event to be handled")
	}
	if cmd == nil {
		t.Fatal("expected dirty-file warning commands")
	}
	if updated.status != "⚠ External change — unsaved edits" {
		t.Fatalf("expected unsaved-edits warning, got %q", updated.status)
	}
	if updated.showAutoReload {
		t.Fatal("did not expect auto-reload indicator while dirty")
	}
	if !strings.Contains(updated.toast, "External change detected") {
		t.Fatalf("expected external change toast, got %q", updated.toast)
	}
}

func TestFileLoadedAddsWatcherForCurrentFileDirectory(t *testing.T) {
	tmp := t.TempDir()
	subdir := filepath.Join(tmp, "docs")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(subdir, "note.md")
	if err := os.WriteFile(path, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer func() { _ = w.Close() }()

	m := testModelNoWatcher()
	m.mode = modePreview
	m.currentPath = path
	m.watcher = w

	updated, _, handled := m.handleAsyncUpdate(fileLoadedMsg{
		path:    path,
		content: "# Hello\n",
	})
	if !handled {
		t.Fatal("expected file load to be handled")
	}

	found := false
	for _, watched := range updated.watcher.WatchList() {
		if sameFile(watched, subdir) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected watcher to include current file directory %q, got %v", subdir, updated.watcher.WatchList())
	}
}

func TestInvalidateRenderCachesClearsFrameCache(t *testing.T) {
	m := testModelNoWatcher()
	m.layoutCache = &layoutCache{
		frameContent: "stale fullscreen frame",
		frameOnMouse: func(tea.MouseMsg) tea.Cmd { return nil },
	}

	m.invalidateRenderCaches()

	if m.layoutCache.frameContent != "" {
		t.Fatalf("expected frame cache content cleared, got %q", m.layoutCache.frameContent)
	}
	if m.layoutCache.frameOnMouse != nil {
		t.Fatal("expected frame cache mouse handler cleared")
	}
}

func TestFullscreenPreviewExternalReloadUpdatesVisibleContent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")
	if err := os.WriteFile(path, []byte("# Old\n\nbefore\n"), 0o644); err != nil {
		t.Fatalf("write initial fixture: %v", err)
	}

	m := testModelNoWatcher()
	m.mode = modePreview
	m.fullScreen = true
	m.width = 80
	m.height = 20
	m.currentPath = path
	m.resizeViews()

	loaded, cmd, handled := m.handleAsyncUpdate(fileLoadedMsg{
		path:    path,
		content: "# Old\n\nbefore\n",
	})
	if !handled || cmd == nil {
		t.Fatal("expected initial file load to schedule render")
	}
	rendered := cmd().(renderedMsg)
	loaded, _, handled = loaded.handleAsyncUpdate(rendered)
	if !handled {
		t.Fatal("expected initial rendered message to be handled")
	}
	initialLayout := stripANSI(loaded.renderLayout())
	if !strings.Contains(initialLayout, "before") {
		t.Fatalf("expected fullscreen layout to show initial content, got %q", initialLayout)
	}

	if err := os.WriteFile(path, []byte("# New\n\nafter\n"), 0o644); err != nil {
		t.Fatalf("write updated fixture: %v", err)
	}
	loaded.externalReloadGen = 1
	updated, cmd, handled := loaded.handleAsyncUpdate(externalReloadMsg{
		generation: 1,
		path:       path,
	})
	if !handled || cmd == nil {
		t.Fatal("expected external reload to schedule follow-up commands")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected external reload batch, got %T", cmd())
	}
	var reloadMsg fileLoadedMsg
	foundReload := false
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		msg := batchCmd()
		if loadedMsg, ok := msg.(fileLoadedMsg); ok {
			reloadMsg = loadedMsg
			foundReload = true
		}
	}
	if !foundReload {
		t.Fatal("expected external reload batch to include file load message")
	}
	updated, cmd, handled = updated.handleAsyncUpdate(reloadMsg)
	if !handled || cmd == nil {
		t.Fatal("expected reloaded file to schedule render")
	}
	rendered = cmd().(renderedMsg)
	updated, _, handled = updated.handleAsyncUpdate(rendered)
	if !handled {
		t.Fatal("expected reloaded rendered message to be handled")
	}
	finalLayout := stripANSI(updated.renderLayout())
	if !strings.Contains(finalLayout, "after") {
		t.Fatalf("expected fullscreen layout to show refreshed content, got %q", finalLayout)
	}
	if strings.Contains(finalLayout, "before") {
		t.Fatalf("expected fullscreen layout to evict stale content, got %q", finalLayout)
	}
}

func TestAllCommandsHaveNameKeysAndAction(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.headings = []headingItem{
		{title: "One"},
		{title: "Two"},
		{title: "Three"},
	}

	cmds := allCommands(&m)
	if len(cmds) == 0 {
		t.Fatal("expected non-empty command list")
	}
	for _, c := range cmds {
		if c.name == "" {
			t.Fatalf("command missing name: %+v", c)
		}
		// keys may be empty for palette-only commands (no keyboard shortcut).
		if c.action == "" {
			t.Fatalf("command missing action: %+v", c)
		}
	}
}

func TestFilterCommandsMatchesKeys(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.currentPath = "note.md"

	matches := m.filterCommands("ctrl+s")
	if len(matches) == 0 {
		t.Fatal("expected save command to match key query")
	}
	found := false
	for _, c := range matches {
		if c.action == "save_file" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected save_file in matches, got %+v", matches)
	}
}

func TestEditNormalizedSelUsesExclusiveEnd(t *testing.T) {
	m := testModelNoWatcher()
	m.textarea.SetValue("abcd")
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(2)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	sR, sC, eR, eC := m.editNormalizedSel()
	if sR != 0 || sC != 1 || eR != 0 || eC != 2 {
		t.Fatalf("unexpected normalized range: (%d,%d)-(%d,%d)", sR, sC, eR, eC)
	}
	if got := m.editSelectedText(); got != "b" {
		t.Fatalf("expected exclusive-end selection text, got %q", got)
	}
}

func TestEditSelectionDisplayRangeWrapsWithinLine(t *testing.T) {
	m := testModelNoWatcher()
	m.textarea.SetWidth(6)
	m.textarea.SetHeight(4)
	m.textarea.SetValue("abcdef")
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(5)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 2

	sR, sC, eR, eC, ok := m.editSelectionDisplayRange()
	if !ok {
		t.Fatal("expected wrapped selection display range")
	}
	if sR != 0 || sC != 2 || eR != 1 || eC != 1 {
		t.Fatalf("unexpected wrapped display range: (%d,%d)-(%d,%d)", sR, sC, eR, eC)
	}
}

func TestEditSelectionVisibleRangeRespectsScrollOffset(t *testing.T) {
	m := testModelNoWatcher()
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(2)
	m.textarea.SetValue("a\nb\nc\nd")
	_ = m.textarea.View()
	m.textarea.MoveToBegin()
	m.textarea.CursorDown()
	m.textarea.CursorDown()
	m.textarea.CursorDown()
	m.textarea.CursorUp()
	m.textarea.SetCursorColumn(1)

	m.editSelActive = true
	m.editSelAnchorRow = 2
	m.editSelAnchorCol = 0

	dSR, dSC, dER, dEC, displayOK := m.editSelectionDisplayRange()
	viewLines := strings.Split(m.textarea.View(), "\n")
	sR, sC, eR, eC, ok := m.editSelectionVisibleRange(len(viewLines))
	if !ok {
		t.Fatalf("expected visible selection range in viewport: displayOK=%v display=(%d,%d)-(%d,%d) scroll=%d viewRows=%d line=%d col=%d",
			displayOK, dSR, dSC, dER, dEC, m.textarea.ScrollYOffset(), len(viewLines), m.textarea.Line(), m.editCursorCol())
	}
	if sR != 0 || sC != 0 || eR != 0 || eC != 1 {
		t.Fatalf("unexpected visible range: (%d,%d)-(%d,%d)", sR, sC, eR, eC)
	}
}

func TestRawEditLeftCollapsesSelectionToStart(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(3)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	updated := updatedAny.(model)
	if updated.editSelActive {
		t.Fatal("expected selection to collapse on left arrow")
	}
	if got := updated.editCursorCol(); got != 1 {
		t.Fatalf("expected cursor to collapse to selection start, got %d", got)
	}
}

func TestRawEditTypingReplacesSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.Focus()
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(3)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "axd" {
		t.Fatalf("expected typed text to replace selection, got %q", got)
	}
	if updated.editSelActive {
		t.Fatal("expected selection cleared after replacement")
	}
}

func TestRawEditBackspaceDeletesSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(3)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "ad" {
		t.Fatalf("expected backspace to delete selection, got %q", got)
	}
	if got := updated.editCursorCol(); got != 1 {
		t.Fatalf("expected cursor at selection start after delete, got %d", got)
	}
}

func TestRawEditEnterReplacesSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(3)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a\nd" {
		t.Fatalf("expected enter to replace selection, got %q", got)
	}
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected cursor to move to inserted newline, got row %d", got)
	}
}

func TestRawEditTabIndentsLineWhenSelectionActive(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(3)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "    abcd" {
		t.Fatalf("expected tab to indent selected line, got %q", got)
	}
}

func TestRawEditTabWithoutSelectionInsertsSpaces(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("ad")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(1)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a    d" {
		t.Fatalf("expected tab to insert 4 spaces without selection, got %q", got)
	}
}

func TestRawEditHomeMovesToLineStart(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("    alpha")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(8)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	updated := updatedAny.(model)
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected home to move to line start, got %d", got)
	}
}

func TestRawEditShiftHomeExtendsSelectionToLineStart(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("    alpha")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(8)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModShift})
	updated := updatedAny.(model)
	if got := updated.editSelectedText(); got != "    alph" {
		t.Fatalf("expected shift+home to select to line start, got %q", got)
	}
}

func TestRawEditEndMovesToLineEnd(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("ab\ncdef")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.editSetCursor(1, 1)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	updated := updatedAny.(model)
	if got := updated.editCursorCol(); got != 4 {
		t.Fatalf("expected end to move to line end, got %d", got)
	}
}

func TestRawEditCtrlHomeAndCtrlEndMoveToDocumentBoundaries(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("one\ntwo\nthree")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.editSetCursor(1, 1)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.textarea.Line(); got != 0 {
		t.Fatalf("expected ctrl+home to move to first line, got %d", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected ctrl+home to move to column 0, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected ctrl+end to move to last line, got %d", got)
	}
	if got := updated.editCursorCol(); got != 5 {
		t.Fatalf("expected ctrl+end to move to last column, got %d", got)
	}
}

func TestRawEditCtrlAAndCtrlEFallbackToLineBoundaries(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(2)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected ctrl+a to move to line start, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	updated = updatedAny.(model)
	if got := updated.editCursorCol(); got != 4 {
		t.Fatalf("expected ctrl+e to move to line end, got %d", got)
	}
}

func TestRawEditCtrlShiftHTogglesWrappedHomeEndMode(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	if m.editHomeEndWrapped {
		t.Fatal("expected wrapped mode disabled by default")
	}
	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl | tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHomeEndWrapped {
		t.Fatal("expected ctrl+shift+h to enable wrapped home/end mode")
	}
}

func TestRawEditHomeWrappedModeUsesVisualSegmentBoundary(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.editHomeEndWrapped = true
	m.textarea.SetValue("word1 word2 word3")
	m.textarea.SetWidth(7)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(8)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	updated := updatedAny.(model)
	homeCol := updated.editCursorCol()
	if homeCol >= 8 {
		t.Fatalf("expected wrapped home to move left within visual segment, got %d", homeCol)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	updated = updatedAny.(model)
	endCol := updated.editCursorCol()
	if endCol <= homeCol {
		t.Fatalf("expected wrapped end to move right from home boundary, home=%d end=%d", homeCol, endCol)
	}
}

func TestRawEditPageDownMovesCursorByViewport(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.textarea.SetValue(strings.Join(lines, "\n"))
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	updated := updatedAny.(model)
	if got := updated.textarea.Line(); got < 5 {
		t.Fatalf("expected pgdown to move by a page, got line %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModShift})
	updated = updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected shift+pgup to extend selection")
	}
}

func TestRawEditCtrlShiftASelectsAll(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("ab\ncd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl | tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected ctrl+shift+a to select all content")
	}
	if got := updated.editSelectedText(); got != "ab\ncd" {
		t.Fatalf("expected full-content selection, got %q", got)
	}
}

func TestRawEditUpDownRepeatBuildsMomentum(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(40, 0)

	updatedAny, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, IsRepeat: true})
	updated := updatedAny.(model)
	if got := updated.textarea.Line(); got != 41 {
		t.Fatalf("expected repeated down to move at least one line immediately, got %d", got)
	}
	if cmd == nil {
		t.Fatal("expected repeated down to start momentum tick")
	}
	if !updated.scrollMomentum {
		t.Fatal("expected repeated down to enable momentum state")
	}
	for i := 0; i < 20; i++ {
		updatedAny, _ = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
		updated = updatedAny.(model)
	}
	if got := updated.textarea.Line(); got <= 41 {
		t.Fatalf("expected momentum ticks to keep moving down, got line %d", got)
	}

	updatedAny, cmd = updated.Update(tea.KeyPressMsg{Code: tea.KeyUp, IsRepeat: true})
	updated = updatedAny.(model)
	if cmd == nil {
		t.Fatal("expected repeated up to keep momentum tick running")
	}
	startLine := updated.textarea.Line()
	for i := 0; i < 20; i++ {
		updatedAny, _ = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
		updated = updatedAny.(model)
	}
	if got := updated.textarea.Line(); got >= startLine {
		t.Fatalf("expected momentum ticks to move up, start=%d got=%d", startLine, got)
	}
}

func TestRawEditLeftRightRepeatBuildsMomentum(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	content := strings.Repeat("x", 320)
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(0, 10)

	updatedAny, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRight, IsRepeat: true})
	updated := updatedAny.(model)
	if got := updated.editCursorCol(); got != 11 {
		t.Fatalf("expected repeated right to move at least one column immediately, got %d", got)
	}
	if cmd == nil {
		t.Fatal("expected repeated right to start momentum tick")
	}
	if !updated.scrollMomentum {
		t.Fatal("expected repeated right to enable momentum state")
	}
	if updated.scrollMomentumAxis != momentumAxisEditHorizontal {
		t.Fatalf("expected horizontal momentum axis, got %d", updated.scrollMomentumAxis)
	}
	for i := 0; i < 20; i++ {
		updatedAny, _ = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
		updated = updatedAny.(model)
	}
	if got := updated.editCursorCol(); got <= 11 {
		t.Fatalf("expected momentum ticks to keep moving right, got col %d", got)
	}
	if got := updated.textarea.Line(); got != 0 {
		t.Fatalf("expected right momentum to stay on same line for long line, got line %d", got)
	}

	updatedAny, cmd = updated.Update(tea.KeyPressMsg{Code: tea.KeyLeft, IsRepeat: true})
	updated = updatedAny.(model)
	if cmd == nil {
		t.Fatal("expected repeated left to keep momentum tick running")
	}
	startCol := updated.editCursorCol()
	for i := 0; i < 20; i++ {
		updatedAny, _ = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
		updated = updatedAny.(model)
	}
	if got := updated.editCursorCol(); got >= startCol {
		t.Fatalf("expected momentum ticks to move left, start=%d got=%d", startCol, got)
	}
}

func TestPreviewRepeatMomentumStopsAtBoundary(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	lines := make([]string, 120)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	content := strings.Join(lines, "\n")
	m.setViewportContent(content)
	maxOffset := max(0, len(m.viewportLines)-m.viewport.Height())
	if maxOffset < 2 {
		t.Fatal("expected enough content to test preview boundary momentum")
	}
	m.viewport.SetYOffset(maxOffset - 1)
	m.previewYOffset = m.viewport.YOffset()

	updatedAny, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, IsRepeat: true})
	updated := updatedAny.(model)
	if cmd == nil {
		t.Fatal("expected repeated down to start preview momentum tick")
	}
	if !updated.scrollMomentum {
		t.Fatal("expected preview repeated down to enable momentum state")
	}

	for i := 0; i < 20 && updated.scrollMomentum; i++ {
		updatedAny, cmd = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
		updated = updatedAny.(model)
	}
	if updated.scrollMomentum {
		t.Fatal("expected preview momentum to stop at boundary within 20 ticks")
	}
	if got := updated.previewYOffset; got != maxOffset {
		t.Fatalf("expected preview momentum to stop at bottom boundary, got %d want %d", got, maxOffset)
	}
	if updated.scrollVelocity != 0 || updated.scrollAccum != 0 {
		t.Fatalf("expected momentum state cleared at boundary, vel=%v accum=%v", updated.scrollVelocity, updated.scrollAccum)
	}
	bottomOffset := updated.previewYOffset

	updatedAny, _ = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
	updated = updatedAny.(model)
	if got := updated.previewYOffset; got != bottomOffset {
		t.Fatalf("expected stale tick after boundary stop to have no effect, got %d want %d", got, bottomOffset)
	}
}

func TestRawEditShiftRightCancelsActiveMomentum(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	lines := make([]string, 120)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i) + " abcdefghijklmnopqrstuvwxyz"
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(60, 5)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, IsRepeat: true})
	updated := updatedAny.(model)
	if !updated.scrollMomentum {
		t.Fatal("expected repeated down to start momentum")
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	updated = updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected shift+right to create selection")
	}
	if updated.scrollMomentum {
		t.Fatal("expected shift+right to cancel active momentum")
	}
	lineAfter := updated.textarea.Line()
	colAfter := updated.editCursorCol()

	updatedAny, _ = updated.Update(scrollTickMsg{generation: updated.scrollMomentumGen})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != lineAfter {
		t.Fatalf("expected stale momentum tick to have no effect on line, got %d want %d", got, lineAfter)
	}
	if got := updated.editCursorCol(); got != colAfter {
		t.Fatalf("expected stale momentum tick to have no effect on col, got %d want %d", got, colAfter)
	}
}

func TestRawEditShiftRightRepeatWithoutShiftModKeepsSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i) + " abcdefghijklmnopqrstuvwxyz"
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(55, 5)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected shift+right to start selection")
	}
	if got := updated.textarea.Line(); got != 55 {
		t.Fatalf("expected selection on current line, got line %d", got)
	}
	if got := updated.editCursorCol(); got != 6 {
		t.Fatalf("expected cursor to move right by one, got %d", got)
	}

	// Simulate terminal repeat events that lose both shift modifier and repeat flag.
	sawMomentum := false
	for i := 0; i < 5; i++ {
		updatedAny, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		updated = updatedAny.(model)
		if !updated.editHasSelection() {
			t.Fatal("expected rapid right without shift mod to continue selection")
		}
		if cmd != nil || updated.scrollMomentum {
			sawMomentum = true
		}
		if got := updated.textarea.Line(); got != 55 {
			t.Fatalf("expected repeat selection to stay on current line, got %d", got)
		}
	}
	if !sawMomentum {
		t.Fatal("expected momentum-capable behavior while extending horizontal selection")
	}
	if got := updated.editCursorCol(); got != 11 {
		t.Fatalf("expected selection continuation to move right across repeats, got %d", got)
	}
}

func TestRawEditShiftDownRepeatWithoutShiftModKeepsSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i) + " abcdefghijklmnopqrstuvwxyz"
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(40, 6)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected shift+down to start selection")
	}
	if got := updated.textarea.Line(); got != 41 {
		t.Fatalf("expected shift+down to move to next line, got %d", got)
	}

	sawMomentum := false
	for i := 0; i < 3; i++ {
		updatedAny, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		updated = updatedAny.(model)
		if !updated.editHasSelection() {
			t.Fatal("expected rapid down without shift mod to continue selection")
		}
		if cmd != nil || updated.scrollMomentum {
			sawMomentum = true
		}
	}
	if !sawMomentum {
		t.Fatal("expected momentum-capable behavior while extending vertical selection")
	}
	if got := updated.textarea.Line(); got != 44 {
		t.Fatalf("expected continued selection to extend down lines, got %d", got)
	}
}

func TestRawEditSelectionLatchBreaksOnDirectionChange(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcdef")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(2)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected shift+right to start selection")
	}

	// Direction change without shift should break latch and collapse selection.
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	updated = updatedAny.(model)
	if updated.editHasSelection() {
		t.Fatal("expected left after direction change to collapse selection")
	}
	if updated.editSelectExtendActive {
		t.Fatal("expected selection latch to clear after direction change")
	}
	if got := updated.editCursorCol(); got != 2 {
		t.Fatalf("expected cursor collapsed back to anchor column, got %d", got)
	}
}

func TestRawEditTypingNearBottomDoesNotJumpViewport(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.resizeViews()

	lines := make([]string, 180)
	lines[0] = "# Title"
	for i := 1; i < len(lines); i++ {
		lines[i] = "line " + strconv.Itoa(i)
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(170, 4)
	_ = m.textarea.View()
	before := m.textarea.ScrollYOffset()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	updated := updatedAny.(model)
	_ = updated.textarea.View()
	after := updated.textarea.ScrollYOffset()

	if after != before {
		t.Fatalf("expected stable raw editor scroll offset while typing, before=%d after=%d", before, after)
	}
}

func TestFileLoadedPreservesRawEditorCursorOnSameFileReload(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 24
	m.currentPath = "note.md"
	m.resizeViews()

	lines := make([]string, 220)
	lines[0] = "# Title"
	for i := 1; i < len(lines); i++ {
		lines[i] = "line " + strconv.Itoa(i)
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.textarea.SetWidth(60)
	m.textarea.SetHeight(8)
	m.sourceContent = content
	m.updateOutlineFromEditor()
	m.editSetCursor(180, 4)

	reloadedContent := strings.Replace(content, "line 180", "line 180 updated", 1)
	updated, _, handled := m.handleAsyncUpdate(fileLoadedMsg{
		path:    "note.md",
		content: reloadedContent,
	})
	if !handled {
		t.Fatal("expected same-file reload to be handled")
	}
	_ = updated.textarea.View()

	if got := updated.textarea.Line(); got != 180 {
		t.Fatalf("expected cursor line 180 after reload, got %d", got)
	}
	if got := updated.editCursorCol(); got != 4 {
		t.Fatalf("expected cursor column 4 after reload, got %d", got)
	}
	if got := updated.textarea.Value(); got != reloadedContent {
		t.Fatal("expected textarea content to reflect reloaded file content")
	}
}

func TestRawEditTabIndentsSelectedLines(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("a\nb\nc")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	lines := strings.Split(m.textarea.Value(), "\n")
	anchor := editRowColToRuneOffset(lines, 0, 0)
	cursor := editRowColToRuneOffset(lines, 1, len([]rune(lines[1])))
	m.editSetSelectionOffsets(anchor, cursor)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "    a\n    b\nc" {
		t.Fatalf("expected tab to indent selected lines, got %q", got)
	}
	sR, sC, eR, eC := updated.editNormalizedSel()
	if sR != 0 || sC != 4 || eR != 1 || eC != 5 {
		t.Fatalf("expected selection to track indented lines, got (%d,%d)-(%d,%d)", sR, sC, eR, eC)
	}
}

func TestRawEditShiftTabOutdentsCurrentLine(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("    a\nb")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(4)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a\nb" {
		t.Fatalf("expected shift+tab to outdent current line, got %q", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected cursor to move with outdent, got col %d", got)
	}
}

func TestRawEditShiftTabOutdentsSelectedLines(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("    a\n  b\nc")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	lines := strings.Split(m.textarea.Value(), "\n")
	anchor := editRowColToRuneOffset(lines, 0, 0)
	cursor := editRowColToRuneOffset(lines, 1, len([]rune(lines[1])))
	m.editSetSelectionOffsets(anchor, cursor)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a\nb\nc" {
		t.Fatalf("expected shift+tab to outdent selected lines, got %q", got)
	}
}

func TestRawEditVerticalMovementPreservesPreferredColumn(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("12345\nx\n12345")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(4)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated := updatedAny.(model)
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected first down to move to line 1, got %d", got)
	}
	if got := updated.editCursorCol(); got != 1 {
		t.Fatalf("expected first down to clamp cursor to col 1, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected second down to move to line 2, got %d", got)
	}
	if got := updated.editCursorCol(); got != 4 {
		t.Fatalf("expected preferred column to restore on longer line, got %d", got)
	}
}

func TestRawEditMouseDragSelectsText(t *testing.T) {
	m := setupRawMouseTestModel()
	startX, startY, ok := m.rawEditorCellForSource(0, 1)
	if !ok {
		t.Fatal("expected to map start source point to visible mouse cell")
	}
	endX, endY, ok := m.rawEditorCellForSource(0, 4)
	if !ok {
		t.Fatal("expected to map end source point to visible mouse cell")
	}

	updatedAny, _ := m.Update(tea.MouseClickMsg{X: startX, Y: startY, Button: tea.MouseLeft})
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(tea.MouseMotionMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(tea.MouseReleaseMsg{X: endX, Y: endY, Button: tea.MouseLeft})
	updated = updatedAny.(model)

	if !updated.editHasSelection() {
		t.Fatal("expected drag selection to remain active")
	}
	if got := updated.editSelectedText(); got != "lph" {
		t.Fatalf("expected dragged text selection, got %q", got)
	}
}

func TestRawEditMouseDoubleClickSelectsWord(t *testing.T) {
	m := setupRawMouseTestModel()
	x, y, ok := m.rawEditorCellForSource(0, 7) // inside "beta"
	if !ok {
		t.Fatal("expected to map source point to visible mouse cell")
	}

	click := tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
	release := tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
	updatedAny, _ := m.Update(click)
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(release)
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(click)
	updated = updatedAny.(model)

	if !updated.editHasSelection() {
		t.Fatal("expected double-click to select a word")
	}
	if got := updated.editSelectedText(); got != "beta" {
		t.Fatalf("expected double-click to select word under cursor, got %q", got)
	}
}

func TestRawEditMouseTripleClickSelectsLine(t *testing.T) {
	m := setupRawMouseTestModel()
	x, y, ok := m.rawEditorCellForSource(0, 2)
	if !ok {
		t.Fatal("expected to map source point to visible mouse cell")
	}

	click := tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
	release := tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
	updatedAny, _ := m.Update(click)
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(release)
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(click)
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(release)
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(click)
	updated = updatedAny.(model)

	if !updated.editHasSelection() {
		t.Fatal("expected triple-click to select line")
	}
	if got := updated.editSelectedText(); got != "alpha beta" {
		t.Fatalf("expected triple-click to select full line, got %q", got)
	}
}

func TestRawEditCtrlBWrapsSelectedText(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.Focus()
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(3)

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 1

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a**bc**d" {
		t.Fatalf("expected ctrl+b to bold selected text, got %q", got)
	}
	if !updated.editSelActive {
		t.Fatal("expected selection to remain active after formatting")
	}
	if got := updated.editSelectedText(); got != "bc" {
		t.Fatalf("expected inner text to stay selected after formatting, got %q", got)
	}
}

func TestRawEditCtrlBTogglesOffWhenSelectionIsAlreadyBold(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.Focus()
	m.textarea.SetValue("a**bc**d")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(5) // select inner "bc"

	m.editSelActive = true
	m.editSelAnchorRow = 0
	m.editSelAnchorCol = 3

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "abcd" {
		t.Fatalf("expected ctrl+b to remove surrounding bold markers, got %q", got)
	}
	if !updated.editSelActive {
		t.Fatal("expected selection to remain active after unformatting")
	}
	if got := updated.editSelectedText(); got != "bc" {
		t.Fatalf("expected inner text to remain selected after unformatting, got %q", got)
	}
}

func TestRawEditCtrlBWithoutSelectionInsertsPlaceholder(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.Focus()
	m.textarea.SetValue("ad")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(1)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a**bold text**d" {
		t.Fatalf("expected ctrl+b to insert placeholder when no selection, got %q", got)
	}
}

func TestRawEditCtrlLeftMovesByWord(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("one two three")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(len([]rune("one two three")))

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.editCursorCol(); got != len([]rune("one two ")) {
		t.Fatalf("expected ctrl+left to jump to start of previous word, got col %d", got)
	}
}

func TestRawEditCtrlShiftRightSelectsWord(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("one two")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(0)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl | tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected ctrl+shift+right to create selection")
	}
	if got := updated.editSelectedText(); got != "one" {
		t.Fatalf("expected first word selected, got %q", got)
	}
}

func TestRawEditAltShiftLeftExpandsSelectionByWord(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("one two three")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(len([]rune("one two ")))

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt | tea.ModShift})
	updated := updatedAny.(model)
	if !updated.editHasSelection() {
		t.Fatal("expected alt+shift+left to create selection")
	}
	if got := updated.editSelectedText(); got != "two " {
		t.Fatalf("expected alt+shift+left to expand by word, got %q", got)
	}
}

func TestRawEditCtrlBackspaceDeletesPreviousWord(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("one two")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(len([]rune("one two")))

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "one " {
		t.Fatalf("expected ctrl+backspace to delete previous word, got %q", got)
	}
}

func TestRawEditAltDownMovesCurrentLine(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("a\nb\nc")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.CursorDown()
	m.textarea.SetCursorColumn(0)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a\nc\nb" {
		t.Fatalf("expected alt+down to move current line down, got %q", got)
	}
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected cursor to follow moved line, got row %d", got)
	}
}

func TestRawEditCtrlAltDownDuplicatesCurrentLine(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("a\nb\nc")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.CursorDown()
	m.textarea.SetCursorColumn(0)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt | tea.ModCtrl})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "a\nb\nb\nc" {
		t.Fatalf("expected ctrl+alt+down to duplicate line, got %q", got)
	}
}

func TestRawEditCtrlYRedoesLastEdit(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.Focus()
	m.textarea.SetValue("x")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(1)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	updated = updatedAny.(model)
	if got := updated.textarea.Value(); got != "x**bold text**" {
		t.Fatalf("expected ctrl+y to redo, got %q", got)
	}
}

func TestRawEditTypingUndoCoalesces(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("a")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(1)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	updated = updatedAny.(model)

	if got := updated.textarea.Value(); got != "a" {
		t.Fatalf("expected a single undo to remove continuous typing, got %q", got)
	}
}

func TestRawEditBackspaceUndoCoalesces(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("abcd")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(4)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated = updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	updated = updatedAny.(model)

	if got := updated.textarea.Value(); got != "abcd" {
		t.Fatalf("expected a single undo to restore continuous backspaces, got %q", got)
	}
}

func TestRawEditPasteCanUndo(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("ab")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(2)

	updatedAny, _ := m.Update(tea.PasteMsg{Content: "cd"})
	updated := updatedAny.(model)
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	updated = updatedAny.(model)

	if got := updated.textarea.Value(); got != "ab" {
		t.Fatalf("expected undo after paste to restore prior content, got %q", got)
	}
}

func TestRawEditAltYCopiesSelectedLine(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("line one\nline two")
	m.textarea.SetWidth(20)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()

	updatedAny, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	updated := updatedAny.(model)
	if updated.status != "Copied selected text" {
		t.Fatalf("unexpected status %q", updated.status)
	}
	if cmd == nil {
		t.Fatal("expected clipboard command on alt+y")
	}
}

func TestRawEditSearchFindsInSourceAndJumpsCursor(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha\nbeta\ngamma beta")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if !updated.showSearch {
		t.Fatal("expected ctrl+f to open search in raw mode")
	}
	updated.searchInput.SetValue("beta")
	updated.refreshSearchMatches()
	if got := len(updated.searchMatches); got != 2 {
		t.Fatalf("expected 2 search matches, got %d", got)
	}
	if updated.searchMatches[0].line != 1 || updated.searchMatches[1].line != 2 {
		t.Fatalf("unexpected search hit lines: %+v", updated.searchMatches)
	}
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if updated.searchIndex != 0 {
		t.Fatalf("expected search index to advance, got %d", updated.searchIndex)
	}
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected search jump to line 1, got %d", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected search jump to col 0, got %d", got)
	}
}

func TestRawEditSearchEscRestoresOriginalCursor(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha\nbeta\ngamma beta")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(2)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updated.searchInput.SetValue("beta")
	updated.refreshSearchMatches()

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected first search jump to line 1, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = updatedAny.(model)
	if updated.showSearch {
		t.Fatal("expected esc to close search")
	}
	if got := updated.textarea.Line(); got != 0 {
		t.Fatalf("expected esc to restore original line, got %d", got)
	}
	if got := updated.editCursorCol(); got != 2 {
		t.Fatalf("expected esc to restore original column, got %d", got)
	}
}

func TestRawEditSearchShiftEnterJumpsPrevious(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha\nbeta\ngamma beta")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updated.searchInput.SetValue("beta")
	updated.refreshSearchMatches()

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected shift+enter to jump to previous/wrapped match, got line %d", got)
	}
	if got := updated.editCursorCol(); got != 6 {
		t.Fatalf("expected shift+enter to place cursor at previous match col, got %d", got)
	}
}

func TestRawEditEnterContinuesTaskListPrefix(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("- [x] done")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()
	m.textarea.CursorEnd()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "- [x] done\n- [ ] " {
		t.Fatalf("expected task-list continuation prefix, got %q", got)
	}
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected cursor to move to next line, got %d", got)
	}
	if got := updated.editCursorCol(); got != 6 {
		t.Fatalf("expected cursor after inserted list prefix, got %d", got)
	}
}

func TestRawEditEnterContinuesListWithoutViewportJump(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true

	lines := make([]string, 220)
	for i := range lines {
		lines[i] = "- item " + strconv.Itoa(i)
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.textarea.SetWidth(60)
	m.textarea.SetHeight(8)

	targetRow := 180
	m.editSetCursor(targetRow, len([]rune(lines[targetRow])))
	_ = m.textarea.View()
	before := m.textarea.ScrollYOffset()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := updatedAny.(model)
	_ = updated.textarea.View()
	after := updated.textarea.ScrollYOffset()

	if got := updated.textarea.Line(); got != targetRow+1 {
		t.Fatalf("expected cursor to move to continuation line, got %d", got)
	}
	if got := updated.editCursorCol(); got != len([]rune("- ")) {
		t.Fatalf("expected cursor after inserted list prefix, got %d", got)
	}
	if got := updated.textarea.Value(); !strings.Contains(got, "- item 180\n- ") {
		t.Fatalf("expected continued list prefix after target item, got %q", got)
	}
	if abs(after-before) > 2 {
		t.Fatalf("expected stable scroll offset around cursor, before=%d after=%d", before, after)
	}
}

func TestRawEditEnterOnEmptyListItemExitsList(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("- item\n- ")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.editSetCursor(1, 2)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := updatedAny.(model)
	if got := updated.textarea.Value(); got != "- item\n" {
		t.Fatalf("expected empty list item to be removed, got %q", got)
	}
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected cursor to stay on exited line, got %d", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected cursor at column 0 after exit, got %d", got)
	}
}

func TestRawEditDoubleEnterListExitDoesNotJumpViewport(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true

	lines := make([]string, 220)
	for i := range lines {
		lines[i] = "- item " + strconv.Itoa(i)
	}
	content := strings.Join(lines, "\n")
	m.textarea.SetValue(content)
	m.textarea.SetWidth(60)
	m.textarea.SetHeight(8)

	targetRow := 180
	m.editSetCursor(targetRow, len([]rune(lines[targetRow])))

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := updatedAny.(model)
	updated.textarea.SetHeight(updated.textarea.Height())
	_ = updated.textarea.View()
	before := updated.textarea.ScrollYOffset()

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	_ = updated.textarea.View()
	after := updated.textarea.ScrollYOffset()

	if got := updated.textarea.Line(); got != targetRow+1 {
		t.Fatalf("expected cursor to stay on exited list line, got %d", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected cursor at line start after list exit, got %d", got)
	}
	if got := updated.textarea.Value(); !strings.Contains(got, "- item 180\n\n- item 181") {
		t.Fatalf("expected empty list line after double enter, got %q", got)
	}
	if before > 0 && abs(after-before) > 2 {
		t.Fatalf("expected stable scroll offset on second enter, before=%d after=%d", before, after)
	}
}

func TestRawEditSearchEnterStartsFromCursorPosition(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("beta one\nalpha\nbeta two\nbeta three")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	m.editSetCursor(2, 0)
	if got := m.textarea.Line(); got != 2 {
		t.Fatalf("expected setup cursor line 2, got %d", got)
	}

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updated.searchInput.SetValue("beta")
	updated.refreshSearchMatches()

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected first jump to nearest match at cursor, got line %d", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected first jump to nearest match column, got %d", got)
	}
}

func TestRawEditSearchJumpFlashesFocusLine(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha\nbeta\ngamma")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updated.searchInput.SetValue("beta")
	updated.refreshSearchMatches()
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if updated.editFocusLine != 1 {
		t.Fatalf("expected focus line flash on search jump, got %d", updated.editFocusLine)
	}
	gen := updated.editFocusLineGen
	updatedAny, _ = updated.Update(editFocusLineClearMsg{generation: gen})
	updated = updatedAny.(model)
	if updated.editFocusLine != -1 {
		t.Fatalf("expected focus line flash to clear, got %d", updated.editFocusLine)
	}
}

func TestApplyEditSearchHighlightsRawMode(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha beta\ngamma beta")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	m.showSearch = true
	m.searchInput.SetValue("beta")
	m.refreshSearchMatches()
	m.searchIndex = 1

	base := strings.Split(m.textarea.View(), "\n")
	got := m.applyEditSearchHighlights(base)
	baseJoined := strings.Join(base, "\n")
	gotJoined := strings.Join(got, "\n")
	if gotJoined == baseJoined {
		t.Fatal("expected search highlights to alter raw editor rows")
	}
	if xansi.Strip(gotJoined) != xansi.Strip(baseJoined) {
		t.Fatalf("expected highlight pass to preserve text content, base=%q got=%q", xansi.Strip(baseJoined), xansi.Strip(gotJoined))
	}
}

func TestApplyEditSearchHighlightsNoopWhenSearchHidden(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha beta\ngamma beta")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	m.showSearch = false
	m.searchInput.SetValue("beta")
	m.refreshSearchMatches()

	base := strings.Split(m.textarea.View(), "\n")
	got := m.applyEditSearchHighlights(base)
	if strings.Join(got, "\n") != strings.Join(base, "\n") {
		t.Fatal("expected no highlight changes when search overlay is hidden")
	}
}

func TestRawEditSearchOpenPrefillsSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("alpha beta gamma")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	lines := strings.Split(m.textarea.Value(), "\n")
	anchor := editRowColToRuneOffset(lines, 0, 6)
	cursor := editRowColToRuneOffset(lines, 0, 10)
	m.editSetSelectionOffsets(anchor, cursor)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if !updated.showSearch {
		t.Fatal("expected ctrl+f to open search")
	}
	if got := updated.searchInput.Value(); got != "beta" {
		t.Fatalf("expected selected text prefilled in search, got %q", got)
	}
	if got := len(updated.searchMatches); got != 1 {
		t.Fatalf("expected prefilled query to resolve matches, got %d", got)
	}
	if updated.searchIndex != -1 {
		t.Fatalf("expected search index to remain unset before first jump, got %d", updated.searchIndex)
	}
}

func TestRenderSearchBarShowsTotalBeforeFirstJump(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.textarea.SetValue("beta one\nalpha\nbeta two")
	m.searchInput.SetValue("beta")
	m.searchIndex = -1
	m.refreshSearchMatches()

	bar := stripANSI(m.renderSearchBar())
	if !strings.Contains(bar, "2 matches") {
		t.Fatalf("expected total match count before first jump, got %q", bar)
	}
	if strings.Contains(bar, "0 of 2 matches") {
		t.Fatalf("expected no zero-indexed count before first jump, got %q", bar)
	}
}

func TestRawEditCtrlHOpensReplaceBar(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("alpha beta")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if !updated.showSearch || !updated.showReplace {
		t.Fatal("expected ctrl+h to open search+replace")
	}
	if !updated.replaceInputFocus {
		t.Fatal("expected replace input to be focused")
	}
}

func TestRawEditReplaceEnterReplacesCurrentMatch(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("foo x foo")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updated.searchInput.SetValue("foo")
	updated.replaceInput.SetValue("bar")
	updated.searchIndex = -1
	updated.refreshSearchMatches()

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if got := updated.textarea.Value(); got != "bar x foo" {
		t.Fatalf("expected first match replaced on enter, got %q", got)
	}
}

func TestRawEditReplaceAllRespectsSelectionScope(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("foo one\nfoo two\nfoo three")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(8)

	lines := strings.Split(m.textarea.Value(), "\n")
	start := editRowColToRuneOffset(lines, 1, 0)
	end := editRowColToRuneOffset(lines, 1, len([]rune(lines[1])))
	m.editSetSelectionOffsets(start, end)

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	updated.searchInput.SetValue("foo")
	updated.replaceInput.SetValue("zap")
	updated.searchIndex = -1
	updated.refreshSearchMatches()

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	updated = updatedAny.(model)
	if got := updated.textarea.Value(); got != "foo one\nzap two\nfoo three" {
		t.Fatalf("expected ctrl+enter replace-all to honor selection scope, got %q", got)
	}
}

func TestRawEditCtrlLGoToLineColumnPrompt(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("one\ntwo\nthree\nfour")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(8)
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	updated := updatedAny.(model)
	if updated.opMode != opGoToLine {
		t.Fatalf("expected go-to-line prompt, got %v", updated.opMode)
	}
	updated.promptInput.SetValue("3:2")
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if updated.opMode != opNone {
		t.Fatalf("expected prompt to close after jump, got %v", updated.opMode)
	}
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected jump to line index 2, got %d", got)
	}
	if got := updated.editCursorCol(); got != 1 {
		t.Fatalf("expected jump to column index 1, got %d", got)
	}
}

func TestRawEditCtrlShiftLGoToHeadingPrompt(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("# Intro\ntext\n## Details\nbody")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(8)
	m.sourceContent = m.textarea.Value()
	m.updateOutlineFromEditor()
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl | tea.ModShift})
	updated := updatedAny.(model)
	if updated.opMode != opGoToHeading {
		t.Fatalf("expected go-to-heading prompt, got %v", updated.opMode)
	}
	updated.promptInput.SetValue("Details")
	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 2 {
		t.Fatalf("expected jump to heading line 2, got %d", got)
	}
	if got := updated.editCursorCol(); got != 0 {
		t.Fatalf("expected heading jump to column 0, got %d", got)
	}
}

func TestRawEditBlockMotionsParagraphAndHeading(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.textarea.SetValue("p1a\np1b\n\np2\n\n# H1\nx\n## H2\ny")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(10)
	m.sourceContent = m.textarea.Value()
	m.updateOutlineFromEditor()
	m.textarea.MoveToBegin()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModAlt})
	updated := updatedAny.(model)
	if got := updated.textarea.Line(); got != 3 {
		t.Fatalf("expected next paragraph at line 3, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: '[', Mod: tea.ModAlt})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 0 {
		t.Fatalf("expected previous paragraph at line 0, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 5 {
		t.Fatalf("expected next heading at line 5, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 7 {
		t.Fatalf("expected second next heading at line 7, got %d", got)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModAlt})
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 5 {
		t.Fatalf("expected previous heading at line 5, got %d", got)
	}
}

func TestNormalizeMarkdownForSave(t *testing.T) {
	input := "##Heading\n \t-  item\nText\n```go\nfmt.Println(\"x\")\n```\nNext"
	got := normalizeMarkdownForSave(input)
	if !strings.Contains(got, "## Heading") {
		t.Fatalf("expected heading spacing normalization, got %q", got)
	}
	if !strings.Contains(got, "    - item") {
		t.Fatalf("expected list indentation normalization, got %q", got)
	}
	if !strings.Contains(got, "Text\n\n```go") {
		t.Fatalf("expected blank line before code fence, got %q", got)
	}
	if !strings.Contains(got, "```\n\nNext") {
		t.Fatalf("expected blank line after code fence, got %q", got)
	}
}

func TestApplyFormatOnSaveIfEnabled(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()
	m.formatOnSave = true
	m.textarea.SetValue("##Heading\n-  item")
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(6)
	m.sourceContent = m.textarea.Value()

	if changed := m.applyFormatOnSaveIfEnabled(); !changed {
		t.Fatal("expected format-on-save to modify markdown")
	}
	if got := m.sourceContent; !strings.Contains(got, "## Heading") {
		t.Fatalf("expected heading normalized in source, got %q", got)
	}
}

func TestRunCommandActionZoomReset(t *testing.T) {
	m := testModelNoWatcher()
	m.zoom = 1.7

	updatedAny, _ := m.runCommandAction("zoom_reset")
	updated := updatedAny.(model)
	if updated.zoom != 1.0 {
		t.Fatalf("expected zoom reset to 1.0, got %.2f", updated.zoom)
	}
	if updated.status != "Zoom 100%" {
		t.Fatalf("unexpected status %q", updated.status)
	}
}

func TestRunCommandActionToggleSectionGauge(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.showGauge = true
	m.width = 120
	m.height = 30
	m.resizeViews()

	updatedAny, _ := m.runCommandAction("toggle_section_gauge")
	updated := updatedAny.(model)
	if updated.showGauge {
		t.Fatal("expected toggle_section_gauge to disable gauge")
	}
	if updated.status != "Section gauge off" {
		t.Fatalf("unexpected status %q", updated.status)
	}
}

func TestPreviewKeyTogglesSectionGauge(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.focusRight = true
	m.showGauge = true
	m.width = 120
	m.height = 30
	m.resizeViews()

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	updated := updatedAny.(model)
	if updated.showGauge {
		t.Fatal("expected u to toggle gauge off")
	}
	if updated.status != "Section gauge off" {
		t.Fatalf("unexpected status %q", updated.status)
	}
}

func TestRunCommandActionPasteClipboardSetsTarget(t *testing.T) {
	m := testModelNoWatcher()
	m.showSearch = true

	updatedAny, cmd := m.runCommandAction("paste_clipboard")
	updated := updatedAny.(model)
	if updated.pendingClipboardTarget != clipboardTargetSearch {
		t.Fatalf("expected search clipboard target, got %v", updated.pendingClipboardTarget)
	}
	if cmd == nil {
		t.Fatal("expected clipboard read command")
	}
}

func TestRunCommandActionCopyFilePath(t *testing.T) {
	m := testModelNoWatcher()
	m.dir = "/tmp/work"
	m.currentPath = "/tmp/work/docs/readme.md"

	updatedAny, cmd := m.runCommandAction("copy_file_path")
	updated := updatedAny.(model)
	if updated.status != "Copied path to clipboard" {
		t.Fatalf("unexpected status %q", updated.status)
	}
	if cmd == nil {
		t.Fatal("expected clipboard command")
	}
}

func TestRunCommandActionCopyHeadingLink(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.dir = "/tmp/work"
	m.currentPath = "/tmp/work/docs/readme.md"
	m.headings = []headingItem{{title: "Intro Section"}}
	m.currentHeading = 0

	updatedAny, cmd := m.runCommandAction("copy_heading_link")
	updated := updatedAny.(model)
	if updated.status != "Copied heading link" {
		t.Fatalf("unexpected status %q", updated.status)
	}
	if cmd == nil {
		t.Fatal("expected clipboard command")
	}
}

func TestRunCommandActionCopySelectedText(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetValue("line one\nline two")
	_ = m.textarea.Focus()

	updatedAny, cmd := m.runCommandAction("copy_selected_text")
	updated := updatedAny.(model)
	if updated.status != "Copied selected text" {
		t.Fatalf("unexpected status %q", updated.status)
	}
	if cmd == nil {
		t.Fatal("expected clipboard command")
	}
}

func TestRunCommandActionCopyCommandOutput(t *testing.T) {
	m := testModelNoWatcher()
	m.status = "Operation complete"

	updatedAny, cmd := m.runCommandAction("copy_command_output")
	updated := updatedAny.(model)
	if updated.status != "Copied command output" {
		t.Fatalf("unexpected status %q", updated.status)
	}
	if cmd == nil {
		t.Fatal("expected clipboard command")
	}
}

func TestRunCommandActionSuspendShell(t *testing.T) {
	m := testModelNoWatcher()
	updatedAny, cmd := m.runCommandAction("suspend_shell")
	_ = updatedAny.(model)
	if cmd == nil {
		t.Fatal("expected suspend command")
	}
}

func TestRunCommandActionRefreshTerminalFeatures(t *testing.T) {
	m := testModelNoWatcher()
	m.capProbeRequested = false

	updatedAny, cmd := m.runCommandAction("refresh_terminal_features")
	updated := updatedAny.(model)
	if !updated.capProbeRequested {
		t.Fatal("expected capability probe flag to be set")
	}
	if cmd == nil {
		t.Fatal("expected terminal refresh command")
	}
}

func TestTargetEditLineUsesPreviewAlignmentInsteadOfHeadingStart(t *testing.T) {
	m := testModelNoWatcher()
	m.sourceContent = strings.Repeat("x\n", 79) + "x"
	m.previewYOffset = 15
	m.currentHeading = 0
	m.headings = []headingItem{
		{line: 10, renderLine: 10},
		{line: 20, renderLine: 20},
	}
	m.headingRenderLines = []int{10, 20}
	m.headingRenderIndices = []int{0, 1}

	got := m.targetEditLine()
	if got != 15 {
		t.Fatalf("expected preview-aligned target line 15, got %d", got)
	}
}

func TestTargetEditLineInterpolatesWhenRenderedSectionIsExpanded(t *testing.T) {
	m := testModelNoWatcher()
	m.sourceContent = strings.Repeat("x\n", 119) + "x"
	m.previewYOffset = 50
	m.headings = []headingItem{
		{line: 10, renderLine: 20},
		{line: 30, renderLine: 80},
	}
	m.headingRenderLines = []int{20, 80}
	m.headingRenderIndices = []int{0, 1}

	got := m.targetEditLine()
	if got != 20 {
		t.Fatalf("expected interpolated source line 20, got %d", got)
	}
}

func TestTargetEditLineUsesPreviewTopAnchor(t *testing.T) {
	// The anchor is the TOP of the preview viewport (previewYOffset), not the
	// bottom. With previewYOffset=40 the heading interpolation maps render
	// line 40 → source line 16 (between headings at renderLine 20/srcLine 10
	// and renderLine 80/srcLine 30: delta=20/span=60 → 10+(20*20/60)=16).
	m := testModelNoWatcher()
	m.sourceContent = strings.Repeat("x\n", 199) + "x"
	m.previewYOffset = 40
	m.viewport.SetHeight(20)
	m.headings = []headingItem{
		{line: 10, renderLine: 20},
		{line: 30, renderLine: 80},
	}
	m.headingRenderLines = []int{20, 80}
	m.headingRenderIndices = []int{0, 1}

	got := m.targetEditLine()
	if got != 16 {
		t.Fatalf("expected top-anchor mapped line 16, got %d", got)
	}
}

func TestTargetEditPositionMatchesTopPreviewText(t *testing.T) {
	// The anchor text is taken from the TOP of the viewport. "alpha" is
	// the first visible line and must be found in the source at line 0.
	m := testModelNoWatcher()
	m.sourceContent = strings.Join([]string{
		"alpha",
		"bravo",
		"charlie delta echo",
		"foxtrot",
	}, "\n")
	m.previewYOffset = 0
	m.viewportLines = []string{
		"alpha",
		"bravo",
		"delta echo",
	}

	line, col := m.targetEditPosition()
	if line != 0 {
		t.Fatalf("expected top-anchor matched source line 0, got %d", line)
	}
	if col != 0 {
		t.Fatalf("expected column 0 for start-of-line match, got %d", col)
	}
}

func TestPositionEditorCursorUsesTopAnchor(t *testing.T) {
	// With previewYOffset=40 and the top-anchor, targetEditLine returns 16.
	// positionEditorCursor should place the cursor at line 16 (at the top of
	// the editor viewport, not the bottom).
	m := testModelNoWatcher()
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "text"
	}
	lines[10] = "# A"
	lines[30] = "# B"
	m.sourceContent = strings.Join(lines, "\n")
	m.previewYOffset = 40
	m.viewport.SetHeight(20)
	m.headings = []headingItem{
		{title: "A", line: 10, renderLine: 20},
		{title: "B", line: 30, renderLine: 80},
	}
	m.headingRenderLines = []int{20, 80}
	m.headingRenderIndices = []int{0, 1}
	_ = m.textarea.Focus()
	m.textarea.SetWidth(80)
	m.textarea.SetHeight(20)

	m.positionEditorCursor()
	if got := m.textarea.Line(); got != 16 {
		t.Fatalf("expected editor cursor at line 16 (top-anchor), got %d", got)
	}
}

func TestRenderedMsgReprocessesViewportWhenReadingModeChanges(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.perfVisualMode = perfVisualForceOff
	m.focusMode = true
	m.readingMode = false
	m.currentHeading = 0
	m.headings = []headingItem{{title: "A"}}

	content := "intro\nA\nbody"
	msg := renderedMsg{
		content:     content,
		yOffset:     1,
		width:       80,
		annotated:   content,
		cacheKey:    content,
		readingMode: false,
		styleName:   m.styleName,
	}

	updatedAny, _ := m.Update(msg)
	updated := updatedAny.(model)
	focusContent := strings.Join(updated.viewportLines, "\n")
	if !strings.Contains(focusContent, updated.styles.sgrDimPrefix+"intro") {
		t.Fatalf("expected focus dim prefix in initial viewport content, got %q", focusContent)
	}

	updated.readingMode = true
	msg.readingMode = true
	updatedAny, _ = updated.Update(msg)
	updated = updatedAny.(model)
	readingContent := strings.Join(updated.viewportLines, "\n")
	if !strings.Contains(readingContent, updated.styles.sgrReadingDim+"intro") {
		t.Fatalf("expected reading dim prefix after mode change, got %q", readingContent)
	}
	if strings.Contains(readingContent, updated.styles.sgrDimPrefix+"intro") {
		t.Fatalf("expected focus dim prefix to be replaced after mode change, got %q", readingContent)
	}
}

func TestApplyHighlightSkipsLeadingPadding(t *testing.T) {
	const prefix = "\x1b[48;2;1;2;3m\x1b[38;2;4;5;6m"
	got := applyHighlight("    heading\nnext", 0, prefix)
	wantStart := "    " + prefix + "heading"
	if !strings.HasPrefix(got, wantStart) {
		t.Fatalf("expected highlight after leading spaces; got %q", got)
	}
	if strings.HasPrefix(got, prefix+"    ") {
		t.Fatalf("expected leading padding to remain unhighlighted; got %q", got)
	}
}

func TestHelpShortcutsContainSections(t *testing.T) {
	items := helpShortcuts()
	sectionCount := 0
	for _, it := range items {
		if it.section {
			sectionCount++
		}
	}
	if sectionCount < 3 {
		t.Fatalf("expected multiple help sections, got %d", sectionCount)
	}
}

func TestAllCommandsIncludesToggleSectionGaugeHotkey(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	cmds := allCommands(&m)

	var found command
	ok := false
	for _, c := range cmds {
		if c.action == "toggle_section_gauge" {
			found = c
			ok = true
			break
		}
	}
	if !ok {
		t.Fatal("expected toggle_section_gauge command in command palette")
	}
	if found.keys != "u" {
		t.Fatalf("expected hotkey u for toggle_section_gauge, got %q", found.keys)
	}
	if found.visible == nil || !found.visible(&m) {
		t.Fatal("expected toggle_section_gauge command to be visible in preview mode")
	}

	m.mode = modeRaw
	if found.visible(&m) {
		t.Fatal("expected toggle_section_gauge command hidden outside preview mode")
	}
}

func TestAllCommandsIncludeCurrentFocusPaneShortcuts(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	cmds := allCommands(&m)

	var found command
	ok := false
	for _, c := range cmds {
		if c.action == "toggle_focus_pane" {
			found = c
			ok = true
			break
		}
	}
	if !ok {
		t.Fatal("expected toggle_focus_pane command in command palette")
	}
	if found.keys != "tab, shift+tab, ctrl+tab" {
		t.Fatalf("expected current focus pane shortcuts, got %q", found.keys)
	}
}

func TestAllCommandsUsePaletteLabelForRecentFileCycling(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.recentFiles = []string{"a.md", "b.md"}
	cmds := allCommands(&m)

	want := map[string]string{
		"next_buffer": "Next recent file",
		"prev_buffer": "Previous recent file",
	}
	for action, name := range want {
		found := false
		for _, c := range cmds {
			if c.action != action {
				continue
			}
			found = true
			if c.name != name {
				t.Fatalf("expected %s to be named %q, got %q", action, name, c.name)
			}
			if c.keys != "palette" {
				t.Fatalf("expected %s to be palette-only, got %q", action, c.keys)
			}
			break
		}
		if !found {
			t.Fatalf("expected %s command in command palette", action)
		}
	}
}

func TestHelpShortcutsIncludeSectionGaugeHotkey(t *testing.T) {
	items := helpShortcuts()
	for _, it := range items {
		if it.keys == "u" && it.desc == "Toggle section gauge" {
			return
		}
	}
	t.Fatal("expected help shortcuts to include 'u' for toggling section gauge")
}

func TestHelpShortcutsIncludeCommandPaletteHotkey(t *testing.T) {
	items := helpShortcuts()
	for _, it := range items {
		if it.keys == "ctrl+k" && it.desc == "Open/close command palette" {
			return
		}
	}
	t.Fatal("expected help shortcuts to include ctrl+k for the command palette")
}

func TestHelpShortcutsDoNotListStaleBufferShortcut(t *testing.T) {
	items := helpShortcuts()
	for _, it := range items {
		if strings.Contains(it.keys, "ctrl+shift+tab") {
			t.Fatalf("expected help shortcuts to omit stale buffer shortcut, got %q", it.keys)
		}
	}
}

func TestPasteMsgUpdatesSearchInput(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.showSearch = true
	_ = m.searchInput.Focus()

	updatedAny, _ := m.Update(tea.PasteMsg{Content: "alpha\nbeta"})
	updated := updatedAny.(model)
	if updated.searchInput.Value() != "alpha beta" {
		t.Fatalf("expected collapsed search paste, got %q", updated.searchInput.Value())
	}
}

func TestPasteMsgUpdatesPromptInput(t *testing.T) {
	m := testModelNoWatcher()
	m.opMode = opCreate
	_ = m.promptInput.Focus()

	updatedAny, _ := m.Update(tea.PasteMsg{Content: "new-file.md\nignored"})
	updated := updatedAny.(model)
	if updated.promptInput.Value() != "new-file.md" {
		t.Fatalf("expected first line paste, got %q", updated.promptInput.Value())
	}
}

func TestClipboardMsgPastesIntoCommandPaletteFilter(t *testing.T) {
	m := testModelNoWatcher()
	m.showCmdPalette = true
	m.filteredCommands = m.filterCommands("")
	m.pendingClipboardTarget = clipboardTargetCmdPalette

	updatedAny, _ := m.Update(tea.ClipboardMsg{Content: "zoom"})
	updated := updatedAny.(model)
	if updated.cmdFilter != "zoom" {
		t.Fatalf("expected command filter paste, got %q", updated.cmdFilter)
	}
	if updated.pendingClipboardTarget != clipboardTargetNone {
		t.Fatalf("expected clipboard target reset, got %v", updated.pendingClipboardTarget)
	}
}

func TestBackgroundColorMsgAppliesFollowSystemThemeFromTerminal(t *testing.T) {
	m := testModelNoWatcher()
	m.followSystem = true
	m.styleName = defaultStyleName

	updatedAny, _ := m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})
	updated := updatedAny.(model)
	if !updated.terminalBgKnown {
		t.Fatal("expected terminal background color to be marked known")
	}
	if updated.styleName != styles.LightStyle {
		t.Fatalf("expected light style from light terminal background, got %q", updated.styleName)
	}
}

func TestColorProfileMsgDisablesAdvancedRenderingOnLowProfile(t *testing.T) {
	m := testModelNoWatcher()
	m.richPreview = true

	updatedAny, _ := m.Update(tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	updated := updatedAny.(model)
	if updated.supportsAdvancedRendering() {
		t.Fatal("expected advanced rendering to be disabled for ASCII profile")
	}
	if updated.effectiveRichPreview() {
		t.Fatal("expected effective rich preview to be off on low color profile")
	}
}

func TestLayerHitMsgClosesOverlays(t *testing.T) {
	m := testModelNoWatcher()
	m.showHelp = true

	updatedAny, _ := m.Update(overlayLayerHitMsg{
		id:    layerIDHelpOverlay,
		mouse: tea.MouseClickMsg{Button: tea.MouseLeft},
	})
	updated := updatedAny.(model)
	if updated.showHelp {
		t.Fatal("expected help overlay to close on layer hit click")
	}
}

func TestResumeMsgRequestsTerminalProbes(t *testing.T) {
	m := testModelNoWatcher()

	updatedAny, cmd := m.Update(tea.ResumeMsg{})
	updated := updatedAny.(model)
	if updated.status != "Resumed from shell" {
		t.Fatalf("unexpected status %q", updated.status)
	}
	if !updated.capProbeRequested {
		t.Fatal("expected capability probe to be marked requested on resume")
	}
	if cmd == nil {
		t.Fatal("expected probe command batch on resume")
	}
}

func TestProgramMsgFilterBlocksSuspendDuringSearch(t *testing.T) {
	m := testModelNoWatcher()
	m.showSearch = true

	msg := programMsgFilter(m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if msg != nil {
		t.Fatal("expected suspend hotkey to be filtered while search is open")
	}
}

func TestProgramMsgFilterBlocksQuitDuringOpPrompt(t *testing.T) {
	m := testModelNoWatcher()
	m.opMode = opCreate

	msg := programMsgFilter(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if msg != nil {
		t.Fatal("expected quit hotkey to be filtered during operation prompt")
	}
}

func TestRenderCmdPaletteStaysWithinViewportWidth(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 68
	m.height = 24
	m.showCmdPalette = true
	m.headings = []headingItem{{title: strings.Repeat("Very long heading title ", 4)}}
	m.filteredCommands = m.filterCommands("")
	m.cmdIdx = 0

	out := m.renderCmdPalette()
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d width %d exceeds viewport %d", i, w, m.width)
		}
	}
}

func TestViewSetsWindowTitle(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 80
	m.height = 24
	m.mode = modeRaw
	m.currentPath = "/tmp/project/note.md"

	v := m.View()
	if !strings.Contains(v.WindowTitle, "note.md - markdownviewer") {
		t.Fatalf("expected file in title, got %q", v.WindowTitle)
	}
	if !strings.Contains(v.WindowTitle, "Raw Edit") {
		t.Fatalf("expected mode in title, got %q", v.WindowTitle)
	}
}

func TestViewUsesLayerContentForOverlays(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 80
	m.height = 24
	m.showHelp = true

	v := m.View()
	if v.OnMouse == nil {
		t.Fatal("expected overlay view to install an OnMouse hit handler")
	}
	cmd := v.OnMouse(tea.MouseClickMsg{Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("expected overlay OnMouse handler to return a command")
	}
	msg := cmd()
	hit, ok := msg.(overlayLayerHitMsg)
	if !ok {
		t.Fatalf("expected overlay layer hit message, got %T", msg)
	}
	if hit.id != layerIDHelpOverlay {
		t.Fatalf("expected help overlay layer id, got %q", hit.id)
	}
}

func TestRenderHelpOverlayStaysWithinViewportWidth(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 72
	m.height = 24

	out := m.renderHelpOverlay()
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d width %d exceeds viewport %d", i, w, m.width)
		}
	}
}

func TestHelpOverlayColumnCount(t *testing.T) {
	if got := helpOverlayColumnCount(70, 6); got != 1 {
		t.Fatalf("expected 1 column on narrow width, got %d", got)
	}
	if got := helpOverlayColumnCount(100, 6); got < 2 {
		t.Fatalf("expected multi-column layout on medium width, got %d", got)
	}
	if got := helpOverlayColumnCount(140, 8); got != 3 {
		t.Fatalf("expected 3 columns on wide width, got %d", got)
	}
}

func TestCommandPaletteWindow(t *testing.T) {
	tests := []struct {
		total, selected, maxShow int
		wantStart, wantEnd       int
	}{
		{total: 0, selected: 0, maxShow: 10, wantStart: 0, wantEnd: 0},
		{total: 5, selected: 2, maxShow: 10, wantStart: 0, wantEnd: 5},
		{total: 20, selected: 0, maxShow: 10, wantStart: 0, wantEnd: 10},
		{total: 20, selected: 8, maxShow: 10, wantStart: 3, wantEnd: 13},
		{total: 20, selected: 19, maxShow: 10, wantStart: 10, wantEnd: 20},
		{total: 20, selected: 999, maxShow: 10, wantStart: 10, wantEnd: 20},
	}
	for _, tc := range tests {
		start, end := commandPaletteWindow(tc.total, tc.selected, tc.maxShow)
		if start != tc.wantStart || end != tc.wantEnd {
			t.Fatalf("window(%d,%d,%d) got (%d,%d) want (%d,%d)",
				tc.total, tc.selected, tc.maxShow, start, end, tc.wantStart, tc.wantEnd)
		}
	}
}

func TestRenderCmdPaletteShowsSelectedWindow(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 80
	m.height = 24
	m.showCmdPalette = true
	m.mode = modePreview
	for i := 1; i <= 12; i++ {
		m.headings = append(m.headings, headingItem{title: "Heading"})
	}
	m.filteredCommands = m.filterCommands("")
	if len(m.filteredCommands) < 10 {
		t.Fatalf("expected command list longer than one page, got %d", len(m.filteredCommands))
	}
	m.cmdIdx = len(m.filteredCommands) - 1

	out := m.renderCmdPalette()
	if !strings.Contains(out, "Jump to heading 9") || !strings.Contains(out, "▸") {
		t.Fatalf("expected end-of-list heading entry to be visible/selected; output=%q", out)
	}
}

func TestCommandPalettePageNavigation(t *testing.T) {
	m := testModelNoWatcher()
	m.showCmdPalette = true
	m.mode = modePreview
	for i := 0; i < 12; i++ {
		m.headings = append(m.headings, headingItem{title: "Heading"})
	}
	m.filteredCommands = m.filterCommands("")
	m.cmdIdx = 0

	updatedAny, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	updated := updatedAny.(model)
	if updated.cmdIdx == 0 {
		t.Fatalf("expected pgdown to advance selection, still at %d", updated.cmdIdx)
	}

	updatedAny, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	updated = updatedAny.(model)
	if updated.cmdIdx != 0 {
		t.Fatalf("expected pgup to return selection to start, got %d", updated.cmdIdx)
	}
}

func TestChromeRowsWithoutQuickActionsFooter(t *testing.T) {
	m := testModelNoWatcher()
	m.height = 40
	m.mode = modePreview

	if got := m.chromeRows(); got != 1 {
		t.Fatalf("expected base chrome rows to be 1 (status only), got %d", got)
	}

	m.showSearch = true
	if got := m.chromeRows(); got != 2 {
		t.Fatalf("expected chrome rows with search to be 2, got %d", got)
	}

	m.opMode = opCreate
	if got := m.chromeRows(); got != 3 {
		t.Fatalf("expected chrome rows with search+prompt to be 3, got %d", got)
	}
}

func TestRenderBreadcrumbOutlineKeepsCurrentHeadingVisible(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 52
	m.palette = paletteForStyle(defaultStyleName)
	for i := 1; i <= 15; i++ {
		m.headings = append(m.headings, headingItem{title: "Section " + strconv.Itoa(i)})
	}
	m.currentHeading = 12

	out := m.renderBreadcrumbOutline()
	if !strings.Contains(out, "13. Section 13") {
		t.Fatalf("expected current heading to be visible in breadcrumb, got %q", out)
	}
}

func TestStatusBarCacheInvalidatesOnBreadcrumbChange(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 96
	m.height = 24
	m.mode = modePreview
	m.fullScreen = true
	m.layoutCache = &layoutCache{breadcrumbHdg: -1}
	m.resizeViews()

	m.breadcrumb = "alpha"
	_ = m.renderLayout()
	first := m.layoutCache.statusStr
	if !strings.Contains(first, "alpha") {
		t.Fatalf("expected initial status bar to include breadcrumb, got %q", first)
	}

	m.breadcrumb = "beta"
	_ = m.renderLayout()
	second := m.layoutCache.statusStr
	if first == second || !strings.Contains(second, "beta") {
		t.Fatalf("expected status bar cache refresh for breadcrumb change; first=%q second=%q", first, second)
	}
}

func TestRenderStatusBarShowsEditCaretAndSelection(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.mode = modeRaw
	m.focusRight = true
	m.sourceContent = "alpha\nbeta"
	m.textarea.SetValue(m.sourceContent)
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.editSetCursor(1, 2)

	line := stripANSI(m.renderStatusBar())
	if !strings.Contains(line, "Ln 2, Col 3") {
		t.Fatalf("expected caret status in status bar, got %q", line)
	}

	lines := strings.Split(m.textarea.Value(), "\n")
	anchor := editRowColToRuneOffset(lines, 1, 1)
	cursor := editRowColToRuneOffset(lines, 1, 3)
	m.editSetSelectionOffsets(anchor, cursor)
	line = stripANSI(m.renderStatusBar())
	if !strings.Contains(line, "Sel 2ch/1ln") {
		t.Fatalf("expected selection summary in status bar, got %q", line)
	}
}

func TestStatusBarCacheInvalidatesOnEditCaretChange(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 96
	m.height = 24
	m.mode = modeRaw
	m.focusRight = true
	m.fullScreen = true
	m.layoutCache = &layoutCache{breadcrumbHdg: -1}
	m.sourceContent = "abc\ndef"
	m.resizeViews()
	m.textarea.SetValue(m.sourceContent)
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(5)
	m.textarea.MoveToBegin()

	_ = m.renderLayout()
	first := m.layoutCache.statusStr
	m.textarea.SetCursorColumn(2)
	_ = m.renderLayout()
	second := m.layoutCache.statusStr
	if first == second {
		t.Fatalf("expected status bar cache refresh for caret movement; first=%q second=%q", first, second)
	}
}

func TestScrollTextareaLinesTreatsWrappedMovementAsProgress(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeRaw
	m.focusRight = true
	m.textarea.SetWidth(6)
	m.textarea.SetHeight(4)
	m.textarea.SetValue("abcdefghijklmnopqrstuvwxyz")
	m.textarea.MoveToBegin()
	m.textarea.SetCursorColumn(0)

	stuck := m.scrollTextareaLines(1, false)
	if stuck {
		t.Fatal("expected wrapped line movement to not be treated as boundary")
	}
}

func TestSplitSyncScrollTracksEditorViewportTop(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeSplit
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.sourceContent = strings.Join(lines, "\n")
	m.textarea.SetValue(m.sourceContent)
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(8)
	m.viewport.SetHeight(8)
	m.renderedLineCount = 400

	const editorTop = 72
	offset, ok := m.splitPreviewOffsetForEditorTop(editorTop)
	if !ok {
		t.Fatal("expected split preview offset calculation to succeed")
	}
	if offset != editorTop {
		t.Fatalf("expected split preview to follow editor viewport top %d, got %d", editorTop, offset)
	}
}

func TestSplitSyncScrollAnchorsBottomAtDocumentEnd(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeSplit
	m.focusRight = true
	m.width = 120
	m.height = 30
	m.resizeViews()

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.sourceContent = strings.Join(lines, "\n")
	m.textarea.SetValue(m.sourceContent)
	m.textarea.SetWidth(40)
	m.textarea.SetHeight(8)
	m.viewport.SetHeight(8)
	m.renderedLineCount = 220
	m.headings = []headingItem{
		{line: 70, renderLine: 70},
		{line: 99, renderLine: 190},
	}

	offset, ok := m.splitPreviewOffsetForEditorTop(92)
	if !ok {
		t.Fatal("expected split preview offset calculation to succeed")
	}
	if offset != 183 {
		t.Fatalf("expected split preview to anchor to bottom-visible content near EOF, got %d", offset)
	}
}

func TestMouseWheelPreviewScrollUsesSingleStepWithoutMomentum(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.resizeViews()

	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.viewport.SetYOffset(10)
	m.previewYOffset = 10
	m.scrollMomentum = true
	m.scrollVelocity = 2.5
	m.scrollAccum = 0.75

	updated := m
	for i := range 3 {
		updatedAny, cmd := updated.Update(tea.MouseWheelMsg{
			X:      updated.listWidth(),
			Y:      5,
			Button: tea.MouseWheelDown,
		})
		if cmd != nil {
			t.Fatal("expected wheel scroll not to start momentum command")
		}
		updated = updatedAny.(model)
		if got := updated.previewYOffset; got != 10 {
			t.Fatalf("expected preview wheel scroll to stay put until accumulated threshold, event=%d got=%d", i+1, got)
		}
	}

	updatedAny, cmd := updated.Update(tea.MouseWheelMsg{
		X:      updated.listWidth(),
		Y:      5,
		Button: tea.MouseWheelDown,
	})
	if cmd != nil {
		t.Fatal("expected wheel scroll not to start momentum command")
	}
	updated = updatedAny.(model)
	if got := updated.previewYOffset; got != 11 {
		t.Fatalf("expected preview wheel scroll to move by one line after accumulation, got %d", got)
	}
	if updated.scrollMomentum {
		t.Fatal("expected wheel scroll to leave momentum disabled")
	}
	if updated.scrollVelocity != 0 || updated.scrollAccum != 0 {
		t.Fatalf("expected wheel scroll to clear momentum state, vel=%v accum=%v", updated.scrollVelocity, updated.scrollAccum)
	}
}

func TestMouseWheelRawScrollUsesSingleStepWithoutMomentum(t *testing.T) {
	m := setupRawMouseTestModel()
	lines := make([]string, 120)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.sourceContent = strings.Join(lines, "\n")
	m.textarea.SetValue(m.sourceContent)
	m.textarea.SetHeight(8)
	m.textarea.SetWidth(50)
	m.textarea.MoveToBegin()
	for range 20 {
		m.textarea.CursorDown()
	}
	m.scrollMomentum = true
	m.scrollVelocity = 1.8
	m.scrollAccum = 0.5

	updated := m
	for i := range 3 {
		updatedAny, cmd := updated.Update(tea.MouseWheelMsg{
			X:      updated.listWidth(),
			Y:      5,
			Button: tea.MouseWheelDown,
		})
		if cmd != nil {
			t.Fatal("expected raw wheel scroll not to start momentum command")
		}
		updated = updatedAny.(model)
		if got := updated.textarea.Line(); got != 20 {
			t.Fatalf("expected raw wheel scroll to stay put until accumulated threshold, event=%d got=%d", i+1, got)
		}
	}

	updatedAny, cmd := updated.Update(tea.MouseWheelMsg{
		X:      updated.listWidth(),
		Y:      5,
		Button: tea.MouseWheelDown,
	})
	if cmd != nil {
		t.Fatal("expected raw wheel scroll not to start momentum command")
	}
	updated = updatedAny.(model)
	if got := updated.textarea.Line(); got != 21 {
		t.Fatalf("expected raw wheel scroll to move cursor by one line after accumulation, got %d", got)
	}
	if updated.scrollMomentum {
		t.Fatal("expected raw wheel scroll to leave momentum disabled")
	}
	if updated.scrollVelocity != 0 || updated.scrollAccum != 0 {
		t.Fatalf("expected raw wheel scroll to clear momentum state, vel=%v accum=%v", updated.scrollVelocity, updated.scrollAccum)
	}
}

func TestMouseWheelPreviewLeavesBoundaryImmediatelyOnReverse(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.resizeViews()

	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.viewport.SetYOffset(0)
	m.previewYOffset = 0

	for range 3 {
		updatedAny, _ := m.Update(tea.MouseWheelMsg{
			X:      m.listWidth(),
			Y:      5,
			Button: tea.MouseWheelUp,
		})
		m = updatedAny.(model)
	}
	if m.previewYOffset != 0 {
		t.Fatalf("expected preview to remain at top boundary, got %d", m.previewYOffset)
	}

	updatedAny, _ := m.Update(tea.MouseWheelMsg{
		X:      m.listWidth(),
		Y:      5,
		Button: tea.MouseWheelDown,
	})
	updated := updatedAny.(model)
	if got := updated.previewYOffset; got != 1 {
		t.Fatalf("expected reverse wheel at top boundary to move immediately, got %d", got)
	}
}

func TestMouseWheelRawLeavesBoundaryImmediatelyOnReverse(t *testing.T) {
	m := setupRawMouseTestModel()
	lines := make([]string, 120)
	for i := range lines {
		lines[i] = "line " + strconv.Itoa(i)
	}
	m.sourceContent = strings.Join(lines, "\n")
	m.textarea.SetValue(m.sourceContent)
	m.textarea.SetHeight(8)
	m.textarea.SetWidth(50)
	m.textarea.MoveToBegin()
	_ = m.textarea.View()

	for range 3 {
		updatedAny, _ := m.Update(tea.MouseWheelMsg{
			X:      m.listWidth(),
			Y:      5,
			Button: tea.MouseWheelUp,
		})
		m = updatedAny.(model)
	}
	if got := m.textarea.Line(); got != 0 {
		t.Fatalf("expected raw editor to remain at top boundary, got line %d", got)
	}

	updatedAny, _ := m.Update(tea.MouseWheelMsg{
		X:      m.listWidth(),
		Y:      5,
		Button: tea.MouseWheelDown,
	})
	updated := updatedAny.(model)
	if got := updated.textarea.Line(); got != 1 {
		t.Fatalf("expected reverse wheel at raw top boundary to move immediately, got line %d", got)
	}
}
