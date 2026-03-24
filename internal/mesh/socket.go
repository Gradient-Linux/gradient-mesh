package mesh

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// DefaultSocketPath is the local query socket used by gradient-mesh.
const DefaultSocketPath = "/run/gradient/mesh.sock"

// DefaultServicePort is the LAN advertisement port used for mDNS records.
const DefaultServicePort = 7778

type socketRequest struct {
	Action string `json:"action"`
}

type socketResponse struct {
	Self  *NodeInfo  `json:"self,omitempty"`
	Peers []NodeInfo `json:"peers,omitempty"`
	Error string     `json:"error,omitempty"`
}

// SnapshotProvider returns the current self and peer snapshot.
type SnapshotProvider interface {
	SelfSnapshot() NodeInfo
	PeerSnapshot() []NodeInfo
}

// ServeSocket serves newline-delimited JSON queries over a Unix socket.
func ServeSocket(ctx context.Context, socketPath string, snapshot SnapshotProvider) error {
	if snapshot == nil {
		return errors.New("snapshot provider is nil")
	}

	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	if err := os.RemoveAll(socketPath); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)
	_ = os.Chmod(socketPath, 0o660)

	acceptErr := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case acceptErr <- err:
				default:
				}
				return
			}
			go handleSocketConn(conn, snapshot)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-acceptErr:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("accept mesh socket: %w", err)
		}
		return nil
	}
}

// QueryFleet returns the visible fleet snapshot from the local socket.
func QueryFleet(socketPath string) ([]NodeInfo, error) {
	resp, err := querySocket(socketPath, socketRequest{Action: "fleet"})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || isConnRefused(err) {
			return []NodeInfo{}, nil
		}
		return nil, err
	}
	return resp.Peers, nil
}

// QuerySelf returns the local node snapshot from the local socket.
func QuerySelf(socketPath string) (NodeInfo, error) {
	resp, err := querySocket(socketPath, socketRequest{Action: "self"})
	if err != nil {
		return NodeInfo{}, err
	}
	if resp.Self == nil {
		return NodeInfo{}, nil
	}
	return *resp.Self, nil
}

func handleSocketConn(conn net.Conn, snapshot SnapshotProvider) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	decoder := json.NewDecoder(reader)
	var req socketRequest
	if err := decoder.Decode(&req); err != nil {
		_ = writeSocketResponse(conn, socketResponse{Error: err.Error()})
		return
	}

	switch req.Action {
	case "self":
		self := snapshot.SelfSnapshot()
		_ = writeSocketResponse(conn, socketResponse{Self: &self})
	default:
		_ = writeSocketResponse(conn, socketResponse{Peers: snapshot.PeerSnapshot()})
	}
}

func querySocket(socketPath string, req socketRequest) (socketResponse, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return socketResponse{}, err
	}
	defer conn.Close()

	if err := writeSocketResponse(conn, req); err != nil {
		return socketResponse{}, err
	}

	decoder := json.NewDecoder(bufio.NewReader(conn))
	var resp socketResponse
	if err := decoder.Decode(&resp); err != nil {
		return socketResponse{}, err
	}
	if resp.Error != "" {
		return socketResponse{}, errors.New(resp.Error)
	}
	return resp, nil
}

func writeSocketResponse(conn net.Conn, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func isConnRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	return false
}
