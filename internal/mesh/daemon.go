package mesh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
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
	if persisted, err := LoadMeshConfig(cfg.WorkspaceRoot); err == nil {
		cfg.Visibility = persisted.Visibility
		cfg.Peers = persisted.Peers
	}

	peers := &PeerList{}
	self, err := d.SelfInfo(cfg.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("build self snapshot: %w", err)
	}
	self.Visibility = cfg.Visibility
	self.LastSeen = time.Now().UTC()
	state := newDaemonState(cfg, self)

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
		return ServeSocket(runCtx, socketPathOrDefault(d.SocketPath), d.snapshotProvider(state, peers))
	})
	start(func(runCtx context.Context) error {
		return d.networkLoop(runCtx, state, peers)
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
	baselineGroups, driftedGroups, baselineUpdated := nodeSummary(normalizeWorkspaceRoot(workspaceRoot))

	return NodeInfo{
		Hostname:        hostname,
		MachineID:       machineID,
		GradientVersion: "dev",
		Visibility:      VisibilityPublic,
		InstalledSuites: suites,
		ResolverRunning: resolverSocketReachable(),
		BaselineGroups:  baselineGroups,
		DriftedGroups:   driftedGroups,
		BaselineUpdated: baselineUpdated,
		LastSeen:        time.Now().UTC(),
		Address:         "",
	}, nil
}

func (d *Daemon) refreshLoop(ctx context.Context, workspaceRoot string, state *daemonState) error {
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
			currentCfg := state.currentConfig()
			next.Visibility = currentCfg.Visibility
			next.LastSeen = time.Now().UTC()
			state.setSelf(next)
		}
	}
}

func (d *Daemon) networkLoop(ctx context.Context, state *daemonState, peers *PeerList) error {
	var (
		currentCfg MeshConfig
		cancel     context.CancelFunc
		wg         sync.WaitGroup
	)

	start := func(cfg MeshConfig) {
		if cancel != nil {
			cancel()
			wg.Wait()
		}
		peers.Clear()

		runCtx, runCancel := context.WithCancel(ctx)
		cancel = runCancel
		currentCfg = cfg
		self := state.currentSelf()

		launch := func(fn func(context.Context) error) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := fn(runCtx); err != nil && !errors.Is(err, context.Canceled) {
					// Leave cleanup to the outer loop. Returning here keeps the
					// network workers isolated from transient restart events.
				}
			}()
		}

		launch(func(runCtx context.Context) error {
			return d.Broadcaster.Broadcast(runCtx, cfg, self)
		})
		launch(func(runCtx context.Context) error {
			return d.Discoverer.Discover(runCtx, cfg, peers)
		})
		launch(func(runCtx context.Context) error {
			return d.servePeerAPI(runCtx, state)
		})
	}

	start(state.currentConfig())
	defer func() {
		if cancel != nil {
			cancel()
		}
		wg.Wait()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			nextCfg := state.currentConfig()
			if !sameMeshConfig(currentCfg, nextCfg) {
				start(nextCfg)
			}
		}
	}
}

func (d *Daemon) snapshotProvider(state *daemonState, peers *PeerList) SocketHandler {
	return snapshotFunc{
		self: func() NodeInfo {
			return state.currentSelf()
		},
		peers: func() []NodeInfo {
			if peers == nil {
				return []NodeInfo{}
			}
			return peers.List()
		},
		setVisibility: func(visibility NodeVisibility) (NodeInfo, error) {
			switch visibility {
			case VisibilityPublic, VisibilityPrivate, VisibilityHidden:
			default:
				return NodeInfo{}, fmt.Errorf("invalid visibility %q", visibility)
			}

			cfg := state.currentConfig()
			if err := SaveMeshConfig(cfg.WorkspaceRoot, MeshNodeConfig{
				Visibility: visibility,
				Peers:      cfg.Peers,
			}); err != nil {
				return NodeInfo{}, err
			}

			state.updateConfig(func(cfg *MeshConfig) {
				cfg.Visibility = visibility
			})
			self := state.currentSelf()
			self.Visibility = visibility
			self.LastSeen = time.Now().UTC()
			state.setSelf(self)
			return self, nil
		},
	}
}

type snapshotFunc struct {
	self          func() NodeInfo
	peers         func() []NodeInfo
	setVisibility func(NodeVisibility) (NodeInfo, error)
}

type daemonState struct {
	mu   sync.RWMutex
	self NodeInfo
	cfg  MeshConfig
}

func newDaemonState(cfg MeshConfig, self NodeInfo) *daemonState {
	return &daemonState{
		self: self,
		cfg:  cfg,
	}
}

func (s *daemonState) currentSelf() NodeInfo {
	if s == nil {
		return NodeInfo{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.self
}

func (s *daemonState) setSelf(self NodeInfo) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.self = self
}

func (s *daemonState) currentConfig() MeshConfig {
	if s == nil {
		return MeshConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *daemonState) updateConfig(update func(*MeshConfig)) {
	if s == nil || update == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.cfg)
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

func (s snapshotFunc) SetVisibility(visibility NodeVisibility) (NodeInfo, error) {
	if s.setVisibility == nil {
		return NodeInfo{}, fmt.Errorf("visibility updates are not supported")
	}
	return s.setVisibility(visibility)
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

func sameMeshConfig(a, b MeshConfig) bool {
	return a.WorkspaceRoot == b.WorkspaceRoot &&
		a.SocketPath == b.SocketPath &&
		a.Visibility == b.Visibility &&
		a.ServicePort == b.ServicePort &&
		reflect.DeepEqual(a.Peers, b.Peers)
}

func resolverSocketReachable() bool {
	conn, err := net.DialTimeout("unix", "/run/gradient/resolver.sock", 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
