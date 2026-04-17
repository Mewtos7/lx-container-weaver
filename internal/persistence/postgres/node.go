package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// compile-time interface check.
var _ persistence.NodeRepository = (*NodeRepo)(nil)

// NodeRepo is the PostgreSQL implementation of persistence.NodeRepository.
type NodeRepo struct {
	q Querier
}

// NewNodeRepo creates a NodeRepo that executes queries against q, which may be
// a *pgxpool.Pool or a pgx.Tx for transactional callers.
func NewNodeRepo(q Querier) *NodeRepo {
	return &NodeRepo{q: q}
}

// ListNodes returns all nodes belonging to the given cluster ordered by name.
func (r *NodeRepo) ListNodes(ctx context.Context, clusterID string) ([]*model.Node, error) {
	const q = `
SELECT id::text, cluster_id::text, name, lxd_member_name,
       COALESCE(hyperscaler_server_id, ''), cpu_cores, memory_bytes, disk_bytes,
       status, created_at, updated_at
FROM nodes
WHERE cluster_id = $1
ORDER BY name`

	rows, err := r.q.Query(ctx, q, clusterID)
	if err != nil {
		return nil, fmt.Errorf("NodeRepo.ListNodes: %w", mapErr(err))
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, fmt.Errorf("NodeRepo.ListNodes scan: %w", err)
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("NodeRepo.ListNodes iterate: %w", mapErr(err))
	}
	return nodes, nil
}

// GetNode returns the node with the given UUID string. Returns
// persistence.ErrNotFound when no matching row exists.
func (r *NodeRepo) GetNode(ctx context.Context, id string) (*model.Node, error) {
	const q = `
SELECT id::text, cluster_id::text, name, lxd_member_name,
       COALESCE(hyperscaler_server_id, ''), cpu_cores, memory_bytes, disk_bytes,
       status, created_at, updated_at
FROM nodes
WHERE id = $1`

	row := r.q.QueryRow(ctx, q, id)
	n, err := scanNode(row)
	if err != nil {
		return nil, fmt.Errorf("NodeRepo.GetNode: %w", mapErr(err))
	}
	return n, nil
}

// CreateNode inserts a new node record. The database assigns the UUID and
// timestamps; the returned struct contains the values produced by the DB.
func (r *NodeRepo) CreateNode(ctx context.Context, n *model.Node) (*model.Node, error) {
	const q = `
INSERT INTO nodes (cluster_id, name, lxd_member_name, hyperscaler_server_id,
                   cpu_cores, memory_bytes, disk_bytes, status)
VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
RETURNING id::text, cluster_id::text, name, lxd_member_name,
          COALESCE(hyperscaler_server_id, ''), cpu_cores, memory_bytes, disk_bytes,
          status, created_at, updated_at`

	row := r.q.QueryRow(ctx, q,
		n.ClusterID,
		n.Name,
		n.LXDMemberName,
		n.HyperscalerServerID,
		n.CPUCores,
		n.MemoryBytes,
		n.DiskBytes,
		n.Status,
	)
	created, err := scanNode(row)
	if err != nil {
		return nil, fmt.Errorf("NodeRepo.CreateNode: %w", mapErr(err))
	}
	return created, nil
}

// UpdateNode applies a full-field update to an existing node row. It returns
// persistence.ErrNotFound when no row with the given ID exists.
func (r *NodeRepo) UpdateNode(ctx context.Context, n *model.Node) (*model.Node, error) {
	const q = `
UPDATE nodes
SET cluster_id = $2, name = $3, lxd_member_name = $4,
    hyperscaler_server_id = NULLIF($5, ''),
    cpu_cores = $6, memory_bytes = $7, disk_bytes = $8, status = $9
WHERE id = $1
RETURNING id::text, cluster_id::text, name, lxd_member_name,
          COALESCE(hyperscaler_server_id, ''), cpu_cores, memory_bytes, disk_bytes,
          status, created_at, updated_at`

	row := r.q.QueryRow(ctx, q,
		n.ID,
		n.ClusterID,
		n.Name,
		n.LXDMemberName,
		n.HyperscalerServerID,
		n.CPUCores,
		n.MemoryBytes,
		n.DiskBytes,
		n.Status,
	)
	updated, err := scanNode(row)
	if err != nil {
		return nil, fmt.Errorf("NodeRepo.UpdateNode: %w", mapErr(err))
	}
	return updated, nil
}

// DeleteNode removes the node with the given UUID string. Returns
// persistence.ErrNotFound when no matching row exists.
func (r *NodeRepo) DeleteNode(ctx context.Context, id string) error {
	const q = `DELETE FROM nodes WHERE id = $1`
	tag, err := r.q.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("NodeRepo.DeleteNode: %w", mapErr(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("NodeRepo.DeleteNode: %w", persistence.ErrNotFound)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Scan helpers
// ──────────────────────────────────────────────────────────────────────────────

func scanNode(s interface{ Scan(...any) error }) (*model.Node, error) {
	var (
		n         model.Node
		createdAt time.Time
		updatedAt time.Time
	)
	if err := s.Scan(
		&n.ID,
		&n.ClusterID,
		&n.Name,
		&n.LXDMemberName,
		&n.HyperscalerServerID,
		&n.CPUCores,
		&n.MemoryBytes,
		&n.DiskBytes,
		&n.Status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	n.CreatedAt = createdAt
	n.UpdatedAt = updatedAt
	return &n, nil
}
