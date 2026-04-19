package scheduler_test

import (
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"github.com/Mewtos7/lx-container-weaver/internal/scheduler"
)

// GiB is one gibibyte in bytes, used to express memory and disk sizes
// in a readable way throughout the test file.
const GiB = 1024 * 1024 * 1024

// newNode is a helper that returns a model.Node with the given ID, CPUCores,
// MemoryBytes, and DiskBytes. Status defaults to NodeStatusOnline.
func newNode(id string, cpuCores int, memBytes, diskBytes int64) *model.Node {
	return &model.Node{
		ID:          id,
		Status:      model.NodeStatusOnline,
		CPUCores:    cpuCores,
		MemoryBytes: memBytes,
		DiskBytes:   diskBytes,
	}
}

// newInstance is a helper that returns a model.Instance placed on nodeID
// with the given resource limits.
func newInstance(nodeID string, cpu int, mem, disk int64) *model.Instance {
	return &model.Instance{
		NodeID:      nodeID,
		CPULimit:    cpu,
		MemoryLimit: mem,
		DiskLimit:   disk,
	}
}

// ─── Scenario: multiple eligible nodes → most-packed wins ────────────────────

// TestSchedule_SelectsMostPackedNode verifies that when multiple nodes are
// eligible, the bin-packing scheduler selects the one with the highest current
// utilisation (most tightly packed), minimising fragmentation as specified in
// ADR-006.
func TestSchedule_SelectsMostPackedNode(t *testing.T) {
	s := scheduler.New()

	// node-a: 8 CPUs, 16 GiB memory, 200 GiB disk — lightly loaded
	// node-b: 8 CPUs, 16 GiB memory, 200 GiB disk — heavily loaded
	nodeA := newNode("node-a", 8, 16*GiB, 200*GiB)
	nodeB := newNode("node-b", 8, 16*GiB, 200*GiB)

	instances := []*model.Instance{
		// node-a has 2 CPUs / 4 GiB / 50 GiB allocated (~25% utilisation)
		newInstance("node-a", 2, 4*GiB, 50*GiB),
		// node-b has 6 CPUs / 12 GiB / 150 GiB allocated (~75% utilisation)
		newInstance("node-b", 6, 12*GiB, 150*GiB),
	}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}

	result, err := s.Schedule([]*model.Node{nodeA, nodeB}, instances, req)
	if err != nil {
		t.Fatalf("Schedule: unexpected error: %v", err)
	}
	if result.ScaleOutRequired {
		t.Fatal("Schedule: ScaleOutRequired should be false when a node is available")
	}
	if result.Node == nil {
		t.Fatal("Schedule: expected a non-nil Node")
	}
	if result.Node.ID != "node-b" {
		t.Errorf("Schedule: want node-b (most packed), got %q", result.Node.ID)
	}
}

// ─── Scenario: no eligible node → scale-out signal ───────────────────────────

// TestSchedule_NoEligibleNode verifies that when no node has sufficient
// headroom for the requested workload, the scheduler returns ErrNoCapacity
// and sets ScaleOutRequired to true, signalling that scale-out is needed.
func TestSchedule_NoEligibleNode(t *testing.T) {
	s := scheduler.New()

	// node-a: 4 CPUs, 8 GiB memory, 100 GiB disk — completely full
	nodeA := newNode("node-a", 4, 8*GiB, 100*GiB)
	instances := []*model.Instance{
		newInstance("node-a", 4, 8*GiB, 100*GiB),
	}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}

	result, err := s.Schedule([]*model.Node{nodeA}, instances, req)
	if !errors.Is(err, scheduler.ErrNoCapacity) {
		t.Fatalf("Schedule: want ErrNoCapacity, got %v", err)
	}
	if !result.ScaleOutRequired {
		t.Error("Schedule: ScaleOutRequired should be true when no node is eligible")
	}
	if result.Node != nil {
		t.Errorf("Schedule: Node should be nil when ScaleOutRequired, got %v", result.Node)
	}
}

// TestSchedule_EmptyNodeList verifies that an empty node list is treated the
// same as no eligible nodes.
func TestSchedule_EmptyNodeList(t *testing.T) {
	s := scheduler.New()
	req := scheduler.Request{CPULimit: 1, MemoryLimit: 512 * 1024 * 1024, DiskLimit: 10 * GiB}

	result, err := s.Schedule(nil, nil, req)
	if !errors.Is(err, scheduler.ErrNoCapacity) {
		t.Fatalf("Schedule: want ErrNoCapacity for empty node list, got %v", err)
	}
	if !result.ScaleOutRequired {
		t.Error("ScaleOutRequired should be true for empty node list")
	}
}

// ─── Scenario: determinism ───────────────────────────────────────────────────

