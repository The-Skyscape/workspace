package github

import "time"

// GitHubRepository represents a repository from the GitHub API.
// This type matches the JSON structure returned by GitHub's API,
// using the exact field names (with json tags) that GitHub provides.
// Templates can access these fields with PascalCase names.
type GitHubRepository struct {
	// Basic information
	Name        string `json:"name"`        // Repository name
	Description string `json:"description"` // Repository description
	Private     bool   `json:"private"`     // Whether the repo is private
	
	// URLs
	HTMLURL  string `json:"html_url"`  // Web URL to the repository
	CloneURL string `json:"clone_url"` // HTTPS clone URL
	SSHURL   string `json:"ssh_url"`   // SSH clone URL
	
	// Repository details
	Language         string    `json:"language"`          // Primary language
	StargazersCount  int       `json:"stargazers_count"`  // Number of stars
	ForksCount       int       `json:"forks_count"`       // Number of forks
	OpenIssuesCount  int       `json:"open_issues_count"` // Number of open issues
	DefaultBranch    string    `json:"default_branch"`    // Default branch name
	CreatedAt        time.Time `json:"created_at"`        // When the repo was created
	UpdatedAt        time.Time `json:"updated_at"`        // Last update time
	PushedAt         time.Time `json:"pushed_at"`         // Last push time
	
	// Owner information
	Owner struct {
		Login     string `json:"login"`      // Username
		AvatarURL string `json:"avatar_url"` // Avatar URL
		HTMLURL   string `json:"html_url"`   // Profile URL
		Type      string `json:"type"`       // User or Organization
	} `json:"owner"`
	
	// Permissions (when authenticated)
	Permissions struct {
		Admin    bool `json:"admin"`
		Maintain bool `json:"maintain"`
		Push     bool `json:"push"`
		Triage   bool `json:"triage"`
		Pull     bool `json:"pull"`
	} `json:"permissions,omitempty"`
	
	// Additional fields
	Fork     bool   `json:"fork"`      // Whether this is a fork
	Archived bool   `json:"archived"`  // Whether the repo is archived
	Disabled bool   `json:"disabled"`  // Whether the repo is disabled
	Topics   []string `json:"topics"`   // Repository topics/tags
}

// ListUserReposOptions represents options for listing user repositories
type ListUserReposOptions struct {
	Type      string // all, owner, member
	Sort      string // created, updated, pushed, full_name
	Direction string // asc, desc
	PerPage   int    // Results per page (max 100)
	Page      int    // Page number
}