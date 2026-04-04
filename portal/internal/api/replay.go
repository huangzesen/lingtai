package api

import (
	"encoding/json"

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
	Nodes []fs.AgentNode   `json:"nodes,omitempty"`
	Mail  []fs.MailEdge    `json:"mail,omitempty"`
	Stats *fs.NetworkStats `json:"stats,omitempty"`
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
