package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	broadcastPort     = 7778
	broadcastInterval = 2 * time.Second
	broadcastMagic    = "CITADEL:"
)

// BroadcastPresence sends UDP presence packets every 2 s so clients on the
// same subnet can find the server even when mDNS multicast is blocked.
// listenIP is the server's primary outbound IP embedded in the payload so
// the client always dials the right address regardless of which interface
// the broadcast packet arrived on.
func BroadcastPresence(ctx context.Context, name string, port int, listenIP string) {
	go func() {
		ticker := time.NewTicker(broadcastInterval)
		defer ticker.Stop()

		// payload: CITADEL:<name>:<ip>:<port>
		payload := []byte(fmt.Sprintf("%s%s:%s:%d", broadcastMagic, name, listenIP, port))

		for {
			select {
			case <-ticker.C:
				for _, dest := range directedBroadcastAddrs() {
					conn, err := net.DialUDP("udp4", nil, dest)
					if err != nil {
						continue
					}
					_, _ = conn.Write(payload)
					_ = conn.Close()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	slog.Info("UDP broadcast presence started", "port", broadcastPort)
}

// directedBroadcastAddrs returns the directed broadcast address for every
// active interface that supports broadcast (skips loopback and v6-only).
func directedBroadcastAddrs() []*net.UDPAddr {
	var addrs []*net.UDPAddr
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
				continue
			}
			ifAddrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, a := range ifAddrs {
				ipnet, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				ip4 := ipnet.IP.To4()
				if ip4 == nil {
					continue
				}
				bcast := make(net.IP, 4)
				for i := range bcast {
					bcast[i] = ip4[i] | ^ipnet.Mask[i]
				}
				addrs = append(addrs, &net.UDPAddr{IP: bcast, Port: broadcastPort})
			}
		}
	}
	if len(addrs) == 0 {
		addrs = []*net.UDPAddr{{IP: net.IPv4bcast, Port: broadcastPort}}
	}
	return addrs
}

// BrowseBroadcast listens for broadcast presence packets from Citadel servers.
// Returns immediately; discovery runs in the background.
func BrowseBroadcast(ctx context.Context) <-chan ServerInfo {
	out := make(chan ServerInfo, 8)
	go func() {
		defer close(out)

		conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: broadcastPort})
		if err != nil {
			slog.Debug("UDP broadcast listen failed", "err", err)
			return
		}
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()

		seen := map[string]bool{}
		buf := make([]byte, 256)

		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			msg := string(buf[:n])
			if !strings.HasPrefix(msg, broadcastMagic) {
				continue
			}
			rest := msg[len(broadcastMagic):]

			// payload format: <name>:<ip>:<port>
			firstColon := strings.Index(rest, ":")
			lastColon := strings.LastIndex(rest, ":")
			if firstColon < 0 || firstColon == lastColon {
				continue
			}
			serverName := rest[:firstColon]
			listenIP := rest[firstColon+1 : lastColon]
			portStr := rest[lastColon+1:]
			serverPort, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			addr := net.JoinHostPort(listenIP, portStr)
			if seen[addr] {
				continue
			}
			seen[addr] = true

			si := ServerInfo{Name: serverName, Addr: addr, Port: serverPort}
			slog.Debug("discovered server via broadcast", "name", si.Name, "addr", si.Addr)
			select {
			case out <- si:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Merge fans multiple ServerInfo channels into one deduplicated stream.
func Merge(ctx context.Context, channels ...<-chan ServerInfo) <-chan ServerInfo {
	out := make(chan ServerInfo, 16)
	seen := make(map[string]bool)
	var mu sync.Mutex

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(c <-chan ServerInfo) {
			defer wg.Done()
			for {
				select {
				case si, ok := <-c:
					if !ok {
						return
					}
					mu.Lock()
					dup := seen[si.Addr]
					if !dup {
						seen[si.Addr] = true
					}
					mu.Unlock()
					if dup {
						continue
					}
					select {
					case out <- si:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
