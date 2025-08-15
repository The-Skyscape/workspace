package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Search controller prefix
func Search() (string, *SearchController) {
	return "search", &SearchController{}
}

// SearchController handles global search operations
type SearchController struct {
	application.BaseController
}

// Handle returns a new controller instance for the request
func (c SearchController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Setup registers routes
func (c *SearchController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Global search
	http.Handle("GET /search", app.Serve("search.html", auth.Required))
	http.Handle("GET /search/results", app.ProtectFunc(c.searchResults, auth.Required))
	http.Handle("GET /search/code", app.ProtectFunc(c.searchCode, auth.Required))
	http.Handle("GET /search/navbar", app.ProtectFunc(c.navbarSearch, auth.Required))
}

// SearchQuery returns the current search query
func (c *SearchController) SearchQuery() string {
	return c.Request.URL.Query().Get("q")
}

// SearchType returns the search type (all, code, repos, issues, prs)
func (c *SearchController) SearchType() string {
	searchType := c.Request.URL.Query().Get("type")
	if searchType == "" {
		searchType = "all"
	}
	return searchType
}

// SearchScope returns the search scope (all, current)
func (c *SearchController) SearchScope() string {
	scope := c.Request.URL.Query().Get("scope")
	if scope == "" {
		scope = "all"
	}
	return scope
}

// CurrentRepoID returns the current repository ID if searching within a repo
func (c *SearchController) CurrentRepoID() string {
	return c.Request.URL.Query().Get("repo")
}

// SearchRepositories searches for repositories
func (c *SearchController) SearchRepositories() ([]*models.Repository, error) {
	query := c.SearchQuery()
	if query == "" {
		return []*models.Repository{}, nil
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}

	// Search repositories by name or description
	searchPattern := "%" + query + "%"
	repos, err := models.Repositories.Search(
		"WHERE (Name LIKE ? OR Description LIKE ?) AND (Visibility = 'public' OR UserID = ?) ORDER BY UpdatedAt DESC LIMIT 10",
		searchPattern, searchPattern, user.ID,
	)
	
	return repos, err
}

// SearchIssues searches for issues across repositories
func (c *SearchController) SearchIssues() ([]*models.Issue, error) {
	query := c.SearchQuery()
	if query == "" {
		return []*models.Issue{}, nil
	}

	searchPattern := "%" + query + "%"
	scope := c.SearchScope()
	repoID := c.CurrentRepoID()

	var condition string
	var args []interface{}

	if scope == "current" && repoID != "" {
		condition = "WHERE RepoID = ? AND (Title LIKE ? OR Body LIKE ?) ORDER BY CreatedAt DESC LIMIT 10"
		args = []interface{}{repoID, searchPattern, searchPattern}
	} else {
		// Search across all accessible repositories
		condition = "WHERE Title LIKE ? OR Body LIKE ? ORDER BY CreatedAt DESC LIMIT 10"
		args = []interface{}{searchPattern, searchPattern}
	}

	return models.Issues.Search(condition, args...)
}

// SearchPullRequests searches for pull requests
func (c *SearchController) SearchPullRequests() ([]*models.PullRequest, error) {
	query := c.SearchQuery()
	if query == "" {
		return []*models.PullRequest{}, nil
	}

	searchPattern := "%" + query + "%"
	scope := c.SearchScope()
	repoID := c.CurrentRepoID()

	var condition string
	var args []interface{}

	if scope == "current" && repoID != "" {
		condition = "WHERE RepoID = ? AND (Title LIKE ? OR Body LIKE ?) ORDER BY CreatedAt DESC LIMIT 10"
		args = []interface{}{repoID, searchPattern, searchPattern}
	} else {
		condition = "WHERE Title LIKE ? OR Body LIKE ? ORDER BY CreatedAt DESC LIMIT 10"
		args = []interface{}{searchPattern, searchPattern}
	}

	return models.PullRequests.Search(condition, args...)
}

// SearchCodeFiles searches for code in files using FTS5
func (c *SearchController) SearchCodeFiles() ([]*models.FileSearchResult, error) {
	query := c.SearchQuery()
	if query == "" {
		return []*models.FileSearchResult{}, nil
	}

	scope := c.SearchScope()
	repoID := c.CurrentRepoID()

	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}

	// Use the FileSearch model to search code
	if models.Search == nil {
		return []*models.FileSearchResult{}, nil
	}

	var results []*models.FileSearchResult
	if scope == "current" && repoID != "" {
		results, err = models.Search.SearchInRepo(repoID, query, 20)
	} else {
		results, err = models.Search.SearchGlobal(query, user.ID, 20)
	}

	return results, err
}

// searchResults handles search results via HTMX
func (c *SearchController) searchResults(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		c.App.Render(w, r, "search-results.html", nil)
		return
	}

	// Render search results partial
	c.App.Render(w, r, "search-results.html", nil)
}

// searchCode handles code search via HTMX
func (c *SearchController) searchCode(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		c.App.Render(w, r, "search-code-results.html", nil)
		return
	}

	// Render code search results partial
	c.App.Render(w, r, "search-code-results.html", nil)
}

// navbarSearch handles navbar dropdown search results
func (c *SearchController) navbarSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		// Return empty dropdown
		w.Write([]byte(`<div class="text-center py-4 text-base-content/50 text-sm">Start typing to search...</div>`))
		return
	}

	// Render navbar search results partial
	c.App.Render(w, r, "search-navbar-results.html", nil)
}

// PageNumber returns the current page number for pagination
func (c *SearchController) PageNumber() int {
	pageStr := c.Request.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// HasResults checks if there are any search results
func (c *SearchController) HasResults() bool {
	query := c.SearchQuery()
	if query == "" {
		return false
	}

	// Check each search type for results
	searchType := c.SearchType()
	
	switch searchType {
	case "repos":
		repos, _ := c.SearchRepositories()
		return len(repos) > 0
	case "issues":
		issues, _ := c.SearchIssues()
		return len(issues) > 0
	case "prs":
		prs, _ := c.SearchPullRequests()
		return len(prs) > 0
	case "code":
		code, _ := c.SearchCodeFiles()
		return len(code) > 0
	default: // "all"
		repos, _ := c.SearchRepositories()
		issues, _ := c.SearchIssues()
		prs, _ := c.SearchPullRequests()
		code, _ := c.SearchCodeFiles()
		return len(repos) > 0 || len(issues) > 0 || len(prs) > 0 || len(code) > 0
	}
}

// HighlightCode returns syntax-highlighted code snippet
func (c *SearchController) HighlightCode(content string, query string) string {
	// Simple highlighting - wrap query matches in <mark> tags
	if query == "" {
		return content
	}
	
	// Case-insensitive replacement
	highlighted := strings.ReplaceAll(
		strings.ToLower(content),
		strings.ToLower(query),
		"<mark class='bg-yellow-300 text-black'>"+query+"</mark>",
	)
	
	return highlighted
}