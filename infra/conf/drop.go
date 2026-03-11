package conf

import (
	"github.com/xtls/xray-core/proxy/drop"
	"google.golang.org/protobuf/proto"
)

// DropConfig is the JSON configuration for the drop outbound protocol.
// It is parsed from the "settings" field of an outbound with protocol "drop".
type DropConfig struct {
	// LossPercent is the packet loss rate in percentage (0 - 100).
	LossPercent uint32 `json:"lossPercent"`
	// Direction specifies which direction to apply packet loss: "upload", "download", or "all".
	// Defaults to "all" if not specified.
	Direction string `json:"direction"`
}

// Build converts DropConfig to the internal drop.Config proto message.
func (v *DropConfig) Build() (proto.Message, error) {
	return &drop.Config{
		LossPercent: v.LossPercent,
		Direction:   v.Direction,
	}, nil
}
