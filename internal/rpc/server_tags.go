// Tag taxonomy and patent-tag RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"patentmine/internal/proto"
)

func (s *Server) tagPatent(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.TagPatent(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) untagPatent(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.UntagPatent(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) tagCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagCreateParams](raw)
	if err != nil {
		return nil, err
	}
	tag, err := s.engine.CreateTaxonomyTag(ctx, p.Project, p.Name)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Server) tagList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagListParams](raw)
	if err != nil {
		return nil, err
	}
	tags, err := s.engine.ListTaxonomyTags(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.TagListResult{Tags: tags}, nil
}

func (s *Server) tagDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteTaxonomyTag(ctx, p.Project, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) tagPatentStrict(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.TagPatentStrict(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) untagPatentStrict(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.UntagPatentStrict(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentTagList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentTagListParams](raw)
	if err != nil {
		return nil, err
	}
	tags, err := s.engine.PatentTags(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	return proto.PatentTagListResult{Tags: tags}, nil
}
