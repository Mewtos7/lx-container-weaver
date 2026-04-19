package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/inventory"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd/fake"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// newInstanceSyncer returns an InstanceSyncer wired with the provided fake LXD
// client and fresh in-memory node and instance repositories.
func newInstanceSyncer(f *fake.Fake) (*inventory.InstanceSyncer, *memory.NodeStore, *memory.InstanceStore) {
	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()
	s := inventory.NewInstanceSyncer(f, instanceStore, nodeStore, discard)
	return s, nodeStore, instanceStore
}

// seedNodeRecord creates a node record in store and returns it.
func seedNodeRecord(t *testing.T, store *memory.NodeStore, n *model.Node) *model.Node {
	t.Helper()
	created, err := store.CreateNode(context.Background(), n)
	if err != nil {
		t.Fatalf("seedNodeRecord: %v", err)
	}
	return created
}

// seedInstance creates an instance record in store and returns it.
func seedInstance(t *testing.T, store *memory.InstanceStore, i *model.Instance) *model.Instance {
	t.Helper()
	created, err := store.CreateInstance(context.Background(), i)
	if err != nil {
		t.Fatalf("seedInstance: %v", err)
	}
	return created
}

// listInstances returns all instances for clusterID from the store.
func listInstances(t *testing.T, store *memory.InstanceStore) []*model.Instance {
	t.Helper()
	instances, err := store.ListInstances(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("listInstances: %v", err)
	}
	return instances
}

// instanceByName finds the instance with the given Name in is or returns nil.
func instanceByName(is []*model.Instance, name string) *model.Instance {
	for _, i := range is {
		if i.Name == name {
			return i
		}
	}
	return nil
}

// ─── InstanceSync: basic creation ────────────────────────────────────────────

// TestInstanceSync_CreatesNewInstances verifies that LXD instances that do not
// yet exist in the repository are created during sync.
func TestInstanceSync_CreatesNewInstances(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddInstance(lxd.InstanceInfo{
		Name:         "web-01",
		Status:       "Running",
		InstanceType: "container",
		Location:     "lxd1",
	})
	f.AddInstance(lxd.InstanceInfo{
		Name:         "db-01",
		Status:       "Stopped",
		InstanceType: "virtual-machine",
		Location:     "lxd1",
	})

	syncer, nodeStore, instanceStore := newInstanceSyncer(f)

	// Seed a node so placement can be resolved.
	node := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 2 {
		t.Fatalf("want 2 instances, got %d", len(instances))
	}

	web := instanceByName(instances, "web-01")
	if web == nil {
		t.Fatal("want instance web-01 to be created")
	}
	if web.Status != model.InstanceStatusRunning {
		t.Errorf("web-01 status: want %q, got %q", model.InstanceStatusRunning, web.Status)
	}
	if web.InstanceType != "container" {
		t.Errorf("web-01 instance_type: want %q, got %q", "container", web.InstanceType)
	}
	if web.ClusterID != clusterID {
		t.Errorf("web-01 cluster_id: want %q, got %q", clusterID, web.ClusterID)
	}
	if web.NodeID != node.ID {
		t.Errorf("web-01 node_id: want %q, got %q", node.ID, web.NodeID)
	}

	db := instanceByName(instances, "db-01")
	if db == nil {
		t.Fatal("want instance db-01 to be created")
	}
	if db.Status != model.InstanceStatusStopped {
		t.Errorf("db-01 status: want %q, got %q", model.InstanceStatusStopped, db.Status)
	}
	if db.InstanceType != "virtual-machine" {
		t.Errorf("db-01 instance_type: want %q, got %q", "virtual-machine", db.InstanceType)
	}
}

// ─── InstanceSync: status update ─────────────────────────────────────────────

// TestInstanceSync_UpdatesExistingInstanceStatus verifies that when an instance
// already exists in the repository, its status is updated to reflect LXD state.
func TestInstanceSync_UpdatesExistingInstanceStatus(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddInstance(lxd.InstanceInfo{
		Name:         "web-01",
		Status:       "Stopped",
		InstanceType: "container",
		Location:     "lxd1",
	})

	syncer, nodeStore, instanceStore := newInstanceSyncer(f)

	node := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
	})
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:    clusterID,
		NodeID:       node.ID,
		Name:         "web-01",
		InstanceType: "container",
		Status:       model.InstanceStatusRunning,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 1 {
		t.Fatalf("want 1 instance, got %d", len(instances))
	}
	if instances[0].Status != model.InstanceStatusStopped {
		t.Errorf("web-01 status: want %q, got %q", model.InstanceStatusStopped, instances[0].Status)
	}
}

