package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	xansi "github.com/charmbracelet/x/ansi"
)

type shortcut struct {
	keys    string
	desc    string
	section bool
}

type helpSection struct {
	title string
	rows  []shortcut
}

type quickAction struct {
	key  string
	desc string
}

type command struct {
	name    string
	keys    string
	action  string
	visible func(*model) bool
}

func (c command) FilterValue() string { return c.name + " " + c.keys }

func allCommands(m *model) []command {
	commands := []command{
		{name: "Search", keys: "/, ctrl+f", action: "search_open", visible: func(m *model) bool {
			return !m.showSearch && (m.mode == modePreview || m.mode == modeRaw || m.mode == modeSplit)
		}},
		{name: "Toggle outline/files", keys: "o", action: "toggle_outline", visible: func(m *model) bool { return len(m.headings) > 0 && !m.fullScreen }},
		{name: "Toggle full screen", keys: "f", action: "toggle_full_screen"},
		{name: "Toggle focus pane", keys: "tab, shift+tab, ctrl+tab", action: "toggle_focus_pane"},
		{name: "Toggle focus mode", keys: "x", action: "toggle_focus_mode"},
		{name: "Toggle reading mode", keys: "z", action: "toggle_reading_mode"},
		{name: "Toggle reduced motion", keys: "m", action: "toggle_reduced_motion"},
		{name: "Toggle section gauge", keys: "u", action: "toggle_section_gauge", visible: func(m *model) bool { return m.mode == modePreview }},
		{name: "Toggle line numbers", keys: "n", action: "toggle_line_numbers"},
		{name: "Toggle code syntax", keys: "y", action: "toggle_code_syntax"},
		{name: "Edit raw", keys: "e", action: "edit_raw", visible: func(m *model) bool { return m.mode == modePreview && m.currentPath != "" }},
		{name: "Return to preview", keys: "p", action: "preview_mode", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Toggle rich/plain preview", keys: "v", action: "toggle_preview_style"},
		{name: "Refresh files", keys: "ctrl+r", action: "refresh_files"},
		{name: "Refresh terminal capabilities/colors", keys: "ctrl+g", action: "refresh_terminal_features"},
		{name: "Save file", keys: "ctrl+s", action: "save_file", visible: func(m *model) bool { return (m.mode == modeRaw || m.mode == modeSplit) && m.currentPath != "" }},
		{name: "Toggle format on save", keys: "ctrl+shift+s", action: "toggle_format_on_save", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Toggle home/end mode", keys: "ctrl+shift+h", action: "toggle_home_end_mode", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Go to line/column", keys: "ctrl+l", action: "goto_line_prompt", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Go to heading", keys: "ctrl+shift+l", action: "goto_heading_prompt", visible: func(m *model) bool { return (m.mode == modeRaw || m.mode == modeSplit) && len(m.headings) > 0 }},
		{name: "Replace", keys: "ctrl+h", action: "replace_open", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Previous paragraph", keys: "alt+[", action: "jump_prev_paragraph", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Next paragraph", keys: "alt+]", action: "jump_next_paragraph", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Previous heading", keys: "alt+h", action: "jump_prev_heading", visible: func(m *model) bool { return (m.mode == modeRaw || m.mode == modeSplit) && len(m.headings) > 0 }},
		{name: "Next heading", keys: "alt+l", action: "jump_next_heading", visible: func(m *model) bool { return (m.mode == modeRaw || m.mode == modeSplit) && len(m.headings) > 0 }},
		{name: "Copy current file path", keys: "Y", action: "copy_file_path", visible: func(m *model) bool { return m.currentPath != "" }},
		{name: "Copy current heading link", keys: "ctrl+y", action: "copy_heading_link", visible: func(m *model) bool { return m.mode == modePreview && len(m.headings) > 0 && m.currentPath != "" }},
		{name: "Copy selected editor line", keys: "alt+y", action: "copy_selected_text", visible: func(m *model) bool { return (m.mode == modeRaw || m.mode == modeSplit) && m.focusRight }},
		{name: "Redo", keys: "ctrl+y, ctrl+shift+z", action: "redo_edit", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Copy status/output", keys: "ctrl+shift+y", action: "copy_command_output", visible: func(m *model) bool { return strings.TrimSpace(m.status) != "" }},
		{name: "Paste clipboard", keys: "ctrl+v", action: "paste_clipboard", visible: func(m *model) bool { return m.activeClipboardTarget() != clipboardTargetNone }},
		{name: "Suspend to shell", keys: "ctrl+o", action: "suspend_shell"},
		{name: "Zoom in", keys: "=", action: "zoom_in"},
		{name: "Zoom out", keys: "-", action: "zoom_out"},
		{name: "Reset zoom", keys: "0", action: "zoom_reset"},
		{name: "Theme picker", keys: "T", action: "theme_picker"},
		{name: "Cycle theme", keys: "t", action: "cycle_theme"},
		{name: "Cycle performance visuals", keys: "H", action: "cycle_perf_visuals"},
		{name: "Toggle runtime perf overlay", keys: "P", action: "toggle_perf_overlay"},
		{name: "Go to top", keys: "g", action: "go_top", visible: func(m *model) bool { return m.mode == modePreview }},
		{name: "Go to bottom", keys: "G", action: "go_bottom", visible: func(m *model) bool { return m.mode == modePreview }},
		{name: "Scroll up", keys: "k", action: "scroll_up", visible: func(m *model) bool { return m.mode == modePreview && m.focusRight }},
		{name: "Scroll down", keys: "j", action: "scroll_down", visible: func(m *model) bool { return m.mode == modePreview && m.focusRight }},
		{name: "Half page up", keys: "ctrl+u", action: "half_page_up", visible: func(m *model) bool { return m.mode == modePreview && m.focusRight }},
		{name: "Half page down", keys: "ctrl+d", action: "half_page_down", visible: func(m *model) bool { return m.mode == modePreview && m.focusRight }},
		{name: "Expand left pane", keys: "[", action: "expand_left"},
		{name: "Expand right pane", keys: "]", action: "expand_right"},
		{name: "Create file", keys: "C", action: "create_file", visible: func(m *model) bool { return !m.showOutline && m.mode == modePreview }},
		{name: "Rename file", keys: "R", action: "rename_file", visible: func(m *model) bool { return !m.showOutline && m.mode == modePreview && m.currentPath != "" }},
		{name: "Delete file", keys: "D", action: "delete_file", visible: func(m *model) bool { return !m.showOutline && m.mode == modePreview && m.currentPath != "" }},
		{name: "Show help", keys: "?", action: "show_help"},
		{name: "Toggle statistics", keys: "W", action: "toggle_stats"},
		{name: "Toggle auto-save", keys: "palette", action: "toggle_auto_save", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Find file", keys: "ctrl+p", action: "find_file"},
		{name: "Search across files", keys: "ctrl+shift+f", action: "content_search"},
		{name: "Split preview", keys: "s", action: "split_preview", visible: func(m *model) bool { return (m.mode == modeRaw || m.mode == modeSplit) && m.currentPath != "" }},
		{name: "Back (navigate)", keys: "alt+left", action: "nav_back", visible: func(m *model) bool { return len(m.navStack) > 0 }},
		{name: "Follow link", keys: "enter", action: "follow_link", visible: func(m *model) bool { return m.mode == modePreview && m.currentPath != "" }},
		{name: "Set bookmark", keys: "b", action: "set_bookmark", visible: func(m *model) bool { return m.mode == modePreview }},
		{name: "Jump to bookmark", keys: "'", action: "jump_bookmark", visible: func(m *model) bool { return m.mode == modePreview && len(m.bookmarks) > 0 }},
		{name: "Toggle fold", keys: "enter", action: "toggle_fold", visible: func(m *model) bool { return m.mode == modePreview && len(m.headings) > 0 }},
		{name: "Fold all sections", keys: "zM", action: "fold_all", visible: func(m *model) bool { return m.mode == modePreview && len(m.headings) > 0 }},
		{name: "Unfold all sections", keys: "zR", action: "unfold_all", visible: func(m *model) bool { return m.mode == modePreview && len(m.headings) > 0 }},
		{name: "Toggle soft wrap", keys: "alt+z", action: "toggle_soft_wrap", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Export HTML", keys: "palette", action: "export_html", visible: func(m *model) bool { return m.currentPath != "" }},
		{name: "Copy as HTML", keys: "palette", action: "copy_html", visible: func(m *model) bool { return m.currentPath != "" }},
		{name: "Next recent file", keys: "palette", action: "next_buffer", visible: func(m *model) bool { return len(m.recentFiles) > 1 }},
		{name: "Previous recent file", keys: "palette", action: "prev_buffer", visible: func(m *model) bool { return len(m.recentFiles) > 1 }},
		// Snippet commands
		{name: "Snippet: Table (NxM)", keys: "palette", action: "snippet_table", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Code Block", keys: "palette", action: "snippet_codeblock", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Frontmatter", keys: "palette", action: "snippet_frontmatter", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Admonition", keys: "palette", action: "snippet_admonition", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Details/Summary", keys: "palette", action: "snippet_details", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Math Block", keys: "palette", action: "snippet_math", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Mermaid Diagram", keys: "palette", action: "snippet_mermaid", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Snippet: Table of Contents", keys: "palette", action: "snippet_toc", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Quit", keys: "q", action: "quit"},
		// Edit mode formatting commands
		{name: "Bold", keys: "ctrl+b", action: "fmt_bold", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Italic", keys: "ctrl+i", action: "fmt_italic", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Bold Italic", keys: "palette", action: "fmt_bold_italic", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Inline Code", keys: "palette", action: "fmt_inline_code", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Strikethrough", keys: "palette", action: "fmt_strikethrough", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Highlight", keys: "palette", action: "fmt_highlight", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Link", keys: "palette", action: "fmt_link", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Image", keys: "palette", action: "fmt_image", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Footnote", keys: "palette", action: "fmt_footnote", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Heading 1", keys: "palette", action: "fmt_h1", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Heading 2", keys: "palette", action: "fmt_h2", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Heading 3", keys: "palette", action: "fmt_h3", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Heading 4", keys: "palette", action: "fmt_h4", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Heading 5", keys: "palette", action: "fmt_h5", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Heading 6", keys: "palette", action: "fmt_h6", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Blockquote", keys: "palette", action: "fmt_blockquote", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Unordered List", keys: "palette", action: "fmt_ul", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Ordered List", keys: "palette", action: "fmt_ol", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Task List", keys: "palette", action: "fmt_tasklist", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Horizontal Rule", keys: "palette", action: "fmt_hr", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Code Block", keys: "palette", action: "fmt_codeblock", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
		{name: "Table", keys: "palette", action: "fmt_table", visible: func(m *model) bool { return m.mode == modeRaw || m.mode == modeSplit }},
	}
	if m != nil {
		limit := min(9, len(m.headings))
		for i := range limit {
			idx := i
			title := m.headings[i].title
			commands = append(commands, command{
				name:   fmt.Sprintf("Jump to heading %d: %s", i+1, title),
				keys:   strconv.Itoa(i + 1),
				action: fmt.Sprintf("jump_heading_%d", i),
				visible: func(m *model) bool {
					return m.mode == modePreview && idx < len(m.headings)
				},
			})
		}
	}
	return commands
}

func (m *model) filterCommands(query string) []command {
	all := allCommands(m)
	if query == "" {
		var visible []command
		for _, c := range all {
			if c.visible == nil || c.visible(m) {
				visible = append(visible, c)
			}
		}
		return visible
	}
	qLower := strings.ToLower(query)
	var matches []command
	for _, c := range all {
		if c.visible != nil && !c.visible(m) {
			continue
		}
		if strings.Contains(strings.ToLower(c.name), qLower) || strings.Contains(strings.ToLower(c.keys), qLower) {
			matches = append(matches, c)
		}
	}
	return matches
}

func sectionShortcut(title string) shortcut {
	return shortcut{keys: title, section: true}
}

func splitHelpSections(items []shortcut) []helpSection {
	sections := make([]helpSection, 0)
	cur := helpSection{}
	flush := func() {
		if cur.title == "" && len(cur.rows) == 0 {
			return
		}
		sections = append(sections, cur)
		cur = helpSection{}
	}
	for _, it := range items {
		if it.section {
			flush()
			cur = helpSection{title: it.keys}
			continue
		}
		if cur.title == "" {
			cur.title = "General"
		}
		cur.rows = append(cur.rows, it)
	}
	flush()
	return sections
}

func helpOverlayColumnCount(totalWidth, sectionCount int) int {
	if sectionCount <= 1 {
		return 1
	}
	available := max(32, totalWidth-8)
	maxCols := min(3, sectionCount)
	for cols := maxCols; cols >= 1; cols-- {
		colWidth := (available - (cols-1)*3) / cols
		if colWidth >= 34 {
			return cols
		}
	}
	return 1
}

func (m model) renderHelpOverlay() string {
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	muted := lipgloss.Color(m.palette.muted)
	sections := splitHelpSections(helpShortcuts())
	cols := helpOverlayColumnCount(m.width, len(sections))
	gapWidth := 3
	if cols == 1 {
		gapWidth = 0
	}
	availableWidth := max(36, m.width-8)
	colWidth := (availableWidth - (cols-1)*gapWidth) / cols
	if colWidth < 30 {
		colWidth = 30
	}
	keyColWidth := max(12, min(22, colWidth/3))
	descColWidth := max(14, colWidth-keyColWidth-1)
	keyStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Width(keyColWidth)
	descStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(muted).
		Width(descColWidth)
	sectionStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Width(colWidth)

	columnSections := make([][]helpSection, cols)
	columnHeights := make([]int, cols)
	for _, sec := range sections {
		target := 0
		for i := 1; i < cols; i++ {
			if columnHeights[i] < columnHeights[target] {
				target = i
			}
		}
		columnSections[target] = append(columnSections[target], sec)
		columnHeights[target] += 2 + len(sec.rows) // title + rows + separator spacing
	}
	maxColHeight := 0
	for _, h := range columnHeights {
		if h > maxColHeight {
			maxColHeight = h
		}
	}
	if maxColHeight < 1 {
		maxColHeight = 1
	}

	colViews := make([]string, 0, cols)
	for i := range cols {
		parts := make([]string, 0)
		for s, sec := range columnSections[i] {
			if s > 0 {
				parts = append(parts, "")
			}
			parts = append(parts, sectionStyle.Render(sec.title))
			for _, row := range sec.rows {
				line := lipgloss.JoinHorizontal(lipgloss.Top, keyStyle.Render(row.keys), descStyle.Render(row.desc))
				parts = append(parts, line)
			}
		}
		colBody := strings.Join(parts, "\n")
		colView := lipgloss.NewStyle().
			Background(bg).
			Width(colWidth).
			Height(maxColHeight).
			Render(colBody)
		colViews = append(colViews, colView)
	}

	gap := lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", gapWidth))
	body := ""
	if len(colViews) > 0 {
		body = colViews[0]
		for i := 1; i < len(colViews); i++ {
			body = lipgloss.JoinHorizontal(lipgloss.Top, body, gap, colViews[i])
		}
	}
	bodyWidth := lipgloss.Width(body)
	if bodyWidth < colWidth {
		bodyWidth = colWidth
	}

	title := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Width(bodyWidth).
		Render("Shortcuts")
	content := title + "\n" + body

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.palette.border)).
		Background(bg).
		Padding(0, 1).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(bg).Foreground(text)))
}

func helpShortcuts() []shortcut {
	return []shortcut{
		sectionShortcut("Global"),
		{keys: "q, ctrl+c", desc: "Quit"},
		{keys: "?", desc: "Show/close help"},
		{keys: "ctrl+k", desc: "Open/close command palette"},
		{keys: "ctrl+p", desc: "Find file (fuzzy)"},
		{keys: "ctrl+shift+f", desc: "Search across files"},
		{keys: "tab, shift+tab (preview/list), ctrl+tab (raw/editor)", desc: "Toggle focus pane"},
		{keys: "[ / ]", desc: "Resize panes"},
		{keys: "ctrl+r", desc: "Refresh files"},
		{keys: "ctrl+g", desc: "Refresh terminal capabilities"},
		{keys: "ctrl+o", desc: "Suspend to shell"},
		{keys: "Y", desc: "Copy current file path"},
		{keys: "ctrl+shift+y", desc: "Copy status/output"},
		{keys: "ctrl+v", desc: "Paste clipboard into active input"},
		{keys: "W", desc: "Toggle document statistics"},
		sectionShortcut("Preview"),
		{keys: "j/k, up/down", desc: "Scroll line"},
		{keys: "pgup/pgdn", desc: "Page up/down"},
		{keys: "ctrl+u / ctrl+d", desc: "Half page up/down"},
		{keys: "g / G", desc: "Jump top/bottom"},
		{keys: "mouse wheel", desc: "Scroll preview"},
		{keys: "click section gauge", desc: "Jump to section/line"},
		{keys: "u", desc: "Toggle section gauge"},
		{keys: "1-9", desc: "Jump to heading"},
		{keys: "o", desc: "Toggle outline/files"},
		{keys: "e / p", desc: "Edit raw / return preview"},
		{keys: "s", desc: "Split preview (editor + preview)"},
		{keys: "f", desc: "Toggle full screen"},
		{keys: "v", desc: "Toggle rich/plain preview"},
		{keys: "n", desc: "Toggle line numbers"},
		{keys: "x / z / m / y", desc: "Focus/reading/motion/code modes"},
		{keys: "= / - / 0", desc: "Zoom in/out/reset"},
		{keys: "t / T", desc: "Cycle theme / open theme picker"},
		{keys: "H / P", desc: "Perf visuals / runtime perf overlay"},
		{keys: "enter", desc: "Follow link / toggle fold"},
		{keys: "zM / zR", desc: "Fold all / unfold all"},
		{keys: "alt+left", desc: "Navigate back"},
		{keys: "b", desc: "Set bookmark (then press a-z)"},
		{keys: "'", desc: "Jump to bookmark (then press a-z)"},
		{keys: "ctrl+y", desc: "Copy heading link"},
		sectionShortcut("Search"),
		{keys: "/ (preview), ctrl+f (edit)", desc: "Open search"},
		{keys: "enter, ctrl+n, f3", desc: "Next match"},
		{keys: "shift+enter, ctrl+p, shift+f3", desc: "Previous match"},
		{keys: "ctrl+v", desc: "Paste search query"},
		{keys: "esc", desc: "Close search"},
		sectionShortcut("Editor"),
		{keys: "ctrl+s", desc: "Save file"},
		{keys: "ctrl+shift+s", desc: "Toggle format on save"},
		{keys: "ctrl+shift+h", desc: "Toggle home/end mode"},
		{keys: "ctrl+y, ctrl+shift+z", desc: "Redo"},
		{keys: "alt+y", desc: "Copy current line"},
		{keys: "ctrl+v", desc: "Paste clipboard"},
		{keys: "ctrl+l", desc: "Go to line/column"},
		{keys: "ctrl+shift+l", desc: "Go to heading"},
		{keys: "ctrl+h", desc: "Find and replace"},
		{keys: "alt+z", desc: "Toggle soft wrap"},
		{keys: "ctrl+left/right", desc: "Move by word"},
		{keys: "ctrl+shift+left/right", desc: "Select by word"},
		{keys: "ctrl+home/end", desc: "Jump to file start/end"},
		{keys: "ctrl+shift+home/end", desc: "Select to file start/end"},
		{keys: "pgup/pgdown", desc: "Move by page"},
		{keys: "shift+pgup/pgdown", desc: "Select by page"},
		{keys: "ctrl+backspace/delete", desc: "Delete word"},
		{keys: "alt+[ / alt+]", desc: "Previous/next paragraph"},
		{keys: "alt+h / alt+l", desc: "Previous/next heading"},
		{keys: "alt+up/down", desc: "Move lines"},
		{keys: "ctrl+alt+up/down", desc: "Duplicate lines"},
		{keys: "alt+shift+left/right", desc: "Expand selection by word"},
		{keys: "alt+shift+up/down", desc: "Expand selection by line"},
		{keys: "home/end", desc: "Line start/end"},
		{keys: "shift+home/end", desc: "Select to line boundary"},
		{keys: "enter", desc: "Smart list continuation"},
		{keys: "tab / shift+tab", desc: "Indent / outdent"},
		{keys: "esc", desc: "Return to preview"},
		sectionShortcut("Table Editing"),
		{keys: "(auto)", desc: "Activates when cursor is in a table"},
		{keys: "tab / shift+tab", desc: "Next / previous cell"},
		{keys: "enter", desc: "Next row (adds row at end)"},
		{keys: "esc", desc: "Exit table mode and auto-format"},
		sectionShortcut("Replace"),
		{keys: "tab", desc: "Switch find/replace field"},
		{keys: "enter", desc: "Replace next match"},
		{keys: "ctrl+enter", desc: "Replace all matches"},
		{keys: "alt+c / alt+w / alt+s", desc: "Case / whole-word / selection"},
		sectionShortcut("File Operations"),
		{keys: "C / R / D", desc: "Create / rename / delete file"},
		{keys: "enter", desc: "Confirm"},
		{keys: "ctrl+v", desc: "Paste file name"},
		{keys: "y / n, esc", desc: "Confirm / cancel delete"},
		sectionShortcut("Command Palette"),
		{keys: "type", desc: "Filter commands"},
		{keys: "up/down", desc: "Move selection"},
		{keys: "enter", desc: "Run command"},
		{keys: "esc, ctrl+k", desc: "Close"},
		sectionShortcut("Theme Picker"),
		{keys: "up/down, j/k", desc: "Move selection"},
		{keys: "enter", desc: "Apply theme"},
		{keys: "s", desc: "Toggle follow system"},
		{keys: "esc, T", desc: "Close"},
	}
}

func (m *model) scrollPreviewBy(lines int) {
	if lines == 0 || m.mode != modePreview {
		return
	}
	m.cancelMomentum()
	m.viewport.SetYOffset(m.viewport.YOffset() + lines)
	m.previewYOffset = m.viewport.YOffset()
	m.syncCurrentHeading(m.viewport.YOffset())
}

func (m model) runCommandAction(action string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(action, "jump_heading_") {
		idxStr := strings.TrimPrefix(action, "jump_heading_")
		idx, err := strconv.Atoi(idxStr)
		if err == nil && idx >= 0 && idx < len(m.headings) && m.mode == modePreview {
			m.setCurrentHeading(idx)
			return m, m.jumpToHeading(m.headings[idx].title)
		}
		return m, nil
	}
	switch action {
	case "search_open":
		return m, m.openSearch()
	case "toggle_outline":
		if len(m.outline.Items()) > 0 {
			m.showOutline = !m.showOutline
			if m.showOutline {
				m.status = "Outline mode"
				m.ensureOutlineSelection()
			} else {
				m.status = "File list"
			}
		}
		return m, nil
	case "toggle_full_screen":
		m.fullScreen = !m.fullScreen
		m.focusRight = true
		if m.fullScreen {
			m.showOutline = false
			m.status = "Full screen"
		} else {
			m.status = "Split view"
		}
		m.resizeViews()
		return m, m.renderCurrentContent()
	case "toggle_focus_pane":
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
		return m, nil
	case "toggle_focus_mode":
		m.toggleFocusMode()
		return m, m.renderCurrentContent()
	case "toggle_reading_mode":
		m.toggleReadingMode()
		return m, m.renderCurrentContent()
	case "toggle_reduced_motion":
		m.reducedMotion = !m.reducedMotion
		label := "Reduced motion off"
		if m.reducedMotion {
			label = "Reduced motion on"
		}
		m.status = label
		return m, nil
	case "toggle_section_gauge":
		m.showGauge = !m.showGauge
		m.status = fmt.Sprintf("Section gauge %v", onOff(m.showGauge))
		m.resizeViews()
		return m, m.renderCurrentContent()
	case "toggle_line_numbers":
		m.showLineNums = !m.showLineNums
		m.textarea.ShowLineNumbers = m.showLineNums
		if m.mode == modePreview {
			m.previewYOffset = m.viewport.YOffset()
			m.status = fmt.Sprintf("Line numbers %v", onOff(m.showLineNums))
			return m, m.renderCurrentContent()
		}
		m.status = fmt.Sprintf("Line numbers %v", onOff(m.showLineNums))
		return m, nil
	case "toggle_code_syntax":
		m.codePlain = !m.codePlain
		if m.codePlain {
			m.status = "Code: plain"
		} else {
			m.status = "Code: syntax"
		}
		return m, m.renderCurrentContent()
	case "edit_raw":
		if m.currentPath != "" {
			m.previewYOffset = m.viewport.YOffset()
			m.mode = modeRaw
			m.textarea.Focus()
			m.focusRight = true
			m.status = "Raw edit mode"
			m.positionEditorCursor()
		}
		return m, nil
	case "preview_mode":
		m.previewYOffset = m.viewport.YOffset()
		m.mode = modePreview
		m.textarea.Blur()
		m.focusRight = true
		m.resizeViews()
		m.status = "Preview mode"
		return m, m.renderCurrentContent()
	case "toggle_preview_style":
		m.previewYOffset = m.viewport.YOffset()
		m.richPreview = !m.richPreview
		m.status = m.previewStatus()
		return m, m.renderCurrentContent()
	case "refresh_files":
		return m, m.refreshFilesAndCurrentFileCmd()
	case "refresh_terminal_features":
		m.status = "Requested terminal capabilities/colors"
		m.capProbeRequested = true
		cmds := []tea.Cmd{
			requestTerminalColorsCmd(),
			requestTermcapProbeCmd(),
		}
		return m, tea.Batch(cmds...)
	case "save_file":
		if m.mode == modeRaw {
			return m, m.saveCurrentFileCmd()
		}
		return m, nil
	case "toggle_format_on_save":
		m.formatOnSave = !m.formatOnSave
		m.status = fmt.Sprintf("Format on save %v", onOff(m.formatOnSave))
		return m, nil
	case "toggle_home_end_mode":
		m.editHomeEndWrapped = !m.editHomeEndWrapped
		if m.editHomeEndWrapped {
			m.status = "Home/end: wrapped"
		} else {
			m.status = "Home/end: logical"
		}
		return m, nil
	case "goto_line_prompt":
		m.startGoToLine()
		return m, nil
	case "goto_heading_prompt":
		m.startGoToHeading()
		return m, nil
	case "replace_open":
		if m.mode != modeRaw {
			return m, nil
		}
		return m, m.openReplace()
	case "jump_prev_paragraph":
		return m, m.editJumpParagraph(false)
	case "jump_next_paragraph":
		return m, m.editJumpParagraph(true)
	case "jump_prev_heading":
		return m, m.editJumpHeading(false)
	case "jump_next_heading":
		return m, m.editJumpHeading(true)
	case "redo_edit":
		if m.mode == modeRaw {
			m.editClearSelection()
			if m.editRedo() {
				m.sourceContent = m.textarea.Value()
				m.updateOutlineFromEditor()
				m.status = "Redone"
			}
		}
		return m, nil
	case "copy_file_path":
		path := m.currentPathForClipboard()
		if path == "" {
			m.status = m.styles.statusWarn.Render("No file selected")
			return m, nil
		}
		m.status = "Copied path to clipboard"
		return m, tea.SetClipboard(path)
	case "copy_heading_link":
		link := m.currentHeadingLinkForClipboard()
		if link == "" {
			m.status = m.styles.statusWarn.Render("No heading link available")
			return m, nil
		}
		m.status = "Copied heading link"
		return m, tea.SetClipboard(link)
	case "copy_selected_text":
		line := m.currentEditorLineForClipboard()
		if line == "" {
			m.status = m.styles.statusWarn.Render("No editor selection to copy")
			return m, nil
		}
		m.status = "Copied selected text"
		return m, tea.SetClipboard(line)
	case "copy_command_output":
		out := strings.TrimSpace(m.status)
		if out == "" {
			m.status = m.styles.statusWarn.Render("No command output to copy")
			return m, nil
		}
		m.status = "Copied command output"
		return m, tea.SetClipboard(out)
	case "paste_clipboard":
		target := m.activeClipboardTarget()
		if target == clipboardTargetNone {
			m.status = m.styles.statusWarn.Render("No active input to paste into")
			return m, nil
		}
		m.pendingClipboardTarget = target
		m.status = "Reading clipboard…"
		return m, readClipboardCmd()
	case "suspend_shell":
		return m, tea.Suspend
	case "zoom_in":
		m.adjustZoom(0.1)
		return m, m.renderCurrentContent()
	case "zoom_out":
		m.adjustZoom(-0.1)
		return m, m.renderCurrentContent()
	case "zoom_reset":
		m.zoom = 1.0
		m.status = "Zoom 100%"
		return m, m.renderCurrentContent()
	case "theme_picker":
		m.showThemePicker = true
		m.status = "Theme picker"
		return m, nil
	case "cycle_theme":
		m.previewYOffset = m.viewport.YOffset()
		themeCmd := m.applyTheme(nextStyle(m.styleName))
		m.status = fmt.Sprintf("Preview style: %s", m.styleName)
		return m, tea.Batch(themeCmd, m.renderCurrentContent())
	case "cycle_perf_visuals":
		m.cyclePerfVisualMode()
		m.resizeViews()
		return m, m.renderCurrentContent()
	case "toggle_perf_overlay":
		m.togglePerfOverlay()
		return m, nil
	case "go_top":
		m.cancelMomentum()
		m.viewport.SetYOffset(0)
		m.previewYOffset = 0
		m.status = "Top"
		return m, nil
	case "go_bottom":
		m.cancelMomentum()
		bottom := bottomOffsetFromRendered(m.rendered, m.viewport.Height())
		m.viewport.SetYOffset(bottom)
		m.previewYOffset = bottom
		m.status = "Bottom"
		return m, nil
	case "scroll_up":
		m.scrollPreviewBy(-1)
		return m, nil
	case "scroll_down":
		m.scrollPreviewBy(1)
		return m, nil
	case "half_page_up":
		m.scrollPreviewBy(-max(1, m.viewport.Height()/2))
		return m, nil
	case "half_page_down":
		m.scrollPreviewBy(max(1, m.viewport.Height()/2))
		return m, nil
	case "expand_left":
		m.adjustListRatio(-0.03)
		m.status = "Adjusted split"
		m.resizeViews()
		return m, nil
	case "expand_right":
		m.adjustListRatio(0.03)
		m.status = "Adjusted split"
		m.resizeViews()
		return m, nil
	case "create_file":
		m.startCreate()
		return m, nil
	case "rename_file":
		m.startRename()
		return m, nil
	case "delete_file":
		m.startDelete()
		return m, nil
	case "show_help":
		m.showHelp = true
		m.status = "Help"
		return m, nil
	case "toggle_stats":
		m.showStats = !m.showStats
		m.status = fmt.Sprintf("Statistics %v", onOff(m.showStats))
		return m, nil
	case "toggle_auto_save":
		m.autoSave = !m.autoSave
		m.status = fmt.Sprintf("Auto-save %v", onOff(m.autoSave))
		return m, nil
	case "find_file":
		m.showFileFinder = true
		m.fileFinderInput.SetValue("")
		m.fileFinderInput.Focus()
		m.fileFinderIdx = 0
		m.fileFinderResults = nil
		m.status = "Find file"
		if len(m.fileFinderAll) == 0 {
			return m, m.walkDirCmd()
		}
		m.fileFinderResults = fuzzyMatch(m.fileFinderAll, "")
		return m, nil
	case "content_search":
		m.showContentSearch = true
		m.contentSearchInput.SetValue("")
		m.contentSearchInput.Focus()
		m.contentSearchIdx = 0
		m.contentSearchResults = nil
		m.status = "Search across files"
		if len(m.fileFinderAll) == 0 {
			return m, m.walkDirCmd()
		}
		return m, nil
	case "split_preview":
		if m.mode == modeSplit {
			m.mode = modeRaw
			m.status = "Raw edit mode"
		} else {
			m.mode = modeSplit
			m.textarea.Focus()
			m.focusRight = true
			m.status = "Split preview"
			m.resizeViews()
			return m, m.renderCurrentContent()
		}
		m.resizeViews()
		return m, nil
	case "nav_back":
		if len(m.navStack) > 0 {
			entry := m.navStack[len(m.navStack)-1]
			m.navStack = m.navStack[:len(m.navStack)-1]
			if entry.path != m.currentPath {
				m.currentPath = entry.path
				m.previewYOffset = entry.yOffset
				m.restoreHeading = entry.heading
				return m, m.loadFileCmd(entry.path)
			}
			m.viewport.SetYOffset(entry.yOffset)
			if entry.heading >= 0 && entry.heading < len(m.headings) {
				m.setCurrentHeading(entry.heading)
			}
			m.status = "Back"
		}
		return m, nil
	case "follow_link":
		return m, m.followLinkAtCursor()
	case "set_bookmark":
		m.markMode = 'b'
		m.status = "Set bookmark: press a-z"
		return m, nil
	case "jump_bookmark":
		m.markMode = '\''
		m.status = "Jump to bookmark: press a-z"
		return m, nil
	case "toggle_fold":
		if m.mode == modePreview && m.currentHeading >= 0 && m.currentHeading < len(m.headings) {
			if m.foldedSections[m.currentHeading] {
				delete(m.foldedSections, m.currentHeading)
			} else {
				m.foldedSections[m.currentHeading] = true
			}
			m.invalidateRenderCaches()
			return m, m.renderCurrentContent()
		}
		return m, nil
	case "fold_all":
		for i := range m.headings {
			m.foldedSections[i] = true
		}
		m.invalidateRenderCaches()
		m.status = "All sections folded"
		return m, m.renderCurrentContent()
	case "unfold_all":
		m.foldedSections = make(map[int]bool)
		m.invalidateRenderCaches()
		m.status = "All sections unfolded"
		return m, m.renderCurrentContent()
	case "toggle_soft_wrap":
		m.editSoftWrap = !m.editSoftWrap
		m.status = fmt.Sprintf("Soft wrap %v", onOff(m.editSoftWrap))
		m.resizeViews()
		return m, nil
	case "export_html":
		return m, m.exportHTMLCmd()
	case "copy_html":
		return m, m.copyHTMLCmd()
	case "next_buffer":
		if len(m.recentFiles) > 1 {
			// Find current and go to next
			idx := 0
			for i, f := range m.recentFiles {
				if f == m.currentPath {
					idx = i
					break
				}
			}
			idx = (idx + 1) % len(m.recentFiles)
			nextPath := m.recentFiles[idx]
			m.currentPath = nextPath
			m.previewYOffset = 0
			m.status = fmt.Sprintf("Buffer: %s", filepath.Base(nextPath))
			return m, m.loadFileCmd(nextPath)
		}
		return m, nil
	case "prev_buffer":
		if len(m.recentFiles) > 1 {
			idx := 0
			for i, f := range m.recentFiles {
				if f == m.currentPath {
					idx = i
					break
				}
			}
			idx = (idx - 1 + len(m.recentFiles)) % len(m.recentFiles)
			prevPath := m.recentFiles[idx]
			m.currentPath = prevPath
			m.previewYOffset = 0
			m.status = fmt.Sprintf("Buffer: %s", filepath.Base(prevPath))
			return m, m.loadFileCmd(prevPath)
		}
		return m, nil
	case "snippet_table":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("| Header 1 | Header 2 | Header 3 |\n| --- | --- | --- |\n| Cell 1 | Cell 2 | Cell 3 |\n")
			return true
		})
		return m, nil
	case "snippet_codeblock":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("```\ncode here\n```\n")
			return true
		})
		return m, nil
	case "snippet_frontmatter":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("---\ntitle: \ndate: \ntags: []\n---\n\n")
			return true
		})
		return m, nil
	case "snippet_admonition":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("> [!NOTE]\n> Content here\n\n")
			return true
		})
		return m, nil
	case "snippet_details":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("<details>\n<summary>Click to expand</summary>\n\nContent here\n\n</details>\n\n")
			return true
		})
		return m, nil
	case "snippet_math":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("$$\nx = \\frac{-b \\pm \\sqrt{b^2 - 4ac}}{2a}\n$$\n\n")
			return true
		})
		return m, nil
	case "snippet_mermaid":
		m.editApplyTransform(func() bool {
			m.textarea.InsertString("```mermaid\ngraph TD;\n    A-->B;\n    A-->C;\n    B-->D;\n    C-->D;\n```\n\n")
			return true
		})
		return m, nil
	case "snippet_toc":
		// Generate TOC from current headings
		if len(m.headings) > 0 {
			var toc strings.Builder
			toc.WriteString("## Table of Contents\n\n")
			for _, h := range m.headings {
				indent := strings.Repeat("  ", max(0, h.level-1))
				slug := slugify(h.title)
				toc.WriteString(fmt.Sprintf("%s- [%s](#%s)\n", indent, h.title, slug))
			}
			toc.WriteString("\n")
			m.editApplyTransform(func() bool {
				m.textarea.InsertString(toc.String())
				return true
			})
		}
		return m, nil
	case "quit":
		return m, m.saveAndQuitCmd()
	case "fmt_bold":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("**", "bold text", "**")
			return true
		})
		return m, nil
	case "fmt_italic":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("_", "italic text", "_")
			return true
		})
		return m, nil
	case "fmt_bold_italic":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("***", "bold italic text", "***")
			return true
		})
		return m, nil
	case "fmt_inline_code":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("`", "code", "`")
			return true
		})
		return m, nil
	case "fmt_strikethrough":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("~~", "strikethrough", "~~")
			return true
		})
		return m, nil
	case "fmt_highlight":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("==", "highlighted", "==")
			return true
		})
		return m, nil
	case "fmt_link":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("[", "link text", "](url)")
			return true
		})
		return m, nil
	case "fmt_image":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("![", "alt text", "](url)")
			return true
		})
		return m, nil
	case "fmt_footnote":
		m.editApplyTransform(func() bool {
			m.insertInlineMarkdown("[^", "label", "]")
			return true
		})
		return m, nil
	case "fmt_h1":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("# ")
			return true
		})
		return m, nil
	case "fmt_h2":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("## ")
			return true
		})
		return m, nil
	case "fmt_h3":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("### ")
			return true
		})
		return m, nil
	case "fmt_h4":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("#### ")
			return true
		})
		return m, nil
	case "fmt_h5":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("##### ")
			return true
		})
		return m, nil
	case "fmt_h6":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("###### ")
			return true
		})
		return m, nil
	case "fmt_blockquote":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("> ")
			return true
		})
		return m, nil
	case "fmt_ul":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("- ")
			return true
		})
		return m, nil
	case "fmt_tasklist":
		m.editApplyTransform(func() bool {
			m.toggleLinePrefix("- [ ] ")
			return true
		})
		return m, nil
	case "fmt_ol":
		m.editApplyTransform(func() bool {
			m.insertBlockAtCursor("1. list item", 0, 3)
			return true
		})
		return m, nil
	case "fmt_hr":
		m.editApplyTransform(func() bool {
			m.insertBlockAtCursor("\n---", 1, 0)
			return true
		})
		return m, nil
	case "fmt_codeblock":
		m.editApplyTransform(func() bool {
			m.insertBlockAtCursor("```\ncode here\n```", 1, 0)
			return true
		})
		return m, nil
	case "fmt_table":
		m.editApplyTransform(func() bool {
			m.insertBlockAtCursor("| Col 1 | Col 2 |\n|---|---|\n| cell | cell |", 1, 2)
			return true
		})
		return m, nil
	default:
		return m, nil
	}
}

