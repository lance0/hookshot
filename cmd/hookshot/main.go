package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fatih/color"
	"github.com/lance0/hookshot/internal/client"
	"github.com/lance0/hookshot/internal/config"
	"github.com/lance0/hookshot/internal/server"
	"github.com/lance0/hookshot/internal/tui"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "hookshot",
	Short: "A self-hostable webhook relay for local development",
	Long: `Hookshot forwards webhooks from a public server to your local machine.

Run 'hookshot server' on your VPS, then 'hookshot client' locally
to receive webhooks at localhost.`,
	Version: version,
}

// Server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the relay server",
	Long:  `Run the hookshot relay server that receives webhooks and forwards them to connected clients.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")

		// Load config file if specified or found
		var fileCfg *config.Config
		if configFile == "" {
			configFile = config.FindConfigFile()
		}
		if configFile != "" {
			var err error
			fileCfg, err = config.Load(configFile)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
		}

		// Get values from flags, fall back to config file
		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")
		publicURL, _ := cmd.Flags().GetString("public-url")
		maxRequests, _ := cmd.Flags().GetInt("max-requests")
		token, _ := cmd.Flags().GetString("token")
		tlsCert, _ := cmd.Flags().GetString("tls-cert")
		tlsKey, _ := cmd.Flags().GetString("tls-key")

		// Apply config file values if flags weren't set
		if fileCfg != nil {
			if !cmd.Flags().Changed("port") && fileCfg.Server.Port != 0 {
				port = fileCfg.Server.Port
			}
			if !cmd.Flags().Changed("host") && fileCfg.Server.Host != "" {
				host = fileCfg.Server.Host
			}
			if !cmd.Flags().Changed("public-url") && fileCfg.Server.PublicURL != "" {
				publicURL = fileCfg.Server.PublicURL
			}
			if !cmd.Flags().Changed("max-requests") && fileCfg.Server.MaxRequests != 0 {
				maxRequests = fileCfg.Server.MaxRequests
			}
			if !cmd.Flags().Changed("token") && fileCfg.Server.Token != "" {
				token = fileCfg.Server.Token
			}
			if !cmd.Flags().Changed("tls-cert") && fileCfg.Server.TLSCert != "" {
				tlsCert = fileCfg.Server.TLSCert
			}
			if !cmd.Flags().Changed("tls-key") && fileCfg.Server.TLSKey != "" {
				tlsKey = fileCfg.Server.TLSKey
			}
		}

		cfg := server.Config{
			Port:        port,
			Host:        host,
			PublicURL:   publicURL,
			MaxRequests: maxRequests,
			Token:       token,
			TLSCert:     tlsCert,
			TLSKey:      tlsKey,
		}

		srv := server.New(cfg)
		return srv.Run()
	},
}

// Client command
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Connect to a relay server",
	Long:  `Connect to a hookshot relay server and forward webhooks to a local target.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")

		// Load config file if specified or found
		var fileCfg *config.Config
		if configFile == "" {
			configFile = config.FindConfigFile()
		}
		if configFile != "" {
			var err error
			fileCfg, err = config.Load(configFile)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
		}

		serverURL, _ := cmd.Flags().GetString("server")
		target, _ := cmd.Flags().GetString("target")
		tunnelID, _ := cmd.Flags().GetString("id")
		token, _ := cmd.Flags().GetString("token")
		verbose, _ := cmd.Flags().GetBool("verbose")
		tuiMode, _ := cmd.Flags().GetBool("tui")

		var routes []client.Route

		// Apply config file values if flags weren't set
		if fileCfg != nil {
			if !cmd.Flags().Changed("server") && fileCfg.Client.Server != "" {
				serverURL = fileCfg.Client.Server
			}
			if !cmd.Flags().Changed("target") && fileCfg.Client.Target != "" {
				target = fileCfg.Client.Target
			}
			if !cmd.Flags().Changed("id") && fileCfg.Client.TunnelID != "" {
				tunnelID = fileCfg.Client.TunnelID
			}
			if !cmd.Flags().Changed("token") && fileCfg.Client.Token != "" {
				token = fileCfg.Client.Token
			}
			if !cmd.Flags().Changed("verbose") && fileCfg.Client.Verbose {
				verbose = fileCfg.Client.Verbose
			}
			// Load routes from config
			for _, r := range fileCfg.Client.Routes {
				routes = append(routes, client.Route{
					Path:   r.Path,
					Target: r.Target,
				})
			}
		}

		if serverURL == "" {
			return fmt.Errorf("--server is required (or set in config file)")
		}
		if target == "" && len(routes) == 0 {
			target = "http://localhost:3000"
		}

		cfg := client.Config{
			ServerURL: serverURL,
			Target:    target,
			Routes:    routes,
			TunnelID:  tunnelID,
			Token:     token,
			Verbose:   verbose,
			TUIMode:   tuiMode,
		}

		c := client.New(cfg)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle interrupt
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			if !tuiMode {
				fmt.Println("\nShutting down...")
			}
			cancel()
		}()

		if tuiMode {
			// Run with TUI
			return runWithTUI(ctx, c, cancel)
		}

		return c.Run(ctx)
	},
}

