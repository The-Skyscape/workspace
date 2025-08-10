# CLAUDE.md - Skyscape Workspace Development Guide

This file provides optimized guidance for Claude Code when working with the **Skyscape Workspace** application.

## Quick Start for Claude

When working on this codebase:
1. **Always use Makefile for builds**: `make clean && make` (NOT `go build` directly)
2. **Use c.Redirect not http.Redirect**: For HTMX compatibility
3. **Follow MVC patterns**: Controllers handle requests, Models handle data, Views handle presentation
4. **Test locally**: `export AUTH_SECRET="dev-secret" && go run .`
5. **Deploy pattern**: `cd /home/coder/skyscape && ./devtools/build/launch-app deploy --name workspace-test-env --binary workspace/build/workspace`

## Project Overview

**Skyscape Workspace** is a GitHub-like platform with containerized development environments. Think of it as self-hosted GitHub + Codespaces.

### Core Features
- 🔐 **Git Repository Management** - Create, browse, search repos
- 🚀 **Ephemeral Workspaces** - Docker-based VS Code environments
- 📋 **Project Management** - Issues, PRs, and automation
- 👥 **Access Control** - Role-based permissions (read/write/admin)

## Architecture Patterns

### 1. Controller Pattern
Every controller follows this structure:
```go
// Factory function returns prefix and instance
func ControllerName() (string, *ControllerNameController) {
    return "prefix", &ControllerNameController{}
}

// Setup registers routes
func (c *ControllerNameController) Setup(app *application.App) {
    auth := app.Use("auth").(*authentication.Controller)
    http.Handle("GET /route", app.Serve("template.html", auth.Required))
    http.Handle("POST /action", app.ProtectFunc(c.handler, auth.Required))
}

// Handle prepares controller for request
func (c *ControllerNameController) Handle(req *http.Request) application.Controller {
    c.Request = req
    return c
}
```

### 2. Model Pattern
Models must implement the Table() method:
```go
type ModelName struct {
    application.Model  // Embeds ID, CreatedAt, UpdatedAt
    Field1 string
    Field2 int
}

func (*ModelName) Table() string { return "table_name" }
```

### 3. View Pattern
Templates access controller methods directly:
```html
<!-- Access controller data -->
{{with controllerPrefix.MethodName}}
    <!-- Use the data -->
{{end}}

<!-- HTMX for dynamic updates -->
<form hx-post="{{host}}/path" hx-target="body" hx-swap="outerHTML">
```

## Key Files Reference

### Controllers (`/controllers/`)
| File | Purpose | Key Methods |
|------|---------|-------------|
| `repos.go` | Repository management | `CurrentRepo()`, `RepoFiles()`, `RepoIssues()` |
| `workspaces.go` | Container management | `CurrentWorkspace()`, `UserWorkspaces()` |
| `home.go` | Dashboard & landing | `UserRepos()`, `RecentActivity()` |
| `public.go` | Unauthenticated access | `CurrentRepo()`, `PublicRepoIssues()` |

### Models (`/models/`)
| File | Purpose | Key Functions |
|------|---------|---------------|
| `database.go` | Global DB setup | `setupDatabase()` - Initializes all repositories |
| `workspace.go` | Workspace model | `Start()`, `Stop()`, `Service()` |
| `coding.go` | Git operations | `NewRepo()`, `GetWorkspaceByID()` |
| `permission.go` | Access control | `HasPermission()`, `CheckRepoAccess()` |

### Views (`/views/`)
| File | Purpose | Controller |
|------|---------|------------|
| `home.html` | Dashboard/Landing | `home` |
| `repo-*.html` | Repository views | `repos` |
| `workspace-*.html` | Workspace views | `workspaces` |
| `workspaces-list.html` | Workspace management | `workspaces` |

## Common Tasks

