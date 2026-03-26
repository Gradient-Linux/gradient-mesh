package mesh

import "time"

// NodeVisibility describes how a node participates in fleet discovery.
type NodeVisibility string

const (
	// VisibilityPublic makes the node discoverable by the local fleet.
	VisibilityPublic NodeVisibility = "public"
	// VisibilityPrivate keeps the node discoverable but reserved for explicit peers.
	VisibilityPrivate NodeVisibility = "private"
	// VisibilityHidden keeps the node off the broadcast surface.
	VisibilityHidden NodeVisibility = "hidden"
)

// NodeInfo describes one node in the mesh fleet.
type NodeInfo struct {
	Hostname        string         `json:"hostname"`
	MachineID       string         `json:"machine_id"`
	GradientVersion string         `json:"gradient_version"`
	Visibility      NodeVisibility `json:"visibility"`
	InstalledSuites []string       `json:"installed_suites"`
	ResolverRunning bool           `json:"resolver_running"`
	BaselineGroups  int            `json:"baseline_groups"`
	DriftedGroups   int            `json:"drifted_groups"`
	BaselineUpdated time.Time      `json:"baseline_updated_at"`
	LastSeen        time.Time      `json:"last_seen"`
	Address         string         `json:"address"`
}

// MeshConfig controls the runtime behavior of the mesh daemon.
type MeshConfig struct {
	WorkspaceRoot string
	SocketPath    string
	Visibility    NodeVisibility
	ServicePort   int
	Peers         []string
}
