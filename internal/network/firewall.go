package network

import (
	"fmt"
	"os"
	"os/exec"
)

// AllowPort opens a port using the first available firewall tool.
func AllowPort(port int, proto string) error {
	// Try UFW first
	if _, err := exec.LookPath("ufw"); err == nil {
		return ufwAllow(port, proto)
	}

	// Try firewall-cmd
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		return firewalldAllow(port, proto)
	}

	// Fallback to iptables
	if _, err := exec.LookPath("iptables"); err == nil {
		return iptablesAllow(port, proto)
	}

	return fmt.Errorf("no supported firewall found (ufw, firewalld, iptables)")
}

// RemovePort removes a firewall rule for a port.
func RemovePort(port int, proto string) error {
	if _, err := exec.LookPath("ufw"); err == nil {
		return run("ufw", "delete", "allow", fmt.Sprintf("%d/%s", port, proto))
	}
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		_ = run("firewall-cmd", "--permanent", "--remove-port", fmt.Sprintf("%d/%s", port, proto))
		return run("firewall-cmd", "--reload")
	}
	if _, err := exec.LookPath("iptables"); err == nil {
		return run("iptables", "-D", "INPUT", "-p", proto, "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
	}
	return nil
}

func ufwAllow(port int, proto string) error {
	return run("ufw", "allow", fmt.Sprintf("%d/%s", port, proto))
}

func firewalldAllow(port int, proto string) error {
	if err := run("firewall-cmd", "--permanent", "--add-port", fmt.Sprintf("%d/%s", port, proto)); err != nil {
		return err
	}
	return run("firewall-cmd", "--reload")
}

func iptablesAllow(port int, proto string) error {
	return run("iptables", "-A", "INPUT", "-p", proto, "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
}

// DisableResolvedStub disables systemd-resolved's DNS stub listener on port 53
// so that slipgate's DNS router can bind to it.
func DisableResolvedStub() error {
	// Check if systemd-resolved is running
	if err := exec.Command("systemctl", "is-active", "systemd-resolved").Run(); err != nil {
		return nil // not running, nothing to do
	}

	// Create drop-in config to disable stub listener
	if err := os.MkdirAll("/etc/systemd/resolved.conf.d", 0755); err != nil {
		return fmt.Errorf("create resolved conf dir: %w", err)
	}

	conf := "[Resolve]\nDNSStubListener=no\n"
	if err := os.WriteFile("/etc/systemd/resolved.conf.d/slipgate-no-stub.conf", []byte(conf), 0644); err != nil {
		return fmt.Errorf("write resolved config: %w", err)
	}

	// Restart systemd-resolved
	if err := run("systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("restart systemd-resolved: %w", err)
	}

	// Fix /etc/resolv.conf to use real DNS (not the stub)
	_ = os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8\nnameserver 1.1.1.1\n"), 0644)

	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}
