package services

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// CoderConfig holds configuration for the coder service
type CoderConfig struct {
	Port          int
	ContainerName string
	DataDir       string
	AdminOnly     bool
	Network       string
	Image         string
}

// CoderService manages a single global code-server instance for admin use
type CoderService struct {
	config  *CoderConfig
	service *containers.Service
	mu      sync.RWMutex
}

// CoderStatus represents the current status of the coder service
type CoderStatus struct {
	Running   bool
	Port      int
	AdminOnly bool
	URL       string
	Health    string
}

var (
	// Global coder instance
	Coder = NewCoderService()
)

// NewCoderService creates a new coder service with default configuration
func NewCoderService() *CoderService {
	return &CoderService{
		config: &CoderConfig{
			Port:          8080,
			ContainerName: "skyscape-coder",
			AdminOnly:     true,
			Network:       "host",
			Image:         "codercom/code-server:latest",
		},
	}
}

// Init initializes the coder service if not already running
// This is called during application startup
func (c *CoderService) Init() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if service already exists and is running
	existing := containers.Local().Service(c.config.ContainerName)
	if existing != nil && existing.IsRunning() {
		log.Println("Coder service already running")
		c.service = existing
		return nil
	}

	// Start the service
	log.Println("Initializing coder service...")
	return c.start()
}

// Start launches the global coder service
func (c *CoderService) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	return c.start()
}

// start is the internal start method (must be called with lock held)
func (c *CoderService) start() error {
	// Check if already running
	if c.service != nil && c.service.IsRunning() {
		log.Println("Coder service already running")
		return nil
	}

	log.Printf("Starting global coder service on port %d", c.config.Port)

	// Prepare directories
	if c.config.DataDir == "" {
		c.config.DataDir = fmt.Sprintf("%s/services/coder", database.DataDir())
	}
	
	configDir := fmt.Sprintf("%s/.config", c.config.DataDir)
	projectDir := fmt.Sprintf("%s/project", c.config.DataDir)

	// Create directories using host
	prepareScript := fmt.Sprintf(`
		mkdir -p %s %s
		chmod -R 777 %s
	`, configDir, projectDir, c.config.DataDir)

	host := containers.Local()
	if err := host.Exec("bash", "-c", prepareScript); err != nil {
		return errors.Wrap(err, "failed to prepare coder directories")
	}

	// Create service configuration
	c.service = c.createServiceConfig()

	// Launch the service
	if err := containers.Launch(host, c.service); err != nil {
		return errors.Wrap(err, "failed to launch coder service")
	}

	// Wait for service to be ready
	if err := c.service.WaitForReady(30*time.Second, c.healthCheck); err != nil {
		log.Printf("Warning: Coder service may not be fully ready: %v", err)
	}

	log.Println("Coder service started successfully")
	return nil
}

// Stop stops the global coder service
func (c *CoderService) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.service == nil {
		log.Println("Coder service not initialized")
		return nil
	}

	if !c.service.IsRunning() {
		log.Println("Coder service not running")
		return nil
	}

	if err := c.service.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop coder service")
	}

	log.Println("Coder service stopped")
	return nil
}

// Restart restarts the coder service
func (c *CoderService) Restart() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.service == nil {
		return errors.New("coder service not initialized")
	}

	if err := c.service.Restart(); err != nil {
		return errors.Wrap(err, "failed to restart coder service")
	}

	// Wait for service to be ready after restart
	if err := c.service.WaitForReady(30*time.Second, c.healthCheck); err != nil {
		log.Printf("Warning: Coder service may not be fully ready after restart: %v", err)
	}

	log.Println("Coder service restarted")
	return nil
}

// IsRunning checks if the service is running
func (c *CoderService) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.service == nil {
		// Try to get existing service
		existing := containers.Local().Service(c.config.ContainerName)
		if existing != nil {
			c.service = existing
		}
	}

	return c.service != nil && c.service.IsRunning()
}

// GetPort returns the port the service is running on
func (c *CoderService) GetPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.config.Port
}

// IsAdminOnly returns whether this service is restricted to admins
func (c *CoderService) IsAdminOnly() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.config.AdminOnly
}

// GetStatus returns the current status of the coder service
func (c *CoderService) GetStatus() *CoderStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := &CoderStatus{
		Running:   c.IsRunning(),
		Port:      c.config.Port,
		AdminOnly: c.config.AdminOnly,
		URL:       "/coder/",
		Health:    "unknown",
	}

	if status.Running {
		if err := c.healthCheck(); err == nil {
			status.Health = "healthy"
		} else {
			status.Health = "unhealthy"
		}
	} else {
		status.Health = "stopped"
	}

	return status
}

// GetLogs retrieves the service logs
func (c *CoderService) GetLogs(tail int) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.service == nil {
		return "", errors.New("service not initialized")
	}

	return c.service.GetLogs(tail)
}


// createServiceConfig creates the container service configuration
func (c *CoderService) createServiceConfig() *containers.Service {
	if c.config.DataDir == "" {
		c.config.DataDir = fmt.Sprintf("%s/services/coder", database.DataDir())
	}
	
	configDir := fmt.Sprintf("%s/.config", c.config.DataDir)
	projectDir := fmt.Sprintf("%s/project", c.config.DataDir)

	return &containers.Service{
		Host:          containers.Local(),
		Name:          c.config.ContainerName,
		Image:         c.config.Image,
		Command:       fmt.Sprintf("--auth none --bind-addr 0.0.0.0:%d", c.config.Port),
		Network:       c.config.Network,
		RestartPolicy: "always", // Restart on failure or reboot
		Mounts: map[string]string{
			configDir:          "/home/coder/.config",
			projectDir:         "/home/coder/project",
			database.DataDir(): "/workspace", // Mount entire workspace for full access
		},
		Env: map[string]string{
			"PORT":         strconv.Itoa(c.config.Port),
			"SERVICE_TYPE": "coder",
			"ADMIN_ONLY":   strconv.FormatBool(c.config.AdminOnly),
		},
	}
}