func bottomOffsetFromRendered(rendered string, height int) int {
	if height <= 0 {
		return 0
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) <= height {
		return 0
	}
	return len(lines) - height
}

func (m *model) scrollTo(target int) tea.Cmd {
	m.cancelMomentum()
	target = max(0, target)
	if m.viewport.Height() <= 0 {
		return nil
	}
	const scrollMargin = 3
	offset := max(0, target-scrollMargin)
	m.viewport.SetYOffset(offset)
	m.previewYOffset = offset
	m.syncCurrentHeading(offset)
	return nil
}

func (m model) renderStickyHeader() string {
	if m.stickyHeaderRows() == 0 {
		return ""
	}
	title := m.currentSectionTitle()
	if title == "" {
		return ""
	}
	bg := lipgloss.Color(m.palette.bg)
	text := lipgloss.Color(m.palette.text)
	bar := lipgloss.NewStyle().
		Background(bg).
		Foreground(text).
		Bold(true).
		Padding(0, 1).
		Render(title)
	return padLineToWidth(bar, m.width, m.palette.bg)
}

func (m model) currentSectionTitle() string {
	if m.currentHeading >= 0 && m.currentHeading < len(m.headings) {
		return m.headings[m.currentHeading].title
	}
	if len(m.headings) > 0 {
		return m.headings[0].title
	}
	return ""
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func applyFocusDim(content string, headings []headingItem, current int, dimPrefix string) string {
	if current < 0 || current >= len(headings) {
		return content
	}
	lines := strings.Split(content, "\n")
	applyFocusDimLines(lines, headings, current, dimPrefix)
	return strings.Join(lines, "\n")
}

func applyReadingFocus(content string, headings []headingItem, current int, dimPrefix string) string {
	if current < 0 || current >= len(headings) {
		return content
	}
	lines := strings.Split(content, "\n")
	applyFocusDimLines(lines, headings, current, dimPrefix)
	return strings.Join(lines, "\n")
}

func dimOutsideRange(content string, start, end int, dimPrefix string) string {
	if start < 0 || end < start {
		return content
	}
	var b strings.Builder
	b.Grow(len(content) + strings.Count(content, "\n")*len(dimPrefix))
	lineIdx := 0
	lineStart := 0
	for i := 0; i <= len(content); i++ {
		if i < len(content) && content[i] != '\n' {
			continue
		}
		line := content[lineStart:i]
		if lineIdx < start || lineIdx > end {
			b.WriteString(dimPrefix)
			b.WriteString(line)
			b.WriteString("\x1b[0m")
		} else {
			b.WriteString(line)
		}
		if i < len(content) {
			b.WriteByte('\n')
		}
		lineIdx++
		lineStart = i + 1
	}
	return b.String()
}

func applyFocusDimLines(lines []string, headings []headingItem, current int, dimPrefix string) {
	if current < 0 || current >= len(headings) || len(lines) == 0 {
		return
	}
	start := headings[current].renderLine
	if start < 0 {
		return
	}
	end := len(lines) - 1
	if current+1 < len(headings) && headings[current+1].renderLine >= 0 {
		end = headings[current+1].renderLine - 1
	}
	if end < start {
		end = start
	}
	for i := range lines {
		if i >= start && i <= end {
			continue
		}
		lines[i] = dimPrefix + lines[i] + "\x1b[0m"
	}
}

func addLineSpacing(content string) string {
	lines := strings.Split(content, "\n")
	// Pre-strip all lines once to avoid redundant regex calls
	stripped := make([]string, len(lines))
	for i, line := range lines {
		stripped[i] = stripANSI(line)
	}
	var buf strings.Builder
	buf.Grow(len(content) + len(lines))
	inCodeBlock := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(stripped[i]), "```") {
			inCodeBlock = !inCodeBlock
		}

		buf.WriteString(line)
		buf.WriteString("\n")

		if i < len(lines)-1 && line != "" && lines[i+1] != "" && !inCodeBlock {
			nextStripped := stripped[i+1]
			isIndented := len(nextStripped) > 0 && (nextStripped[0] == ' ' || nextStripped[0] == '\t')
			if !isIndented {
				buf.WriteString("\n")
			}
		}
	}
	return buf.String()
}

