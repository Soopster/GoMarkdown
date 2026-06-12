package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
)

const (
	fileFinderMaxResults        = 50
	linkScanMaxLines            = 50
	workspaceSearchContextLimit = 120
	workspaceSearchMaxHits      = 100
	recentFilesLimit            = 20
	splitRenderDelaySmall       = 75 * time.Millisecond
	splitRenderDelayMedium      = 110 * time.Millisecond
	splitRenderDelayLarge       = 140 * time.Millisecond
	splitRenderDelayXLarge      = 180 * time.Millisecond
)

func themePreview(pal colorPalette) string {
	blocks := []string{pal.bg, pal.highlight, pal.text}
	var parts []string
	for _, c := range blocks {
		parts = append(parts, lipgloss.NewStyle().
			Background(lipgloss.Color(c)).
			Foreground(lipgloss.Color(pal.text)).
			Render("   "))
	}
	return strings.Join(parts, "")
}

func themeLabel(name string) string {
	switch name {
	case defaultStyleName:
		return "Default"
	case styles.LightStyle:
		return "Light"
	case styles.DarkStyle:
		return "Dark"
	case styles.DraculaStyle:
		return "Dracula"
	case styles.PinkStyle:
		return "Pink"
	case styles.TokyoNightStyle:
		return "Tokyo Night"
	case catppuccinStyle:
		return "Catppuccin"
	case nordStyle:
		return "Nord"
	case gruvboxStyle:
		return "Gruvbox"
	case solarizedDarkStyle:
		return "Solarized Dark"
	case solarizedLightStyle:
		return "Solarized Light"
	case everforestStyle:
		return "Everforest"
	case kanagawaStyle:
		return "Kanagawa"
	default:
		return name
	}
}

func buildNavigatorTree(dir string) (*navigatorNode, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	root := &navigatorNode{
		kind:    navigatorDirNode,
		name:    filepath.Base(dir),
		path:    dir,
		modTime: info.ModTime(),
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	children := make([]*navigatorNode, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipNavigatorDir(name) {
				continue
			}
			child, err := buildNavigatorTree(filepath.Join(dir, name))
			if err != nil || len(child.children) == 0 {
				continue
			}
			children = append(children, child)
			continue
		}
		if !isMarkdown(entry) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		children = append(children, &navigatorNode{
			kind:    navigatorFileNode,
			name:    name,
			path:    filepath.Join(dir, name),
			modTime: info.ModTime(),
		})
	}
	sortNavigatorChildren(children)
	root.children = children
	return root, nil
}

func shouldSkipNavigatorDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".obsidian":
		return true
	default:
		return false
	}
}

func sortNavigatorChildren(children []*navigatorNode) {
	sort.Slice(children, func(i, j int) bool {
		if children[i].kind != children[j].kind {
			return children[i].kind == navigatorDirNode
		}
		return strings.ToLower(children[i].name) < strings.ToLower(children[j].name)
	})
}

