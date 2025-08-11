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
	existingRepo, err := Repositories.Get(id)
	if err == nil && existingRepo != nil {
		// Repository exists, try adding a suffix
		for i := 2; i <= 10; i++ {
			newID := fmt.Sprintf("%s-%d", id, i)
			_, err = Repositories.Get(newID)
			if err != nil {
				// This ID is available
				id = newID
				break
			}
		}
		// Check one more time after loop
		_, err = Repositories.Get(id)
		if err == nil {
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
	}
	
	// Insert into database
	repo, err = Repositories.Insert(repo)
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

// GetFileModTime returns the last modification time of a file from git history
func (r *Repository) GetFileModTime(branch, path string) (time.Time, error) {
	stdout, _, err := r.Git("log", "-1", "--format=%aI", branch, "--", path)
	if err != nil {
		// If error, return zero time which will be handled by caller
		return time.Time{}, err
	}
	
	timeStr := strings.TrimSpace(stdout.String())
	if timeStr == "" {
		// File has no history yet, return zero time
		return time.Time{}, nil
	}
	
	return time.Parse(time.RFC3339, timeStr)
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
	return Repositories.Search("WHERE UserID = ? ORDER BY UpdatedAt DESC", userID)
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
	// UpdatedAt is automatically updated by the database layer
	return Repositories.Update(r)
}

// Contributor represents a repository contributor
type Contributor struct {
	Name    string
	Email   string
	Commits int
	Avatar  string
}

// GetLanguageStats returns language statistics for the repository
func (r *Repository) GetLanguageStats() (map[string]int, error) {
	stats := make(map[string]int)
	
	// Get list of all files in the repository
	stdout, _, err := r.Git("ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		// Repository might be empty
		return stats, nil
	}
	
	files := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(files) == 0 || (len(files) == 1 && files[0] == "") {
		return stats, nil
	}
	
	// Language mapping by file extension
	langMap := map[string]string{
		".go":     "Go",
		".js":     "JavaScript",
		".jsx":    "JavaScript",
		".ts":     "TypeScript",
		".tsx":    "TypeScript",
		".py":     "Python",
		".rb":     "Ruby",
		".java":   "Java",
		".c":      "C",
		".h":      "C",
		".cpp":    "C++",
		".cc":     "C++",
		".hpp":    "C++",
		".cs":     "C#",
		".php":    "PHP",
		".swift":  "Swift",
		".kt":     "Kotlin",
		".rs":     "Rust",
		".html":   "HTML",
		".htm":    "HTML",
		".css":    "CSS",
		".scss":   "SCSS",
		".sass":   "Sass",
		".less":   "Less",
		".sql":    "SQL",
		".sh":     "Shell",
		".bash":   "Shell",
		".yml":    "YAML",
		".yaml":   "YAML",
		".json":   "JSON",
		".xml":    "XML",
		".md":     "Markdown",
		".vue":    "Vue",
		".dart":   "Dart",
		".r":      "R",
		".lua":    "Lua",
		".pl":     "Perl",
		".ex":     "Elixir",
		".exs":    "Elixir",
		".scala":  "Scala",
		".clj":    "Clojure",
		".elm":    "Elm",
		".hs":     "Haskell",
	}
	
	// Count lines for each language
	for _, file := range files {
		ext := filepath.Ext(file)
		if ext == "" {
			continue
		}
		
		lang, ok := langMap[strings.ToLower(ext)]
		if !ok {
			continue
		}
		
		// Get file content to count lines
		stdout, _, err := r.Git("show", "HEAD:"+file)
		if err != nil {
			continue
		}
		
		lines := strings.Count(stdout.String(), "\n")
		if lines > 0 {
			stats[lang] += lines
		}
	}
	
	return stats, nil
}

// GetContributors returns a list of repository contributors
func (r *Repository) GetContributors() ([]*Contributor, error) {
	contributorMap := make(map[string]*Contributor)
	
	// Use git shortlog to get contributor statistics
	stdout, _, err := r.Git("shortlog", "-sne", "HEAD")
	if err != nil {
		// Repository might be empty or have no commits
		return []*Contributor{}, nil
	}
	
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		// Parse format: "   123  Name <email@example.com>"
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) != 2 {
			continue
		}
		
		// Parse commit count
		commits := 0
		fmt.Sscanf(parts[0], "%d", &commits)
		
		// Parse name and email
		nameEmail := parts[1]
		var name, email string
		
		if idx := strings.Index(nameEmail, " <"); idx != -1 {
			name = nameEmail[:idx]
			if endIdx := strings.Index(nameEmail[idx:], ">"); endIdx != -1 {
				email = nameEmail[idx+2 : idx+endIdx]
			}
		} else {
			name = nameEmail
		}
		
		if name == "" {
			continue
		}
		
		// Create or update contributor
		key := strings.ToLower(email)
		if key == "" {
			key = strings.ToLower(name)
		}
		
		if existing, ok := contributorMap[key]; ok {
			existing.Commits += commits
		} else {
			contributorMap[key] = &Contributor{
				Name:    name,
				Email:   email,
				Commits: commits,
				Avatar:  "", // Could generate gravatar URL from email
			}
		}
	}
	
	// Convert map to slice
	contributors := make([]*Contributor, 0, len(contributorMap))
	for _, contributor := range contributorMap {
		contributors = append(contributors, contributor)
	}
	
	// Sort by commit count (descending)
	for i := 0; i < len(contributors); i++ {
		for j := i + 1; j < len(contributors); j++ {
			if contributors[j].Commits > contributors[i].Commits {
				contributors[i], contributors[j] = contributors[j], contributors[i]
			}
		}
	}
	
	return contributors, nil
}

// GetRecentActivities returns recent activities for this repository
func (r *Repository) GetRecentActivities(limit int) ([]*Activity, error) {
	if limit <= 0 {
		limit = 10
	}
	
	// Get activities related to this repository
	return Activities.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC LIMIT ?", r.ID, limit)
}

// GetFileCount returns the total number of files in the repository
func (r *Repository) GetFileCount() (int, error) {
	stdout, _, err := r.Git("ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		// Repository might be empty
		return 0, nil
	}
	
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0, nil
	}
	
	return len(lines), nil
}

// GetDefaultREADME returns the README file for the default branch
func (r *Repository) GetDefaultREADME() (*File, error) {
	return r.GetREADME(r.GetDefaultBranch())
}