package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"workspace/models"
)

// SearchParams represents common search parameters
type SearchParams struct {
	Query         string
	Status        string
	IncludeClosed bool
	Page          int
	Limit         int
}

// GetSearchParams extracts search parameters from request
func GetSearchParams(r *http.Request) SearchParams {
	params := SearchParams{
		Query:         r.URL.Query().Get("q"),
		Status:        r.URL.Query().Get("status"),
		IncludeClosed: r.URL.Query().Get("includeClosed") == "true",
		Page:          1,
		Limit:         30,
	}

	// Parse page number
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			params.Page = page
		}
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			params.Limit = limit
		}
	}

	return params
}

// BuildSearchQuery builds a SQL query from search parameters
func BuildSearchQuery(baseTable string, params SearchParams, repoID string) (string, []any) {
	var conditions []string
	var args []any

	// Always filter by repository
	if repoID != "" {
		conditions = append(conditions, "RepoID = ?")
		args = append(args, repoID)
	}

	// Add status filter
	if !params.IncludeClosed {
		conditions = append(conditions, "Status = ?")
		args = append(args, "open")
	} else if params.Status != "" {
		conditions = append(conditions, "Status = ?")
		args = append(args, params.Status)
	}

	// Add search query
	if params.Query != "" {
		searchPattern := "%" + params.Query + "%"
		if baseTable == "issues" || baseTable == "pull_requests" {
			conditions = append(conditions, "(Title LIKE ? OR Body LIKE ?)")
			args = append(args, searchPattern, searchPattern)
		} else if baseTable == "repositories" {
			conditions = append(conditions, "(Name LIKE ? OR Description LIKE ?)")
			args = append(args, searchPattern, searchPattern)
		}
	}

	// Build WHERE clause
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Add ORDER BY
	orderBy := " ORDER BY CreatedAt DESC"
	if baseTable == "repositories" {
		orderBy = " ORDER BY UpdatedAt DESC"
	}

	// Add pagination
	offset := (params.Page - 1) * params.Limit
	pagination := fmt.Sprintf(" LIMIT %d OFFSET %d", params.Limit, offset)

	return whereClause + orderBy + pagination, args
}

// GetRepositoryWithAccess gets a repository and checks access in one operation
func GetRepositoryWithAccess(r *http.Request, needsWrite bool, auth *AuthController) (*models.Repository, error) {
	repoID := r.PathValue("id")
	if repoID == "" {
		return nil, fmt.Errorf("repository ID required")
	}

	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found")
	}

	// Check authentication and access
	user, _, err := auth.Authenticate(r)
	if err != nil {
		// Check if repository is public and read-only access is needed
		if repo.Visibility == "public" && !needsWrite {
			return repo, nil
		}
		return nil, fmt.Errorf("authentication required")
	}

	// For write access, must be admin
	if needsWrite && !user.IsAdmin {
		return nil, fmt.Errorf("admin access required")
	}

	// For read access on private repos, must be admin
	if repo.Visibility != "public" && !user.IsAdmin {
		return nil, fmt.Errorf("access denied - private repository")
	}

	return repo, nil
}

// FormatTimeAgo formats a time as "X ago" string
func FormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 30*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case duration < 365*24*time.Hour:
		months := int(duration.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(duration.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// FormatFileSize formats bytes into human-readable format
func FormatFileSize(bytes int64) string {
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

// GetStatusBadgeClass returns the DaisyUI badge class for a status
func GetStatusBadgeClass(status string) string {
	switch strings.ToLower(status) {
	case "open":
		return "badge badge-success"
	case "closed":
		return "badge badge-neutral"
	case "merged":
		return "badge badge-info"
	case "running":
		return "badge badge-warning"
	case "success":
		return "badge badge-success"
	case "failure", "failed":
		return "badge badge-error"
	case "pending":
		return "badge badge-ghost"
	default:
		return "badge"
	}
}

// GetPaginationInfo calculates pagination information
func GetPaginationInfo(totalCount, currentPage, itemsPerPage int) PaginationInfo {
	totalPages := (totalCount + itemsPerPage - 1) / itemsPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	// Calculate range of items being shown
	startItem := (currentPage-1)*itemsPerPage + 1
	endItem := currentPage * itemsPerPage
	if endItem > totalCount {
		endItem = totalCount
	}
	if startItem > totalCount {
		startItem = totalCount
	}

	return PaginationInfo{
		CurrentPage:  currentPage,
		TotalPages:   totalPages,
		TotalItems:   totalCount,
		ItemsPerPage: itemsPerPage,
		HasPrevious:  currentPage > 1,
		HasNext:      currentPage < totalPages,
		StartItem:    startItem,
		EndItem:      endItem,
	}
}

// PaginationInfo holds pagination metadata
type PaginationInfo struct {
	CurrentPage  int
	TotalPages   int
	TotalItems   int
	ItemsPerPage int
	HasPrevious  bool
	HasNext      bool
	StartItem    int
	EndItem      int
}

// ValidatePath ensures a file path is safe and within bounds
func ValidatePath(path string) error {
	// Remove any leading/trailing spaces
	path = strings.TrimSpace(path)

	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid path: path traversal not allowed")
	}

	// Check for absolute paths
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid path: absolute paths not allowed")
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("invalid path: null bytes not allowed")
	}

	return nil
}

// SanitizeInput removes potentially dangerous characters from user input
func SanitizeInput(input string, maxLength int) string {
	// Trim whitespace
	input = strings.TrimSpace(input)

	// Limit length
	if len(input) > maxLength {
		input = input[:maxLength]
	}

	// Remove control characters
	result := strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, input)

	return result
}
