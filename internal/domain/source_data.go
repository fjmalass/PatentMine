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

// USPTOApplication is the normalized subset of File Wrapper application attrs.
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

// USPTOGrantSummary is the single-row data extracted from a us-patent-grant
// XML envelope: counts, examiner, attorney, term, dates.
type USPTOGrantSummary struct {
	ApplicationNumber     string   `json:"application_number"`
	GrantDocNumber        string   `json:"grant_doc_number,omitempty"`
	GrantKind             string   `json:"grant_kind,omitempty"`
	GrantDate             string   `json:"grant_date,omitempty"`
	GrantDTDVersion       string   `json:"grant_dtd_version,omitempty"`
	GrantStatus           string   `json:"grant_status,omitempty"`
	GrantDateProduced     string   `json:"grant_date_produced,omitempty"`
	GrantFileName         string   `json:"grant_file_name,omitempty"`
	GrantLang             string   `json:"grant_lang,omitempty"`
	TermExtensionDays     int      `json:"term_extension_days,omitempty"`
	NumberOfClaims        int      `json:"number_of_claims,omitempty"`
	ExemplaryClaim        string   `json:"exemplary_claim,omitempty"`
	NumberOfDrawingSheets int      `json:"number_of_drawing_sheets,omitempty"`
	NumberOfFigures       int      `json:"number_of_figures,omitempty"`
	PrimaryExaminerFirst  string   `json:"primary_examiner_first,omitempty"`
	PrimaryExaminerLast   string   `json:"primary_examiner_last,omitempty"`
	PrimaryExaminerDept   string   `json:"primary_examiner_department,omitempty"`
	AssistantExaminers    []string `json:"assistant_examiners,omitempty"`
	AttorneyOrg           string   `json:"attorney_org,omitempty"`
	AttorneyType          string   `json:"attorney_type,omitempty"`
	FieldOfSearch         []string `json:"field_of_search,omitempty"`
	ParsedAt              string   `json:"parsed_at,omitempty"`
}

// USPTOGrantClaim is one numbered claim of a patent.
type USPTOGrantClaim struct {
	Number string `json:"number"`
	Text   string `json:"text"`
	XML    string `json:"xml,omitempty"`
}

// USPTOGrantBody is the human-readable body of the patent: abstract,
// description and claims.
type USPTOGrantBody struct {
	ApplicationNumber string            `json:"application_number"`
	Kind              string            `json:"kind"`
	AbstractText      string            `json:"abstract_text,omitempty"`
	AbstractXML       string            `json:"abstract_xml,omitempty"`
	DescriptionText   string            `json:"description_text,omitempty"`
	DescriptionXML   string            `json:"description_xml,omitempty"`
	ClaimStatement    string            `json:"claim_statement,omitempty"`
	ClaimsText        string            `json:"claims_text,omitempty"`
	Claims            []USPTOGrantClaim `json:"claims,omitempty"`
	ParsedAt          string            `json:"parsed_at,omitempty"`
}

// USPTODrawing is one figure referenced from the drawings section.
type USPTODrawing struct {
	ApplicationNumber string `json:"application_number"`
	Kind              string `json:"kind"`
	Ordinal           int    `json:"ordinal"`
	FigureNum         string `json:"figure_num,omitempty"`
	FigureID          string `json:"figure_id,omitempty"`
	ImgID             string `json:"img_id,omitempty"`
	FileName          string `json:"file_name,omitempty"`
	Width             string `json:"width,omitempty"`
	Height            string `json:"height,omitempty"`
	AltText           string `json:"alt_text,omitempty"`
	ImgFormat         string `json:"img_format,omitempty"`
	ImgContent        string `json:"img_content,omitempty"`
}

