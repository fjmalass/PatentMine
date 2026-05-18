package proto

import (
	"encoding/json"
	"net"
	"testing"
)

func TestConnRoundTrip(t *testing.T) {
	a, b := net.Pipe()
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()

	ca := NewConn(a)
	cb := NewConn(b)

	want := Request{JSONRPC: Version, ID: 7, Method: MethodPing}
	go func() {
		if err := ca.WriteMessage(want); err != nil {
			t.Errorf("WriteMessage: %v", err)
		}
	}()

	raw, err := cb.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var got Request
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != want.ID || got.Method != want.Method {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestConnFramesMultipleMessages(t *testing.T) {
	a, b := net.Pipe()
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()

	ca := NewConn(a)
	cb := NewConn(b)

	go func() {
		for id := uint64(1); id <= 3; id++ {
			if err := ca.WriteMessage(Request{JSONRPC: Version, ID: id, Method: MethodPing}); err != nil {
				t.Errorf("WriteMessage: %v", err)
				return
			}
		}
	}()

	for id := uint64(1); id <= 3; id++ {
		raw, err := cb.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage %d: %v", id, err)
		}
		var got Request
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("Unmarshal %d: %v", id, err)
		}
		if got.ID != id {
			t.Fatalf("frame %d decoded id %d", id, got.ID)
		}
	}
}
