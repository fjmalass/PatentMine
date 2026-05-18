package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"patentmine/internal/proto"
)

// ErrClosed is returned by calls on a Client whose connection has dropped.
var ErrClosed = errors.New("rpc: client connection closed")

// eventBuffer bounds the client-side event queue; an overwhelmed consumer
// drops the newest events rather than blocking the read loop.
const eventBuffer = 64

// Client is a thin frontend's handle to the daemon. It multiplexes request
// replies and pushed events over one connection.
type Client struct {
	conn   *proto.Conn
	events chan proto.Event
	closed chan struct{}

	mu       sync.Mutex
	nextID   uint64
	pending  map[uint64]chan proto.Reply
	closeErr error
	stopped  bool
}

// Dial connects to the daemon's unix socket and starts the read loop.
func Dial(socketPath string) (*Client, error) {
	nc, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("rpc: dial %s: %w", socketPath, err)
	}
	c := &Client{
		conn:    proto.NewConn(nc),
		events:  make(chan proto.Event, eventBuffer),
		closed:  make(chan struct{}),
		pending: make(map[uint64]chan proto.Reply),
	}
	go c.readLoop()
	return c, nil
}

// Events returns the channel of daemon-pushed events. It is closed when the
// connection drops.
func (c *Client) Events() <-chan proto.Event {
	return c.events
}

// Call sends a request and waits for its reply, decoding result into the
// caller's value. It returns early if ctx is cancelled or the connection drops.
func (c *Client) Call(ctx context.Context, method proto.Method, params, result any) error {
	var raw json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("rpc: encode params: %w", err)
		}
		raw = encoded
	}

	c.mu.Lock()
	if c.stopped {
		err := c.closeErr
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	replyCh := make(chan proto.Reply, 1)
	c.pending[id] = replyCh
	c.mu.Unlock()

	cleanup := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}

	if err := c.conn.WriteMessage(proto.Request{
		JSONRPC: proto.Version,
		ID:      id,
		Method:  method,
		Params:  raw,
	}); err != nil {
		cleanup()
		return fmt.Errorf("rpc: write request: %w", err)
	}

	select {
	case <-ctx.Done():
		cleanup()
		return ctx.Err()
	case <-c.closed:
		return c.closeErr
	case reply := <-replyCh:
		if reply.Error != nil {
			return reply.Error
		}
		if result != nil && len(reply.Result) > 0 {
			if err := json.Unmarshal(reply.Result, result); err != nil {
				return fmt.Errorf("rpc: decode result: %w", err)
			}
		}
		return nil
	}
}

// Close shuts the client down.
func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	return nil
}

// readLoop demultiplexes incoming frames into replies and events.
func (c *Client) readLoop() {
	defer close(c.events)
	for {
		raw, err := c.conn.ReadMessage()
		if err != nil {
			c.shutdown(fmt.Errorf("%w: %v", ErrClosed, err))
			return
		}
		// A frame with a method name and no id is an event; otherwise a reply.
		var probe struct {
			ID     *uint64 `json:"id"`
			Method *string `json:"method"`
		}
		_ = json.Unmarshal(raw, &probe)
		if probe.ID == nil && probe.Method != nil {
			var ev proto.Event
			if json.Unmarshal(raw, &ev) == nil {
				select {
				case c.events <- ev:
				default: // consumer behind; drop newest
				}
			}
			continue
		}
		var reply proto.Reply
		if json.Unmarshal(raw, &reply) != nil {
			continue
		}
		c.deliver(reply)
	}
}

func (c *Client) deliver(reply proto.Reply) {
	c.mu.Lock()
	ch, ok := c.pending[reply.ID]
	if ok {
		delete(c.pending, reply.ID)
	}
	c.mu.Unlock()
	if ok {
		ch <- reply
	}
}

// shutdown marks the client closed exactly once and wakes blocked callers.
func (c *Client) shutdown(err error) {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.closeErr = err
	c.mu.Unlock()

	close(c.closed)
	_ = c.conn.Close()
}
