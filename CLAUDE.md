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
- 🚀 **Containerized Development** - Docker-based VS Code (Coder) and Jupyter environments  
- 🤖 **CI/CD Actions** - Docker sandbox execution with artifact collection
- 📋 **Project Management** - Issues, PRs, and automation
- 🔗 **GitHub Integration** - Bidirectional sync and OAuth
- 🔒 **Secret Management** - HashiCorp Vault for secure credential storage
- 👥 **Access Control** - Role-based permissions (read/write/admin)
- 🧠 **AI Assistant** - Ollama-powered chat with tool calling for repository operations

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

### 2. HTMX/HATEOAS Patterns
```html
<!-- Use c.Redirect/c.Refresh for happy path, hx-target for errors -->
<form hx-post="/action" hx-target=".error-container" hx-swap="innerHTML">

<!-- External links need hx-boost="false" -->
<a href="https://external.com" hx-boost="false" target="_blank">

<!-- IDE/workspace links open in new tab -->
<a href="/coder/{{.ID}}/" target="_blank">Open VS Code</a>
```

### 3. Template Organization
- Templates are in `views/` directory
- Use unique filenames (no paths in template references)
- Access controllers via prefix: `{{repos.CurrentRepo}}`
- Built-in helpers: `{{host}}`, `{{path}}`, `{{theme}}`

### 4. Model & Repository Pattern
```go
type Model struct {
    application.Model  // Embed for ID, CreatedAt, UpdatedAt
    // ... fields
}

func (*Model) Table() string { return "models" }

// Access via typed repositories
var Models = database.Manage(db, new(Model))
```

## Service Patterns

### Container Services
Services are singletons that manage Docker containers:
```go
type Service struct {
    container *containers.Service
    mu        sync.Mutex
}

func (s *Service) Init() error {
    // Always check IsRunning() to prevent duplicates
    // Initialize in background to prevent blocking
    go func() {
        s.container = &containers.Service{
            Name:          "service-name",
            RestartPolicy: "always",
        }
        s.Start()
    }()
}
```

### Critical Services
- **VaultService** - Secret management with automatic unseal
- **OllamaService** - Ollama container for AI features (qwen2.5-coder:1.5b model)
- **ActionsService** - CI/CD execution environment
- **NotebookService** - Jupyter notebook server

## Key Implementation Details

### AI Assistant System
The AI assistant provides an intelligent interface for repository operations:

1. **Architecture**:
   - Conversation-based system (not worker-based)
   - Admin-only access for all AI features
   - Ollama service runs qwen2.5-coder:1.5b model locally
   - Tool calling system for repository operations

2. **Available Tools**:
   - `list_repos` - Lists repositories with visibility filtering
   - `get_repo` - Gets detailed repository information
   - `create_repo` - Creates new repositories (admin only)
   - `get_repo_link` - Generates links to repository pages

3. **Tool Call Format**:
   ```xml
   <tool_call>
   {"tool": "list_repos", "params": {"visibility": "all"}}
   </tool_call>
   ```

4. **Implementation Files**:
   - `controllers/ai.go` - Main AI controller with admin-only routes
   - `models/conversation.go` - Conversation model
   - `models/message.go` - Message model with roles (user/assistant/tool/error)
   - `internal/ai/tools.go` - Tool registry and interface
   - `internal/ai/parser.go` - Tool call parser (supports XML and JSON formats)
   - `internal/ai/tools/repos.go` - Repository tool implementations

5. **Key Design Decisions**:
   - Simplified from worker-based to conversation-based system
   - Parser handles both XML-wrapped and plain JSON tool calls
   - Tools execute with user context for proper authorization
   - System prompt explicitly instructs when to use tools

### Actions System (CI/CD)
1. Actions run in Docker sandboxes (`action-{id}-{runid}`)
2. Repository mounted at `/workspace`
3. Commands wrapped in `bash -c` for proper execution
4. Artifacts collected and versioned
5. 5-minute cleanup delay after completion

### Repository Search (FTS5)
- SQLite Full-Text Search for code
- Automatic index updates on file changes
- Supports regex and fuzzy matching
- Language-aware tokenization

### Permissions System
```go
// Always check access before operations
err := models.CheckRepoAccess(user, repoID, models.RoleRead)
if err != nil {
    c.Render(w, r, "error-message.html", err)
    return
}
```

Roles: `read`, `write`, `admin`

