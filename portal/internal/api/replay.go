package api

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
			// Emit a tombstone: address + sentinel state so it can't be
			// confused with a real node that has empty fields.
			delta.Nodes = append(delta.Nodes, fs.AgentNode{
				Address: n.Address,
				State:   "__REMOVED__",
			})
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

// buildManifest constructs the manifest from a cached manifest.json on disk
// plus a quick tail-scan of the JSONL for any new frames since the last build.
// This is O(new_frames) — typically just the current hour.
func buildManifest(topologyPath, replayDir string) (ReplayManifest, error) {
	os.MkdirAll(replayDir, 0o755)

	// 1. Try reading the cached manifest
	manifestPath := filepath.Join(replayDir, "manifest.json")
	var cached ReplayManifest
	var hasCached bool
	if data, err := os.ReadFile(manifestPath); err == nil {
		if json.Unmarshal(data, &cached) == nil && len(cached.Chunks) > 0 {
			hasCached = true
		}
	}

	// 2. Determine where to start scanning the JSONL
	// We need to re-scan from the start of the last cached chunk
	// (it may have been the "current" hour that has grown since last build).
	// However, if the last cached chunk already has a .json.gz on disk
	// (e.g. written by a rebuild), trust it and only scan after it.
	var scanFrom int64
	var fixedChunks []ChunkInfo // chunks we trust (completed hours)
	if hasCached {
		lastCached := cached.Chunks[len(cached.Chunks)-1]
		lastCachePath := filepath.Join(replayDir, strconv.FormatInt(lastCached.Start, 10)+".json.gz")
		if _, err := os.Stat(lastCachePath); err == nil {
			// Last chunk has a cache file — trust all chunks
			fixedChunks = cached.Chunks
		} else {
			// Last chunk has no cache — drop it for re-scan from JSONL
			fixedChunks = cached.Chunks[:len(cached.Chunks)-1]
		}
		if len(fixedChunks) > 0 {
			scanFrom = fixedChunks[len(fixedChunks)-1].Start + hourMs
		}
	}

	// 3. Scan JSONL tail for new/updated frames
	tailFrames, err := scanJSONLFrom(topologyPath, scanFrom)
	if err != nil && !os.IsNotExist(err) {
		return ReplayManifest{}, err
	}

	// 4. Group tail frames into hours
	chunks := append([]ChunkInfo{}, fixedChunks...)
	if len(tailFrames) > 0 {
		type hourGroup struct {
			start  int64
			frames []fs.TapeFrame
		}
		var tailHours []*hourGroup
		hourIndex := make(map[int64]*hourGroup)

		for _, f := range tailFrames {
			bucket := hourBucket(f.T)
			g, ok := hourIndex[bucket]
			if !ok {
				g = &hourGroup{start: bucket}
				hourIndex[bucket] = g
				tailHours = append(tailHours, g)
			}
			g.frames = append(g.frames, f)
		}

		// Cache all completed tail hours (all except the last)
		for i, g := range tailHours {
			info := ChunkInfo{
				Start:  g.start,
				End:    g.frames[len(g.frames)-1].T,
				Frames: len(g.frames),
			}
			chunks = append(chunks, info)

			if i < len(tailHours)-1 {
				cachePath := filepath.Join(replayDir, strconv.FormatInt(g.start, 10)+".json.gz")
				if _, err := os.Stat(cachePath); err != nil {
					// Only write if not already cached
					chunk := deltaEncode(g.frames, defaultKeyframeInterval)
					writeChunkCache(cachePath, chunk)
				}
			}
		}
	}

	if len(chunks) == 0 {
		return ReplayManifest{}, nil
	}

	sort.Slice(chunks, func(i, j int) bool { return chunks[i].Start < chunks[j].Start })

	manifest := ReplayManifest{
		TapeStart: chunks[0].Start,
		TapeEnd:   chunks[len(chunks)-1].End,
		Chunks:    chunks,
	}

	// 5. Write manifest cache to disk
	if data, err := json.Marshal(manifest); err == nil {
		os.WriteFile(manifestPath, data, 0o644)
	}

	return manifest, nil
}

// scanJSONLFrom reads topology.jsonl and returns frames with T >= fromMs.
// Scans from the beginning but skips frames before fromMs. For large files
// where most data is cached, only the tail is collected.
func scanJSONLFrom(topologyPath string, fromMs int64) ([]fs.TapeFrame, error) {
	f, err := os.Open(topologyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var frames []fs.TapeFrame
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Quick timestamp extraction without full unmarshal for skipping
		var frame fs.TapeFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		if frame.T < fromMs {
			continue
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// fullCompile does a complete re-scan of topology.jsonl, rebuilding all caches.
// Used by the rebuild endpoint. This is the slow path — O(all_frames).
func fullCompile(topologyPath, replayDir string) (ReplayManifest, error) {
	// Clear existing caches
	os.RemoveAll(replayDir)
	os.MkdirAll(replayDir, 0o755)

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

// NewManifestHandler serves GET /api/topology/manifest.
// Builds the manifest from cached chunks + a tail scan — fast even for large tapes.
func NewManifestHandler(baseDir string) http.HandlerFunc {
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(baseDir, ".portal", "replay", "chunks")

	return func(w http.ResponseWriter, r *http.Request) {
		TopologyMu.Lock()
		manifest, err := buildManifest(topologyPath, replayDir)
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

// NewRebuildHandler serves POST /api/topology/rebuild.
// Reconstructs the full tape from source data (events.jsonl + mailbox),
// then writes compressed hourly chunks.
func NewRebuildHandler(baseDir string) http.HandlerFunc {
	topologyPath := filepath.Join(baseDir, ".portal", "topology.jsonl")
	replayDir := filepath.Join(baseDir, ".portal", "replay", "chunks")

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Reconstruct from source data
		frames, err := fs.ReconstructTape(baseDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(frames) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			json.NewEncoder(w).Encode(ReplayManifest{Chunks: []ChunkInfo{}})
			return
		}

		// Clear old caches
		os.RemoveAll(replayDir)
		os.MkdirAll(replayDir, 0o755)

		// Stream into hourly compressed chunks
		var currentHour int64 = -1
		var hourFrames []fs.TapeFrame
		var chunks []ChunkInfo

		flushHour := func() {
			if len(hourFrames) == 0 {
				return
			}
			info := ChunkInfo{
				Start:  currentHour,
				End:    hourFrames[len(hourFrames)-1].T,
				Frames: len(hourFrames),
			}
			chunks = append(chunks, info)
			cachePath := filepath.Join(replayDir, strconv.FormatInt(currentHour, 10)+".json.gz")
			chunk := deltaEncode(hourFrames, defaultKeyframeInterval)
			writeChunkCache(cachePath, chunk)
			hourFrames = nil
		}

		for _, f := range frames {
			bucket := hourBucket(f.T)
			if bucket != currentHour {
				flushHour()
				currentHour = bucket
			}
			hourFrames = append(hourFrames, f)
		}
		flushHour()

		// Write minimal topology.jsonl (last frame for live recording)
		TopologyMu.Lock()
		lastFrame := frames[len(frames)-1]
		line, _ := json.Marshal(lastFrame)
		os.WriteFile(topologyPath, append(line, '\n'), 0o644)
		TopologyMu.Unlock()

		// Write manifest
		manifest := ReplayManifest{
			TapeStart: chunks[0].Start,
			TapeEnd:   chunks[len(chunks)-1].End,
			Chunks:    chunks,
		}
		mdata, _ := json.Marshal(manifest)
		os.WriteFile(filepath.Join(replayDir, "manifest.json"), mdata, 0o644)

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