### Adding a New Route
```go
// In controller's Setup() method
http.Handle("GET /new-route/{id}", app.Serve("template.html", auth.Required))
http.Handle("POST /new-route/{id}/action", app.ProtectFunc(c.handleAction, auth.Required))

// Handler method
func (c *Controller) handleAction(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    
    // Always use c.Redirect for redirects (HTMX compatibility)
    c.Redirect(w, r, "/success-page")
    
    // Or use c.Refresh for HTMX partial updates
    c.Refresh(w, r)
}
```

### Working with Models
```go
// Create
model := &ModelType{Field: "value"}
model, err := Models.Insert(model)

// Read
model, err := Models.Get(id)

// Update
model.Field = "new value"
err := Models.Update(model)

// Delete
err := Models.Delete(model)

// Search
results, err := Models.Search("WHERE field = ?", value)
```

### Template Helpers
```go
// Make data available to templates
func (c *Controller) PublicData() string {
    return "This is accessible as {{controller.PublicData}} in templates"
}

// Return complex data
func (c *Controller) ComplexData() ([]Model, error) {
    return Models.All()
}
```

## Workspace System

### Architecture
```
User Request → /coder/{workspace-id}/ → WorkspaceHandler → Docker Container
                                              ↓
                                    Persistent Volumes:
                                    - /home/coder/.config
                                    - /home/coder/project  
                                    - /workspace/repos/{repo-id}
```

### Key Operations
```go
// Create workspace
workspace, err := models.NewWorkspace(userID, port, repo)

// Start workspace
err := workspace.Start(user, repo)

// Access workspace
/coder/{workspace.ID}/

// Stop workspace  
err := workspace.Stop()
```

## Error Handling

### Standard Pattern
```go
func (c *Controller) handler(w http.ResponseWriter, r *http.Request) {
    // Get authenticated user
    auth := c.App.Use("auth").(*authentication.Controller)
    user, _, err := auth.Authenticate(r)
    if err != nil {
        c.Render(w, r, "error-message.html", errors.New("unauthorized"))
        return
    }
    
    // Check permissions
    err = models.CheckRepoAccess(user, repoID, models.RoleRead)
    if err != nil {
        c.Render(w, r, "error-message.html", err)
        return
    }
    
    // Success - use appropriate response
    c.Refresh(w, r)  // For HTMX partial update
    // OR
    c.Redirect(w, r, "/success")  // For full page redirect
}
```

## UI Development Patterns

### DaisyUI v5 Forms
```html
<!-- Use this pattern for forms (NOT validator component) -->
<form class="flex flex-col gap-2">
  <label class="form-control w-full">
    <div class="label">
      <span class="label-text text-sm font-medium">Field Name</span>
      <span class="label-text-alt text-xs">Helper text</span>
    </div>
    <input type="text" class="input input-bordered w-full" />
  </label>
</form>
```

### Spacing Preferences
- **ALWAYS use**: `flex flex-col gap-2` for form spacing
- **NEVER use**: `space-y-n` classes
- **Form spacing**: Use `gap-2` between form fields
- **Section spacing**: Use `gap-4` or `gap-6` between sections

### Template Helpers
- **Active nav states**: Use `{{if path_eq "route"}}class="active"{{end}}`
- **No HTMX on external links**: Regular `<a>` tags without `hx-boost="true"`
- **Open in new tab**: Add `target="_blank"` for IDE/external links

## Security Checklist

- ✅ **Authentication**: Use `auth.Required` middleware
- ✅ **Authorization**: Check `models.CheckRepoAccess()` 
- ✅ **Path Traversal**: Validate file paths with `isSubPath()`
- ✅ **SQL Injection**: Use parameterized queries via repositories
- ✅ **XSS**: Templates auto-escape, use `{{.Field}}` not `{{.Field | safe}}`

## Testing Patterns

### Local Development
```bash
# Set required environment
export AUTH_SECRET="dev-secret"

# Run with auto-reload
go run .

# Build and run
go build -o workspace && ./workspace
```

### Common Test Scenarios
1. **Repository Creation**: Sign in → Create Repo → Verify in list
2. **Workspace Launch**: Open repo → Launch Workspace → Verify /coder/ proxy
3. **Permissions**: Create private repo → Sign out → Verify 404
4. **Issue Creation**: Open repo → Create issue → Verify in list

