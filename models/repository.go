package models

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// Repository represents a git repository with enhanced tracking
type Repository struct {
	application.Model
	Name          string    // Display name (e.g., "My Awesome Repo")
	Description   string    // Repository description
	Visibility    string    // "public" or "private"
	UserID        string    // Owner ID
	State         string    // "empty", "initialized", "active"
	DefaultBranch string    // Default branch name
	Size          int64     // Repository size in bytes
	LastActivityAt time.Time // Last commit or update time
}

// Table returns the database table name
func (*Repository) Table() string { return "repositories" }

// RepositoryState constants
const (
	StateEmpty       = "empty"
	StateInitialized = "initialized"
	StateActive      = "active"
)

// Visibility constants
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// GenerateRepositoryID creates a URL-safe ID from a repository name
func GenerateRepositoryID(name string) string {
	// Convert to lowercase
	id := strings.ToLower(name)
	
	// Replace spaces and underscores with hyphens
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	
	// Remove any character that isn't alphanumeric or hyphen
	reg := regexp.MustCompile(`[^a-z0-9\-]+`)
	id = reg.ReplaceAllString(id, "")
	
	// Replace multiple hyphens with single hyphen
	reg = regexp.MustCompile(`\-+`)
	id = reg.ReplaceAllString(id, "-")
	
	// Remove leading/trailing hyphens
	id = strings.Trim(id, "-")
	
	// Ensure it starts with a letter or number
	if len(id) > 0 && !regexp.MustCompile(`^[a-z0-9]`).MatchString(id) {
		id = "repo-" + id
	}
	
	// If empty after cleaning, generate a default
	if id == "" {
		id = fmt.Sprintf("repo-%d", time.Now().Unix())
	}
	
	// Truncate if too long (max 100 characters)
	if len(id) > 100 {
		id = id[:100]
		// Ensure we don't end with a hyphen after truncation
		id = strings.TrimRight(id, "-")
	}
	
	return id
}

// ValidateRepositoryID checks if a repository ID is valid
func ValidateRepositoryID(id string) error {
	if id == "" {
		return errors.New("repository ID cannot be empty")
	}
	
	if len(id) > 100 {
		return errors.New("repository ID cannot exceed 100 characters")
	}
	
	// Must contain only lowercase letters, numbers, and hyphens
	if !regexp.MustCompile(`^[a-z0-9\-]+$`).MatchString(id) {
		return errors.New("repository ID can only contain lowercase letters, numbers, and hyphens")
	}
	
	// Must start with a letter or number
	if !regexp.MustCompile(`^[a-z0-9]`).MatchString(id) {
		return errors.New("repository ID must start with a letter or number")
	}
	
	// Must end with a letter or number
	if !regexp.MustCompile(`[a-z0-9]$`).MatchString(id) {
		return errors.New("repository ID must end with a letter or number")
	}
	
	return nil
}

// CreateRepository creates a new repository with URL-safe ID
func CreateRepository(name, description, visibility, userID string) (*Repository, error) {
	// Generate URL-safe ID
	id := GenerateRepositoryID(name)
	
	// Validate the generated ID
	if err := ValidateRepositoryID(id); err != nil {
		return nil, errors.Wrap(err, "invalid repository ID")
	}
	
	// Check for conflicts
	existing, _ := Repositories.Get(id)
	if existing != nil {
		// Try adding a suffix if there's a conflict
		for i := 2; i <= 10; i++ {
			newID := fmt.Sprintf("%s-%d", id, i)
			existing, _ = Repositories.Get(newID)
			if existing == nil {
				id = newID
				break
			}
		}
		// If still conflicting after 10 attempts, fail
		if existing != nil {
			return nil, errors.New("repository ID already exists")
		}
	}
	
	// Validate visibility
	if visibility != VisibilityPublic && visibility != VisibilityPrivate {
		visibility = VisibilityPrivate
	}
	
	// Create the repository record
	repo := &Repository{
		Model:         DB.NewModel(id),
		Name:          name,
		Description:   description,
		Visibility:    visibility,
		UserID:        userID,
		State:         StateEmpty,
		DefaultBranch: "master",
		Size:          0,
		LastActivityAt: time.Now(),
	}
	
	// Insert into database
	repo, err := Repositories.Insert(repo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create repository record")
	}
	
	// Create git directory
	repoPath := repo.Path()
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		// Rollback database record
		Repositories.Delete(repo)
		return nil, errors.Wrap(err, "failed to create repository directory")
	}
	
	// Initialize bare git repository
	if _, _, err := repo.Git("init", "--bare"); err != nil {
		// Rollback
		os.RemoveAll(repoPath)
		Repositories.Delete(repo)
		return nil, errors.Wrap(err, "failed to initialize git repository")
	}
	
	// Log activity
	LogActivity("repo_created", "Created repository "+name, 
		"Created a new repository", userID, repo.ID, "repository", "")
	
	return repo, nil
}

