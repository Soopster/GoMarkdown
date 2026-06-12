package main

import (
	"strings"
	"testing"
)

func TestExtractMermaidBlocksReplacesOnlyMermaidFences(t *testing.T) {
	content := strings.Join([]string{
		"# Demo",
		"",
		"```mermaid",
		"graph TD;",
		"    A-->B;",
		"```",
		"",
		"```go",
		`fmt.Println("keep me")`,
		"```",
	}, "\n")

	processed, blocks := extractMermaidBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 mermaid block, got %d", len(blocks))
	}
	if !strings.Contains(processed, blocks[0].token) {
		t.Fatalf("expected processed content to contain placeholder token, got %q", processed)
	}
	if !strings.Contains(processed, "```go") {
		t.Fatalf("expected non-mermaid fence to remain untouched, got %q", processed)
	}
	if got := strings.TrimSpace(blocks[0].source); got != "graph TD;\n    A-->B;" {
		t.Fatalf("unexpected mermaid source %q", got)
	}
}

func TestRenderCurrentContentReplacesMermaidPlaceholderWithDiagram(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"# Demo",
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
	if strings.Contains(msg.content, mermaidPlaceholderPrefix) {
		t.Fatalf("expected placeholder token removed from output, got %q", msg.content)
	}
	if !strings.Contains(msg.content, "Start") || !strings.Contains(msg.content, "Done") {
		t.Fatalf("expected rendered preview to contain Mermaid diagram text, got %q", msg.content)
	}
}

func TestRenderCurrentContentFallsBackOnUnsupportedMermaidSyntax(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.width = 100
	m.height = 24
	m.resizeViews()
	m.sourceContent = strings.Join([]string{
		"# Demo",
		"",
		"```mermaid",
		"stateDiagram-v2",
		"    [*] --> Active",
		"```",
	}, "\n")

	msg := m.renderCurrentContent()().(renderedMsg)
	if msg.err != nil {
		t.Fatalf("unexpected render error: %v", msg.err)
	}
	if !strings.Contains(msg.content, "unsupported") && !strings.Contains(msg.content, "failed to parse") {
		t.Fatalf("expected Mermaid fallback error, got %q", msg.content)
	}
	if !strings.Contains(msg.content, "stateDiagram-v2") {
		t.Fatalf("expected fallback to include Mermaid source, got %q", msg.content)
	}
}

func TestMermaidSupportCachesRenderedOutput(t *testing.T) {
	support := newMermaidSupport()

	first, err := support.render("flowchart TD\nA[One] --> B[Two]", 72, "default")
	if err != nil {
		t.Fatalf("unexpected first render error: %v", err)
	}
	second, err := support.render("flowchart TD\nA[One] --> B[Two]", 72, "default")
	if err != nil {
		t.Fatalf("unexpected second render error: %v", err)
	}
	if first != second {
		t.Fatalf("expected cached output reuse, first=%q second=%q", first, second)
	}
	if got := len(support.cache); got != 1 {
		t.Fatalf("expected one cached Mermaid render, got %d", got)
	}
}

func TestFitMermaidWidthClampsOutput(t *testing.T) {
	rendered := "1234567890\nshort"
	got := fitMermaidWidth(rendered, 5)
	lines := strings.Split(got, "\n")
	if lines[0] != "12345" {
		t.Fatalf("expected first line to be clamped, got %q", lines[0])
	}
	if lines[1] != "short" {
		t.Fatalf("expected second line to remain intact, got %q", lines[1])
	}
}