// runWithTUI runs the client with the TUI
func runWithTUI(ctx context.Context, c *client.Client, cancel context.CancelFunc) error {
	// Create TUI model
	m := tui.NewModel()

	// Set up TUI channels
	c.SetTUIChannels(m.RequestChannel(), m.ConnectionChannel())

	// Run client in background
	go func() {
		if err := c.Run(ctx); err != nil && ctx.Err() == nil {
			// Client error - could send to TUI
		}
	}()

	// Run TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// TUI exited, cancel context
	cancel()
	return nil
}

// Requests command
var requestsCmd = &cobra.Command{
	Use:   "requests",
	Short: "List recent requests for a tunnel",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		tunnelID, _ := cmd.Flags().GetString("tunnel")
		token, _ := cmd.Flags().GetString("token")

		if serverURL == "" {
			return fmt.Errorf("--server is required")
		}
		if tunnelID == "" {
			return fmt.Errorf("--tunnel is required")
		}

		url := fmt.Sprintf("%s/api/tunnels/%s/requests", serverURL, tunnelID)
		req, _ := http.NewRequest("GET", url, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to fetch requests: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server returned %d", resp.StatusCode)
		}

		var requests []struct {
			ID         string `json:"id"`
			Method     string `json:"method"`
			Path       string `json:"path"`
			Timestamp  string `json:"timestamp"`
			StatusCode int    `json:"status_code"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&requests); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if len(requests) == 0 {
			fmt.Println("No requests found")
			return nil
		}

		fmt.Printf("Recent requests for tunnel %s:\n\n", color.CyanString(tunnelID))
		for _, r := range requests {
			statusColor := color.GreenString
			if r.StatusCode >= 400 {
				statusColor = color.RedString
			} else if r.StatusCode >= 300 {
				statusColor = color.YellowString
			}

			status := "-"
			if r.StatusCode > 0 {
				status = statusColor("%d", r.StatusCode)
			}

			fmt.Printf("  %s  %-7s %s  %s\n",
				color.HiBlackString(r.ID),
				color.YellowString(r.Method),
				r.Path,
				status,
			)
		}
		return nil
	},
}

// Replay command
var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a previous request",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		tunnelID, _ := cmd.Flags().GetString("tunnel")
		requestID, _ := cmd.Flags().GetString("request")
		token, _ := cmd.Flags().GetString("token")

		if serverURL == "" {
			return fmt.Errorf("--server is required")
		}
		if tunnelID == "" {
			return fmt.Errorf("--tunnel is required")
		}
		if requestID == "" {
			return fmt.Errorf("--request is required")
		}

		url := fmt.Sprintf("%s/api/tunnels/%s/requests/%s/replay", serverURL, tunnelID, requestID)
		req, _ := http.NewRequest("POST", url, nil)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to replay request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("replay failed with status %d", resp.StatusCode)
		}

		var result struct {
			RequestID  string `json:"request_id"`
			StatusCode int    `json:"status_code"`
			BodyLength int    `json:"body_length"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		fmt.Printf("Replayed request %s\n", color.CyanString(requestID))
		fmt.Printf("  New request ID: %s\n", color.CyanString(result.RequestID))
		fmt.Printf("  Status: %s\n", color.GreenString("%d", result.StatusCode))
		fmt.Printf("  Body length: %d bytes\n", result.BodyLength)

		return nil
	},
}

func init() {
	// Server flags
	serverCmd.Flags().StringP("config", "c", "", "Config file path")
	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	serverCmd.Flags().String("host", "0.0.0.0", "Host to bind to")
	serverCmd.Flags().String("public-url", "", "Public URL for the server (for display)")
	serverCmd.Flags().Int("max-requests", 100, "Maximum requests to store per tunnel")
	serverCmd.Flags().String("token", "", "Auth token (required for client connections if set)")
	serverCmd.Flags().String("tls-cert", "", "Path to TLS certificate file")
	serverCmd.Flags().String("tls-key", "", "Path to TLS key file")

	// Client flags
	clientCmd.Flags().StringP("config", "c", "", "Config file path")
	clientCmd.Flags().StringP("server", "s", "", "Server URL (e.g., https://relay.example.com)")
	clientCmd.Flags().StringP("target", "t", "http://localhost:3000", "Local target URL")
	clientCmd.Flags().String("id", "", "Requested tunnel ID (optional)")
	clientCmd.Flags().String("token", "", "Auth token for server")
	clientCmd.Flags().BoolP("verbose", "v", false, "Show request/response bodies")
	clientCmd.Flags().Bool("tui", false, "Enable interactive TUI mode")

	// Requests flags
	requestsCmd.Flags().StringP("server", "s", "", "Server URL")
	requestsCmd.Flags().String("tunnel", "", "Tunnel ID")
	requestsCmd.Flags().String("token", "", "Auth token for server")
	requestsCmd.MarkFlagRequired("server")
	requestsCmd.MarkFlagRequired("tunnel")

	// Replay flags
	replayCmd.Flags().StringP("server", "s", "", "Server URL")
	replayCmd.Flags().String("tunnel", "", "Tunnel ID")
	replayCmd.Flags().StringP("request", "r", "", "Request ID to replay")
	replayCmd.Flags().String("token", "", "Auth token for server")
	replayCmd.MarkFlagRequired("server")
	replayCmd.MarkFlagRequired("tunnel")
	replayCmd.MarkFlagRequired("request")

	// Add commands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	rootCmd.AddCommand(requestsCmd)
	rootCmd.AddCommand(replayCmd)
}
