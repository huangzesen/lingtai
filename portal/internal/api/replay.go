package api

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anthropics/lingtai-portal/internal/fs"
)

const defaultKeyframeInterval = 100

// ReplayChunk is the wire format for a range of delta-encoded topology frames.
type ReplayChunk struct {
	Start            int64         `json:"start"`
	End              int64         `json:"end"`
	KeyframeInterval int           `json:"keyframe_interval"`
	Frames           []ReplayFrame `json:"frames"`
}

// ReplayFrame is either a keyframe (Net set) or a delta (Delta set).
type ReplayFrame struct {
	T     int64        `json:"t"`
	Net   *fs.Network  `json:"net,omitempty"`
	Delta *FrameDelta  `json:"d,omitempty"`
}

// FrameDelta holds only the fields that changed relative to the previous frame.
type FrameDelta struct {
	Nodes        []fs.AgentNode   `json:"nodes,omitempty"`
	AvatarEdges  []fs.AvatarEdge  `json:"avatar_edges,omitempty"`
	ContactEdges []fs.ContactEdge `json:"contact_edges,omitempty"`
	Mail         []fs.MailEdge    `json:"mail,omitempty"`
	Stats        *fs.NetworkStats `json:"stats,omitempty"`
}

// ChunkInfo is a manifest entry describing one chunk.
type ChunkInfo struct {
	Start  int64 `json:"start"`
	End    int64 `json:"end"`
	Frames int   `json:"frames"`
}

// ReplayManifest lists all available chunks for a tape.
type ReplayManifest struct {
	TapeStart int64       `json:"tape_start"`
	TapeEnd   int64       `json:"tape_end"`
	Chunks    []ChunkInfo `json:"chunks"`
}

// deltaEncode converts a slice of TapeFrame into a ReplayChunk with keyframes
// every keyframeInterval frames and compact deltas in between.
func deltaEncode(frames []fs.TapeFrame, keyframeInterval int) ReplayChunk {
	chunk := ReplayChunk{
		KeyframeInterval: keyframeInterval,
	}
	if len(frames) == 0 {
		return chunk
	}

	chunk.Start = frames[0].T
	chunk.End = frames[len(frames)-1].T

	var prev *fs.Network
	for i, f := range frames {
		if i%keyframeInterval == 0 || prev == nil {
			net := f.Net
			chunk.Frames = append(chunk.Frames, ReplayFrame{T: f.T, Net: &net})
			prev = &net
			continue
		}

		delta := computeDelta(prev, &f.Net)
		chunk.Frames = append(chunk.Frames, ReplayFrame{T: f.T, Delta: delta})
		curr := f.Net
		prev = &curr
	}

	return chunk
}