func navigatorFileNameCounts(root *navigatorNode) map[string]int {
	counts := make(map[string]int)
	var walk func(*navigatorNode)
	walk = func(node *navigatorNode) {
		if node == nil {
			return
		}
		if node.kind == navigatorFileNode {
			counts[strings.ToLower(node.name)]++
			return
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(root)
	return counts
}

func buildNavigatorItems(root *navigatorNode, expanded map[string]bool) []navigatorItem {
	if root == nil {
		return nil
	}
	counts := navigatorFileNameCounts(root)
	items := make([]navigatorItem, 0)
	var walk func(children []*navigatorNode, depth int)
	walk = func(children []*navigatorNode, depth int) {
		for _, child := range children {
			item := navigatorItem{
				kind:     child.kind,
				name:     child.name,
				path:     child.path,
				depth:    depth,
				expanded: expanded[child.path],
				modTime:  child.modTime,
			}
			if child.kind == navigatorFileNode && counts[strings.ToLower(child.name)] > 1 {
				if rel, err := filepath.Rel(root.path, child.path); err == nil {
					dir := filepath.Dir(rel)
					if dir != "." {
						item.relDir = dir
					}
				}
			}
			items = append(items, item)
			if child.kind == navigatorDirNode && expanded[child.path] {
				walk(child.children, depth+1)
			}
		}
	}
	walk(root.children, 0)
	return items
}

func toListItems(files []navigatorItem) []list.Item {
	out := make([]list.Item, 0, len(files))
	for _, f := range files {
		out = append(out, f)
	}
	return out
}

func isMarkdown(entry fs.DirEntry) bool {
	ext := strings.ToLower(filepath.Ext(entry.Name()))
	_, ok := markdownExts[ext]
	return ok
}

func toOutlineItems(headings []headingItem) []list.Item {
	items := make([]list.Item, 0, len(headings))
	for _, h := range headings {
		items = append(items, h)
	}
	return items
}

func indexForPath(files []navigatorItem, path string) int {
	for i, f := range files {
		if sameFile(f.path, path) {
			return i
		}
	}
	return -1
}

func (m model) navigatorItems() []navigatorItem {
	items := m.fileList.Items()
	out := make([]navigatorItem, 0, len(items))
	for _, item := range items {
		if nav, ok := item.(navigatorItem); ok {
			out = append(out, nav)
		}
	}
	return out
}

func (m model) selectedNavigatorItem() (navigatorItem, bool) {
	item, ok := m.fileList.SelectedItem().(navigatorItem)
	return item, ok
}

func (m model) selectedNavigatorPath() string {
	if item, ok := m.selectedNavigatorItem(); ok {
		return item.path
	}
	return ""
}

func (m *model) expandNavigatorAncestors(path string) {
	if path == "" {
		return
	}
	current := filepath.Clean(path)
	root := filepath.Clean(m.dir)
	for {
		parent := filepath.Dir(current)
		if parent == current || sameFile(parent, root) {
			return
		}
		m.fileTreeExpanded[parent] = true
		current = parent
	}
}

func (m *model) rebuildNavigatorItems(selectedPath string) {
	items := buildNavigatorItems(m.fileTree, m.fileTreeExpanded)
	m.fileList.SetItems(toListItems(items))
	if len(items) == 0 {
		return
	}
	idx := indexForPath(items, selectedPath)
	if idx < 0 && m.currentPath != "" {
		idx = indexForPath(items, m.currentPath)
	}
	if idx < 0 {
		idx = 0
	}
	m.fileList.Select(idx)
}

func (m *model) syncNavigatorSelection(path string) {
	if path == "" || m.fileTree == nil {
		return
	}
	m.expandNavigatorAncestors(path)
	m.rebuildNavigatorItems(path)
}

func (m *model) toggleNavigatorDir(path string) {
	if path == "" {
		return
	}
	if m.fileTreeExpanded[path] {
		delete(m.fileTreeExpanded, path)
	} else {
		m.fileTreeExpanded[path] = true
	}
	m.rebuildNavigatorItems(path)
}

func headingIndexForSlug(headings []headingItem, anchor string) int {
	anchor = slugify(anchor)
	if anchor == "" {
		return -1
	}
	for i, h := range headings {
		if slugify(h.title) == anchor {
			return i
		}
	}
	return -1
}

func parseHeadings(content string) []headingItem {
	lines := strings.Split(content, "\n")
	out := make([]headingItem, 0)
	for i, line := range lines {
		matches := reHeading.FindStringSubmatch(line)
		if len(matches) == 3 {
			level := len(matches[1])
			title := strings.TrimSpace(matches[2])
			out = append(out, headingItem{
				title: title,
				level: level,
				line:  i,
			})
		}
	}
	return out
}

func (m *model) setOutline(headings []headingItem, resetSelection bool) {
	prevRows := m.breadcrumbRows()
	built := buildOutlineItems(headings)
	m.headings = built
	m.outline.SetItems(toOutlineItems(built))
	if resetSelection {
		if len(built) > 0 {
			m.setCurrentHeading(0)
		} else {
			m.setCurrentHeading(-1)
		}
	}
	m.updateBreadcrumb()
	if m.breadcrumbRows() != prevRows {
		m.resizeViews()
	}
}

func (m *model) setCurrentHeading(idx int) {
	m.currentHeading = idx
	if idx >= 0 && idx < len(m.headings) {
		m.outline.Select(idx)
	}
}

func (m *model) ensureOutlineSelection() {
	target := m.currentHeading
	if target < 0 && len(m.headings) > 0 {
		target = 0
	}
	if target >= 0 && target < len(m.headings) {
		m.outline.Select(target)
	}
}

func buildOutlineItems(headings []headingItem) []headingItem {
	if len(headings) == 0 {
		return nil
	}
	lastFlags := make([]bool, len(headings))
	for i := range headings {
		lastFlags[i] = isLastAtLevel(i, headings)
	}
	out := make([]headingItem, 0, len(headings))
	for i, h := range headings {
		prefix := treePrefix(i, headings, lastFlags)
		h.display = prefix + h.title
		h.renderLine = -1
		out = append(out, h)
	}
	return out
}

func isLastAtLevel(idx int, headings []headingItem) bool {
	level := headings[idx].level
	for j := idx + 1; j < len(headings); j++ {
		if headings[j].level == level {
			return false
		}
		if headings[j].level < level {
			return true
		}
	}
	return true
}

func treePrefix(idx int, headings []headingItem, lastFlags []bool) string {
	level := headings[idx].level
	if level <= 1 {
		if lastFlags[idx] {
			return "└╴"
		}
		return "├╴"
	}
	var parts []string
	for l := 1; l < level; l++ {
		ancestorIdx := findAncestorIndex(idx, l, headings)
		if ancestorIdx >= 0 && !lastFlags[ancestorIdx] {
			parts = append(parts, "┊ ")
		} else {
			parts = append(parts, "  ")
		}
	}
	if lastFlags[idx] {
		parts = append(parts, "└╴")
	} else {
		parts = append(parts, "├╴")
	}
	return strings.Join(parts, "")
}

func findAncestorIndex(idx, level int, headings []headingItem) int {
	for i := idx - 1; i >= 0; i-- {
		if headings[i].level == level {
			return i
		}
	}
	return -1
}

func headingIndexAtOffset(renderLines []int, renderIndices []int, y int) int {
	if len(renderLines) == 0 || len(renderIndices) == 0 {
		return -1
	}
	limit := len(renderLines)
	if len(renderIndices) < limit {
		limit = len(renderIndices)
	}
	pos := sort.Search(limit, func(i int) bool {
		return renderLines[i] > y
	}) - 1
	if pos < 0 || pos >= limit {
		return -1
	}
	return renderIndices[pos]
}

func currentHeadingIndex(headings []headingItem, y int) int {
	best := -1
	bestLine := -1
	for i, h := range headings {
		if h.renderLine >= 0 && h.renderLine <= y && h.renderLine >= bestLine {
			bestLine = h.renderLine
			best = i
		}
	}
	return best
}

func headingIndexForSourceLine(headings []headingItem, line int) int {
	best := -1
	bestLine := -1
	for i, h := range headings {
		if h.line <= line && h.line >= bestLine {
			bestLine = h.line
			best = i
		}
	}
	return best
}

func headingIndexForTitle(headings []headingItem, title string) int {
	title = strings.TrimSpace(strings.ToLower(title))
	for i, h := range headings {
		if strings.TrimSpace(strings.ToLower(h.title)) == title {
			return i
		}
	}
	return -1
}

func parseGoToLineColInput(input string) (line int, col int, ok bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, 0, false
	}
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ':' || r == ',' || unicode.IsSpace(r)
	})
	if len(parts) == 0 || len(parts) > 2 {
		return 0, 0, false
	}
	lineNum, err := strconv.Atoi(parts[0])
	if err != nil || lineNum <= 0 {
		return 0, 0, false
	}
	colNum := 1
	if len(parts) == 2 {
		colNum, err = strconv.Atoi(parts[1])
		if err != nil || colNum <= 0 {
			return 0, 0, false
		}
	}
	return lineNum - 1, colNum - 1, true
}

