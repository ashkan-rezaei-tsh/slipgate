package handlers

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anonvector/slipgate/internal/actions"
	"github.com/anonvector/slipgate/internal/config"
	"github.com/anonvector/slipgate/internal/prompt"
	"github.com/anonvector/slipgate/internal/scanner"
)

const scanConcurrency = 50
const scanTimeoutMs = 3000

func handleTunnelScan(ctx *actions.Context) error {
	cfg := ctx.Config.(*config.Config)

	// Collect scannable tunnels (DNSTT with pubkey, Slipstream with cert)
	var tunnels []*config.TunnelConfig
	for i := range cfg.Tunnels {
		t := &cfg.Tunnels[i]
		switch t.Transport {
		case config.TransportDNSTT:
			if t.DNSTT != nil && t.DNSTT.PublicKey != "" {
				tunnels = append(tunnels, t)
			}
		case config.TransportSlipstream:
			if t.Slipstream != nil && t.Slipstream.Cert != "" {
				tunnels = append(tunnels, t)
			}
		}
	}
	if len(tunnels) == 0 {
		return actions.NewErrorWithHint(actions.TunnelScan,
			"no scannable tunnels found",
			"Add a DNSTT or Slipstream tunnel first (option 1 in Tunnel Management)", nil)
	}

	// Select tunnel
	opts := make([]actions.SelectOption, len(tunnels))
	for i, t := range tunnels {
		opts[i] = actions.SelectOption{
			Value: t.Tag,
			Label: fmt.Sprintf("%s (%s, %s)", t.Tag, t.Domain, t.Transport),
		}
	}
	tag, err := prompt.Select("Tunnel to scan for", opts)
	if err != nil {
		return err
	}
	var tunnel *config.TunnelConfig
	for i := range tunnels {
		if tunnels[i].Tag == tag {
			tunnel = tunnels[i]
			break
		}
	}

	// Derive HMAC key from tunnel type
	pubkey, err := scannerKeyForTunnel(tunnel)
	if err != nil {
		return actions.NewError(actions.TunnelScan, "failed to load tunnel key", err)
	}

	// Ask for resolver list file
	resolverFile, err := prompt.String("Resolver list file path", "")
	if err != nil {
		return err
	}
	if resolverFile == "" {
		return actions.NewError(actions.TunnelScan, "resolver file path is required", nil)
	}

	f, err := os.Open(resolverFile)
	if err != nil {
		return actions.NewError(actions.TunnelScan, "failed to open resolver file", err)
	}
	defer f.Close()

	var resolvers []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		resolvers = append(resolvers, line)
	}
	if err := sc.Err(); err != nil {
		return actions.NewError(actions.TunnelScan, "failed to read resolver file", err)
	}
	if len(resolvers) == 0 {
		return actions.NewError(actions.TunnelScan, "resolver file is empty", nil)
	}

	domain := tunnel.Domain
	fmt.Printf("\n  Scanning %d resolvers for tunnel %q (%s)...\n\n", len(resolvers), tag, domain)

	// Run concurrently
	sem := make(chan struct{}, scanConcurrency)
	var wg sync.WaitGroup
	var passed atomic.Int64
	var tested atomic.Int64

	for _, host := range resolvers {
		wg.Add(1)
		sem <- struct{}{}
		go func(h string) {
			defer wg.Done()
			defer func() { <-sem }()

			ok := scanner.VerifyResolver(h, 53, domain, pubkey, scanTimeoutMs)
			tested.Add(1)
			if ok {
				passed.Add(1)
				fmt.Printf("  \033[0;32m[+]\033[0m %s\n", h)
			}
		}(host)
	}
	wg.Wait()

	fmt.Printf("\n  Done. %d/%d resolvers passed.\n", passed.Load(), tested.Load())
	return nil
}

// scannerKeyForTunnel returns the HMAC key for the given tunnel:
// - DNSTT: hex-decoded public key (file or inline)
// - Slipstream: SHA-256 of the DER-encoded certificate
func scannerKeyForTunnel(tunnel *config.TunnelConfig) ([]byte, error) {
	switch tunnel.Transport {
	case config.TransportDNSTT:
		pubkeyHex := strings.TrimSpace(tunnel.DNSTT.PublicKey)
		if _, err := os.Stat(pubkeyHex); err == nil {
			data, err := os.ReadFile(pubkeyHex)
			if err != nil {
				return nil, fmt.Errorf("read pubkey file: %w", err)
			}
			pubkeyHex = strings.TrimSpace(string(data))
		}
		b, err := hex.DecodeString(pubkeyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid public key (expected hex): %w", err)
		}
		return b, nil

	case config.TransportSlipstream:
		data, err := os.ReadFile(tunnel.Slipstream.Cert)
		if err != nil {
			return nil, fmt.Errorf("read cert: %w", err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("no PEM block in %s", tunnel.Slipstream.Cert)
		}
		hash := sha256.Sum256(block.Bytes)
		return hash[:], nil

	default:
		return nil, fmt.Errorf("unsupported transport: %s", tunnel.Transport)
	}
}
