package monitoring

import (
	"context"
	"errors"
	"strings"
	"testing"

	"rootcause/internal/mcp"
)

// fakeAPI is an in-memory MetricsAPI for handler tests.
type fakeAPI struct {
	mqlSeries       []map[string]any
	mqlProject      string
	mqlErr          error
	mqlSeen         string
	descriptors     []map[string]any
	descriptorsErr  error
	descriptorsSeen string
	services        []map[string]any
	servicesProject string
	servicesErr     error
	slosByService   map[string][]map[string]any
	slosSeen        []string
}

func (f *fakeAPI) RunMQL(ctx context.Context, project, mql string) ([]map[string]any, string, error) {
	f.mqlSeen = mql
	if f.mqlErr != nil {
		return nil, "", f.mqlErr
	}
	used := f.mqlProject
	if used == "" {
		used = project
	}
	return f.mqlSeries, used, nil
}

func (f *fakeAPI) ListDescriptors(ctx context.Context, project, filter string, limit int) ([]map[string]any, string, error) {
	f.descriptorsSeen = filter
	if f.descriptorsErr != nil {
		return nil, "", f.descriptorsErr
	}
	return f.descriptors, project, nil
}

func (f *fakeAPI) ListServices(ctx context.Context, project string, limit int) ([]map[string]any, string, error) {
	if f.servicesErr != nil {
		return nil, "", f.servicesErr
	}
	used := f.servicesProject
	if used == "" {
		used = project
	}
	return f.services, used, nil
}

func (f *fakeAPI) ListSLOs(ctx context.Context, serviceName string, limit int) ([]map[string]any, error) {
	f.slosSeen = append(f.slosSeen, serviceName)
	if f.slosByService == nil {
		return nil, nil
	}
	return f.slosByService[serviceName], nil
}

func newService(api API) *Service {
	return &Service{api: api}
}

func TestHandleQueryRequiresQuery(t *testing.T) {
	svc := newService(&fakeAPI{})
	_, err := svc.handleQuery(context.Background(), mcp.ToolRequest{Arguments: map[string]any{}})
	if err == nil {
		t.Fatalf("expected error when query is missing")
	}
}

func TestHandleQuerySuccess(t *testing.T) {
	fake := &fakeAPI{
		mqlSeries:  []map[string]any{{"labelValues": []string{"a"}, "pointCount": 3}},
		mqlProject: "my-proj",
	}
	svc := newService(fake)
	res, err := svc.handleQuery(context.Background(), mcp.ToolRequest{Arguments: map[string]any{"query": "fetch k8s_container"}})
	if err != nil {
		t.Fatalf("handleQuery: %v", err)
	}
	root := res.Data.(map[string]any)
	if root["project"] != "my-proj" {
		t.Errorf("expected project=my-proj, got %v", root["project"])
	}
	if root["count"].(int) != 1 {
		t.Errorf("expected count=1, got %v", root["count"])
	}
	if fake.mqlSeen != "fetch k8s_container" {
		t.Errorf("expected MQL forwarded, got %q", fake.mqlSeen)
	}
}

func TestHandleWorkloadRequiresFields(t *testing.T) {
	svc := newService(&fakeAPI{})
	_, err := svc.handleWorkload(context.Background(), mcp.ToolRequest{Arguments: map[string]any{"namespace": ""}})
	if err == nil {
		t.Fatalf("expected error when namespace/workload missing")
	}
}

func TestHandleWorkloadAggregatesPartialErrors(t *testing.T) {
	calls := 0
	fake := &errAfterFakeAPI{
		series: []map[string]any{{"labelValues": []string{}, "points": []map[string]any{}}},
		errOn: map[int]error{
			1: errors.New("memory failed"),
		},
		callCount: &calls,
	}
	svc := newService(fake)
	res, err := svc.handleWorkload(context.Background(), mcp.ToolRequest{Arguments: map[string]any{
		"namespace": "payments",
		"workload":  "api",
	}})
	if err != nil {
		t.Fatalf("handleWorkload should not hard-fail on partial errors: %v", err)
	}
	root := res.Data.(map[string]any)
	errs, ok := root["errors"].(map[string]string)
	if !ok {
		t.Fatalf("expected errors map, got %v", root["errors"])
	}
	if _, has := errs["memory"]; !has {
		t.Errorf("expected memory error to be reported, got %v", errs)
	}
	if errs["cpu"] != "" {
		t.Errorf("expected no cpu error, got %v", errs["cpu"])
	}
}

