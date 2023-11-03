package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/josharian/pex/shell"
)

const (
	// TODO: add keyboard bindings to make maxPages adjustable
	maxPagers        = 3
	bottomAreaHeight = 2
)

type keymap = struct {
	next, prev  key.Binding
	top, bottom key.Binding
	quit        key.Binding
	pageDown    key.Binding
	pageUp      key.Binding
	down        key.Binding
	up          key.Binding
}

var defaultKeymap = keymap{
	next: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next"),
	),
	prev: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev"),
	),
	top: key.NewBinding(
		key.WithKeys("ctrl+["),
		key.WithHelp("ctrl+[", "top"),
	),
	bottom: key.NewBinding(
		key.WithKeys("ctrl+]"),
		key.WithHelp("ctrl+]", "bottom"),
	),
	quit: key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "quit"),
	),
	pageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	pageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	up: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "up"),
	),
	down: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "down"),
	),
}

type model struct {
	width           int
	height          int
	keymap          keymap
	help            help.Model
	pagers          []*pager
	bottomTextInput textinput.Model
	errText         textinput.Model
	commands        []shell.Command
	pipes           []int
	minPager        int
	maxPager        int
	focusedPager    int
	err             error
}

func initialBottom() textinput.Model {
	ti := textinput.New()
	ti.Focus()
	ti.Prompt = "| "
	return ti
}

func initialErrText() textinput.Model {
	ti := textinput.New()
	ti.Blur()
	ti.Prompt = ""
	ti.TextStyle = lipgloss.NewStyle().Italic(true)
	return ti
}