// ─── InstanceSync: placement tracking ────────────────────────────────────────

// TestInstanceSync_PlacementTracking verifies that the NodeID on persisted
// instances reflects the LXD instance Location field resolved through the
// node repository.
func TestInstanceSync_PlacementTracking(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-01", Status: "Running", InstanceType: "container", Location: "lxd1",
	})
	f.AddInstance(lxd.InstanceInfo{
		Name: "db-01", Status: "Running", InstanceType: "container", Location: "lxd2",
	})

	syncer, nodeStore, instanceStore := newInstanceSyncer(f)

	node1 := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID: clusterID, Name: "lxd1", LXDMemberName: "lxd1", Status: model.NodeStatusOnline,
	})
	node2 := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID: clusterID, Name: "lxd2", LXDMemberName: "lxd2", Status: model.NodeStatusOnline,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 2 {
		t.Fatalf("want 2 instances, got %d", len(instances))
	}

	web := instanceByName(instances, "web-01")
	if web == nil {
		t.Fatal("want web-01 to exist")
	}
	if web.NodeID != node1.ID {
		t.Errorf("web-01 node_id: want %q (lxd1), got %q", node1.ID, web.NodeID)
	}

	db := instanceByName(instances, "db-01")
	if db == nil {
		t.Fatal("want db-01 to exist")
	}
	if db.NodeID != node2.ID {
		t.Errorf("db-01 node_id: want %q (lxd2), got %q", node2.ID, db.NodeID)
	}
}

// TestInstanceSync_PlacementUpdatedOnMigration verifies that when an instance
// moves to a different node between sync runs, the persisted NodeID is updated.
func TestInstanceSync_PlacementUpdatedOnMigration(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-01", Status: "Running", InstanceType: "container", Location: "lxd1",
	})

	syncer, nodeStore, instanceStore := newInstanceSyncer(f)

	node1 := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID: clusterID, Name: "lxd1", LXDMemberName: "lxd1", Status: model.NodeStatusOnline,
	})
	node2 := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID: clusterID, Name: "lxd2", LXDMemberName: "lxd2", Status: model.NodeStatusOnline,
	})

	// First sync: instance is on lxd1.
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	instances := listInstances(t, instanceStore)
	if web := instanceByName(instances, "web-01"); web == nil || web.NodeID != node1.ID {
		t.Fatalf("before migration: want web-01 on node1 (%s), got node_id=%q", node1.ID, web.NodeID)
	}

	// Simulate migration: instance moves to lxd2.
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-01", Status: "Running", InstanceType: "container", Location: "lxd2",
	})

	// Second sync: placement must be updated to lxd2.
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	instances = listInstances(t, instanceStore)
	web := instanceByName(instances, "web-01")
	if web == nil {
		t.Fatal("want web-01 to still exist after migration sync")
	}
	if web.NodeID != node2.ID {
		t.Errorf("after migration: web-01 node_id: want %q (lxd2), got %q", node2.ID, web.NodeID)
	}
}

// TestInstanceSync_UnknownLocationEmptyNodeID verifies that when an instance's
// Location does not match any persisted node, NodeID is stored as an empty
// string rather than failing the sync.
func TestInstanceSync_UnknownLocationEmptyNodeID(t *testing.T) {
	f := fake.New()
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-01", Status: "Running", InstanceType: "container",
		Location: "unknown-node",
	})

	syncer, _, instanceStore := newInstanceSyncer(f)

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 1 {
		t.Fatalf("want 1 instance, got %d", len(instances))
	}
	if instances[0].NodeID != "" {
		t.Errorf("node_id: want empty string for unknown location, got %q", instances[0].NodeID)
	}
}

// ─── InstanceSync: disappeared instances ─────────────────────────────────────

