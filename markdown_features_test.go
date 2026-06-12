package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRenderCurrentContentRendersGitHubCallout(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"# Demo",
		"",
		"> [!NOTE]",
		"> This is important.",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "Note") || !strings.Contains(plain, "This is important.") {
		t.Fatalf("expected rendered callout panel, got %q", plain)
	}
	if strings.Contains(plain, "[!NOTE]") {
		t.Fatalf("expected callout marker to be transformed, got %q", plain)
	}
}

func TestRenderCurrentContentAppendsFootnotesSection(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"Footnote ref[^a] in body.",
		"",
		"[^a]: This is the footnote text.",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "Footnotes") || !strings.Contains(plain, "This is the footnote text.") {
		t.Fatalf("expected rendered footnotes section, got %q", plain)
	}
	if !strings.Contains(plain, "[1]") {
		t.Fatalf("expected numbered footnote reference, got %q", plain)
	}
}

func TestRenderCurrentContentRendersMath(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"Inline math $a^2+b^2=c^2$ should be highlighted.",
		"",
		"$$",
		"x = \\frac{-b \\pm \\sqrt{b^2 - 4ac}}{2a}",
		"$$",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "⟪a^2+b^2=c^2⟫") {
		t.Fatalf("expected inline math replacement, got %q", plain)
	}
	if !strings.Contains(plain, "Math") || !strings.Contains(plain, "\\frac") {
		t.Fatalf("expected block math panel, got %q", plain)
	}
}

func TestRenderCurrentContentRendersImageBlock(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "diagram.png")
	if err := os.WriteFile(imgPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.currentPath = filepath.Join(tmp, "note.md")
	m.resizeViews()
	m.sourceContent = "![Architecture](diagram.png)"

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "Image") || !strings.Contains(plain, "Alt: Architecture") {
		t.Fatalf("expected image panel, got %q", plain)
	}
	if !strings.Contains(plain, "Status: local file found") {
		t.Fatalf("expected local image existence status, got %q", plain)
	}
}

func TestRenderCurrentContentMixesMarkdownFeaturesWithMermaid(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"> [!TIP]",
		"> Mermaid should still render below.",
		"",
		"```mermaid",
		"flowchart TD",
		"    A[Start] --> B[Done]",
		"```",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "Tip") || !strings.Contains(plain, "Mermaid should still render below.") {
		t.Fatalf("expected callout to render, got %q", plain)
	}
	if !strings.Contains(plain, "Start") || !strings.Contains(plain, "Done") {
		t.Fatalf("expected Mermaid diagram to still render, got %q", plain)
	}
}

func TestRenderCurrentContentCleansUpFencedCodeBlocksWithLanguage(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"Before",
		"",
		"```go",
		`fmt.Println("hi")`,
		"```",
		"",
		"After",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, " go ") {
		t.Fatalf("expected cleaned code block header, got %q", plain)
	}
	if !strings.Contains(plain, `fmt.Println("hi")`) {
		t.Fatalf("expected code body to render, got %q", plain)
	}
	if strings.Contains(plain, "```") || strings.Contains(plain, "╶─") || strings.Contains(plain, "─╴") {
		t.Fatalf("expected old fence ornamentation to be removed, got %q", plain)
	}
}

func TestRenderCurrentContentCleansUpFencedCodeBlocksWithoutHeaderArtifact(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"```",
		"alpha",
		"beta",
		"```",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "alpha") || !strings.Contains(plain, "beta") {
		t.Fatalf("expected unlabeled code block body to render, got %q", plain)
	}
	if strings.Contains(plain, "```") || strings.Contains(plain, "╶─") || strings.Contains(plain, "─╴") {
		t.Fatalf("expected unlabeled block to render without old ornamentation, got %q", plain)
	}
}

func TestRenderCurrentContentLeavesInlineCodeUntouched(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = "Use `go test ./...` here."

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "go test ./...") {
		t.Fatalf("expected inline code to remain visible, got %q", plain)
	}
	if strings.Contains(plain, "╶─") || strings.Contains(plain, "─╴") {
		t.Fatalf("did not expect fenced code chrome for inline code, got %q", plain)
	}
}

