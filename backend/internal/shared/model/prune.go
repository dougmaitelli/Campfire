package model

// PruneResult counts Docker resources removed without touching game data.
type PruneResult struct {
	Containers int `json:"containers"`
	Images     int `json:"images"`
	Volumes    int `json:"volumes"`
}
