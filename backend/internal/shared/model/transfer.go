package model

// TransferRequest carries one bounded, idempotent transfer operation between
// Campfire and an authenticated Spark. Archives never pass through browser APIs.
type TransferRequest struct {
	ID        string `json:"id"`
	ServerID  string `json:"server_id"`
	Operation string `json:"operation"`
	Offset    int64  `json:"offset,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

// TransferResult reports durable local progress or one immutable archive chunk.
type TransferResult struct {
	ID       string `json:"id"`
	ServerID string `json:"server_id"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Digest   string `json:"digest,omitempty"`
	Offset   int64  `json:"offset,omitempty"`
	Data     []byte `json:"data,omitempty"`
}
