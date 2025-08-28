package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	
	"workspace/models"
)

// GitHubSyncService handles synchronization between local and GitHub issues/PRs
type GitHubSyncService struct {
	mu sync.Mutex
}

// NewGitHubSyncService creates a new GitHub sync service
func NewGitHubSyncService() *GitHubSyncService {
	return &GitHubSyncService{}
}

// SyncRepository syncs all issues and PRs for a repository
func (s *GitHubSyncService) SyncRepository(repoID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}
	
	// Check if GitHub integration is enabled
	if repo.GitHubURL == "" {
		return fmt.Errorf("repository has no GitHub integration")
	}
	
	// Get integration settings from vault
	integration, err := models.GetGitHubRepoIntegration(repoID)
	if err != nil {
		return fmt.Errorf("failed to get integration settings: %w", err)
	}
	
	if enabled, ok := integration["enabled"].(bool); !ok || !enabled {
		return fmt.Errorf("GitHub integration is disabled")
	}
	
	// Create GitHub client
	client, err := NewGitHubClient(userID)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	
	// Sync issues and PRs
	ctx := context.Background()
	
	log.Printf("Starting GitHub sync for repository %s", repo.Name)
	
	// Sync issues
	if err := s.syncIssues(ctx, client, repo); err != nil {
		log.Printf("Failed to sync issues for %s: %v", repo.Name, err)
	}
	
	// Sync pull requests
	if err := s.syncPullRequests(ctx, client, repo); err != nil {
		log.Printf("Failed to sync pull requests for %s: %v", repo.Name, err)
	}
	
	log.Printf("GitHub sync completed for repository %s", repo.Name)
	
	return nil
}

// syncIssues syncs issues between local and GitHub
func (s *GitHubSyncService) syncIssues(ctx context.Context, client *GitHubClient, repo *models.Repository) error {
	// Get issues from GitHub
	githubIssues, err := client.ListIssues(ctx, repo.GitHubURL)
	if err != nil {
		return fmt.Errorf("failed to list GitHub issues: %w", err)
	}
	
	// Get local issues
	localIssues, err := models.Issues.Search("WHERE RepoID = ?", repo.ID)
	if err != nil {
		return fmt.Errorf("failed to get local issues: %w", err)
	}
	
	// Create maps for efficient lookup
	localByGitHubID := make(map[int64]*models.Issue)
	localByGitHubNumber := make(map[int]*models.Issue)
	for _, issue := range localIssues {
		if issue.GitHubID > 0 {
			localByGitHubID[issue.GitHubID] = issue
		}
		if issue.GitHubNumber > 0 {
			localByGitHubNumber[issue.GitHubNumber] = issue
		}
	}
	
	// Process GitHub issues
	for _, ghIssue := range githubIssues {
		// Check if issue already exists locally
		localIssue, exists := localByGitHubID[ghIssue.ID]
		if !exists {
			localIssue, exists = localByGitHubNumber[ghIssue.Number]
		}
		
		if exists {
			// Update existing issue if needed
			if s.shouldUpdateLocalIssue(localIssue, ghIssue) {
				s.updateLocalIssue(localIssue, ghIssue)
			}
		} else {
			// Create new local issue
			s.createLocalIssue(repo, ghIssue)
		}
	}
	
	// Handle local-only issues based on sync direction
	if repo.SyncDirection == "push" || repo.SyncDirection == "both" {
		for _, localIssue := range localIssues {
			// Skip if already synced
			if localIssue.GitHubID > 0 {
				continue
			}
			
			// Push to GitHub if it's a local-only issue
			if localIssue.SyncDirection == "push" || localIssue.SyncDirection == "both" {
				s.pushIssueToGitHub(ctx, client, repo, localIssue)
			}
		}
	}
	
	return nil
}