// TestInstanceSync_MarksDisappearedInstanceUnknown verifies that an instance
// present in the repository but absent from LXD is marked unknown.
func TestInstanceSync_MarksDisappearedInstanceUnknown(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	// Only web-02 is in LXD; web-01 has disappeared.
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-02", Status: "Running", InstanceType: "container", Location: "lxd1",
	})

	syncer, nodeStore, instanceStore := newInstanceSyncer(f)

	node := seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID: clusterID, Name: "lxd1", LXDMemberName: "lxd1", Status: model.NodeStatusOnline,
	})
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID: clusterID, NodeID: node.ID,
		Name: "web-01", InstanceType: "container", Status: model.InstanceStatusRunning,
	})
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID: clusterID, NodeID: node.ID,
		Name: "web-02", InstanceType: "container", Status: model.InstanceStatusStopped,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 2 {
		t.Fatalf("want 2 instances (disappeared kept, not deleted), got %d", len(instances))
	}

	web01 := instanceByName(instances, "web-01")
	if web01 == nil {
		t.Fatal("want web-01 to remain in repository (not deleted)")
	}
	if web01.Status != model.InstanceStatusUnknown {
		t.Errorf("web-01 status: want %q, got %q", model.InstanceStatusUnknown, web01.Status)
	}
	if web01.NodeID != "" {
		t.Errorf("web-01 node_id: want empty (cleared) after disappearance, got %q", web01.NodeID)
	}

	web02 := instanceByName(instances, "web-02")
	if web02 == nil {
		t.Fatal("want web-02 to remain in repository")
	}
	if web02.Status != model.InstanceStatusRunning {
		t.Errorf("web-02 status: want %q, got %q", model.InstanceStatusRunning, web02.Status)
	}
}

// ─── InstanceSync: idempotency ────────────────────────────────────────────────

