// Package pulumi provides the Pulumi Automation API runtime for in-process
// infrastructure provisioning as specified in ADR-005. The [Runtime] type
// manages Pulumi stack lifecycle (create, update, destroy) using a local
// filesystem backend suitable for development.
//
// For production deployments, configure an object-storage backend by setting
// the PULUMI_BACKEND_URL environment variable before starting the manager.
// The Runtime honours that value when constructing the workspace.
package pulumi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	gopulumi "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ProgramFunc is a Pulumi inline program function that defines the desired
// infrastructure state for a stack. Implementations register Pulumi resources
// and export output values via ctx.Export.
type ProgramFunc = gopulumi.RunFunc

// StackConfig holds plaintext key-value configuration applied to a Pulumi
// stack before execution. Keys follow Pulumi's namespaced format (e.g.
// "hcloud:token").
type StackConfig map[string]string

// OutputMap holds the key-value outputs exported by a Pulumi stack program
// after a successful [Runtime.Up] operation.
type OutputMap map[string]any

// UpResult is returned by [Runtime.Up] and contains the stack outputs after
// a successful run.
type UpResult struct {
	// Outputs contains the values exported by the Pulumi program via
	// ctx.Export.
	Outputs OutputMap
}

// Runtime manages Pulumi stack lifecycle in-process using the Automation API
// (ADR-005). It is safe for concurrent use.
//
// Stack state is stored on the local filesystem under StateDir, which is
// suitable for local development. For production deployments, set
// PULUMI_BACKEND_URL to an S3-compatible object-storage URL before starting
// the manager; the Runtime honours that override.
type Runtime struct {
	projectName string
	stateDir    string
}

// New creates a Runtime that persists stack state in stateDir.
//
// The state directory is created if it does not already exist. Both
// projectName and stateDir must be non-empty. Returns an error if either
// argument is invalid or if the state directory cannot be created.
func New(projectName, stateDir string) (*Runtime, error) {
	if projectName == "" {
		return nil, fmt.Errorf("pulumi: project name must not be empty")
	}
	if stateDir == "" {
		return nil, fmt.Errorf("pulumi: state directory must not be empty")
	}
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return nil, fmt.Errorf("pulumi: cannot resolve state directory path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("pulumi: failed to create state directory %q: %w", abs, err)
	}
	return &Runtime{projectName: projectName, stateDir: abs}, nil
}

// Up creates or selects the named stack, applies cfg, runs program, and
// returns the stack outputs.
//
// If the stack does not exist it is created automatically. Errors from the
// Pulumi engine are wrapped with the stack name and operation so that callers
// can surface actionable context to operators.
func (r *Runtime) Up(ctx context.Context, stackName string, program ProgramFunc, cfg StackConfig) (*UpResult, error) {
	stack, err := r.upsertStack(ctx, stackName, program)
	if err != nil {
		return nil, err
	}
	if err := r.applyConfig(ctx, &stack, cfg); err != nil {
		return nil, err
	}
	res, err := stack.Up(ctx, optup.ProgressStreams(os.Stderr), optup.SuppressOutputs())
	if err != nil {
		return nil, fmt.Errorf("pulumi: stack %q up failed: %w", stackName, err)
	}
	return &UpResult{Outputs: outputsFromMap(res.Outputs)}, nil
}

// Destroy selects the named stack and destroys all its managed resources.
//
// Returns nil if destruction succeeds. Errors are wrapped with the stack name
// and operation context so that callers can surface actionable context.
func (r *Runtime) Destroy(ctx context.Context, stackName string, program ProgramFunc) error {
	stack, err := r.upsertStack(ctx, stackName, program)
	if err != nil {
		return err
	}
	if _, err := stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stderr)); err != nil {
		return fmt.Errorf("pulumi: stack %q destroy failed: %w", stackName, err)
	}
	return nil
}

// StateDir returns the absolute path to the directory used for local stack
// state storage.
func (r *Runtime) StateDir() string {
	return r.stateDir
}

// ProjectName returns the Pulumi project name used by this Runtime.
func (r *Runtime) ProjectName() string {
	return r.projectName
}

// upsertStack creates or selects a local inline stack backed by the file-system
// state backend at r.stateDir.
func (r *Runtime) upsertStack(ctx context.Context, stackName string, program ProgramFunc) (auto.Stack, error) {
	stateURL := "file://" + filepath.ToSlash(r.stateDir)
	proj := workspace.Project{
		Name:    tokens.PackageName(r.projectName),
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
		Backend: &workspace.ProjectBackend{URL: stateURL},
	}
	stack, err := auto.UpsertStackInlineSource(ctx, stackName, r.projectName, program,
		auto.Project(proj),
		auto.EnvVars(map[string]string{
			// An empty passphrase enables the passphrase secrets provider
			// without operator interaction, which is appropriate for local
			// development. Production deployments should use a dedicated
			// secrets provider (e.g. AWS KMS, Azure Key Vault).
			"PULUMI_CONFIG_PASSPHRASE": "",
		}),
	)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("pulumi: failed to create or select stack %q: %w", stackName, err)
	}
	return stack, nil
}

// applyConfig sets each entry in cfg as a plaintext config value on the stack.
func (r *Runtime) applyConfig(ctx context.Context, stack *auto.Stack, cfg StackConfig) error {
	for k, v := range cfg {
		if err := stack.SetConfig(ctx, k, auto.ConfigValue{Value: v}); err != nil {
			return fmt.Errorf("pulumi: failed to set config key %q: %w", k, err)
		}
	}
	return nil
}

// outputsFromMap converts the auto.OutputMap to the simpler OutputMap type,
// stripping the secret metadata that is unnecessary for callers.
func outputsFromMap(m auto.OutputMap) OutputMap {
	out := make(OutputMap, len(m))
	for k, v := range m {
		out[k] = v.Value
	}
	return out
}
