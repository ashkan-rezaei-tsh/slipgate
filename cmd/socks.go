package cmd

import (
	"github.com/ashkan-rezaei-tsh/slipgate/internal/proxy"
	"github.com/spf13/cobra"
)

func init() {
	socksCmd := &cobra.Command{
		Use:   "socks",
		Short: "Built-in SOCKS5 proxy",
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the SOCKS5 proxy (used by systemd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("addr")
			port, _ := cmd.Flags().GetInt("port")
			user, _ := cmd.Flags().GetString("user")
			pass, _ := cmd.Flags().GetString("pass")
			return proxy.Serve(addr, port, user, pass)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	serveCmd.Flags().String("addr", "127.0.0.1", "Listen address")
	serveCmd.Flags().Int("port", 1080, "Listen port")
	serveCmd.Flags().String("user", "", "Username for auth")
	serveCmd.Flags().String("pass", "", "Password for auth")

	socksCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(socksCmd)
}
