// Package drop is an outbound handler that randomly drops packets based on configurable loss rate.
package drop

const (
	// DirectionUpload applies packet loss only to upload traffic (client -> server).
	DirectionUpload = "upload"
	// DirectionDownload applies packet loss only to download traffic (server -> client).
	DirectionDownload = "download"
	// DirectionAll applies packet loss to all traffic (both upload and download).
	DirectionAll = "all"
)

// GetEffectiveDirection returns the effective direction, defaulting to "all" if not set.
func (c *Config) GetEffectiveDirection() string {
	if c.Direction == "" {
		return DirectionAll
	}
	return c.Direction
}
