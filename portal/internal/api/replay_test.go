package api

import (
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
