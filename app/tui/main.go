package main

import (
	"log"
	"os/exec"
	"regexp"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/knipferrc/teacup/code"
	"github.com/noelzubin/notes_search/search"
	"github.com/noelzubin/notes_search/search/bleve_indexer"
	"github.com/noelzubin/notes_search/utils"
	"github.com/samber/lo"
)

var ListStyle = lipgloss.NewStyle().MarginTop(1)

type Note struct {
	path    string
	content string
}

func (n Note) Title() string       { return n.path }
func (n Note) Description() string { return n.content }
func (n Note) FilterValue() string { return "" }

type Model struct {
	width     int
	height    int
	notes     []Note
	preview   *code.Bubble
	list      list.Model
	textInput textinput.Model
	indexer   search.NotesIndexer
	results   []search.SearchResult
	editor    Editor
}

func New(indexer search.NotesIndexer, config *utils.Config) *Model {
	ti := textinput.New()
	ti.Placeholder = "query"

	ti.Prompt = "Search:"

	ti.PromptStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		MarginRight(1).
		MarginLeft(2).
		Padding(0, 1)

	ti.Focus()

	mylist := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	mylist.SetShowFilter(false)
	mylist.SetShowHelp(false)
	mylist.SetShowTitle(false)
	mylist.Styles.NoItems = mylist.Styles.NoItems.Copy().PaddingLeft(2)

	return &Model{list: mylist, textInput: ti, indexer: indexer, editor: Editor{editMode: false, app: config.Editor}}
}

func (m *Model) setListSize() {
	width := m.width
	height := m.height

	if m.preview != nil {
		width = m.width / 2
	}

	m.list.SetSize(width, height-2)
}

func (m *Model) setPreviewSize() {
	if m.preview != nil {
		m.preview.SetSize(m.width/2, m.height)
	}
}

func (m *Model) updateSize(width, height int) {
	m.height = height
	m.width = width

	m.setListSize()
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen,
		func() tea.Msg {
			results := m.indexer.Search("")
			return ResultMsg{results}
		},
	)
}

// Formats the content of the file
// removes newslines and replaces tabs with single space.
func formatContent(content string) string {
	s := stripansi.Strip(content)
	s = strings.ReplaceAll(s, "\n", " ↵ ")
	re := regexp.MustCompile(`\s{2,}|\t+`)
	return string(re.ReplaceAll([]byte(s), []byte(" ")))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ResultMsg:
		m.list.SetItems(lo.Map(msg.Hits, func(hit search.DocumentMatch, _ int) list.Item {
			content := formatContent(hit.Content)
			return Note{hit.Path, content}
		}))
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.list.CursorDown()
		case "shift+tab":
			m.list.CursorUp()
		case "enter":
			if m.list.SelectedItem() != nil {
				path := m.list.SelectedItem().(Note).path
				codeModel := code.New(false, true, lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})
				cmds = append(cmds, codeModel.SetFileName(path))
				codeModel.SetSize(m.width/1, m.height)
				m.preview = &codeModel
			}
		case "esc":
			m.preview = nil
		case "ctrl+c":
			return m, tea.Batch(tea.ExitAltScreen, tea.Quit)
		case "ctrl+r":
			return m, func() tea.Msg {
				m.indexer.IndexNotes()
				return nil
			}
		case "ctrl+k":
			m.preview.Viewport.LineUp(5)
		case "ctrl+j":
			m.preview.Viewport.LineDown(5)
		case "ctrl+o":
			if m.list.SelectedItem() != nil {
				path := m.list.SelectedItem().(Note).path
				cmd = m.editor.EditFile(path)
				cmds = append(cmds, cmd)
			}
		default:
			log.Print(msg.String())
		}

	case tea.WindowSizeMsg:
		m.updateSize(msg.Width, msg.Height)
	}

	// m.setTableSize()
	m.setListSize()
	m.setPreviewSize()

	// check if input changed.
	oldValue := m.textInput.Value()

	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)

	m.editor, cmd = m.editor.Update(msg)
	cmds = append(cmds, cmd)

	if m.preview != nil {
		var newPreview code.Bubble
		newPreview, cmd = m.preview.Update(msg)
		cmds = append(cmds, cmd)
		m.preview = &newPreview
	}

	newValue := m.textInput.Value()
	if oldValue != newValue {
		return m, func() tea.Msg {
			results := m.indexer.Search(newValue)
			return ResultMsg{results}
		}
	}

	return m, tea.Batch(cmds...)
}

type ResultMsg struct {
	search.SearchResult
}

func (m Model) View() string {
	listContent := ListStyle.Render(m.list.View())
	innerContent := listContent

	// if preview then preview takes up half the width
	if m.preview != nil {
		innerContent = lipgloss.JoinHorizontal(lipgloss.Left,
			listContent,
			m.preview.View(),
		)
	}

	// render the input box and the content
	return lipgloss.JoinVertical(lipgloss.Left, m.textInput.View(),
		innerContent,
	)
}

func main() {
	// Setup logging.
	f, err := tea.LogToFile("debug.log", "debug")

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	// get a config
	config := utils.NewConfig()

	// create the indexer.
	indexer, err := bleve_indexer.NewBleveIndexer(config)
	if err != nil {
		log.Fatal(err)
	}

	// Create a new Model
	m := New(indexer, config)
	p := tea.NewProgram(m)
	if err := p.Start(); err != nil {
		panic(err)
	}
}

// / EIDTOR
type Editor struct {
	editMode bool
	app      string
}

type editingFinished struct{}

type startEditing struct {
	filepath string
}

func openEditor(app string, args ...string) tea.Cmd {
	return tea.ExecProcess(exec.Command(app, args...), func(err error) tea.Msg {
		return editingFinished{}
	})
}

func (m *Editor) Init() tea.Cmd {
	return nil
}

func (m *Editor) EditFile(filepath string) tea.Cmd {
	m.editMode = true
	tea.HideCursor()
	return openEditor(m.app, filepath)
}

func (m Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if m.editMode {
		return m, nil
	}

	switch msg.(type) {
	case editingFinished:
		m.editMode = false
		return m, nil
	}

	return m, nil
}

func (m Editor) View() string {
	return ""
}
