# Skyscape Workspace - Quick Reference

## 🚀 Common Operations

### Start Development Server
```bash
export AUTH_SECRET="dev-secret"
go run .
# OR
go build -o workspace && ./workspace
```

### Access Points
- **Home**: http://localhost:5000
- **Repos**: http://localhost:5000/repos
- **Workspaces**: http://localhost:5000/workspaces
- **VS Code**: http://localhost:5000/coder/{workspace-id}/

### Create Test Data
1. Sign up at http://localhost:5000/signup
2. Create repository via UI
3. Launch workspace from repository page
4. Access VS Code via /coder/ URL

## 📁 Project Structure

```
workspace/
├── controllers/     # HTTP request handlers
├── models/         # Database models & business logic  
├── views/          # HTML templates (HTMX + DaisyUI)
├── main.go         # Entry point
└── CLAUDE.md       # Detailed docs for Claude
```

## 🔧 Common Patterns

### Add New Route
```go
// In controller Setup()
http.Handle("GET /path/{id}", app.Serve("template.html", auth.Required))
```

### Database Operations
```go
// Create
model, err := Models.Insert(&Model{Field: "value"})

// Read
model, err := Models.Get(id)

// Update  
err := Models.Update(model)

// Search
results, err := Models.Search("WHERE field = ?", value)
```

### Template Access
```html
<!-- Access controller method -->
{{controller.MethodName}}

<!-- HTMX form -->
<form hx-post="/path" hx-target="body" hx-swap="outerHTML">
```

## ⚠️ Important Rules

1. **Always use `c.Redirect()`** not `http.Redirect()`
2. **Check permissions** with `models.CheckRepoAccess()`
3. **Build before commit**: `go build -o workspace`
4. **Models need `Table()`** method returning table name

## 🐛 Common Issues

| Error | Solution |
|-------|----------|
| `undefined: time` | Add `import "time"` |
| Template not found | Check file exists in `/views/` |
| Permission denied | Check `HasPermission()` logic |
| Route not working | Ensure registered in `Setup()` |

## 🔍 Debugging

```go
// Log values
log.Printf("Debug: %+v", variable)

// Template debug
{{printf "%+v" .}}

// Check auth
user, _, err := auth.Authenticate(r)
```

## 🚢 Deployment

```bash
# Build production binary
go build -o workspace

# Required environment
export AUTH_SECRET="production-secret"
export PORT=8080

# Run
./workspace
```

## 📚 More Info

See `CLAUDE.md` for detailed documentation and patterns.