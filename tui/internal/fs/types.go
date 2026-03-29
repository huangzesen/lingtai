// internal/fs/types.go
package fs

// AgentNode represents a discovered agent in the network.
type AgentNode struct {
	Address      string   `json:"address"`
	AgentName    string   `json:"agent_name"`
	Nickname     string   `json:"nickname"`
	State        string   `json:"state"`
	Alive        bool     `json:"alive"`
	IsHuman      bool     `json:"is_human"`
	Capabilities []string `json:"capabilities"`
	WorkingDir   string   `json:"-"` // not serialized to API
}

// AvatarEdge is a parent → child spawning relationship.
type AvatarEdge struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	ChildName string `json:"child_name"`
}

// ContactEdge is an agent's declared knowledge of another address.
type ContactEdge struct {
	Owner  string `json:"owner"`
	Target string `json:"target"`
	Name   string `json:"name"`
}

// MailEdge is aggregated communication from sender → recipient.
type MailEdge struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Count     int    `json:"count"`
}

// NetworkStats holds aggregate counts.
type NetworkStats struct {
	Active     int `json:"active"`
	Idle       int `json:"idle"`
	Stuck      int `json:"stuck"`
	Asleep     int `json:"asleep"`
	Suspended  int `json:"suspended"`
	TotalMails int `json:"total_mails"`
}

// Network is the full topology payload returned by the API.
type Network struct {
	Nodes        []AgentNode   `json:"nodes"`
	AvatarEdges  []AvatarEdge  `json:"avatar_edges"`
	ContactEdges []ContactEdge `json:"contact_edges"`
	MailEdges    []MailEdge    `json:"mail_edges"`
	Stats        NetworkStats  `json:"stats"`
}

// MailMessage is the schema for messages written to mailbox/inbox/{uuid}/message.json.
type MailMessage struct {
	ID         string                 `json:"id"`
	MailboxID  string                 `json:"_mailbox_id"`
	From       string                 `json:"from"`
	To         interface{}            `json:"to"` // string or []string
	CC         []string               `json:"cc"`
	Subject    string                 `json:"subject"`
	Message    string                 `json:"message"`
	Type        string                 `json:"type"`
	ReceivedAt  string                 `json:"received_at"`
	Attachments []string               `json:"attachments,omitempty"`
	Identity    map[string]interface{} `json:"identity,omitempty"`
}
