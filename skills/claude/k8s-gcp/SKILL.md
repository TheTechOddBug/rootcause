---
category: Cloud Observability
description: GCP Cloud Monitoring + Cloud Logging triage for Kubernetes workloads (any cluster shipping to a GCP project).
tags: [gcp, observability, incident, rootcause]
---

# Skill: k8s-gcp

GCP-side observability for Kubernetes incidents using RootCause MCP tool names only.

This skill is evidence-first. Project identity is **independent** of the cluster's control plane: an EKS or AKS cluster can ship telemetry to a GCP project. Never infer the GCP project from the kubeconfig context — require it from the user or `GOOGLE_CLOUD_PROJECT`.

## Purpose

Use this skill for:
- triaging workload health using GCP Cloud Monitoring metrics,
- pulling Cloud Logging errors/warnings for a workload,
- finding the inflection point of an incident via bucketed error timelines,
- correlating logs with a `rootcause.incident_bundle` event window,
- discovering available metric descriptors and SLO configuration in a project.

## Strict Tooling Contract

Use only these GCP tool names:
- `gcp.metrics.query`
- `gcp.metrics.workload`
- `gcp.metrics.list_descriptors`
- `gcp.metrics.slo_list`
- `gcp.logs.query`
- `gcp.logs.workload`
- `gcp.logs.error_timeline`
- `gcp.logs.correlated_with_bundle`

Pair with these RootCause tools for evidence and correlation:
- `rootcause.incident_bundle` (pass both `namespace` and `workload` so GCP steps trigger automatically)
- `rootcause.change_timeline`
- `rootcause.rca_generate`

## Triggers

Enable when user intent includes:
- "diagnose workload using GCP",
- "show me errors for service in Cloud Logging",
- "what's the error rate trend in GCP",
- "find the inflection point",
- "correlate logs with the incident timeline",
- "what SLOs do we have",
- "list GCP metrics for kubernetes".

## Workflow

1. **Confirm project**. If `projectId` is not provided, confirm `GOOGLE_CLOUD_PROJECT` / `GCP_PROJECT` is set. Do not infer from kubeconfig.
2. **Build evidence**. Call `rootcause.incident_bundle` with `namespace` + `workload` set. This triggers `gcp.metrics.workload` and `gcp.logs.workload` automatically when the toolset is enabled.
3. **Find the inflection point**. Call `gcp.logs.error_timeline` with the same namespace+workload. Use bucketSize `1m` for narrow incidents (≤15m), `5m` for normal, `15m` for multi-hour.
4. **Inspect truncation**. If the response sets `truncated: true`, buckets older than `oldestSeen` are incomplete. Raise `scanLimit` or narrow `duration` before drawing conclusions about pre-incident baseline.
5. **Pull correlated logs**. Call `gcp.logs.correlated_with_bundle` with the bundle from step 2 to get the exact log entries inside the bundle's event window.
6. **SLO context**. If the team has SLOs, call `gcp.metrics.slo_list` to surface goal/period. Live burn-rate is out of scope — use `gcp.metrics.query` with `select_slo_burn_rate(...)` MQL when needed.
7. **Discovery**. When a metric type is unfamiliar, call `gcp.metrics.list_descriptors` with a filter like `metric.type=starts_with("kubernetes.io/")` to enumerate available signals.

## Output Contract

- Time-aligned summary: k8s events vs GCP metric anomalies vs error timeline buckets.
- Identified inflection point with bucket evidence.
- Root-cause hypothesis citing specific metric + log entries (with timestamps).
- Whether the data is complete or `truncated` — flag clearly if incomplete.
- Remediation actions and validation checks.

## Safety

All GCP tools are read-only. They never mutate cloud resources. Cloud Logging entries may contain PII or secrets — rely on the redactor pipeline and avoid echoing raw payloads in postmortems without review.
