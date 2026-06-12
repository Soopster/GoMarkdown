package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	calloutPlaceholderPrefix     = "@@MV_CALLOUT_"
	blockMathPlaceholderPrefix   = "@@MV_MATH_BLOCK_"
	blockImagePlaceholderPrefix  = "@@MV_IMAGE_BLOCK_"
	inlineMathPlaceholderPrefix  = "@@MV_MATH_INLINE_"
	inlineImagePlaceholderPrefix = "@@MV_IMAGE_INLINE_"
	footnoteRefPlaceholderPrefix = "@@MV_FOOTNOTE_REF_"
)

var footnoteDefPattern = regexp.MustCompile(`^\[\^([^\]]+)\]:\s*(.*)$`)

type markdownFeatureSet struct {
	callouts     []markdownCallout
	blockMath    []markdownMathBlock
	blockImages  []markdownImage
	inlineMath   []markdownInlineMath
	inlineImages []markdownImage
	footnotes    markdownFootnotes
}

type markdownCallout struct {
	token string
	kind  string
	body  string
}

type markdownMathBlock struct {
	token string
	body  string
}

type markdownInlineMath struct {
	token string
	body  string
}

type markdownImage struct {
	token  string
	alt    string
	target string
	title  string
	inline bool
}

type markdownFootnotes struct {
	defs      []markdownFootnoteDef
	indexByID map[string]int
}

type markdownFootnoteDef struct {
	id    string
	index int
	body  string
}

func extractMarkdownFeatures(content string) (string, markdownFeatureSet) {
	withoutBlocks, features := extractMarkdownBlockFeatures(content)
	withInline := applyInlineMarkdownFeatures(withoutBlocks, &features)
	return withInline, features
}

func extractMarkdownBlockFeatures(content string) (string, markdownFeatureSet) {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	features := markdownFeatureSet{footnotes: markdownFootnotes{indexByID: make(map[string]int)}}
	inFence := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if isFenceDelimiter(trimmed) {
			inFence = !inFence
			out = append(out, line)
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}

		if kind, ok := parseCalloutMarker(line); ok {
			bodyLines := make([]string, 0, 4)
			end := i
			for ; end+1 < len(lines); end++ {
				next := lines[end+1]
				if !isCalloutContinuation(next) {
					break
				}
				bodyLines = append(bodyLines, stripCalloutQuote(next))
			}
			token := fmt.Sprintf("%s%d@@", calloutPlaceholderPrefix, len(features.callouts))
			features.callouts = append(features.callouts, markdownCallout{
				token: token,
				kind:  kind,
				body:  strings.TrimRight(strings.Join(bodyLines, "\n"), "\n"),
			})
			out = append(out, token)
			i = end
			continue
		}

		if trimmed == "$$" {
			end := -1
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "$$" {
					end = j
					break
				}
			}
			if end == -1 {
				out = append(out, line)
				continue
			}
			token := fmt.Sprintf("%s%d@@", blockMathPlaceholderPrefix, len(features.blockMath))
			features.blockMath = append(features.blockMath, markdownMathBlock{
				token: token,
				body:  strings.Join(lines[i+1:end], "\n"),
			})
			out = append(out, token)
			i = end
			continue
		}

		if defID, defBody, ok := parseFootnoteDefinition(line); ok {
			bodyLines := []string{defBody}
			end := i
			for ; end+1 < len(lines); end++ {
				next := lines[end+1]
				if next == "" {
					bodyLines = append(bodyLines, "")
					continue
				}
				if strings.HasPrefix(next, "    ") {
					bodyLines = append(bodyLines, strings.TrimPrefix(next, "    "))
					continue
				}
				if strings.HasPrefix(next, "\t") {
					bodyLines = append(bodyLines, strings.TrimPrefix(next, "\t"))
					continue
				}
				break
			}
			def := markdownFootnoteDef{
				id:    defID,
				index: len(features.footnotes.defs) + 1,
				body:  strings.TrimSpace(strings.Join(bodyLines, "\n")),
			}
			features.footnotes.defs = append(features.footnotes.defs, def)
			features.footnotes.indexByID[defID] = def.index
			i = end
			continue
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n"), features
}