### Path Security
```go
// Always validate paths to prevent traversal
if !isSubPath(basePath, requestedPath) {
    return errors.New("invalid path")
}
```

## Development Workflow

### Local Development
```bash
# Required environment
export AUTH_SECRET="dev-secret"

# Run locally (port 5000)
go run .

# Or build and run
make clean && make
./build/workspace
```

### Testing Changes
```bash
# Run tests
go test ./...

# Check for issues
go vet ./...

# Format code
go fmt ./...
```

### Deployment
```bash
# Build the application
make clean && make

# Deploy to test environment
cd /home/coder/skyscape
./devtools/build/launch-app deploy \
  --name workspace-test-env \
  --binary workspace/build/workspace

# Verify deployment
ssh root@SERVER_IP "docker logs sky-app --tail 50"
```

## Common Gotchas & Solutions

| Issue | Solution |
|-------|----------|
| Build fails | Use `make clean && make`, not `go build` |
| Redirect not working | Use `c.Redirect()` not `http.Redirect()` |
| Template not found | Check filename is unique, no paths |
| HTMX not updating | Use `c.Refresh()` or proper hx-target |
| Service won't start | Check `IsRunning()` first |
| Permission denied | Add `models.CheckRepoAccess()` |
| Action output missing | Wrap command in `bash -c` |
| Duplicate containers | Always mutex lock and check existence |

## Security Checklist

- ✅ Authentication: Use `auth.Required` middleware
- ✅ Authorization: Check `models.CheckRepoAccess()`
- ✅ Path validation: Use `isSubPath()` checks
- ✅ SQL injection: Use repositories (parameterized queries)
- ✅ XSS: Templates auto-escape by default
- ✅ CSRF: HTMX same-origin policy
- ✅ Secrets: Store in Vault, never in code

## File Structure

```
workspace/
├── controllers/     # HTTP handlers (MVC)
│   ├── repos.go    # Repository management
│   ├── issues.go   # Issue tracking
│   ├── prs.go      # Pull requests
│   ├── actions.go  # CI/CD
│   └── coder.go    # IDE management
├── models/         # Data models & repositories
├── services/       # Docker container services
├── views/          # HTML templates
├── coding/         # Git operations (internal)
├── auth/          # Authentication (moved from pkg)
└── Makefile       # Build configuration
```

## Environment Variables

### Required
- `AUTH_SECRET` - JWT signing key (required)

### Optional
- `PORT` - Server port (default: 5000)
- `THEME` - DaisyUI theme (default: corporate)
- `PREFIX` - URL prefix for reverse proxy
- `GITHUB_CLIENT_ID` - GitHub OAuth app ID
- `GITHUB_CLIENT_SECRET` - GitHub OAuth secret

## Quick Commands

```bash
# View logs
docker logs sky-app --tail 50

# Access database
sqlite3 ~/.skyscape/workspace.db

# Check services
docker ps | grep -E "(vault|ollama|jupyter)"

# Restart application
docker restart sky-app
```

## Recent Changes & Patterns

### Service Initialization
- Services now initialize asynchronously to prevent blocking
- Always use goroutines for Docker pulls and container starts
- Check `IsRunning()` before creating new containers

### HTMX Best Practices
- Controller methods `Refresh()` and `Redirect()` handle happy path
- Use `hx-target` only for error containers
- Forms should target error divs: `hx-target="previous .error-message"`
- Never use `hx-target="body"` with `hx-swap="outerHTML"`

### Template Variables
- Only use template variables for values accessed multiple times
- Single-use values should be called directly
- Cache expensive operations at template level: `{{$value := controller.Method}}`

### Error Handling
- Methods that can fail should return nil instead of error for optional data
- Example: `Workspace()` returns nil for orphaned servers
- This prevents template execution errors

## Integration Points

- **GitHub API** - Import, sync, OAuth
- **Docker API** - Container management
- **HashiCorp Vault** - Secret storage
- **SQLite FTS5** - Full-text search
- **HTMX** - Dynamic UI updates
- **DaisyUI v5** - Component library

## Performance Tips

1. Use lazy loading with `hx-trigger="revealed"` for heavy content
2. Cache template calculations: `{{$expensive := controller.Method}}`
3. Initialize services in background goroutines
4. Use database indexes for frequent queries
5. Implement pagination for large datasets

## Support & Documentation

- Main documentation: `/docs` route in application
- DevTools framework: https://github.com/The-Skyscape/devtools
- Issues: https://github.com/The-Skyscape/workspace/issues