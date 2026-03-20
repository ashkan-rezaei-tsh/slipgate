package transport

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/anonvector/slipgate/internal/config"
	"github.com/anonvector/slipgate/internal/network"
)

const wgServerConfTmpl = `[Interface]
PrivateKey = {{ .ServerPrivKey }}
Address = {{ .ServerAddress }}
ListenPort = {{ .ListenPort }}
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o {{ .Iface }} -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -t nat -D POSTROUTING -o {{ .Iface }} -j MASQUERADE

[Peer]
PublicKey = {{ .ClientPubKey }}
AllowedIPs = {{ .ClientAddress }}
`

// EnsureWireguardInstalled checks if WireGuard tools are available, tries to install if not.
// Returns true if wg is available, false if not (caller should skip WireGuard setup).
func EnsureWireguardInstalled() bool {
	if _, err := exec.LookPath("wg"); err == nil {
		return true
	}

	// Try to install
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		s := string(data)
		switch {
		case strings.Contains(s, "ubuntu") || strings.Contains(s, "debian") || strings.Contains(s, "Ubuntu") || strings.Contains(s, "Debian"):
			_ = exec.Command("apt-get", "update", "-qq").Run()
			_ = exec.Command("apt-get", "install", "-y", "wireguard-tools").Run()
		case strings.Contains(s, "centos") || strings.Contains(s, "rhel") || strings.Contains(s, "fedora"):
			_ = exec.Command("yum", "install", "-y", "wireguard-tools").Run()
		}
	}

	// Check again
	_, err := exec.LookPath("wg")
	return err == nil
}

// GenerateWireguardKeys generates a WireGuard keypair and returns (privateKey, publicKey).
func GenerateWireguardKeys() (string, string, error) {
	privOut, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}
	privKey := strings.TrimSpace(string(privOut))

	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(privKey)
	pubOut, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}
	pubKey := strings.TrimSpace(string(pubOut))

	return privKey, pubKey, nil
}

// EnableIPForwarding enables IPv4 forwarding via sysctl.
func EnableIPForwarding() error {
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if err := cmd.Run(); err != nil {
		return err
	}
	// Persist
	data, _ := os.ReadFile("/etc/sysctl.conf")
	if !strings.Contains(string(data), "net.ipv4.ip_forward") {
		f, err := os.OpenFile("/etc/sysctl.conf", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		f.WriteString("\nnet.ipv4.ip_forward = 1\n")
	}
	return nil
}

func createWireguardService(tunnel *config.TunnelConfig, cfg *config.Config) error {
	wg := tunnel.Wireguard
	if wg == nil {
		return fmt.Errorf("missing wireguard config")
	}

	// Write server config
	confPath := fmt.Sprintf("/etc/wireguard/slipgate-%s.conf", tunnel.Tag)
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		return err
	}

	// Read server private key
	privKeyData, err := os.ReadFile(wg.ServerPrivKey)
	if err != nil {
		return fmt.Errorf("reading server private key: %w", err)
	}

	iface := network.DefaultInterface()
	if iface == "" {
		iface = "eth0"
	}

	tmplData := struct {
		ServerPrivKey string
		ServerAddress string
		ListenPort    int
		ClientPubKey  string
		ClientAddress string
		Iface         string
	}{
		ServerPrivKey: strings.TrimSpace(string(privKeyData)),
		ServerAddress: wg.ServerAddress,
		ListenPort:    wg.ListenPort,
		ClientPubKey:  wg.ClientPubKey,
		ClientAddress: wg.ClientAddress,
		Iface:         iface,
	}

	tmpl, err := template.New("wg").Parse(wgServerConfTmpl)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(confPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, tmplData); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Start via wg-quick
	ifaceName := fmt.Sprintf("slipgate-%s", tunnel.Tag)
	cmd := exec.Command("wg-quick", "up", ifaceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wg-quick up: %w", err)
	}

	// Enable at boot via systemd
	_ = exec.Command("systemctl", "enable", fmt.Sprintf("wg-quick@%s.service", ifaceName)).Run()
	return nil
}

// RemoveWireguardService stops and removes a WireGuard tunnel.
func RemoveWireguardService(tag string) error {
	ifaceName := fmt.Sprintf("slipgate-%s", tag)
	_ = exec.Command("wg-quick", "down", ifaceName).Run()
	_ = exec.Command("systemctl", "disable", fmt.Sprintf("wg-quick@%s.service", ifaceName)).Run()

	confPath := fmt.Sprintf("/etc/wireguard/%s.conf", ifaceName)
	os.Remove(confPath)

	// Remove key files
	tunnelDir := config.TunnelDir(tag)
	os.Remove(filepath.Join(tunnelDir, "wg-server.key"))
	os.Remove(filepath.Join(tunnelDir, "wg-client.key"))

	return nil
}

// GenerateClientConfig returns the wg-quick client config string for the app.
func GenerateClientConfig(tunnel *config.TunnelConfig, serverIP string) string {
	wg := tunnel.Wireguard
	if wg == nil {
		return ""
	}

	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s

[Peer]
PublicKey = %s
Endpoint = %s:%d
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`, wg.ClientPrivKey, wg.ClientAddress, wg.DNS, wg.ServerPubKey, serverIP, wg.ListenPort)
}