// computeDelta returns a FrameDelta describing what changed between prev and
// curr, or nil if nothing changed.
func computeDelta(prev, curr *fs.Network) *FrameDelta {
	var delta FrameDelta
	hasChange := false

	// --- Node changes ---
	prevNodes := make(map[string]fs.AgentNode, len(prev.Nodes))
	for _, n := range prev.Nodes {
		prevNodes[n.Address] = n
	}

	for _, n := range curr.Nodes {
		if pn, ok := prevNodes[n.Address]; !ok || !nodesEqual(pn, n) {
			delta.Nodes = append(delta.Nodes, n)
			hasChange = true
		}
	}

	// Detect removed nodes (present in prev but absent in curr).
	currNodes := make(map[string]bool, len(curr.Nodes))
	for _, n := range curr.Nodes {
		currNodes[n.Address] = true
	}
	for _, n := range prev.Nodes {
		if !currNodes[n.Address] {
			// Emit a tombstone with only the address filled in.
			delta.Nodes = append(delta.Nodes, fs.AgentNode{Address: n.Address})
			hasChange = true
		}
	}

	// --- Avatar edge changes ---
	type avatarKey struct{ parent, child string }
	prevAvatars := make(map[avatarKey]fs.AvatarEdge, len(prev.AvatarEdges))
	for _, e := range prev.AvatarEdges {
		prevAvatars[avatarKey{e.Parent, e.Child}] = e
	}
	for _, e := range curr.AvatarEdges {
		k := avatarKey{e.Parent, e.Child}
		if pe, ok := prevAvatars[k]; !ok || pe != e {
			delta.AvatarEdges = append(delta.AvatarEdges, e)
			hasChange = true
		}
	}

	// --- Contact edge changes ---
	type contactKey struct{ owner, target string }
	prevContacts := make(map[contactKey]fs.ContactEdge, len(prev.ContactEdges))
	for _, e := range prev.ContactEdges {
		prevContacts[contactKey{e.Owner, e.Target}] = e
	}
	for _, e := range curr.ContactEdges {
		k := contactKey{e.Owner, e.Target}
		if pe, ok := prevContacts[k]; !ok || pe != e {
			delta.ContactEdges = append(delta.ContactEdges, e)
			hasChange = true
		}
	}

	// --- Mail edge changes ---
	type mailKey struct{ sender, recipient string }
	prevMail := make(map[mailKey]fs.MailEdge, len(prev.MailEdges))
	for _, e := range prev.MailEdges {
		prevMail[mailKey{e.Sender, e.Recipient}] = e
	}
	for _, e := range curr.MailEdges {
		k := mailKey{e.Sender, e.Recipient}
		if pe, ok := prevMail[k]; !ok || pe != e {
			delta.Mail = append(delta.Mail, e)
			hasChange = true
		}
	}

	// --- Stats ---
	if curr.Stats != prev.Stats {
		stats := curr.Stats
		delta.Stats = &stats
		hasChange = true
	}

	if !hasChange {
		return nil
	}
	return &delta
}

// nodesEqual compares two AgentNode values for semantic equality.
func nodesEqual(a, b fs.AgentNode) bool {
	if a.Address != b.Address || a.AgentName != b.AgentName || a.Nickname != b.Nickname {
		return false
	}
	if a.State != b.State || a.Alive != b.Alive || a.IsHuman != b.IsHuman {
		return false
	}
	if len(a.Capabilities) != len(b.Capabilities) {
		return false
	}
	for i := range a.Capabilities {
		if a.Capabilities[i] != b.Capabilities[i] {
			return false
		}
	}
	if a.Location == nil && b.Location == nil {
		return true
	}
	if a.Location == nil || b.Location == nil {
		return false
	}
	aLoc, _ := json.Marshal(a.Location)
	bLoc, _ := json.Marshal(b.Location)
	return string(aLoc) == string(bLoc)
}

// ---------------------------------------------------------------------------
// Chunk compilation and caching
// ---------------------------------------------------------------------------

const hourMs int64 = 3600 * 1000

func hourBucket(t int64) int64 {
	return (t / hourMs) * hourMs
}

func compileChunks(topologyPath, replayDir string) (ReplayManifest, error) {
	f, err := os.Open(topologyPath)
	if err != nil {
		return ReplayManifest{}, err
	}
	defer f.Close()

	type hourGroup struct {
		start  int64
		frames []fs.TapeFrame
	}
	var hours []*hourGroup
	hourIndex := make(map[int64]*hourGroup)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var frame fs.TapeFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		bucket := hourBucket(frame.T)
		g, ok := hourIndex[bucket]
		if !ok {
			g = &hourGroup{start: bucket}
			hourIndex[bucket] = g
			hours = append(hours, g)
		}
		g.frames = append(g.frames, frame)
	}

	if len(hours) == 0 {
		return ReplayManifest{}, nil
	}

	os.MkdirAll(replayDir, 0o755)

	lastHourStart := hours[len(hours)-1].start

	manifest := ReplayManifest{
		TapeStart: hours[0].frames[0].T,
		TapeEnd:   hours[len(hours)-1].frames[len(hours[len(hours)-1].frames)-1].T,
	}

	for _, g := range hours {
		info := ChunkInfo{
			Start:  g.start,
			End:    g.frames[len(g.frames)-1].T,
			Frames: len(g.frames),
		}
		manifest.Chunks = append(manifest.Chunks, info)

		if g.start != lastHourStart {
			cachePath := filepath.Join(replayDir, strconv.FormatInt(g.start, 10)+".json.gz")
			if _, err := os.Stat(cachePath); err == nil {
				continue
			}
			chunk := deltaEncode(g.frames, defaultKeyframeInterval)
			if err := writeChunkCache(cachePath, chunk); err != nil {
				return ReplayManifest{}, fmt.Errorf("cache chunk %d: %w", g.start, err)
			}
		}
	}

	return manifest, nil
}

