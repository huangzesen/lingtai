package fs

import (
	"fmt"
	"strings"
)

func BuildNetwork(baseDir string) (Network, error) {
	nodes, err := DiscoverAgents(baseDir)
	if err != nil {
		return Network{}, fmt.Errorf("discover agents: %w", err)
	}

	for i := range nodes {
		// Normalize state to uppercase (Python kernel writes lowercase)
		nodes[i].State = strings.ToUpper(nodes[i].State)
		if nodes[i].IsHuman {
			nodes[i].Alive = true
		} else {
			nodes[i].Alive = IsAlive(nodes[i].WorkingDir, 2.0)
			// Heartbeat is ground truth — no heartbeat means SUSPENDED
			if !nodes[i].Alive && nodes[i].State != "" {
				nodes[i].State = "SUSPENDED"
			}
		}
	}

	nodeIndex := make(map[string]bool)
	for _, n := range nodes {
		nodeIndex[n.WorkingDir] = true
	}

	var avatarEdges []AvatarEdge
	for _, n := range nodes {
		edges, childDirs := ReadLedger(n.WorkingDir)
		avatarEdges = append(avatarEdges, edges...)
		for _, cd := range childDirs {
			if !nodeIndex[cd] {
				nodes = append(nodes, AgentNode{
					Address:    cd,
					AgentName:  "",
					WorkingDir: cd,
				})
				nodeIndex[cd] = true
			}
		}
	}

	var contactEdges []ContactEdge
	for _, n := range nodes {
		contactEdges = append(contactEdges, ReadContacts(n.WorkingDir)...)
	}

	// Count from inbox + archive — messages exist in exactly one folder
	mailEdges := buildMailEdges(nodes)
	stats := computeStats(nodes, mailEdges)

	return Network{
		Nodes:        nodes,
		AvatarEdges:  avatarEdges,
		ContactEdges: contactEdges,
		MailEdges:    mailEdges,
		Stats:        stats,
	}, nil
}

func buildMailEdges(nodes []AgentNode) []MailEdge {
	type edgeKey struct{ sender, recipient string }
	type edgeCounts struct{ direct, cc, bcc int }
	counts := make(map[edgeKey]*edgeCounts)

	ensure := func(k edgeKey) *edgeCounts {
		if c, ok := counts[k]; ok {
			return c
		}
		c := &edgeCounts{}
		counts[k] = c
		return c
	}

	for _, n := range nodes {
		if n.WorkingDir == "" {
			continue
		}
		inbox, _ := ReadInbox(n.WorkingDir)
		archive, _ := ReadArchive(n.WorkingDir)
		allMail := append(inbox, archive...)
		for _, msg := range allMail {
			// Direct recipients (To field)
			recipients := resolveRecipients(msg.To)
			for _, r := range recipients {
				ensure(edgeKey{msg.From, r}).direct++
			}
			// CC recipients
			for _, r := range msg.CC {
				ensure(edgeKey{msg.From, r}).cc++
			}
			// BCC recipients
			for _, r := range msg.BCC {
				ensure(edgeKey{msg.From, r}).bcc++
			}
		}
	}

	var edges []MailEdge
	for k, c := range counts {
		edges = append(edges, MailEdge{
			Sender:    k.sender,
			Recipient: k.recipient,
			Count:     c.direct + c.cc + c.bcc,
			Direct:    c.direct,
			CC:        c.cc,
			BCC:       c.bcc,
		})
	}
	return edges
}

func resolveRecipients(to interface{}) []string {
	switch v := to.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func computeStats(nodes []AgentNode, mailEdges []MailEdge) NetworkStats {
	var s NetworkStats
	for _, n := range nodes {
		switch strings.ToUpper(n.State) {
		case "ACTIVE":
			s.Active++
		case "IDLE":
			s.Idle++
		case "STUCK":
			s.Stuck++
		case "ASLEEP":
			s.Asleep++
		case "SUSPENDED":
			s.Suspended++
		}
	}
	for _, e := range mailEdges {
		s.TotalMails += e.Count
	}
	return s
}
