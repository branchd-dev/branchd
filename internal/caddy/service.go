package caddy

import (
	"fmt"
	"os"
	"os/exec"
	"text/template"

	"github.com/rs/zerolog"
)

const (
	// CaddyfilePath is the location of the Caddyfile on disk
	CaddyfilePath = "/etc/caddy/Caddyfile"

	// CaddyfileTemplate is the template for generating Caddyfile
	CaddyfileTemplate = `# Branchd web UI and API reverse proxy
{
    # Global options
    # Admin API on localhost only (required for zero-downtime reloads)
    admin localhost:2019
}

# HTTP to HTTPS redirect
:80 {
    redir https://{host}{uri} permanent
}

# HTTPS configuration
{{if .Domain}}{{.Domain}}{{else}}:443{{end}} {
    {{if .Domain}}
    # Let's Encrypt with custom domain
    tls {{.LetsEncryptEmail}}
    {{else}}
    # Self-signed certificate (internal CA)
    tls internal
    {{end}}

    # Logging (to stdout, captured by systemd journal)
    log {
        format json
    }

    # API endpoints - reverse proxy to Go server
    handle /api/* {
        reverse_proxy localhost:8080
    }

    # Health check endpoint - proxy to Go server
    handle /health {
        reverse_proxy localhost:8080
    }

    # Static web UI files
    handle /* {
        root * /var/www/branchd
        try_files {path} /index.html
        file_server

        # Security headers
        header {
            X-Content-Type-Options "nosniff"
            X-Frame-Options "DENY"
            X-XSS-Protection "1; mode=block"
            Referrer-Policy "strict-origin-when-cross-origin"
            Strict-Transport-Security "max-age=31536000; includeSubDomains"
        }
    }

    # Error handling
    handle_errors {
        respond "{http.error.status_code} {http.error.status_text}"
    }
}
`
)

// Service handles Caddyfile generation and reload operations
type Service struct {
	logger zerolog.Logger
	tmpl   *template.Template
}

// Config represents the configuration needed to generate a Caddyfile
type Config struct {
	Domain           string // Custom domain (e.g., "db.company.com"), empty for self-signed
	LetsEncryptEmail string // Email for Let's Encrypt, required if Domain is set
}

// NewService creates a new Caddy service
func NewService(logger zerolog.Logger) (*Service, error) {
	tmpl, err := template.New("caddyfile").Parse(CaddyfileTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Caddyfile template: %w", err)
	}

	return &Service{
		logger: logger,
		tmpl:   tmpl,
	}, nil
}

// GenerateAndReload generates a new Caddyfile and reloads Caddy
func (s *Service) GenerateAndReload(cfg Config) error {
	// Validate configuration
	if cfg.Domain != "" && cfg.LetsEncryptEmail == "" {
		return fmt.Errorf("lets_encrypt_email is required when domain is set")
	}

	// Generate Caddyfile content
	content, err := s.generateCaddyfile(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate Caddyfile: %w", err)
	}

	// Write to temporary file first (atomic write)
	tmpPath := CaddyfilePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temporary Caddyfile: %w", err)
	}

	// Validate Caddyfile syntax
	if err := s.validateCaddyfile(tmpPath); err != nil {
		os.Remove(tmpPath) // Clean up invalid file
		return fmt.Errorf("generated Caddyfile is invalid: %w", err)
	}

	// Move temporary file to actual location
	if err := os.Rename(tmpPath, CaddyfilePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to move Caddyfile to final location: %w", err)
	}

	s.logger.Info().
		Str("domain", cfg.Domain).
		Str("path", CaddyfilePath).
		Msg("Caddyfile generated successfully")

	// Reload Caddy (zero downtime)
	if err := s.reloadCaddy(); err != nil {
		return fmt.Errorf("failed to reload Caddy: %w", err)
	}

	s.logger.Info().Msg("Caddy reloaded successfully")
	return nil
}

// generateCaddyfile generates Caddyfile content from template
func (s *Service) generateCaddyfile(cfg Config) (string, error) {
	var buf []byte
	writer := &bufWriter{buf: buf}

	if err := s.tmpl.Execute(writer, cfg); err != nil {
		return "", err
	}

	return string(writer.buf), nil
}

// validateCaddyfile validates Caddyfile syntax using caddy validate
func (s *Service) validateCaddyfile(path string) error {
	cmd := exec.Command("caddy", "validate", "--config", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("validation failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// reloadCaddy reloads Caddy configuration without downtime
func (s *Service) reloadCaddy() error {
	cmd := exec.Command("caddy", "reload", "--config", CaddyfilePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// bufWriter is a simple writer that appends to a byte slice
type bufWriter struct {
	buf []byte
}

func (w *bufWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}
