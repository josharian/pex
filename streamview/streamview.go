package streamview

import (
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/josharian/pex/stream"
)

// New returns a new model with default key mappings.
// The zero value is not valid.
func New(shared *stream.Shared) (m Model) {
	// todo: re-enable, needs a hit-test so we only scroll the correct column
	m.MouseWheelEnabled = false
	m.MouseWheelDelta = 3
	m.buffer = shared.Buffer()
	m.reader = shared.Reader()
	m.id = streamviewID.Add(1)
	return m
}

var streamviewID atomic.Uint64

// Model is the Bubble Tea model for this viewport element.
type Model struct {
	Name string // for debugging
	id   uint64

	Width  int
	Height int

	// Whether or not to respond to the mouse. The mouse must be enabled in
	// Bubble Tea for this to work. For details, see the Bubble Tea docs.
	MouseWheelEnabled bool

	// The number of lines the mouse wheel will scroll. By default, this is 3.
	MouseWheelDelta int

	// CurrentLine is the current line number at the top of the viewport, 0-based.
	// It may be larger than the number of lines.
	CurrentLine int

	// TODO: subline count for line wrapping

	// Style applies a lipgloss style to the viewport. Realistically, it's most
	// useful for setting borders, margins and padding.
	Style      lipgloss.Style
	FocusStyle lipgloss.Style

	focused bool
	lastErr error
	// lastSleep time.Time
	// TODO:
	// linewrap bool

	buffer *stream.Buffer
	reader *stream.Reader
}

type readMsg struct {
	id  uint64 // of the streamview this read is for
	err error
}

func (x readMsg) String() string {
	return fmt.Sprintf("readMsg{id: %d, err: %v}", x.id, x.err)
}

func readCmd(m *Model) tea.Cmd {
	var pc [1]uintptr
	n := runtime.Callers(2, pc[:])
	frames := runtime.CallersFrames(pc[:n])
	frame, _ := frames.Next()
	slog.Debug("streamview.readCmd",
		"id", m.id,
		"triggered by", fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function),
	)
	return func() tea.Msg {
		slog.Debug("streamview.readCmd", "id", m.id, "lastErr", m.lastErr)
		if m.lastErr == io.EOF {
			// The error might just be because the program is still working.
			// Avoid busy-looping, but keep trying.
			time.Sleep(50 * time.Millisecond)
		}
		buf := make([]byte, 4096)
		_, err := m.reader.Read(buf)
		return readMsg{id: m.id, err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return readCmd(&m)
}

// AtTop returns whether or not the viewport is at the very top position.
func (m Model) AtTop() bool {
	return m.CurrentLine <= 0
}

// AtBottom returns whether or not the viewport is at or past the very bottom
// position.
func (m Model) AtBottom() bool {
	_, bottom := m.visibleLineRange()
	return m.CurrentLine >= bottom
}

// PastBottom returns whether or not the viewport is scrolled beyond the last
// line. This can happen when adjusting the viewport height.
func (m Model) PastBottom() bool {
	return m.CurrentLine > m.maxLine()
}

// maxLine returns the maximum possible value of the y-offset based on the
// viewport's content and set height.
func (m Model) maxLine() int {
	// allow scrolling past the end of the file
	// require one line to be visible at top
	return m.buffer.NLines() + m.Height - 1
}

func (m Model) visibleLineRange() (top, bottom int) {
	top = max(0, m.CurrentLine)
	bottom = clamp(m.maxLine(), top, m.buffer.NLines()-1)
	return top, bottom
}

// visibleLines returns the lines that should currently be visible in the
// viewport.
func (m Model) visibleLines() (lines []string) {
	if !m.hasLines() {
		return nil
	}
	top, bottom := m.visibleLineRange()
	for i := top; i <= bottom; i++ {
		lines = append(lines, m.buffer.Line(i))
	}
	return lines
}

func (m Model) VisibleLineCount() int {
	if !m.hasLines() {
		return 0
	}
	top, bottom := m.visibleLineRange()
	return bottom - top + 1
}

// SetCurrentLine sets the current line.
func (m *Model) SetCurrentLine(n int) {
	m.CurrentLine = clamp(n, 0, m.maxLine())
}

// ViewDown moves the view down by the number of lines in the viewport.
// Basically, "page down".
func (m *Model) ViewDown() tea.Cmd {
	return m.LineDown(m.Height)
}

// ViewUp moves the view up by one height of the viewport. Basically, "page up".
func (m *Model) ViewUp() {
	m.LineUp(m.Height)
}

// HalfViewDown moves the view down by half the height of the viewport.
func (m *Model) HalfViewDown() tea.Cmd {
	return m.LineDown(m.Height / 2)
}

// HalfViewUp moves the view up by half the height of the viewport.
func (m *Model) HalfViewUp() {
	m.LineUp(m.Height / 2)
}

// LineDown moves the view down by the given number of lines.
func (m *Model) LineDown(n int) (cmd tea.Cmd) {
	next := min(m.CurrentLine+n, m.buffer.NLines()-1)
	m.SetCurrentLine(next)
	if m.shouldReadMore() {
		cmd = readCmd(m)
	}
	return cmd
}

// LineUp moves the view down by the given number of lines. Returns the new
// lines to show.
func (m *Model) LineUp(n int) {
	next := max(0, m.CurrentLine-n)
	m.SetCurrentLine(next)
}

// TotalLineCount returns the total number of lines (both hidden and visible) within the viewport.
func (m Model) TotalLineCount() int {
	return m.buffer.NLines()
}

func (m Model) hasLines() bool {
	return m.buffer.NLines() > 0
}

// GotoTop sets the viewport to the top position.
func (m *Model) GotoTop() {
	m.SetCurrentLine(0)
}

// GotoBottom sets the viewport to the bottom position.
func (m *Model) GotoBottom() {
	m.SetCurrentLine(m.maxLine())
	// return readCmd(m)
}

// Update handles standard message-based viewport updates.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case readMsg:
		if msg.id != m.id {
			// for a different streamview
			break
		}
		m.lastErr = msg.err
		if m.shouldReadMore() {
			cmd = readCmd(&m)
		}
		// m.SetCurrentLine(clamp(m.CurrentLine, 0, m.maxLine()))

	case tea.MouseMsg:
		if !m.MouseWheelEnabled {
			break
		}
		switch msg.Type {
		case tea.MouseWheelUp:
			m.LineUp(m.MouseWheelDelta)
		case tea.MouseWheelDown:
			cmd = m.LineDown(m.MouseWheelDelta)
		}
	}

	return m, cmd
}

