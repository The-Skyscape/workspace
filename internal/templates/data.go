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
// Using struct with lowercase field names for template compatibility
type GitHubRepoData struct {
	// Lowercase field names to match template usage
	name            string
	description     string
	html_url        string
	private         bool
	language        string
	stargazers_count int
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
	Items       interface{} // Can be any slice of items
}