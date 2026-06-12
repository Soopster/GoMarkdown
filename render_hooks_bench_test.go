package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func benchmarkMarkdown(sections int, bodyLines int) string {
	var b strings.Builder
	b.Grow(sections * bodyLines * 80)
	for s := 0; s < sections; s++ {
		fmt.Fprintf(&b, "# Section %03d\n\n", s)
		for i := 0; i < bodyLines; i++ {
			fmt.Fprintf(&b, "Line %03d in section %03d with lorem ipsum dolor sit amet and fast scrolling targets.\n", i, s)
			if i > 0 && i%10 == 0 {
				b.WriteString("```go\n")
				b.WriteString("func benchmarkHook(value int) int {\n")
				b.WriteString("    return value * 2\n")
				b.WriteString("}\n")
				b.WriteString("```\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func newBenchPreviewModel(content string, width int, height int) *model {
	m := newModel()
	if m.watcher != nil {
		_ = m.watcher.Close()
		m.watcher = nil
	}
	m.width = width
	m.height = height
	m.mode = modePreview
	m.fullScreen = true
	m.focusRight = true
	m.richPreview = true
	m.showGauge = true
	m.perfVisualMode = perfVisualForceOn
	m.textarea.SetValue(content)
	m.sourceContent = content
	m.headings = buildOutlineItems(parseHeadings(content))
	if len(m.headings) > 0 {
		m.setCurrentHeading(0)
	}
	m.resizeViews()
	return &m
}

func mustRenderBenchmark(b *testing.B, m *model) renderedMsg {
	b.Helper()
	cmd := m.renderCurrentContent()
	if cmd == nil {
		b.Fatal("renderCurrentContent returned nil command")
	}
	msgAny := cmd()
	msg, ok := msgAny.(renderedMsg)
	if !ok {
		b.Fatalf("expected renderedMsg, got %T", msgAny)
	}
	if msg.err != nil {
		b.Fatal(msg.err)
	}
	return msg
}

func primeRenderCachesBenchmark(m *model, msg renderedMsg) {
	m.lastAnnotateContent = m.textarea.Value()
	m.lastAnnotateWidth = msg.width
	m.lastAnnotateOutput = msg.annotated
	m.lastRenderCacheKey = msg.cacheKey
	m.lastRenderWidth = msg.width
	m.lastRenderLineNums = msg.lineNums
	m.lastRenderReading = msg.readingMode
	m.lastRenderRich = msg.richPreview
	m.lastRenderCode = msg.codePlain
	m.lastRenderStyle = msg.styleName
	m.lastRenderOutput = msg.content
}

func BenchmarkRenderHooksCurrentContent(b *testing.B) {
	sizes := []struct {
		name     string
		sections int
		lines    int
	}{
		{name: "medium", sections: 40, lines: 8},
		{name: "large", sections: 180, lines: 10},
	}

	for _, sz := range sizes {
		doc := benchmarkMarkdown(sz.sections, sz.lines)

		b.Run("cold_"+sz.name, func(b *testing.B) {
			m := newBenchPreviewModel(doc, 160, 52)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.lastAnnotateContent = ""
				m.lastAnnotateOutput = ""
				m.lastRenderCacheKey = ""
				m.lastRenderWidth = 0
				m.lastRenderLineNums = false
				m.lastRenderReading = false
				m.lastRenderRich = false
				m.lastRenderCode = false
				m.lastRenderStyle = ""
				m.invalidateRenderCaches()
				_ = mustRenderBenchmark(b, m)
			}
		})

		b.Run("warm_"+sz.name, func(b *testing.B) {
			m := newBenchPreviewModel(doc, 160, 52)
			msg := mustRenderBenchmark(b, m)
			primeRenderCachesBenchmark(m, msg)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = mustRenderBenchmark(b, m)
			}
		})
	}
}

func BenchmarkRenderHooksPostProcessing(b *testing.B) {
	doc := benchmarkMarkdown(120, 10)
	m := newBenchPreviewModel(doc, 160, 52)
	msg := mustRenderBenchmark(b, m)
	m.rendered = msg.content
	m.updateHeadingOffsets()
	currentHeading := 0
	if len(m.headings) > 1 {
		currentHeading = 1
	}

	hits, queryLen := findSearchMatches(strings.Split(msg.content, "\n"), "lorem")
	if len(hits) == 0 {
		b.Fatal("expected search hits for benchmark fixture")
	}
	currentMatch := len(hits) / 2
	highlightLine := hits[currentMatch].line

	b.Run("search_highlights", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = applySearchHighlights(msg.content, hits, currentMatch, queryLen, m.styles.sgrSearchPri, m.styles.sgrSearchSec)
		}
	})

	b.Run("focus_dim", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = applyFocusDim(msg.content, m.headings, currentHeading, m.styles.sgrDimPrefix)
		}
	})

	b.Run("line_highlight", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = applyHighlight(msg.content, highlightLine, m.styles.sgrHighlight)
		}
	})

	b.Run("heading_offset_scan", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.rendered = msg.content
			m.updateHeadingOffsets()
		}
	})
}

func BenchmarkRenderHooksLayoutAndView(b *testing.B) {
	doc := benchmarkMarkdown(220, 10)
	m := newBenchPreviewModel(doc, 180, 54)
	msg := mustRenderBenchmark(b, m)
	m.rendered = msg.content
	m.updateHeadingOffsets()
	m.setViewportContent(msg.content)
	m.viewport.SetYOffset(0)
	maxOffset := len(m.viewportLines) - m.viewport.Height()
	if maxOffset < 1 {
		maxOffset = 1
	}

	b.Run("layout_scroll", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			offset := (i * 11) % maxOffset
			m.viewport.SetYOffset(offset)
			m.syncCurrentHeading(offset)
			_ = m.renderLayout()
		}
	})

	b.Run("view_heavy_visuals_on", func(b *testing.B) {
		m.perfVisualMode = perfVisualForceOn
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = m.View()
		}
	})

	b.Run("view_heavy_visuals_off", func(b *testing.B) {
		m.perfVisualMode = perfVisualForceOff
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = m.View()
		}
	})
}

func BenchmarkRenderHooksHeadingLookup(b *testing.B) {
	const headingCount = 5000
	lines := make([]int, headingCount)
	indices := make([]int, headingCount)
	for i := 0; i < headingCount; i++ {
		lines[i] = i * 4
		indices[i] = i
	}
	maxY := lines[len(lines)-1] + 4

	var sink int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		y := (i * 17) % maxY
		sink += headingIndexAtOffset(lines, indices, y)
	}
	runtime.KeepAlive(sink)
}