func applyInlineMarkdownFeatures(content string, features *markdownFeatureSet) string {
	lines := strings.Split(content, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFenceDelimiter(trimmed) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if img, ok := parseImageMarkdown(strings.TrimSpace(line)); ok && strings.TrimSpace(line) == img.full {
			img.markdownImage.inline = false
			img.markdownImage.token = fmt.Sprintf("%s%d@@", blockImagePlaceholderPrefix, len(features.blockImages))
			features.blockImages = append(features.blockImages, img.markdownImage)
			lines[i] = img.markdownImage.token
			continue
		}
		line = replaceInlineImages(line, features)
		line = replaceFootnoteRefs(line, features.footnotes)
		line = replaceInlineMath(line, features)
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func renderMarkdownFeatures(renderer *glamour.TermRenderer, features markdownFeatureSet, width int, palette colorPalette, currentPath string) (map[string]string, map[string]string, string) {
	blockReplacements := make(map[string]string, len(features.callouts)+len(features.blockMath)+len(features.blockImages))

	for _, callout := range features.callouts {
		blockReplacements[callout.token] = renderCalloutBlock(renderer, callout, width, palette)
	}
	for _, mathBlock := range features.blockMath {
		blockReplacements[mathBlock.token] = renderMathBlock(mathBlock, width, palette)
	}
	for _, image := range features.blockImages {
		blockReplacements[image.token] = renderImageBlock(image, width, palette, currentPath)
	}

	footnotesSection := renderFootnotesSection(renderer, features.footnotes)
	return blockReplacements, nil, footnotesSection
}

func renderCalloutBlock(renderer *glamour.TermRenderer, callout markdownCallout, width int, palette colorPalette) string {
	body := strings.TrimSpace(callout.body)
	if body == "" {
		body = "No content"
	}
	rendered := body
	if renderer != nil {
		if out, err := renderer.Render(body); err == nil {
			rendered = strings.TrimRight(out, "\n")
		}
	}
	color := palette.border
	upper := strings.ToUpper(callout.kind)
	switch upper {
	case "TIP", "IMPORTANT":
		color = palette.highlight
	case "WARNING", "CAUTION":
		color = palette.warn
	}
	return renderMarkdownPanel(calloutTitle(upper), rendered, width, color)
}

func renderMathBlock(block markdownMathBlock, width int, palette colorPalette) string {
	body := strings.TrimSpace(block.body)
	if body == "" {
		body = "(empty math block)"
	}
	return renderMarkdownPanel("Math", body, width, palette.highlight)
}

func renderImageBlock(image markdownImage, width int, palette colorPalette, currentPath string) string {
	lines := []string{}
	alt := image.alt
	if alt == "" {
		alt = "(no alt text)"
	}
	lines = append(lines, "Alt: "+alt)
	if image.title != "" {
		lines = append(lines, "Title: "+image.title)
	}
	status := describeImageTarget(image.target, currentPath)
	lines = append(lines, "Target: "+image.target)
	if status != "" {
		lines = append(lines, "Status: "+status)
	}
	return renderMarkdownPanel("Image", strings.Join(lines, "\n"), width, palette.border)
}

func renderInlineMath(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		body = "?"
	}
	return "⟪" + body + "⟫"
}

func renderInlineImage(image markdownImage) string {
	alt := image.alt
	if alt == "" {
		alt = image.target
	}
	return "[image: " + alt + "]"
}

func renderFootnoteRef(index int) string {
	return "[" + strconv.Itoa(index) + "]"
}

func renderFootnotesSection(renderer *glamour.TermRenderer, footnotes markdownFootnotes) string {
	if len(footnotes.defs) == 0 || renderer == nil {
		return ""
	}
	var src strings.Builder
	src.WriteString("## Footnotes\n\n")
	for _, def := range footnotes.defs {
		bodyLines := strings.Split(def.body, "\n")
		src.WriteString(strconv.Itoa(def.index))
		src.WriteString(". ")
		if len(bodyLines) > 0 {
			src.WriteString(bodyLines[0])
			src.WriteByte('\n')
			for _, line := range bodyLines[1:] {
				if line == "" {
					src.WriteByte('\n')
					continue
				}
				src.WriteString("   ")
				src.WriteString(line)
				src.WriteByte('\n')
			}
		} else {
			src.WriteByte('\n')
		}
		src.WriteByte('\n')
	}
	out, err := renderer.Render(src.String())
	if err != nil {
		return strings.TrimSpace(src.String())
	}
	return strings.TrimRight(out, "\n")
}

func renderMarkdownPanel(title string, body string, width int, borderColor string) string {
	innerWidth := max(24, width)
	titleLabel := " " + title + " "
	fill := max(0, innerWidth-lipgloss.Width(titleLabel))
	borderPrefix := foregroundSGR(borderColor)
	reset := "\x1b[0m"
	lines := []string{borderPrefix + "╭" + titleLabel + strings.Repeat("─", fill) + "╮" + reset}
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		lines = append(lines, renderMarkdownPanelLine(line, innerWidth, borderPrefix, reset))
	}
	if len(lines) == 1 {
		lines = append(lines, renderMarkdownPanelLine("", innerWidth, borderPrefix, reset))
	}
	lines = append(lines, borderPrefix+"╰"+strings.Repeat("─", innerWidth)+"╯"+reset)
	return strings.Join(lines, "\n")
}

