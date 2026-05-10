package discovery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/grandcat/zeroconf"
)

const (
	serviceType = "_citadel._tcp"
	domain      = "local."
)

// Advertise registers the server under name via mDNS on the given port.
// It runs until ctx is cancelled.
func Advertise(ctx context.Context, name string, port int) error {
	txtRecords := []string{
		fmt.Sprintf("name=%s", name),
		"version=1",
	}
	srv, err := zeroconf.Register(name, serviceType, domain, port, txtRecords, nil)
	if err != nil {
		return fmt.Errorf("registering mDNS service: %w", err)
	}
	go func() {
		<-ctx.Done()
		slog.Info("shutting down mDNS advertise")
		srv.Shutdown()
	}()
	slog.Info("mDNS advertise started", "name", name, "port", port)
	return nil
}
