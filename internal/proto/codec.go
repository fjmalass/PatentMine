package proto

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

// maxMessageBytes caps a single framed message. Patent listings can be large,
// so the ceiling is generous; it exists only to bound memory on bad input.
const maxMessageBytes = 16 << 20 // 16 MiB

// Conn frames messages as line-delimited JSON over a byte stream. Writes are
// mutex-guarded so replies and pushed events never interleave on the wire.
type Conn struct {
	closer io.Closer
	sc     *bufio.Scanner
	w      io.Writer
	wmu    sync.Mutex
}

// NewConn wraps a stream (typically a unix-socket net.Conn) as a Conn.
func NewConn(rwc io.ReadWriteCloser) *Conn {
	sc := bufio.NewScanner(rwc)
	sc.Buffer(make([]byte, 0, 64<<10), maxMessageBytes)
	return &Conn{closer: rwc, sc: sc, w: rwc}
}

// WriteMessage marshals v and writes it as one newline-terminated frame.
func (c *Conn) WriteMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.w.Write(data); err != nil {
		return err
	}
	_, err = c.w.Write([]byte{'\n'})
	return err
}

// ReadMessage reads one frame. The returned bytes are owned by the Conn and
// valid only until the next ReadMessage; unmarshal before calling again.
func (c *Conn) ReadMessage() ([]byte, error) {
	if c.sc.Scan() {
		return c.sc.Bytes(), nil
	}
	if err := c.sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// Close closes the underlying stream.
func (c *Conn) Close() error {
	return c.closer.Close()
}