// USPTOGrantCitation is one cited reference (patent or non-patent literature).
type USPTOGrantCitation struct {
	ApplicationNumber string `json:"application_number"`
	Kind              string `json:"kind"`
	Ordinal           int    `json:"ordinal"`
	CitationNum       string `json:"citation_num,omitempty"`
	CitationType      string `json:"citation_type"`
	Category          string `json:"category,omitempty"`
	CitedCountry      string `json:"cited_country,omitempty"`
	CitedDocNumber    string `json:"cited_doc_number,omitempty"`
	CitedKind         string `json:"cited_kind,omitempty"`
	CitedDate         string `json:"cited_date,omitempty"`
	CitedName         string `json:"cited_name,omitempty"`
	CPCText           string `json:"cpc_text,omitempty"`
	NationalCountry   string `json:"national_country,omitempty"`
	NationalClass     string `json:"national_class,omitempty"`
	NPLText           string `json:"npl_text,omitempty"`
}

// USPTOGrantClassification is one IPCR or CPC classification row.
type USPTOGrantClassification struct {
	ApplicationNumber    string `json:"application_number"`
	Kind                 string `json:"kind"`
	Scheme               string `json:"scheme"`
	Role                 string `json:"role"`
	Ordinal              int    `json:"ordinal"`
	FullCode             string `json:"full_code"`
	Section              string `json:"section,omitempty"`
	Class                string `json:"class,omitempty"`
	Subclass             string `json:"subclass,omitempty"`
	MainGroup            string `json:"main_group,omitempty"`
	Subgroup             string `json:"subgroup,omitempty"`
	SymbolPosition       string `json:"symbol_position,omitempty"`
	ClassificationValue  string `json:"classification_value,omitempty"`
	ClassificationLevel  string `json:"classification_level,omitempty"`
	ClassificationStatus string `json:"classification_status,omitempty"`
	DataSource           string `json:"data_source,omitempty"`
	ActionDate           string `json:"action_date,omitempty"`
	GeneratingOffice     string `json:"generating_office,omitempty"`
	VersionDate          string `json:"version_date,omitempty"`
	SchemeOrigination    string `json:"scheme_origination,omitempty"`
}

// USPTOGrantRelation is one family relationship parsed from us-related-documents.
type USPTOGrantRelation struct {
	ApplicationNumber  string `json:"application_number"`
	Kind               string `json:"kind"`
	Ordinal            int    `json:"ordinal"`
	RelationKind       string `json:"relation_kind"`
	ParentCountry      string `json:"parent_country,omitempty"`
	ParentAppNumber    string `json:"parent_app_number,omitempty"`
	ParentAppDate      string `json:"parent_app_date,omitempty"`
	ParentGrantCountry string `json:"parent_grant_country,omitempty"`
	ParentGrantNumber  string `json:"parent_grant_number,omitempty"`
	ParentGrantDate    string `json:"parent_grant_date,omitempty"`
	ChildCountry       string `json:"child_country,omitempty"`
	ChildAppNumber     string `json:"child_app_number,omitempty"`
	RelatedCountry     string `json:"related_country,omitempty"`
	RelatedDocNumber   string `json:"related_doc_number,omitempty"`
	RelatedKind        string `json:"related_kind,omitempty"`
	RelatedDate        string `json:"related_date,omitempty"`
}

// USPTOGrantIngest is the bundle of parsed grant XML data; persisting it is a
// single transactional save.
type USPTOGrantIngest struct {
	Summary         USPTOGrantSummary          `json:"summary"`
	Body            USPTOGrantBody             `json:"body"`
	Drawings        []USPTODrawing             `json:"drawings,omitempty"`
	Citations       []USPTOGrantCitation       `json:"citations,omitempty"`
	Classifications []USPTOGrantClassification `json:"classifications,omitempty"`
	Relations       []USPTOGrantRelation       `json:"relations,omitempty"`
}

// USPTOXMLDownload tracks per-document XML downloads (pgpub or grant).
// download_count is bumped on every fetch (including cache hits), so callers
// can see how often each document has been requested.
type USPTOXMLDownload struct {
	ApplicationNumber string `json:"application_number"`
	Kind              string `json:"kind"`
	SourceURL         string `json:"source_url"`
	LocalPath         string `json:"local_path"`
	Bytes             int64  `json:"bytes"`
	DownloadCount     int64  `json:"download_count"`
	FirstDownloadedAt string `json:"first_downloaded_at,omitempty"`
	LastDownloadedAt  string `json:"last_downloaded_at,omitempty"`
	LastAccessedAt    string `json:"last_accessed_at,omitempty"`
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

