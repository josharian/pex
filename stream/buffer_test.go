package stream

import "testing"

func TestBufferBasic(t *testing.T) {
	in := []byte("hello\nworld\n")
	buf := new(Buffer)
	buf.Append(in)
	if buf.Len() != len(in) {
		t.Errorf("len: got %d, want %d", buf.Len(), len(in))
	}
	if buf.NLines() != 2 {
		t.Errorf("nlines: got %d, want %d", buf.NLines(), 2)
	}
	if buf.Line(0) != "hello" {
		t.Errorf("line 0: got %q, want %q", buf.Line(0), "hello")
	}
	if buf.Line(1) != "world" {
		t.Errorf("line 1: got %q, want %q", buf.Line(1), "world")
	}
}

func TestBufferLineFeeds(t *testing.T) {
	in := []byte("hello\r\nworld\nline feeds\r\n\r\n")
	buf := new(Buffer)
	buf.Append(in)
	if buf.Len() != len(in) {
		t.Errorf("len: got %d, want %d", buf.Len(), len(in))
	}
	if buf.NLines() != 4 {
		t.Errorf("nlines: got %d, want %d", buf.NLines(), 4)
	}
	if buf.Line(0) != "hello" {
		t.Errorf("line 0: got %q, want %q", buf.Line(0), "hello")
	}
	if buf.Line(1) != "world" {
		t.Errorf("line 1: got %q, want %q", buf.Line(1), "world")
	}
	if buf.Line(2) != "line feeds" {
		t.Errorf("line 2: got %q, want %q", buf.Line(2), "line feeds")
	}
	if buf.Line(3) != "" {
		t.Errorf("line 3: got %q, want %q", buf.Line(3), "")
	}
}

func TestBufferCharAtATime(t *testing.T) {
	in := []byte("hello\nworld\n")
	buf := new(Buffer)
	for _, b := range in {
		buf.Append([]byte{b})
	}
	if buf.Len() != len(in) {
		t.Errorf("len: got %d, want %d", buf.Len(), len(in))
	}
	if buf.NLines() != 2 {
		t.Errorf("nlines: got %d, want %d", buf.NLines(), 2)
	}
	if buf.Line(0) != "hello" {
		t.Errorf("line 0: got %q, want %q", buf.Line(0), "hello")
	}
	if buf.Line(1) != "world" {
		t.Errorf("line 1: got %q, want %q", buf.Line(1), "world")
	}
}

func TestBufferLineFeedsCharAtATime(t *testing.T) {
	in := []byte("hello\r\nworld\nline feeds\r\n\r\n")
	buf := new(Buffer)
	for _, b := range in {
		buf.Append([]byte{b})
	}
	if buf.Len() != len(in) {
		t.Errorf("len: got %d, want %d", buf.Len(), len(in))
	}
	if buf.NLines() != 4 {
		t.Errorf("nlines: got %d, want %d", buf.NLines(), 4)
	}
	if buf.Line(0) != "hello" {
		t.Errorf("line 0: got %q, want %q", buf.Line(0), "hello")
	}
	if buf.Line(1) != "world" {
		t.Errorf("line 1: got %q, want %q", buf.Line(1), "world")
	}
	if buf.Line(2) != "line feeds" {
		t.Errorf("line 2: got %q, want %q", buf.Line(2), "line feeds")
	}
	if buf.Line(3) != "" {
		t.Errorf("line 3: got %q, want %q", buf.Line(3), "")
	}
}

func TestBufferHellos(t *testing.T) {
	in := "V\nhello 1\nhello 2\nhello 3\nhello 4\nhello 5\nhello 6\nhello 7\nhello 8\nhello 9\nhello 10\nhello 11\nhello 12\nhello 13\nhello 14\nhello 15\nhello 16\nhello"
	buf := new(Buffer)
	buf.Append([]byte(in))
	if buf.Len() != len(in) {
		t.Errorf("len: got %d, want %d", buf.Len(), len(in))
	}
	if buf.NLines() != 18 {
		t.Errorf("nlines: got %d, want %d", buf.NLines(), 18)
	}
	if buf.Line(0) != "V" {
		t.Errorf("line 0: got %q, want %q", buf.Line(0), "V")
	}
	if buf.Line(16) != "hello 16" {
		t.Errorf("line 16: got %q, want %q", buf.Line(16), "hello 16")
	}
	if buf.Line(17) != "hello" {
		t.Errorf("line 17: got %q, want %q", buf.Line(17), "hello")
	}
}
