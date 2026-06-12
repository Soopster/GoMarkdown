package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func BenchmarkRenderWithScrollbarCachedRows(b *testing.B) {
	palette := paletteForStyle(defaultStyleName)
	styles := buildStyles(palette)

	const (
		totalRows = 6000
		height    = 46
		width     = 108
	)
	rows := make([]string, totalRows)
	widths := make([]int, totalRows)
	for i := 0; i < totalRows; i++ {
		line := fmt.Sprintf("%04d %s", i, strings.Repeat("lorem ipsum ", 8))
		rows[i] = line
		widths[i] = len(line)
	}

	maxStart := totalRows - height
	if maxStart <= 0 {
		maxStart = 1
	}
	lc := &layoutCache{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := (i * 7) % maxStart
		end := start + height
		if end > len(rows) {
			end = len(rows)
		}
		_, _ = renderWithScrollbarRows(
			rows[start:end],
			widths[start:end],
			width,
			height,
			float64(start)/float64(maxStart),
			styles,
			0,
			false,
			lc,
		)
	}
}

func BenchmarkHeadingIndexAtOffsetBinary(b *testing.B) {
	const headingCount = 3000
	lines := make([]int, headingCount)
	indices := make([]int, headingCount)
	for i := 0; i < headingCount; i++ {
		lines[i] = i * 5
		indices[i] = i
	}
	maxY := lines[len(lines)-1] + 4

	var sink int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		y := (i * 13) % maxY
		sink += headingIndexAtOffset(lines, indices, y)
	}
	runtime.KeepAlive(sink)
}
