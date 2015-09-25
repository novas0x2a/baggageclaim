package baggageclaim

import (
	"encoding/json"
	"time"
)

type VolumeRequest struct {
	Strategy     *json.RawMessage `json:"strategy"`
	Properties   VolumeProperties `json:"properties"`
	TTLInSeconds uint             `json:"ttl,omitempty"`
}

type VolumeResponse struct {
	Handle     string           `json:"handle"`
	Path       string           `json:"path"`
	Properties VolumeProperties `json:"properties"`
	TTL        uint             `json:"ttl,omitempty"`
	ExpiresAt  time.Time        `json:"expires_at"`
}

type PropertyRequest struct {
	Value string `json:"value"`
}

type TTLRequest struct {
	Value uint `json:"value"`
}
