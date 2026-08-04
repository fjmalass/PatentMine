package rpc

import (
	"context"
	"encoding/json"

	"patentmine/internal/proto"
)

func (s *Server) docsList(ctx context.Context, _ json.RawMessage) (any, error) {
	return s.engine.ListDocs(ctx)
}

func (s *Server) docsGet(ctx context.Context, raw json.RawMessage) (any, error) {
	params, err := decodeParams[proto.DocsGetParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.GetDoc(ctx, params)
}
