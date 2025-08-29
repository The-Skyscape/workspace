package services

import "workspace/internal/claude"

// SandboxAdapter adapts the sandbox service to the Claude interface
type SandboxAdapter struct{}

// NewSandbox creates a new sandbox
func (s SandboxAdapter) NewSandbox(name, repoPath, repoName, command string, timeoutSecs int) (claude.SandboxInterface, error) {
	return NewSandbox(name, repoPath, repoName, command, timeoutSecs)
}

// GetSandbox retrieves an existing sandbox
func (s SandboxAdapter) GetSandbox(name string) (claude.SandboxInterface, error) {
	return GetSandbox(name)
}