func TestRenderCurrentContentCodePlainKeepsCleanFencedCodeBlocks(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.codePlain = true
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"```sh",
		"echo hi",
		"```",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, " sh ") || !strings.Contains(plain, "echo hi") {
		t.Fatalf("expected plain-mode code block to stay readable, got %q", plain)
	}
	if strings.Contains(plain, "╶─") || strings.Contains(plain, "─╴") {
		t.Fatalf("expected plain-mode code block to avoid old ornamentation, got %q", plain)
	}
}

func TestRenderCurrentContentSyntaxModeKeepsMoreCodeColoringThanPlainMode(t *testing.T) {
	source := strings.Join([]string{
		"```go",
		`func main() { fmt.Println("color") }`,
		"```",
	}, "\n")

	syntax := testModelNoWatcher()
	syntax.width = 100
	syntax.height = 24
	syntax.resizeViews()
	syntax.sourceContent = source
	syntaxMsg := syntax.renderCurrentContent()().(renderedMsg)
	if syntaxMsg.err != nil {
		t.Fatalf("unexpected syntax render error: %v", syntaxMsg.err)
	}

	plain := testModelNoWatcher()
	plain.width = 100
	plain.height = 24
	plain.codePlain = true
	plain.resizeViews()
	plain.sourceContent = source
	plainMsg := plain.renderCurrentContent()().(renderedMsg)
	if plainMsg.err != nil {
		t.Fatalf("unexpected plain render error: %v", plainMsg.err)
	}

	syntaxText := stripANSI(syntaxMsg.content)
	plainText := stripANSI(plainMsg.content)
	if syntaxText != plainText {
		t.Fatalf("expected syntax and plain modes to keep the same visible text, got syntax=%q plain=%q", syntaxText, plainText)
	}
	syntaxStyles := countForegroundOrAttributeSGR(syntaxMsg.content)
	plainStyles := countForegroundOrAttributeSGR(plainMsg.content)
	if syntaxStyles <= plainStyles {
		t.Fatalf("expected syntax mode to retain more ANSI styling than plain mode, got syntax=%d plain=%d", syntaxStyles, plainStyles)
	}
}

func countForegroundOrAttributeSGR(s string) int {
	count := 0
	for {
		start := strings.Index(s, "\x1b[")
		if start < 0 {
			return count
		}
		s = s[start+2:]
		end := strings.IndexByte(s, 'm')
		if end < 0 {
			return count
		}
		if hasForegroundOrAttributeSGR(s[:end]) {
			count++
		}
		s = s[end+1:]
	}
}

func hasForegroundOrAttributeSGR(params string) bool {
	for _, part := range strings.Split(params, ";") {
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if n == 1 || n == 2 || n == 3 || n == 4 || n == 7 || n == 9 || n == 22 || n == 23 || n == 24 || n == 27 || n == 29 {
			return true
		}
		if n == 38 || (n >= 30 && n <= 37) || (n >= 90 && n <= 97) {
			return true
		}
	}
	return false
}

