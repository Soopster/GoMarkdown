package mermaidascii

import (
	"sync"

	"github.com/Soopster/GoMarkdown/internal/mermaidascii/diagram"
)

var (
	renderMu         sync.Mutex
	Coords           bool
	boxBorderPadding = 1
	paddingBetweenX  = 5
	paddingBetweenY  = 5
	graphDirection   = "LR"
)

func applyConfig(config *diagram.Config) {
	if config == nil {
		config = diagram.DefaultConfig()
	}
	Coords = config.ShowCoords
	boxBorderPadding = config.BoxBorderPadding
	paddingBetweenX = config.PaddingBetweenX
	paddingBetweenY = config.PaddingBetweenY
	graphDirection = config.GraphDirection
}