func headingIndexForQuery(headings []headingItem, query string) int {
	query = strings.TrimSpace(query)
	if query == "" {
		return -1
	}
	if n, err := strconv.Atoi(query); err == nil {
		idx := n - 1
		if idx >= 0 && idx < len(headings) {
			return idx
		}
	}
	if idx := headingIndexForTitle(headings, query); idx >= 0 {
		return idx
	}
	qLower := strings.ToLower(query)
	for i, h := range headings {
		if strings.Contains(strings.ToLower(h.title), qLower) {
			return i
		}
	}
	return -1
}

func (m *model) startCreate() {
	m.opMode = opCreate
	m.opTarget = ""
	m.promptInput.SetValue("")
	m.promptInput.Focus()
	m.status = "Create file"
	m.resizeViews()
}

func (m *model) startRename() {
	if m.currentPath == "" {
		return
	}
	m.opMode = opRename
	m.opTarget = m.currentPath
	m.promptInput.SetValue(filepath.Base(m.currentPath))
	m.promptInput.CursorEnd()
	m.promptInput.Focus()
	m.status = fmt.Sprintf("Rename %s", filepath.Base(m.currentPath))
	m.resizeViews()
}

func (m *model) startDelete() {
	if m.currentPath == "" {
		return
	}
	m.opMode = opDeleteConfirm
	m.opTarget = m.currentPath
	m.status = fmt.Sprintf("Delete %s? (y/n)", filepath.Base(m.currentPath))
	m.resizeViews()
}

