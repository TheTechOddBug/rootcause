package gcp

import "testing"

func TestResolveProjectExplicit(t *testing.T) {
	if got := ResolveProject("my-project"); got != "my-project" {
		t.Fatalf("expected explicit value, got %q", got)
	}
}

func TestResolveProjectFromEnv(t *testing.T) {
	t.Setenv(envProject, "from-env")
	t.Setenv(envProjectAlt, "")
	if got := ResolveProject(""); got != "from-env" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestResolveProjectFromAlt(t *testing.T) {
	t.Setenv(envProject, "")
	t.Setenv(envProjectAlt, "from-alt")
	if got := ResolveProject(""); got != "from-alt" {
		t.Fatalf("expected alt env value, got %q", got)
	}
}

func TestResolveProjectEmpty(t *testing.T) {
	t.Setenv(envProject, "")
	t.Setenv(envProjectAlt, "")
	if got := ResolveProject("   "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestResolveProjectPrefersExplicitOverEnv(t *testing.T) {
	t.Setenv(envProject, "env-proj")
	t.Setenv(envProjectAlt, "alt-proj")
	if got := ResolveProject("explicit"); got != "explicit" {
		t.Fatalf("expected explicit to win, got %q", got)
	}
}
