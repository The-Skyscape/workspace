# Skyscape Workspace

A GitHub-like developer workspace platform built with TheSkyscape DevTools, featuring AI-powered development tools, containerized workspaces, and comprehensive project management.

## ✨ Features

### 🏠 **Public Developer Portfolio**
- Professional homepage with developer profile and bio
- Public repository showcase with visibility controls
- Community issue submission for open source projects
- Mobile-responsive design across all devices

### 🔐 **Authenticated Developer Environment**
- Private repository management with Git integration
- One-click containerized VS Code workspaces
- Complete issue and pull request lifecycle management
- Advanced code search with context highlighting
- README rendering with markdown support
- Permission-based access control system

### 🎯 **Project Management**
- **Issues**: Full CRUD operations with status management
- **Pull Requests**: Create, merge, close with branch selection
- **Actions**: AI-powered automation and workflow management
- **File Browser**: Syntax highlighting and file type detection
- **Search**: Regex-based code search across repository files

### 🚀 **Development Tools**
- **Workspaces**: Docker-based VS Code environments
- **Git Integration**: Commit history and branch management
- **File Management**: Browse, view, and edit repository files
- **Mobile Support**: Responsive design for development on any device

## 🛠 Technology Stack

- **Backend**: Go with TheSkyscape DevTools MVC framework
- **Frontend**: HTMX + DaisyUI + TailwindCSS
- **Database**: SQLite3 with automatic migrations
- **Authentication**: JWT-based with bcrypt password hashing
- **Containerization**: Docker for isolated development environments
- **UI Framework**: DaisyUI components with mobile-first design

## 📱 Architecture

### MVC Pattern
- **Models**: Database entities with Table() methods and global repositories
- **Views**: HTML templates with HTMX integration and DaisyUI styling
- **Controllers**: HTTP handlers with factory functions and Setup/Handle methods
- **Internal Packages**: Self-contained modules for Git and workspace management

### Key Components
- **Repository Management**: Self-contained Git integration with file browsing and version control
- **Workspace Orchestration**: Docker container lifecycle management
- **Permission System**: Role-based access control (read/write/admin)
- **Activity Logging**: Comprehensive audit trail for all user actions
- **Search Engine**: File content indexing with context-aware results
- **Internal Coding Package**: Dedicated Git repository and workspace management (moved from devtools)

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Docker
- Git

### Installation

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd workspace
   ```

2. **Set environment variables**
   ```bash
   export AUTH_SECRET="your-super-secret-jwt-key"
   export PORT=8080  # Optional, defaults to 5000
   ```

3. **Run the application**
   ```bash
   go run .
   ```

4. **Visit the application**
   ```
   http://localhost:8080
   ```

### Development Commands

```bash
# Run with hot reload
go run .

# Build for production
go build -o workspace

# Run tests
go test ./...

# Update dependencies
go mod tidy
```

## 📋 Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `AUTH_SECRET` | JWT signing secret | - | ✅ |
| `PORT` | Application server port | `5000` | ❌ |
| `THEME` | DaisyUI theme | `corporate` | ❌ |
| `CONGO_SSL_FULLCHAIN` | SSL certificate path | `/root/fullchain.pem` | ❌ |
| `CONGO_SSL_PRIVKEY` | SSL private key path | `/root/privkey.pem` | ❌ |

## 🎨 User Interface

### Desktop Experience
- Clean, GitHub-like interface with comprehensive navigation
- Advanced file browser with syntax highlighting
- Responsive grid layouts and professional typography
- Modal-based workflows for creating issues and pull requests

### Mobile Experience
- Collapsible hamburger navigation menu
- Touch-optimized buttons and form controls
- Responsive tables with hidden columns on small screens
- Mobile-first design patterns throughout

## 🔒 Security Features

- **Authentication**: Secure JWT-based sessions with httpOnly cookies
- **Authorization**: Role-based permissions (read/write/admin) for repositories
- **Password Security**: bcrypt hashing with secure defaults
- **Path Traversal Protection**: Validated file access within repository boundaries
- **Input Validation**: Comprehensive server-side validation for all forms

## 🏗 Development Patterns

### Controller Factory Pattern
```go
func Repos() (string, *ReposController) {
    return "repos", &ReposController{}
}
```

### Template Integration
- Controllers accessible as `{{controllerName.Method}}` in templates
- Built-in helpers: `{{theme}}`, `{{host}}`, `{{auth.CurrentUser}}`
- HTMX integration with `c.Refresh(w, r)` for dynamic updates

### Permission Checking
```go
err := models.CheckRepoAccess(user, repoID, models.RoleWrite)
if err != nil {
    return errors.New("insufficient permissions")
}
```

## 🔧 Deployment

### Using TheSkyscape launch-app
```bash
go build -o workspace
export DIGITAL_OCEAN_API_KEY="your-token"
# Note: launch-app tool is from the parent devtools repository
../launch-app --name workspace --domain workspace.example.com --binary ./workspace
```

### Docker Deployment
```bash
docker build -t skyscape-workspace .
docker run -p 8080:8080 -e AUTH_SECRET="your-secret" skyscape-workspace
```

## 📖 API Reference

The application uses HTMX for dynamic updates rather than traditional REST APIs. All interactions are handled through server-rendered templates with HTMX attributes:

- `hx-get` - Dynamic content loading
- `hx-post` - Form submissions
- `hx-swap` - Content replacement strategies
- `hx-target` - Element targeting for updates

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is built using TheSkyscape DevTools and follows its licensing terms.

## 🙏 Acknowledgments

- Built with [TheSkyscape DevTools](https://github.com/The-Skyscape/devtools)
- UI components by [DaisyUI](https://daisyui.com/)
- Dynamic interactions powered by [HTMX](https://htmx.org/)
- Icons by [Heroicons](https://heroicons.com/)

---

**Skyscape Workspace** - Where cloud development meets developer productivity ☁️✨