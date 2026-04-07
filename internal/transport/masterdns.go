package transport

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ashkan-rezaei-tsh/slipgate/internal/config"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/service"
)

func createMasterDNSService(tunnel *config.TunnelConfig, cfg *config.Config) error {
	if tunnel.MasterDNS == nil {
		return fmt.Errorf("masterdns config is nil")
	}

	backend := cfg.GetBackend(tunnel.Backend)
	if backend == nil {
		return fmt.Errorf("backend %q not found", tunnel.Backend)
	}

	tunnelDir := config.TunnelDir(tunnel.Tag)
	if err := os.MkdirAll(tunnelDir, 0750); err != nil {
		return err
	}

	keyPath := filepath.Join(tunnelDir, "encrypt_key.txt")
	if err := os.WriteFile(keyPath, []byte(tunnel.MasterDNS.EncryptionKey), 0600); err != nil {
		return err
	}

	parts := strings.Split(backend.Address, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid backend address format: %s", backend.Address)
	}
	forwardIP := parts[0]
	forwardPort, _ := strconv.Atoi(parts[1])

	mtu := tunnel.MasterDNS.MTU
	if mtu == 0 {
		mtu = config.DefaultMTU
	}

	// Assuming MasterDnsVPN supports TCP forwarding mode or SOCKS5.
	// Since we are forwarding to a local backend like SSH/SOCKS, we define it here.
	protocolType := "TCP"
	if backend.Type == config.BackendSOCKS {
		// If MasterDnsVPN handles SOCKS internally, we can either use it directly
		// or forward TCP packets to the actual slipgate socks port. Let's forward via TCP.
		protocolType = "TCP"
	}

	tomlConfig := fmt.Sprintf(`DOMAIN = ["%s"]
PROTOCOL_TYPE = "%s"
UDP_HOST = "0.0.0.0"
UDP_PORT = %d
MAX_PACKET_SIZE = %d
FORWARD_IP = "%s"
FORWARD_PORT = %d
DATA_ENCRYPTION_METHOD = 1
ENCRYPTION_KEY_FILE = "encrypt_key.txt"
LOG_LEVEL = "INFO"
CONFIG_VERSION = "10"
`, tunnel.Domain, protocolType, tunnel.Port, mtu, forwardIP, forwardPort)

	tomlPath := filepath.Join(tunnelDir, "server_config.toml")
	if err := os.WriteFile(tomlPath, []byte(tomlConfig), 0644); err != nil {
		return err
	}

	binPath := filepath.Join(config.DefaultBinDir, "masterdnsvpn-server")

	unit := &service.Unit{
		Name:        service.TunnelServiceName(tunnel.Tag),
		Description: fmt.Sprintf("SlipGate tunnel: %s (%s/%s)", tunnel.Tag, tunnel.Transport, tunnel.Backend),
		ExecStart:   binPath,
		User:        "root",
		Group:       config.SystemGroup,
		After:       "network.target",
		Restart:     "always",
		WorkingDir:  tunnelDir,
	}

	if err := service.Create(unit); err != nil {
		return err
	}
	return service.Start(unit.Name)
}
