package templates

// Template data structures for common partial templates
// These types are used for validation by check-views tool

// ErrorData is used by partials/error-alert.html and partials/monitoring-error.html
type ErrorData struct {
	Error string
}

// SuccessData is used by partials/success-alert.html
type SuccessData struct {
	Success string
}

// InfiniteScrollData is used by partials/infinite-scroll-trigger.html for pagination
type InfiniteScrollData struct {
	HasMore  bool
	NextPage int
	Endpoint string
}

// GitHubRepoData represents a GitHub repository from the API
// Used by partials/github-repos-import.html
type GitHubRepoData struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	HTMLURL         string `json:"html_url"`
	Private         bool   `json:"private"`
	Language        string `json:"language"`
	StargazersCount int    `json:"stargazers_count"`

	// Template compatibility methods - allows lowercase access in templates
	// This approach maintains proper Go conventions while supporting template usage
}

// MonitoringErrorData is used for monitoring error displays
type MonitoringErrorData struct {
	Error string
}

// PaginationData is a generic pagination structure
type PaginationData struct {
	CurrentPage int
	TotalPages  int
	HasNext     bool
	HasPrev     bool
	NextPage    int
	PrevPage    int
	Items       any // Can be any slice of items
}
