package citationlookup

import (
	"context"
	"log/slog"

	"patentmine/internal/domain"
	"patentmine/internal/importer"
)

type USPTOImporter struct {
	APIKey string
	Logger *slog.Logger
}

func (i USPTOImporter) ImportPatent(ctx context.Context, number domain.PatentNumber) (domain.PatentBundle, error) {
	select {
	case <-ctx.Done():
		return domain.PatentBundle{}, ctx.Err()
	default:
	}
	bundle, err := importer.ImportUSPTO(string(number), i.APIKey, i.Logger)
	if err == nil {
		bundle.Patent.ImportSource = "uspto-odp"
	}
	return bundle, err
}