func TestRenderCurrentContentWrapsLongCodeLinesInPreviewCards(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 34
	m.height = 24
	m.resizeViews()
	longLine := `const previewLine = "abcdefghijklmnopqrstuvwxyz0123456789"`
	m.sourceContent = strings.Join([]string{
		"```ts",
		longLine,
		"```",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "previewLine =") || !strings.Contains(plain, "789\"") {
		t.Fatalf("expected wrapped output to preserve both start and end of the long line, got %q", plain)
	}
	nonEmpty := 0
	for _, line := range strings.Split(plain, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	if nonEmpty < 4 {
		t.Fatalf("expected code block to span multiple visible rows after wrapping, got %q", plain)
	}
}

func TestRenderCurrentContentWrapsLongCodeLinesInPlainMode(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 32
	m.height = 24
	m.codePlain = true
	m.resizeViews()
	longLine := "print('this-preview-code-line-should-wrap-cleanly')"
	m.sourceContent = strings.Join([]string{
		"```python",
		longLine,
		"```",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	plain := stripANSI(msg.content)
	if !strings.Contains(plain, "print('this-") || !strings.Contains(plain, "cleanly')") {
		t.Fatalf("expected plain-mode wrapped output to preserve both start and end of the long line, got %q", plain)
	}
	nonEmpty := 0
	for _, line := range strings.Split(plain, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	if nonEmpty < 3 {
		t.Fatalf("expected plain-mode code block to span multiple visible rows after wrapping, got %q", plain)
	}
}

func TestRenderCurrentContentInvalidatesCacheWhenCodeBlockBodyChanges(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modeSplit
	m.width = 100
	m.height = 24
	m.resizeViews()

	m.sourceContent = strings.Join([]string{
		"Before",
		"",
		"```go",
		`fmt.Println("before")`,
		"```",
	}, "\n")
	first := m.renderCurrentContent()().(renderedMsg)
	if first.err != nil {
		t.Fatalf("unexpected initial render error: %v", first.err)
	}
	updated, cmd, handled := m.handleAsyncUpdate(first)
	if !handled {
		t.Fatal("expected rendered message to be handled")
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command from initial render")
	}
	m = updated

	m.sourceContent = strings.Join([]string{
		"Before",
		"",
		"```go",
		`fmt.Println("after")`,
		"```",
	}, "\n")
	second := m.renderCurrentContent()().(renderedMsg)
	if second.err != nil {
		t.Fatalf("unexpected second render error: %v", second.err)
	}
	plain := stripANSI(second.content)
	if !strings.Contains(plain, `fmt.Println("after")`) {
		t.Fatalf("expected updated code block body in second render, got %q", plain)
	}
	if strings.Contains(plain, `fmt.Println("before")`) {
		t.Fatalf("expected stale code block body to be evicted from cache, got %q", plain)
	}
}

func TestRenderCurrentContentInvalidatesCacheWhenCalloutBodyChanges(t *testing.T) {
	m := testModelNoWatcher()
	m.width = 100
	m.height = 24
	m.resizeViews()

	m.sourceContent = strings.Join([]string{
		"> [!NOTE]",
		"> before",
	}, "\n")
	first := m.renderCurrentContent()().(renderedMsg)
	if first.err != nil {
		t.Fatalf("unexpected initial render error: %v", first.err)
	}
	updated, cmd, handled := m.handleAsyncUpdate(first)
	if !handled {
		t.Fatal("expected rendered message to be handled")
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command from initial render")
	}
	m = updated

	m.sourceContent = strings.Join([]string{
		"> [!NOTE]",
		"> after",
	}, "\n")
	second := m.renderCurrentContent()().(renderedMsg)
	if second.err != nil {
		t.Fatalf("unexpected second render error: %v", second.err)
	}
	plain := stripANSI(second.content)
	if !strings.Contains(plain, "after") {
		t.Fatalf("expected updated callout body in second render, got %q", plain)
	}
	if strings.Contains(plain, "before") {
		t.Fatalf("expected stale callout body to be evicted from cache, got %q", plain)
	}
}

func TestRenderPreviewCodeCardBorderlessOmitsRightPadding(t *testing.T) {
	styles := buildStyles(paletteForStyle(defaultStyleName))
	card := renderPreviewCodeCard([]string{"echo hi"}, "sh", 20, styles, true)
	lines := strings.Split(stripANSI(card), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header and body rows, got %q", lines)
	}
	if lines[0] != " sh " {
		t.Fatalf("expected compact borderless header, got %q", lines[0])
	}
	if strings.TrimRight(lines[1], " ") != lines[1] {
		t.Fatalf("expected borderless code row to avoid right padding, got %q", lines[1])
	}
	if !strings.Contains(card, styles.sgrBgHighlight) {
		t.Fatalf("expected borderless header to use highlight styling")
	}
}
