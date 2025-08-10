package models

import (
	"strings"

	"github.com/pkg/errors"
)

// Branch represents a git branch
type Branch struct {
	Name       string
	IsCurrent  bool
	IsDefault  bool
	LastCommit string
}

// GetBranches returns all branches in the repository
func (r *Repository) GetBranches() ([]*Branch, error) {
	// Check if repository has any commits first
	if r.IsEmpty() {
		return []*Branch{}, nil
	}

	// Get all branches
	stdout, stderr, err := r.Git("branch", "-a", "--format=%(refname:short)|%(HEAD)|%(objectname:short)")
	if err != nil {
		// If error, might be empty repository
		if strings.Contains(stderr.String(), "not a valid object") {
			return []*Branch{}, nil
		}
		return nil, errors.Wrap(err, stderr.String())
	}

	var branches []*Branch
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 3 {
			name := parts[0]
			isCurrent := parts[1] == "*"
			lastCommit := parts[2]

			// Skip remote branches for now
			if strings.HasPrefix(name, "origin/") {
				continue
			}

			branch := &Branch{
				Name:       name,
				IsCurrent:  isCurrent,
				IsDefault:  name == r.DefaultBranch,
				LastCommit: lastCommit,
			}

			branches = append(branches, branch)
		}
	}

	return branches, nil
}

// GetDefaultBranch returns the default branch name
func (r *Repository) GetDefaultBranch() string {
	if r.DefaultBranch != "" {
		return r.DefaultBranch
	}

	// Try to detect from git
	stdout, _, err := r.Git("symbolic-ref", "--short", "HEAD")
	if err == nil {
		branch := strings.TrimSpace(stdout.String())
		if branch != "" {
			r.DefaultBranch = branch
			Repositories.Update(r)
			return branch
		}
	}

	// Check if master exists
	if r.BranchExists("master") {
		r.DefaultBranch = "master"
		Repositories.Update(r)
		return "master"
	}

	// Check if main exists
	if r.BranchExists("main") {
		r.DefaultBranch = "main"
		Repositories.Update(r)
		return "main"
	}

	// Default to master for new repos
	return "master"
}

// SetDefaultBranch sets the default branch
func (r *Repository) SetDefaultBranch(branch string) error {
	// Verify branch exists
	if !r.BranchExists(branch) {
		return errors.New("branch does not exist")
	}

	// Update git symbolic ref
	_, stderr, err := r.Git("symbolic-ref", "HEAD", "refs/heads/"+branch)
	if err != nil {
		return errors.Wrap(err, stderr.String())
	}

	// Update database
	r.DefaultBranch = branch
	return Repositories.Update(r)
}

// CreateBranch creates a new branch
func (r *Repository) CreateBranch(name, fromBranch string) error {
	if name == "" {
		return errors.New("branch name cannot be empty")
	}

	// Check if repository is empty
	if r.IsEmpty() {
		return errors.New("cannot create branch in empty repository")
	}

	// Check if branch already exists
	if r.BranchExists(name) {
		return errors.New("branch already exists")
	}

	// If no source branch specified, use default
	if fromBranch == "" {
		fromBranch = r.GetDefaultBranch()
	}

	// Create the branch
	_, stderr, err := r.Git("branch", name, fromBranch)
	if err != nil {
		return errors.Wrap(err, "failed to create branch: "+stderr.String())
	}

	r.UpdateLastActivity()
	return nil
}

// DeleteBranch removes a branch
func (r *Repository) DeleteBranch(name string) error {
	if name == "" {
		return errors.New("branch name cannot be empty")
	}

	// Can't delete default branch
	if name == r.GetDefaultBranch() {
		return errors.New("cannot delete default branch")
	}

	// Delete the branch
	_, stderr, err := r.Git("branch", "-D", name)
	if err != nil {
		return errors.Wrap(err, "failed to delete branch: "+stderr.String())
	}

	r.UpdateLastActivity()
	return nil
}

// HasBranches checks if repository has any branches
func (r *Repository) HasBranches() bool {
	branches, err := r.GetBranches()
	return err == nil && len(branches) > 0
}

// BranchExists checks if a branch exists
func (r *Repository) BranchExists(name string) bool {
	_, _, err := r.Git("rev-parse", "--verify", "refs/heads/"+name)
	return err == nil
}

// GetActiveBranch returns the currently checked out branch
func (r *Repository) GetActiveBranch() string {
	stdout, _, err := r.Git("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(stdout.String())
	if branch == "" {
		return r.GetDefaultBranch()
	}
	return branch
}

// ListBranches returns branch names as a simple string slice
func (r *Repository) ListBranches() ([]string, error) {
	branches, err := r.GetBranches()
	if err != nil {
		return nil, err
	}

	names := make([]string, len(branches))
	for i, branch := range branches {
		names[i] = branch.Name
	}
	return names, nil
}
