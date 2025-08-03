# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with this workspace application.

## Project Overview

This is a **workspace** application built with TheSkyscape DevTools - a Go-based web application framework with built-in authentication, database management, and HTMX-powered UI.

## Architecture

This application follows the TheSkyscape DevTools MVC pattern:

### Core Structure

- **`controllers/`** - HTTP handlers with factory functions and Setup/Handle methods
- **`models/`** - Database models with Table() methods and global repository setup
- **`views/`** - HTML templates with HTMX integration and DaisyUI styling
- **`main.go`** - Application entry point with embedded views

### Key Design Patterns

- **Controller Factory Functions**: Return `(string, *Controller)` for registration
- **Global Database Setup**: DB, Auth, and repositories initialized in `models/database.go`
- **Template Naming**: Use unique filenames (not paths) for template rendering
- **HTMX Integration**: Dynamic updates with `c.Refresh(w, r)` after form submissions
- **Authentication**: JWT-based with `models.Auth.Required` middleware

## Development Commands

### Running the Application
```bash
# Set required environment variable
export AUTH_SECRET="your-super-secret-jwt-key"

# Run in development
go run .

# Build for production
go build -o app
```

### Testing
```bash
go test ./...
go test -v ./controllers
go test -race ./models
```

### Dependencies
```bash
go mod tidy
go mod download
```

## Environment Variables

### Required
- `AUTH_SECRET` - JWT signing secret (required for authentication)

### Optional
- `PORT` - Server port (default: 5000)
- `THEME` - DaisyUI theme (default: corporate)
- `CONGO_SSL_FULLCHAIN` - SSL certificate path
- `CONGO_SSL_PRIVKEY` - SSL private key path

## Application Patterns

### Controller Pattern
Controllers should:
1. Implement factory function returning `(string, *ControllerType)`
2. Embed `application.BaseController`
3. Implement `Setup(app *App)` to register routes
4. Implement `Handle(r *Request) Controller` returning instance
5. Use public methods for template access (e.g., `AllTodos()`)
6. Use private methods for HTTP handlers (e.g., `createTodo()`)

### Model Pattern
Models should:
1. Embed `application.Model` for ID, CreatedAt, UpdatedAt
2. Implement `Table() string` method
3. Use global repositories from `models/database.go`
4. Keep business logic in model methods

### Template Pattern
Templates should:
1. Use unique filenames to avoid namespace conflicts
2. Access controllers as `{{controllerName.Method}}`
3. Use layout components: `{{template "layout/start"}}`/ `{{template "layout/end"}}` 
4. Include HTMX attributes for dynamic behavior
5. Use DaisyUI classes for styling

## Error Handling

Use consistent error patterns:
```go
// In controllers, render errors using error template
c.Render(w, r, "error-message.html", errors.New("something went wrong"))

// Use c.Refresh(w, r) after successful operations for HTMX updates
```

## Security

- **Authentication**: Routes protected with `app.ProtectFunc()` and `models.Auth.Required`
- **Passwords**: Automatically hashed with bcrypt
- **Sessions**: JWT tokens with secure cookie settings
- **Environment**: Never commit `AUTH_SECRET` to git

## Database

- **Type**: SQLite3 with automatic table creation
- **Location**: `app.db` in working directory
- **Repositories**: Type-safe with Go generics
- **Queries**: Dynamic ORM with `Search()`, `Get()`, `Insert()`, `Update()`, `Delete()`

## Integration Points

- **HTMX**: All forms use HTMX for dynamic updates
- **DaisyUI**: Complete component library with theme support
- **Authentication**: Built-in signin/signup/signout flows
- **File System**: Views embedded at build time with `//go:embed all:views`

## Common Development Tasks

### Adding New Models
1. Create model in `models/newmodel.go`
2. Add repository to `models/database.go`
3. Create controller in `controllers/newmodels.go`
4. Register controller in `main.go`
5. Create templates in `views/`

### Adding Routes
```go
// In controller Setup method
app.Serve("GET /path", "template.html", models.Auth.Required)
app.ProtectFunc("POST /path", c.handler, models.Auth.Required)
```

### Template Helpers
- `{{theme}}` - Current DaisyUI theme
- `{{host}}` - Host prefix for URLs
- `{{auth.CurrentUser}}` - Current user
- `{{auth.SigninURL}}`, `{{auth.SignupURL}}` - Auth URLs

## Deployment

Build and deploy using launch-app:
```bash
go build -o app
curl -L -o launch-app https://github.com/The-Skyscape/devtools/releases/download/v1.0.1/launch-app
chmod +x launch-app
export DIGITAL_OCEAN_API_KEY="your-token"
./launch-app --name workspace --domain workspace.example.com --binary ./app
```

## Framework Documentation

- **DevTools Repository**: https://github.com/The-Skyscape/devtools
- **Tutorial**: https://github.com/The-Skyscape/devtools/blob/main/docs/tutorial.md
- **API Reference**: https://github.com/The-Skyscape/devtools/blob/main/docs/api.md