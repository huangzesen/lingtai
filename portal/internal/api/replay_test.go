package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/lingtai-portal/internal/fs"
)

func TestDeltaEncode_KeyframesAndDeltas(t *testing.T) {
	base := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", AgentName: "agent-a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{{Sender: "a", Recipient: "b", Count: 1, Direct: 1}},
		Stats:        fs.NetworkStats{Active: 1, TotalMails: 1},
	}

	frames := make([]fs.TapeFrame, 5)
	for i := range frames {
		net := base
		net.MailEdges = []fs.MailEdge{{Sender: "a", Recipient: "b", Count: 1 + i, Direct: 1 + i}}
		net.Stats = fs.NetworkStats{Active: 1, TotalMails: 1 + i}
		frames[i] = fs.TapeFrame{T: int64(1000 + i*3000), Net: net}
	}

	chunk := deltaEncode(frames, 3)

	if chunk.Start != 1000 {
		t.Errorf("Start = %d, want 1000", chunk.Start)
	}
	if chunk.End != 13000 {
		t.Errorf("End = %d, want 13000", chunk.End)
	}
	if chunk.KeyframeInterval != 3 {
		t.Errorf("KeyframeInterval = %d, want 3", chunk.KeyframeInterval)
	}
	if len(chunk.Frames) != 5 {
		t.Fatalf("len(Frames) = %d, want 5", len(chunk.Frames))
	}

	for _, idx := range []int{0, 3} {
		if chunk.Frames[idx].Net == nil {
			t.Errorf("frame[%d] should be keyframe (Net != nil)", idx)
		}
		if chunk.Frames[idx].Delta != nil {
			t.Errorf("frame[%d] keyframe should not have Delta", idx)
		}
	}

	for _, idx := range []int{1, 2, 4} {
		if chunk.Frames[idx].Net != nil {
			t.Errorf("frame[%d] should be delta (Net == nil)", idx)
		}
	}

	if chunk.Frames[1].Delta == nil {
		t.Fatal("frame[1] delta is nil")
	}
	if len(chunk.Frames[1].Delta.Mail) != 1 {
		t.Errorf("frame[1] delta.Mail len = %d, want 1", len(chunk.Frames[1].Delta.Mail))
	}
}

func TestDeltaEncode_EmptyDelta(t *testing.T) {
	net := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{{Sender: "a", Recipient: "b", Count: 5, Direct: 5}},
		Stats:        fs.NetworkStats{Active: 1, TotalMails: 5},
	}

	frames := []fs.TapeFrame{
		{T: 1000, Net: net},
		{T: 4000, Net: net},
	}

	chunk := deltaEncode(frames, 100)

	if chunk.Frames[1].Delta != nil {
		t.Errorf("expected nil delta for identical frame, got %+v", chunk.Frames[1].Delta)
	}
}

func TestDeltaEncode_NodeChanges(t *testing.T) {
	net0 := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{},
		Stats:        fs.NetworkStats{Active: 1},
	}
	net1 := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "SUSPENDED"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{},
		Stats:        fs.NetworkStats{Suspended: 1},
	}

	frames := []fs.TapeFrame{
		{T: 1000, Net: net0},
		{T: 4000, Net: net1},
	}

	chunk := deltaEncode(frames, 100)

	if chunk.Frames[1].Delta == nil {
		t.Fatal("expected delta for node state change")
	}
	if len(chunk.Frames[1].Delta.Nodes) != 1 {
		t.Errorf("delta.Nodes len = %d, want 1", len(chunk.Frames[1].Delta.Nodes))
	}
	if chunk.Frames[1].Delta.Nodes[0].State != "SUSPENDED" {
		t.Errorf("delta node state = %q, want SUSPENDED", chunk.Frames[1].Delta.Nodes[0].State)
	}
}

func TestCompileChunks_CreatesCache(t *testing.T) {
	dir := t.TempDir()
	topologyPath := filepath.Join(dir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(dir, ".portal", "replay", "chunks")

	net := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{},
		Stats:        fs.NetworkStats{Active: 1},
	}
	for _, ts := range []int64{3600000, 3601000, 3602000, 3603000, 7200000, 7201000, 7202000, 7203000} {
		AppendTopologyAt(topologyPath, net, ts)
	}

	manifest, err := compileChunks(topologyPath, replayDir)
	if err != nil {
		t.Fatalf("compileChunks: %v", err)
	}

	if manifest.TapeStart != 3600000 {
		t.Errorf("TapeStart = %d, want 3600000", manifest.TapeStart)
	}
	if manifest.TapeEnd != 7203000 {
		t.Errorf("TapeEnd = %d, want 7203000", manifest.TapeEnd)
	}
	if len(manifest.Chunks) != 2 {
		t.Fatalf("len(Chunks) = %d, want 2", len(manifest.Chunks))
	}

	hour1File := filepath.Join(replayDir, "3600000.json.gz")
	if _, err := os.Stat(hour1File); err != nil {
		t.Errorf("hour-1 cache file missing: %v", err)
	}

	hour2File := filepath.Join(replayDir, "7200000.json.gz")
	if _, err := os.Stat(hour2File); !os.IsNotExist(err) {
		t.Errorf("hour-2 (current) chunk should not be cached, but file exists")
	}
}

