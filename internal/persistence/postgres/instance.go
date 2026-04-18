package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// compile-time interface check.
var _ persistence.InstanceRepository = (*InstanceRepo)(nil)

// InstanceRepo is the PostgreSQL implementation of persistence.InstanceRepository.
type InstanceRepo struct {
	q Querier
}

// NewInstanceRepo creates an InstanceRepo that executes queries against q,
// which may be a *pgxpool.Pool or a pgx.Tx for transactional callers.
func NewInstanceRepo(q Querier) *InstanceRepo {
	return &InstanceRepo{q: q}
}

// ListInstances returns all instances belonging to the given cluster ordered by
// name.
func (r *InstanceRepo) ListInstances(ctx context.Context, clusterID string) ([]*model.Instance, error) {
	const q = `
SELECT id::text, cluster_id::text, COALESCE(node_id::text, ''), name,
       instance_type, status, cpu_limit, memory_limit, disk_limit,
       config, created_at, updated_at
FROM instances
WHERE cluster_id = $1
ORDER BY name`

	rows, err := r.q.Query(ctx, q, clusterID)
	if err != nil {
		return nil, fmt.Errorf("InstanceRepo.ListInstances: %w", mapErr(err))
	}
	defer rows.Close()

	var instances []*model.Instance
	for rows.Next() {
		i, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("InstanceRepo.ListInstances scan: %w", err)
		}
		instances = append(instances, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("InstanceRepo.ListInstances iterate: %w", mapErr(err))
	}
	return instances, nil
}

// GetInstance returns the instance with the given UUID string. Returns
// persistence.ErrNotFound when no matching row exists.
func (r *InstanceRepo) GetInstance(ctx context.Context, id string) (*model.Instance, error) {
	const q = `
SELECT id::text, cluster_id::text, COALESCE(node_id::text, ''), name,
       instance_type, status, cpu_limit, memory_limit, disk_limit,
       config, created_at, updated_at
FROM instances
WHERE id = $1`

	row := r.q.QueryRow(ctx, q, id)
	i, err := scanInstance(row)
	if err != nil {
		return nil, fmt.Errorf("InstanceRepo.GetInstance: %w", mapErr(err))
	}
	return i, nil
}

// CreateInstance inserts a new instance record. The database assigns the UUID
// and timestamps; the returned struct contains the values produced by the DB.
func (r *InstanceRepo) CreateInstance(ctx context.Context, i *model.Instance) (*model.Instance, error) {
	configJSON, err := json.Marshal(i.Config)
	if err != nil {
		return nil, fmt.Errorf("InstanceRepo.CreateInstance marshal config: %w", err)
	}

	const q = `
INSERT INTO instances (cluster_id, node_id, name, instance_type, status,
                       cpu_limit, memory_limit, disk_limit, config)
VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, $8, $9)
RETURNING id::text, cluster_id::text, COALESCE(node_id::text, ''), name,
          instance_type, status, cpu_limit, memory_limit, disk_limit,
          config, created_at, updated_at`

	row := r.q.QueryRow(ctx, q,
		i.ClusterID,
		i.NodeID,
		i.Name,
		i.InstanceType,
		i.Status,
		i.CPULimit,
		i.MemoryLimit,
		i.DiskLimit,
		configJSON,
	)
	created, err := scanInstance(row)
	if err != nil {
		return nil, fmt.Errorf("InstanceRepo.CreateInstance: %w", mapErr(err))
	}
	return created, nil
}

// UpdateInstance applies a full-field update to an existing instance row. It
// returns persistence.ErrNotFound when no row with the given ID exists.
func (r *InstanceRepo) UpdateInstance(ctx context.Context, i *model.Instance) (*model.Instance, error) {
	configJSON, err := json.Marshal(i.Config)
	if err != nil {
		return nil, fmt.Errorf("InstanceRepo.UpdateInstance marshal config: %w", err)
	}

	const q = `
UPDATE instances
SET cluster_id = $2, node_id = NULLIF($3, '')::uuid, name = $4,
    instance_type = $5, status = $6,
    cpu_limit = $7, memory_limit = $8, disk_limit = $9, config = $10
WHERE id = $1
RETURNING id::text, cluster_id::text, COALESCE(node_id::text, ''), name,
          instance_type, status, cpu_limit, memory_limit, disk_limit,
          config, created_at, updated_at`

	row := r.q.QueryRow(ctx, q,
		i.ID,
		i.ClusterID,
		i.NodeID,
		i.Name,
		i.InstanceType,
		i.Status,
		i.CPULimit,
		i.MemoryLimit,
		i.DiskLimit,
		configJSON,
	)
	updated, err := scanInstance(row)
	if err != nil {
		return nil, fmt.Errorf("InstanceRepo.UpdateInstance: %w", mapErr(err))
	}
	return updated, nil
}

// DeleteInstance removes the instance with the given UUID string. Returns
// persistence.ErrNotFound when no matching row exists.
func (r *InstanceRepo) DeleteInstance(ctx context.Context, id string) error {
	const q = `DELETE FROM instances WHERE id = $1`
	tag, err := r.q.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("InstanceRepo.DeleteInstance: %w", mapErr(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("InstanceRepo.DeleteInstance: %w", persistence.ErrNotFound)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Scan helpers
// ──────────────────────────────────────────────────────────────────────────────

func scanInstance(s interface{ Scan(...any) error }) (*model.Instance, error) {
	var (
		i         model.Instance
		configRaw []byte
		createdAt time.Time
		updatedAt time.Time
	)
	if err := s.Scan(
		&i.ID,
		&i.ClusterID,
		&i.NodeID,
		&i.Name,
		&i.InstanceType,
		&i.Status,
		&i.CPULimit,
		&i.MemoryLimit,
		&i.DiskLimit,
		&configRaw,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(configRaw, &i.Config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	i.CreatedAt = createdAt
	i.UpdatedAt = updatedAt
	return &i, nil
}
