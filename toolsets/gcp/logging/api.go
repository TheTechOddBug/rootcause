package logging

import "context"

// API is the data layer used by logging tool handlers. Production code wires
// up an SDK-backed implementation in the parent gcp toolset; tests inject a
// fake.
type API interface {
	// FetchEntries runs a Cloud Logging filter and returns matching entries.
	FetchEntries(ctx context.Context, project, filter string, limit int) (entries []map[string]any, usedProject string, err error)
}
