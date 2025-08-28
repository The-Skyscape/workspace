package services

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
	
	"workspace/models"
)

// GitOperationsService handles Git remote operations and synchronization
type GitOperationsService struct{}

// NewGitOperationsService creates a new Git operations service
func NewGitOperationsService() *GitOperationsService {
	return &GitOperationsService{}
}

// ConfigureRemote adds or updates the GitHub remote for a repository
func (s *GitOperationsService) ConfigureRemote(repo *models.Repository, githubURL string) error {
	if githubURL == "" {
		return fmt.Errorf("GitHub URL is required")
	}
	
	repoPath := repo.Path()
	
	// Check if remote exists
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	output, _ := cmd.Output()
	
	if len(output) > 0 {
		// Update existing remote
		cmd = exec.Command("git", "remote", "set-url", "origin", githubURL)
	} else {
		// Add new remote
		cmd = exec.Command("git", "remote", "add", "origin", githubURL)
	}
	
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to configure remote: %w", err)
	}
	
	// Update repository
	repo.GitHubURL = githubURL
	repo.RemoteConfigured = true
	if err := models.Repositories.Update(repo); err != nil {
		return fmt.Errorf("failed to update repository: %w", err)
	}
	
	// Log without exposing any potential embedded credentials
	sanitizedURL := githubURL
	if strings.Contains(sanitizedURL, "@") {
		parts := strings.SplitN(sanitizedURL, "@", 2)
		if len(parts) == 2 {
			sanitizedURL = "***@" + parts[1]
		}
	}
	log.Printf("Configured GitHub remote for repository %s: %s", repo.ID, sanitizedURL)
	return nil
}

// FetchFromRemote fetches latest changes from GitHub
func (s *GitOperationsService) FetchFromRemote(repo *models.Repository) error {
	if !repo.RemoteConfigured {
		return fmt.Errorf("remote not configured")
	}
	
	repoPath := repo.Path()
	
	// Fetch all branches and tags
	cmd := exec.Command("git", "fetch", "origin", "--all", "--tags")
	cmd.Dir = repoPath
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch from remote: %s", stderr.String())
	}
	
	log.Printf("Fetched latest changes from GitHub for repository %s", repo.ID)
	return nil
}

// PushToRemote pushes changes to GitHub
func (s *GitOperationsService) PushToRemote(repo *models.Repository, branch string, userToken string) error {
	if !repo.RemoteConfigured {
		return fmt.Errorf("remote not configured")
	}
	
	if branch == "" {
		branch = repo.CurrentBranch
		if branch == "" {
			branch = "main"
		}
	}
	
	repoPath := repo.Path()
	
	// Construct authenticated URL if token provided
	pushURL := repo.GitHubURL
	if userToken != "" && strings.HasPrefix(pushURL, "https://github.com/") {
		// Insert token into URL for authentication
		pushURL = strings.Replace(pushURL, "https://", fmt.Sprintf("https://%s@", userToken), 1)
	}
	
	// Push to remote
	cmd := exec.Command("git", "push", pushURL, branch)
	cmd.Dir = repoPath
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		// Provide more helpful error messages
		if strings.Contains(errMsg, "Authentication failed") || strings.Contains(errMsg, "fatal: could not read Username") {
			return fmt.Errorf("GitHub authentication failed. Please reconnect your GitHub account in Settings")
		}
		if strings.Contains(errMsg, "Permission denied") || strings.Contains(errMsg, "403") {
			return fmt.Errorf("permission denied. Check that your GitHub account has write access to this repository")
		}
		if strings.Contains(errMsg, "non-fast-forward") {
			return fmt.Errorf("push rejected: remote has changes that you don't have locally. Pull changes first, then push again")
		}
		if strings.Contains(errMsg, "Could not resolve host") {
			return fmt.Errorf("network error: unable to connect to GitHub. Check your internet connection")
		}
		return fmt.Errorf("failed to push to remote: %s", errMsg)
	}
	
	// Update last push time
	repo.LastPushAt = time.Now()
	if err := models.Repositories.Update(repo); err != nil {
		log.Printf("Failed to update last push time: %v", err)
	}
	
	log.Printf("Pushed branch %s to GitHub for repository %s", branch, repo.ID)
	return nil
}

// PullFromRemote pulls changes from GitHub
func (s *GitOperationsService) PullFromRemote(repo *models.Repository, branch string, userToken string) error {
	if !repo.RemoteConfigured {
		return fmt.Errorf("remote not configured")
	}
	
	if branch == "" {
		branch = repo.CurrentBranch
		if branch == "" {
			branch = "main"
		}
	}
	
	repoPath := repo.Path()
	
	// Construct authenticated URL if token provided
	pullURL := repo.GitHubURL
	if userToken != "" && strings.HasPrefix(pullURL, "https://github.com/") {
		pullURL = strings.Replace(pullURL, "https://", fmt.Sprintf("https://%s@", userToken), 1)
	}
	
	// Pull from remote
	cmd := exec.Command("git", "pull", pullURL, branch)
	cmd.Dir = repoPath
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		// Provide more helpful error messages
		if strings.Contains(errMsg, "Authentication failed") || strings.Contains(errMsg, "fatal: could not read Username") {
			return fmt.Errorf("GitHub authentication failed. Please reconnect your GitHub account in Settings")
		}
		if strings.Contains(errMsg, "Permission denied") || strings.Contains(errMsg, "403") {
			return fmt.Errorf("permission denied. Check that your GitHub account has read access to this repository")
		}
		if strings.Contains(errMsg, "CONFLICT") || strings.Contains(errMsg, "Merge conflict") {
			return fmt.Errorf("merge conflict detected. Manual conflict resolution required")
		}
		if strings.Contains(errMsg, "Could not resolve host") {
			return fmt.Errorf("network error: unable to connect to GitHub. Check your internet connection")
		}
		return fmt.Errorf("failed to pull from remote: %s", errMsg)
	}
	
	// Update last pull time
	repo.LastPullAt = time.Now()
	if err := models.Repositories.Update(repo); err != nil {
		log.Printf("Failed to update last pull time: %v", err)
	}
	
	log.Printf("Pulled branch %s from GitHub for repository %s", branch, repo.ID)
	return nil
}