func centerContent(content string, viewportWidth, contentWidth int) string {
	if contentWidth >= viewportWidth {
		return content
	}
	leftPadding := (viewportWidth - contentWidth) / 2
	if leftPadding <= 0 {
		return content
	}
	padding := strings.Repeat(" ", leftPadding)
	newlines := strings.Count(content, "\n")
	var buf strings.Builder
	buf.Grow(len(content) + leftPadding*newlines)
	first := true
	for i := 0; i < len(content); {
		j := strings.IndexByte(content[i:], '\n')
		var line string
		if j < 0 {
			line = content[i:]
			i = len(content)
		} else {
			line = content[i : i+j]
			i += j + 1
		}
		if !first {
			buf.WriteByte('\n')
		}
		first = false
		if line != "" {
			buf.WriteString(padding)
		}
		buf.WriteString(line)
	}
	return buf.String()
}

const previewCodeBlockPlaceholderPrefix = "@@MV_CODE_BLOCK_"

type previewCodeBlock struct {
	token    string
	language string
	body     string
}

func extractPreviewCodeBlocks(content string) (string, []previewCodeBlock) {
	lines := strings.Split(content, "\n")
	var out []string
	blocks := make([]previewCodeBlock, 0, 4)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") {
			out = append(out, line)
			continue
		}

		lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		if strings.EqualFold(lang, "mermaid") {
			out = append(out, line)
			continue
		}

		end := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
				end = j
				break
			}
		}
		if end == -1 {
			out = append(out, line)
			continue
		}

		token := fmt.Sprintf("%s%d@@", previewCodeBlockPlaceholderPrefix, len(blocks))
		blocks = append(blocks, previewCodeBlock{
			token:    token,
			language: lang,
			body:     strings.Join(lines[i+1:end], "\n"),
		})
		out = append(out, token)
		i = end
	}
	return strings.Join(out, "\n"), blocks
}

