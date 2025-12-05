package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatih/color"
	"github.com/lance0/hookshot/internal/client"
	"github.com/lance0/hookshot/internal/server"
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
		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")
		publicURL, _ := cmd.Flags().GetString("public-url")
		maxRequests, _ := cmd.Flags().GetInt("max-requests")

		cfg := server.Config{
			Port:        port,
			Host:        host,
			PublicURL:   publicURL,
			MaxRequests: maxRequests,
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
		serverURL, _ := cmd.Flags().GetString("server")
		target, _ := cmd.Flags().GetString("target")
		tunnelID, _ := cmd.Flags().GetString("id")

		if serverURL == "" {
			return fmt.Errorf("--server is required")
		}
		if target == "" {
			target = "http://localhost:3000"
		}

		cfg := client.Config{
			ServerURL: serverURL,
			Target:    target,
			TunnelID:  tunnelID,
		}

		c := client.New(cfg)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle interrupt
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")
			cancel()
		}()

		return c.Run(ctx)
	},
}

// Requests command
var requestsCmd = &cobra.Command{
	Use:   "requests",
	Short: "List recent requests for a tunnel",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		tunnelID, _ := cmd.Flags().GetString("tunnel")

		if serverURL == "" {
			return fmt.Errorf("--server is required")
		}
		if tunnelID == "" {
			return fmt.Errorf("--tunnel is required")
		}

		url := fmt.Sprintf("%s/api/tunnels/%s/requests", serverURL, tunnelID)
		resp, err := http.Get(url)
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
		resp, err := http.Post(url, "application/json", nil)
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
	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	serverCmd.Flags().String("host", "0.0.0.0", "Host to bind to")
	serverCmd.Flags().String("public-url", "", "Public URL for the server (for display)")
	serverCmd.Flags().Int("max-requests", 100, "Maximum requests to store per tunnel")

	// Client flags
	clientCmd.Flags().StringP("server", "s", "", "Server URL (e.g., https://relay.example.com)")
	clientCmd.Flags().StringP("target", "t", "http://localhost:3000", "Local target URL")
	clientCmd.Flags().String("id", "", "Requested tunnel ID (optional)")
	clientCmd.MarkFlagRequired("server")

	// Requests flags
	requestsCmd.Flags().StringP("server", "s", "", "Server URL")
	requestsCmd.Flags().String("tunnel", "", "Tunnel ID")
	requestsCmd.MarkFlagRequired("server")
	requestsCmd.MarkFlagRequired("tunnel")

	// Replay flags
	replayCmd.Flags().StringP("server", "s", "", "Server URL")
	replayCmd.Flags().String("tunnel", "", "Tunnel ID")
	replayCmd.Flags().StringP("request", "r", "", "Request ID to replay")
	replayCmd.MarkFlagRequired("server")
	replayCmd.MarkFlagRequired("tunnel")
	replayCmd.MarkFlagRequired("request")

	// Add commands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	rootCmd.AddCommand(requestsCmd)
	rootCmd.AddCommand(replayCmd)
}