// syncPullRequests syncs pull requests between local and GitHub
func (s *GitHubSyncService) syncPullRequests(ctx context.Context, client *GitHubClient, repo *models.Repository) error {
	// Get PRs from GitHub
	githubPRs, err := client.ListPullRequests(ctx, repo.GitHubURL)
	if err != nil {
		return fmt.Errorf("failed to list GitHub pull requests: %w", err)
	}
	
	// Get local PRs
	localPRs, err := models.PullRequests.Search("WHERE RepoID = ?", repo.ID)
	if err != nil {
		return fmt.Errorf("failed to get local pull requests: %w", err)
	}
	
	// Create maps for efficient lookup
	localByGitHubID := make(map[int64]*models.PullRequest)
	localByGitHubNumber := make(map[int]*models.PullRequest)
	for _, pr := range localPRs {
		if pr.GitHubID > 0 {
			localByGitHubID[pr.GitHubID] = pr
		}
		if pr.GitHubNumber > 0 {
			localByGitHubNumber[pr.GitHubNumber] = pr
		}
	}
	
	// Process GitHub PRs
	for _, ghPR := range githubPRs {
		// Check if PR already exists locally
		localPR, exists := localByGitHubID[ghPR.ID]
		if !exists {
			localPR, exists = localByGitHubNumber[ghPR.Number]
		}
		
		if exists {
			// Update existing PR if needed
			if s.shouldUpdateLocalPR(localPR, ghPR) {
				s.updateLocalPR(localPR, ghPR)
			}
		} else {
			// Create new local PR
			s.createLocalPR(repo, ghPR)
		}
	}
	
	return nil
}

// shouldUpdateLocalIssue determines if a local issue needs updating from GitHub
func (s *GitHubSyncService) shouldUpdateLocalIssue(local *models.Issue, github *GitHubIssue) bool {
	// Skip if sync direction doesn't allow pulls
	if local.SyncDirection == "push" || local.SyncDirection == "none" {
		return false
	}
	
	// Update if GitHub was updated more recently
	return github.UpdatedAt.After(local.LastSyncAt)
}

// updateLocalIssue updates a local issue from GitHub data
func (s *GitHubSyncService) updateLocalIssue(local *models.Issue, github *GitHubIssue) {
	local.Title = github.Title
	local.Body = github.Body
	local.GitHubURL = github.HTMLURL
	local.GitHubState = github.State
	
	// Map GitHub state to local status
	if github.State == "closed" {
		local.Status = "closed"
	} else {
		local.Status = "open"
	}
	
	local.LastSyncAt = time.Now()
	local.SyncStatus = "synced"
	
	if err := models.Issues.Update(local); err != nil {
		log.Printf("Failed to update issue %s: %v", local.ID, err)
		local.SyncStatus = "conflict"
		models.Issues.Update(local)
	}
}

// createLocalIssue creates a new local issue from GitHub data
func (s *GitHubSyncService) createLocalIssue(repo *models.Repository, github *GitHubIssue) {
	issue := &models.Issue{
		Title:         github.Title,
		Body:          github.Body,
		RepoID:        repo.ID,
		Status:        "open",
		GitHubNumber:  github.Number,
		GitHubID:      github.ID,
		GitHubURL:     github.HTMLURL,
		GitHubState:   github.State,
		LastSyncAt:    time.Now(),
		SyncStatus:    "synced",
		SyncDirection: repo.SyncDirection,
	}
	
	// Map GitHub state to local status
	if github.State == "closed" {
		issue.Status = "closed"
	}
	
	// Try to find author by GitHub username
	// For now, use the repository owner as author
	issue.AuthorID = repo.UserID
	
	if _, err := models.Issues.Insert(issue); err != nil {
		log.Printf("Failed to create issue from GitHub: %v", err)
	}
}

// shouldUpdateLocalPR determines if a local PR needs updating from GitHub
func (s *GitHubSyncService) shouldUpdateLocalPR(local *models.PullRequest, github *GitHubPullRequest) bool {
	// Skip if sync direction doesn't allow pulls
	if local.SyncDirection == "push" || local.SyncDirection == "none" {
		return false
	}
	
	// Update if GitHub was updated more recently
	return github.UpdatedAt.After(local.LastSyncAt)
}