// Path returns the filesystem path for this repository
func (r *Repository) Path() string {
	return filepath.Join(database.DataDir(), "repos", r.ID)
}

// Git executes a git command in the repository directory
func (r *Repository) Git(args ...string) (stdout, stderr *bytes.Buffer, err error) {
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	
	err = cmd.Run()
	return stdout, stderr, err
}

// GetRepositoryByID retrieves a repository by its ID
func GetRepositoryByID(id string) (*Repository, error) {
	return Repositories.Get(id)
}

// GetRepositoryByName finds a repository by name and user
func GetRepositoryByName(name, userID string) (*Repository, error) {
	// Generate the expected ID
	expectedID := GenerateRepositoryID(name)
	
	// Try to find by exact ID first
	repo, err := Repositories.Get(expectedID)
	if err == nil && repo != nil && repo.UserID == userID {
		return repo, nil
	}
	
	// Search by name and user
	repos, err := Repositories.Search("WHERE Name = ? AND UserID = ?", name, userID)
	if err != nil {
		return nil, err
	}
	if len(repos) > 0 {
		return repos[0], nil
	}
	
	return nil, errors.New("repository not found")
}

// ListUserRepositories returns all repositories for a user
func ListUserRepositories(userID string) ([]*Repository, error) {
	return Repositories.Search("WHERE UserID = ? ORDER BY LastActivityAt DESC", userID)
}

// DeleteRepository removes a repository and its git directory
func DeleteRepository(id string) error {
	repo, err := Repositories.Get(id)
	if err != nil {
		return err
	}
	
	// Remove git directory
	if err := os.RemoveAll(repo.Path()); err != nil {
		return errors.Wrap(err, "failed to remove repository directory")
	}
	
	// Delete database record
	if err := Repositories.Delete(repo); err != nil {
		return errors.Wrap(err, "failed to delete repository record")
	}
	
	// Delete related records (permissions, issues, etc.)
	DB.Query("DELETE FROM permissions WHERE RepoID = ?", id)
	DB.Query("DELETE FROM issues WHERE RepoID = ?", id)
	DB.Query("DELETE FROM pull_requests WHERE RepoID = ?", id)
	DB.Query("DELETE FROM access_tokens WHERE RepoID = ?", id)
	
	return nil
}

// UpdateState checks the repository state and updates it
func (r *Repository) UpdateState() error {
	// Check if repository has any branches
	branches, err := r.GetBranches()
	if err != nil || len(branches) == 0 {
		r.State = StateEmpty
	} else {
		// Check if there are any commits
		count, err := r.GetCommitCount("HEAD")
		if err != nil || count == 0 {
			r.State = StateInitialized
		} else {
			r.State = StateActive
		}
	}
	
	return Repositories.Update(r)
}

// IsEmpty checks if the repository has no commits
func (r *Repository) IsEmpty() bool {
	count, err := r.GetCommitCount("HEAD")
	return err != nil || count == 0
}

// GetState returns the current repository state
func (r *Repository) GetState() string {
	if r.State == "" {
		r.UpdateState()
	}
	return r.State
}

// GetSize calculates the repository size in bytes
func (r *Repository) GetSize() (int64, error) {
	var size int64
	
	err := filepath.Walk(r.Path(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	
	if err != nil {
		return 0, err
	}
	
	// Update cached size
	r.Size = size
	Repositories.Update(r)
	
	return size, nil
}

// UpdateLastActivity updates the last activity timestamp
func (r *Repository) UpdateLastActivity() error {
	r.LastActivityAt = time.Now()
	return Repositories.Update(r)
}