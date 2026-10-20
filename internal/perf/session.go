package perf

import "time"

// SessionInfo is what the automatic match report adds to a Run: how the
// session was recorded (windows of PresentMon) and how it ended.
type SessionInfo struct {
	Name        string           `json:"name"` // game name shown
	Ended       time.Time        `json:"ended"`
	Reason      string           `json:"reason"`           // fim | limite | fechado | desligado | falha
	Late        bool             `json:"late,omitempty"`   // the game was already open when Axion started following it
	Paused      bool             `json:"paused,omitempty"` // windows skipped while a manual measurement used PresentMon
	SegmentSecs int              `json:"segmentSecs"`
	Segments    []SessionSegment `json:"segments,omitempty"`
	Empty       int              `json:"empty,omitempty"`   // windows without a frame with the game open
	WallSecs    float64          `json:"wallSecs"`          // from the first window to the last (coverage = Summary.Seconds / WallSecs)
	Setting     string           `json:"setting,omitempty"` // e.g. "fps_max 400" saved in the CS2 account
	Seen        bool             `json:"seen,omitempty"`
}

// SessionSegment is one PresentMon window of the session.
type SessionSegment struct {
	From    float64 `json:"from"` // seconds recorded before this window
	Summary Summary `json:"summary"`
}
