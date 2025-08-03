# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with the **Skyscape Workspace** application.

## Overview

**Skyscape Workspace** is a production-ready, GitHub-like developer platform built with TheSkyscape DevTools. It provides comprehensive repository management, containerized development environments, and AI-powered development tools in a mobile-responsive interface.

## Application Architecture

### MVC Pattern with TheSkyscape DevTools
- **Models**: Database entities with `Table()` methods and global repositories in `models/database.go`
- **Views**: HTML templates with HTMX integration and DaisyUI styling
- **Controllers**: HTTP handlers with factory functions returning `(string, *Controller)`

### Core Technologies
- **Backend**: Go with TheSkyscape DevTools MVC framework
- **Frontend**: HTMX + DaisyUI + TailwindCSS (mobile-first responsive design)
- **Database**: SQLite3 with automatic migrations and typed repositories
- **Authentication**: JWT with bcrypt password hashing
- **Containerization**: Docker for VS Code development environments

## Project Structure

```
workspace/
├── controllers/
│   ├── assistant.go      # AI assistant integration
│   ├── home.go          # Homepage with public/private views
│   ├── public.go        # Public repository access
│   ├── repos.go         # Repository management (main controller)
│   └── workspaces.go    # Container workspace management
├── internal/
│   └── coding/          # Git repository and workspace management
│       ├── git-blob.go  # Git blob operations
│       ├── git-repo.go  # Git repository model
│       ├── repository.go # Repository manager
│       ├── tokens.go    # Access token management
│       ├── workspace.go # Workspace model
│       └── resources/   # Shell scripts for workspace setup
├── models/
│   ├── action.go        # AI actions and automation
│   ├── database.go      # Global DB setup and repositories
│   ├── issue.go         # Issue tracking
│   ├── permission.go    # Role-based access control
│   ├── pullrequest.go   # Pull request management
│   └── todo.go          # Task management
├── views/
│   ├── partials/        # Reusable template components
│   ├── public/          # Static assets (CSS, JS, images)
│   ├── home.html        # Public portfolio + authenticated dashboard
│   ├── repos-list.html  # Repository listing page
│   ├── repo-view.html   # Repository overview with README
│   ├── repo-files.html  # File browser with syntax highlighting
│   ├── repo-search.html # Code search with context
│   ├── repo-issues.html # Issue management
│   ├── repo-prs.html    # Pull request management
│   └── workspace-launcher.html # VS Code workspace control
├── main.go              # Application entry point
├── go.mod               # Go module dependencies
├── README.md            # Project documentation
└── CLAUDE.md           # This file
```

## Key Features Implemented

### ✅ Repository Management
- **File Browsing**: Complete directory navigation with file type detection
- **Code Search**: Regex-based search with context highlighting
- **README Rendering**: Automatic markdown parsing and display
- **Permission System**: Role-based access (read/write/admin)
- **Git Integration**: Repository creation, cloning, commit history

### ✅ Project Management
- **Issues**: Full CRUD with status management (open/closed/reopen)
- **Pull Requests**: Create, merge, close with branch selection
- **Actions**: AI automation workflow management
- **Activity Logging**: Comprehensive audit trail

### ✅ Development Environment
- **Containerized Workspaces**: Docker-based VS Code environments
- **One-click Launch**: Instant development environment setup
- **Port Management**: Automatic port allocation for workspaces

### ✅ User Experience
- **Mobile Responsive**: Mobile-first design with collapsible navigation
- **Public Portfolio**: Developer showcase with public repositories
- **Authentication**: Secure JWT-based sessions
- **Clean UI**: GitHub-like interface with DaisyUI components

## Development Patterns

### Controller Factory Pattern
```go
// controllers/repos.go
func Repos() (string, *ReposController) {
    return "repos", &ReposController{}
}

func (c *ReposController) Setup(app *application.App) {
    // Register routes and middleware
    auth := app.Use("auth").(*authentication.Controller)
    http.Handle("GET /repos/{id}", app.Serve("repo-view.html", auth.Required))
}

func (c *ReposController) Handle(req *http.Request) application.Controller {
    c.Request = req
    return c
}
```

### Model Pattern
```go
// models/issue.go
type Issue struct {
    application.Model
    Title      string
    Body       string
    Status     string // "open", "closed"
    RepoID     string
    AssigneeID string
    Tags       string
}

func (*Issue) Table() string { return "issues" }
```

### Template Integration
```html
<!-- Views access controllers as {{controllerName.Method}} -->
{{with repos.CurrentRepo}}
    <h1>{{.Name}}</h1>
    {{range repos.RepoIssues}}
        <div>{{.Title}} - {{.Status}}</div>
    {{end}}
{{end}}

<!-- HTMX for dynamic updates -->
<form hx-post="/repos/{{.ID}}/issues/create" hx-target="body" hx-swap="outerHTML">
    <input name="title" required>
    <button type="submit">Create Issue</button>
</form>
```

### Permission System
```go
// Built on auth.Required with role hierarchy
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*coding.GitRepo, error) {
    // Check repository access permissions
    err = models.CheckRepoAccess(user, id, models.RoleRead)
    if err != nil {
        return nil, errors.New("access denied: " + err.Error())
    }
    return repo, nil
}
```

