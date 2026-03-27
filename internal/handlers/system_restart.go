package handlers

import (
	"fmt"

	"github.com/anonvector/slipgate/internal/actions"
	"github.com/anonvector/slipgate/internal/config"
	"github.com/anonvector/slipgate/internal/network"
	"github.com/anonvector/slipgate/internal/service"
)

func handleSystemRestart(ctx *actions.Context) error {
	out := ctx.Output
	cfg := ctx.Config.(*config.Config)

	out.Info("Restarting all services...")

	// Ensure port 53 is available
	for _, t := range cfg.Tunnels {
		if t.IsDNSTunnel() {
			if err := network.DisableResolvedStub(); err != nil {
				out.Warning("Failed to disable systemd-resolved stub: " + err.Error())
			}
			break
		}
	}

	// Restart infrastructure services first
	for _, svc := range []string{"slipgate-dnsrouter", "slipgate-socks5"} {
		if service.Exists(svc) {
			if err := service.Restart(svc); err != nil {
				out.Warning(fmt.Sprintf("Failed to restart %s: %v", svc, err))
			} else {
				out.Success(fmt.Sprintf("  %s restarted", svc))
			}
		}
	}

	// Restart tunnel services
	for _, t := range cfg.Tunnels {
		if t.IsDirectTransport() {
			continue
		}
		svcName := service.TunnelServiceName(t.Tag)
		if service.Exists(svcName) {
			if err := service.Restart(svcName); err != nil {
				out.Warning(fmt.Sprintf("Failed to restart %s: %v", svcName, err))
			} else {
				out.Success(fmt.Sprintf("  %s restarted", svcName))
			}
		}
	}

	out.Print("")
	out.Success("All services restarted")
	return nil
}
