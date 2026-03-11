// Package drop is an outbound handler that forwards traffic with configurable random packet loss.
// It simulates a lossy network by randomly discarding data chunks while keeping the connection open.
package drop

import (
	"context"
	"math/rand"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/retry"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/signal"
	"github.com/xtls/xray-core/common/task"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/stat"
)

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		h := new(Handler)
		if err := core.RequireFeatures(ctx, func(pm policy.Manager) error {
			return h.Init(config.(*Config), pm)
		}); err != nil {
			return nil, err
		}
		return h, nil
	}))
}

// Handler handles Drop connections with configurable packet loss.
type Handler struct {
	policyManager policy.Manager
	config        *Config
}

// Init initializes the Handler with necessary parameters.
func (h *Handler) Init(config *Config, pm policy.Manager) error {
	h.config = config
	h.policyManager = pm
	return nil
}

func (h *Handler) policy() policy.Session {
	p := h.policyManager.ForLevel(0)
	return p
}

// shouldDrop returns true if a packet should be dropped based on the configured loss rate.
// LossPercent is uint32 in range 0-100.
func (h *Handler) shouldDrop() bool {
	lossPercent := h.config.GetLossPercent()
	if lossPercent == 0 {
		return false
	}
	if lossPercent >= 100 {
		return true
	}
	// rand.Intn(100) returns [0, 100), drop if result < lossPercent
	return rand.Intn(100) < int(lossPercent)
}

// lossWriter is a buf.Writer that randomly drops data chunks based on configured loss rate.
// Dropped chunks are released (memory freed) and NOT forwarded, keeping the connection open.
type lossWriter struct {
	writer  buf.Writer
	handler *Handler
}

func (w *lossWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	var kept buf.MultiBuffer
	for {
		mb2, b := buf.SplitFirst(mb)
		mb = mb2
		if b == nil {
			break
		}
		if w.handler.shouldDrop() {
			// Drop: release memory and do NOT forward this chunk.
			// The connection stays open; TCP will trigger retransmission automatically.
			b.Release()
		} else {
			// Keep: will be forwarded
			kept = append(kept, b)
		}
	}
	if len(kept) == 0 {
		// All chunks dropped in this round — connection stays open, return nil
		return nil
	}
	return w.writer.WriteMultiBuffer(kept)
}

// Process implements proxy.Outbound.
func (h *Handler) Process(ctx context.Context, link *transport.Link, dialer internet.Dialer) error {
	outbounds := session.OutboundsFromContext(ctx)
	ob := outbounds[len(outbounds)-1]
	if !ob.Target.IsValid() {
		return errors.New("target not specified.")
	}
	ob.Name = "drop"

	destination := ob.Target

	// Dial the target connection (same approach as freedom protocol)
	var conn stat.Connection
	err := retry.ExponentialBackoff(5, 100).On(func() error {
		rawConn, err := dialer.Dial(ctx, destination)
		if err != nil {
			return err
		}
		conn = rawConn
		return nil
	})
	if err != nil {
		return errors.New("failed to open connection to ", destination).Base(err)
	}
	defer conn.Close()

	errors.LogInfo(ctx, "drop: connection to ", destination,
		", lossPercent=", h.config.GetLossPercent(),
		"%, direction=", h.config.GetEffectiveDirection())

	plcy := h.policy()
	ctx, cancel := context.WithCancel(ctx)
	timer := signal.CancelAfterInactivity(ctx, cancel, plcy.Timeouts.ConnectionIdle)

	direction := h.config.GetEffectiveDirection()

	// requestDone handles upload direction: client -> target server
	// Reads from link.Reader, optionally applies drop, writes to conn.
	requestDone := func() error {
		defer timer.SetTimeout(plcy.Timeouts.DownlinkOnly)

		var writer buf.Writer
		if direction == DirectionUpload || direction == DirectionAll {
			// Apply packet loss on upload
			writer = &lossWriter{
				writer:  buf.NewWriter(conn),
				handler: h,
			}
		} else {
			// Pass through without loss
			writer = buf.NewWriter(conn)
		}

		if err := buf.Copy(link.Reader, writer, buf.UpdateActivity(timer)); err != nil {
			return errors.New("failed to process request").Base(err)
		}
		return nil
	}

	// responseDone handles download direction: target server -> client
	// Reads from conn, optionally applies drop, writes to link.Writer.
	responseDone := func() error {
		defer timer.SetTimeout(plcy.Timeouts.UplinkOnly)

		var baseWriter buf.Writer
		if direction == DirectionDownload || direction == DirectionAll {
			// Apply packet loss on download
			baseWriter = &lossWriter{
				writer:  link.Writer,
				handler: h,
			}
		} else {
			// Pass through without loss
			baseWriter = link.Writer
		}

		var reader buf.Reader
		if destination.Network == net.Network_UDP {
			reader = &buf.PacketReader{Reader: conn}
		} else {
			reader = buf.NewReader(conn)
		}

		if err := buf.Copy(reader, baseWriter, buf.UpdateActivity(timer)); err != nil {
			return errors.New("failed to process response").Base(err)
		}
		return nil
	}

	if err := task.Run(ctx, requestDone, task.OnSuccess(responseDone, task.Close(link.Writer))); err != nil {
		return errors.New("connection ends").Base(err)
	}

	return nil
}
