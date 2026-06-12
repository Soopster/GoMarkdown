package main

import "testing"

func TestThemeRegistryIncludesAdditionalThemes(t *testing.T) {
	themes := []struct {
		name  string
		label string
	}{
		{name: solarizedDarkStyle, label: "Solarized Dark"},
		{name: solarizedLightStyle, label: "Solarized Light"},
		{name: everforestStyle, label: "Everforest"},
		{name: kanagawaStyle, label: "Kanagawa"},
	}

	for _, theme := range themes {
		if idx := themeIndex(theme.name); idx < 0 || idx >= len(themeOrder) {
			t.Fatalf("expected %q to be present in themeOrder, got idx=%d", theme.name, idx)
		}
		if got := themeLabel(theme.name); got != theme.label {
			t.Fatalf("expected label %q for %q, got %q", theme.label, theme.name, got)
		}
		pal := paletteForStyle(theme.name)
		if pal.bg == "" || pal.surface == "" || pal.text == "" || pal.highlight == "" {
			t.Fatalf("expected populated palette for %q, got %#v", theme.name, pal)
		}
		if pal == paletteForStyle(defaultStyleName) {
			t.Fatalf("expected %q palette to differ from default", theme.name)
		}
	}
}

func TestNextStyleCyclesThroughAdditionalThemes(t *testing.T) {
	if got := nextStyle(gruvboxStyle); got != solarizedDarkStyle {
		t.Fatalf("expected next theme after %q to be %q, got %q", gruvboxStyle, solarizedDarkStyle, got)
	}
	if got := nextStyle(kanagawaStyle); got != defaultStyleName {
		t.Fatalf("expected next theme after %q to wrap to %q, got %q", kanagawaStyle, defaultStyleName, got)
	}
}
