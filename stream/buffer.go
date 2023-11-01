package stream

import (
	"bytes"
	"fmt"
	"sync"
)

type Buffer struct {
	mu    sync.Mutex
	buf   []byte   // TODO: use something rope-like?
	lines [][3]int // line start / \r / \n
}

func (b *Buffer) Append(p []byte) {
	if len(p) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// need to re-process the final line
	if len(b.lines) > 0 {
		b.lines = b.lines[:len(b.lines)-1]
	}

	// add line offsets
	lastLineEnd := b.lastLineEnd()
	b.buf = append(b.buf, p...)

	appendLine := func(start, end int) {
		line := b.buf[start:end]
		lineLen := bytes.LastIndexByte(line, '\r')
		if lineLen < 0 {
			lineLen = len(line)
		}
		b.lines = append(b.lines, [3]int{start, start + lineLen, start + len(line)})
	}

	lineBuf := b.buf[lastLineEnd:]
	j := 0
	for {
		i := bytes.IndexByte(lineBuf, '\n')
		if i < 0 {
			break
		}
		lineStart := lastLineEnd + j
		appendLine(lineStart, lineStart+i)
		j += i + 1
		lineBuf = lineBuf[i+1:]
	}
	if b.buf[len(b.buf)-1] != '\n' {
		appendLine(b.lastLineEnd(), len(b.buf))
	}
}

func (b *Buffer) lastLineEnd() int {
	lastLineEnd := 0
	if len(b.lines) > 0 {
		lastLineEnd = b.lines[len(b.lines)-1][2] + 1
	}
	return lastLineEnd
}

func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

func (b *Buffer) ReadAt(p []byte, off int) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return copy(p, b.buf[off:]), nil
}

func (b *Buffer) NLines() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.lines)
}

func (b *Buffer) Line(n int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n < 0 || n >= len(b.lines) {
		return ""
	}
	start, end := b.lines[n][0], b.lines[n][1]
	return string(b.buf[start:end])
}

func (b *Buffer) Debug() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fmt.Sprintf("%q (%v)", b.buf, b.lines)
}
