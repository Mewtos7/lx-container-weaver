package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

var ctx = context.Background()

// ──────────────────────────────────────────────────────────────────────────────
// ClusterStore tests
// ──────────────────────────────────────────────────────────────────────────────

func TestClusterStore_CreateAndGet(t *testing.T) {
	store := memory.NewClusterStore()

	in := &model.Cluster{
		Name:                "prod",
		LXDEndpoint:         "https://lxd.example.com",
		HyperscalerProvider: "hetzner",
		HyperscalerConfig:   map[string]any{"token": "tok"},
		ScalingConfig:       map[string]any{"max": 5},
		Status:              "active",
	}
	created, err := store.CreateCluster(ctx, in)
	if err != nil {
		t.Fatalf("CreateCluster: unexpected error: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateCluster: expected a non-empty ID")
	}
	if created.Name != in.Name {
		t.Errorf("CreateCluster: name: want %q, got %q", in.Name, created.Name)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreateCluster: expected non-zero CreatedAt")
	}

	got, err := store.GetCluster(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetCluster: unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetCluster: ID: want %q, got %q", created.ID, got.ID)
	}
}

func TestClusterStore_GetNotFound(t *testing.T) {
	store := memory.NewClusterStore()

	_, err := store.GetCluster(ctx, "nonexistent-id")
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("GetCluster: want ErrNotFound, got %v", err)
	}
}

func TestClusterStore_CreateConflict(t *testing.T) {
	store := memory.NewClusterStore()

	c := &model.Cluster{Name: "dup", LXDEndpoint: "x", HyperscalerProvider: "hetzner",
		HyperscalerConfig: map[string]any{}, ScalingConfig: map[string]any{}, Status: "active"}

	if _, err := store.CreateCluster(ctx, c); err != nil {
		t.Fatalf("first CreateCluster: %v", err)
	}
	_, err := store.CreateCluster(ctx, c)
	if !errors.Is(err, persistence.ErrConflict) {
		t.Errorf("second CreateCluster: want ErrConflict, got %v", err)
	}
}

func TestClusterStore_List(t *testing.T) {
	store := memory.NewClusterStore()

	for _, name := range []string{"a", "b", "c"} {
		_, err := store.CreateCluster(ctx, &model.Cluster{
			Name: name, LXDEndpoint: "x", HyperscalerProvider: "hetzner",
			HyperscalerConfig: map[string]any{}, ScalingConfig: map[string]any{},
			Status: "active",
		})
		if err != nil {
			t.Fatalf("CreateCluster %q: %v", name, err)
		}
	}

	list, err := store.ListClusters(ctx)
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("ListClusters: want 3, got %d", len(list))
	}
}

func TestClusterStore_Update(t *testing.T) {
	store := memory.NewClusterStore()

	created, err := store.CreateCluster(ctx, &model.Cluster{
		Name: "original", LXDEndpoint: "x", HyperscalerProvider: "hetzner",
		HyperscalerConfig: map[string]any{}, ScalingConfig: map[string]any{},
		Status: "active",
	})
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	updated := *created
	updated.Name = "renamed"
	updated.Status = "inactive"

	got, err := store.UpdateCluster(ctx, &updated)
	if err != nil {
		t.Fatalf("UpdateCluster: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("UpdateCluster: name: want %q, got %q", "renamed", got.Name)
	}
	if got.CreatedAt != created.CreatedAt {
		t.Error("UpdateCluster: CreatedAt should not change")
	}
	if got.UpdatedAt.Before(created.UpdatedAt) {
		t.Error("UpdateCluster: UpdatedAt should be newer than or equal to previous UpdatedAt")
	}
}

func TestClusterStore_UpdateNotFound(t *testing.T) {
	store := memory.NewClusterStore()

	_, err := store.UpdateCluster(ctx, &model.Cluster{ID: "nonexistent"})
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("UpdateCluster: want ErrNotFound, got %v", err)
	}
}

