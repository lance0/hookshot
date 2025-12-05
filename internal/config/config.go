package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the full configuration file
type Config struct {
	Server ServerConfig `yaml:"server,omitempty"`
	Client ClientConfig `yaml:"client,omitempty"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port        int    `yaml:"port,omitempty"`
	Host        string `yaml:"host,omitempty"`
	PublicURL   string `yaml:"public_url,omitempty"`
	MaxRequests int    `yaml:"max_requests,omitempty"`
	Token       string `yaml:"token,omitempty"`
	TLSCert     string `yaml:"tls_cert,omitempty"`
	TLSKey      string `yaml:"tls_key,omitempty"`
}

// ClientConfig holds client configuration
type ClientConfig struct {
	Server   string   `yaml:"server,omitempty"`
	Target   string   `yaml:"target,omitempty"`
	TunnelID string   `yaml:"tunnel_id,omitempty"`
	Token    string   `yaml:"token,omitempty"`
	Verbose  bool     `yaml:"verbose,omitempty"`
	Routes   []Route  `yaml:"routes,omitempty"` // Multiple targets by path
}

// Route maps a path prefix to a target
type Route struct {
	Path   string `yaml:"path"`   // Path prefix to match (e.g., "/api")
	Target string `yaml:"target"` // Target URL (e.g., "http://localhost:3000")
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// FindConfigFile looks for hookshot.yaml in common locations
func FindConfigFile() string {
	// Check current directory
	if _, err := os.Stat("hookshot.yaml"); err == nil {
		return "hookshot.yaml"
	}
	if _, err := os.Stat("hookshot.yml"); err == nil {
		return "hookshot.yml"
	}

	// Check home directory
	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".config", "hookshot", "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
		configPath = filepath.Join(home, ".hookshot.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	return ""
}

// MatchRoute finds the best matching route for a path
func (c *ClientConfig) MatchRoute(path string) string {
	if len(c.Routes) == 0 {
		return c.Target
	}

	// Find longest matching prefix
	var bestMatch Route
	bestLen := -1

	for _, route := range c.Routes {
		if strings.HasPrefix(path, route.Path) && len(route.Path) > bestLen {
			bestMatch = route
			bestLen = len(route.Path)
		}
	}

	if bestLen >= 0 {
		return bestMatch.Target
	}

	// Fall back to default target
	return c.Target
}

// Validate validates the server configuration
func (c *ServerConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 0-65535)", c.Port)
	}

	if c.PublicURL != "" {
		if _, err := url.Parse(c.PublicURL); err != nil {
			return fmt.Errorf("invalid public_url: %w", err)
		}
	}

	// TLS cert and key must both be set or both be empty
	if (c.TLSCert != "") != (c.TLSKey != "") {
		return fmt.Errorf("both tls_cert and tls_key must be set, or neither")
	}

	// If TLS files are specified, verify they exist
	if c.TLSCert != "" {
		if _, err := os.Stat(c.TLSCert); err != nil {
			return fmt.Errorf("tls_cert file not found: %s", c.TLSCert)
		}
	}
	if c.TLSKey != "" {
		if _, err := os.Stat(c.TLSKey); err != nil {
			return fmt.Errorf("tls_key file not found: %s", c.TLSKey)
		}
	}

	if c.MaxRequests < 0 {
		return fmt.Errorf("invalid max_requests: %d (must be >= 0)", c.MaxRequests)
	}

	return nil
}

// Validate validates the client configuration
func (c *ClientConfig) Validate() error {
	if c.Server != "" {
		u, err := url.Parse(c.Server)
		if err != nil {
			return fmt.Errorf("invalid server URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ws" && u.Scheme != "wss" {
			return fmt.Errorf("invalid server URL scheme: %s (must be http, https, ws, or wss)", u.Scheme)
		}
	}

	if c.Target != "" {
		if _, err := url.Parse(c.Target); err != nil {
			return fmt.Errorf("invalid target URL: %w", err)
		}
	}

	// Validate routes
	for i, route := range c.Routes {
		if route.Path == "" {
			return fmt.Errorf("route %d: path is required", i)
		}
		if route.Target == "" {
			return fmt.Errorf("route %d: target is required", i)
		}
		if _, err := url.Parse(route.Target); err != nil {
			return fmt.Errorf("route %d: invalid target URL: %w", i, err)
		}
	}

	return nil
}

// Example config file content
const ExampleConfig = `# Hookshot configuration file

# Server configuration (for 'hookshot server')
server:
  port: 8080
  host: 0.0.0.0
  public_url: https://relay.example.com
  max_requests: 100
  token: your-secret-token
  # tls_cert: /path/to/cert.pem
  # tls_key: /path/to/key.pem

# Client configuration (for 'hookshot client')
client:
  server: https://relay.example.com
  tunnel_id: my-project
  token: your-secret-token
  verbose: false

  # Single target (simple mode)
  target: http://localhost:3000

  # OR multiple targets (route by path)
  # routes:
  #   - path: /api
  #     target: http://localhost:3000
  #   - path: /webhooks
  #     target: http://localhost:4000
  #   - path: /
  #     target: http://localhost:8080
`