func (m *model) startGoToLine() {
	if m.mode != modeRaw {
		return
	}
	m.opMode = opGoToLine
	m.opTarget = ""
	m.promptInput.SetValue(fmt.Sprintf("%d:%d", m.textarea.Line()+1, m.editCursorCol()+1))
	m.promptInput.CursorEnd()
	m.promptInput.Focus()
	m.status = "Go to line"
	m.resizeViews()
}

func (m *model) startGoToHeading() {
	if m.mode != modeRaw {
		return
	}
	m.opMode = opGoToHeading
	m.opTarget = ""
	m.promptInput.SetValue("")
	m.promptInput.Focus()
	m.status = "Go to heading"
	m.resizeViews()
}

func (m *model) clearOp() {
	m.opMode = opNone
	m.opTarget = ""
	m.promptInput.SetValue("")
	m.promptInput.Blur()
	m.resizeViews()
}

func (m model) createFileCmd(name string) tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		path := filepath.Join(dir, name)
		err := os.WriteFile(path, []byte(""), 0644)
		return fileCreatedMsg{path: path, err: err}
	}
}

func (m model) renameFileCmd(oldPath, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath := filepath.Join(filepath.Dir(oldPath), newName)
		err := os.Rename(oldPath, newPath)
		return fileRenamedMsg{oldPath: oldPath, newPath: newPath, err: err}
	}
}

func (m model) deleteFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		err := os.Remove(path)
		return fileDeletedMsg{path: path, err: err}
	}
}

func (m model) handleOpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.opMode {
	case opDeleteConfirm:
		switch msg.String() {
		case "y":
			if m.opTarget != "" {
				cmd := m.deleteFileCmd(m.opTarget)
				m.clearOp()
				return m, cmd
			}
		case "n", "esc":
			m.status = "Delete cancelled"
			m.clearOp()
			return m, nil
		}
		return m, nil
	case opCreate, opRename:
		switch msg.String() {
		case "esc":
			m.status = "Cancelled"
			m.clearOp()
			return m, nil
		case "enter":
			name := strings.TrimSpace(m.promptInput.Value())
			if name == "" {
				m.status = m.styles.statusWarn.Render("Name required")
				return m, nil
			}
			if fileExists(filepath.Join(m.dir, name)) {
				m.status = m.styles.statusWarn.Render("File exists")
				return m, nil
			}
			if m.opMode == opCreate {
				cmd := m.createFileCmd(name)
				m.clearOp()
				return m, cmd
			}
			if m.opMode == opRename && m.opTarget != "" {
				cmd := m.renameFileCmd(m.opTarget, name)
				m.clearOp()
				return m, cmd
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(msg)
			return m, cmd
		}
	case opGoToLine:
		switch msg.String() {
		case "esc":
			m.status = "Cancelled"
			m.clearOp()
			return m, nil
		case "enter":
			line, col, ok := parseGoToLineColInput(m.promptInput.Value())
			if !ok {
				m.status = m.styles.statusWarn.Render("Use line[:col], e.g. 12:3")
				return m, nil
			}
			m.clearOp()
			m.status = fmt.Sprintf("Line %d", line+1)
			return m, m.editJumpToPosition(line, col)
		default:
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(msg)
			return m, cmd
		}
	case opGoToHeading:
		switch msg.String() {
		case "esc":
			m.status = "Cancelled"
			m.clearOp()
			return m, nil
		case "enter":
			if len(m.headings) == 0 {
				m.status = m.styles.statusWarn.Render("No headings")
				return m, nil
			}
			idx := headingIndexForQuery(m.headings, m.promptInput.Value())
			if idx < 0 || idx >= len(m.headings) {
				m.status = m.styles.statusWarn.Render("Heading not found")
				return m, nil
			}
			target := m.headings[idx]
			m.clearOp()
			m.status = fmt.Sprintf("Heading: %s", target.title)
			return m, m.editJumpToPosition(target.line, 0)
		default:
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(msg)
			return m, cmd
		}
	default:
		return m, nil
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (m *model) updateBreadcrumb() {
	var parts []string
	if m.dir != "" {
		parts = append(parts, filepath.Base(m.dir))
	}
	if m.currentPath != "" {
		if rel, err := filepath.Rel(m.dir, m.currentPath); err == nil {
			parts = append(parts, rel)
		} else {
			parts = append(parts, filepath.Base(m.currentPath))
		}
	}
	if m.currentHeading >= 0 && m.currentHeading < len(m.headings) {
		parts = append(parts, m.headings[m.currentHeading].title)
	}
	m.breadcrumb = strings.Join(parts, " › ")
}

func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ap, _ := filepath.Abs(a)
	bp, _ := filepath.Abs(b)
	return ap == bp
}

func programMsgFilter(tm tea.Model, msg tea.Msg) tea.Msg {
	m, ok := tm.(model)
	if !ok {
		return msg
	}
	keyMsg, ok := keyPressFromMsg(msg)
	if !ok {
		return msg
	}
	switch keyMsg.String() {
	case "q", "ctrl+c":
		// In modal file-operation prompts, force explicit cancel/confirm keys.
		if m.opMode != opNone {
			return nil
		}
	case "ctrl+o":
		// Avoid accidental suspend while actively entering input.
		if m.opMode != opNone || m.showSearch || m.showCmdPalette || m.showHelp || m.showThemePicker || m.showFileFinder || m.showContentSearch {
			return nil
		}
	}
	return msg
}

// ─── A2: Auto-save ───────────────────────────────────────────

func (m *model) scheduleAutoSave() tea.Cmd {
	if !m.autoSave {
		return nil
	}
	m.autoSaveGen++
	gen := m.autoSaveGen
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return autoSaveTickMsg{generation: gen}
	})
}

