package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/grandcat/zeroconf"
)

// ServerInfo describes a discovered Citadel server.
type ServerInfo struct {
	Name string
	Addr string // host:port
	Port int
}

// Browse discovers Citadel servers on the LAN via mDNS.
// It returns immediately; discovery runs in the background.
// Results are sent to the returned channel, which is closed when ctx is cancelled.
// Errors during setup are logged and the channel is closed silently.
func Browse(ctx context.Context) <-chan ServerInfo {
	out := make(chan ServerInfo, 16)
	go func() {
		defer close(out)

		entries := make(chan *zeroconf.ServiceEntry, 16)
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			slog.Warn("mDNS resolver failed", "err", err)
			return
		}
		if err := resolver.Browse(ctx, serviceType, domain, entries); err != nil {
			slog.Warn("mDNS browse failed", "err", err)
			return
		}

		seen := map[string]bool{}
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					return
				}
				if seen[entry.Instance] {
					continue
				}
				seen[entry.Instance] = true

				host := entry.HostName
				if len(entry.AddrIPv4) > 0 {
					host = entry.AddrIPv4[0].String()
				} else if len(entry.AddrIPv6) > 0 {
					host = entry.AddrIPv6[0].String()
				}

				si := ServerInfo{
					Name: entry.Instance,
					Addr: net.JoinHostPort(host, fmt.Sprintf("%d", entry.Port)),
					Port: entry.Port,
				}
				slog.Debug("discovered server", "name", si.Name, "addr", si.Addr)
				select {
				case out <- si:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