func newModel(args []string) (*model, error) {
	var files []io.Reader
	for _, path := range args {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		// we never close these files; they'll go away when the process ends
		files = append(files, f)
	}

	var in io.Reader = os.Stdin
	if len(files) > 0 {
		in = io.MultiReader(files...)
	}

	p0 := newReadPager(in)
	m := &model{
		pagers: []*pager{
			p0,
			newEmptyPager(),
		},
		minPager:        0,
		maxPager:        0,
		bottomTextInput: initialBottom(),
		errText:         initialErrText(),
		help:            help.New(),
		keymap:          defaultKeymap,
	}
	m.pagers[m.focusedPager].Focus()
	m.bottomTextInput.Focus()
	return m, nil
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range m.pagers {
		cmds = append(cmds, p.Init())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	slog.Debug("model.Update", "msg", msg)

	prevPos := m.bottomTextInput.Position()
	prevRawShell := m.bottomTextInput.Value()

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case cursor.BlinkMsg:
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.quit):
			return m, tea.Quit
		case key.Matches(msg, m.keymap.pageDown):
			p := m.pagers[m.focusedPager]
			cmd := p.view.ViewDown()
			cmds = append(cmds, cmd)
		case key.Matches(msg, m.keymap.pageUp):
			p := m.pagers[m.focusedPager]
			p.view.ViewUp()
		case key.Matches(msg, m.keymap.down):
			p := m.pagers[m.focusedPager]
			cmd := p.view.LineDown(1)
			cmds = append(cmds, cmd)
		case key.Matches(msg, m.keymap.up):
			p := m.pagers[m.focusedPager]
			p.view.LineUp(1)
		case key.Matches(msg, m.keymap.next):
			pos := m.bottomTextInput.Position()
			cur := sort.SearchInts(m.pipes, pos)
			switch {
			case cur == len(m.pipes):
				pos = len(m.bottomTextInput.Value())
			case m.pipes[cur] != pos:
				pos = m.pipes[cur]
			case cur < len(m.pipes)-1:
				pos = m.pipes[cur+1]
			default:
				pos = len(m.bottomTextInput.Value())
			}
			m.bottomTextInput.SetCursor(pos)
		case key.Matches(msg, m.keymap.prev):
			pos := m.bottomTextInput.Position()
			cur := sort.SearchInts(m.pipes, pos)
			switch {
			case cur == 0:
				pos = 0
			case m.pipes[cur-1] != pos:
				pos = m.pipes[cur-1]
			default:
				pos = 0
			}
			m.bottomTextInput.SetCursor(pos)
		}
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	}

	newBottom, cmd := m.bottomTextInput.Update(msg)
	m.bottomTextInput = newBottom
	cmds = append(cmds, cmd)

	posChanged := prevPos != m.bottomTextInput.Position()
	rawShellChanged := prevRawShell != m.bottomTextInput.Value()
	if posChanged || rawShellChanged {
		cmds = append(cmds, m.updatePagers()...)
	}

	if m.width > 0 {
		m.sizeInputs()
	}

	for _, p := range m.pagers {
		cmd := p.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updatePagers() []tea.Cmd {
	var cmds []tea.Cmd
	rawShell := m.bottomTextInput.Value()
	shellCommands, pipeOffsets, err := shell.Parse(rawShell)
	pipeOffsets = append([]int{0}, pipeOffsets...) // add implicit pipe at position 0
	if err == nil && len(shellCommands) > 0 {
		last := shellCommands[len(shellCommands)-1]
		if last.Name() != "" && len(last.Args()) == 0 && !strings.HasSuffix(rawShell, " ") {
			err = fmt.Errorf("to execute %q without args, press space", last.Name())
		}
	}
	// on err, keep last good shell parse, display error in help area
	m.SetErr(nil)
	if err != nil {
		m.SetErr(err)
	} else {
		m.commands = shellCommands
		m.pipes = pipeOffsets
	}
	nPagers := len(m.commands) + 1
	// this should be "if", not "for", but "for" makes it more resilient
	// when I invariably mess up other things
	for nPagers > len(m.pagers) {
		p := newEmptyPager()
		cmds = append(cmds, p.Init())
		m.pagers = append(m.pagers, p)
	}

	// rebuild pagers as necessary
	for _, p := range m.pagers[nPagers:] {
		p.close()
	}
	m.pagers = m.pagers[:nPagers]
	rebuildIdx := -1
	for i, p := range m.pagers {
		if i == 0 {
			continue
		}
		if !p.command.Equal(m.commands[i-1]) {
			rebuildIdx = i
			break
		}
	}
	if rebuildIdx > 0 {
		for i := rebuildIdx; i < len(m.pagers); i++ {
			m.pagers[i].close()
			p := newCommandPager(m.pagers[i-1].shared, m.commands[i-1])
			cmds = append(cmds, p.Init())
			m.pagers[i] = p
		}
	}

	pos := m.bottomTextInput.Position()
	m.focusedPager = sort.SearchInts(m.pipes, pos)
	slog.Warn("pipeOffset", "search", m.pipes, "pos", pos, "chose", m.focusedPager)
	for i, p := range m.pagers {
		if i == m.focusedPager {
			p.Focus()
		} else {
			p.Blur()
		}
	}
	// show up to maxPagers pagers,
	// ensuring that the focused pager is visible
	slog.Warn("before min/max pager", "min", m.minPager, "max", m.maxPager, "focus", m.focusedPager)
	m.maxPager = min(m.maxPager, len(m.pagers)-1)
	if m.focusedPager > m.maxPager {
		m.maxPager = m.focusedPager
		m.minPager = max(0, m.maxPager-maxPagers+1)
	}
	if m.focusedPager < m.minPager {
		m.minPager = m.focusedPager
		m.maxPager = min(m.minPager+maxPagers-1, len(m.pagers))
	}
	slog.Warn("after min/max pager", "min", m.minPager, "max", m.maxPager, "focus", m.focusedPager)
	return cmds
}

func (m *model) sizeInputs() {
	nPagers := m.numVisiblePagers()
	per := m.width / nPagers
	extra := m.width % nPagers
	for i := range m.pagers {
		// TODO: latter columns bigger, earlier smaller?
		w := per
		if i > nPagers-extra-1 {
			w++
		}
		h := m.height - bottomAreaHeight
		v := m.pagers[i]
		v.view.Width = w
		v.view.Height = h
		m.pagers[i] = v
	}

	m.bottomTextInput.Width = m.width - len(m.bottomTextInput.Prompt)
	m.errText.Width = m.width
}

func (m *model) SetErr(err error) {
	m.err = err
	if err != nil {
		m.errText.SetValue(err.Error())
	}
}

func (m *model) visiblePagers() []*pager {
	return m.pagers[m.minPager : m.maxPager+1]
}

func (m *model) numVisiblePagers() int {
	return len(m.visiblePagers())
}

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	help := m.help.ShortHelpView([]key.Binding{
		m.keymap.next,
		m.keymap.prev,
		m.keymap.quit,
	})

	var views []string
	for _, p := range m.visiblePagers() {
		views = append(views, p.view.View())
	}

	inputs := lipgloss.JoinHorizontal(lipgloss.Top, views...)
	lastLine := help
	if m.err != nil {
		lastLine = m.errText.View()
	}
	all := lipgloss.JoinVertical(lipgloss.Left, inputs, m.bottomTextInput.View(), lastLine)
	return all
}

var flagDebugLog = flag.String("log", "", "log to file `log`")

func main() {
	// override ErrHelp handling to hide -log flag from regular users, it is for debugging
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `
usage:
  pex [files...]
or
  command | pex
`[1:])
		os.Exit(0)
	}
	err := flag.CommandLine.Parse(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		flag.Usage()
	}

	var logWriter io.Writer = io.Discard
	if *flagDebugLog != "" {
		// todo: open append
		f, err := os.Create(*flagDebugLog)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		logWriter = f
	}
	// TODO: -v flags for verbosity
	// todo: audit and adjust all logging
	ho := &slog.HandlerOptions{Level: slog.LevelDebug}
	lh := slog.NewTextHandler(logWriter, ho)
	logger := slog.New(lh)
	slog.SetDefault(logger)

	args := flag.Args()
	m, err := newModel(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to launch: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // turn on mouse support so we can track the mouse wheel
	)

	final, err := p.Run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	finalModel, ok := final.(model)
	if !ok {
		return
	}
	finalStr := finalModel.bottomTextInput.Value()
	if finalStr != "" {
		fmt.Println("|", finalStr)
	}
}
