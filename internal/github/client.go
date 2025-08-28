package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	
	"workspace/models"
)

// GitHubClient provides GitHub API operations using OAuth tokens from vault
type GitHubClient struct {
	userID string
	token  string
	client *http.Client
}

// GitHubIssue represents a GitHub issue
type GitHubIssue struct {
	ID        int64     `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	User      GitHubUser `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// GitHubPullRequest represents a GitHub pull request
type GitHubPullRequest struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	User      GitHubUser `json:"user"`
	Head      GitHubRef  `json:"head"`
	Base      GitHubRef  `json:"base"`
	Merged    bool       `json:"merged"`
	MergedAt  *time.Time `json:"merged_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubRef represents a git reference (branch)
type GitHubRef struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	User GitHubUser `json:"user"`
	Repo GitHubRepo `json:"repo"`
}

// GitHubRepo represents a GitHub repository
type GitHubRepo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
}

// NewGitHubClient creates a new GitHub client for a user using OAuth token from vault
func NewGitHubClient(userID string) (*GitHubClient, error) {
	// Get OAuth token from vault
	token, err := models.GetGitHubOAuthToken(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub OAuth token: %w", err)
	}
	
	return &GitHubClient{
		userID: userID,
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// NewGitHubClientWithToken creates a GitHub client with a specific token (for testing)
func NewGitHubClientWithToken(token string) *GitHubClient {
	return &GitHubClient{
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// doRequest performs an authenticated GitHub API request
func (c *GitHubClient) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	
	// Check for rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		resetHeader := resp.Header.Get("X-RateLimit-Reset")
		return resp, fmt.Errorf("GitHub API rate limit exceeded. Try again after %s", resetHeader)
	}
	
	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized {
		return resp, fmt.Errorf("GitHub authentication failed. Please reconnect your GitHub account")
	}
	
	if resp.StatusCode == http.StatusForbidden {
		// Check if it's a rate limit or permission issue
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		if remaining == "0" {
			resetHeader := resp.Header.Get("X-RateLimit-Reset")
			return resp, fmt.Errorf("GitHub API rate limit exceeded. Try again after %s", resetHeader)
		}
		return resp, fmt.Errorf("insufficient permissions for this GitHub operation")
	}
	
	return resp, nil
}

// parseGitHubURL extracts owner and repo from a GitHub URL
func parseGitHubURL(githubURL string) (owner, repo string, err error) {
	// Validate that this is actually a GitHub URL
	if !strings.Contains(githubURL, "github.com") {
		return "", "", fmt.Errorf("not a GitHub URL: %s", githubURL)
	}
	
	// Handle various GitHub URL formats
	githubURL = strings.TrimSpace(githubURL)
	githubURL = strings.TrimSuffix(githubURL, ".git")
	githubURL = strings.TrimSuffix(githubURL, "/")
	
	// Try HTTPS URL format: https://github.com/owner/repo
	if strings.HasPrefix(githubURL, "https://github.com/") {
		parts := strings.Split(strings.TrimPrefix(githubURL, "https://github.com/"), "/")
		if len(parts) >= 2 {
			// Sanitize owner and repo names
			owner = strings.TrimSpace(parts[0])
			repo = strings.TrimSpace(parts[1])
			if owner == "" || repo == "" {
				return "", "", fmt.Errorf("invalid GitHub URL: missing owner or repository name")
			}
			// Validate owner and repo names (GitHub's rules)
			if !isValidGitHubName(owner) || !isValidGitHubName(repo) {
				return "", "", fmt.Errorf("invalid GitHub owner or repository name")
			}
			return owner, repo, nil
		}
	}
	
	// Try SSH URL format: git@github.com:owner/repo.git
	if strings.HasPrefix(githubURL, "git@github.com:") {
		parts := strings.Split(strings.TrimPrefix(githubURL, "git@github.com:"), "/")
		if len(parts) >= 2 {
			// Sanitize owner and repo names
			owner = strings.TrimSpace(parts[0])
			repo = strings.TrimSpace(parts[1])
			if owner == "" || repo == "" {
				return "", "", fmt.Errorf("invalid GitHub URL: missing owner or repository name")
			}
			// Validate owner and repo names
			if !isValidGitHubName(owner) || !isValidGitHubName(repo) {
				return "", "", fmt.Errorf("invalid GitHub owner or repository name")
			}
			return owner, repo, nil
		}
	}
	
	return "", "", fmt.Errorf("invalid GitHub URL format: %s", githubURL)
}

// isValidGitHubName validates GitHub username or repository name
func isValidGitHubName(name string) bool {
	// GitHub names can contain alphanumeric characters or hyphens
	// Cannot have multiple consecutive hyphens
	// Cannot begin or end with a hyphen
	// Maximum length is 39 characters
	if len(name) == 0 || len(name) > 39 {
		return false
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	if strings.Contains(name, "--") {
		return false
	}
	// Check for valid characters
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || 
			 (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// ListIssues lists all issues for a repository
func (c *GitHubClient) ListIssues(ctx context.Context, githubURL string) ([]*GitHubIssue, error) {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=all&per_page=100", owner, repo)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var issues []*GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("failed to decode issues: %w", err)
	}
	
	// Filter out pull requests (GitHub returns them in issues endpoint too)
	var realIssues []*GitHubIssue
	for _, issue := range issues {
		// PRs have pull_request field, but we check if URL contains /pull/
		if !strings.Contains(issue.HTMLURL, "/pull/") {
			realIssues = append(realIssues, issue)
		}
	}
	
	return realIssues, nil
}

// ListPullRequests lists all pull requests for a repository
func (c *GitHubClient) ListPullRequests(ctx context.Context, githubURL string) ([]*GitHubPullRequest, error) {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=all&per_page=100", owner, repo)
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var prs []*GitHubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, fmt.Errorf("failed to decode pull requests: %w", err)
	}
	
	return prs, nil
}

// CreateIssue creates a new issue on GitHub
func (c *GitHubClient) CreateIssue(ctx context.Context, githubURL, title, body string) (*GitHubIssue, error) {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)
	
	payload := map[string]string{
		"title": title,
		"body":  body,
	}
	
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	
	resp, err := c.doRequest("POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var issue GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode created issue: %w", err)
	}
	
	return &issue, nil
}

// UpdateIssue updates an existing issue on GitHub
func (c *GitHubClient) UpdateIssue(ctx context.Context, githubURL string, number int, title, body, state string) (*GitHubIssue, error) {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, number)
	
	payload := map[string]string{
		"title": title,
		"body":  body,
		"state": state,
	}
	
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	
	resp, err := c.doRequest("PATCH", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to update issue: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var issue GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode updated issue: %w", err)
	}
	
	return &issue, nil
}

// GetRateLimit returns the current rate limit status
func (c *GitHubClient) GetRateLimit(ctx context.Context) (remaining, limit int, resetAt time.Time, err error) {
	url := "https://api.github.com/rate_limit"
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("failed to get rate limit: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, 0, time.Time{}, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var rateLimitResp struct {
		Rate struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"rate"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&rateLimitResp); err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("failed to decode rate limit: %w", err)
	}
	
	return rateLimitResp.Rate.Remaining, 
		   rateLimitResp.Rate.Limit, 
		   time.Unix(rateLimitResp.Rate.Reset, 0), 
		   nil
}

// CreatePullRequest creates a new pull request on GitHub
func (c *GitHubClient) CreatePullRequest(ctx context.Context, githubURL, title, body, head, base string) (*GitHubPullRequest, error) {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo)
	
	payload := map[string]string{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	}
	
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	
	resp, err := c.doRequest("POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var pr GitHubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode created pull request: %w", err)
	}
	
	return &pr, nil
}

// UpdatePullRequest updates an existing pull request on GitHub
func (c *GitHubClient) UpdatePullRequest(ctx context.Context, githubURL string, number int, title, body, state string) (*GitHubPullRequest, error) {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, number)
	
	payload := make(map[string]string)
	if title != "" {
		payload["title"] = title
	}
	if body != "" {
		payload["body"] = body
	}
	if state != "" {
		payload["state"] = state
	}
	
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	
	resp, err := c.doRequest("PATCH", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to update pull request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	var pr GitHubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("failed to decode updated pull request: %w", err)
	}
	
	return &pr, nil
}

// MergePullRequest merges a pull request on GitHub
func (c *GitHubClient) MergePullRequest(ctx context.Context, githubURL string, number int, commitTitle, commitMessage, mergeMethod string) error {
	owner, repo, err := parseGitHubURL(githubURL)
	if err != nil {
		return err
	}
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/merge", owner, repo, number)
	
	payload := make(map[string]string)
	if commitTitle != "" {
		payload["commit_title"] = commitTitle
	}
	if commitMessage != "" {
		payload["commit_message"] = commitMessage
	}
	if mergeMethod == "" {
		mergeMethod = "merge" // default to merge commit
	}
	payload["merge_method"] = mergeMethod // "merge", "squash", or "rebase"
	
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	
	resp, err := c.doRequest("PUT", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// ValidateToken checks if the OAuth token is valid
func (c *GitHubClient) ValidateToken(ctx context.Context) (bool, error) {
	url := "https://api.github.com/user"
	
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == http.StatusOK, nil
}