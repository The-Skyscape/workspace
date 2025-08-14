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
	DefaultBranch string    // Default branch name
	Size          int64     // Repository size in bytes
	
	// Statistics
	PrimaryLanguage string    // Primary programming language
	LastActivityAt  time.Time // Last activity timestamp
	StarCount       int       // Number of stars (for future use)
	ForkCount       int       // Number of forks (for future use)
	
	// GitHub Integration
	GitHubURL     string    // GitHub repository URL
	GitHubToken   string    // GitHub personal access token (should be encrypted)
	SyncDirection string    // "push", "pull", or "both"
	AutoSync      bool      // Whether to auto-sync on changes
	LastSyncAt    time.Time // Last sync timestamp
}

// Table returns the database table name
func (*Repository) Table() string { return "repositories" }

// Visibility constants
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

func init() {
	// Create indexes for repositories table
	go func() {
		Repositories.Index("UserID")
		Repositories.Index("Visibility")
		Repositories.Index("CreatedAt DESC")
	}()
}


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

// IsEmpty checks if the repository has no commits
func (r *Repository) IsEmpty() bool {
	count, err := r.GetCommitCount("HEAD")
	return err != nil || count == 0
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
	// UpdatedAt is automatically updated by the database layer
	return Repositories.Update(r)
}

// GetStats returns comprehensive statistics for the repository
func (r *Repository) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Basic stats
	stats["name"] = r.Name
	stats["description"] = r.Description
	stats["visibility"] = r.Visibility
	stats["primary_language"] = r.PrimaryLanguage
	stats["last_activity"] = r.LastActivityAt
	stats["star_count"] = r.StarCount
	stats["fork_count"] = r.ForkCount
	
	// Get repository size
	size, err := r.GetSize()
	if err == nil {
		stats["size"] = size
		stats["size_formatted"] = formatBytes(size)
	}
	
	// Get file count
	fileCount, err := r.GetFileCount()
	if err == nil {
		stats["file_count"] = fileCount
	}
	
	// Get branch count
	branches, err := r.GetBranches()
	if err == nil {
		stats["branch_count"] = len(branches)
		stats["default_branch"] = r.GetDefaultBranch()
	}
	
	// Get commit count
	stdout, _, err := r.Git("rev-list", "--count", "HEAD")
	if err == nil {
		commitCount := strings.TrimSpace(stdout.String())
		stats["commit_count"] = commitCount
	}
	
	// Get contributor count
	contributors, err := r.GetContributors()
	if err == nil {
		stats["contributor_count"] = len(contributors)
	}
	
	// Get language statistics
	langStats, err := r.GetLanguageStats()
	if err == nil {
		stats["languages"] = langStats
		
		// Determine primary language if not set
		if r.PrimaryLanguage == "" && len(langStats) > 0 {
			maxBytes := 0
			primaryLang := ""
			for lang, bytes := range langStats {
				if bytes > maxBytes {
					maxBytes = bytes
					primaryLang = lang
				}
			}
			r.PrimaryLanguage = primaryLang
			stats["primary_language"] = primaryLang
			// Update the repository with the detected primary language
			go func() {
				Repositories.Update(r)
			}()
		}
	}
	
	// Get issue and PR counts
	openIssues, _ := Issues.Search("WHERE RepoID = ? AND Status = ?", r.ID, "open")
	closedIssues, _ := Issues.Search("WHERE RepoID = ? AND Status = ?", r.ID, "closed")
	stats["open_issues"] = len(openIssues)
	stats["closed_issues"] = len(closedIssues)
	
	openPRs, _ := PullRequests.Search("WHERE RepoID = ? AND Status = ?", r.ID, "open")
	mergedPRs, _ := PullRequests.Search("WHERE RepoID = ? AND Status = ?", r.ID, "merged")
	stats["open_prs"] = len(openPRs)
	stats["merged_prs"] = len(mergedPRs)
	
	// Get action statistics
	actions, _ := Actions.Search("WHERE RepoID = ?", r.ID)
	stats["action_count"] = len(actions)
	
	// Get recent action runs
	recentRuns, _ := ActionRuns.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC LIMIT 10", r.ID)
	successCount := 0
	failureCount := 0
	for _, run := range recentRuns {
		if run.Status == "success" {
			successCount++
		} else if run.Status == "failure" {
			failureCount++
		}
	}
	if len(recentRuns) > 0 {
		stats["action_success_rate"] = float64(successCount) / float64(len(recentRuns)) * 100
	}
	
	return stats, nil
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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