// TestSchedule_Deterministic verifies that repeated calls with identical input
// state always produce the same placement decision, satisfying the determinism
// acceptance criterion from the issue.
func TestSchedule_Deterministic(t *testing.T) {
	s := scheduler.New()

	// Three nodes with identical load — tie-breaking by node ID must be stable.
	nodes := []*model.Node{
		newNode("node-c", 8, 16*GiB, 200*GiB),
		newNode("node-a", 8, 16*GiB, 200*GiB),
		newNode("node-b", 8, 16*GiB, 200*GiB),
	}
	// All nodes have the same allocation, so scores are equal.
	instances := []*model.Instance{
		newInstance("node-a", 2, 4*GiB, 50*GiB),
		newInstance("node-b", 2, 4*GiB, 50*GiB),
		newInstance("node-c", 2, 4*GiB, 50*GiB),
	}
	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}

	var firstID string
	for i := range 5 {
		result, err := s.Schedule(nodes, instances, req)
		if err != nil {
			t.Fatalf("pass %d: Schedule: unexpected error: %v", i, err)
		}
		if result.Node == nil {
			t.Fatalf("pass %d: Schedule: expected a non-nil Node", i)
		}
		if i == 0 {
			firstID = result.Node.ID
		} else if result.Node.ID != firstID {
			t.Errorf("pass %d: Schedule: want %q (deterministic), got %q", i, firstID, result.Node.ID)
		}
	}
}

// ─── Scenario: non-online nodes are excluded ─────────────────────────────────

// TestSchedule_ExcludesNonOnlineNodes verifies that nodes with statuses other
// than NodeStatusOnline are never selected as scheduling candidates.
func TestSchedule_ExcludesNonOnlineNodes(t *testing.T) {
	s := scheduler.New()

	statuses := []string{
		model.NodeStatusOffline,
		model.NodeStatusDraining,
		model.NodeStatusProvisioning,
		model.NodeStatusDeprovisioning,
		model.NodeStatusError,
	}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			n := newNode("node-a", 8, 16*GiB, 200*GiB)
			n.Status = status

			req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}
			result, err := s.Schedule([]*model.Node{n}, nil, req)
			if !errors.Is(err, scheduler.ErrNoCapacity) {
				t.Fatalf("status=%q: want ErrNoCapacity, got %v", status, err)
			}
			if !result.ScaleOutRequired {
				t.Errorf("status=%q: ScaleOutRequired should be true", status)
			}
		})
	}
}

// ─── Scenario: insufficient resource headroom ─────────────────────────────────

// TestSchedule_InsufficientCPU verifies that a node whose remaining CPU
// headroom is less than the requested amount is excluded.
func TestSchedule_InsufficientCPU(t *testing.T) {
	s := scheduler.New()
	n := newNode("node-a", 4, 16*GiB, 200*GiB)
	// Allocate all 4 CPUs.
	instances := []*model.Instance{newInstance("node-a", 4, 1*GiB, 10*GiB)}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}
	_, err := s.Schedule([]*model.Node{n}, instances, req)
	if !errors.Is(err, scheduler.ErrNoCapacity) {
		t.Fatalf("want ErrNoCapacity when CPU is exhausted, got %v", err)
	}
}

// TestSchedule_InsufficientMemory verifies that a node whose remaining memory
// headroom is less than the requested amount is excluded.
func TestSchedule_InsufficientMemory(t *testing.T) {
	s := scheduler.New()
	n := newNode("node-a", 8, 4*GiB, 200*GiB)
	// Allocate all 4 GiB of memory.
	instances := []*model.Instance{newInstance("node-a", 1, 4*GiB, 10*GiB)}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}
	_, err := s.Schedule([]*model.Node{n}, instances, req)
	if !errors.Is(err, scheduler.ErrNoCapacity) {
		t.Fatalf("want ErrNoCapacity when memory is exhausted, got %v", err)
	}
}

// TestSchedule_InsufficientDisk verifies that a node whose remaining disk
// headroom is less than the requested amount is excluded.
func TestSchedule_InsufficientDisk(t *testing.T) {
	s := scheduler.New()
	n := newNode("node-a", 8, 16*GiB, 20*GiB)
	// Allocate all 20 GiB of disk.
	instances := []*model.Instance{newInstance("node-a", 1, 1*GiB, 20*GiB)}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}
	_, err := s.Schedule([]*model.Node{n}, instances, req)
	if !errors.Is(err, scheduler.ErrNoCapacity) {
		t.Fatalf("want ErrNoCapacity when disk is exhausted, got %v", err)
	}
}

// ─── Scenario: zero-capacity nodes are skipped ───────────────────────────────