// ─── A3: Fuzzy file finder ──────────────────────────────────

func (m model) walkDirCmd() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		var files []string
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if shouldSkipNavigatorDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".md" || ext == ".markdown" || ext == ".mdown" || ext == ".mkd" || ext == ".mdx" {
				rel, err := filepath.Rel(dir, path)
				if err == nil {
					files = append(files, rel)
				}
			}
			return nil
		})
		sort.Strings(files)
		return walkDirResultMsg{files: files}
	}
}

func fuzzyMatch(candidates []string, query string) []fileFinderEntry {
	if query == "" {
		results := make([]fileFinderEntry, 0, min(fileFinderMaxResults, len(candidates)))
		for i, c := range candidates {
			if i >= fileFinderMaxResults {
				break
			}
			results = append(results, fileFinderEntry{path: c, score: 0})
		}
		return results
	}
	qLower := strings.ToLower(query)
	var results []fileFinderEntry
	for _, candidate := range candidates {
		cLower := strings.ToLower(candidate)
		score, matchIdxs := fuzzyScore(cLower, qLower, candidate)
		if score > 0 {
			results = append(results, fileFinderEntry{
				path:       candidate,
				score:      score,
				matchRunes: matchIdxs,
			})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if len(results) > fileFinderMaxResults {
		results = results[:fileFinderMaxResults]
	}
	return results
}

func fuzzyScore(candidate, query, original string) (int, []int) {
	cRunes := []rune(candidate)
	qRunes := []rune(query)
	if len(qRunes) == 0 {
		return 1, nil
	}
	if len(qRunes) > len(cRunes) {
		return 0, nil
	}
	matchIdxs := make([]int, 0, len(qRunes))
	score := 0
	ci := 0
	prevMatch := -1
	basename := strings.ToLower(filepath.Base(original))
	basenameBonus := 0
	if strings.Contains(basename, string(qRunes)) {
		basenameBonus = 50
	}
	for _, qr := range qRunes {
		found := false
		for ci < len(cRunes) {
			if cRunes[ci] == qr {
				matchIdxs = append(matchIdxs, ci)
				score += 10
				// Contiguity bonus
				if prevMatch >= 0 && ci == prevMatch+1 {
					score += 15
				}
				// Start of word bonus
				if ci == 0 || cRunes[ci-1] == '/' || cRunes[ci-1] == '-' || cRunes[ci-1] == '_' || cRunes[ci-1] == '.' {
					score += 10
				}
				prevMatch = ci
				ci++
				found = true
				break
			}
			ci++
		}
		if !found {
			return 0, nil
		}
	}
	score += basenameBonus
	return score, matchIdxs
}

// ─── B1: Internal link navigation ───────────────────────────

func slugify(s string) string {
	s = strings.ToLower(s)
	var out strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' {
			out.WriteByte('-')
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func (m *model) followLinkAtCursor() tea.Cmd {
	if m.mode != modePreview || len(m.headings) == 0 {
		return nil
	}
	// Try to find link at current heading context by checking source
	heading := m.currentHeading
	if heading < 0 || heading >= len(m.headings) {
		return nil
	}
	hLine := m.headings[heading].line
	lines := strings.Split(m.sourceContent, "\n")
	if hLine >= len(lines) {
		return nil
	}
	// Scan lines near current heading for links
	start := hLine
	end := len(lines)
	if heading+1 < len(m.headings) {
		end = m.headings[heading+1].line
	}
	if end > start+linkScanMaxLines {
		end = start + linkScanMaxLines
	}
	reLink := regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
	reWiki := regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	for i := start; i < end; i++ {
		line := lines[i]
		// Check markdown links
		if matches := reLink.FindStringSubmatch(line); len(matches) == 3 {
			target := matches[2]
			return m.navigateToLink(target)
		}
		// Check wiki links
		if matches := reWiki.FindStringSubmatch(line); len(matches) == 2 {
			return m.navigateToWikiLink(matches[1])
		}
	}
	m.status = "No link found near heading"
	return nil
}

func (m *model) navigateToLink(target string) tea.Cmd {
	// Anchor link
	if strings.HasPrefix(target, "#") {
		anchor := slugify(target[1:])
		for i, h := range m.headings {
			if slugify(h.title) == anchor {
				m.setCurrentHeading(i)
				return m.jumpToHeading(h.title)
			}
		}
		m.status = fmt.Sprintf("Anchor not found: %s", target)
		return nil
	}
	// Cross-file link
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(m.currentPath), resolved)
	}
	// Strip anchor from cross-file link
	anchor := ""
	if idx := strings.Index(resolved, "#"); idx >= 0 {
		anchor = resolved[idx+1:]
		resolved = resolved[:idx]
	}
	if _, err := os.Stat(resolved); err != nil {
		m.status = fmt.Sprintf("File not found: %s", target)
		return nil
	}
	// Push current state
	m.navStack = append(m.navStack, navEntry{
		path:    m.currentPath,
		yOffset: m.viewport.YOffset(),
		heading: m.currentHeading,
	})
	m.currentPath = resolved
	m.previewYOffset = 0
	m.pendingLinkAnchor = anchor
	return m.loadFileCmd(resolved)
}

func (m *model) navigateToWikiLink(name string) tea.Cmd {
	nameLower := strings.ToLower(name)
	for _, f := range m.fileFinderAll {
		baseLower := strings.ToLower(strings.TrimSuffix(filepath.Base(f), filepath.Ext(f)))
		if baseLower == nameLower {
			resolved := filepath.Join(m.dir, f)
			m.navStack = append(m.navStack, navEntry{
				path:    m.currentPath,
				yOffset: m.viewport.YOffset(),
				heading: m.currentHeading,
			})
			m.currentPath = resolved
			m.previewYOffset = 0
			return m.loadFileCmd(resolved)
		}
	}
	m.status = fmt.Sprintf("Wiki link not found: [[%s]]", name)
	return nil
}

// ─── B2: Split preview ──────────────────────────────────────

func (m *model) scheduleSplitRender() tea.Cmd {
	m.splitRenderGen++
	gen := m.splitRenderGen
	delay := m.splitRenderDebounceDelay()
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return splitRenderDebounceMsg{generation: gen}
	})
}