func (m Model) shouldReadMore() bool {
	slog.Debug("streamview.shouldReadMore",
		"lastErr", m.lastErr,
		"Height", m.Height,
		"VisibleLineCount", m.VisibleLineCount(),
		"CurrentLine", m.CurrentLine,
		"buffered lines", m.buffer.NLines(),
		"maxLine", m.maxLine(),
		// "decision", x,
	)
	if m.VisibleLineCount() >= m.Height {
		// Screen is full, and we're not at the bottom.
		// We definitely don't need more data.
		return false
	}
	if m.lastErr != nil /* && m.lastErr != io.EOF */ {
		// There was an error reading the stream.
		// Stop trying.
		return false
	}
	return true
}

func (m *Model) Focus() {
	m.focused = true
}

func (m *Model) Blur() {
	m.focused = false
}

// View renders the viewport into a string.
func (m *Model) View() string {
	w, h := m.Width, m.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	style := m.Style
	if m.focused {
		style = m.FocusStyle
	}

	if sw := style.GetWidth(); sw != 0 {
		w = min(w, sw)
	}
	if sh := style.GetHeight(); sh != 0 {
		h = min(h, sh)
	}

	contentWidth := w - style.GetHorizontalFrameSize()
	contentHeight := h - style.GetVerticalFrameSize()

	contents := lipgloss.NewStyle().
		Width(contentWidth).      // pad to width.
		Height(contentHeight).    // pad to height.
		MaxHeight(contentHeight). // truncate height if taller.
		MaxWidth(contentWidth).   // truncate width.
		Render(strings.Join(m.visibleLines(), "\n"))
	return style.Copy().
		UnsetWidth().UnsetHeight(). // Style size already applied in contents.
		Render(contents)
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}
