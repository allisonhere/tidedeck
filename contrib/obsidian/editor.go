package main

import (
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/allisonhere/ripple"

	"github.com/allisonhere/tideui"
)

// editorModel is a full-screen Ripple editor for one note. ctrl+s saves and
// leaves, esc leaves without saving; in vim mode Ripple's own `:w` and `:q`
// arrive as SubmitMsg and CancelMsg and mean the same thing.
type editorModel struct {
	vault  *Vault
	rel    string
	ed     ripple.Model
	width  int
	height int
	status string
}

func newEditorModel(vault *Vault, rel string, body []byte, mode string) editorModel {
	ed := ripple.New()
	ed.SetClipboard(systemClipboard{})
	ed.SetValue(string(body))
	ed.SetPlaceholder("Write the note. ctrl+s saves, esc leaves.")
	if mode == "vim" {
		ed.SetInputMode(ripple.ModeVim)
	}
	return editorModel{
		vault: vault, rel: rel, ed: ed,
		status: "ctrl+s save · esc cancel",
	}
}

func (m editorModel) Init() tea.Cmd { return nil }

func (m editorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ed.SetSize(max(20, m.width-4), max(3, m.height-4))
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlS {
			return m.save()
		}
		// Esc cancels in plain mode. In vim mode Esc belongs to Ripple (leave
		// insert, then emit CancelMsg from Normal), so it is forwarded.
		if msg.Type == tea.KeyEsc && m.ed.InputMode() != ripple.ModeVim {
			return m, tea.Quit
		}

	case ripple.SubmitMsg: // vim :w / :wq / :x
		return m.save()

	case ripple.CancelMsg: // vim :q / double-Esc
		return m, tea.Quit

	case ripple.CopiedMsg:
		if msg.Err != nil {
			m.status = "copy failed: " + msg.Err.Error()
		}
		return m, nil

	case ripple.PasteMsg:
		if msg.Err != nil {
			m.status = "paste failed: " + msg.Err.Error()
		}
	}

	var cmd tea.Cmd
	m.ed, cmd = m.ed.Update(msg)
	return m, cmd
}

// save writes the note and leaves. A failed save keeps the editor open with the
// reason on the status line, so the text is never lost to a bad path.
func (m editorModel) save() (tea.Model, tea.Cmd) {
	if err := saveNote(m.vault, m.rel, m.ed.Value()); err != nil {
		m.status = "save failed: " + err.Error()
		return m, nil
	}
	return m, tea.Quit
}

// saveNote is the write path on its own, so a test can prove what ctrl+s writes
// without a terminal.
func saveNote(vault *Vault, rel, body string) error {
	return WriteNote(vault, rel, []byte(body))
}

func (m editorModel) View() string {
	r := themeRenderer()
	ws := r.Styles.Workspace
	cursor := lipgloss.NewStyle().Background(ws.BodyFg).Foreground(ws.Bg).Render(" ")
	selected := lipgloss.NewStyle().Background(ws.SelectionBg).Foreground(ws.SelectionFg)
	placeholder := lipgloss.NewStyle().Foreground(ws.HintFg)

	opts := ripple.Options{
		Cursor:      cursor,
		Selected:    func(s string) string { return selected.Render(s) },
		Placeholder: func(s string) string { return placeholder.Render(s) },
	}
	frame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ws.FrameActive).
		Width(max(20, m.width-2)).
		Render(m.ed.View(opts))
	header := lipgloss.NewStyle().Bold(true).Foreground(ws.BodyFg).Render("Obsidian · " + m.rel)
	status := lipgloss.NewStyle().Foreground(ws.HintFg).Render(m.status)
	return strings.Join([]string{header, frame, status}, "\n")
}

// editNote reads a note and hands it to Ripple.
func editNote(vault *Vault, rel, mode string) error {
	body, err := ReadNote(vault, rel)
	if err != nil {
		return err
	}
	return runEditor(vault, rel, body, mode)
}

// runEditor runs the editor with the terminal. It is a variable so a test can
// watch that the edit verb reaches Ripple without a terminal.
var runEditor = func(vault *Vault, rel string, body []byte, mode string) error {
	_, err := tea.NewProgram(newEditorModel(vault, rel, body, mode), tea.WithAltScreen()).Run()
	return err
}

// themeRenderer is the theme the editor draws in. A plugin program cannot ask
// the dashboard which theme it resolved, so it uses the default theme and its
// README says so; a mismatch is cosmetic.
func themeRenderer() tideui.Renderer {
	return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})
}

// systemClipboard shells out to the OS clipboard so Ripple's copy/cut/paste
// round-trip with the real one on X11, Wayland and macOS.
type systemClipboard struct{}

func (systemClipboard) Read() (string, error) {
	for _, command := range [][]string{
		{"wl-paste", "--no-newline"},
		{"xclip", "-selection", "clipboard", "-o"},
		{"xsel", "-b", "-o"},
		{"pbpaste"},
	} {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		if out, err := exec.Command(command[0], command[1:]...).Output(); err == nil {
			return string(out), nil
		}
	}
	return "", nil
}

func (systemClipboard) Write(text string) error {
	for _, command := range [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "-b", "-i"},
		{"pbcopy"},
	} {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return nil
}
