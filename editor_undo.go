package main

import tea "charm.land/bubbletea/v2"

func editAppendHistory(stack []editUndoEntry, e editUndoEntry) []editUndoEntry {
	if len(stack) > 0 && stack[len(stack)-1] == e {
		return stack
	}
	stack = append(stack, e)
	if len(stack) > editUndoMaxEntries {
		stack = stack[1:]
	}
	return stack
}

func editUndoKeyCoalesceKind(msg tea.KeyPressMsg) editUndoCoalesceKind {
	if msg.Text != "" {
		return editUndoCoalesceInsert
	}
	switch msg.String() {
	case "backspace":
		return editUndoCoalesceDeleteBackward
	case "delete":
		return editUndoCoalesceDeleteForward
	default:
		return editUndoCoalesceNone
	}
}

func (m *model) editCurrentEntry() editUndoEntry {
	return editUndoEntry{m.textarea.Value(), m.textarea.Line(), m.editCursorCol()}
}

func (m *model) editApplyEntry(e editUndoEntry) {
	m.textarea.SetValue(e.content)
	m.editSetCursor(e.row, e.col)
	m.editClearSelection()
}

func (m *model) editPushUndo() {
	m.editResetUndoCoalescing()
	m.editUndoStack = editAppendHistory(m.editUndoStack, m.editCurrentEntry())
	m.editRedoStack = nil
}

func (m *model) editResetUndoCoalescing() {
	m.editUndoCoalesceKind = editUndoCoalesceNone
}

func (m *model) editBeginUndoCoalesced(kind editUndoCoalesceKind) {
	if kind == editUndoCoalesceNone {
		m.editPushUndo()
		return
	}
	if m.editUndoCoalesceKind != kind {
		m.editPushUndo()
	}
	m.editUndoCoalesceKind = kind
}

func (m *model) editUndo() bool {
	if len(m.editUndoStack) == 0 {
		return false
	}
	m.editResetUndoCoalescing()
	m.editRedoStack = editAppendHistory(m.editRedoStack, m.editCurrentEntry())
	e := m.editUndoStack[len(m.editUndoStack)-1]
	m.editUndoStack = m.editUndoStack[:len(m.editUndoStack)-1]
	m.editApplyEntry(e)
	return true
}

func (m *model) editRedo() bool {
	if len(m.editRedoStack) == 0 {
		return false
	}
	m.editResetUndoCoalescing()
	m.editUndoStack = editAppendHistory(m.editUndoStack, m.editCurrentEntry())
	e := m.editRedoStack[len(m.editRedoStack)-1]
	m.editRedoStack = m.editRedoStack[:len(m.editRedoStack)-1]
	m.editApplyEntry(e)
	return true
}