func renderPreviewCodeBlocks(renderer *glamour.TermRenderer, blocks []previewCodeBlock, width int, styles uiStyles, borderless bool, plain bool) map[string]string {
	if len(blocks) == 0 {
		return nil
	}
	replacements := make(map[string]string, len(blocks))
	for _, block := range blocks {
		replacements[block.token] = renderPreviewCodeBlock(renderer, block, width, styles, borderless, plain)
	}
	return replacements
}

func buildRenderCacheKey(content string, features markdownFeatureSet, mermaidBlocks []mermaidBlock, previewCodeBlocks []previewCodeBlock) string {
	if len(features.callouts) == 0 &&
		len(features.blockMath) == 0 &&
		len(features.blockImages) == 0 &&
		len(features.inlineMath) == 0 &&
		len(features.inlineImages) == 0 &&
		len(features.footnotes.defs) == 0 &&
		len(mermaidBlocks) == 0 &&
		len(previewCodeBlocks) == 0 {
		return content
	}

	var b strings.Builder
	b.Grow(len(content) + len(previewCodeBlocks)*16)
	b.WriteString(content)
	b.WriteString("\n\x00markdown-features:")
	for _, callout := range features.callouts {
		writeCacheKeyPart(&b, callout.kind)
		writeCacheKeyPart(&b, callout.body)
	}
	for _, block := range features.blockMath {
		writeCacheKeyPart(&b, block.body)
	}
	for _, image := range features.blockImages {
		writeCacheKeyPart(&b, image.alt)
		writeCacheKeyPart(&b, image.target)
		writeCacheKeyPart(&b, image.title)
	}
	for _, math := range features.inlineMath {
		writeCacheKeyPart(&b, math.body)
	}
	for _, image := range features.inlineImages {
		writeCacheKeyPart(&b, image.alt)
		writeCacheKeyPart(&b, image.target)
		writeCacheKeyPart(&b, image.title)
	}
	for _, def := range features.footnotes.defs {
		writeCacheKeyPart(&b, def.id)
		writeCacheKeyPart(&b, def.body)
	}
	b.WriteString("\n\x00mermaid:")
	for _, block := range mermaidBlocks {
		writeCacheKeyPart(&b, block.source)
	}
	b.WriteString("\n\x00codeblocks:")
	for _, block := range previewCodeBlocks {
		writeCacheKeyPart(&b, block.language)
		writeCacheKeyPart(&b, block.body)
	}
	return b.String()
}

