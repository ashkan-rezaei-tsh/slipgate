package handlers

import (
	"fmt"

	"github.com/ashkan-rezaei-tsh/slipgate/internal/actions"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/clientcfg"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/config"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/prompt"
)

func handleTunnelShare(ctx *actions.Context) error {
	cfg := ctx.Config.(*config.Config)
	tag := ctx.GetArg("tag")

	if tag == "" {
		return actions.NewError(actions.TunnelShare, "tunnel tag is required", nil)
	}

	tunnel := cfg.GetTunnel(tag)
	if tunnel == nil {
		return actions.NewErrorWithHint(actions.TunnelShare, "tunnel not found",
			"Run 'slipgate tunnel status' to see available tunnels", nil)
	}

	backend := cfg.GetBackend(tunnel.Backend)
	if backend == nil {
		return actions.NewError(actions.TunnelShare, "backend not found", nil)
	}

	opts := clientcfg.URIOptions{}

	// For DNSTT transport, ask which client mode
	if tunnel.Transport == config.TransportDNSTT {
		opts.ClientMode = ctx.GetArg("mode")
		if opts.ClientMode == "" {
			var err error
			opts.ClientMode, err = prompt.Select("Client mode", actions.ClientModeOptions)
			if err != nil {
				return err
			}
		}
	}

	// Ask which user's credentials to embed
	if len(cfg.Users) > 0 {
		userOpts := make([]actions.SelectOption, 0, len(cfg.Users)+1)
		userOpts = append(userOpts, actions.SelectOption{Value: "", Label: "No credentials"})
		for _, u := range cfg.Users {
			userOpts = append(userOpts, actions.SelectOption{Value: u.Username, Label: u.Username})
		}
		username, err := prompt.Select("User", userOpts)
		if err != nil {
			return err
		}
		if user := cfg.GetUser(username); user != nil {
			opts.Username = user.Username
			opts.Password = user.Password
		}
	}

	if tunnel.Transport == config.TransportMasterDNS {
		ctx.Output.Print("# MasterDnsVPN client basic setup snippet:")
		ctx.Output.Print(fmt.Sprintf("DOMAIN = [\"%s\"]", tunnel.Domain))
		ctx.Output.Print(fmt.Sprintf("ENCRYPTION_KEY_FILE = \"%s\"", tunnel.MasterDNS.EncryptionKey))
		ctx.Output.Print(fmt.Sprintf("MAX_PACKET_SIZE = %d", tunnel.MasterDNS.MTU))
		ctx.Output.Print("PROTOCOL_TYPE = \"TCP\"")
		return nil
	}

	uri, err := clientcfg.GenerateURI(tunnel, backend, cfg, opts)
	if err != nil {
		return actions.NewError(actions.TunnelShare, "failed to generate URI", err)
	}

	ctx.Output.Print(uri)
	return nil
}
