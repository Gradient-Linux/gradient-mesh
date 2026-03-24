package mesh

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPeerList_AddAndRemove(t *testing.T) {
	var peers PeerList

	peers.Add(NodeInfo{Hostname: "alpha", MachineID: "a", LastSeen: time.Now().UTC()})
	peers.Add(NodeInfo{Hostname: "beta", MachineID: "b", LastSeen: time.Now().UTC()})

	if got := len(peers.List()); got != 2 {
		t.Fatalf("List() len = %d, want 2", got)
	}

	peers.Remove("a")
	if _, ok := peers.Get("a"); ok {
		t.Fatalf("Get(a) ok = true, want false")
	}
	if got := len(peers.List()); got != 1 {
		t.Fatalf("List() len = %d, want 1", got)
	}
}

func TestPeerList_ListReturnsSortedCopy(t *testing.T) {
	var peers PeerList

	peers.Add(NodeInfo{Hostname: "bravo", MachineID: "2"})
	peers.Add(NodeInfo{Hostname: "alpha", MachineID: "1"})

	got := peers.List()
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	if got[0].Hostname != "alpha" || got[1].Hostname != "bravo" {
		t.Fatalf("List() order = %#v, want alpha then bravo", got)
	}
}

func TestPeerList_ConcurrentAccess(t *testing.T) {
	var peers PeerList

	const workers = 32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("machine-%02d", i%8)
			peers.Add(NodeInfo{Hostname: id, MachineID: id})
			if i%3 == 0 {
				peers.Remove(id)
			}
			_ = peers.List()
			_, _ = peers.Get(id)
		}(i)
	}
	wg.Wait()
}
