package services

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/authentication"
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
		Host:          c.host,
		Name:          "skyscape-coder",
		Image:         "codercom/code-server:latest",
		Command:       fmt.Sprintf("--auth none --bind-addr 0.0.0.0:%d", c.port),
		Network:       "host",   // Use host network for easier access to services
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

// CloneRepository creates a working copy of the repository in the Code Server
func (c *CoderService) CloneRepository(repo *models.Repository, user *authentication.User) error {
	// Create access token for cloning
	token, err := models.CreateAccessToken(repo.ID, user.ID, 100*365*24*time.Hour)
	if err != nil {
		return errors.Wrap(err, "failed to create access token")
	}

	// Build clone URL for localhost (port 80 in production)
	cloneURL := fmt.Sprintf("http://%s:%s@localhost/repo/%s", token.ID, token.Token, repo.ID)

	// Get user details for git config
	gitUserName := user.Email // Default to email
	gitUserEmail := user.Email
	if user.Name != "" {
		gitUserName = user.Name
	}

	// Escape repository name for shell
	escapedName := strings.ReplaceAll(repo.Name, "'", "'\\''")

	// Clone or update repository (safer - preserves local changes)
	cloneCmd := fmt.Sprintf(`
		cd /home/coder/project && 
		if [ -d '%s/.git' ]; then
			echo "Repository exists, updating..." &&
			cd '%s' &&
			git fetch origin || true
		else
			echo "Cloning repository..." &&
			git clone '%s' '%s' &&
			cd '%s' &&
			git config user.name "%s" &&
			git config user.email "%s"
		fi
	`, escapedName, escapedName, cloneURL, escapedName, escapedName,
		gitUserName, gitUserEmail)

	cmd := exec.Command("docker", "exec", "skyscape-coder", "bash", "-c", cloneCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "clone failed: %s", string(output))
	}

	log.Printf("Repository %s cloned/updated in Code Server: %s", repo.ID, string(output))
	return nil
}

// UpdateRepository updates the working copy after a git push
func (c *CoderService) UpdateRepository(repoID string) error {
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return errors.Wrap(err, "repository not found")
	}

	user, err := models.Auth.Users.Get(repo.UserID)
	if err != nil {
		return errors.Wrap(err, "failed to get repository owner")
	}

	escapedName := strings.ReplaceAll(repo.Name, "'", "'\\''")

	// Check if repository exists in Code Server
	checkCmd := fmt.Sprintf(`
		cd /home/coder/project && 
		[ -d '%s/.git' ] && echo "exists" || echo "not-exists"
	`, escapedName)

	cmd := exec.Command("docker", "exec", "skyscape-coder", "bash", "-c", checkCmd)
	output, _ := cmd.CombinedOutput()

	if strings.TrimSpace(string(output)) == "not-exists" {
		// Repository doesn't exist, clone it
		return c.CloneRepository(repo, user)
	}

	// Update existing repository (safer - stash local changes)
	updateCmd := fmt.Sprintf(`
		cd /home/coder/project/%s &&
		git stash save "Auto-stash before update" &&
		git fetch origin &&
		git pull origin $(git symbolic-ref --short HEAD 2>/dev/null || echo 'master') &&
		git stash pop || true
	`, escapedName)

	cmd = exec.Command("docker", "exec", "skyscape-coder", "bash", "-c", updateCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Update failed for %s: %s", repoID, string(output))
		// If update fails, try to re-clone
		return c.CloneRepository(repo, user)
	}

	log.Printf("Repository %s updated in Code Server", repoID)
	return nil
}

// RemoveRepository removes the working copy when a repository is deleted
func (c *CoderService) RemoveRepository(repoID string) error {
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		// Repository might already be deleted, use the ID as name
		repo = &models.Repository{Name: repoID}
	}

	escapedName := strings.ReplaceAll(repo.Name, "'", "'\\''")
	removeCmd := fmt.Sprintf(`rm -rf '/home/coder/project/%s'`, escapedName)

	cmd := exec.Command("docker", "exec", "skyscape-coder", "bash", "-c", removeCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Failed to remove %s from Code Server: %v - %s", repoID, err, string(output))
		// Don't return error as this is cleanup
	}

	log.Printf("Repository %s removed from Code Server", repoID)
	return nil
}
