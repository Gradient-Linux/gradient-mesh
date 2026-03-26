package mesh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

const serviceType = "_gradient-linux._tcp"
const serviceDomain = "local."

// ZeroconfMesh implements the mesh broadcaster and discoverer using zeroconf.
type ZeroconfMesh struct{}

// Broadcast advertises the local node on the LAN.
func (ZeroconfMesh) Broadcast(ctx context.Context, cfg MeshConfig, self NodeInfo) error {
	if cfg.Visibility == VisibilityHidden {
		<-ctx.Done()
		return ctx.Err()
	}

	text := []string{
		"machine_id=" + self.MachineID,
		"version=" + self.GradientVersion,
		"suites=" + strings.Join(self.InstalledSuites, ","),
		"resolver=" + fmt.Sprintf("%t", self.ResolverRunning),
		"baselines=" + strconv.Itoa(self.BaselineGroups),
		"drifted=" + strconv.Itoa(self.DriftedGroups),
		"baseline_updated=" + self.BaselineUpdated.UTC().Format(time.RFC3339),
		"visibility=" + string(cfg.Visibility),
	}

	server, err := zeroconf.Register(
		self.Hostname,
		serviceType,
		serviceDomain,
		cfg.ServicePort,
		text,
		nil,
	)
	if err != nil {
		return fmt.Errorf("register mDNS service: %w", err)
	}
	defer server.Shutdown()

	<-ctx.Done()
	return ctx.Err()
}

// Discover listens for local Gradient Linux peers and updates the peer list.
func (ZeroconfMesh) Discover(ctx context.Context, cfg MeshConfig, peers *PeerList) error {
	if cfg.Visibility == VisibilityHidden {
		<-ctx.Done()
		return ctx.Err()
	}
	selfMachineID, _ := readMachineID()

	go purgeExpiredPeers(ctx, peers, 20*time.Second)

	for {
		if err := browseOnce(ctx, cfg, peers, selfMachineID); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func browseOnce(ctx context.Context, cfg MeshConfig, peers *PeerList, selfMachineID string) error {
	cycleCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("create zeroconf resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-cycleCtx.Done():
				return
			case entry := <-entries:
				if entry == nil {
					continue
				}
				node := nodeFromServiceEntry(entry, cfg)
				if node.MachineID == "" || node.MachineID == selfMachineID {
					continue
				}
				if !allowDiscoveredNode(cfg, node) {
					peers.Remove(node.MachineID)
					continue
				}
				peers.Add(node)
				go syncPeerBaselines(cycleCtx, cfg.WorkspaceRoot, node)
			}
		}
	}()

	if err := resolver.Browse(cycleCtx, serviceType, serviceDomain, entries); err != nil {
		<-done
		return fmt.Errorf("browse mDNS services: %w", err)
	}
	<-cycleCtx.Done()
	<-done
	if errors.Is(cycleCtx.Err(), context.DeadlineExceeded) || errors.Is(cycleCtx.Err(), context.Canceled) {
		return nil
	}
	return cycleCtx.Err()
}

func nodeFromServiceEntry(entry *zeroconf.ServiceEntry, cfg MeshConfig) NodeInfo {
	node := NodeInfo{
		Hostname:        entry.Instance,
		GradientVersion: "dev",
		Visibility:      VisibilityPublic,
		LastSeen:        time.Now().UTC(),
	}

	if len(entry.Text) > 0 {
		for _, record := range entry.Text {
			switch {
			case strings.HasPrefix(record, "machine_id="):
				node.MachineID = strings.TrimPrefix(record, "machine_id=")
			case strings.HasPrefix(record, "version="):
				node.GradientVersion = strings.TrimPrefix(record, "version=")
			case strings.HasPrefix(record, "suites="):
				suites := strings.TrimPrefix(record, "suites=")
				if suites != "" {
					node.InstalledSuites = strings.Split(suites, ",")
				}
			case strings.HasPrefix(record, "resolver="):
				node.ResolverRunning = strings.TrimPrefix(record, "resolver=") == "true"
			case strings.HasPrefix(record, "baselines="):
				node.BaselineGroups, _ = strconv.Atoi(strings.TrimPrefix(record, "baselines="))
			case strings.HasPrefix(record, "drifted="):
				node.DriftedGroups, _ = strconv.Atoi(strings.TrimPrefix(record, "drifted="))
			case strings.HasPrefix(record, "baseline_updated="):
				if ts, err := time.Parse(time.RFC3339, strings.TrimPrefix(record, "baseline_updated=")); err == nil {
					node.BaselineUpdated = ts
				}
			case strings.HasPrefix(record, "visibility="):
				node.Visibility = NodeVisibility(strings.TrimPrefix(record, "visibility="))
			}
		}
	}

	if len(entry.AddrIPv4) > 0 {
		node.Address = net.JoinHostPort(entry.AddrIPv4[0].String(), fmt.Sprintf("%d", entry.Port))
	}
	return node
}

func allowDiscoveredNode(cfg MeshConfig, node NodeInfo) bool {
	if node.Visibility != VisibilityPrivate {
		return true
	}
	if len(cfg.Peers) == 0 {
		return false
	}
	host := node.Address
	if parsedHost, _, err := net.SplitHostPort(node.Address); err == nil {
		host = parsedHost
	}
	for _, peer := range cfg.Peers {
		if peer == host || peer == node.Address || peer == node.MachineID {
			return true
		}
	}
	return false
}

func purgeExpiredPeers(ctx context.Context, peers *PeerList, ttl time.Duration) {
	ticker := time.NewTicker(ttl / 2)
	defer ticker.Stop()

	lastSeen := make(map[string]time.Time)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, peer := range peers.List() {
				if peer.MachineID == "" {
					continue
				}
				if peer.LastSeen.IsZero() {
					continue
				}
				lastSeen[peer.MachineID] = peer.LastSeen
			}
			for machineID, seen := range lastSeen {
				if now.Sub(seen) > ttl {
					peers.Remove(machineID)
					delete(lastSeen, machineID)
				}
			}
		}
	}
}
