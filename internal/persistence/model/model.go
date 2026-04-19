// Package model defines the domain model structs that are passed between the
// persistence layer and the rest of the application. These structs mirror the
// database schema defined in db/migrations/0001_initial_schema.up.sql.
package model

import "time"

// Node status constants define the lifecycle states of a node as stored in the
// database. The bootstrap workflow drives transitions from NodeStatusProvisioning
// to either NodeStatusOnline (success) or NodeStatusError (failure).
const (
	// NodeStatusProvisioning means the cloud server has been created but the
	// LXD bootstrap process has not yet completed. A node in this state must
	// not be used for workload scheduling.
	NodeStatusProvisioning = "provisioning"

	// NodeStatusOnline means the node has been successfully bootstrapped and
	// is a healthy cluster member available for workload scheduling.
	NodeStatusOnline = "online"

	// NodeStatusOffline means the node is not currently reachable by the
	// cluster. The node may recover without intervention (transient network
	// partition) or may require operator action.
	NodeStatusOffline = "offline"

	// NodeStatusDraining means workloads are being live-migrated off the node
	// in preparation for deprovisioning. No new instances are scheduled here.
	NodeStatusDraining = "draining"

	// NodeStatusDeprovisioning means the node is being removed from the LXD
	// cluster and its cloud server is being deprovisioned via the hyperscaler
	// provider.
	NodeStatusDeprovisioning = "deprovisioning"

	// NodeStatusError means the node encountered an unrecoverable error during
	// bootstrap or another lifecycle operation. A node in this state must never
	// be treated as available capacity; it requires operator investigation or
	// an automated retry by the reconciliation loop.
	NodeStatusError = "error"
)

// Cluster represents a single LXD cluster managed by this service.
type Cluster struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	LXDEndpoint         string         `json:"lxd_endpoint"`
	HyperscalerProvider string         `json:"hyperscaler_provider"`
	HyperscalerConfig   map[string]any `json:"hyperscaler_config"`
	ScalingConfig       map[string]any `json:"scaling_config"`
	Status              string         `json:"status"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// Node represents a physical or virtual server that is a member of a cluster.
type Node struct {
	ID                  string    `json:"id"`
	ClusterID           string    `json:"cluster_id"`
	Name                string    `json:"name"`
	LXDMemberName       string    `json:"lxd_member_name"`
	HyperscalerServerID string    `json:"hyperscaler_server_id,omitempty"`
	CPUCores            int       `json:"cpu_cores"`
	MemoryBytes         int64     `json:"memory_bytes"`
	DiskBytes           int64     `json:"disk_bytes"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Instance represents a container or VM managed within a cluster.
type Instance struct {
	ID           string         `json:"id"`
	ClusterID    string         `json:"cluster_id"`
	NodeID       string         `json:"node_id,omitempty"`
	Name         string         `json:"name"`
	InstanceType string         `json:"instance_type"`
	Status       string         `json:"status"`
	CPULimit     int            `json:"cpu_limit"`
	MemoryLimit  int64          `json:"memory_limit"`
	DiskLimit    int64          `json:"disk_limit"`
	Config       map[string]any `json:"config"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