// updateLocalPR updates a local PR from GitHub data
func (s *GitHubSyncService) updateLocalPR(local *models.PullRequest, github *GitHubPullRequest) {
	local.Title = github.Title
	local.Body = github.Body
	local.GitHubURL = github.HTMLURL
	local.GitHubState = github.State
	local.GitHubMerged = github.Merged
	if github.MergedAt != nil {
		local.GitHubMergedAt = *github.MergedAt
	}
	
	// Map GitHub state to local status
	if github.Merged {
		local.Status = "merged"
	} else if github.State == "closed" {
		local.Status = "closed"
	} else {
		local.Status = "open"
	}
	
	local.LastSyncAt = time.Now()
	local.SyncStatus = "synced"
	
	if err := models.PullRequests.Update(local); err != nil {
		log.Printf("Failed to update PR %s: %v", local.ID, err)
		local.SyncStatus = "conflict"
		models.PullRequests.Update(local)
	}
}

// createLocalPR creates a new local PR from GitHub data
func (s *GitHubSyncService) createLocalPR(repo *models.Repository, github *GitHubPullRequest) {
	pr := &models.PullRequest{
		Title:          github.Title,
		Body:           github.Body,
		RepoID:         repo.ID,
		Status:         "open",
		BaseBranch:     github.Base.Ref,
		CompareBranch:  github.Head.Ref,
		GitHubNumber:   github.Number,
		GitHubID:       github.ID,
		GitHubURL:      github.HTMLURL,
		GitHubState:    github.State,
		GitHubMerged:   github.Merged,
		LastSyncAt:     time.Now(),
		SyncStatus:     "synced",
		SyncDirection:  repo.SyncDirection,
	}
	
	if github.MergedAt != nil {
		pr.GitHubMergedAt = *github.MergedAt
	}
	
	// Map GitHub state to local status
	if github.Merged {
		pr.Status = "merged"
	} else if github.State == "closed" {
		pr.Status = "closed"
	}
	
	// Try to find author by GitHub username
	// For now, use the repository owner as author
	pr.AuthorID = repo.UserID
	
	if _, err := models.PullRequests.Insert(pr); err != nil {
		log.Printf("Failed to create PR from GitHub: %v", err)
	}
}

// pushIssueToGitHub creates a GitHub issue from a local issue
func (s *GitHubSyncService) pushIssueToGitHub(ctx context.Context, client *GitHubClient, repo *models.Repository, issue *models.Issue) {
	// Create issue on GitHub
	ghIssue, err := client.CreateIssue(ctx, repo.GitHubURL, issue.Title, issue.Body)
	if err != nil {
		log.Printf("Failed to push issue to GitHub: %v", err)
		issue.SyncStatus = "conflict"
		models.Issues.Update(issue)
		return
	}
	
	// Update local issue with GitHub data
	issue.GitHubNumber = ghIssue.Number
	issue.GitHubID = ghIssue.ID
	issue.GitHubURL = ghIssue.HTMLURL
	issue.GitHubState = ghIssue.State
	issue.LastSyncAt = time.Now()
	issue.SyncStatus = "synced"
	
	if err := models.Issues.Update(issue); err != nil {
		log.Printf("Failed to update issue after push: %v", err)
	}
}

// pushPullRequestToGitHub pushes a local pull request to GitHub
func (s *GitHubSyncService) pushPullRequestToGitHub(ctx context.Context, client *GitHubClient, repo *models.Repository, pr *models.PullRequest) {
	// Create pull request on GitHub
	ghPR, err := client.CreatePullRequest(ctx, repo.GitHubURL, pr.Title, pr.Body, pr.CompareBranch, pr.BaseBranch)
	if err != nil {
		log.Printf("Failed to create PR on GitHub: %v", err)
		pr.SyncStatus = "error"
		pr.SyncDirection = "push"
		models.PullRequests.Update(pr)
		return
	}
	
	// Update local PR with GitHub data
	pr.GitHubNumber = ghPR.Number
	pr.GitHubID = ghPR.ID
	pr.GitHubURL = ghPR.HTMLURL
	pr.GitHubState = ghPR.State
	pr.LastSyncAt = time.Now()
	pr.SyncStatus = "synced"
	pr.SyncDirection = "push"
	
	if err := models.PullRequests.Update(pr); err != nil {
		log.Printf("Failed to update PR after push: %v", err)
	}
}

