package config

import (
	"fmt"
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