func TestClusterStore_Delete(t *testing.T) {
	store := memory.NewClusterStore()

	c, _ := store.CreateCluster(ctx, &model.Cluster{
		Name: "to-delete", LXDEndpoint: "x", HyperscalerProvider: "hetzner",
		HyperscalerConfig: map[string]any{}, ScalingConfig: map[string]any{},
		Status: "active",
	})

	if err := store.DeleteCluster(ctx, c.ID); err != nil {
		t.Fatalf("DeleteCluster: %v", err)
	}
	if err := store.DeleteCluster(ctx, c.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("second DeleteCluster: want ErrNotFound, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// NodeStore tests
// ──────────────────────────────────────────────────────────────────────────────

func TestNodeStore_CreateAndGet(t *testing.T) {
	store := memory.NewNodeStore()

	in := &model.Node{
		ClusterID:     "cluster-1",
		Name:          "node-1",
		LXDMemberName: "node-1.lxd",
		CPUCores:      4,
		MemoryBytes:   8 * 1024 * 1024 * 1024,
		DiskBytes:     100 * 1024 * 1024 * 1024,
		Status:        "online",
	}
	created, err := store.CreateNode(ctx, in)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateNode: expected non-empty ID")
	}

	got, err := store.GetNode(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.ClusterID != in.ClusterID {
		t.Errorf("GetNode: ClusterID: want %q, got %q", in.ClusterID, got.ClusterID)
	}
}

func TestNodeStore_GetNotFound(t *testing.T) {
	store := memory.NewNodeStore()

	_, err := store.GetNode(ctx, "no-such-id")
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("GetNode: want ErrNotFound, got %v", err)
	}
}

func TestNodeStore_CreateConflict(t *testing.T) {
	store := memory.NewNodeStore()

	n := &model.Node{ClusterID: "cluster-1", Name: "dup", LXDMemberName: "x",
		CPUCores: 1, MemoryBytes: 1, DiskBytes: 1, Status: "online"}

	if _, err := store.CreateNode(ctx, n); err != nil {
		t.Fatalf("first CreateNode: %v", err)
	}
	_, err := store.CreateNode(ctx, n)
	if !errors.Is(err, persistence.ErrConflict) {
		t.Errorf("second CreateNode: want ErrConflict, got %v", err)
	}
}

func TestNodeStore_ListByCluster(t *testing.T) {
	store := memory.NewNodeStore()

	for _, name := range []string{"n1", "n2"} {
		_, err := store.CreateNode(ctx, &model.Node{
			ClusterID: "c1", Name: name, LXDMemberName: name,
			CPUCores: 1, MemoryBytes: 1, DiskBytes: 1, Status: "online",
		})
		if err != nil {
			t.Fatalf("CreateNode %q: %v", name, err)
		}
	}
	// Node in a different cluster.
	_, err := store.CreateNode(ctx, &model.Node{
		ClusterID: "c2", Name: "n3", LXDMemberName: "n3",
		CPUCores: 1, MemoryBytes: 1, DiskBytes: 1, Status: "online",
	})
	if err != nil {
		t.Fatalf("CreateNode n3: %v", err)
	}

	list, err := store.ListNodes(ctx, "c1")
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListNodes: want 2, got %d", len(list))
	}
}

func TestNodeStore_Update(t *testing.T) {
	store := memory.NewNodeStore()

	created, err := store.CreateNode(ctx, &model.Node{
		ClusterID: "c1", Name: "orig", LXDMemberName: "orig.lxd",
		CPUCores: 2, MemoryBytes: 4 * 1024 * 1024 * 1024, DiskBytes: 50 * 1024 * 1024 * 1024,
		Status: "online",
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	updated := *created
	updated.Status = "draining"

	got, err := store.UpdateNode(ctx, &updated)
	if err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	if got.Status != "draining" {
		t.Errorf("UpdateNode: status: want %q, got %q", "draining", got.Status)
	}
}

func TestNodeStore_DeleteNotFound(t *testing.T) {
	store := memory.NewNodeStore()

	err := store.DeleteNode(ctx, "no-such-id")
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("DeleteNode: want ErrNotFound, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// InstanceStore tests
// ──────────────────────────────────────────────────────────────────────────────

func TestInstanceStore_CreateAndGet(t *testing.T) {
	store := memory.NewInstanceStore()

	in := &model.Instance{
		ClusterID:    "cluster-1",
		Name:         "web-1",
		InstanceType: "container",
		Status:       "running",
		CPULimit:     2,
		MemoryLimit:  512 * 1024 * 1024,
		DiskLimit:    10 * 1024 * 1024 * 1024,
		Config:       map[string]any{"image": "ubuntu:22.04"},
	}
	created, err := store.CreateInstance(ctx, in)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateInstance: expected non-empty ID")
	}

	got, err := store.GetInstance(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.Name != in.Name {
		t.Errorf("GetInstance: name: want %q, got %q", in.Name, got.Name)
	}
}

func TestInstanceStore_GetNotFound(t *testing.T) {
	store := memory.NewInstanceStore()

	_, err := store.GetInstance(ctx, "ghost")
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("GetInstance: want ErrNotFound, got %v", err)
	}
}

func TestInstanceStore_CreateConflict(t *testing.T) {
	store := memory.NewInstanceStore()

	i := &model.Instance{
		ClusterID: "cluster-1", Name: "dup", InstanceType: "container",
		Status: "running", CPULimit: 1, MemoryLimit: 1, DiskLimit: 1,
		Config: map[string]any{},
	}

	if _, err := store.CreateInstance(ctx, i); err != nil {
		t.Fatalf("first CreateInstance: %v", err)
	}
	_, err := store.CreateInstance(ctx, i)
	if !errors.Is(err, persistence.ErrConflict) {
		t.Errorf("second CreateInstance: want ErrConflict, got %v", err)
	}
}

func TestInstanceStore_ListByCluster(t *testing.T) {
	store := memory.NewInstanceStore()

	for _, name := range []string{"i1", "i2", "i3"} {
		_, err := store.CreateInstance(ctx, &model.Instance{
			ClusterID: "c1", Name: name, InstanceType: "container",
			Status: "running", CPULimit: 1, MemoryLimit: 1, DiskLimit: 1,
			Config: map[string]any{},
		})
		if err != nil {
			t.Fatalf("CreateInstance %q: %v", name, err)
		}
	}
	// Instance in another cluster — must not appear in the list.
	_, err := store.CreateInstance(ctx, &model.Instance{
		ClusterID: "c2", Name: "other", InstanceType: "container",
		Status: "running", CPULimit: 1, MemoryLimit: 1, DiskLimit: 1,
		Config: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CreateInstance other: %v", err)
	}

	list, err := store.ListInstances(ctx, "c1")
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("ListInstances: want 3, got %d", len(list))
	}
}

func TestInstanceStore_Update(t *testing.T) {
	store := memory.NewInstanceStore()

	created, err := store.CreateInstance(ctx, &model.Instance{
		ClusterID: "c1", Name: "inst", InstanceType: "container",
		Status: "pending", CPULimit: 1, MemoryLimit: 512 * 1024 * 1024,
		DiskLimit: 10 * 1024 * 1024 * 1024, Config: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	updated := *created
	updated.Status = "running"
	updated.NodeID = "node-42"

	got, err := store.UpdateInstance(ctx, &updated)
	if err != nil {
		t.Fatalf("UpdateInstance: %v", err)
	}
	if got.Status != "running" {
		t.Errorf("UpdateInstance: status: want %q, got %q", "running", got.Status)
	}
	if got.NodeID != "node-42" {
		t.Errorf("UpdateInstance: node_id: want %q, got %q", "node-42", got.NodeID)
	}
}

func TestInstanceStore_Delete(t *testing.T) {
	store := memory.NewInstanceStore()

	i, _ := store.CreateInstance(ctx, &model.Instance{
		ClusterID: "c1", Name: "doomed", InstanceType: "container",
		Status: "running", CPULimit: 1, MemoryLimit: 1, DiskLimit: 1,
		Config: map[string]any{},
	})

	if err := store.DeleteInstance(ctx, i.ID); err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	if err := store.DeleteInstance(ctx, i.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("second DeleteInstance: want ErrNotFound, got %v", err)
	}
}

func TestInstanceStore_UpdateNotFound(t *testing.T) {
	store := memory.NewInstanceStore()

	_, err := store.UpdateInstance(ctx, &model.Instance{ID: "ghost"})
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Errorf("UpdateInstance: want ErrNotFound, got %v", err)
	}
}
