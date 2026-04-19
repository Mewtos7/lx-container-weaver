// Package orchestrator implements the continuous reconciliation loop described
// in ADR-006. The loop periodically evaluates each cluster's state and drives
// it toward the desired state by making scheduling, scale-out, consolidation,
// and eviction decisions.
package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	"github.com/Mewtos7/lx-container-weaver/internal/scheduler"
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

// NodeInventorySyncer synchronises the node inventory for a single cluster.
// It is satisfied by [inventory.Syncer].
//
// The interface is kept narrow so that tests can substitute a lightweight stub
// without pulling in the full LXD client dependency.
type NodeInventorySyncer interface {
	Sync(ctx context.Context, clusterID string) error
}

// InstanceInventorySyncer synchronises the instance inventory for a single
// cluster. It is satisfied by [inventory.InstanceSyncer].
type InstanceInventorySyncer interface {
	Sync(ctx context.Context, clusterID string) error
}

// Orchestrator runs the per-cluster reconciliation loop.
type Orchestrator struct {
	interval       time.Duration
	logger         *slog.Logger
	provider       provider.HyperscalerProvider
	bootstrap      NodeBootstrapRunner
	clusterRepo    persistence.ClusterRepository
	nodeRepo       persistence.NodeRepository
	instanceRepo   persistence.InstanceRepository
	nodeSyncer     NodeInventorySyncer
	instanceSyncer InstanceInventorySyncer
	scheduler      scheduler.Scheduler
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

// WithClusterRepository wires a [persistence.ClusterRepository] into the
// Orchestrator so that the reconciliation loop can enumerate all registered
// clusters on each pass.
//
// If no repository is configured the loop logs a debug message and skips the
// reconciliation step; this allows the service to start without a database
// connection during development.
func WithClusterRepository(r persistence.ClusterRepository) Option {
	return func(o *Orchestrator) { o.clusterRepo = r }
}

// WithNodeSyncer wires a [NodeInventorySyncer] into the Orchestrator. On each
// reconciliation pass the syncer is called for every registered cluster so
// that the node inventory reflects the current LXD cluster-member state.
func WithNodeSyncer(s NodeInventorySyncer) Option {
	return func(o *Orchestrator) { o.nodeSyncer = s }
}

// WithInstanceSyncer wires an [InstanceInventorySyncer] into the Orchestrator.
// On each reconciliation pass the syncer is called for every registered cluster
// so that the instance inventory reflects the current LXD instance state.
func WithInstanceSyncer(s InstanceInventorySyncer) Option {
	return func(o *Orchestrator) { o.instanceSyncer = s }
}

// WithNodeRepository wires a [persistence.NodeRepository] into the
// Orchestrator. The repository is used by the scheduling step to read the
// current node state for placement decisions.
//
// If no repository is configured, the scheduling step is skipped.
func WithNodeRepository(r persistence.NodeRepository) Option {
	return func(o *Orchestrator) { o.nodeRepo = r }
}

// WithInstanceRepository wires a [persistence.InstanceRepository] into the
// Orchestrator. The repository is used by the scheduling step to read current
// instance placements and to record new placement decisions.
//
// If no repository is configured, the scheduling step is skipped.
func WithInstanceRepository(r persistence.InstanceRepository) Option {
	return func(o *Orchestrator) { o.instanceRepo = r }
}

// WithScheduler wires a [scheduler.Scheduler] into the Orchestrator. On each
// reconciliation pass the scheduler is called to assign unplaced instances to
// nodes using the configured placement strategy (ADR-006).
//
// If no scheduler is configured, the scheduling step is skipped and instances
// with no assigned node are left unplaced until a scheduler is available.
func WithScheduler(s scheduler.Scheduler) Option {
	return func(o *Orchestrator) { o.scheduler = s }
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

// ReconcileOnce executes a single reconciliation pass synchronously. It is
// equivalent to one tick of the loop driven by [Run] and is primarily
// intended for use in tests and manual validation tooling.
func (o *Orchestrator) ReconcileOnce(ctx context.Context) {
	o.reconcile(ctx)
}

// reconcile performs a single evaluation pass across all managed clusters.
//
// It reads the current cluster list from the repository and calls
// reconcileCluster for each entry. Failures within a single cluster are
// logged and do not abort the pass for the remaining clusters, ensuring the
// loop stays stable across partial failures (ADR-006).
//
// The intended per-cluster scale-out sequence (ADR-006) is:
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
func (o *Orchestrator) reconcile(ctx context.Context) {
	o.logger.Debug("reconcile pass started")

	if o.clusterRepo == nil {
		o.logger.Debug("reconcile pass skipped: no cluster repository configured")
		return
	}

	clusters, err := o.clusterRepo.ListClusters(ctx)
	if err != nil {
		o.logger.Error("reconcile: failed to list clusters", "error", err)
		return
	}

	o.logger.Debug("reconcile: evaluating clusters", "count", len(clusters))
	for _, cluster := range clusters {
		o.reconcileCluster(ctx, cluster)
	}

	o.logger.Debug("reconcile pass completed", "clusters", len(clusters))
}

// reconcileCluster performs a single reconciliation pass for one cluster.
//
// It runs the node and instance inventory sync steps before any scheduling or
// scaling decisions so that those steps always operate on up-to-date state.
// Errors from individual sync steps are logged and do not abort the remaining
// steps for the same cluster, keeping the loop resilient to partial failures.
func (o *Orchestrator) reconcileCluster(ctx context.Context, cluster *model.Cluster) {
	log := o.logger.With("cluster_id", cluster.ID, "cluster_name", cluster.Name)
	log.Debug("reconcile cluster: started")

	if o.nodeSyncer != nil {
		if err := o.nodeSyncer.Sync(ctx, cluster.ID); err != nil {
			log.Error("reconcile cluster: node inventory sync failed", "error", err)
			// Continue — instance sync and scaling steps are independent.
		}
	}

	if o.instanceSyncer != nil {
		if err := o.instanceSyncer.Sync(ctx, cluster.ID); err != nil {
			log.Error("reconcile cluster: instance inventory sync failed", "error", err)
			// Continue — scaling steps do not depend on a successful instance sync.
		}
	}

	o.scheduleCluster(ctx, cluster, log)

	if o.provider == nil {
		log.Debug("reconcile cluster: no provider configured; skipping scaling steps")
	} else {
		if o.bootstrap == nil {
			log.Warn("reconcile cluster: bootstrap workflow not configured; newly provisioned nodes will not be onboarded")
		}
		// TODO: evaluate scaling decisions (ADR-006 high/low-water mark logic)
		// and invoke o.provider.ProvisionServer / o.bootstrap.Run /
		// o.provider.DeprovisionServer as needed.
	}

	log.Debug("reconcile cluster: completed")
}

// scheduleCluster runs the placement step for one cluster. It lists all nodes
// and instances, finds instances that have not yet been placed on a node
// (NodeID == ""), and calls the scheduler to assign each one.
//
// A successful placement is recorded by updating the instance's NodeID in the
// repository. When no eligible node is available the scheduler returns
// [scheduler.ErrNoCapacity] and the event is logged so that an operator or a
// future scale-out step can react.
//
// scheduleCluster is a no-op when the scheduler, node repository, or instance
// repository is not configured.
func (o *Orchestrator) scheduleCluster(ctx context.Context, cluster *model.Cluster, log *slog.Logger) {
	if o.scheduler == nil || o.nodeRepo == nil || o.instanceRepo == nil {
		return
	}

	nodes, err := o.nodeRepo.ListNodes(ctx, cluster.ID)
	if err != nil {
		log.Error("schedule cluster: failed to list nodes", "error", err)
		return
	}

	instances, err := o.instanceRepo.ListInstances(ctx, cluster.ID)
	if err != nil {
		log.Error("schedule cluster: failed to list instances", "error", err)
		return
	}

	for _, inst := range instances {
		if inst.NodeID != "" {
			// Instance already placed; nothing to do.
			continue
		}

		req := scheduler.Request{
			CPULimit:    inst.CPULimit,
			MemoryLimit: inst.MemoryLimit,
			DiskLimit:   inst.DiskLimit,
		}

		result, schedErr := o.scheduler.Schedule(nodes, instances, req)
		if errors.Is(schedErr, scheduler.ErrNoCapacity) {
			log.Warn("schedule cluster: no eligible node for instance; scale-out may be required",
				"instance_id", inst.ID, "instance_name", inst.Name)
			continue
		}
		if schedErr != nil {
			log.Error("schedule cluster: scheduler error", "instance_id", inst.ID, "error", schedErr)
			continue
		}

		// Record the placement decision.
		placed := *inst
		placed.NodeID = result.Node.ID
		if _, updateErr := o.instanceRepo.UpdateInstance(ctx, &placed); updateErr != nil {
			log.Error("schedule cluster: failed to record placement",
				"instance_id", inst.ID, "node_id", result.Node.ID, "error", updateErr)
			continue
		}

		log.Info("schedule cluster: placed instance",
			"instance_id", inst.ID, "instance_name", inst.Name,
			"node_id", result.Node.ID)

		// Refresh the local instances slice so subsequent scheduling decisions
		// in this pass account for the resources committed to this placement.
		for i, in := range instances {
			if in.ID == inst.ID {
				instances[i] = &placed
				break
			}
		}
	}
}
