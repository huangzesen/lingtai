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
		if nodes[i].IsHuman {
			nodes[i].Alive = true
		} else {
			nodes[i].Alive = IsAlive(nodes[i].WorkingDir, 2.0)
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

	// Count from inbox only — sent would double-count
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
	counts := make(map[edgeKey]int)

	for _, n := range nodes {
		if n.WorkingDir == "" {
			continue
		}
		inbox, _ := ReadInbox(n.WorkingDir)
		for _, msg := range inbox {
			recipients := resolveRecipients(msg.To)
			for _, r := range recipients {
				counts[edgeKey{msg.From, r}]++
			}
		}
	}

	var edges []MailEdge
	for k, c := range counts {
		edges = append(edges, MailEdge{
			Sender:    k.sender,
			Recipient: k.recipient,
			Count:     c,
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
