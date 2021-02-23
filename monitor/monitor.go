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
	maxPacketSize = 1448
)

type Monitor struct {
	statuses map[string]*Status
	resolved map[string]*net.UDPAddr // XXX: do we need this?
	ipToName map[string]string
	m        sync.RWMutex // guards statuses, resolved, and ipToName
}

func NewMonitor() *Monitor {
	return &Monitor{
		statuses: make(map[string]*Status),
		resolved: make(map[string]*net.UDPAddr),
		ipToName: make(map[string]string),
	}
}

func (m *Monitor) AddServer(serverName string) error {
	if m == nil {
		return nil
	}

	m.m.Lock()
	defer m.m.Unlock()
	// TODO: check that an existing server isn't getting overwritten
	m.statuses[serverName] = &Status{
		inFlight: make(map[uint64]time.Time),
	}

	dst, err := net.ResolveUDPAddr("udp", serverName)
	if err != nil {
		return fmt.Errorf("failed to resolve server name: %w", err)
	}
	m.resolved[serverName] = dst
	ipStr := dst.String()
	m.ipToName[ipStr] = serverName
	return nil
}

type Status struct {
	// inFlight is a map of GetStatus nonces to times at which they were sent
	//XXX needs cleanup mechanism so it can't increase without bound
	inFlight map[uint64]time.Time
	// rtts is a slice of ping round trip times. The newest has the highest index
	//XXX needs cleanup mechanism so it can't increase without bound
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

// TODO: panic recovery
func Send(ctx context.Context, log *zap.Logger, m *Monitor, conn net.PacketConn) error {
	ticker := time.NewTicker(delayInterval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		m.m.Lock()
		func() {
			defer m.m.Unlock()
			for serverName, status := range m.statuses {
				log := log.With(zap.String("serverName", serverName))
				log.Debug("sending server ping")

				packet := &ServerGetStatus{
					Nonce: rand.Uint64(),
				}
				packetBytes, err := Marshal(packet)
				if err != nil {
					log.Error("failed to marshal GetStatus", zap.Error(err))
					continue
				}
				dst, ok := m.resolved[serverName]
				if !ok {
					log.Error("address not resolved")
					continue
				}
				_, err = conn.WriteTo(packetBytes, dst)
				if err != nil {
					log.Error("failed to send GetStatus", zap.Error(err))
					continue
				}
				log.Debug("sent successfully")
				status.inFlight[packet.Nonce] = time.Now()
			}
		}()
	}
}

// TODO: panic recovery
func Receive(ctx context.Context, log *zap.Logger, m *Monitor, conn net.PacketConn) error {
	packetBuf := make([]byte, maxPacketSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// TODO: conn.SetReadDeadline(...)
		n, addr, err := conn.ReadFrom(packetBuf)
		if n > 0 {
			remoteAddr, ok := addr.(*net.UDPAddr)
			if !ok {
				log.Error("unexpected type for address") // Probably can't happen
				continue
			}

			pLog := log.With(zap.String("remoteAddr", remoteAddr.String()))
			go processPacket(ctx, pLog, m, remoteAddr, packetBuf[:n])
			packetBuf = make([]byte, maxPacketSize)
		}
		if err != nil {
			// TODO: should we always exit here?
			log.Error("Receive goroutine exiting due to error")
		}
	}
}

func processPacket(ctx context.Context, log *zap.Logger, m *Monitor, remoteAddr *net.UDPAddr, buf []byte) {
	log.Debug("started processing packet")
	defer func() { log.Debug("finished processing packet") }()

	m.m.Lock()
	defer m.m.Unlock()

	serverName, ok := m.ipToName[remoteAddr.String()]
	if !ok {
		log.Error("could not look up server name by IP of received packet")
		return
	}
	status, ok := m.statuses[serverName]
	if !ok {
		log.Error("could not find Status by server name")
		return
	}

	packetStatus := ServerStatus{}
	if err := Unmarshal(buf, &packetStatus); err != nil {
		log.Error("failed to unmarshal packet", zap.Error(err))
		return
	}

	log.Debug("received Status packet", zap.Any("packetStatus", packetStatus))
	nonce := packetStatus.Nonce
	sentTime, ok := status.inFlight[nonce]
	if !ok {
		log.Error("unrecognized nonce from received packet", zap.Uint64("nonce", nonce))
		return
	}
	delete(status.inFlight, nonce)
	rtt := time.Since(sentTime)

	status.rtts = append(status.rtts, rtt)

	ping := status.CalcPing()
	if ping != nil {
		log.Debug("calculated ping", zap.Duration("ping", *ping))
	}
	status.PlayerCount = packetStatus.PlayerCount
	status.RoomCount = packetStatus.RoomCount
	status.ServerName = packetStatus.ServerName
	status.ServerVersion = packetStatus.ServerVersion
}
