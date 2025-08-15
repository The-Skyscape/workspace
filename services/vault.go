package services

import (
	"fmt"
	"log"
	"sync"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// VaultConfig holds configuration for the vault service
type VaultConfig struct {
	Port          int
	ContainerName string
	DataDir       string
	DevMode       bool
	RootToken     string
}

// VaultService manages a single global Hashicorp Vault instance
type VaultService struct {
	config  *VaultConfig
	service *containers.Service
	mu      sync.RWMutex
}

// VaultStatus represents the current status of the vault service
type VaultStatus struct {
	Running   bool
	Port      int
	DevMode   bool
	URL       string
	Health    string
	RootToken string
}

var (
	// Global vault instance
	Vault = NewVaultService()
)

// NewVaultService creates a new vault service with default configuration
func NewVaultService() *VaultService {
	return &VaultService{
		config: &VaultConfig{
			Port:          8200,
			ContainerName: "skyscape-vault",
			DataDir:       fmt.Sprintf("%s/vault", database.DataDir()),
			DevMode:       true,
			RootToken:     "skyscape-dev-token",
		},
	}
}

// Init initializes the vault service and starts it if not already running
func (v *VaultService) Init() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if already running
	if v.IsRunning() {
		log.Println("Vault service already running")
		return nil
	}

	// Create the service configuration
	host := containers.Local()
	v.service = &containers.Service{
		Host:          host,
		Name:          v.config.ContainerName,
		Image:         "hashicorp/vault:latest",
		Network:       "bridge",
		RestartPolicy: "always",
		Ports: map[int]int{
			8200: v.config.Port,
		},
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":     v.config.RootToken,
			"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
			"VAULT_ADDR":                  "http://0.0.0.0:8200",
			"VAULT_API_ADDR":              "http://0.0.0.0:8200",
		},
	}

	// In dev mode, Vault runs in memory
	if v.config.DevMode {
		v.service.Command = "vault server -dev -dev-listen-address=0.0.0.0:8200"
	} else {
		// For production mode, mount data directory
		v.service.Mounts = map[string]string{
			v.config.DataDir: "/vault/data",
		}
		v.service.Command = "vault server -config=/vault/config"
	}

	// Start the service
	return v.Start()
}

// Start starts the vault service
func (v *VaultService) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.service == nil {
		return errors.New("vault service not initialized")
	}

	if v.IsRunning() {
		return nil
	}

	log.Printf("Starting Vault service on port %d", v.config.Port)
	
	// Launch the container
	if err := containers.Launch(v.service.Host, v.service); err != nil {
		return errors.Wrap(err, "failed to launch vault container")
	}

	// Wait for vault to be ready
	if err := v.service.WaitForReady(30, func() error {
		// Simple health check - vault will respond on its API port
		return v.service.Host.Exec("curl", "-f", fmt.Sprintf("http://localhost:%d/v1/sys/health", v.config.Port))
	}); err != nil {
		log.Printf("Warning: Vault may not be fully ready: %v", err)
	}

	log.Printf("Vault service started successfully")
	if v.config.DevMode {
		log.Printf("Vault running in dev mode with root token: %s", v.config.RootToken)
		log.Printf("Access Vault UI at: http://localhost:%d", v.config.Port)
	}

	return nil
}

// Stop stops the vault service
func (v *VaultService) Stop() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.service == nil {
		return nil
	}

	log.Println("Stopping Vault service")
	return v.service.Stop()
}

// IsRunning checks if the vault service is running
func (v *VaultService) IsRunning() bool {
	if v.service == nil {
		// Check if container exists from a previous run
		host := containers.Local()
		service := &containers.Service{
			Host: host,
			Name: v.config.ContainerName,
		}
		return service.IsRunning()
	}
	return v.service.IsRunning()
}

// Restart restarts the vault service
func (v *VaultService) Restart() error {
	if err := v.Stop(); err != nil {
		return err
	}
	return v.Start()
}

// GetStatus returns the current status of the vault service
func (v *VaultService) GetStatus() VaultStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()

	status := VaultStatus{
		Running:   v.IsRunning(),
		Port:      v.config.Port,
		DevMode:   v.config.DevMode,
		URL:       fmt.Sprintf("http://localhost:%d", v.config.Port),
		RootToken: v.config.RootToken,
	}

	if status.Running {
		status.Health = "healthy"
	} else {
		status.Health = "stopped"
	}

	return status
}

// GetConfig returns the current configuration
func (v *VaultService) GetConfig() *VaultConfig {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	// Return a copy to prevent external modification
	config := *v.config
	return &config
}