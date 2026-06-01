// Classification taxonomy/lookup RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"patentmine/internal/proto"
)

func (s *Server) classificationGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationGetParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.ClassificationDefinition(ctx, p.System, p.Code)
}

func (s *Server) classificationList(ctx context.Context, _ json.RawMessage) (any, error) {
	classifications, err := s.engine.ListClassificationDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	return proto.ClassificationListResult{Classifications: classifications}, nil
}

func (s *Server) classificationSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.SaveClassificationDefinition(ctx, p.Classification); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) classificationDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteClassificationDefinition(ctx, p.System, p.Code); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) classificationLookup(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationLookupParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.LookupClassification(ctx, p.Code)
}

func (s *Server) classificationListByCodes(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationListByCodesParams](raw)
	if err != nil {
		return nil, err
	}
	classifications, err := s.engine.ListClassificationDefinitionsByCodes(ctx, p.Codes)
	if err != nil {
		return nil, err
	}
	return proto.ClassificationListResult{Classifications: classifications}, nil
}

func (s *Server) patentClassificationList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentClassificationListParams](raw)
	if err != nil {
		return nil, err
	}
	classifications, err := s.engine.ListPatentClassifications(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	return proto.ClassificationListResult{Classifications: classifications}, nil
}
