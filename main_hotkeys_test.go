package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type compatKeyMsg struct {
	key tea.Key
}

func (k compatKeyMsg) String() string {
	return k.key.String()
}

func (k compatKeyMsg) Key() tea.Key {
	return k.key
}

func TestUpdateHandlesCompatKeyMsgQuit(t *testing.T) {
	m := testModelNoWatcher()

	_, cmd := m.Update(compatKeyMsg{key: tea.Key{Code: 'q', Text: "q"}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit message, got %T", cmd())
	}
}

func TestUpdateHandlesCompatKeyMsgToggleFullScreen(t *testing.T) {
	m := testModelNoWatcher()
	m.mode = modePreview
	m.focusRight = true
	m.fullScreen = false
	m.width = 100
	m.height = 24
	m.resizeViews()

	updatedAny, _ := m.Update(compatKeyMsg{key: tea.Key{Code: 'f', Text: "f"}})
	updated := updatedAny.(model)
	if !updated.fullScreen {
		t.Fatal("expected fullscreen toggle hotkey to enable fullscreen")
	}
}
