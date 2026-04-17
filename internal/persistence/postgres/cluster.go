package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// compile-time interface checks.
var _ persistence.ClusterRepository = (*ClusterRepo)(nil)

// ClusterRepo is the PostgreSQL implementation of persistence.ClusterRepository.
type ClusterRepo struct {
	q Querier
}

// NewClusterRepo creates a ClusterRepo that executes queries against q, which
// may be a *pgxpool.Pool or a pgx.Tx for transactional callers.
func NewClusterRepo(q Querier) *ClusterRepo {
	return &ClusterRepo{q: q}
}

// ListClusters returns all registered clusters ordered by name.
func (r *ClusterRepo) ListClusters(ctx context.Context) ([]*model.Cluster, error) {
	const q = `
SELECT id::text, name, lxd_endpoint, hyperscaler_provider,
       hyperscaler_config, scaling_config, status, created_at, updated_at
FROM clusters
ORDER BY name`

	rows, err := r.q.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.ListClusters: %w", mapErr(err))
	}
	defer rows.Close()

	var clusters []*model.Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, fmt.Errorf("ClusterRepo.ListClusters scan: %w", err)
		}
		clusters = append(clusters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ClusterRepo.ListClusters iterate: %w", mapErr(err))
	}
	return clusters, nil
}

// GetCluster returns the cluster with the given UUID string. Returns
// persistence.ErrNotFound when no matching row exists.
func (r *ClusterRepo) GetCluster(ctx context.Context, id string) (*model.Cluster, error) {
	const q = `
SELECT id::text, name, lxd_endpoint, hyperscaler_provider,
       hyperscaler_config, scaling_config, status, created_at, updated_at
FROM clusters
WHERE id = $1`

	row := r.q.QueryRow(ctx, q, id)
	c, err := scanCluster(row)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.GetCluster: %w", mapErr(err))
	}
	return c, nil
}

// CreateCluster inserts a new cluster record. The database assigns the UUID
// and timestamps; the returned struct contains the values produced by the DB.
func (r *ClusterRepo) CreateCluster(ctx context.Context, c *model.Cluster) (*model.Cluster, error) {
	hyperscalerConfigJSON, err := json.Marshal(c.HyperscalerConfig)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.CreateCluster marshal hyperscaler_config: %w", err)
	}
	scalingConfigJSON, err := json.Marshal(c.ScalingConfig)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.CreateCluster marshal scaling_config: %w", err)
	}

	const q = `
INSERT INTO clusters (name, lxd_endpoint, hyperscaler_provider, hyperscaler_config, scaling_config, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id::text, name, lxd_endpoint, hyperscaler_provider,
          hyperscaler_config, scaling_config, status, created_at, updated_at`

	row := r.q.QueryRow(ctx, q,
		c.Name,
		c.LXDEndpoint,
		c.HyperscalerProvider,
		hyperscalerConfigJSON,
		scalingConfigJSON,
		c.Status,
	)
	created, err := scanCluster(row)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.CreateCluster: %w", mapErr(err))
	}
	return created, nil
}

// UpdateCluster applies a full-field update to an existing cluster row. It
// returns persistence.ErrNotFound when no row with the given ID exists.
func (r *ClusterRepo) UpdateCluster(ctx context.Context, c *model.Cluster) (*model.Cluster, error) {
	hyperscalerConfigJSON, err := json.Marshal(c.HyperscalerConfig)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.UpdateCluster marshal hyperscaler_config: %w", err)
	}
	scalingConfigJSON, err := json.Marshal(c.ScalingConfig)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.UpdateCluster marshal scaling_config: %w", err)
	}

	const q = `
UPDATE clusters
SET name = $2, lxd_endpoint = $3, hyperscaler_provider = $4,
    hyperscaler_config = $5, scaling_config = $6, status = $7
WHERE id = $1
RETURNING id::text, name, lxd_endpoint, hyperscaler_provider,
          hyperscaler_config, scaling_config, status, created_at, updated_at`

	row := r.q.QueryRow(ctx, q,
		c.ID,
		c.Name,
		c.LXDEndpoint,
		c.HyperscalerProvider,
		hyperscalerConfigJSON,
		scalingConfigJSON,
		c.Status,
	)
	updated, err := scanCluster(row)
	if err != nil {
		return nil, fmt.Errorf("ClusterRepo.UpdateCluster: %w", mapErr(err))
	}
	return updated, nil
}

// DeleteCluster removes the cluster with the given UUID string. Returns
// persistence.ErrNotFound when no matching row exists.
func (r *ClusterRepo) DeleteCluster(ctx context.Context, id string) error {
	const q = `DELETE FROM clusters WHERE id = $1`
	tag, err := r.q.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("ClusterRepo.DeleteCluster: %w", mapErr(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ClusterRepo.DeleteCluster: %w", persistence.ErrNotFound)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Scan helpers
// ──────────────────────────────────────────────────────────────────────────────

func scanCluster(s interface{ Scan(...any) error }) (*model.Cluster, error) {
	var (
		c                    model.Cluster
		hyperscalerConfigRaw []byte
		scalingConfigRaw     []byte
		createdAt            time.Time
		updatedAt            time.Time
	)
	if err := s.Scan(
		&c.ID,
		&c.Name,
		&c.LXDEndpoint,
		&c.HyperscalerProvider,
		&hyperscalerConfigRaw,
		&scalingConfigRaw,
		&c.Status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(hyperscalerConfigRaw, &c.HyperscalerConfig); err != nil {
		return nil, fmt.Errorf("unmarshal hyperscaler_config: %w", err)
	}
	if err := json.Unmarshal(scalingConfigRaw, &c.ScalingConfig); err != nil {
		return nil, fmt.Errorf("unmarshal scaling_config: %w", err)
	}

	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return &c, nil
}