func (m model) splitRenderDebounceDelay() time.Duration {
	// Keep split preview responsive for small/medium docs while backing off
	// slightly for very large buffers to avoid render thrash.
	lines := strings.Count(m.sourceContent, "\n") + 1
	switch {
	case lines < 200:
		return splitRenderDelaySmall
	case lines < 1200:
		return splitRenderDelayMedium
	case lines < 4000:
		return splitRenderDelayLarge
	default:
		return splitRenderDelayXLarge
	}
}

func (m *model) splitSyncScroll() {
	if m.mode != modeSplit {
		return
	}
	offset, ok := m.splitPreviewOffsetForEditorTop(m.textarea.ScrollYOffset())
	if !ok {
		return
	}
	if offset == m.previewYOffset && offset == m.viewport.YOffset() {
		return
	}
	m.viewport.SetYOffset(offset)
	m.previewYOffset = offset
}

func (m model) splitPreviewOffsetForEditorTop(topDisplayRow int) (int, bool) {
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) <= 1 {
		return 0, true
	}
	totalRendered := max(1, m.renderedLineCount)
	viewportHeight := max(1, m.viewport.Height())
	maxOffset := max(0, totalRendered-viewportHeight)
	if maxOffset == 0 {
		return 0, true
	}
	editorHeight, totalEditorRows, maxEditorOffset := m.editScrollMetrics()
	if editorHeight <= 0 {
		editorHeight = max(1, m.textarea.Height())
	}
	topDisplayRow = max(0, min(topDisplayRow, maxEditorOffset))
	topSourceLine, _, ok := editDisplayToSourcePoint(lines, max(1, m.textarea.Width()), topDisplayRow, 0)
	if !ok {
		return 0, false
	}
	offset := m.renderLineForSourceLine(topSourceLine)
	if totalEditorRows > 0 {
		bottomDisplayRow := min(totalEditorRows-1, topDisplayRow+editorHeight-1)
		if bottomDisplayRow >= totalEditorRows-1 {
			bottomSourceLine, _, ok := editDisplayToSourcePoint(lines, max(1, m.textarea.Width()), bottomDisplayRow, 0)
			if ok {
				bottomOffset := m.renderLineForSourceLine(bottomSourceLine) - (viewportHeight - 1)
				offset = max(offset, bottomOffset)
			}
		}
	}
	offset = max(0, min(maxOffset, offset))
	return offset, true
}