func writeCacheKeyPart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('\n')
}

func renderPreviewCodeBlock(renderer *glamour.TermRenderer, block previewCodeBlock, width int, styles uiStyles, borderless bool, plain bool) string {
	lines := strings.Split(block.body, "\n")
	if renderer != nil {
		markdown := "```\n" + block.body + "\n```"
		if block.language != "" {
			markdown = "```" + block.language + "\n" + block.body + "\n```"
		}
		if rendered, err := renderer.Render(markdown); err == nil {
			lines = trimPreviewCodeOuterSpace(strings.Split(strings.TrimRight(rendered, "\n"), "\n"))
			if plain {
				for i, line := range lines {
					lines[i] = stripANSI(line)
				}
			}
		}
	}
	return renderPreviewCodeCard(lines, block.language, width, styles, borderless)
}

func trimPreviewCodeOuterSpace(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(stripANSI(lines[start])) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(stripANSI(lines[end-1])) == "" {
		end--
	}
	if start >= end {
		return []string{""}
	}
	return lines[start:end]
}

func renderPreviewCodeCard(lines []string, language string, width int, styles uiStyles, borderless bool) string {
	width = max(1, width)
	rows := make([]string, 0, len(lines)+1)
	if language != "" {
		rows = append(rows, renderPreviewCodeHeader(language, width, styles, borderless))
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	for _, line := range lines {
		for _, wrapped := range wrapPreviewCodeLine(line, width) {
			rows = append(rows, renderPreviewCodeLine(wrapped, width, styles, borderless))
		}
	}
	return strings.Join(rows, "\n")
}

func renderPreviewCodeHeader(language string, width int, styles uiStyles, borderless bool) string {
	label := " " + language + " "
	if lipgloss.Width(label) > width {
		label = xansi.Truncate(label, width, "")
	}
	if borderless {
		return styles.sgrBgHighlight + styles.sgrFgText + styles.sgrBold + label + styles.sgrReset
	}
	pad := max(0, width-lipgloss.Width(label))
	return styles.sgrBgHighlight + styles.sgrFgText + styles.sgrBold + label + strings.Repeat(" ", pad) + styles.sgrReset
}

func renderPreviewCodeLine(line string, width int, styles uiStyles, borderless bool) string {
	line = strings.TrimRight(line, "\r")
	if borderless {
		if line == "" {
			return ""
		}
		return line
	}
	pad := max(0, width-lipgloss.Width(stripANSI(line)))
	if line == "" {
		return styles.sgrBgSurface + strings.Repeat(" ", width) + styles.sgrReset
	}
	if strings.Contains(line, "\x1b[") {
		return line + styles.sgrBgSurface + strings.Repeat(" ", pad) + styles.sgrReset
	}
	return styles.sgrBgSurface + line + strings.Repeat(" ", pad) + styles.sgrReset
}

func wrapPreviewCodeLine(line string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return []string{""}
	}
	wrapped := xansi.Hardwrap(line, width, true)
	parts := strings.Split(wrapped, "\n")
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

func (m model) showSectionGauge() bool {
	return m.showGauge && m.heavyVisualsEnabled() && m.mode == modePreview && len(m.headings) > 0 && m.viewport.Height() > 0
}

func (m model) sectionGaugeWidth() int {
	// Keep gauge compact by default and expand when the pane is wide enough.
	if m.rightWidth() >= 76 {
		return 3
	}
	return 2
}

func (m model) renderSectionGauge() string {
	rows := m.renderSectionGaugeRows()
	if len(rows) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m model) renderSectionGaugeRows() []string {
	if !m.showSectionGauge() {
		return nil
	}
	gaugeWidth := m.sectionGaugeWidth()
	height := m.viewport.Height()
	if height <= 0 {
		return nil
	}
	total := m.renderedLineCount
	if total == 0 {
		return nil
	}

	rows := make([]string, height)
	type barKind uint8
	const (
		barEmpty barKind = iota
		barMuted
		barActive
		barMark
	)
	barRows := make([]barKind, height)
	depthRows := make([]int, height)
	boundaryRows := make([]barKind, height) // uses barMuted/barActive

	empty := m.styles.gaugeMetaEmpty
	mutedBar := m.styles.gaugeMuted
	activeBar := m.styles.gaugeActive
	activeMark := m.styles.gaugeMark
	for i := range rows {
		rows[i] = empty + m.styles.gaugeEmpty
	}

	curLine := m.viewport.YOffset()
	for i, h := range m.headings {
		if h.renderLine < 0 {
			continue
		}
		level := h.level
		if level < 1 {
			level = 1
		}
		if level > 4 {
			level = 4
		}
		start := h.renderLine
		end := total - 1
		if i+1 < len(m.headings) && m.headings[i+1].renderLine >= 0 {
			end = m.headings[i+1].renderLine - 1
		}
		if end < start {
			end = start
		}
		startRow := rowForRenderLine(start, total, height)
		endRow := rowForRenderLine(end, total, height)
		if startRow > endRow {
			startRow, endRow = endRow, startRow
		}
		if level > depthRows[startRow] {
			depthRows[startRow] = level
		}
		if boundaryRows[startRow] < barMuted {
			boundaryRows[startRow] = barMuted
		}

		if i == m.currentHeading {
			sectionLen := max(1, end-start)
			progress := float64(curLine-start) / float64(sectionLen)
			if progress < 0 {
				progress = 0
			}
			if progress > 1 {
				progress = 1
			}
			progressRow := startRow + int(progress*float64(max(1, endRow-startRow)))
			if progressRow > endRow {
				progressRow = endRow
			}
			for r := startRow; r <= progressRow && r < len(rows); r++ {
				if barRows[r] < barActive {
					barRows[r] = barActive
				}
			}
			barRows[startRow] = barMark
			boundaryRows[startRow] = barActive
			continue
		}
		// Preserve active marker/bar when multiple headings collapse to same row.
		if barRows[startRow] < barMuted {
			barRows[startRow] = barMuted
		}
	}

	for i := range rows {
		var barGlyph string
		switch barRows[i] {
		case barMark:
			barGlyph = activeMark
		case barActive:
			barGlyph = activeBar
		case barMuted:
			barGlyph = mutedBar
		default:
			barGlyph = m.styles.gaugeEmpty
		}

		depthGlyph := m.styles.gaugeMetaEmpty
		if depth := depthRows[i]; depth > 0 {
			if barRows[i] >= barActive {
				depthGlyph = m.styles.gaugeDepthActive[depth-1]
			} else {
				depthGlyph = m.styles.gaugeDepthMuted[depth-1]
			}
		}
		boundaryGlyph := m.styles.gaugeMetaEmpty
		switch boundaryRows[i] {
		case barActive:
			boundaryGlyph = m.styles.gaugeBoundaryActive
		case barMuted:
			boundaryGlyph = m.styles.gaugeBoundaryMuted
		}

		if gaugeWidth >= 3 {
			rows[i] = depthGlyph + boundaryGlyph + barGlyph
		} else {
			metaGlyph := depthGlyph
			if boundaryRows[i] == barActive {
				metaGlyph = m.styles.gaugeBoundaryActive
			} else if boundaryRows[i] == barMuted {
				metaGlyph = m.styles.gaugeBoundaryMuted
			}
			rows[i] = metaGlyph + barGlyph
		}
	}

	return rows
}

func rowForRenderLine(line, total, height int) int {
	if total <= 1 || height <= 1 {
		return 0
	}
	if line < 0 {
		line = 0
	}
	if line >= total {
		line = total - 1
	}
	return (line * (height - 1)) / (total - 1)
}

func lineForGaugeRow(row, total, height int) int {
	if total <= 1 || height <= 1 {
		return 0
	}
	if row < 0 {
		row = 0
	}
	if row >= height {
		row = height - 1
	}
	return (row*(total-1) + (height-1)/2) / (height - 1)
}

func isKeyMsg(msg tea.Msg) bool {
	_, ok := msg.(tea.KeyMsg)
	return ok
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (m model) selectedPath() string {
	if item, ok := m.fileList.SelectedItem().(navigatorItem); ok && item.kind == navigatorFileNode {
		return item.path
	}
	return ""
}

func (m model) windowTitle() string {
	base := "markdownviewer"
	if m.currentPath != "" {
		base = filepath.Base(m.currentPath) + " - " + base
	}
	state := "Preview"
	switch {
	case m.opMode != opNone:
		state = "Prompt"
	case m.showCmdPalette:
		state = "Command Palette"
	case m.showThemePicker:
		state = "Theme Picker"
	case m.showSearch:
		state = "Search"
	case m.mode == modeRaw:
		state = "Raw Edit"
	}
	return base + " [" + state + "]"
}

func (m model) activeOverlayLayerID() string {
	switch {
	case m.showHelp:
		return layerIDHelpOverlay
	case m.showThemePicker:
		return layerIDThemeOverlay
	case m.showCmdPalette:
		return layerIDCmdOverlay
	default:
		return ""
	}
}

func (m model) layeredViewContent(content string) (string, func(tea.MouseMsg) tea.Cmd) {
	id := m.activeOverlayLayerID()
	if id == "" {
		return content, nil
	}
	layer := lipgloss.NewLayer(content).ID(id)
	compositor := lipgloss.NewCompositor(layer)
	return compositor.Render(), func(msg tea.MouseMsg) tea.Cmd {
		hit := compositor.Hit(msg.Mouse().X, msg.Mouse().Y)
		if hit.Empty() {
			return nil
		}
		return func() tea.Msg {
			return overlayLayerHitMsg{id: hit.ID(), mouse: msg}
		}
	}
}