// TestInstanceSync_Idempotent verifies that running Sync twice with the same
// LXD state produces the same repository state without creating duplicates.
func TestInstanceSync_Idempotent(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-01", Status: "Running", InstanceType: "container", Location: "lxd1",
	})

	syncer, nodeStore, instanceStore := newInstanceSyncer(f)
	seedNodeRecord(t, nodeStore, &model.Node{
		ClusterID: clusterID, Name: "lxd1", LXDMemberName: "lxd1", Status: model.NodeStatusOnline,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 1 {
		t.Fatalf("want 1 instance after two syncs (no duplicates), got %d", len(instances))
	}
	if instances[0].Status != model.InstanceStatusRunning {
		t.Errorf("status: want %q, got %q", model.InstanceStatusRunning, instances[0].Status)
	}
}

// ─── InstanceSync: LXD failure ───────────────────────────────────────────────

// TestInstanceSync_LXDUnreachable verifies that when ListInstances fails the
// repository is not modified and the error is propagated to the caller.
func TestInstanceSync_LXDUnreachable(t *testing.T) {
	unreachable := &errListInstancesClient{err: lxd.ErrUnreachable}
	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()
	syncer := inventory.NewInstanceSyncer(unreachable, instanceStore, nodeStore, discard)

	ctx := context.Background()
	seeded, err := instanceStore.CreateInstance(ctx, &model.Instance{
		ClusterID:    clusterID,
		Name:         "web-01",
		InstanceType: "container",
		Status:       model.InstanceStatusRunning,
	})
	if err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	syncErr := syncer.Sync(ctx, clusterID)
	if syncErr == nil {
		t.Fatal("Sync: want error when LXD is unreachable, got nil")
	}
	if !errors.Is(syncErr, lxd.ErrUnreachable) {
		t.Errorf("Sync error: want to wrap ErrUnreachable, got %v", syncErr)
	}

	// Persisted state must be unchanged.
	got, err := instanceStore.GetInstance(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("GetInstance after failed sync: %v", err)
	}
	if got.Status != model.InstanceStatusRunning {
		t.Errorf("instance status: want %q unchanged, got %q", model.InstanceStatusRunning, got.Status)
	}
}

// ─── InstanceSync: status mapping ────────────────────────────────────────────

// TestInstanceSync_StatusMapping verifies that LXD instance status strings are
// correctly translated to persistence model status constants.
func TestInstanceSync_StatusMapping(t *testing.T) {
	tests := []struct {
		lxdStatus  string
		wantStatus string
	}{
		{"Running", model.InstanceStatusRunning},
		{"Stopped", model.InstanceStatusStopped},
		{"Frozen", model.InstanceStatusFrozen},
		{"Unknown", model.InstanceStatusUnknown},
		{"", model.InstanceStatusUnknown},
		{"Starting", model.InstanceStatusUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.lxdStatus, func(t *testing.T) {
			f := fake.New()
			f.AddInstance(lxd.InstanceInfo{
				Name: "inst-01", Status: tc.lxdStatus, InstanceType: "container",
			})

			syncer, _, instanceStore := newInstanceSyncer(f)
			if err := syncer.Sync(context.Background(), clusterID); err != nil {
				t.Fatalf("Sync: %v", err)
			}

			instances := listInstances(t, instanceStore)
			if len(instances) != 1 {
				t.Fatalf("want 1 instance, got %d", len(instances))
			}
			if instances[0].Status != tc.wantStatus {
				t.Errorf("status: want %q, got %q", tc.wantStatus, instances[0].Status)
			}
		})
	}
}

// TestInstanceSync_EmptyCluster verifies that syncing against an LXD cluster
// with no instances is a no-op and does not return an error.
func TestInstanceSync_EmptyCluster(t *testing.T) {
	f := fake.New() // no instances seeded

	syncer, _, instanceStore := newInstanceSyncer(f)
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync with empty cluster: unexpected error: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 0 {
		t.Fatalf("want 0 instances for empty cluster, got %d", len(instances))
	}
}

// TestInstanceSync_ConfigStored verifies that the LXD instance config
// key-value map is stored on the persisted record.
func TestInstanceSync_ConfigStored(t *testing.T) {
	f := fake.New()
	f.AddInstance(lxd.InstanceInfo{
		Name:         "web-01",
		Status:       "Running",
		InstanceType: "container",
		Config: map[string]string{
			"limits.cpu":    "2",
			"limits.memory": "512MB",
		},
	})

	syncer, _, instanceStore := newInstanceSyncer(f)
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	instances := listInstances(t, instanceStore)
	if len(instances) != 1 {
		t.Fatalf("want 1 instance, got %d", len(instances))
	}
	cfg := instances[0].Config
	if cfg == nil {
		t.Fatal("want non-nil Config")
	}
	if v, ok := cfg["limits.cpu"]; !ok || v != "2" {
		t.Errorf("limits.cpu: want %q, got %v", "2", v)
	}
	if v, ok := cfg["limits.memory"]; !ok || v != "512MB" {
		t.Errorf("limits.memory: want %q, got %v", "512MB", v)
	}
}

// TestInstanceSync_MultipleClusterIsolation verifies that instances belonging
// to different clusters are not mixed during a sync pass.
func TestInstanceSync_MultipleClusterIsolation(t *testing.T) {
	const otherCluster = "cluster-other"

	f := fake.New()
	f.AddInstance(lxd.InstanceInfo{
		Name: "web-01", Status: "Running", InstanceType: "container",
	})

	syncer, _, instanceStore := newInstanceSyncer(f)

	// Seed an instance in a different cluster.
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:    otherCluster,
		Name:         "other-inst",
		InstanceType: "container",
		Status:       model.InstanceStatusRunning,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// The synced cluster should have exactly one instance.
	clusterInsts, err := instanceStore.ListInstances(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(clusterInsts) != 1 {
		t.Fatalf("cluster %q: want 1 instance, got %d", clusterID, len(clusterInsts))
	}

	// The other cluster's instance must not be modified.
	otherInsts, err := instanceStore.ListInstances(context.Background(), otherCluster)
	if err != nil {
		t.Fatalf("ListInstances other: %v", err)
	}
	if len(otherInsts) != 1 {
		t.Fatalf("other cluster: want 1 instance unchanged, got %d", len(otherInsts))
	}
}

// ─── stubs ────────────────────────────────────────────────────────────────────

// errListInstancesClient is a minimal lxd.Client stub that returns a
// configurable error from ListInstances and panics on any other method.
type errListInstancesClient struct {
	err error
}

func (c *errListInstancesClient) ListInstances(_ context.Context) ([]lxd.InstanceInfo, error) {
	return nil, c.err
}

func (c *errListInstancesClient) GetClusterMembers(_ context.Context) ([]lxd.NodeInfo, error) {
	panic("unexpected call to GetClusterMembers")
}

func (c *errListInstancesClient) GetClusterMember(_ context.Context, _ string) (*lxd.NodeInfo, error) {
	panic("unexpected call to GetClusterMember")
}

func (c *errListInstancesClient) GetNodeResources(_ context.Context, _ string) (*lxd.NodeResources, error) {
	panic("unexpected call to GetNodeResources")
}

func (c *errListInstancesClient) GetInstance(_ context.Context, _ string) (*lxd.InstanceInfo, error) {
	panic("unexpected call to GetInstance")
}

func (c *errListInstancesClient) MoveInstance(_ context.Context, _, _ string) error {
	panic("unexpected call to MoveInstance")
}

func (c *errListInstancesClient) GetClusterStatus(_ context.Context) (*lxd.ClusterStatus, error) {
	panic("unexpected call to GetClusterStatus")
}

func (c *errListInstancesClient) GetClusterCertificate(_ context.Context) (string, error) {
	panic("unexpected call to GetClusterCertificate")
}

func (c *errListInstancesClient) InitCluster(_ context.Context, _ lxd.ClusterInitConfig) error {
	panic("unexpected call to InitCluster")
}

func (c *errListInstancesClient) JoinCluster(_ context.Context, _ lxd.ClusterJoinConfig) error {
	panic("unexpected call to JoinCluster")
}
