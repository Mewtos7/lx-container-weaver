package bootstrap

import (
	"context"
	"fmt"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
)

// ReadinessChecker is a precondition check that must pass before the bootstrap
// workflow attempts LXD cluster formation on a newly provisioned node.
//
// Each check is independent: the workflow runs them in order and aborts at the
// first failure so that bootstrap is never attempted against an unhealthy node.
type ReadinessChecker interface {
	// Name returns a human-readable label for the check. The value is included
	// in [Result.FailedStep] when the check fails (format: "readiness:<name>")
	// so that callers can identify which precondition was not met.
	Name() string

	// Check performs the precondition check. It returns nil when the
	// precondition is satisfied, or a descriptive error when it is not.
	Check(ctx context.Context) error
}

// LXDReadinessCheck verifies that an LXD endpoint is reachable and responsive
// by calling [lxd.Client.GetClusterStatus]. This is the standard gate used
// before attempting cluster bootstrap on a newly provisioned node: if LXD is
// not yet running the call will fail and the workflow will defer the attempt.
type LXDReadinessCheck struct {
	name   string
	client lxd.Client
}

// NewLXDReadinessCheck creates a [ReadinessChecker] that calls
// [lxd.Client.GetClusterStatus] on client. name is used in log output and in
// [Result.FailedStep] when the check fails.
func NewLXDReadinessCheck(name string, client lxd.Client) *LXDReadinessCheck {
	return &LXDReadinessCheck{name: name, client: client}
}

// Name returns the check label supplied at construction time.
func (c *LXDReadinessCheck) Name() string { return c.name }

// Check calls [lxd.Client.GetClusterStatus] on the configured client. A
// successful call confirms the LXD endpoint is reachable and operational. A
// failure (e.g. the server has not finished booting or LXD is not yet
// installed) is returned as-is so the caller can log or inspect it.
func (c *LXDReadinessCheck) Check(ctx context.Context) error {
	if _, err := c.client.GetClusterStatus(ctx); err != nil {
		return fmt.Errorf("LXD endpoint not ready: %w", err)
	}
	return nil
}

// Result records the observable outcome of a single [Workflow.Run] call.
//
// Callers must inspect [Result.Ready] before treating a node as available
// capacity. A failed workflow always sets [Result.Ready] to false and populates
// [Result.Err] and [Result.FailedStep] so that the failure can be recorded for
// later reconciliation decisions.
type Result struct {
	// NodeName is the name of the node that the workflow was run against. It
	// matches the nodeName argument passed to [Workflow.Run] and is included
	// here so the result is self-describing when stored or logged by the caller.
	NodeName string

	// Ready is true when all readiness checks passed and the LXD bootstrap
	// completed successfully. When false the node must not be scheduled for
	// workloads; the caller should update the node's status to the error state
	// and record [Result.Err] for operator visibility.
	Ready bool

	// FailedStep identifies the workflow step that did not complete
	// successfully. It is empty when [Result.Ready] is true.
	//
	// Format:
	//   - "readiness:<check-name>" when a [ReadinessChecker] failed.
	//   - "bootstrap" when [Bootstrapper.Bootstrap] failed.
	FailedStep string

	// Err is the error returned by the failed step. It is nil when
	// [Result.Ready] is true.
	Err error
}

// Workflow orchestrates the full node bootstrap sequence:
//
//  1. Precondition / readiness checks — verifies that each registered
//     [ReadinessChecker] passes before bootstrap is attempted. The first
//     failing check aborts the run without calling the bootstrapper, preventing
//     a partially initialised cluster state.
//  2. LXD cluster formation — delegates to a [Bootstrapper] which handles the
//     seed-node initialisation and joiner-node attachment.
//
// # Failure semantics
//
// A failed [Workflow.Run] always sets [Result.Ready] to false and populates
// [Result.Err]. The node is never silently marked as usable when bootstrap has
// not completed successfully. Callers are responsible for recording the result
// (e.g. updating the node status in the repository) so it is available for
// later reconciliation decisions.
//
// # Retry behaviour
//
// Workflow itself does not retry. Transient failures (e.g. the LXD endpoint
// not yet reachable on a freshly provisioned server) surface as a failed
// [Result] with a descriptive error. The calling reconciliation loop decides
// when and how often to retry based on the node's persisted status.
type Workflow struct {
	checks       []ReadinessChecker
	bootstrapper *Bootstrapper
}

// NewWorkflow creates a [Workflow]. bootstrapper performs the LXD cluster
// formation step. checks are run as preconditions before bootstrap is
// attempted; they are evaluated in the order provided, and the first failure
// aborts the run.
func NewWorkflow(bootstrapper *Bootstrapper, checks ...ReadinessChecker) *Workflow {
	return &Workflow{
		checks:       checks,
		bootstrapper: bootstrapper,
	}
}

// Run executes the bootstrap workflow for the named node and returns a [Result]
// that describes the outcome. nodeName is stored in the result for
// identification by the caller; cfg is passed to [Bootstrapper.Bootstrap].
//
// The workflow never panics: all errors are captured in [Result.Err].
//
// Callers must not treat the node as healthy capacity when [Result.Ready] is
// false.
func (w *Workflow) Run(ctx context.Context, nodeName string, cfg Config) Result {
	result := Result{NodeName: nodeName}

	// Phase 1: precondition / readiness checks.
	for _, check := range w.checks {
		if err := check.Check(ctx); err != nil {
			result.FailedStep = "readiness:" + check.Name()
			result.Err = fmt.Errorf("readiness check %q failed: %w", check.Name(), err)
			return result
		}
	}

	// Phase 2: LXD cluster formation.
	if err := w.bootstrapper.Bootstrap(ctx, cfg); err != nil {
		result.FailedStep = "bootstrap"
		result.Err = err
		return result
	}

	result.Ready = true
	return result
}