// GetSyncStatus checks how many commits the local repo is ahead/behind the remote
func (s *GitOperationsService) GetSyncStatus(repo *models.Repository) (ahead int, behind int, status string, err error) {
	if !repo.RemoteConfigured {
		return 0, 0, "no-remote", nil
	}
	
	repoPath := repo.Path()
	
	// First fetch to get latest remote info
	if err := s.FetchFromRemote(repo); err != nil {
		log.Printf("Warning: Failed to fetch during sync status check: %v", err)
	}
	
	// Get current branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, "error", fmt.Errorf("failed to get current branch: %w", err)
	}
	
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		branch = "main"
	}
	
	// Check ahead/behind status
	cmd = exec.Command("git", "rev-list", "--left-right", "--count", fmt.Sprintf("origin/%s...%s", branch, branch))
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		// Remote branch might not exist yet
		cmd = exec.Command("git", "rev-list", "--count", branch)
		cmd.Dir = repoPath
		output, err = cmd.Output()
		if err != nil {
			return 0, 0, "error", fmt.Errorf("failed to get commit count: %w", err)
		}
		
		// All commits are ahead (remote branch doesn't exist)
		ahead, _ = strconv.Atoi(strings.TrimSpace(string(output)))
		return ahead, 0, "ahead", nil
	}
	
	// Parse ahead/behind counts
	parts := strings.Fields(string(output))
	if len(parts) >= 2 {
		behind, _ = strconv.Atoi(parts[0])
		ahead, _ = strconv.Atoi(parts[1])
	}
	
	// Determine status
	if ahead > 0 && behind > 0 {
		status = "diverged"
	} else if ahead > 0 {
		status = "ahead"
	} else if behind > 0 {
		status = "behind"
	} else {
		status = "synced"
	}
	
	// Update repository sync status
	repo.LocalAhead = ahead
	repo.LocalBehind = behind
	repo.SyncStatus = status
	repo.CurrentBranch = branch
	if err := models.Repositories.Update(repo); err != nil {
		log.Printf("Failed to update sync status: %v", err)
	}
	
	return ahead, behind, status, nil
}

// ListRemoteBranches lists all branches from the GitHub remote
func (s *GitOperationsService) ListRemoteBranches(repo *models.Repository) ([]string, error) {
	if !repo.RemoteConfigured {
		return nil, fmt.Errorf("remote not configured")
	}
	
	repoPath := repo.Path()
	
	// List remote branches
	cmd := exec.Command("git", "branch", "-r")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list remote branches: %w", err)
	}
	
	// Parse branch names
	var branches []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "HEAD") {
			continue
		}
		
		// Remove "origin/" prefix
		if strings.HasPrefix(line, "origin/") {
			branches = append(branches, strings.TrimPrefix(line, "origin/"))
		}
	}
	
	return branches, nil
}

// SyncWithRemote performs a full sync (fetch, merge/rebase, push) with conflict detection
func (s *GitOperationsService) SyncWithRemote(repo *models.Repository, userToken string) error {
	if !repo.RemoteConfigured {
		return fmt.Errorf("remote not configured")
	}
	
	// Get sync status first
	ahead, behind, status, err := s.GetSyncStatus(repo)
	if err != nil {
		return fmt.Errorf("failed to get sync status: %w", err)
	}
	
	log.Printf("Repository %s sync status: %s (ahead: %d, behind: %d)", repo.ID, status, ahead, behind)
	
	// Handle based on status
	switch status {
	case "synced":
		// Already in sync
		return nil
		
	case "ahead":
		// Push changes
		return s.PushToRemote(repo, "", userToken)
		
	case "behind":
		// Pull changes
		return s.PullFromRemote(repo, "", userToken)
		
	case "diverged":
		// Need to pull then push (with potential conflict resolution)
		if err := s.PullFromRemote(repo, "", userToken); err != nil {
			return fmt.Errorf("failed to pull during sync: %w", err)
		}
		return s.PushToRemote(repo, "", userToken)
		
	default:
		return fmt.Errorf("unknown sync status: %s", status)
	}
}

// CheckForConflicts checks if there are any merge conflicts in the repository
func (s *GitOperationsService) CheckForConflicts(repo *models.Repository) (bool, []string, error) {
	repoPath := repo.Path()
	
	// Check for conflict markers in tracked files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// No conflicts
		return false, nil, nil
	}
	
	if len(output) == 0 {
		return false, nil, nil
	}
	
	// Parse conflicted files
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	return true, files, nil
}

// Global instance
var GitOperations = NewGitOperationsService()