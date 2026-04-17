// Package memory provides in-memory implementations of the persistence
// repository interfaces. These implementations are intended for use in unit
// tests and local development where a running PostgreSQL instance is not
// available.
package memory

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// compile-time interface checks.
var (
	_ persistence.ClusterRepository  = (*ClusterStore)(nil)
	_ persistence.NodeRepository     = (*NodeStore)(nil)
	_ persistence.InstanceRepository = (*InstanceStore)(nil)
)

// ──────────────────────────────────────────────────────────────────────────────
// UUID helper
// ──────────────────────────────────────────────────────────────────────────────

// newUUID generates a random UUID v4 string. It returns an error if the
// system random number generator is unavailable.
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("memory: generate UUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// ──────────────────────────────────────────────────────────────────────────────
// ClusterStore
// ──────────────────────────────────────────────────────────────────────────────

// ClusterStore is a thread-safe in-memory ClusterRepository.
type ClusterStore struct {
	mu   sync.RWMutex
	rows map[string]*model.Cluster // keyed by ID
}

// NewClusterStore returns an empty ClusterStore.
func NewClusterStore() *ClusterStore {
	return &ClusterStore{rows: make(map[string]*model.Cluster)}
}

// ListClusters returns all clusters in an arbitrary order.
func (s *ClusterStore) ListClusters(_ context.Context) ([]*model.Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*model.Cluster, 0, len(s.rows))
	for _, c := range s.rows {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

// GetCluster returns the cluster with the given ID or persistence.ErrNotFound.
func (s *ClusterStore) GetCluster(_ context.Context, id string) (*model.Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.rows[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cp := *c
	return &cp, nil
}

// CreateCluster assigns a UUID and timestamps, persists the cluster, and
// returns the stored copy. Returns persistence.ErrConflict when a cluster with
// the same name already exists.
func (s *ClusterStore) CreateCluster(_ context.Context, c *model.Cluster) (*model.Cluster, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.rows {
		if existing.Name == c.Name {
			return nil, fmt.Errorf("%w: cluster name %q already exists", persistence.ErrConflict, c.Name)
		}
	}

	now := time.Now().UTC()
	stored := *c
	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	stored.ID = id
	stored.CreatedAt = now
	stored.UpdatedAt = now
	s.rows[stored.ID] = &stored

	out := stored
	return &out, nil
}

// UpdateCluster replaces the stored cluster. Returns persistence.ErrNotFound
// when no cluster with c.ID exists, and persistence.ErrConflict when the new
// name conflicts with an existing cluster.
func (s *ClusterStore) UpdateCluster(_ context.Context, c *model.Cluster) (*model.Cluster, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.rows[c.ID]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	for _, row := range s.rows {
		if row.ID != c.ID && row.Name == c.Name {
			return nil, fmt.Errorf("%w: cluster name %q already exists", persistence.ErrConflict, c.Name)
		}
	}

	stored := *c
	stored.CreatedAt = existing.CreatedAt
	stored.UpdatedAt = time.Now().UTC()
	s.rows[stored.ID] = &stored

	out := stored
	return &out, nil
}

// DeleteCluster removes the cluster with the given ID or returns
// persistence.ErrNotFound.
func (s *ClusterStore) DeleteCluster(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rows[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// NodeStore
// ──────────────────────────────────────────────────────────────────────────────

// NodeStore is a thread-safe in-memory NodeRepository.
type NodeStore struct {
	mu   sync.RWMutex
	rows map[string]*model.Node // keyed by ID
}

// NewNodeStore returns an empty NodeStore.
func NewNodeStore() *NodeStore {
	return &NodeStore{rows: make(map[string]*model.Node)}
}

// ListNodes returns all nodes belonging to clusterID.
func (s *NodeStore) ListNodes(_ context.Context, clusterID string) ([]*model.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*model.Node
	for _, n := range s.rows {
		if n.ClusterID == clusterID {
			cp := *n
			out = append(out, &cp)
		}
	}
	return out, nil
}

// GetNode returns the node with the given ID or persistence.ErrNotFound.
func (s *NodeStore) GetNode(_ context.Context, id string) (*model.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, ok := s.rows[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cp := *n
	return &cp, nil
}

// CreateNode assigns a UUID and timestamps, persists the node, and returns the
// stored copy. Returns persistence.ErrConflict when a node with the same name
// already exists in the same cluster.
func (s *NodeStore) CreateNode(_ context.Context, n *model.Node) (*model.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.rows {
		if existing.ClusterID == n.ClusterID && existing.Name == n.Name {
			return nil, fmt.Errorf("%w: node name %q already exists in cluster %s",
				persistence.ErrConflict, n.Name, n.ClusterID)
		}
	}

	now := time.Now().UTC()
	stored := *n
	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	stored.ID = id
	stored.CreatedAt = now
	stored.UpdatedAt = now
	s.rows[stored.ID] = &stored

	out := stored
	return &out, nil
}

// UpdateNode replaces the stored node. Returns persistence.ErrNotFound when no
// node with n.ID exists, and persistence.ErrConflict when the new name
// conflicts within the same cluster.
func (s *NodeStore) UpdateNode(_ context.Context, n *model.Node) (*model.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.rows[n.ID]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	for _, row := range s.rows {
		if row.ID != n.ID && row.ClusterID == n.ClusterID && row.Name == n.Name {
			return nil, fmt.Errorf("%w: node name %q already exists in cluster %s",
				persistence.ErrConflict, n.Name, n.ClusterID)
		}
	}

	stored := *n
	stored.CreatedAt = existing.CreatedAt
	stored.UpdatedAt = time.Now().UTC()
	s.rows[stored.ID] = &stored

	out := stored
	return &out, nil
}

// DeleteNode removes the node with the given ID or returns
// persistence.ErrNotFound.
func (s *NodeStore) DeleteNode(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rows[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// InstanceStore
// ──────────────────────────────────────────────────────────────────────────────

// InstanceStore is a thread-safe in-memory InstanceRepository.
type InstanceStore struct {
	mu   sync.RWMutex
	rows map[string]*model.Instance // keyed by ID
}

// NewInstanceStore returns an empty InstanceStore.
func NewInstanceStore() *InstanceStore {
	return &InstanceStore{rows: make(map[string]*model.Instance)}
}

// ListInstances returns all instances belonging to clusterID.
func (s *InstanceStore) ListInstances(_ context.Context, clusterID string) ([]*model.Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*model.Instance
	for _, i := range s.rows {
		if i.ClusterID == clusterID {
			cp := *i
			out = append(out, &cp)
		}
	}
	return out, nil
}

// GetInstance returns the instance with the given ID or persistence.ErrNotFound.
func (s *InstanceStore) GetInstance(_ context.Context, id string) (*model.Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	i, ok := s.rows[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cp := *i
	return &cp, nil
}

// CreateInstance assigns a UUID and timestamps, persists the instance, and
// returns the stored copy. Returns persistence.ErrConflict when an instance
// with the same name already exists in the same cluster.
func (s *InstanceStore) CreateInstance(_ context.Context, i *model.Instance) (*model.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.rows {
		if existing.ClusterID == i.ClusterID && existing.Name == i.Name {
			return nil, fmt.Errorf("%w: instance name %q already exists in cluster %s",
				persistence.ErrConflict, i.Name, i.ClusterID)
		}
	}

	now := time.Now().UTC()
	stored := *i
	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	stored.ID = id
	stored.CreatedAt = now
	stored.UpdatedAt = now
	s.rows[stored.ID] = &stored

	out := stored
	return &out, nil
}

// UpdateInstance replaces the stored instance. Returns persistence.ErrNotFound
// when no instance with i.ID exists, and persistence.ErrConflict when the new
// name conflicts within the same cluster.
func (s *InstanceStore) UpdateInstance(_ context.Context, i *model.Instance) (*model.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.rows[i.ID]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	for _, row := range s.rows {
		if row.ID != i.ID && row.ClusterID == i.ClusterID && row.Name == i.Name {
			return nil, fmt.Errorf("%w: instance name %q already exists in cluster %s",
				persistence.ErrConflict, i.Name, i.ClusterID)
		}
	}

	stored := *i
	stored.CreatedAt = existing.CreatedAt
	stored.UpdatedAt = time.Now().UTC()
	s.rows[stored.ID] = &stored

	out := stored
	return &out, nil
}

// DeleteInstance removes the instance with the given ID or returns
// persistence.ErrNotFound.
func (s *InstanceStore) DeleteInstance(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rows[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}
