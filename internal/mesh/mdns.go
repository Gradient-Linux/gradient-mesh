package mesh

import (
	"context"
	"fmt"
	"net"
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
		"version=" + self.GradientVersion,
		"suites=" + strings.Join(self.InstalledSuites, ","),
		"resolver=" + fmt.Sprintf("%t", self.ResolverRunning),
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
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("create zeroconf resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case entry := <-entries:
				if entry == nil {
					continue
				}
				node := nodeFromServiceEntry(entry, cfg)
				if node.MachineID != "" {
					peers.Add(node)
				}
			}
		}
	}()

	go purgeExpiredPeers(ctx, peers, 60*time.Second)

	if err := resolver.Browse(ctx, serviceType, serviceDomain, entries); err != nil {
		return fmt.Errorf("browse mDNS services: %w", err)
	}
	<-ctx.Done()
	return ctx.Err()
}

func nodeFromServiceEntry(entry *zeroconf.ServiceEntry, cfg MeshConfig) NodeInfo {
	node := NodeInfo{
		Hostname:        entry.Instance,
		GradientVersion: "dev",
		Visibility:      cfg.Visibility,
		LastSeen:        time.Now().UTC(),
	}

	if len(entry.Text) > 0 {
		for _, record := range entry.Text {
			switch {
			case strings.HasPrefix(record, "version="):
				node.GradientVersion = strings.TrimPrefix(record, "version=")
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
