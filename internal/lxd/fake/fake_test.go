package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd/fake"
)

// Compile-time assertion: Fake must satisfy lxd.Client.
var _ lxd.Client = (*fake.Fake)(nil)

func TestFake_GetClusterMembers_Empty(t *testing.T) {
	f := fake.New()
	nodes, err := f.GetClusterMembers(context.Background())
	if err != nil {
		t.Fatalf("GetClusterMembers: unexpected error: %v", err)
	}
	if nodes == nil {
		t.Error("GetClusterMembers: want non-nil slice, got nil")
	}
	if len(nodes) != 0 {
		t.Errorf("GetClusterMembers: want 0 nodes, got %d", len(nodes))
	}
}

func TestFake_GetClusterMembers_Seeded(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Offline"})

	nodes, err := f.GetClusterMembers(context.Background())
	if err != nil {
		t.Fatalf("GetClusterMembers: unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("GetClusterMembers: want 2 nodes, got %d", len(nodes))
	}
}

func TestFake_GetClusterMember_Found(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online", Architecture: "x86_64"})

	node, err := f.GetClusterMember(context.Background(), "lxd1")
	if err != nil {
		t.Fatalf("GetClusterMember: unexpected error: %v", err)
	}
	if node.Name != "lxd1" {
		t.Errorf("Name: want %q, got %q", "lxd1", node.Name)
	}
	if node.Architecture != "x86_64" {
		t.Errorf("Architecture: want %q, got %q", "x86_64", node.Architecture)
	}
}

func TestFake_GetClusterMember_NotFound(t *testing.T) {
	f := fake.New()
	_, err := f.GetClusterMember(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetClusterMember: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrNodeNotFound) {
		t.Errorf("GetClusterMember: want errors.Is(err, ErrNodeNotFound), got %v", err)
	}
}

func TestFake_GetNodeResources(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1"})
	f.SetNodeResources("lxd1", lxd.NodeResources{
		CPU:    lxd.CPUResources{Total: 8},
		Memory: lxd.MemoryResources{Total: 8589934592, Used: 1073741824},
		Disk:   lxd.DiskResources{Total: 107374182400},
	})

	res, err := f.GetNodeResources(context.Background(), "lxd1")
	if err != nil {
		t.Fatalf("GetNodeResources: unexpected error: %v", err)
	}
	if res.CPU.Total != 8 {
		t.Errorf("CPU.Total: want 8, got %d", res.CPU.Total)
	}
	if res.Memory.Total != 8589934592 {
		t.Errorf("Memory.Total: want 8589934592, got %d", res.Memory.Total)
	}
}

func TestFake_GetNodeResources_NotFound(t *testing.T) {
	f := fake.New()
	_, err := f.GetNodeResources(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetNodeResources: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrNodeNotFound) {
		t.Errorf("GetNodeResources: want errors.Is(err, ErrNodeNotFound), got %v", err)
	}
}

func TestFake_ListInstances(t *testing.T) {
	f := fake.New()
	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Status: "Running", InstanceType: "container", Location: "lxd1"})
	f.AddInstance(lxd.InstanceInfo{Name: "db-01", Status: "Running", InstanceType: "virtual-machine", Location: "lxd2"})

	instances, err := f.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: unexpected error: %v", err)
	}
	if len(instances) != 2 {
		t.Errorf("ListInstances: want 2 instances, got %d", len(instances))
	}
}

func TestFake_GetInstance_Found(t *testing.T) {
	f := fake.New()
	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Status: "Running", Location: "lxd1"})

	inst, err := f.GetInstance(context.Background(), "web-01")
	if err != nil {
		t.Fatalf("GetInstance: unexpected error: %v", err)
	}
	if inst.Name != "web-01" {
		t.Errorf("Name: want %q, got %q", "web-01", inst.Name)
	}
}

func TestFake_GetInstance_NotFound(t *testing.T) {
	f := fake.New()
	_, err := f.GetInstance(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetInstance: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrInstanceNotFound) {
		t.Errorf("GetInstance: want errors.Is(err, ErrInstanceNotFound), got %v", err)
	}
}

func TestFake_MoveInstance_Success(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1"})
	f.AddNode(lxd.NodeInfo{Name: "lxd2"})
	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Location: "lxd1"})

	if err := f.MoveInstance(context.Background(), "web-01", "lxd2"); err != nil {
		t.Fatalf("MoveInstance: unexpected error: %v", err)
	}

	// Verify the location was updated.
	inst, _ := f.GetInstance(context.Background(), "web-01")
	if inst.Location != "lxd2" {
		t.Errorf("Location after move: want %q, got %q", "lxd2", inst.Location)
	}

	// Verify the move was recorded.
	if len(f.Moves) != 1 {
		t.Fatalf("Moves: want 1, got %d", len(f.Moves))
	}
	if f.Moves[0].InstanceName != "web-01" || f.Moves[0].TargetNode != "lxd2" {
		t.Errorf("Moves[0]: want {web-01, lxd2}, got %+v", f.Moves[0])
	}
}

func TestFake_MoveInstance_InstanceNotFound(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd2"})

	err := f.MoveInstance(context.Background(), "nonexistent", "lxd2")
	if err == nil {
		t.Fatal("MoveInstance: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrInstanceNotFound) {
		t.Errorf("MoveInstance: want errors.Is(err, ErrInstanceNotFound), got %v", err)
	}
}

func TestFake_MoveInstance_TargetNodeNotFound(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1"})
	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Location: "lxd1"})

	err := f.MoveInstance(context.Background(), "web-01", "nonexistent")
	if err == nil {
		t.Fatal("MoveInstance: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrNodeNotFound) {
		t.Errorf("MoveInstance: want errors.Is(err, ErrNodeNotFound), got %v", err)
	}
}

func TestFake_MoveInstance_CustomError(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1"})
	f.AddNode(lxd.NodeInfo{Name: "lxd2"})
	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Location: "lxd1"})
	f.MoveError = lxd.ErrMigrationFailed

	err := f.MoveInstance(context.Background(), "web-01", "lxd2")
	if err == nil {
		t.Fatal("MoveInstance: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrMigrationFailed) {
		t.Errorf("MoveInstance: want errors.Is(err, ErrMigrationFailed), got %v", err)
	}
}

func TestFake_RemoveNode(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1"})
	f.RemoveNode("lxd1")

	_, err := f.GetClusterMember(context.Background(), "lxd1")
	if !errors.Is(err, lxd.ErrNodeNotFound) {
		t.Errorf("after RemoveNode: want ErrNodeNotFound, got %v", err)
	}
}

func TestFake_RemoveInstance(t *testing.T) {
	f := fake.New()
	f.AddInstance(lxd.InstanceInfo{Name: "web-01"})
	f.RemoveInstance("web-01")

	_, err := f.GetInstance(context.Background(), "web-01")
	if !errors.Is(err, lxd.ErrInstanceNotFound) {
		t.Errorf("after RemoveInstance: want ErrInstanceNotFound, got %v", err)
	}
}
