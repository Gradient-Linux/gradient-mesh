package mesh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Broadcaster publishes the local node on the mesh network.
type Broadcaster interface {
	Broadcast(ctx context.Context, cfg MeshConfig, self NodeInfo) error
}

// Discoverer listens for mesh peers and keeps the peer list updated.
type Discoverer interface {
	Discover(ctx context.Context, cfg MeshConfig, peers *PeerList) error
}

// SelfInferrer builds a NodeInfo snapshot for the current machine.
type SelfInferrer func(workspaceRoot string) (NodeInfo, error)

// Daemon coordinates self snapshots, discovery, broadcast, and socket serving.
type Daemon struct {
	Broadcaster Broadcaster
	Discoverer  Discoverer
	SelfInfo    SelfInferrer
	SocketPath  string
	Tick        time.Duration
}

// Run starts the mesh daemon using the default daemon wiring.
func Run(ctx context.Context, cfg MeshConfig) error {
	return NewDaemon().Run(ctx, cfg)
}

// NewDaemon returns the default daemon wiring.
func NewDaemon() *Daemon {
	return &Daemon{
		Broadcaster: ZeroconfMesh{},
		Discoverer:  ZeroconfMesh{},
		SelfInfo:    SelfInfo,
		SocketPath:  DefaultSocketPath,
		Tick:        30 * time.Second,
	}
}

// Run starts the mesh daemon shell and blocks until the context is cancelled.
func (d *Daemon) Run(ctx context.Context, cfg MeshConfig) error {
	if d == nil {
		d = NewDaemon()
	}
	cfg = normalizeConfig(cfg)

	peers := &PeerList{}
	self, err := d.SelfInfo(cfg.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("build self snapshot: %w", err)
	}
	self.Visibility = cfg.Visibility
	self.LastSeen = time.Now().UTC()
	state := &nodeState{self: self}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 3)
	var wg sync.WaitGroup

	start := func(fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
				default:
				}
				cancel()
			}
		}()
	}

	start(func(runCtx context.Context) error {
		return d.Broadcaster.Broadcast(runCtx, cfg, self)
	})
	start(func(runCtx context.Context) error {
		return d.Discoverer.Discover(runCtx, cfg, peers)
	})
	start(func(runCtx context.Context) error {
		return ServeSocket(runCtx, socketPathOrDefault(d.SocketPath), d.snapshotProvider(state, peers))
	})
	start(func(runCtx context.Context) error {
		return d.refreshLoop(runCtx, cfg.WorkspaceRoot, state)
	})

	select {
	case <-ctx.Done():
		cancel()
	case err := <-errCh:
		cancel()
		if err != nil {
			wg.Wait()
			return err
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}

	return ctx.Err()
}

// SelfInfo builds a NodeInfo snapshot from the local machine state.
func SelfInfo(workspaceRoot string) (NodeInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return NodeInfo{}, fmt.Errorf("hostname: %w", err)
	}

	machineID, _ := readMachineID()
	suites, _ := readInstalledSuites(filepath.Join(normalizeWorkspaceRoot(workspaceRoot), "config", "state.json"))

	return NodeInfo{
		Hostname:        hostname,
		MachineID:       machineID,
		GradientVersion: "dev",
		Visibility:      VisibilityPublic,
		InstalledSuites: suites,
		ResolverRunning: false,
		LastSeen:        time.Now().UTC(),
		Address:         "",
	}, nil
}

func (d *Daemon) refreshLoop(ctx context.Context, workspaceRoot string, state *nodeState) error {
	ticker := time.NewTicker(d.Tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			next, err := d.SelfInfo(workspaceRoot)
			if err != nil {
				continue
			}
			current := state.get()
			next.Visibility = current.Visibility
			next.LastSeen = time.Now().UTC()
			state.set(next)
		}
	}
}

func (d *Daemon) snapshotProvider(self *nodeState, peers *PeerList) SnapshotProvider {
	return snapshotFunc{
		self: func() NodeInfo {
			if self == nil {
				return NodeInfo{}
			}
			return self.get()
		},
		peers: func() []NodeInfo {
			if peers == nil {
				return []NodeInfo{}
			}
			return peers.List()
		},
	}
}

type snapshotFunc struct {
	self  func() NodeInfo
	peers func() []NodeInfo
}

type nodeState struct {
	mu   sync.RWMutex
	self NodeInfo
}

func (s *nodeState) get() NodeInfo {
	if s == nil {
		return NodeInfo{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.self
}

func (s *nodeState) set(self NodeInfo) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.self = self
}

func (s snapshotFunc) SelfSnapshot() NodeInfo {
	if s.self == nil {
		return NodeInfo{}
	}
	return s.self()
}

func (s snapshotFunc) PeerSnapshot() []NodeInfo {
	if s.peers == nil {
		return []NodeInfo{}
	}
	return s.peers()
}

func normalizeConfig(cfg MeshConfig) MeshConfig {
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = DefaultWorkspaceRoot()
	}
	if cfg.SocketPath == "" {
		cfg.SocketPath = DefaultSocketPath
	}
	if cfg.Visibility == "" {
		cfg.Visibility = VisibilityPublic
	}
	if cfg.ServicePort == 0 {
		cfg.ServicePort = DefaultServicePort
	}
	return cfg
}

func socketPathOrDefault(path string) string {
	if path == "" {
		return DefaultSocketPath
	}
	return path
}

func normalizeWorkspaceRoot(workspaceRoot string) string {
	if workspaceRoot == "" {
		return DefaultWorkspaceRoot()
	}
	return workspaceRoot
}
