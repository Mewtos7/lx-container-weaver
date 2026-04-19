// Package orchestrator implements the continuous reconciliation loop described
// in ADR-006. The loop periodically evaluates each cluster's state and drives
// it toward the desired state by making scheduling, scale-out, consolidation,
// and eviction decisions.
package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/provider"
)

// NodeBootstrapRunner is the interface that the Orchestrator uses to execute
// the node bootstrap workflow after a server has been provisioned. It is
// satisfied by [bootstrap.Workflow].
//
// The interface is kept narrow (a single method) so that tests can substitute
// a lightweight stub without pulling in the full bootstrap dependency.
type NodeBootstrapRunner interface {
	// Run executes the node bootstrap workflow for the named node and returns a
	// [bootstrap.Result] that describes whether the node is ready for
	// workload scheduling. Callers must not treat the node as available when
	// [bootstrap.Result.Ready] is false.
	Run(ctx context.Context, nodeName string, cfg bootstrap.Config) bootstrap.Result
}

// Orchestrator runs the per-cluster reconciliation loop.
type Orchestrator struct {
	interval  time.Duration
	logger    *slog.Logger
	provider  provider.HyperscalerProvider
	bootstrap NodeBootstrapRunner
}

// Option is a functional option for configuring an Orchestrator at
// construction time.
type Option func(*Orchestrator)

// WithProvider wires a [provider.HyperscalerProvider] into the Orchestrator
// so that the reconciliation loop can invoke provisioning and deprovisioning
// operations via the Pulumi Automation API (ADR-005).
//
// If no provider is configured, the reconciliation loop logs a warning and
// skips provisioning steps until a provider is available.
func WithProvider(p provider.HyperscalerProvider) Option {
	return func(o *Orchestrator) { o.provider = p }
}

// WithBootstrapWorkflow wires a [NodeBootstrapRunner] into the Orchestrator.
// The runner is invoked during scale-out after a new server has been
// provisioned via the hyperscaler provider, bridging the gap between cloud
// provisioning and usable LXD cluster capacity (ADR-006).
//
// If no runner is configured, the reconciliation loop skips the bootstrap step
// and logs a warning; newly provisioned nodes will not be added to the cluster
// until a runner is available.
func WithBootstrapWorkflow(r NodeBootstrapRunner) Option {
	return func(o *Orchestrator) { o.bootstrap = r }
}

// New creates an Orchestrator that runs a reconciliation pass every interval.
func New(interval time.Duration, logger *slog.Logger, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		interval: interval,
		logger:   logger,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled, which
// triggers a clean exit after the current reconcile pass (if any) completes.
//
// Each iteration calls reconcile to evaluate cluster state. Only one
// scale-out or scale-in action is taken per iteration per cluster to prevent
// oscillation (ADR-006).
func (o *Orchestrator) Run(ctx context.Context) {
	o.logger.Info("orchestrator starting", "interval", o.interval)
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			o.logger.Info("orchestrator stopped")
			return
		case <-ticker.C:
			o.reconcile(ctx)
		}
	}
}

// reconcile performs a single evaluation pass across all managed clusters.
// This is a stub implementation; the full scheduling, scale-out, and
// consolidation logic will be added in subsequent stories.
//
// The intended scale-out sequence (ADR-006) is:
//  1. Detect that capacity is insufficient (high-water mark exceeded or no
//     node can accept a pending workload).
//  2. Call o.provider.ProvisionServer to create a new cloud server.
//  3. Call o.bootstrap.Run to bootstrap the server as an LXD cluster node.
//     The bootstrap workflow runs precondition / readiness checks before
//     attempting LXD cluster formation; a failed run sets Ready=false and
//     the node status is updated to the error state so that the next
//     reconciliation pass can retry or alert the operator.
//  4. Add the successfully bootstrapped node to the cluster inventory so that
//     the scheduler can place workloads on it.
func (o *Orchestrator) reconcile(_ context.Context) {
	o.logger.Debug("reconcile pass started")
	if o.provider == nil {
		o.logger.Debug("reconcile pass completed", "provider", "none")
		return
	}
	if o.bootstrap == nil {
		o.logger.Warn("bootstrap workflow not configured; newly provisioned nodes will not be onboarded")
	}
	// TODO: query cluster list from repository, evaluate each cluster, and
	// invoke o.provider.ProvisionServer / o.bootstrap.Run / DeprovisionServer
	// as needed.
	o.logger.Debug("reconcile pass completed")
}
