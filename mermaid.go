package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/Soopster/GoMarkdown/internal/mermaidascii"
	mermaiddiagram "github.com/Soopster/GoMarkdown/internal/mermaidascii/diagram"
	xansi "github.com/charmbracelet/x/ansi"
)

const mermaidPlaceholderPrefix = "@@MV_MERMAID_"

type mermaidBlock struct {
	token  string
	source string
}

type mermaidCacheKey struct {
	sourceHash string
	width      int
	theme      string
}

type mermaidSupport struct {
	mu    sync.Mutex
	cache map[mermaidCacheKey]string
}

func newMermaidSupport() *mermaidSupport {
	return &mermaidSupport{cache: make(map[mermaidCacheKey]string)}
}

func (m *model) mermaidState() *mermaidSupport {
	if m.mermaid == nil {
		m.mermaid = newMermaidSupport()
	}
	return m.mermaid
}

func hashMermaidSource(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func (s *mermaidSupport) render(source string, width int, theme string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("mermaid support unavailable")
	}
	key := mermaidCacheKey{
		sourceHash: hashMermaidSource(source),
		width:      width,
		theme:      theme,
	}
	s.mu.Lock()
	if cached, ok := s.cache[key]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	config := mermaiddiagram.DefaultConfig()
	config.StyleType = "cli"
	config.UseAscii = false
	rendered, err := mermaidascii.RenderDiagram(source, config)
	if err != nil {
		return "", err
	}
	rendered = fitMermaidWidth(rendered, width)

	s.mu.Lock()
	s.cache[key] = rendered
	s.mu.Unlock()
	return rendered, nil
}

func extractMermaidBlocks(content string) (string, []mermaidBlock) {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	blocks := make([]mermaidBlock, 0, 2)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !isMermaidFence(line) {
			out = append(out, line)
			continue
		}

		end := -1
		for j := i + 1; j < len(lines); j++ {
			if isClosingFence(lines[j]) {
				end = j
				break
			}
		}
		if end == -1 {
			out = append(out, line)
			continue
		}

		token := fmt.Sprintf("%s%d@@", mermaidPlaceholderPrefix, len(blocks))
		source := strings.Join(lines[i+1:end], "\n")
		blocks = append(blocks, mermaidBlock{token: token, source: source})
		out = append(out, token)
		i = end
	}

	return strings.Join(out, "\n"), blocks
}

func isMermaidFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "```") {
		return false
	}
	lang := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
	return lang == "mermaid"
}

func isClosingFence(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "```")
}

func mermaidThemeName(palette colorPalette) string {
	bg := strings.TrimPrefix(strings.TrimSpace(palette.bg), "#")
	if len(bg) != 6 {
		return "default"
	}
	switch bg {
	case "000000", "111111", "1e1e1e", "202020":
		return "dark"
	default:
		return "default"
	}
}

func fitMermaidWidth(rendered string, width int) string {
	if width <= 0 || rendered == "" {
		return strings.TrimRight(rendered, "\n")
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		if lipgloss.Width(line) > width {
			line = xansi.Truncate(line, width, "")
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderMermaidReplacements(blocks []mermaidBlock, width int) map[string]string {
	if len(blocks) == 0 {
		return nil
	}
	state := m.mermaidState()
	theme := mermaidThemeName(m.palette)
	replacements := make(map[string]string, len(blocks))
	for _, block := range blocks {
		rendered, err := state.render(block.source, width, theme)
		if err == nil {
			replacements[block.token] = rendered
			continue
		}
		replacements[block.token] = renderMermaidFallback(block.source, width, err.Error())
	}
	return replacements
}

func replaceMermaidPlaceholders(rendered string, replacements map[string]string) string {
	if rendered == "" || len(replacements) == 0 {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimSpace(stripANSI(line))
		replaced := false
		for token, replacement := range replacements {
			if plain == token {
				out = append(out, strings.Split(replacement, "\n")...)
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func renderMermaidFallback(source string, width int, status string) string {
	innerWidth := max(24, width)
	title := " Mermaid "
	bodyWidth := max(1, innerWidth-2)
	topFill := max(0, innerWidth-lipgloss.Width(title))
	lines := []string{"╭" + title + strings.Repeat("─", topFill) + "╮"}

	if status != "" {
		lines = append(lines, renderMermaidBoxLine(" "+status, bodyWidth))
		lines = append(lines, renderMermaidBoxLine("", bodyWidth))
	}
	for _, line := range strings.Split(source, "\n") {
		if line == "" {
			lines = append(lines, renderMermaidBoxLine("", bodyWidth))
			continue
		}
		lines = append(lines, renderMermaidBoxLine(line, bodyWidth))
	}
	lines = append(lines, "╰"+strings.Repeat("─", innerWidth)+"╯")
	return strings.Join(lines, "\n")
}

func renderMermaidBoxLine(content string, width int) string {
	truncated := xansi.Truncate(content, width, "")
	pad := max(0, width-lipgloss.Width(truncated))
	return "│" + truncated + strings.Repeat(" ", pad) + "│"
}
