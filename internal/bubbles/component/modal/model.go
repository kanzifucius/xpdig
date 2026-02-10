package modal

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// EventClosed is sent when the modal is dismissed.
type EventClosed struct{}

type KeyMap struct {
	Close    key.Binding
	LineUp   key.Binding
	LineDown key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Close: key.NewBinding(
			key.WithKeys("esc", "s", "q"),
			key.WithHelp("esc/s/q", "close"),
		),
		LineUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		LineDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
	}
}

type Model struct {
	KeyMap   KeyMap
	Title    string
	Content  string
	Visible  bool
	viewport viewport.Model
	width    int
	height   int
}

func New() Model {
	return Model{
		KeyMap:   DefaultKeyMap(),
		viewport: viewport.New(0, 0),
	}
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Modal takes ~60% width, ~50% height
	modalW := max(width*60/100, 40)
	modalH := max(height*50/100, 6)

	m.viewport.Width = modalW - 4  // padding
	m.viewport.Height = modalH - 4 // border + title + padding
}

func (m *Model) Show(title, content string) {
	m.Title = title
	m.Content = content
	m.Visible = true
	m.viewport.SetContent(m.Content)
	m.viewport.GotoTop()
}

func (m *Model) Hide() {
	m.Visible = false
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Close):
			m.Hide()
			return m, func() tea.Msg { return EventClosed{} }
		case key.Matches(msg, m.KeyMap.LineUp):
			m.viewport.LineUp(1)
		case key.Matches(msg, m.KeyMap.LineDown):
			m.viewport.LineDown(1)
		}
	}

	return m, nil
}

func (m Model) View() string {
	if !m.Visible {
		return ""
	}

	modalW := max(m.width*60/100, 40)
	modalH := max(m.height*50/100, 6)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.ANSIColor(ansi.White)).
		Background(lipgloss.ANSIColor(ansi.Magenta)).
		Width(modalW-2).
		Padding(0, 1)

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.ANSIColor(ansi.BrightBlack)).
		Width(modalW-2).
		Align(lipgloss.Right).
		Padding(0, 1)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.ANSIColor(ansi.Magenta)).
		Width(modalW).
		Height(modalH)

	title := titleStyle.Render(m.Title)
	footer := footerStyle.Render("esc/s/q: close  ↑/↓: scroll")
	body := m.viewport.View()

	// Pad body lines to fill available height
	bodyLines := strings.Count(body, "\n") + 1
	footerLines := 1
	titleLines := 1
	availableLines := modalH - titleLines - footerLines
	if bodyLines < availableLines {
		body += strings.Repeat("\n", availableLines-bodyLines)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
	box := borderStyle.Render(content)

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}