func renderMarkdownPanelLine(content string, width int, borderPrefix string, reset string) string {
	content = strings.TrimRight(content, "\r")
	if lipgloss.Width(stripANSI(content)) > width {
		content = xansi.Truncate(content, width, "")
	}
	pad := max(0, width-lipgloss.Width(stripANSI(content)))
	return borderPrefix + "│" + reset + content + strings.Repeat(" ", pad) + borderPrefix + "│" + reset
}

func replaceMarkdownBlockPlaceholders(rendered string, replacements map[string]string) string {
	if rendered == "" || len(replacements) == 0 {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimSpace(stripANSI(line))
		if replacement, ok := replacements[plain]; ok {
			out = append(out, strings.Split(replacement, "\n")...)
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func replaceMarkdownInlinePlaceholders(rendered string, replacements map[string]string) string {
	if rendered == "" || len(replacements) == 0 {
		return rendered
	}
	for token, replacement := range replacements {
		rendered = strings.ReplaceAll(rendered, token, replacement)
	}
	return rendered
}

func appendMarkdownFootnotes(rendered string, footnotes string) string {
	footnotes = strings.TrimSpace(footnotes)
	if footnotes == "" {
		return rendered
	}
	if strings.TrimSpace(rendered) == "" {
		return footnotes
	}
	return strings.TrimRight(rendered, "\n") + "\n\n" + footnotes
}

func parseCalloutMarker(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, ">") {
		return "", false
	}
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	if !strings.HasPrefix(trimmed, "[!") || !strings.HasSuffix(trimmed, "]") {
		return "", false
	}
	kind := strings.TrimSuffix(strings.TrimPrefix(trimmed, "[!"), "]")
	if kind == "" {
		return "", false
	}
	return kind, true
}

func isCalloutContinuation(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == ">" || strings.HasPrefix(trimmed, "> ") || strings.HasPrefix(trimmed, ">")
}

func stripCalloutQuote(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == ">" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, ">")
	return strings.TrimPrefix(trimmed, " ")
}

func calloutTitle(kind string) string {
	switch kind {
	case "NOTE":
		return "Note"
	case "TIP":
		return "Tip"
	case "IMPORTANT":
		return "Important"
	case "WARNING":
		return "Warning"
	case "CAUTION":
		return "Caution"
	default:
		return strings.Title(strings.ToLower(kind))
	}
}

func parseFootnoteDefinition(line string) (string, string, bool) {
	match := footnoteDefPattern.FindStringSubmatch(line)
	if match == nil {
		return "", "", false
	}
	return match[1], match[2], true
}

func replaceFootnoteRefs(line string, footnotes markdownFootnotes) string {
	if len(footnotes.defs) == 0 || !strings.Contains(line, "[^") {
		return line
	}
	var out strings.Builder
	for i := 0; i < len(line); {
		if line[i] == '[' && i+2 < len(line) && line[i+1] == '^' {
			end := strings.IndexByte(line[i:], ']')
			if end > 2 {
				id := line[i+2 : i+end]
				if idx, ok := footnotes.indexByID[id]; ok {
					out.WriteString(renderFootnoteRef(idx))
					i += end + 1
					continue
				}
			}
		}
		out.WriteByte(line[i])
		i++
	}
	return out.String()
}

func replaceInlineMath(line string, features *markdownFeatureSet) string {
	if !strings.Contains(line, "$") {
		return line
	}
	var out strings.Builder
	inCode := false
	for i := 0; i < len(line); {
		if line[i] == '`' {
			inCode = !inCode
			out.WriteByte(line[i])
			i++
			continue
		}
		if inCode || line[i] != '$' || (i > 0 && line[i-1] == '\\') || (i+1 < len(line) && line[i+1] == '$') {
			out.WriteByte(line[i])
			i++
			continue
		}
		end := i + 1
		for end < len(line) {
			if line[end] == '$' && line[end-1] != '\\' {
				break
			}
			end++
		}
		if end >= len(line) || end == i+1 {
			out.WriteByte(line[i])
			i++
			continue
		}
		out.WriteString(renderInlineMath(line[i+1 : end]))
		i = end + 1
	}
	return out.String()
}

func replaceInlineImages(line string, features *markdownFeatureSet) string {
	for {
		img, ok := parseFirstImage(line)
		if !ok {
			return line
		}
		line = line[:img.start] + renderInlineImage(img.markdownImage) + line[img.end:]
	}
}

type parsedImage struct {
	markdownImage
	full  string
	start int
	end   int
}

func parseStandaloneImage(line string) (parsedImage, bool) { return parseImageMarkdown(line) }

func parseFirstImage(line string) (parsedImage, bool) {
	start := strings.Index(line, "![")
	for start >= 0 {
		img, ok := parseImageMarkdown(line[start:])
		if ok {
			img.start = start
			img.end = start + len(img.full)
			return img, true
		}
		next := strings.Index(line[start+2:], "![")
		if next < 0 {
			break
		}
		start += next + 2
	}
	return parsedImage{}, false
}

func parseImageMarkdown(input string) (parsedImage, bool) {
	if !strings.HasPrefix(input, "![") {
		return parsedImage{}, false
	}
	altEnd := strings.Index(input, "](")
	if altEnd < 0 {
		return parsedImage{}, false
	}
	rest := input[altEnd+2:]
	targetEnd := strings.IndexByte(rest, ')')
	if targetEnd < 0 {
		return parsedImage{}, false
	}
	full := input[:altEnd+2+targetEnd+1]
	alt := input[2:altEnd]
	inside := strings.TrimSpace(rest[:targetEnd])
	target, title := splitImageTarget(inside)
	if target == "" {
		return parsedImage{}, false
	}
	return parsedImage{
		markdownImage: markdownImage{alt: alt, target: target, title: title},
		full:          full,
	}, true
}

func splitImageTarget(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if idx := strings.Index(value, " \""); idx >= 0 && strings.HasSuffix(value, "\"") {
		return strings.TrimSpace(value[:idx]), strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value[idx+1:]), "\""), "\"")
	}
	return value, ""
}

func describeImageTarget(target string, currentPath string) string {
	if target == "" {
		return ""
	}
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "remote URL"
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		base := currentPath
		if base != "" {
			if info, err := os.Stat(base); err == nil {
				if !info.IsDir() {
					base = filepath.Dir(base)
				}
			} else if filepath.Ext(base) != "" {
				base = filepath.Dir(base)
			}
		}
		if base != "" {
			resolved = filepath.Join(base, target)
		}
	}
	if _, err := os.Stat(resolved); err == nil {
		return "local file found"
	}
	return "local file missing"
}

func isFenceDelimiter(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}
