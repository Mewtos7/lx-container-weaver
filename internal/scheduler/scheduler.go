// Package scheduler implements the bin-packing placement strategy defined in
// ADR-006. The scheduling algorithm is encapsulated behind a [Scheduler]
// interface so that alternative strategies can be introduced later without
// changing the reconciliation loop.
//
// # Bin-packing strategy
//
// [BinPacking.Schedule] selects the online node with the highest current
// utilisation that still has sufficient headroom for the requested CPU, memory,
// and disk. This minimises fragmentation across the cluster and delays
// scale-out, which aligns with the project's cost-efficiency goals.
//
// The selection is deterministic for a given input state: among equally-scored
// candidates the node with the lexicographically smallest ID is preferred, so
// repeated calls with identical arguments always return the same node.
package scheduler

import (
	"errors"
	"sort"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// ErrNoCapacity is returned by [Scheduler.Schedule] when no eligible node can
// accommodate the requested workload. Callers should treat this as a signal
// that a scale-out event may be required (ADR-006).
var ErrNoCapacity = errors.New("scheduler: no eligible node; scale-out may be required")

// Scheduler is the interface for placement strategies. Implementations must
// be safe for concurrent use from multiple goroutines.
type Scheduler interface {
	// Schedule selects a node from nodes for a workload described by req,
	// taking into account the resources already committed to instances.
	// It returns [ErrNoCapacity] when no node can accommodate the request.
	Schedule(nodes []*model.Node, instances []*model.Instance, req Request) (Result, error)
}

// Request describes the resource requirements of a workload to be placed.
type Request struct {
	// CPULimit is the number of logical CPU cores requested.
	CPULimit int

	// MemoryLimit is the amount of memory requested, in bytes.
	MemoryLimit int64

	// DiskLimit is the amount of disk space requested, in bytes.
	DiskLimit int64
}

// Result holds the outcome of a scheduling decision.
type Result struct {
	// Node is the selected node. It is nil when ScaleOutRequired is true.
	Node *model.Node

	// ScaleOutRequired is true when no existing node has sufficient headroom
	// to accommodate the workload. The caller should trigger a scale-out event.
	ScaleOutRequired bool
}

// BinPacking implements the bin-packing placement strategy from ADR-006:
// select the online node with the highest current utilisation that still has
// sufficient headroom to accommodate the workload's requested resources. This
// minimises fragmentation and delays scale-out.
//
// Ties between equally-scored candidates are broken by node ID in
// lexicographic ascending order, which guarantees deterministic output for
// identical input state across repeated calls.
type BinPacking struct{}

// compile-time interface check.
var _ Scheduler = (*BinPacking)(nil)

// New returns a new [BinPacking] scheduler.
func New() *BinPacking {
	return &BinPacking{}
}

// Schedule selects the best-fit node using the bin-packing strategy.
//
// The algorithm proceeds in four steps:
//
//  1. Compute the CPU, memory, and disk already committed to instances on each
//     node from the instances slice.
//  2. For each online node with non-zero capacity, compute the remaining
//     headroom. Discard nodes whose headroom is insufficient for req.
//  3. Among eligible nodes, compute a utilisation score as the average of the
//     three per-dimension utilisation ratios (CPU, memory, disk). The node with
//     the highest score (most packed) is selected.
//  4. When no eligible node exists, return [ErrNoCapacity] with
//     [Result.ScaleOutRequired] set to true.
func (b *BinPacking) Schedule(nodes []*model.Node, instances []*model.Instance, req Request) (Result, error) {
	// ── Step 1: compute per-node allocation ─────────────────────────────────
	//
	// Only instances with a non-empty NodeID contribute to the allocation.
	// Instances without a NodeID are awaiting placement and carry no committed
	// resources on any node yet.

	cpuAlloc := make(map[string]int64, len(nodes))
	memAlloc := make(map[string]int64, len(nodes))
	diskAlloc := make(map[string]int64, len(nodes))

	for _, inst := range instances {
		if inst.NodeID == "" {
			continue
		}
		cpuAlloc[inst.NodeID] += int64(inst.CPULimit)
		memAlloc[inst.NodeID] += inst.MemoryLimit
		diskAlloc[inst.NodeID] += inst.DiskLimit
	}

	// ── Step 2 & 3: filter and score eligible nodes ──────────────────────────

	type candidate struct {
		node  *model.Node
		score float64
	}

	eligible := make([]candidate, 0, len(nodes))

	for _, n := range nodes {
		if n.Status != model.NodeStatusOnline {
			continue
		}
		// Skip nodes with zero capacity to avoid division by zero in the
		// score calculation and because such nodes cannot host any workload.
		if n.CPUCores <= 0 || n.MemoryBytes <= 0 || n.DiskBytes <= 0 {
			continue
		}

		cpuFree := int64(n.CPUCores) - cpuAlloc[n.ID]
		memFree := n.MemoryBytes - memAlloc[n.ID]
		diskFree := n.DiskBytes - diskAlloc[n.ID]

		// Discard nodes that cannot satisfy the request.
		if cpuFree < int64(req.CPULimit) || memFree < req.MemoryLimit || diskFree < req.DiskLimit {
			continue
		}

		// Compute the average utilisation score across all three dimensions.
		// A higher score means the node is more tightly packed, which is
		// preferred by the bin-packing strategy.
		cpuUtil := float64(cpuAlloc[n.ID]) / float64(n.CPUCores)
		memUtil := float64(memAlloc[n.ID]) / float64(n.MemoryBytes)
		diskUtil := float64(diskAlloc[n.ID]) / float64(n.DiskBytes)
		score := (cpuUtil + memUtil + diskUtil) / 3.0

		eligible = append(eligible, candidate{node: n, score: score})
	}

	// ── Step 4: no eligible node ─────────────────────────────────────────────

	if len(eligible) == 0 {
		return Result{ScaleOutRequired: true}, ErrNoCapacity
	}

	// Sort by score descending; break ties by node ID ascending for determinism.
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].score != eligible[j].score {
			return eligible[i].score > eligible[j].score
		}
		return eligible[i].node.ID < eligible[j].node.ID
	})

	return Result{Node: eligible[0].node}, nil
}