// ─── C1: Section folding ────────────────────────────────────

func (m model) applyFolding(content string) string {
	if len(m.foldedSections) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	var out []string
	i := 0
	headingIdx := 0
	for i < len(lines) {
		// Check if current line is a heading
		if headingIdx < len(m.headings) && i == m.headings[headingIdx].line {
			if m.foldedSections[headingIdx] {
				// Output the heading line with fold indicator
				level := m.headings[headingIdx].level
				out = append(out, lines[i])
				// Find end of this section (next heading of same or higher level)
				foldEnd := len(lines)
				hiddenCount := 0
				for j := headingIdx + 1; j < len(m.headings); j++ {
					if m.headings[j].level <= level {
						foldEnd = m.headings[j].line
						break
					}
				}
				hiddenCount = foldEnd - i - 1
				if hiddenCount > 0 {
					out = append(out, fmt.Sprintf("*▸ %d lines hidden*", hiddenCount))
				}
				i = foldEnd
				// Advance headingIdx past folded headings
				for headingIdx < len(m.headings) && m.headings[headingIdx].line < foldEnd {
					headingIdx++
				}
				continue
			}
			headingIdx++
		} else {
			// Advance headingIdx if we passed its line
			for headingIdx < len(m.headings) && m.headings[headingIdx].line <= i {
				headingIdx++
			}
		}
		out = append(out, lines[i])
		i++
	}
	return strings.Join(out, "\n")
}

// ─── C2: Content search across files ────────────────────────

func (m model) searchAcrossFilesCmd(query string, gen int) tea.Cmd {
	files := make([]string, len(m.fileFinderAll))
	copy(files, m.fileFinderAll)
	dir := m.dir
	return func() tea.Msg {
		qLower := strings.ToLower(query)
		var hits []contentSearchHit
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, f := range files {
			wg.Add(1)
			go func(rel string) {
				defer wg.Done()
				fullPath := filepath.Join(dir, rel)
				data, err := os.ReadFile(fullPath)
				if err != nil {
					return
				}
				lines := strings.Split(string(data), "\n")
				for lineNum, line := range lines {
					lineLower := strings.ToLower(line)
					idx := strings.Index(lineLower, qLower)
					if idx >= 0 {
						ctx := strings.TrimSpace(line)
						if len(ctx) > workspaceSearchContextLimit {
							ctx = ctx[:workspaceSearchContextLimit] + "…"
						}
						mu.Lock()
						hits = append(hits, contentSearchHit{
							path:       rel,
							line:       lineNum,
							context:    ctx,
							matchStart: idx,
							matchEnd:   idx + len(query),
						})
						if len(hits) >= workspaceSearchMaxHits {
							mu.Unlock()
							return
						}
						mu.Unlock()
					}
				}
			}(f)
		}
		wg.Wait()
		return contentSearchResultMsg{hits: hits, generation: gen}
	}
}

