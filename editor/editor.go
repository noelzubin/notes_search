package editor

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type Editor struct {
	Editing   bool   // Is the editor open
	EditorCmd string // Command to open the editor on shell
}

// Msg for when editor is closed.
type editingFinished struct{}

// this opens up an external editor.
func openEditor(app string, args ...string) tea.Cmd {
	return tea.ExecProcess(exec.Command(app, args...), func(err error) tea.Msg {
		return editingFinished{}
	})
}

func (m *Editor) Init() tea.Cmd {
	return nil
}

func (m *Editor) EditFile(filepath string) tea.Cmd {
	m.Editing = true
	return openEditor(m.EditorCmd, filepath)
}

func (m Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if m.Editing {
		return m, nil
	}

	switch msg.(type) {
	case editingFinished:
		m.Editing = false
		return m, nil
	}

	return m, nil
}

// Doesnt render anything
func (m Editor) View() string {
	return ""
}
