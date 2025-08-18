package services

import (
	"os/exec"
)

// IsDockerAvailable checks if Docker daemon is accessible
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	err := cmd.Run()
	return err == nil
}