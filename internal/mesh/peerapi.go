package mesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	meshSnapshotDir = "config/env-snapshots"
	meshBaselineDir = "config/env-baselines"
)

type baselineSnapshot struct {
	Group     string            `json:"group"`
	Timestamp time.Time         `json:"timestamp"`
	Packages  map[string]string `json:"packages"`
	Backend   string            `json:"backend"`
	MLflowIDs []string          `json:"mlflow_ids"`
}

type baselineMeta struct {
	Group     string    `json:"group"`
	Timestamp time.Time `json:"timestamp"`
	Packages  int       `json:"packages"`
}

type peerState struct {
	Node      NodeInfo       `json:"node"`
	Baselines []baselineMeta `json:"baselines"`
}

func (d *Daemon) servePeerAPI(ctx context.Context, state *daemonState) error {
	cfg := state.currentConfig()
	if cfg.Visibility == VisibilityHidden {
		<-ctx.Done()
		return ctx.Err()
	}

	addr := fmt.Sprintf(":%d", cfg.ServicePort)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/state", func(w http.ResponseWriter, r *http.Request) {
		snapshot := peerState{
			Node:      state.currentSelf(),
			Baselines: listBaselineMeta(cfg.WorkspaceRoot),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
	mux.HandleFunc("/v1/baselines/", func(w http.ResponseWriter, r *http.Request) {
		group := strings.TrimPrefix(r.URL.Path, "/v1/baselines/")
		group, err := url.PathUnescape(group)
		if err != nil {
			http.Error(w, "invalid group", http.StatusBadRequest)
			return
		}
		snapshot, err := loadBaselineSnapshot(cfg.WorkspaceRoot, group)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func syncPeerBaselines(ctx context.Context, workspaceRoot string, node NodeInfo) {
	host, port, ok := splitHostPort(node.Address)
	if !ok {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}

	stateURL := fmt.Sprintf("http://%s:%s/v1/state", host, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stateURL, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var state peerState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return
	}

	local := baselineMetaMap(workspaceRoot)
	for _, meta := range state.Baselines {
		current, ok := local[meta.Group]
		if ok && !meta.Timestamp.After(current.Timestamp) {
			continue
		}
		baselineURL := fmt.Sprintf("http://%s:%s/v1/baselines/%s", host, port, url.PathEscape(meta.Group))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baselineURL, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}
		var snapshot baselineSnapshot
		if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		_ = saveBaselineSnapshot(workspaceRoot, snapshot)
	}
}

func nodeSummary(workspaceRoot string) (baselineGroups int, driftedGroups int, baselineUpdated time.Time) {
	baselines := listBaselineMeta(workspaceRoot)
	baselineGroups = len(baselines)
	for _, meta := range baselines {
		if meta.Timestamp.After(baselineUpdated) {
			baselineUpdated = meta.Timestamp
		}
		latest, err := latestSnapshotForGroup(workspaceRoot, meta.Group)
		if err != nil {
			continue
		}
		baseline, err := loadBaselineSnapshot(workspaceRoot, meta.Group)
		if err != nil {
			continue
		}
		if !samePackages(baseline.Packages, latest.Packages) {
			driftedGroups++
		}
	}
	return baselineGroups, driftedGroups, baselineUpdated
}

func listBaselineMeta(workspaceRoot string) []baselineMeta {
	dir := filepath.Join(workspaceRoot, meshBaselineDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []baselineMeta{}
	}
	items := make([]baselineMeta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		group := strings.TrimSuffix(entry.Name(), ".json")
		snapshot, err := loadBaselineSnapshot(workspaceRoot, group)
		if err != nil {
			continue
		}
		items = append(items, baselineMeta{
			Group:     snapshot.Group,
			Timestamp: snapshot.Timestamp,
			Packages:  len(snapshot.Packages),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Group < items[j].Group
	})
	return items
}

func baselineMetaMap(workspaceRoot string) map[string]baselineMeta {
	items := listBaselineMeta(workspaceRoot)
	out := make(map[string]baselineMeta, len(items))
	for _, item := range items {
		out[item.Group] = item
	}
	return out
}

func loadBaselineSnapshot(workspaceRoot, group string) (baselineSnapshot, error) {
	return loadSnapshotFile(filepath.Join(workspaceRoot, meshBaselineDir, fmt.Sprintf("%s.json", sanitizeGroup(group))))
}

func saveBaselineSnapshot(workspaceRoot string, snapshot baselineSnapshot) error {
	dir := filepath.Join(workspaceRoot, meshBaselineDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.json", sanitizeGroup(snapshot.Group)))
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".baseline-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func latestSnapshotForGroup(workspaceRoot, group string) (baselineSnapshot, error) {
	dir := filepath.Join(workspaceRoot, meshSnapshotDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return baselineSnapshot{}, err
	}
	groupKey := sanitizeGroup(group) + "."
	type candidate struct {
		path string
		ts   time.Time
	}
	var latest candidate
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".lock" || !strings.HasPrefix(entry.Name(), groupKey) {
			continue
		}
		tsStr := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), groupKey), ".lock")
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			continue
		}
		if latest.path == "" || ts.After(latest.ts) {
			latest = candidate{path: filepath.Join(dir, entry.Name()), ts: ts}
		}
	}
	if latest.path == "" {
		return baselineSnapshot{}, os.ErrNotExist
	}
	return loadSnapshotFile(latest.path)
}

func loadSnapshotFile(path string) (baselineSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return baselineSnapshot{}, err
	}
	var snapshot baselineSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return baselineSnapshot{}, err
	}
	if snapshot.Packages == nil {
		snapshot.Packages = map[string]string{}
	}
	return snapshot, nil
}

func sanitizeGroup(group string) string {
	group = strings.TrimSpace(strings.ToLower(group))
	if group == "" {
		return "default"
	}
	group = strings.ReplaceAll(group, " ", "-")
	group = strings.ReplaceAll(group, "/", "-")
	return group
}

func samePackages(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func splitHostPort(address string) (string, string, bool) {
	if address == "" {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", "", false
	}
	return host, port, true
}

