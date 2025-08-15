# Error Handling Patterns for Skyscape Workspace

## Standard Error Response Pattern

### For Web Handlers

```go
func (c *Controller) handler(w http.ResponseWriter, r *http.Request) {
    // 1. Authentication Check
    auth := c.App.Use("auth").(*authentication.Controller)
    user, _, err := auth.Authenticate(r)
    if err != nil {
        c.Render(w, r, "error-message.html", errors.New("authentication required"))
        return
    }

    // 2. Input Validation
    id := r.PathValue("id")
    if id == "" {
        c.Render(w, r, "error-message.html", errors.New("invalid request: missing ID"))
        return
    }

    // 3. Permission Check (Binary: Admin or Public Repo)
    repo, err := models.Repositories.Get(id)
    if err != nil {
        c.Render(w, r, "error-message.html", errors.New("repository not found"))
        return
    }
    if repo.Visibility != "public" && !user.IsAdmin {
        c.Render(w, r, "error-message.html", errors.New("access denied"))
        return
    }

    // 4. Business Logic
    result, err := performOperation(id)
    if err != nil {
        // Log internal error, show user-friendly message
        log.Printf("Operation failed for user %s: %v", user.ID, err)
        c.Render(w, r, "error-message.html", errors.New("operation failed, please try again"))
        return
    }

    // 5. Success Response
    c.Refresh(w, r)  // For HTMX partial updates
    // OR
    c.Redirect(w, r, "/success-page")  // For full page navigation
}
```

### For Model Methods

```go
func DoSomething(param string) (*Result, error) {
    // Validate inputs
    if param == "" {
        return nil, errors.New("parameter is required")
    }

    // Perform operation
    result, err := database.Query(param)
    if err != nil {
        // Wrap with context
        return nil, fmt.Errorf("failed to query database: %w", err)
    }

    // Validate result
    if result == nil {
        return nil, errors.New("no results found")
    }

    return result, nil
}
```

## Error Types and Responses

### Authentication Errors
```go
if err != nil {
    c.Render(w, r, "error-message.html", errors.New("authentication required"))
    return
}
```

### Authorization Errors
```go
if !models.HasPermission(user.ID, resource.ID, requiredRole) {
    c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
    return
}
```

### Validation Errors
```go
if title == "" {
    c.Render(w, r, "error-message.html", errors.New("title is required"))
    return
}
```

### Not Found Errors
```go
repo, err := models.GitRepos.Get(id)
if err != nil {
    c.Render(w, r, "error-message.html", errors.New("repository not found"))
    return
}
```

### Operation Failures
```go
if err := workspace.Start(); err != nil {
    log.Printf("Failed to start workspace %s: %v", workspace.ID, err)
    c.Render(w, r, "error-message.html", errors.New("failed to start workspace"))
    return
}
```

## Best Practices

### 1. User-Friendly Messages
```go
// Bad: Show internal error to user
c.Render(w, r, "error-message.html", err)  // "pq: duplicate key value"

// Good: Show friendly message
c.Render(w, r, "error-message.html", errors.New("this name is already taken"))
```

### 2. Log Internal Errors
```go
// Always log the actual error for debugging
log.Printf("Database error for user %s: %v", user.ID, err)

// Show generic message to user
c.Render(w, r, "error-message.html", errors.New("something went wrong"))
```

### 3. Consistent Error Format
```go
// Always use lowercase, no punctuation
errors.New("repository not found")      // Good
errors.New("Repository not found.")     // Bad
errors.New("Error: Repo not found!")    // Bad
```

### 4. Early Returns
```go
// Check errors early and return
if err != nil {
    c.Render(w, r, "error-message.html", err)
    return
}

// Continue with success path
// This keeps code flat and readable
```

### 5. Wrap Errors with Context
```go
// In models/functions
if err != nil {
    return nil, fmt.Errorf("failed to create workspace: %w", err)
}
```

## HTMX Error Handling

### For Forms
```html
<!-- Form will be replaced with error message -->
<form hx-post="/action" hx-target="body" hx-swap="outerHTML">
    <!-- If error, entire body is replaced with error-message.html -->
</form>
```

### For Partial Updates
```go
// On error, render just the error message
if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    c.Render(w, r, "partials/error-alert.html", err)
    return
}

// On success, refresh the current view
c.Refresh(w, r)
```

## Template Error Display

### error-message.html
```html
{{template "layout/start"}}
<div class="container mx-auto p-4">
    <div class="alert alert-error">
        <svg xmlns="http://www.w3.org/2000/svg" class="stroke-current shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span>{{.Error}}</span>
    </div>
    <div class="mt-4">
        <button class="btn btn-primary" onclick="history.back()">Go Back</button>
    </div>
</div>
{{template "layout/end"}}
```

### Inline Error Alert (Partial)
```html
<div class="alert alert-error alert-sm">
    <span>{{.Error}}</span>
</div>
```

## Common Error Scenarios

### Repository Access
```go
func (c *ReposController) getRepo(r *http.Request) (*models.GitRepo, error) {
    id := r.PathValue("id")
    if id == "" {
        return nil, errors.New("repository ID required")
    }

    repo, err := models.GitRepos.Get(id)
    if err != nil {
        return nil, errors.New("repository not found")
    }

    user, _, _ := c.auth.Authenticate(r)
    if err := models.CheckRepoAccess(user, id, models.RoleRead); err != nil {
        return nil, errors.New("access denied")
    }

    return repo, nil
}
```

### Workspace Operations
```go
func (c *WorkspacesController) startWorkspace(w http.ResponseWriter, r *http.Request) {
    workspace, err := c.getCurrentWorkspaceFromRequest(r)
    if err != nil {
        c.Render(w, r, "error-message.html", err)
        return
    }

    // Start asynchronously to avoid timeout
    go func() {
        if err := workspace.Start(user, nil); err != nil {
            log.Printf("Failed to start workspace %s: %v", workspace.ID, err)
            // Could send notification or update status
        }
    }()

    c.Refresh(w, r)
}
```

## Testing Error Cases

```go
// Test authentication required
resp := httptest.NewRecorder()
req := httptest.NewRequest("GET", "/protected", nil)
handler.ServeHTTP(resp, req)
assert.Equal(t, 401, resp.Code)

// Test permission denied
req = httptest.NewRequest("GET", "/repos/123", nil)
req.Header.Set("Authorization", "Bearer invalid")
handler.ServeHTTP(resp, req)
assert.Contains(t, resp.Body.String(), "access denied")
```