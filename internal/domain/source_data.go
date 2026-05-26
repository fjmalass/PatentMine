package domain

// AuthorityIdentifier maps source-specific identifiers (USPTO application
// numbers, PCT applications, foreign priorities, Google IDs) onto a Patent row.
type AuthorityIdentifier struct {
	Authority      string       `json:"authority"`
	IdentifierType string       `json:"identifier_type"`
	Identifier     string       `json:"identifier"`
	RawIdentifier  string       `json:"raw_identifier,omitempty"`
	RecordNumber   PatentNumber `json:"record_number"`
	DocumentNumber string       `json:"document_number,omitempty"`
	Country        string       `json:"country,omitempty"`
	Kind           string       `json:"kind,omitempty"`
	Dated          string       `json:"dated,omitempty"`
	Source         string       `json:"source,omitempty"`
	Confidence     int          `json:"confidence,omitempty"`
}

// USPTOApplication is the normalized subset of File Wrapper application metadata.
type USPTOApplication struct {
	ApplicationNumber             string `json:"application_number"`
	RecordNumber                  PatentNumber
	InventionTitle                string `json:"invention_title,omitempty"`
	FilingDate                    string `json:"filing_date,omitempty"`
	EffectiveFilingDate           string `json:"effective_filing_date,omitempty"`
	ApplicationStatusCode         string `json:"application_status_code,omitempty"`
	ApplicationStatusText         string `json:"application_status_text,omitempty"`
	ApplicationStatusDate         string `json:"application_status_date,omitempty"`
	ApplicationTypeCode           string `json:"application_type_code,omitempty"`
	ApplicationTypeLabel          string `json:"application_type_label,omitempty"`
	ApplicationTypeCategory       string `json:"application_type_category,omitempty"`
	FirstInventorToFile           bool   `json:"first_inventor_to_file,omitempty"`
	NationalStage                 bool   `json:"national_stage,omitempty"`
	FirstInventorName             string `json:"first_inventor_name,omitempty"`
	FirstApplicantName            string `json:"first_applicant_name,omitempty"`
	CustomerNumber                string `json:"customer_number,omitempty"`
	GroupArtUnitNumber            string `json:"group_art_unit_number,omitempty"`
	ExaminerName                  string `json:"examiner_name,omitempty"`
	DocketNumber                  string `json:"docket_number,omitempty"`
	ApplicationConfirmationNumber string `json:"application_confirmation_number,omitempty"`
	USPCSymbolText                string `json:"uspc_symbol_text,omitempty"`
	USPCClass                     string `json:"uspc_class,omitempty"`
	USPCSubclass                  string `json:"uspc_subclass,omitempty"`
	SmallEntityStatus             bool   `json:"small_entity_status,omitempty"`
	BusinessEntityStatus          string `json:"business_entity_status,omitempty"`
	PublicationCategoryJSON       string `json:"publication_category_json,omitempty"`
	LastIngestionDateTime         string `json:"last_ingestion_datetime,omitempty"`
	FetchedAt                     string `json:"fetched_at,omitempty"`
	PGPubXMLURL                   string `json:"pgpub_xml_url,omitempty"`
	PGPubXMLName                  string `json:"pgpub_xml_name,omitempty"`
	PatentGrantXMLURL             string `json:"patent_grant_xml_url,omitempty"`
	PatentGrantXMLName            string `json:"patent_grant_xml_name,omitempty"`
}

type USPTOParty struct {
	ApplicationNumber    string `json:"application_number"`
	Role                 string `json:"role"`
	Ordinal              int    `json:"ordinal"`
	NameText             string `json:"name_text,omitempty"`
	FirstName            string `json:"first_name,omitempty"`
	MiddleName           string `json:"middle_name,omitempty"`
	LastName             string `json:"last_name,omitempty"`
	NameSuffix           string `json:"name_suffix,omitempty"`
	OrganizationName     string `json:"organization_name,omitempty"`
	RegistrationNumber   string `json:"registration_number,omitempty"`
	ActiveIndicator      string `json:"active_indicator,omitempty"`
	PractitionerCategory string `json:"practitioner_category,omitempty"`
	AddressJSON          string `json:"address_json,omitempty"`
	TelecomJSON          string `json:"telecom_json,omitempty"`
}