func TestCompileChunks_UsesExistingCache(t *testing.T) {
	dir := t.TempDir()
	topologyPath := filepath.Join(dir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(dir, ".portal", "replay", "chunks")

	net := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{},
		Stats:        fs.NetworkStats{Active: 1},
	}
	for _, ts := range []int64{3600000, 3601000, 7200000, 7201000} {
		AppendTopologyAt(topologyPath, net, ts)
	}

	m1, err := compileChunks(topologyPath, replayDir)
	if err != nil {
		t.Fatal(err)
	}

	hour1File := filepath.Join(replayDir, "3600000.json.gz")
	info1, _ := os.Stat(hour1File)
	modTime1 := info1.ModTime()

	m2, err := compileChunks(topologyPath, replayDir)
	if err != nil {
		t.Fatal(err)
	}

	info2, _ := os.Stat(hour1File)
	if !info2.ModTime().Equal(modTime1) {
		t.Error("hour-1 cache was rewritten, should have been reused")
	}

	if m1.TapeStart != m2.TapeStart || m1.TapeEnd != m2.TapeEnd {
		t.Error("manifests differ between compilations")
	}
}

func TestLoadChunk_FromCache(t *testing.T) {
	dir := t.TempDir()
	topologyPath := filepath.Join(dir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(dir, ".portal", "replay", "chunks")

	net := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{{Sender: "a", Recipient: "b", Count: 3, Direct: 3}},
		Stats:        fs.NetworkStats{Active: 1, TotalMails: 3},
	}
	for _, ts := range []int64{3600000, 3601000, 3602000, 7200000} {
		n := net
		n.MailEdges = []fs.MailEdge{{Sender: "a", Recipient: "b", Count: int(ts / 1000), Direct: int(ts / 1000)}}
		AppendTopologyAt(topologyPath, n, ts)
	}

	compileChunks(topologyPath, replayDir)

	chunk, err := loadChunk(replayDir, topologyPath, 3600000)
	if err != nil {
		t.Fatalf("loadChunk: %v", err)
	}

	if chunk.Start != 3600000 {
		t.Errorf("chunk.Start = %d, want 3600000", chunk.Start)
	}
	if len(chunk.Frames) != 3 {
		t.Errorf("len(Frames) = %d, want 3", len(chunk.Frames))
	}
}

func TestManifestHandler(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "base")
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")

	net := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{},
		Stats:        fs.NetworkStats{Active: 1},
	}
	AppendTopologyAt(topologyPath, net, 3600000)
	AppendTopologyAt(topologyPath, net, 3601000)

	handler := NewManifestHandler(baseDir)
	req := httptest.NewRequest("GET", "/api/topology/manifest", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var manifest ReplayManifest
	if err := json.NewDecoder(rr.Body).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.TapeStart != 3600000 {
		t.Errorf("TapeStart = %d, want 3600000", manifest.TapeStart)
	}
	if len(manifest.Chunks) == 0 {
		t.Error("expected at least 1 chunk")
	}
}

func TestChunkHandler(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "base")
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")

	net := fs.Network{
		Nodes:        []fs.AgentNode{{Address: "a", State: "ACTIVE"}},
		AvatarEdges:  []fs.AvatarEdge{},
		ContactEdges: []fs.ContactEdge{},
		MailEdges:    []fs.MailEdge{},
		Stats:        fs.NetworkStats{Active: 1},
	}
	AppendTopologyAt(topologyPath, net, 3600000)
	AppendTopologyAt(topologyPath, net, 3601000)

	// Compile first so cache exists
	replayDir := filepath.Join(baseDir, ".portal", "replay", "chunks")
	compileChunks(topologyPath, replayDir)

	handler := NewChunkHandler(baseDir)
	req := httptest.NewRequest("GET", "/api/topology/chunk?start=3600000", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	// Response should be gzipped
	var body []byte
	if rr.Header().Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(rr.Body)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		body, _ = io.ReadAll(gr)
		gr.Close()
	} else {
		body, _ = io.ReadAll(rr.Body)
	}

	var chunk ReplayChunk
	if err := json.Unmarshal(body, &chunk); err != nil {
		t.Fatalf("decode chunk: %v", err)
	}
	if len(chunk.Frames) != 2 {
		t.Errorf("len(Frames) = %d, want 2", len(chunk.Frames))
	}
}