// SyncPullRequest syncs a single pull request with GitHub
func (s *GitHubSyncService) SyncPullRequest(prID, userID string) error {
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}
	
	repo, err := models.Repositories.Get(pr.RepoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}
	
	if repo.GitHubURL == "" {
		return fmt.Errorf("repository has no GitHub integration")
	}
	
	client, err := NewGitHubClient(userID)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	
	ctx := context.Background()
	
	// If PR has GitHub ID, sync status with GitHub
	if pr.GitHubNumber > 0 {
		// Update PR state on GitHub if local state changed
		if pr.Status == "closed" && pr.GitHubState != "closed" {
			_, err = client.UpdatePullRequest(ctx, repo.GitHubURL, pr.GitHubNumber, "", "", "closed")
			if err != nil {
				log.Printf("Failed to close PR on GitHub: %v", err)
			} else {
				pr.GitHubState = "closed"
				pr.LastSyncAt = time.Now()
				pr.SyncStatus = "synced"
				models.PullRequests.Update(pr)
			}
		}
		
		// If PR was merged locally, merge on GitHub
		if pr.Status == "merged" && !pr.GitHubMerged {
			mergeMsg := fmt.Sprintf("Merge pull request #%d: %s", pr.GitHubNumber, pr.Title)
			err = client.MergePullRequest(ctx, repo.GitHubURL, pr.GitHubNumber, mergeMsg, "", "merge")
			if err != nil {
				log.Printf("Failed to merge PR on GitHub: %v", err)
			} else {
				pr.GitHubMerged = true
				pr.GitHubMergedAt = time.Now()
				pr.LastSyncAt = time.Now()
				pr.SyncStatus = "synced"
				models.PullRequests.Update(pr)
			}
		}
		
		return nil
	} else {
		// Push to GitHub if sync direction allows
		if pr.SyncDirection == "push" || pr.SyncDirection == "both" {
			s.pushPullRequestToGitHub(ctx, client, repo, pr)
		}
		return nil
	}
}

// SyncIssue syncs a single issue with GitHub
func (s *GitHubSyncService) SyncIssue(issueID, userID string) error {
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}
	
	repo, err := models.Repositories.Get(issue.RepoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}
	
	if repo.GitHubURL == "" {
		return fmt.Errorf("repository has no GitHub integration")
	}
	
	client, err := NewGitHubClient(userID)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	
	ctx := context.Background()
	
	// If issue has GitHub ID, update from GitHub
	if issue.GitHubNumber > 0 {
		// For now, we'll need to list all issues and find the matching one
		// In a real implementation, we'd have a GetIssue method
		githubIssues, err := client.ListIssues(ctx, repo.GitHubURL)
		if err != nil {
			return fmt.Errorf("failed to list GitHub issues: %w", err)
		}
		
		for _, ghIssue := range githubIssues {
			if ghIssue.Number == issue.GitHubNumber {
				s.updateLocalIssue(issue, ghIssue)
				return nil
			}
		}
		
		// Issue not found on GitHub
		issue.SyncStatus = "conflict"
		models.Issues.Update(issue)
		return fmt.Errorf("issue not found on GitHub")
	} else {
		// Push to GitHub
		s.pushIssueToGitHub(ctx, client, repo, issue)
		return nil
	}
}

// StartBackgroundSync starts a background sync process that runs periodically
func (s *GitHubSyncService) StartBackgroundSync(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			s.syncAllRepositories()
		}
	}()
}

// syncAllRepositories syncs all repositories with GitHub integration enabled
func (s *GitHubSyncService) syncAllRepositories() {
	repos, err := models.Repositories.Search("WHERE GitHubURL != '' AND AutoSync = 1")
	if err != nil {
		log.Printf("Failed to get repositories for sync: %v", err)
		return
	}
	
	for _, repo := range repos {
		// Get integration data to find owner
		integration, err := models.GetGitHubRepoIntegration(repo.ID)
		if err != nil {
			continue
		}
		
		if ownerID, ok := integration["owner_id"].(string); ok {
			if err := s.SyncRepository(repo.ID, ownerID); err != nil {
				log.Printf("Failed to sync repository %s: %v", repo.Name, err)
			}
		}
	}
}