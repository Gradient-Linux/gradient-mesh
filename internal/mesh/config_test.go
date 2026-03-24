package mesh

import (
	"path/filepath"
	"testing"
)

func TestLoadMeshConfig_DefaultsToPublic(t *testing.T) {
	root := t.TempDir()

	cfg, err := LoadMeshConfig(root)
	if err != nil {
		t.Fatalf("LoadMeshConfig() error = %v", err)
	}

	if cfg.Visibility != VisibilityPublic {
		t.Fatalf("Visibility = %q, want %q", cfg.Visibility, VisibilityPublic)
	}
	if len(cfg.Peers) != 0 {
		t.Fatalf("Peers len = %d, want 0", len(cfg.Peers))
	}
}

func TestSetVisibility_PersistsAcrossLoad(t *testing.T) {
	root := t.TempDir()

	if err := SetVisibility(root, VisibilityHidden); err != nil {
		t.Fatalf("SetVisibility() error = %v", err)
	}

	cfg, err := LoadMeshConfig(root)
	if err != nil {
		t.Fatalf("LoadMeshConfig() error = %v", err)
	}
	if cfg.Visibility != VisibilityHidden {
		t.Fatalf("Visibility = %q, want %q", cfg.Visibility, VisibilityHidden)
	}
}

func TestExplicitPeersRoundTrip(t *testing.T) {
	root := t.TempDir()

	if err := AddExplicitPeer(root, "10.0.0.10"); err != nil {
		t.Fatalf("AddExplicitPeer() error = %v", err)
	}
	if err := AddExplicitPeer(root, "10.0.0.10"); err != nil {
		t.Fatalf("AddExplicitPeer() duplicate error = %v", err)
	}
	if err := AddExplicitPeer(root, "10.0.0.11"); err != nil {
		t.Fatalf("AddExplicitPeer() error = %v", err)
	}
	if err := RemoveExplicitPeer(root, "10.0.0.10"); err != nil {
		t.Fatalf("RemoveExplicitPeer() error = %v", err)
	}

	cfg, err := LoadMeshConfig(root)
	if err != nil {
		t.Fatalf("LoadMeshConfig() error = %v", err)
	}
	if len(cfg.Peers) != 1 || cfg.Peers[0] != "10.0.0.11" {
		t.Fatalf("Peers = %#v, want [10.0.0.11]", cfg.Peers)
	}

	if got := meshConfigPath(root); got != filepath.Join(root, "config", "mesh.json") {
		t.Fatalf("meshConfigPath() = %q, want %q", got, filepath.Join(root, "config", "mesh.json"))
	}
}