### Internal Coding Package
```go
// workspace/internal/coding provides Git repository management
import "workspace/internal/coding"

// Repository operations are self-contained
repo, err := models.Coding.GetRepo(repoID)
workspace := models.Coding.CreateWorkspace(repoID)
```

## Environment Variables

### Required
- `AUTH_SECRET` - JWT signing secret (required for authentication)

### Optional
- `PORT` - Server port (default: 5000)
- `THEME` - DaisyUI theme (default: corporate)
- `CONGO_SSL_FULLCHAIN` - SSL certificate path
- `CONGO_SSL_PRIVKEY` - SSL private key path

## Development Commands

```bash
# Run application
export AUTH_SECRET="your-secret-key"
go run .

# Build for production
go build -o workspace

# Run tests
go test ./...

# Update dependencies
go mod tidy
```

## Key Files and Their Purpose

### Controllers
- **`repos.go`** - Main repository controller with file browsing, search, issues, PRs
- **`home.go`** - Homepage controller with public portfolio and dashboard
- **`public.go`** - Public repository access for unauthenticated users
- **`workspaces.go`** - Docker workspace lifecycle management
- **`assistant.go`** - AI-powered development assistance

### Internal Packages
- **`internal/coding/`** - Self-contained Git repository and workspace management
  - **`repository.go`** - Repository manager with database integration
  - **`git-repo.go`** - Git repository model and operations
  - **`workspace.go`** - Docker workspace model and lifecycle
  - **`tokens.go`** - Access token management for Git operations

### Models
- **`database.go`** - Global database setup with typed repositories
- **`permission.go`** - Role-based access control system
- **`issue.go`** - Issue tracking with status management
- **`pullrequest.go`** - Pull request workflow management

### Views
- **`repo-view.html`** - Repository overview with README rendering
- **`repo-files.html`** - File browser with syntax highlighting
- **`repo-search.html`** - Code search with context results
- **`home.html`** - Public portfolio + authenticated dashboard

## Security Considerations

### Authentication & Authorization
- JWT tokens with secure httpOnly cookies
- Role-based permissions (read/write/admin)
- Repository ownership with explicit permission grants

### Input Validation
- Path traversal protection for file access
- SQL injection protection via typed repositories
- XSS prevention through template escaping

### File System Security
- Repository files sandboxed within `repos/` directory
- Binary file detection to prevent malicious uploads
- Validated file paths for all file operations

## Mobile Responsiveness

### Navigation
- Hamburger menu for mobile devices
- Responsive logo and button sizing
- Collapsible sidebar on small screens

### Content Layout
- Responsive grid layouts (1 column mobile, 3+ desktop)
- Hidden table columns on small screens
- Touch-optimized button sizes
- Mobile-first CSS classes throughout

### Forms and Modals
- Full-screen modals on mobile devices
- Stacked form layouts for narrow screens
- Accessible form controls with proper labeling

## Deployment

### Using TheSkyscape launch-app
```bash
go build -o workspace
export DIGITAL_OCEAN_API_KEY="your-token"
./launch-app --name workspace --domain workspace.example.com --binary ./workspace
```

### Manual Deployment
```bash
# Build application
go build -o workspace

# Set environment variables
export AUTH_SECRET="your-production-secret"
export PORT=8080

# Run application
./workspace
```

## Integration Points

### Docker Runtime
- All workspace operations require Docker daemon
- Automatic container lifecycle management
- Port allocation and cleanup

### File System
- Repository files stored in `repos/` directory
- Template views embedded at build time
- Static assets served from `views/public/`

### Database
- SQLite3 with automatic table creation
- Type-safe repositories with Go generics
- Migration-free schema updates

## Error Handling

### Template Rendering
```go
// Use consistent error templates
c.Render(w, r, "error-message.html", errors.New("something went wrong"))

// Use c.Refresh(w, r) for HTMX updates after successful operations
```

### Permission Failures
```go
// Check permissions before operations
err := models.CheckRepoAccess(user, repoID, models.RoleWrite)
if err != nil {
    c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
    return
}
```

## Common Development Tasks

### Adding New Routes
```go
// In controller Setup method
http.Handle("GET /new-route", app.Serve("template.html", auth.Required))
http.Handle("POST /new-action", app.ProtectFunc(c.handler, auth.Required))
```

### Creating New Models
```go
// 1. Define model with Table() method
type NewModel struct {
    application.Model
    Field string
}
func (*NewModel) Table() string { return "new_models" }

// 2. Add repository to models/database.go
var NewModels *database.Repository[NewModel]

// 3. Initialize in setupDatabase()
NewModels = database.Manage(DB, new(NewModel))
```

### Adding Template Helpers
```go
// Controllers methods are accessible as {{controllerName.Method}}
func (c *Controller) PublicMethod() ([]Data, error) {
    // This method can be called from templates
    return data, nil
}
```

## Production Readiness

### Performance
- Embedded views for zero-disk I/O
- Efficient SQLite3 with connection pooling
- Optimized Docker container management

### Reliability
- Comprehensive error handling
- Graceful degradation for missing features
- Robust permission system

### Scalability
- Stateless application design
- Container-based workspace isolation
- Database connection management

---

This application represents a complete, production-ready GitHub alternative with modern web technologies and mobile-first design principles.