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
- 🔐 **Git Repository Management** - Create, browse, search repos with FTS5
- 🚀 **Ephemeral Workspaces** - Docker-based VS Code environments
- 🤖 **CI/CD Actions** - Docker sandbox execution with artifact collection
- 📋 **Project Management** - Issues, PRs, and automation
- 🔗 **GitHub Integration** - Bidirectional sync and OAuth
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

### Controllers (`/controllers/`) - Modular Architecture
| File | Purpose | Key Methods |
|------|---------|-------------|
| `repos.go` | Repository management | `CurrentRepo()`, `RepoFiles()`, delegates to other controllers |
| `actions.go` | CI/CD actions | `CurrentAction()`, `ActionRuns()`, `GroupedArtifacts()` |
| `issues.go` | Issue tracking | `RepoIssues()`, `CurrentIssue()`, `IssueComments()` |
| `pullrequests.go` | PR management | `RepoPullRequests()`, `CurrentPR()` |
| `workspaces.go` | Container management | `CurrentWorkspace()`, `UserWorkspaces()` |
| `integrations.go` | GitHub sync | `RepoIntegrations()`, `GitHubSync()` |
| `settings.go` | Settings management | `RepoSettings()`, `UserSettings()` |
| `home.go` | Dashboard & landing | `UserRepos()`, `RecentActivity()` |
| `public.go` | Unauthenticated access | `CurrentRepo()`, `PublicRepoIssues()` |

### Models (`/models/`)
| File | Purpose | Key Functions |
|------|---------|---------------|
| `database.go` | Global DB setup | `setupDatabase()` - Initializes all repositories |
| `action.go` | CI/CD workflows | `Execute()`, `CollectArtifacts()`, `monitorSandboxExecution()` |
| `action_run.go` | Execution history | `GetRunsByAction()`, `FormatDuration()` |
| `action_artifact.go` | Build artifacts | Versioned artifact storage with grouping |
| `workspace.go` | Workspace model | `Start()`, `Stop()`, `Service()` |
| `repository.go` | Git repos | Enhanced with FTS5 search support |
| `coding.go` | Git operations | `NewRepo()`, `GetWorkspaceByID()` |
| `permission.go` | Access control | `HasPermission()`, `CheckRepoAccess()` |

### Services (`/services/`)
| File | Purpose | Key Functions |
|------|---------|---------------|
| `sandbox.go` | Docker sandboxes | `StartSandbox()`, `GetOutput()`, `ExtractFile()` |
| `git.go` | Git operations | Repository management and operations |
| `workspace.go` | VS Code workspaces | Workspace lifecycle management |

### Views (`/views/`)
| File | Purpose | Controller |
|------|---------|------------|
| `home.html` | Dashboard/Landing | `home` |
| `repo-*.html` | Repository views | `repos` |
| `repo-action-*.html` | Action views (info, logs, history, artifacts) | `actions` |
| `repo-issues.html` | Issue tracking | `issues` |
| `repo-prs.html` | Pull requests | `pullrequests` |
| `repo-integrations.html` | GitHub integration | `integrations` |
| `workspace-*.html` | Workspace views | `workspaces` |
| `workspaces-list.html` | Workspace management | `workspaces` |

## Cross-Controller Communication

### Using Other Controllers
```go
// Access another controller from within a controller
func (c *ReposController) RepoIssues(r application.Request) {
    // Use the Use() method to get another controller
    issues := c.Use("issues").(*IssuesController)
    return issues.RepoIssues(r)
}
```

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

## Actions System (CI/CD)

### Architecture
```
User Trigger → Action Controller → Sandbox Service → Docker Container
                    ↓                    ↓              ↓
              ActionRun Record    Monitor Output    Collect Artifacts
                    ↓                    ↓              ↓
              Update Status        Stream Logs    Store in Database
```

### Action Execution Flow
1. User triggers action (manual/scheduled/event)
2. ActionRun record created with "running" status
3. Docker sandbox created with repository mounted at /workspace
4. Command executed in isolated environment
5. Output captured and streamed to logs
6. Artifacts collected based on configured paths
7. ActionRun updated with final status and metrics
8. Sandbox cleaned up after 5 minute delay

### Key Operations
```go
// Create action
action := &Action{
    Title: "Build and Test",
    Type: "manual",
    Command: "npm test && npm build",
    Branch: "main",
    ArtifactPaths: "dist/, coverage/",
}

// Execute action
err := action.Execute()

// Monitor in background
go action.monitorSandboxExecution(sandboxName, runID)

// Collect artifacts
err := action.CollectArtifacts(sandboxName, paths, runID)
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
9. **Controller not found** - Use `c.Use("controllerName")` to access other controllers
10. **Action output missing** - Wrap Docker command in `bash -c` for proper shell redirection
11. **Database column errors** - Check model uses `application.Model` for base fields

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
ssh root@SERVER_IP "docker ps -a"

# View application logs
ssh root@SERVER_IP "docker logs sky-app -n 50"

# Restart application
ssh root@SERVER_IP "docker restart sky-app"
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

## UI/UX Patterns

### Consistent Header Style
- All action/repo pages use same header layout
- Avatar placeholder with icon on left
- Title and metadata below
- Status badge and dropdown menu on right

### Tab Navigation
- Use route-based tabs (not JavaScript)
- Active tab indicated with `tab-active` class
- Each tab has its own route and template

### Borders and Cards
- Use `border border-base-300` for faint borders
- Cards should have `shadow-lg` for depth
- Consistent spacing with `gap-4` between sections

## Recent Architecture Changes (2025)

### Controller Refactoring
- Split monolithic `repos` controller into focused controllers
- Each controller handles single responsibility
- Cross-controller communication via `Use()` method

### Actions System Implementation
- Docker-based sandbox service for isolated execution
- Full execution history with ActionRun model
- Artifact versioning and grouping
- Tab-based UI for logs, history, artifacts

### Database Enhancements
- SQLite FTS5 for full-text search
- ActionRun model for execution tracking
- Enhanced artifact storage with versioning

---

**Remember**: When in doubt, follow existing patterns in the codebase. The modular controller architecture and consistent UI patterns are key to maintaining the application.