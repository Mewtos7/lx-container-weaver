// Package hetzner provides the Hetzner Cloud implementation of the
// HyperscalerProvider interface (ADR-005). All operations use the Hetzner
// Cloud REST API directly via hcloud-go — no external state files are
// required. ProvisionServer is idempotent: if a server with the given name
// already exists it is returned as-is; otherwise a new server is created.
package hetzner

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	hcloud "github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/Mewtos7/lx-container-weaver/internal/provider"
)

// Provider is the Hetzner Cloud implementation of provider.HyperscalerProvider.
type Provider struct {
	// client is the Hetzner Cloud REST API client used for all operations.
	client *hcloud.Client
}

// New creates a new Hetzner Provider authenticated with the given API token.
func New(apiToken string) (*Provider, error) {
	if apiToken == "" {
		return nil, errors.New("hetzner: api token must not be empty")
	}
	return &Provider{
		client: hcloud.NewClient(hcloud.WithToken(apiToken)),
	}, nil
}

// ProvisionServer ensures a Hetzner Cloud server matching spec exists and
// returns its provider-assigned ID.
//
// The operation is idempotent: if a server with spec.Name already exists its
// ID is returned immediately. Otherwise a new server is created. No external
// state files are used, so re-calling ProvisionServer after any failure is
// always safe.
//
// Returns [provider.ErrInvalidSpec] when required spec fields are missing.
func (p *Provider) ProvisionServer(ctx context.Context, spec provider.ServerSpec) (string, error) {
	if err := validateSpec(spec); err != nil {
		return "", fmt.Errorf("%w: %s", provider.ErrInvalidSpec, err)
	}

	// Return early if a server with this name already exists — safe to call
	// again after state loss or a partial failure.
	existing, _, err := p.client.Server.GetByName(ctx, spec.Name)
	if err != nil {
		return "", fmt.Errorf("hetzner: provision cluster %q: check existing server: %w", spec.ClusterID, err)
	}
	if existing != nil {
		return strconv.FormatInt(existing.ID, 10), nil
	}

	// Server does not exist — create it.
	result, _, err := p.client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:       spec.Name,
		ServerType: &hcloud.ServerType{Name: spec.ServerType},
		Image:      &hcloud.Image{Name: spec.Image},
		Location:   &hcloud.Location{Name: spec.Region},
		Labels: map[string]string{
			"managed-by": "lx-container-weaver",
			"cluster-id": spec.ClusterID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("hetzner: provision cluster %q: create server: %w", spec.ClusterID, err)
	}
	return strconv.FormatInt(result.Server.ID, 10), nil
}

// DeprovisionServer removes the Hetzner Cloud server identified by serverID
// using the Hetzner Cloud REST API.
//
// serverID must be the decimal string representation of the Hetzner Cloud
// server's numeric ID as returned by [ProvisionServer].
//
// Returns [provider.ErrServerNotFound] when no server with that ID exists.
func (p *Provider) DeprovisionServer(ctx context.Context, serverID string) error {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return fmt.Errorf("hetzner: deprovision server %q: invalid server ID format: %w", serverID, err)
	}
	server, _, err := p.client.Server.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("hetzner: deprovision server %q: %w", serverID, err)
	}
	if server == nil {
		return fmt.Errorf("hetzner: deprovision server %q: %w", serverID, provider.ErrServerNotFound)
	}
	if _, _, err := p.client.Server.DeleteWithResult(ctx, server); err != nil {
		return fmt.Errorf("hetzner: deprovision server %q: %w", serverID, err)
	}
	return nil
}

// GetServer returns the current state of the Hetzner Cloud server identified
// by serverID using the Hetzner Cloud REST API.
//
// serverID must be the decimal string representation of the Hetzner Cloud
// server's numeric ID as returned by [ProvisionServer].
//
// Returns [provider.ErrServerNotFound] when no server with that ID exists.
func (p *Provider) GetServer(ctx context.Context, serverID string) (*provider.ServerInfo, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("hetzner: get server %q: invalid server ID format: %w", serverID, err)
	}
	server, _, err := p.client.Server.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("hetzner: get server %q: %w", serverID, err)
	}
	if server == nil {
		return nil, fmt.Errorf("hetzner: get server %q: %w", serverID, provider.ErrServerNotFound)
	}
	return serverToInfo(server), nil
}

// ListServers returns all Hetzner Cloud servers visible to the configured API
// token. An empty slice (not nil) is returned when no servers exist.
func (p *Provider) ListServers(ctx context.Context) ([]*provider.ServerInfo, error) {
	servers, err := p.client.Server.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("hetzner: list servers: %w", err)
	}
	infos := make([]*provider.ServerInfo, 0, len(servers))
	for _, s := range servers {
		infos = append(infos, serverToInfo(s))
	}
	return infos, nil
}

// serverToInfo maps a Hetzner Cloud server to the provider-agnostic
// ServerInfo type.
func serverToInfo(s *hcloud.Server) *provider.ServerInfo {
	ipv4 := ""
	if !s.PublicNet.IPv4.IsUnspecified() {
		ipv4 = s.PublicNet.IPv4.IP.String()
	}
	return &provider.ServerInfo{
		ID:         strconv.FormatInt(s.ID, 10),
		Name:       s.Name,
		Status:     string(s.Status),
		PublicIPv4: ipv4,
	}
}

// validateSpec checks that the required ServerSpec fields are non-empty.
func validateSpec(spec provider.ServerSpec) error {
	var errs []error
	if spec.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if spec.ServerType == "" {
		errs = append(errs, errors.New("serverType is required"))
	}
	if spec.Region == "" {
		errs = append(errs, errors.New("region is required"))
	}
	if spec.Image == "" {
		errs = append(errs, errors.New("image is required"))
	}
	if spec.ClusterID == "" {
		errs = append(errs, errors.New("clusterID is required"))
	}
	return errors.Join(errs...)
}
