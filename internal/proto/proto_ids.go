// IDS export/preview/entry and project-update payloads.
package proto

import "patentmine/internal/domain"

// IDSExportParams selects the project to build an Information Disclosure
// Statement for.
type IDSExportParams struct {
	Project domain.ProjectID `json:"project"`
}

// IDSResult carries a generated Information Disclosure Statement.
type IDSResult struct {
	IDS domain.IDS `json:"ids"`
}

// IDSPDFExportParams selects the project to render an IDS PDF bundle for.
type IDSPDFExportParams struct {
	Project         domain.ProjectID `json:"project"`
	CumulativeCount int              `json:"cumulative_count,omitempty"`
	FeeAmount       string           `json:"fee_amount,omitempty"`
	DepositAccount  string           `json:"deposit_account,omitempty"`
	SignerName      string           `json:"signer_name,omitempty"`
	SignerSignature string           `json:"signer_signature,omitempty"`
	SignerRegNumber string           `json:"signer_reg_number,omitempty"`
}

// IDSPDFExportResult reports where the generated bundle was written.
type IDSPDFExportResult struct {
	Dir       string   `json:"dir"`
	Files     []string `json:"files"`
	FeeTier   int      `json:"fee_tier"`
	PageCount int      `json:"page_count"`
}

// ProjectUpdateParams updates a project's mutable fields (name + IDS header
// fields, inventor list, examiner history).
type ProjectUpdateParams struct {
	Project domain.Project `json:"project"`
}

// IDSPDFPreviewResult is the dry-run summary of an IDS export. It tells the
// caller where files would land, how many sheets the renderer would emit, the
// fee tier, any IDS header fields the project is still missing, and which
// previous exports already exist on disk for the same project.
type IDSPDFPreviewResult struct {
	BaseDir         string   `json:"base_dir"`
	USCount         int      `json:"us_count"`
	ForeignCount    int      `json:"foreign_count"`
	Sheets          int      `json:"sheets"`
	FeeTier         int      `json:"fee_tier"`
	CumulativeCount int      `json:"cumulative_count"`
	ExistingDirs    []string `json:"existing_dirs,omitempty"`
	MissingFields   []string `json:"missing_fields,omitempty"`
}

// IDSEntryParams identifies one project/patent IDS entry.
type IDSEntryParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// IDSEntrySaveParams carries the IDS entry to insert or update.
type IDSEntrySaveParams struct {
	Entry domain.IDSEntry `json:"entry"`
}

// IDSEntryResult carries one curated IDS entry.
type IDSEntryResult struct {
	Entry domain.IDSEntry `json:"entry"`
}

// IDSEntryBulkSetStatusParams applies a single IDSEntryStatus to every patent
// in Patents under Project. Missing entries are created with InFull=DefaultInFull.
type IDSEntryBulkSetStatusParams struct {
	Project       domain.ProjectID      `json:"project"`
	Patents       []domain.PatentNumber `json:"patents"`
	Status        domain.IDSEntryStatus `json:"status"`
	DefaultInFull bool                  `json:"default_in_full"`
}

// IDSEntriesResult carries every curated IDS entry touched by a bulk write.
type IDSEntriesResult struct {
	Entries []domain.IDSEntry `json:"entries"`
}
