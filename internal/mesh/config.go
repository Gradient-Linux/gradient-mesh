package mesh

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MeshNodeConfig persists the local node's mesh preferences.
type MeshNodeConfig struct {
	Visibility NodeVisibility `json:"visibility"`
	Peers      []string       `json:"peers"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// DefaultWorkspaceRoot returns the default Gradient workspace root.
func DefaultWorkspaceRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean("/home/gradient")
	}
	if filepath.Base(home) == "gradient" {
		return home
	}
	return filepath.Join(home, "gradient")
}

// DefaultMeshConfig returns the runtime defaults for the mesh daemon.
func DefaultMeshConfig() MeshConfig {
	return MeshConfig{
		WorkspaceRoot: DefaultWorkspaceRoot(),
		SocketPath:    DefaultSocketPath,
		Visibility:    VisibilityPublic,
		ServicePort:   DefaultServicePort,
	}
}

// DefaultMeshNodeConfig returns the persisted defaults for a fresh install.
func DefaultMeshNodeConfig() MeshNodeConfig {
	return MeshNodeConfig{
		Visibility: VisibilityPublic,
		Peers:      []string{},
		UpdatedAt:  time.Now().UTC(),
	}
}

// LoadMeshConfig loads the persisted node configuration.
func LoadMeshConfig(workspaceRoot string) (MeshNodeConfig, error) {
	cfg := DefaultMeshNodeConfig()
	path := meshConfigPath(workspaceRoot)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return MeshNodeConfig{}, fmt.Errorf("read mesh config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return MeshNodeConfig{}, fmt.Errorf("decode mesh config: %w", err)
	}
	cfg.normalize()
	return cfg, nil
}

// SaveMeshConfig writes the node configuration atomically.
func SaveMeshConfig(workspaceRoot string, cfg MeshNodeConfig) error {
	cfg.normalize()
	cfg.UpdatedAt = time.Now().UTC()

	path := meshConfigPath(workspaceRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create mesh config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mesh config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), "mesh-*.json")
	if err != nil {
		return fmt.Errorf("create mesh config temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write mesh config temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod mesh config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close mesh config temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace mesh config: %w", err)
	}

	return nil
}

// SetVisibility updates the local mesh visibility and persists it.
func SetVisibility(workspaceRoot string, visibility NodeVisibility) error {
	cfg, err := LoadMeshConfig(workspaceRoot)
	if err != nil {
		return err
	}
	cfg.Visibility = visibility
	return SaveMeshConfig(workspaceRoot, cfg)
}

// AddExplicitPeer adds an explicit peer address to the persisted config.
func AddExplicitPeer(workspaceRoot string, ip string) error {
	cfg, err := LoadMeshConfig(workspaceRoot)
	if err != nil {
		return err
	}

	cfg.Peers = uniqueStrings(append(cfg.Peers, ip))
	return SaveMeshConfig(workspaceRoot, cfg)
}

// RemoveExplicitPeer removes an explicit peer address from the persisted config.
func RemoveExplicitPeer(workspaceRoot string, ip string) error {
	cfg, err := LoadMeshConfig(workspaceRoot)
	if err != nil {
		return err
	}

	filtered := cfg.Peers[:0]
	for _, peer := range cfg.Peers {
		if peer != ip {
			filtered = append(filtered, peer)
		}
	}
	cfg.Peers = filtered
	return SaveMeshConfig(workspaceRoot, cfg)
}

func meshConfigPath(workspaceRoot string) string {
	root := workspaceRoot
	if root == "" {
		root = DefaultWorkspaceRoot()
	}
	return filepath.Join(root, "config", "mesh.json")
}

func (cfg *MeshNodeConfig) normalize() {
	if cfg.Visibility == "" {
		cfg.Visibility = VisibilityPublic
	}
	cfg.Peers = uniqueStrings(cfg.Peers)
	if cfg.UpdatedAt.IsZero() {
		cfg.UpdatedAt = time.Now().UTC()
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
