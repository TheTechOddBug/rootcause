package monitoring

import "context"

// API is the data layer used by monitoring tool handlers. Production code wires
// up an SDK-backed implementation in the parent gcp toolset; tests inject a
// fake.
type API interface {
	// RunMQL executes an MQL query and returns encoded time series.
	RunMQL(ctx context.Context, project, mql string) (series []map[string]any, usedProject string, err error)
	// ListDescriptors enumerates metric descriptors matching filter.
	ListDescriptors(ctx context.Context, project, filter string, limit int) (descriptors []map[string]any, usedProject string, err error)
	// ListServices enumerates Service Monitoring services.
	ListServices(ctx context.Context, project string, limit int) (services []map[string]any, usedProject string, err error)
	// ListSLOs enumerates SLOs for a fully-qualified service name.
	ListSLOs(ctx context.Context, serviceName string, limit int) ([]map[string]any, error)
}