// ─── C3: Frontmatter parsing ────────────────────────────────

func parseFrontmatter(content string) (map[string]string, int) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "+++\n") {
		return nil, 0
	}
	delim := "---"
	if content[0] == '+' {
		delim = "+++"
	}
	// Find closing delimiter
	rest := content[4:] // skip opening delimiter + newline
	endIdx := strings.Index(rest, "\n"+delim+"\n")
	if endIdx < 0 {
		// Try end of file
		if strings.HasSuffix(rest, "\n"+delim) {
			endIdx = len(rest) - len(delim) - 1
		} else {
			return nil, 0
		}
	}
	fmContent := rest[:endIdx]
	lineCount := strings.Count(content[:4+endIdx+len(delim)+1], "\n") + 1
	// Simple key: value parsing
	fm := make(map[string]string)
	for _, line := range strings.Split(fmContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sepIdx := strings.Index(line, ":")
		if sepIdx < 0 {
			// TOML format: key = value
			sepIdx = strings.Index(line, "=")
		}
		if sepIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:sepIdx])
		value := strings.TrimSpace(line[sepIdx+1:])
		value = strings.Trim(value, "\"'")
		if key != "" {
			fm[key] = value
		}
	}
	return fm, lineCount
}

func stripFrontmatter(content string, fmLines int) string {
	if fmLines == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	if fmLines >= len(lines) {
		return ""
	}
	return strings.Join(lines[fmLines:], "\n")
}

// ─── D4: Export ─────────────────────────────────────────────

func (m *model) exportHTMLCmd() tea.Cmd {
	if m.currentPath == "" || m.sourceContent == "" {
		return nil
	}
	content := m.sourceContent
	path := m.currentPath
	return func() tea.Msg {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			return fileSavedMsg{path: path, err: err}
		}
		out, err := renderer.Render(content)
		if err != nil {
			return fileSavedMsg{path: path, err: err}
		}
		htmlPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".html"
		// Wrap in basic HTML
		html := fmt.Sprintf("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>%s</title></head>\n<body><pre>%s</pre></body></html>",
			filepath.Base(path), out)
		err = os.WriteFile(htmlPath, []byte(html), 0644)
		return fileSavedMsg{path: htmlPath, err: err}
	}
}

func (m *model) copyHTMLCmd() tea.Cmd {
	if m.sourceContent == "" {
		return nil
	}
	content := m.sourceContent
	return func() tea.Msg {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			return nil
		}
		out, err := renderer.Render(content)
		if err != nil {
			return nil
		}
		return tea.SetClipboard(out)()
	}
}

// ─── D2: Recent files ring ──────────────────────────────────

func (m *model) trackRecentFile(path string) {
	if path == "" {
		return
	}
	if idx := slices.Index(m.recentFiles, path); idx >= 0 {
		m.recentFiles = slices.Delete(m.recentFiles, idx, idx+1)
	}
	m.recentFiles = slices.Insert(m.recentFiles, 0, path)
	if len(m.recentFiles) > recentFilesLimit {
		m.recentFiles = m.recentFiles[:recentFilesLimit]
	}
}

// ─── D5: Git integration ────────────────────────────────────

func (m model) gitStatusCmd() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
		out, err := cmd.Output()
		if err != nil {
			return gitStatusMsg{status: nil}
		}
		status := make(map[string]string)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if len(line) < 4 {
				continue
			}
			code := strings.TrimSpace(line[:2])
			file := strings.TrimSpace(line[3:])
			// Handle renames: "R  old -> new"
			if idx := strings.Index(file, " -> "); idx >= 0 {
				file = file[idx+4:]
			}
			status[file] = code
		}
		return gitStatusMsg{status: status}
	}
}

func (m model) gitStatusIndicator(path string) string {
	if len(m.gitFileStatus) == 0 || path == "" {
		return ""
	}
	rel, err := filepath.Rel(m.dir, path)
	if err != nil {
		return ""
	}
	if code, ok := m.gitFileStatus[rel]; ok {
		switch {
		case strings.Contains(code, "M"):
			return "M"
		case strings.Contains(code, "A"):
			return "A"
		case strings.Contains(code, "?"):
			return "?"
		case strings.Contains(code, "D"):
			return "D"
		case strings.Contains(code, "R"):
			return "R"
		default:
			return code
		}
	}
	return ""
}

// ─── Overlay rendering helpers ──────────────────────────────
