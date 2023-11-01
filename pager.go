package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/josharian/pex/shell"
	"github.com/josharian/pex/stream"
	"github.com/josharian/pex/streamview"
)

var (
	focusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#999"))
	blurredBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#333"))
)

type pager struct {
	isErr   bool
	command shell.Command
	cmd     *exec.Cmd
	shared  *stream.Shared
	view    *streamview.Model
	cancel  func()
}

func newCommandPager(r *stream.Shared, command shell.Command) *pager {
	if command.Empty() {
		return newEmptyPager()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, command.Name(), command.Args()...)
	cmd.Stdin = r.Reader()
	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return newErrorPager(err, command)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return newErrorPager(err, command)
	}
	p := newPager(stdOut, command.Raw)
	p.command = command
	p.cmd = cmd
	p.cancel = cancel
	return p
}

func newEmptyPager() *pager {
	return newPager(strings.NewReader(""), "empty")
}

func newStringPager(s string) *pager {
	return newPager(strings.NewReader(s), "string: "+s)
}

func newErrorPager(err error, command shell.Command) *pager {
	s := err.Error()
	p := newPager(strings.NewReader(s), "error: "+s)
	p.isErr = true
	p.command = command
	return p
}

func newReadPager(r io.Reader) *pager {
	return newPager(r, fmt.Sprintf("read: %T", r))
}

func newPager(r io.Reader, name string) *pager {
	shared := stream.NewShared(r)
	t := streamview.New(shared)
	t.Name = name
	t.Style = blurredBorderStyle.Copy()
	t.FocusStyle = focusedBorderStyle.Copy()
	return &pager{view: &t, shared: shared}
}

func (p *pager) Update(msg tea.Msg) tea.Cmd {
	v, cmd := p.view.Update(msg)
	p.view = &v
	return cmd
}

func (p *pager) Blur() {
	p.view.Blur()
}

func (p *pager) Focus() {
	p.view.Focus()
}

func (p *pager) Init() tea.Cmd {
	return p.view.Init()
}

func (p *pager) close() {
	if p.cancel != nil {
		p.cancel()
	}
}