// TestSchedule_ZeroCapacityNodeSkipped verifies that a node with zero CPU,
// memory, or disk capacity is not considered as a candidate. Such nodes
// have not yet had their resources synced and cannot safely host workloads.
func TestSchedule_ZeroCapacityNodeSkipped(t *testing.T) {
	s := scheduler.New()
	zeroCPU := &model.Node{ID: "zero-cpu", Status: model.NodeStatusOnline, CPUCores: 0, MemoryBytes: 16 * GiB, DiskBytes: 200 * GiB}
	zeroMem := &model.Node{ID: "zero-mem", Status: model.NodeStatusOnline, CPUCores: 4, MemoryBytes: 0, DiskBytes: 200 * GiB}
	zeroDisk := &model.Node{ID: "zero-disk", Status: model.NodeStatusOnline, CPUCores: 4, MemoryBytes: 16 * GiB, DiskBytes: 0}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}

	for _, n := range []*model.Node{zeroCPU, zeroMem, zeroDisk} {
		t.Run(n.ID, func(t *testing.T) {
			_, err := s.Schedule([]*model.Node{n}, nil, req)
			if !errors.Is(err, scheduler.ErrNoCapacity) {
				t.Fatalf("node %q: want ErrNoCapacity for zero-capacity node, got %v", n.ID, err)
			}
		})
	}
}

// ─── Scenario: unplaced instances don't inflate allocation ────────────────────

// TestSchedule_UnplacedInstancesNotCounted verifies that instances with an
// empty NodeID (pending placement) do not contribute to any node's allocation.
func TestSchedule_UnplacedInstancesNotCounted(t *testing.T) {
	s := scheduler.New()
	n := newNode("node-a", 4, 8*GiB, 100*GiB)

	// A pending instance with no NodeID must not count against node-a's headroom.
	instances := []*model.Instance{
		{NodeID: "", CPULimit: 4, MemoryLimit: 8 * GiB, DiskLimit: 100 * GiB},
	}

	req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}
	result, err := s.Schedule([]*model.Node{n}, instances, req)
	if err != nil {
		t.Fatalf("Schedule: unexpected error: %v", err)
	}
	if result.Node == nil || result.Node.ID != "node-a" {
		t.Errorf("Schedule: want node-a, got %v", result.Node)
	}
}

// ─── Scenario: packed-state vs sparse-state ────────────────────────────────────

// TestSchedule_PackedVsSparseState validates both packed and non-packed cluster
// states with representative inputs, as required by the Definition of Done.
func TestSchedule_PackedVsSparseState(t *testing.T) {
	s := scheduler.New()

	// Non-packed state: two empty nodes. Scheduler should pick the tie-broken winner.
	t.Run("sparse", func(t *testing.T) {
		nodes := []*model.Node{
			newNode("node-b", 8, 16*GiB, 200*GiB),
			newNode("node-a", 8, 16*GiB, 200*GiB),
		}
		req := scheduler.Request{CPULimit: 2, MemoryLimit: 4 * GiB, DiskLimit: 50 * GiB}
		result, err := s.Schedule(nodes, nil, req)
		if err != nil {
			t.Fatalf("sparse: unexpected error: %v", err)
		}
		// With identical scores (no instances), tie-break by ID ascending → node-a.
		if result.Node.ID != "node-a" {
			t.Errorf("sparse: want node-a (tie-break by ID), got %q", result.Node.ID)
		}
	})

	// Packed state: all nodes full except one. Scheduler picks the only
	// eligible node.
	t.Run("packed", func(t *testing.T) {
		nodes := []*model.Node{
			newNode("node-a", 4, 8*GiB, 100*GiB),  // full
			newNode("node-b", 8, 16*GiB, 200*GiB), // has headroom
		}
		instances := []*model.Instance{
			newInstance("node-a", 4, 8*GiB, 100*GiB), // fills node-a completely
		}
		req := scheduler.Request{CPULimit: 2, MemoryLimit: 4 * GiB, DiskLimit: 50 * GiB}
		result, err := s.Schedule(nodes, instances, req)
		if err != nil {
			t.Fatalf("packed: unexpected error: %v", err)
		}
		if result.Node.ID != "node-b" {
			t.Errorf("packed: want node-b (only eligible), got %q", result.Node.ID)
		}
	})

	// Fully packed state: no node can accommodate the request.
	t.Run("fully_packed", func(t *testing.T) {
		nodes := []*model.Node{
			newNode("node-a", 4, 8*GiB, 100*GiB),
			newNode("node-b", 4, 8*GiB, 100*GiB),
		}
		instances := []*model.Instance{
			newInstance("node-a", 4, 8*GiB, 100*GiB),
			newInstance("node-b", 4, 8*GiB, 100*GiB),
		}
		req := scheduler.Request{CPULimit: 1, MemoryLimit: 1 * GiB, DiskLimit: 10 * GiB}
		result, err := s.Schedule(nodes, instances, req)
		if !errors.Is(err, scheduler.ErrNoCapacity) {
			t.Fatalf("fully_packed: want ErrNoCapacity, got %v", err)
		}
		if !result.ScaleOutRequired {
			t.Error("fully_packed: ScaleOutRequired should be true")
		}
	})
}