## Docker Service Management

### Service Initialization Pattern
```go
// In controller Setup() to auto-start services
func (c *Controller) Setup(app *application.App) {
    // ... routes ...
    
    // Initialize service on startup
    if err := services.ServiceName.Init(); err != nil {
        log.Printf("Warning: Failed to initialize service: %v", err)
    }
}
```

### Preventing Duplicate Containers
```go
// In service Init() method
func (s *Service) Init() error {
    if s.IsRunning() {
        log.Println("Service already running")
        s.running = true
        return nil
    }
    // Start service...
}
```

### Container Restart Policy
```go
// Add to Service struct for persistent containers
service := &containers.Service{
    RestartPolicy: "always",  // Survives crashes and reboots
    // ... other config
}
```

## Debugging Tips

### Check Request Context
```go
// In any controller method
id := r.PathValue("id")  // Get path parameter
value := r.FormValue("field")  // Get form value
user, _, _ := auth.Authenticate(r)  // Get current user
```

### Template Debugging
```html
<!-- Show available data -->
<pre>{{printf "%+v" .}}</pre>

<!-- Check specific controller -->
<pre>{{printf "%+v" repos}}</pre>
```

### Common Issues & Solutions

1. **"undefined: time"** - Add `import "time"` to the file
2. **Template not found** - Check file exists in `/views/`
3. **Route not working** - Ensure it's registered in Setup()
4. **Permission denied** - Check HasPermission() logic
5. **SSH connection refused** - Wait a few seconds after deployment for SSH to restart
6. **Duplicate containers** - Check IsRunning() before creating new containers
7. **Build failures** - Always use `make clean && make` instead of direct `go build`
8. **Validator not working** - DaisyUI v5 doesn't have validator component, use HTML5 validation

## Performance Considerations

1. **Database Queries**: Use `Search()` with limits for large datasets
2. **File Operations**: Cache file stats when browsing
3. **Docker Containers**: Reuse existing workspaces when possible
4. **Templates**: Use partials for repeated components

## Code Style Guide

1. **Error Messages**: User-friendly, lowercase start
2. **HTTP Status**: Use proper codes (200, 404, 403, 500)
3. **Redirects**: Always use `c.Redirect()` not `http.Redirect()`
4. **Logs**: Use `log.Printf()` for debugging, remove in production

## Deployment Checklist

Before deploying changes:
1. ✅ Build with Makefile: `make clean && make`
2. ✅ Test locally if possible
3. ✅ Commit changes with descriptive message
4. ✅ Deploy to test environment first: `workspace-test-env`
5. ✅ Verify no duplicate containers after deployment
6. ✅ Check logs: `ssh root@IP "docker logs sky-app -n 50"`

### Standard Deployment Command
```bash
cd /home/coder/skyscape
./devtools/build/launch-app deploy \
  --name workspace-test-env \
  --binary workspace/build/workspace
```

## Quick Command Reference

```bash
# Build (ALWAYS use Makefile)
make clean && make

# Run tests
go test ./...

# Check for issues
go vet ./...

# Format code
go fmt ./...

# Update dependencies
go mod tidy

# Check container status on server
ssh root@64.23.187.176 "docker ps -a"

# View application logs
ssh root@64.23.187.176 "docker logs sky-app -n 50"

# Restart application
ssh root@64.23.187.176 "docker restart sky-app"
```

## Integration Points

### Docker Requirements
- Docker daemon must be running
- User must have docker permissions
- Ports 8000-9000 reserved for workspaces

### File System
- Repos stored in `./repos/`
- Templates in `./views/`
- Static assets in `./views/public/`

### Database
- SQLite file: `./workspace.db`
- Auto-creates tables on startup
- No manual migrations needed

---

**Remember**: When in doubt, follow existing patterns in the codebase. The controllers in `/controllers/repos.go` and `/controllers/workspaces.go` are good examples of the standard patterns used throughout the application.