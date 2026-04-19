package pulumi_test

import (
	"os"
	"path/filepath"
	"testing"

	pulumiruntime "github.com/Mewtos7/lx-container-weaver/internal/pulumi"
)

func TestNew_EmptyProjectName(t *testing.T) {
	_, err := pulumiruntime.New("", t.TempDir())
	if err == nil {
		t.Fatal("New: want error for empty projectName, got nil")
	}
}

func TestNew_EmptyStateDir(t *testing.T) {
	_, err := pulumiruntime.New("lx-container-weaver", "")
	if err == nil {
		t.Fatal("New: want error for empty stateDir, got nil")
	}
}

func TestNew_ValidArgs(t *testing.T) {
	dir := t.TempDir()
	rt, err := pulumiruntime.New("lx-container-weaver", dir)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("New: expected non-nil runtime")
	}
}

func TestNew_CreatesStateDir(t *testing.T) {
	// Verify that New creates a nested state directory that does not yet exist.
	base := t.TempDir()
	dir := filepath.Join(base, "subdir", "state")

	_, err := pulumiruntime.New("lx-container-weaver", dir)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		t.Fatalf("New: state directory was not created at %q", dir)
	}
}

func TestNew_StateDirAccessor(t *testing.T) {
	dir := t.TempDir()
	rt, err := pulumiruntime.New("lx-container-weaver", dir)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if rt.StateDir() != abs {
		t.Errorf("StateDir: got %q, want %q", rt.StateDir(), abs)
	}
}

func TestNew_ProjectNameAccessor(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", t.TempDir())
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt.ProjectName() != "lx-container-weaver" {
		t.Errorf("ProjectName: got %q, want %q", rt.ProjectName(), "lx-container-weaver")
	}
}
