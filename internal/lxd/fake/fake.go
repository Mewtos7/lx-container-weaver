// Package fake provides an in-memory implementation of [lxd.Client] for use
// in unit tests of packages that depend on the LXD integration layer (e.g.
// the orchestrator, inventory-sync, and migration stories). It is not intended
// for production use.
//
// Seed the fake with test data before passing it to code under test:
//
//	f := fake.New()
//	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
//	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Location: "lxd1"})
//
//	// Pass f wherever a lxd.Client is expected.
//	orchestrator := orchestrator.New(f, ...)
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
)

// compile-time check: Fake must satisfy lxd.Client.
var _ lxd.Client = (*Fake)(nil)

// MoveRecord records a single MoveInstance call made against the fake.
type MoveRecord struct {
	InstanceName string
	TargetNode   string
}

// Fake is a thread-safe in-memory implementation of [lxd.Client] for testing.
// All write methods (AddNode, AddInstance, etc.) are safe to call concurrently
// with client methods.
type Fake struct {
	mu        sync.RWMutex
	nodes     map[string]lxd.NodeInfo      // keyed by node name
	resources map[string]lxd.NodeResources // keyed by node name
	instances map[string]lxd.InstanceInfo  // keyed by instance name

	// MoveError, if non-nil, is returned by MoveInstance for every call.
	MoveError error

	// Moves records the history of MoveInstance calls.
	Moves []MoveRecord
}

// New returns an empty Fake with no nodes or instances.
func New() *Fake {
	return &Fake{
		nodes:     make(map[string]lxd.NodeInfo),
		resources: make(map[string]lxd.NodeResources),
		instances: make(map[string]lxd.InstanceInfo),
	}
}

// AddNode seeds the fake with a cluster member. Subsequent calls with the same
// node Name overwrite the existing entry.
func (f *Fake) AddNode(n lxd.NodeInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes[n.Name] = n
}

// SetNodeResources seeds the fake with resource data for the named node.
func (f *Fake) SetNodeResources(nodeName string, r lxd.NodeResources) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources[nodeName] = r
}

// AddInstance seeds the fake with an instance. Subsequent calls with the same
// instance Name overwrite the existing entry.
func (f *Fake) AddInstance(i lxd.InstanceInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances[i.Name] = i
}

// RemoveNode removes a node from the fake, simulating it going offline.
func (f *Fake) RemoveNode(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.nodes, name)
	delete(f.resources, name)
}

// RemoveInstance removes an instance from the fake.
func (f *Fake) RemoveInstance(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.instances, name)
}

// GetClusterMembers returns all seeded nodes in an unspecified order.
func (f *Fake) GetClusterMembers(_ context.Context) ([]lxd.NodeInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]lxd.NodeInfo, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, n)
	}
	return out, nil
}

// GetClusterMember returns the seeded node with the given name.
// Returns [lxd.ErrNodeNotFound] if no node with that name was seeded.
func (f *Fake) GetClusterMember(_ context.Context, name string) (*lxd.NodeInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	n, ok := f.nodes[name]
	if !ok {
		return nil, fmt.Errorf("fake lxd: get cluster member %q: %w", name, lxd.ErrNodeNotFound)
	}
	cp := n
	return &cp, nil
}

// GetNodeResources returns the seeded resources for the named node.
// Returns [lxd.ErrNodeNotFound] if no resources were seeded for that node.
func (f *Fake) GetNodeResources(_ context.Context, nodeName string) (*lxd.NodeResources, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	r, ok := f.resources[nodeName]
	if !ok {
		return nil, fmt.Errorf("fake lxd: get node resources %q: %w", nodeName, lxd.ErrNodeNotFound)
	}
	cp := r
	return &cp, nil
}

// ListInstances returns all seeded instances in an unspecified order.
func (f *Fake) ListInstances(_ context.Context) ([]lxd.InstanceInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]lxd.InstanceInfo, 0, len(f.instances))
	for _, i := range f.instances {
		out = append(out, i)
	}
	return out, nil
}

// GetInstance returns the seeded instance with the given name.
// Returns [lxd.ErrInstanceNotFound] if no instance with that name was seeded.
func (f *Fake) GetInstance(_ context.Context, name string) (*lxd.InstanceInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	i, ok := f.instances[name]
	if !ok {
		return nil, fmt.Errorf("fake lxd: get instance %q: %w", name, lxd.ErrInstanceNotFound)
	}
	cp := i
	return &cp, nil
}

// MoveInstance records the move in [Fake.Moves]. If [Fake.MoveError] is set,
// that error is returned immediately without updating the instance's Location.
// Otherwise, the instance's Location field is updated to targetNode, simulating
// a successful migration.
//
// Returns [lxd.ErrInstanceNotFound] if the instance is not seeded.
// Returns [lxd.ErrNodeNotFound] if the target node is not seeded.
func (f *Fake) MoveInstance(_ context.Context, instanceName, targetNode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Moves = append(f.Moves, MoveRecord{InstanceName: instanceName, TargetNode: targetNode})

	if f.MoveError != nil {
		return f.MoveError
	}

	inst, ok := f.instances[instanceName]
	if !ok {
		return fmt.Errorf("fake lxd: move instance %q: %w", instanceName, lxd.ErrInstanceNotFound)
	}
	if _, ok := f.nodes[targetNode]; !ok {
		return fmt.Errorf("fake lxd: move instance %q: target %q: %w", instanceName, targetNode, lxd.ErrNodeNotFound)
	}

	inst.Location = targetNode
	f.instances[instanceName] = inst
	return nil
}
