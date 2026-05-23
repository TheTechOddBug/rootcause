package gcp

import (
	"os"
	"strings"
)

const (
	envProject     = "GOOGLE_CLOUD_PROJECT"
	envProjectAlt  = "GCP_PROJECT"
	envCredentials = "GOOGLE_APPLICATION_CREDENTIALS"
)

// ResolveProject returns the explicit project id when non-empty, else the value
// of GOOGLE_CLOUD_PROJECT, else GCP_PROJECT, else "". Project resolution is
// intentionally decoupled from the active kubeconfig context: EKS/AKS/GKE
// clusters can all ship telemetry to GCP, so the observability project must
// come from the user, not from cluster identity.
func ResolveProject(explicit string) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(envProject)); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(envProjectAlt)); v != "" {
		return v
	}
	return ""
}

func CredentialsFile() string {
	return strings.TrimSpace(os.Getenv(envCredentials))
}
