package profile

import "time"

// Profile describes one BrowserRuntime-owned browser profile.
type Profile struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	EngineID        string     `json:"engine_id"`
	ProfilePath     string     `json:"profile_path"`
	FingerprintSeed string     `json:"fingerprint_seed"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastLaunchedAt  *time.Time `json:"last_launched_at,omitempty"`
}
