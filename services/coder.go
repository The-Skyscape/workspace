package services

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// CoderService manages a single global code-server instance for admin use
type CoderService struct {
	host      containers.Host
	port      int
	running   bool
	adminOnly bool
}

var (
	// Global coder instance
	Coder = &CoderService{
		host:      containers.Local(),
		port:      8080, // Fixed port for the single instance
		adminOnly: true,
	}
)

// Start launches the global coder service
func (c *CoderService) Start() error {
	if c.running {
		log.Println("Coder service already running")
		return nil
	}

	log.Println("Starting global coder service on port", c.port)

	// Prepare directories
	dataDir := fmt.Sprintf("%s/services/coder", database.DataDir())
	configDir := fmt.Sprintf("%s/.config", dataDir)
	projectDir := fmt.Sprintf("%s/project", dataDir)
	
	// Create directories
	prepareScript := fmt.Sprintf(`
		mkdir -p %s %s
		chmod -R 777 %s
	`, configDir, projectDir, dataDir)

	if err := c.host.Exec("bash", "-c", prepareScript); err != nil {
		return errors.Wrap(err, "failed to prepare coder directories")
	}

	// Create service configuration
	service := &containers.Service{
		Host:  c.host,
		Name:  "skyscape-coder",
		Image: "codercom/code-server:latest",
		Command: fmt.Sprintf("--auth none --bind-addr 0.0.0.0:%d", c.port),
		Network: "host", // Use host network for easier access to services
		RestartPolicy: "always", // Restart on failure or reboot
		Mounts: map[string]string{
			configDir:  "/home/coder/.config",
			projectDir: "/home/coder/project",
			// Mount the entire workspace directory for full access
			database.DataDir(): "/workspace",
		},
		// No port mapping needed with host network
		Env: map[string]string{
			"PORT":         strconv.Itoa(c.port),
			"SERVICE_TYPE": "coder",
			"ADMIN_ONLY":   "true",
		},
	}

	// Launch the service
	if err := containers.Launch(c.host, service); err != nil {
		return errors.Wrap(err, "failed to launch coder service")
	}

	c.running = true
	log.Println("Coder service started successfully")
	return nil
}

// Stop stops the global coder service
func (c *CoderService) Stop() error {
	if !c.running {
		log.Println("Coder service not running")
		return nil
	}

	service := c.getService()
	if err := service.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop coder service")
	}

	c.running = false
	log.Println("Coder service stopped")
	return nil
}

// IsRunning checks if the service is running
func (c *CoderService) IsRunning() bool {
	service := c.getService()
	return service.IsRunning()
}

// GetPort returns the port the service is running on
func (c *CoderService) GetPort() int {
	return c.port
}

// IsAdminOnly returns whether this service is restricted to admins
func (c *CoderService) IsAdminOnly() bool {
	return c.adminOnly
}

// getService returns the container service configuration
func (c *CoderService) getService() *containers.Service {
	return &containers.Service{
		Host: c.host,
		Name: "skyscape-coder",
	}
}

// HTTPRequest makes an HTTP request to the service
func (c *CoderService) HTTPRequest(method, path string, timeout time.Duration) (*http.Response, error) {
	if !c.IsRunning() {
		return nil, errors.New("service is not running")
	}
	
	url := fmt.Sprintf("http://localhost:%d%s", c.port, path)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	
	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

// HealthCheck performs a health check on the service
func (c *CoderService) HealthCheck() error {
	resp, err := c.HTTPRequest("GET", "/healthz", 2*time.Second)
	if err != nil {
		// Try root path as fallback
		resp, err = c.HTTPRequest("GET", "/", 2*time.Second)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		// Any non-5xx response means the service is alive
		return nil
	}
	
	return fmt.Errorf("unhealthy response: %d", resp.StatusCode)
}

// WaitForReady waits for the coder service to be ready
func (c *CoderService) WaitForReady(timeout time.Duration) error {
	start := time.Now()
	for {
		if c.IsRunning() {
			// Check if the service is actually responding
			if err := c.HealthCheck(); err == nil {
				return nil
			}
			
			// If we're within the first 10 seconds, keep trying
			if time.Since(start) < 10*time.Second {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			
			// After 10 seconds, if container is running, assume it's ready
			// (code-server might not respond immediately)
			return nil
		}
		
		if time.Since(start) > timeout {
			return errors.New("timeout waiting for coder service to be ready")
		}
		
		time.Sleep(1 * time.Second)
	}
}

// GetProxyPath returns the path prefix for proxying requests
func (c *CoderService) GetProxyPath() string {
	return "/coder/"
}

// StripProxyPath strips the proxy path from a request path
func (c *CoderService) StripProxyPath(path string) string {
	return strings.TrimPrefix(path, c.GetProxyPath())
}

// Init initializes the coder service if not already running
// This is called during application startup
func (c *CoderService) Init() error {
	// Check if service already exists and is running
	if c.IsRunning() {
		log.Println("Coder service already running")
		c.running = true
		return nil
	}

	// Start the service
	log.Println("Initializing coder service...")
	if err := c.Start(); err != nil {
		return errors.Wrap(err, "failed to initialize coder service")
	}

	return nil
}