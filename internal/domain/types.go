package domain

// Named types for the domain identifiers that were previously raw strings.
// They are compile-time only — identical machine code and memory layout to a
// string — but let the compiler reject swapped or wrong-meaning arguments
// (e.g. GetPatent(ctx, projectID, number) with the two reversed).

// ProjectID identifies a project.
type ProjectID string

// PatentNumber identifies a patent (e.g. "US11611785B2").
type PatentNumber string
