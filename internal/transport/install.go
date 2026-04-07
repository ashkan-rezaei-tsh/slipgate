package transport

import (
	"github.com/ashkan-rezaei-tsh/slipgate/internal/binary"
	"github.com/ashkan-rezaei-tsh/slipgate/internal/config"
)

// EnsureInstalled downloads the binary for a transport if not already present.
func EnsureInstalled(transport string) error {
	bin, ok := config.TransportBinaries[transport]
	if !ok {
		return nil
	}
	return binary.EnsureInstalled(bin)
}
