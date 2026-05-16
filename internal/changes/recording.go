package changes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"patentmine/internal/storage"
)

// recordEnvelope is one serialized change in a recording file.
type recordEnvelope struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// changeDecoders maps a change kind to a constructor that decodes its data.
// Every concrete change registers here so a recording can be replayed.
var changeDecoders = map[string]func(json.RawMessage) (Change, error){
	"citationReview": func(d json.RawMessage) (Change, error) { c := &citationReview{}; return c, json.Unmarshal(d, c) },
	"patentReview":   func(d json.RawMessage) (Change, error) { c := &patentReview{}; return c, json.Unmarshal(d, c) },
	"addNote":        func(d json.RawMessage) (Change, error) { c := &addNote{}; return c, json.Unmarshal(d, c) },
	"applyTags":      func(d json.RawMessage) (Change, error) { c := &applyTags{}; return c, json.Unmarshal(d, c) },
	"setPatentDate":  func(d json.RawMessage) (Change, error) { c := &setPatentDate{}; return c, json.Unmarshal(d, c) },
	"editIDS":        func(d json.RawMessage) (Change, error) { c := &editIDS{}; return c, json.Unmarshal(d, c) },
	"deletePatents":  func(d json.RawMessage) (Change, error) { c := &deletePatents{}; return c, json.Unmarshal(d, c) },
}

// SaveRecording writes the session's ordered change log to path as JSON.
func (h *History) SaveRecording(path string) error {
	envs := make([]recordEnvelope, 0, len(h.log))
	for _, c := range h.log {
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		envs = append(envs, recordEnvelope{Kind: c.kind(), Data: data})
	}
	out, err := json.MarshalIndent(envs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// Replay decodes a recording file and runs every change against repo in order.
// It returns the number of changes successfully applied; on the first failure
// it stops and returns that count with the error.
func Replay(ctx context.Context, repo storage.Repository, path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var envs []recordEnvelope
	if err := json.Unmarshal(raw, &envs); err != nil {
		return 0, fmt.Errorf("malformed recording: %w", err)
	}
	applied := 0
	for _, e := range envs {
		decode, ok := changeDecoders[e.Kind]
		if !ok {
			return applied, fmt.Errorf("unknown change kind %q", e.Kind)
		}
		c, err := decode(e.Data)
		if err != nil {
			return applied, fmt.Errorf("decode %s: %w", e.Kind, err)
		}
		if err := c.run(ctx, repo); err != nil {
			return applied, fmt.Errorf("apply %s: %w", e.Kind, err)
		}
		applied++
	}
	return applied, nil
}
