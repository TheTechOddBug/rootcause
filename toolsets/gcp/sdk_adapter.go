package gcp

import (
	"context"
	"fmt"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/iterator"

	gcplogging "rootcause/toolsets/gcp/logging"
	gcpmonitoring "rootcause/toolsets/gcp/monitoring"
)

// sdkMetricsAPI implements monitoring.API by wrapping the live GCP SDK clients
// constructed lazily through the parent toolset's per-project cache.
type sdkMetricsAPI struct {
	t *Toolset
}

func newSDKMetricsAPI(t *Toolset) gcpmonitoring.API {
	return &sdkMetricsAPI{t: t}
}

func (a *sdkMetricsAPI) RunMQL(ctx context.Context, project, mql string) ([]map[string]any, string, error) {
	client, usedProject, err := a.t.queryClient(ctx, project)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		return nil, usedProject, fmt.Errorf("monitoring query client is nil")
	}
	it := client.QueryTimeSeries(ctx, &monitoringpb.QueryTimeSeriesRequest{
		Name:  "projects/" + usedProject,
		Query: mql,
	})
	out := make([]map[string]any, 0)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, usedProject, err
		}
		out = append(out, gcpmonitoring.EncodeTimeSeriesData(resp))
		if len(out) >= 200 {
			break
		}
	}
	return out, usedProject, nil
}

func (a *sdkMetricsAPI) ListDescriptors(ctx context.Context, project, filter string, limit int) ([]map[string]any, string, error) {
	client, usedProject, err := a.t.metricClient(ctx, project)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		return nil, usedProject, fmt.Errorf("metric client is nil")
	}
	it := client.ListMetricDescriptors(ctx, &monitoringpb.ListMetricDescriptorsRequest{
		Name:   "projects/" + usedProject,
		Filter: filter,
	})
	out := make([]map[string]any, 0, limit)
	for len(out) < limit {
		md, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, usedProject, err
		}
		out = append(out, gcpmonitoring.EncodeMetricDescriptor(gcpmonitoring.MetricDescriptorView{
			Type:        md.GetType(),
			DisplayName: md.GetDisplayName(),
			Description: md.GetDescription(),
			Unit:        md.GetUnit(),
			MetricKind:  md.GetMetricKind().String(),
			ValueType:   md.GetValueType().String(),
			Labels:      gcpmonitoring.EncodeLabelDescriptors(md.GetLabels()),
		}))
	}
	return out, usedProject, nil
}

func (a *sdkMetricsAPI) ListServices(ctx context.Context, project string, limit int) ([]map[string]any, string, error) {
	client, usedProject, err := a.t.slmClient(ctx, project)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		return nil, usedProject, fmt.Errorf("service monitoring client is nil")
	}
	it := client.ListServices(ctx, &monitoringpb.ListServicesRequest{
		Parent: "projects/" + usedProject,
	})
	out := make([]map[string]any, 0, limit)
	for len(out) < limit {
		svc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, usedProject, err
		}
		out = append(out, map[string]any{
			"name":        svc.GetName(),
			"id":          gcpmonitoring.ServiceIDFromName(svc.GetName()),
			"displayName": svc.GetDisplayName(),
		})
	}
	return out, usedProject, nil
}

func (a *sdkMetricsAPI) ListSLOs(ctx context.Context, serviceName string, limit int) ([]map[string]any, error) {
	// serviceName is a full parent like "projects/P/services/S". We need a
	// service-monitoring client; the project is encoded in the name.
	project := projectFromServiceName(serviceName)
	client, _, err := a.t.slmClient(ctx, project)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("service monitoring client is nil")
	}
	it := client.ListServiceLevelObjectives(ctx, &monitoringpb.ListServiceLevelObjectivesRequest{
		Parent: serviceName,
	})
	out := make([]map[string]any, 0, limit)
	for len(out) < limit {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		out = append(out, gcpmonitoring.EncodeSLO(obj))
	}
	return out, nil
}

func projectFromServiceName(name string) string {
	// "projects/X/services/Y" -> "X"
	const prefix = "projects/"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return ""
	}
	rest := name[len(prefix):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			return rest[:i]
		}
	}
	return rest
}

// sdkLoggingAPI implements logging.API by wrapping the live Cloud Logging Admin
// SDK client.
type sdkLoggingAPI struct {
	t *Toolset
}

func newSDKLoggingAPI(t *Toolset) gcplogging.API {
	return &sdkLoggingAPI{t: t}
}

func (a *sdkLoggingAPI) FetchEntries(ctx context.Context, project, filter string, limit int) ([]map[string]any, string, error) {
	client, usedProject, err := a.t.logClient(ctx, project)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		return nil, usedProject, fmt.Errorf("logging client is nil")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	it := client.Entries(ctx, logadmin.Filter(filter), logadmin.NewestFirst())
	out := make([]map[string]any, 0, limit)
	for len(out) < limit {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, usedProject, err
		}
		out = append(out, gcplogging.EncodeEntry(entry))
	}
	return out, usedProject, nil
}

// Ensure compile-time interface conformance.
var (
	_ gcpmonitoring.API = (*sdkMetricsAPI)(nil)
	_ gcplogging.API    = (*sdkLoggingAPI)(nil)
	_                   = monitoring.NewQueryClient // keep monitoring import referenced if other usages drop
)
