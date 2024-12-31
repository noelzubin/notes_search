package main

import (
	"log"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/knipferrc/teacup/code"
	"github.com/noelzubin/notes_search/editor"
	"github.com/noelzubin/notes_search/search"
	"github.com/noelzubin/notes_search/search/bleve_indexer"
	"github.com/noelzubin/notes_search/utils"
	"github.com/samber/lo"
)

var ListStyle = lipgloss.NewStyle().MarginTop(1)

// Main app model for bubbletea
type Model struct {
	width        int                 // height of terminal
	height       int                 // width of terminal
	preview      *code.Bubble        // the preview widget model
	list         list.Model          // the list widget model
	textInput    textinput.Model     // the input search widget model
	indexer      search.NotesIndexer // the indexer for searching and indexing notes.
	editor       editor.Editor       // for opening up external editor.
	isQueryValid bool                // if the query is valid
	queryId      int                 // Unique id for the query.
}

// Create a new model for the app
func New(indexer search.NotesIndexer, config *utils.Config) *Model {
	return &Model{
		list:         create_list_model(),
		textInput:    create_text_input(),
		indexer:      indexer,
		editor:       editor.Editor{Editing: false, EditorCmd: config.Editor},
		isQueryValid: false,
		queryId:      0,
	}
}

func (m *Model) setListSize() {
	width := m.width
	height := m.height

	// If preview is open take half width
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
			return ResultMsg{results: results, queryId: 0}
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

// The update fn for the bubbletea model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ResultMsg:
		// Ignore this slow result
		if m.queryId != msg.queryId {
			return m, nil
		}

		text_style := lipgloss.Color("255")
		if msg.results.Err != nil {
			text_style = lipgloss.Color("9")
		}

		m.textInput.TextStyle = lipgloss.NewStyle().Foreground(text_style)
		m.list.SetItems(lo.Map(msg.results.Hits, func(hit search.DocumentMatch, _ int) list.Item {
			content := formatContent(hit.Content)
			return Note{hit.Path, content}
		}))
	case tea.KeyMsg:
		// Keybindings:
		// Tab - move down in the list
		// Shift+Tab - move up in the list
		// Enter - toggle preview for the selected note
		// Esc - close preview
		// Ctrl+R - refresh the index
		// Ctrl+K - Preview lineup
		// Ctrl+J - Preview line down
		// Ctrl+O - Open the file in the editor
		// Ctrl+C - quit the application
		switch msg.String() {
		case "tab":
			m.list.CursorDown()
		case "shift+tab":
			m.list.CursorUp()
		case "enter":
			if m.list.SelectedItem() != nil {
				path := m.list.SelectedItem().(Note).path
				codeModel := code.New(false, true, lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})
				codeModel.SetSize(m.width/1, m.height)
				cmds = append(cmds, codeModel.SetFileName(path))
				m.preview = &codeModel
			}
		case "esc":
			m.preview = nil
		case "ctrl+c":
			return m, tea.Quit
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
				m.indexer.CloseIndex()
				cmd = m.editor.EditFile(path)
				cmds = append(cmds, cmd)
			}
		default:
			log.Print(msg.String())
		}
	case editor.EditingFinished:
		m.indexer.OpenIndex()
	case tea.WindowSizeMsg:
		m.updateSize(msg.Width, msg.Height)
	}

	// Update the widgets sizes
	m.setListSize()
	m.setPreviewSize()

	// save to commpare if changed
	oldValue := m.textInput.Value()

	// pass on message to the other components
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

	// If input has changed, search for the new value
	newValue := m.textInput.Value()
	if oldValue != newValue {
		// This returns a funciton that returns a message(ResultMsg) eventually
		m.queryId++
		return m, func() tea.Msg {
			results := m.indexer.Search(newValue)
			return ResultMsg{results: results, queryId: m.queryId}
		}
	}

	return m, tea.Batch(cmds...)
}

// This is emitted when new events are fetchenew events are fetched
type ResultMsg struct {
	results search.SearchResult
	queryId int
}

// View fn for bubbletea model
func (m Model) View() string {
	listContent := ListStyle.Render(m.list.View())

	// render list
	innerContent := listContent

	// if preview then preview takes up half the width
	if m.preview != nil {
		innerContent = lipgloss.JoinHorizontal(lipgloss.Left,
			listContent,      // render list
			m.preview.View(), // render preview.
		)
	}

	// render the input box and the content
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.textInput.View(), // render the text input
		innerContent,       // render the main content
	)
}

func main() {
	// Setup logging.
	homedir, _ := os.UserHomeDir()
	log_path := path.Join(homedir, "/.config/notes_search/debug.log")
	f, err := tea.LogToFile(log_path, "debug")
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	// read application config
	config := utils.NewConfig()

	// create the indexer.
	indexer, err := bleve_indexer.NewBleveIndexer(config)
	if err != nil {
		log.Fatal(err)
	}

	// Create a new bubbletea Model
	m := New(&indexer, config)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		panic(err)
	}
}

// Note implements list.Item interface
type Note struct {
	path    string
	content string
}

func (n Note) Title() string       { return n.path }
func (n Note) Description() string { return format_string(n.content) }
func (n Note) FilterValue() string { return "" }

// Create the list model
func create_list_model() list.Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.Styles.NoItems = l.Styles.NoItems.Copy().PaddingLeft(2)
	return l
}

// Create the text input model
func create_text_input() textinput.Model {
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
	return ti
}

// format the string returned by bleve with simple highligting
// example: "This is a <mark>test</mark> string" -> "This is a test string" with
// test in pink.
func format_string(input string) string {
	re := regexp.MustCompile(`<mark>(.*?)</mark>`)

	matches := re.FindAllStringSubmatchIndex(input, -1)

	var result []string
	prevIndex := 0

	pinkText := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	grayText := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	for _, match := range matches {
		// Append the text before the match
		if match[0] > prevIndex {
			result = append(result, grayText.Render(input[prevIndex:match[0]]))
		}

		// Append the matched text
		result = append(result, pinkText.Render(input[match[0]+6:match[1]-7]))

		// Update the previous index to the end of the match
		prevIndex = match[1]
	}

	if prevIndex < len(input) {
		result = append(result, grayText.Render(input[prevIndex:]))
	}

	return strings.Join(result, "")
}