// healthCheck performs a health check on the service
func (c *CoderService) healthCheck() error {
	resp, err := c.httpRequest("GET", "/healthz", 2*time.Second)
	if err != nil {
		// Try root path as fallback
		resp, err = c.httpRequest("GET", "/", 2*time.Second)
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

// httpRequest makes an HTTP request to the service
func (c *CoderService) httpRequest(method, path string, timeout time.Duration) (*http.Response, error) {
	if !c.IsRunning() {
		return nil, errors.New("service is not running")
	}

	url := fmt.Sprintf("http://localhost:%d%s", c.config.Port, path)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

// WaitForReady waits for the coder service to be ready
func (c *CoderService) WaitForReady(timeout time.Duration) error {
	return c.service.WaitForReady(timeout, c.healthCheck)
}

// ====== Repository Operations ======

// CloneRepository creates a working copy of the repository in the Code Server
func (c *CoderService) CloneRepository(repo *models.Repository, user *authentication.User) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.service == nil || !c.IsRunning() {
		return errors.New("coder service is not running")
	}

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

	// Build clone command
	cloneCmd := c.buildCloneCommand(repo.Name, cloneURL, gitUserName, gitUserEmail)

	// Execute using container abstraction
	if err := c.service.ExecInContainer("bash", "-c", cloneCmd); err != nil {
		return errors.Wrapf(err, "failed to clone repository %s", repo.ID)
	}

	log.Printf("Repository %s cloned in Code Server", repo.ID)
	return nil
}

// UpdateRepository updates the working copy after a git push
func (c *CoderService) UpdateRepository(repoID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.service == nil || !c.IsRunning() {
		return errors.New("coder service is not running")
	}

	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return errors.Wrap(err, "repository not found")
	}

	user, err := models.Auth.Users.Get(repo.UserID)
	if err != nil {
		return errors.Wrap(err, "failed to get repository owner")
	}

	// Check if repository exists in Code Server
	checkCmd := c.buildCheckCommand(repo.Name)
	output, err := c.service.ExecInContainerWithOutput("bash", "-c", checkCmd)
	if err != nil {
		log.Printf("Failed to check repository existence: %v", err)
		// Try to clone if check fails
		return c.CloneRepository(repo, user)
	}

	if strings.TrimSpace(output) == "not-exists" {
		// Repository doesn't exist, clone it
		return c.CloneRepository(repo, user)
	}

	// Update existing repository
	updateCmd := c.buildUpdateCommand(repo.Name)
	if err := c.service.ExecInContainer("bash", "-c", updateCmd); err != nil {
		log.Printf("Update failed for %s: %v", repoID, err)
		// If update fails, try to re-clone
		return c.CloneRepository(repo, user)
	}

	log.Printf("Repository %s updated in Code Server", repoID)
	return nil
}

// RemoveRepository removes the working copy when a repository is deleted
func (c *CoderService) RemoveRepository(repoID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.service == nil || !c.IsRunning() {
		// Service not running, nothing to remove
		return nil
	}

	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		// Repository might already be deleted, use the ID as name
		repo = &models.Repository{Name: repoID}
	}

	removeCmd := c.buildRemoveCommand(repo.Name)
	if err := c.service.ExecInContainer("bash", "-c", removeCmd); err != nil {
		log.Printf("Failed to remove %s from Code Server: %v", repoID, err)
		// Don't return error as this is cleanup
	}

	log.Printf("Repository %s removed from Code Server", repoID)
	return nil
}

// buildCloneCommand builds the git clone command
func (c *CoderService) buildCloneCommand(repoName, cloneURL, gitUserName, gitUserEmail string) string {
	// Escape repository name for shell
	escapedName := strings.ReplaceAll(repoName, "'", "'\\''")

	return fmt.Sprintf(`
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
}

// buildCheckCommand builds the command to check if repository exists
func (c *CoderService) buildCheckCommand(repoName string) string {
	escapedName := strings.ReplaceAll(repoName, "'", "'\\''")

	return fmt.Sprintf(`
		cd /home/coder/project && 
		[ -d '%s/.git' ] && echo "exists" || echo "not-exists"
	`, escapedName)
}

// buildUpdateCommand builds the git pull command
func (c *CoderService) buildUpdateCommand(repoName string) string {
	escapedName := strings.ReplaceAll(repoName, "'", "'\\''")

	return fmt.Sprintf(`
		cd /home/coder/project/%s &&
		git stash save "Auto-stash before update" &&
		git fetch origin &&
		git pull origin $(git symbolic-ref --short HEAD 2>/dev/null || echo 'master') &&
		git stash pop || true
	`, escapedName)
}

// buildRemoveCommand builds the rm command
func (c *CoderService) buildRemoveCommand(repoName string) string {
	escapedName := strings.ReplaceAll(repoName, "'", "'\\''")
	return fmt.Sprintf(`rm -rf '/home/coder/project/%s'`, escapedName)
}