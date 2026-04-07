package router

import (
	"github.com/ashkan-rezaei-tsh/slipgate/internal/config"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/dnsrouter"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/service"
)

// AddTunnel registers a tunnel with the routing layer.
// The DNS router forwards port 53 traffic to internal tunnel ports.
func AddTunnel(cfg *config.Config, tunnel *config.TunnelConfig) error {
	if !tunnel.IsDNSTunnel() {
		return nil
	}

	status, _ := service.Status("slipgate-dnsrouter")
	if status == "active" {
		return dnsrouter.RestartRouterService()
	}
	return ensureRouterRunning()
}

// RemoveTunnel unregisters a tunnel from routing.
// Restarts the router to drop the removed tunnel's route.
func RemoveTunnel(cfg *config.Config, tag string) error {
	status, _ := service.Status("slipgate-dnsrouter")
	if status == "active" {
		return dnsrouter.RestartRouterService()
	}
	return nil
}

