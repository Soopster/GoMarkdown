package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestBuildNavigatorTreeRecursesAndSkipsIgnoredDirs(t *testing.T) {
	tmp := t.TempDir()
	mustWriteFile(t, filepath.Join(tmp, "root.md"), "# root\n")
	mustWriteFile(t, filepath.Join(tmp, "notes", "nested.md"), "# nested\n")
	mustWriteFile(t, filepath.Join(tmp, "notes", "ignore.txt"), "nope\n")
	mustWriteFile(t, filepath.Join(tmp, ".git", "hidden.md"), "# hidden\n")
	mustWriteFile(t, filepath.Join(tmp, "node_modules", "pkg.md"), "# pkg\n")

	root, err := buildNavigatorTree(tmp)
	if err != nil {
		t.Fatalf("buildNavigatorTree error: %v", err)
	}

	dirPath := filepath.Join(tmp, "notes")
	items := buildNavigatorItems(root, map[string]bool{dirPath: true})
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.path)
	}

	if !containsPath(paths, filepath.Join(tmp, "root.md")) {
		t.Fatalf("expected root file in navigator: %v", paths)
	}
	if !containsPath(paths, dirPath) {
		t.Fatalf("expected notes dir in navigator: %v", paths)
	}
	if !containsPath(paths, filepath.Join(tmp, "notes", "nested.md")) {
		t.Fatalf("expected nested markdown file in navigator: %v", paths)
	}
	if containsPath(paths, filepath.Join(tmp, ".git", "hidden.md")) {
		t.Fatalf("did not expect ignored .git file in navigator: %v", paths)
	}
	if containsPath(paths, filepath.Join(tmp, "node_modules", "pkg.md")) {
		t.Fatalf("did not expect ignored node_modules file in navigator: %v", paths)
	}
}

func TestHandleNavigatorKeyPressExpandsDirAndOpensNestedFile(t *testing.T) {
	tmp := t.TempDir()
	nestedPath := filepath.Join(tmp, "notes", "nested.md")
	mustWriteFile(t, nestedPath, "# nested\n")

	m := testModelNoWatcher()
	m.dir = tmp
	root, err := buildNavigatorTree(tmp)
	if err != nil {
		t.Fatalf("buildNavigatorTree error: %v", err)
	}
	m.fileTree = root
	m.fileTreeExpanded = make(map[string]bool)
	m.rebuildNavigatorItems(filepath.Join(tmp, "notes"))

	selected, ok := m.selectedNavigatorItem()
	if !ok || selected.kind != navigatorDirNode {
		t.Fatalf("expected initial selection to be the top-level directory, got %#v", selected)
	}

	expanded, _, handled := m.handleNavigatorKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("expected enter to be handled by navigator")
	}
	if !expanded.fileTreeExpanded[filepath.Join(tmp, "notes")] {
		t.Fatal("expected enter to expand selected directory")
	}

	opened, cmd, handled := expanded.handleNavigatorKeyPress(tea.KeyPressMsg{Code: tea.KeyRight})
	if !handled {
		t.Fatal("expected right to be handled by navigator")
	}
	if opened.currentPath != nestedPath {
		t.Fatalf("expected right to open nested file %q, got %q", nestedPath, opened.currentPath)
	}
	if cmd == nil {
		t.Fatal("expected opening nested file to return a load command")
	}
}

func TestFilesRefreshKeepsCurrentFileWhenFolderSelected(t *testing.T) {
	tmp := t.TempDir()
	rootPath := filepath.Join(tmp, "root.md")
	dirPath := filepath.Join(tmp, "notes")
	nestedPath := filepath.Join(dirPath, "nested.md")
	mustWriteFile(t, rootPath, "# root\n")
	mustWriteFile(t, nestedPath, "# nested\n")

	m := testModelNoWatcher()
	m.dir = tmp
	m.currentPath = rootPath
	m.loadedPath = rootPath
	m.fileTreeExpanded = map[string]bool{dirPath: true}
	root, err := buildNavigatorTree(tmp)
	if err != nil {
		t.Fatalf("buildNavigatorTree error: %v", err)
	}
	m.fileTree = root
	m.rebuildNavigatorItems(dirPath)

	if selected, ok := m.selectedNavigatorItem(); !ok || selected.kind != navigatorDirNode {
		t.Fatalf("expected folder selection before refresh, got %#v", selected)
	}

	updated, cmd, handled := m.handleAsyncUpdate(filesRefreshedMsg{root: root})
	if !handled {
		t.Fatal("expected filesRefreshedMsg to be handled")
	}
	if updated.currentPath != rootPath {
		t.Fatalf("expected refresh to keep current file %q, got %q", rootPath, updated.currentPath)
	}
	if cmd != nil {
		t.Fatal("expected navigator refresh to avoid reloading unchanged current file")
	}
}

func TestFilesRefreshReloadsCurrentFileWhenRequested(t *testing.T) {
	tmp := t.TempDir()
	rootPath := filepath.Join(tmp, "root.md")
	mustWriteFile(t, rootPath, "# root\n")

	m := testModelNoWatcher()
	m.dir = tmp
	m.currentPath = rootPath
	m.loadedPath = rootPath
	root, err := buildNavigatorTree(tmp)
	if err != nil {
		t.Fatalf("buildNavigatorTree error: %v", err)
	}

	updated, cmd, handled := m.handleAsyncUpdate(filesRefreshedMsg{root: root, reloadCurrent: true})
	if !handled {
		t.Fatal("expected filesRefreshedMsg to be handled")
	}
	if updated.currentPath != rootPath {
		t.Fatalf("expected refresh to keep current file %q, got %q", rootPath, updated.currentPath)
	}
	if cmd == nil {
		t.Fatal("expected explicit refresh to reload the current file")
	}
	msg := cmd()
	loaded, ok := msg.(fileLoadedMsg)
	if !ok {
		t.Fatalf("expected file load message, got %T", msg)
	}
	if !sameFile(loaded.path, rootPath) {
		t.Fatalf("expected explicit refresh to load %q, got %q", rootPath, loaded.path)
	}
}

func TestRenderLayoutShowsTreeEntriesInFilesPane(t *testing.T) {
	tmp := t.TempDir()
	nestedPath := filepath.Join(tmp, "notes", "nested.md")
	mustWriteFile(t, nestedPath, "# nested\n")

	m := testModelNoWatcher()
	m.dir = tmp
	m.width = 100
	m.height = 24
	m.showOutline = false
	m.fullScreen = false
	root, err := buildNavigatorTree(tmp)
	if err != nil {
		t.Fatalf("buildNavigatorTree error: %v", err)
	}
	m.fileTree = root
	m.fileTreeExpanded = map[string]bool{filepath.Join(tmp, "notes"): true}
	m.rebuildNavigatorItems(nestedPath)
	m.resizeViews()
	m.setViewportContent("nested")

	layout := xansi.Strip(m.renderLayout())
	if !strings.Contains(layout, "notes/") {
		t.Fatalf("expected rendered layout to include tree directory, got %q", layout)
	}
	if !strings.Contains(layout, "nested.md") {
		t.Fatalf("expected rendered layout to include nested file, got %q", layout)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if sameFile(path, want) {
			return true
		}
	}
	return false
}
