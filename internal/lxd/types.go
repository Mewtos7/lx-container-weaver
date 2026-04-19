package lxd

// NodeInfo holds the state of an LXD cluster member as returned by the
// LXD REST API (/1.0/cluster/members endpoint).
type NodeInfo struct {
	// Name is the LXD cluster member name (e.g. "lxd1").
	Name string

	// URL is the HTTPS REST API endpoint for this specific cluster member
	// (e.g. "https://192.168.1.1:8443").
	URL string

	// Status is the current state of the cluster member as reported by LXD
	// (e.g. "Online", "Offline", "Evacuating").
	Status string

	// Message is a human-readable status description from LXD
	// (e.g. "Fully operational").
	Message string

	// Architecture is the CPU architecture of the node (e.g. "x86_64").
	Architecture string

	// Description is an optional operator-set description for the member.
	Description string

	// Roles is the list of cluster roles this member fulfils
	// (e.g. ["database", "database-leader"]).
	Roles []string
}

// NodeResources holds resource capacity and usage information for an LXD
// cluster member, derived from the /1.0/resources endpoint.
type NodeResources struct {
	// CPU holds CPU capacity information for the node.
	CPU CPUResources

	// Memory holds memory capacity and usage information for the node.
	Memory MemoryResources

	// Disk holds aggregated disk capacity and usage information for the node.
	Disk DiskResources
}

// CPUResources holds CPU capacity information for an LXD node.
type CPUResources struct {
	// Total is the total number of logical CPU threads available on the node.
	Total uint64
}

// MemoryResources holds memory capacity and usage information for an LXD node.
type MemoryResources struct {
	// Total is the total physical memory in bytes.
	Total uint64

	// Used is the currently consumed memory in bytes.
	Used uint64
}

// DiskResources holds aggregated disk capacity and usage information for an
// LXD node across all storage pools.
type DiskResources struct {
	// Total is the total disk capacity in bytes.
	Total uint64

	// Used is the currently consumed disk space in bytes.
	Used uint64
}

// InstanceInfo holds the state of an LXD instance (container or VM) as
// returned by the LXD REST API (/1.0/instances endpoint).
type InstanceInfo struct {
	// Name is the instance name (e.g. "web-01").
	Name string

	// Status is the current lifecycle state of the instance as reported by
	// LXD (e.g. "Running", "Stopped", "Frozen").
	Status string

	// InstanceType describes the kind of workload: "container" or
	// "virtual-machine".
	InstanceType string

	// Location is the name of the cluster member on which the instance
	// currently resides (e.g. "lxd1").
	Location string

	// Description is an optional operator-set description for the instance.
	Description string

	// Config is the raw LXD instance configuration key-value map
	// (e.g. {"limits.cpu": "2", "limits.memory": "512MB"}).
	Config map[string]string
}
