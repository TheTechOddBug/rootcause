package monitoring

import (
	"strings"
	"testing"
	"time"

	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestServiceIDFromName(t *testing.T) {
	cases := map[string]string{
		"projects/my-proj/services/checkout": "checkout",
		"projects/p/services/svc-with-slash": "svc-with-slash",
		"no-slash":                           "no-slash",
		"":                                   "",
	}
	for in, want := range cases {
		if got := serviceIDFromName(in); got != want {
			t.Errorf("serviceIDFromName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeSLOIncludesGoalAndPeriod(t *testing.T) {
	obj := &monitoringpb.ServiceLevelObjective{
		Name:        "projects/p/services/s/serviceLevelObjectives/o",
		DisplayName: "Checkout availability",
		Goal:        0.995,
		Period: &monitoringpb.ServiceLevelObjective_RollingPeriod{
			RollingPeriod: durationpb.New(24 * time.Hour),
		},
		ServiceLevelIndicator: &monitoringpb.ServiceLevelIndicator{
			Type: &monitoringpb.ServiceLevelIndicator_BasicSli{},
		},
	}
	got := encodeSLO(obj)
	if got["name"] != obj.Name {
		t.Errorf("expected name %q, got %v", obj.Name, got["name"])
	}
	if got["displayName"] != obj.DisplayName {
		t.Errorf("expected displayName %q, got %v", obj.DisplayName, got["displayName"])
	}
	if got["goal"].(float64) != 0.995 {
		t.Errorf("expected goal=0.995, got %v", got["goal"])
	}
	if got["rollingPeriod"] != "24h0m0s" {
		t.Errorf("expected rollingPeriod=24h0m0s, got %v", got["rollingPeriod"])
	}
	if got["indicatorType"] != "basicSli" {
		t.Errorf("expected indicatorType=basicSli, got %v", got["indicatorType"])
	}
}

func TestEncodeSLOIndicatorVariants(t *testing.T) {
	cases := map[string]*monitoringpb.ServiceLevelIndicator{
		"basicSli":     {Type: &monitoringpb.ServiceLevelIndicator_BasicSli{}},
		"requestBased": {Type: &monitoringpb.ServiceLevelIndicator_RequestBased{}},
		"windowsBased": {Type: &monitoringpb.ServiceLevelIndicator_WindowsBased{}},
	}
	for want, sli := range cases {
		obj := &monitoringpb.ServiceLevelObjective{ServiceLevelIndicator: sli}
		got := encodeSLO(obj)
		if got["indicatorType"] != want {
			t.Errorf("indicator %s: got %v", want, got["indicatorType"])
		}
	}
}

func TestEncodeSLOWithoutIndicatorOmitsField(t *testing.T) {
	obj := &monitoringpb.ServiceLevelObjective{Goal: 0.9}
	got := encodeSLO(obj)
	if _, ok := got["indicatorType"]; ok {
		t.Errorf("expected no indicatorType when SLI absent, got %v", got)
	}
}

func TestWorkloadCPUQueryHasShape(t *testing.T) {
	got := workloadCPUQuery("payments", "api", 30*time.Minute)
	for _, must := range []string{
		"fetch k8s_container",
		"namespace_name = 'payments'",
		"pod_name =~ 'api-.*'",
		"cpu/core_usage_time",
		"rate(1m)",
		"within 30m",
	} {
		if !strings.Contains(got, must) {
			t.Errorf("workloadCPUQuery missing %q in %s", must, got)
		}
	}
}

func TestDurationLiteral(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "30m",
		-time.Second:     "30m",
		time.Hour:        "1h",
		2 * time.Hour:    "2h",
		15 * time.Minute: "15m",
		90 * time.Second: "90s",
	}
	for in, want := range cases {
		if got := durationLiteral(in); got != want {
			t.Errorf("durationLiteral(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDurationFallback(t *testing.T) {
	if d := parseDuration("", 5*time.Minute); d != 5*time.Minute {
		t.Errorf("empty -> fallback expected, got %v", d)
	}
	if d := parseDuration("bogus", 5*time.Minute); d != 5*time.Minute {
		t.Errorf("invalid -> fallback expected, got %v", d)
	}
	if d := parseDuration("15m", time.Hour); d != 15*time.Minute {
		t.Errorf("expected 15m, got %v", d)
	}
}

func TestEscapeQuotes(t *testing.T) {
	if got := escape("won't"); got != `won\'t` {
		t.Errorf("escape(won't) = %q", got)
	}
}