type USPTOEvent struct {
	ApplicationNumber    string `json:"application_number"`
	Ordinal              int    `json:"ordinal"`
	EventCode            string `json:"event_code"`
	EventDescriptionText string `json:"event_description_text,omitempty"`
	EventDate            string `json:"event_date,omitempty"`
}

type USPTOContinuity struct {
	ApplicationNumber                 string       `json:"application_number"`
	Ordinal                           int          `json:"ordinal"`
	ParentApplicationNumberText       string       `json:"parent_application_number_text,omitempty"`
	ChildApplicationNumberText        string       `json:"child_application_number_text,omitempty"`
	ParentApplicationFilingDate       string       `json:"parent_application_filing_date,omitempty"`
	ParentApplicationStatusCode       string       `json:"parent_application_status_code,omitempty"`
	ParentApplicationStatusText       string       `json:"parent_application_status_text,omitempty"`
	ClaimParentageTypeCode            string       `json:"claim_parentage_type_code,omitempty"`
	ClaimParentageTypeDescriptionText string       `json:"claim_parentage_type_description_text,omitempty"`
	ParentRecordNumber                PatentNumber `json:"parent_record_number,omitempty"`
	ChildRecordNumber                 PatentNumber `json:"child_record_number,omitempty"`
}

type USPTOForeignPriority struct {
	ApplicationNumber        string       `json:"application_number"`
	Ordinal                  int          `json:"ordinal"`
	ForeignApplicationNumber string       `json:"foreign_application_number,omitempty"`
	FilingDate               string       `json:"filing_date,omitempty"`
	IPOfficeName             string       `json:"ip_office_name,omitempty"`
	Authority                string       `json:"authority,omitempty"`
	LinkedRecordNumber       PatentNumber `json:"linked_record_number,omitempty"`
}

type SourceSnapshot struct {
	ID             string       `json:"id"`
	PatentNumber   PatentNumber `json:"patent_number"`
	Source         string       `json:"source"`
	SourceRecordID string       `json:"source_record_id,omitempty"`
	SourceURL      string       `json:"source_url,omitempty"`
	FetchedAt      string       `json:"fetched_at"`
	PayloadKind    string       `json:"payload_kind,omitempty"`
	PayloadHash    string       `json:"payload_hash,omitempty"`
	PayloadPath    string       `json:"payload_path,omitempty"`
	ResponseBytes  int64        `json:"response_bytes,omitempty"`
	HTTPStatus     int          `json:"http_status,omitempty"`
	ETag           string       `json:"etag,omitempty"`
	LastModified   string       `json:"last_modified,omitempty"`
	SummaryJSON    string       `json:"summary_json,omitempty"`
}

type SourceDiff struct {
	ID               string       `json:"id"`
	PatentNumber     PatentNumber `json:"patent_number"`
	FieldPath        string       `json:"field_path"`
	USPTOValue       string       `json:"uspto_value,omitempty"`
	GoogleValue      string       `json:"google_value,omitempty"`
	ChosenValue      string       `json:"chosen_value,omitempty"`
	ChosenSource     string       `json:"chosen_source,omitempty"`
	Severity         string       `json:"severity,omitempty"`
	RecordedAt       string       `json:"recorded_at"`
	USPTOSnapshotID  string       `json:"uspto_snapshot_id,omitempty"`
	GoogleSnapshotID string       `json:"google_snapshot_id,omitempty"`
}

type USPTOCandidate struct {
	ApplicationNumber string `json:"application_number"`
	Title             string `json:"title"`
	FilingDate        string `json:"filing_date"`
	FirstInventorName string `json:"first_inventor_name"`
}