func writeChunkCache(path string, chunk ReplayChunk) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	if err := json.NewEncoder(gw).Encode(chunk); err != nil {
		gw.Close()
		return err
	}
	return gw.Close()
}

func readChunkCache(path string) (ReplayChunk, error) {
	f, err := os.Open(path)
	if err != nil {
		return ReplayChunk{}, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return ReplayChunk{}, err
	}
	defer gr.Close()

	var chunk ReplayChunk
	if err := json.NewDecoder(gr).Decode(&chunk); err != nil {
		return ReplayChunk{}, err
	}
	return chunk, nil
}

func loadChunk(replayDir, topologyPath string, hourStart int64) (ReplayChunk, error) {
	cachePath := filepath.Join(replayDir, strconv.FormatInt(hourStart, 10)+".json.gz")
	if chunk, err := readChunkCache(cachePath); err == nil {
		return chunk, nil
	}

	f, err := os.Open(topologyPath)
	if err != nil {
		return ReplayChunk{}, err
	}
	defer f.Close()

	hourEnd := hourStart + hourMs
	var frames []fs.TapeFrame

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var frame fs.TapeFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		if frame.T < hourStart {
			continue
		}
		if frame.T >= hourEnd {
			break
		}
		frames = append(frames, frame)
	}

	if len(frames) == 0 {
		return ReplayChunk{Start: hourStart, End: hourStart}, nil
	}

	return deltaEncode(frames, defaultKeyframeInterval), nil
}

// cachedManifest avoids re-scanning the full JSONL on every manifest request.
// Invalidated when the topology file grows.
var (
	manifestCache     *ReplayManifest
	manifestCacheSize int64
)

// NewManifestHandler serves GET /api/topology/manifest.
func NewManifestHandler(baseDir string) http.HandlerFunc {
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(baseDir, ".portal", "replay", "chunks")

	return func(w http.ResponseWriter, r *http.Request) {
		TopologyMu.Lock()
		// Check if cached manifest is still valid (file hasn't grown)
		fi, _ := os.Stat(topologyPath)
		fileSize := int64(0)
		if fi != nil {
			fileSize = fi.Size()
		}
		if manifestCache != nil && fileSize == manifestCacheSize {
			manifest := *manifestCache
			TopologyMu.Unlock()
			if manifest.Chunks == nil {
				manifest.Chunks = []ChunkInfo{}
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			json.NewEncoder(w).Encode(manifest)
			return
		}

		manifest, err := compileChunks(topologyPath, replayDir)
		if err == nil {
			manifestCache = &manifest
			manifestCacheSize = fileSize
		}
		TopologyMu.Unlock()

		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			json.NewEncoder(w).Encode(ReplayManifest{Chunks: []ChunkInfo{}})
			return
		}
		if manifest.Chunks == nil {
			manifest.Chunks = []ChunkInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(manifest)
	}
}

// NewChunkHandler serves GET /api/topology/chunk?start=<hourMs>.
func NewChunkHandler(baseDir string) http.HandlerFunc {
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(baseDir, ".portal", "replay", "chunks")

	return func(w http.ResponseWriter, r *http.Request) {
		startStr := r.URL.Query().Get("start")
		if startStr == "" {
			http.Error(w, "missing start parameter", http.StatusBadRequest)
			return
		}
		hourStart, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid start parameter", http.StatusBadRequest)
			return
		}

		// Lock while reading JSONL (loadChunk may fall back to scanning the file)
		TopologyMu.Lock()
		chunk, err := loadChunk(replayDir, topologyPath, hourStart)
		TopologyMu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			gw := gzip.NewWriter(w)
			json.NewEncoder(gw).Encode(chunk)
			gw.Close()
			return
		}

		json.NewEncoder(w).Encode(chunk)
	}
}
