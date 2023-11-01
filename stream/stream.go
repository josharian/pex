package stream

import (
	"io"
	"sync"
)

const pageSize = 4096

// Shared is a shared Reader.
// It can be consumed concurrently by multiple Readers.
// It caches data in a buffer so that new Readers can start any time.
type Shared struct {
	buf *Buffer
	mu  sync.Mutex // protects reads from r
	r   io.Reader
}

func NewShared(r io.Reader) *Shared {
	return &Shared{r: r, buf: new(Buffer)}
}

func (s *Shared) Reader() *Reader {
	return &Reader{s: s}
}

func (s *Shared) Buffer() *Buffer {
	return s.buf
}

type Reader struct {
	s   *Shared
	off int
}

func (r *Reader) Read(p []byte) (int, error) {
	// Use cached data if it exists.
	if r.off < r.s.buf.Len() {
		n, err := r.s.buf.ReadAt(p, r.off)
		r.off += n
		return n, err
	}
	// Fetch more data. Lock to single-flight.
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	// Check again in case we lost a race,
	// and some data is now available.
	if r.off < r.s.buf.Len() {
		n, err := r.s.buf.ReadAt(p, r.off)
		r.off += n
		return n, err
	}
	// Read from underlying reader.
	n, err := r.s.r.Read(p)
	if n > 0 {
		r.s.buf.Append(p[:n])
	}
	return n, err
}