func TestHandleListDescriptorsForwardsFilter(t *testing.T) {
	fake := &fakeAPI{descriptors: []map[string]any{{"type": "kubernetes.io/container/cpu/core_usage_time"}}}
	svc := newService(fake)
	res, err := svc.handleListDescriptors(context.Background(), mcp.ToolRequest{Arguments: map[string]any{
		"projectId": "p",
		"filter":    `metric.type=starts_with("kubernetes.io/")`,
	}})
	if err != nil {
		t.Fatalf("handleListDescriptors: %v", err)
	}
	root := res.Data.(map[string]any)
	if root["count"].(int) != 1 {
		t.Errorf("expected count=1, got %v", root["count"])
	}
	if !strings.Contains(fake.descriptorsSeen, "kubernetes.io/") {
		t.Errorf("expected filter forwarded, got %q", fake.descriptorsSeen)
	}
}

func TestHandleSLOListEnumeratesPerService(t *testing.T) {
	fake := &fakeAPI{
		services: []map[string]any{
			{"name": "projects/p/services/checkout", "id": "checkout", "displayName": "Checkout"},
			{"name": "projects/p/services/payments", "id": "payments", "displayName": "Payments"},
		},
		slosByService: map[string][]map[string]any{
			"projects/p/services/checkout": {{"name": "projects/p/services/checkout/serviceLevelObjectives/o1", "goal": 0.999}},
			"projects/p/services/payments": {},
		},
	}
	svc := newService(fake)
	res, err := svc.handleSLOList(context.Background(), mcp.ToolRequest{Arguments: map[string]any{"projectId": "p"}})
	if err != nil {
		t.Fatalf("handleSLOList: %v", err)
	}
	root := res.Data.(map[string]any)
	if root["serviceCount"].(int) != 2 {
		t.Errorf("expected serviceCount=2, got %v", root["serviceCount"])
	}
	if root["objectiveCount"].(int) != 1 {
		t.Errorf("expected objectiveCount=1, got %v", root["objectiveCount"])
	}
	if len(fake.slosSeen) != 2 {
		t.Errorf("expected ListSLOs called for each service, got %v", fake.slosSeen)
	}
}

func TestHandleSLOListWithExplicitServiceID(t *testing.T) {
	fake := &fakeAPI{
		services: []map[string]any{{"name": "projects/p/services/anything"}},
		slosByService: map[string][]map[string]any{
			"projects/p/services/checkout": {{"name": "x"}},
		},
	}
	svc := newService(fake)
	res, err := svc.handleSLOList(context.Background(), mcp.ToolRequest{Arguments: map[string]any{
		"projectId": "p",
		"serviceId": "checkout",
	}})
	if err != nil {
		t.Fatalf("handleSLOList: %v", err)
	}
	root := res.Data.(map[string]any)
	services := root["services"].([]map[string]any)
	if len(services) != 1 || services[0]["id"] != "checkout" {
		t.Errorf("expected single service entry for checkout, got %v", services)
	}
	if len(fake.slosSeen) != 1 {
		t.Errorf("expected single SLO lookup, got %v", fake.slosSeen)
	}
}

func TestHandleListDescriptorsPropagatesError(t *testing.T) {
	fake := &fakeAPI{descriptorsErr: errors.New("denied")}
	svc := newService(fake)
	_, err := svc.handleListDescriptors(context.Background(), mcp.ToolRequest{Arguments: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected denied error, got %v", err)
	}
}

// errAfterFakeAPI returns an error on the Nth RunMQL call. Used to simulate
// partial-failure aggregation in handleWorkload.
type errAfterFakeAPI struct {
	series    []map[string]any
	errOn     map[int]error
	callCount *int
}

func (f *errAfterFakeAPI) RunMQL(ctx context.Context, project, mql string) ([]map[string]any, string, error) {
	idx := *f.callCount
	*f.callCount++
	if err, ok := f.errOn[idx]; ok {
		return nil, project, err
	}
	return f.series, project, nil
}
func (f *errAfterFakeAPI) ListDescriptors(ctx context.Context, project, filter string, limit int) ([]map[string]any, string, error) {
	return nil, project, nil
}
func (f *errAfterFakeAPI) ListServices(ctx context.Context, project string, limit int) ([]map[string]any, string, error) {
	return nil, project, nil
}
func (f *errAfterFakeAPI) ListSLOs(ctx context.Context, serviceName string, limit int) ([]map[string]any, error) {
	return nil, nil
}
