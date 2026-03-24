package mesh

import (
	"sort"
	"sync"
)

// PeerList stores the current mesh peers in a concurrency-safe map.
type PeerList struct {
	mu    sync.RWMutex
	peers map[string]NodeInfo
}

// Add inserts or updates a node in the peer list.
func (p *PeerList) Add(node NodeInfo) {
	if node.MachineID == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensure()
	p.peers[node.MachineID] = node
}

// Remove deletes a node from the peer list by machine ID.
func (p *PeerList) Remove(machineID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.peers == nil {
		return
	}
	delete(p.peers, machineID)
}

// List returns a snapshot of all peers sorted by hostname and machine ID.
func (p *PeerList) List() []NodeInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.peers) == 0 {
		return []NodeInfo{}
	}

	out := make([]NodeInfo, 0, len(p.peers))
	for _, peer := range p.peers {
		out = append(out, peer)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Hostname == out[j].Hostname {
			return out[i].MachineID < out[j].MachineID
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out
}

// Get returns the peer for a machine ID if it exists.
func (p *PeerList) Get(machineID string) (NodeInfo, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.peers == nil {
		return NodeInfo{}, false
	}

	node, ok := p.peers[machineID]
	return node, ok
}

func (p *PeerList) ensure() {
	if p.peers == nil {
		p.peers = make(map[string]NodeInfo)
	}
}
