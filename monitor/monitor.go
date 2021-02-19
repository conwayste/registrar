package monitor

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	delayInterval = 5 * time.Second //XXX fiddle with later
)

type Monitor struct {
	statuses map[string]Status
	m        sync.RWMutex // guards statuses
}

func NewMonitor() *Monitor {
	return &Monitor{
		statuses: make(map[string]Status),
	}
}

func (m *Monitor) AddServer(serverName string) {
	if m == nil {
		return
	}

	m.m.Lock()
	defer m.m.Unlock()
	// TODO: check that an existing server isn't getting overwritten
	m.statuses[serverName] = Status{
		inFlight: make(map[uint64]time.Time),
	}
}

type Status struct {
	// inFlight is a map of GetStatus nonces to times at which they were sent
	inFlight map[uint64]time.Time
	// rtts is a slice of ping round trip times. The newest has the highest index
	rtts          []time.Duration
	ServerVersion string
	PlayerCount   uint64
	RoomCount     uint64
	ServerName    string
}

// Ping returns the average ping, or nil if unknown.
func (s *Status) CalcPing() *time.Duration {
	if s == nil {
		return nil
	}

	var sum time.Duration
	for _, rtt := range s.rtts {
		sum += rtt
	}
	if len(s.rtts) > 0 {
		avg := sum / time.Duration(len(s.rtts))
		return &avg
	} else {
		return nil
	}
}

func Start(ctx context.Context, log *zap.Logger, m *Monitor) error {
	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return fmt.Errorf("failed to open UDP port: %w", err)
	}
	defer conn.Close()

	ticker := time.NewTicker(delayInterval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		m.m.Lock()
		for serverName, _ := range m.statuses {
			log := log.With(zap.String("serverName", serverName))
			log.Debug("sending server ping")
			dst, err := net.ResolveUDPAddr("udp", serverName)
			if err != nil {
				log.Error("failed to resolve server name")
				continue
			}

			packet := &ServerGetStatus{
				Nonce: rand.Uint64(),
			}
			packetBytes, err := Marshal(packet)
			if err != nil {
				log.Error("failed to marshal GetStatus", zap.Error(err))
				continue
			}
			_, err = conn.WriteTo(packetBytes, dst)
			if err != nil {
				log.Error("failed to send GetStatus", zap.Error(err))
				continue
			}
			log.Debug("sent successfully")
		}
		m.m.Unlock()
	}
}
