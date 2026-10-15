package model

// ConsoleEvent carries output or a stream lifecycle notification, never console input.
type ConsoleEvent struct {
	Content string `json:"content,omitempty"`
	Ready   bool   `json:"ready,omitempty"`
	End     bool   `json:"end,omitempty"`
	Error   string `json:"error,omitempty"`
}
