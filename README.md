# Skyscape Workspace

A comprehensive developer platform combining Git repository hosting, containerized development environments, and automated CI/CD workflows. Built with Go and modern web technologies, Skyscape provides a self-hosted alternative to GitHub + Codespaces.

## 🚀 Overview

Skyscape Workspace is a full-featured development platform that empowers teams to:
- Host and manage Git repositories with fine-grained access control
- Launch instant, containerized VS Code development environments
- Automate workflows with Docker-based CI/CD actions
- Track issues, pull requests, and project progress
- Search and analyze code with SQLite FTS5 full-text search

## ✨ Core Features

### 📦 **Repository Management**
- **Git Hosting**: Full Git server implementation with SSH and HTTPS support
- **Access Control**: Role-based permissions (read/write/admin)
- **Visibility**: Public and private repository support
- **File Browser**: Web-based file explorer with syntax highlighting
- **Code Search**: Fast, regex-based search with SQLite FTS5
- **Commit History**: Visual commit log with diff viewing

### 🖥️ **Development Environments (Coder Service)**
- **VS Code in Browser**: Full-featured code-server IDE
- **Per-Repository Workspaces**: Isolated development environments
- **Persistent Storage**: Your work is saved between sessions
- **Docker Integration**: Full Docker access within environments
- **Global Coder Service**: Centralized VS Code service management
- **Authenticated Access**: Secure workspace access via /coder/{workspace-id}/

### 🤖 **CI/CD Actions System**
- **Docker Sandboxes**: Isolated execution environments for each action
- **Action Types**: Manual, scheduled, and event-triggered workflows
- **Execution History**: Complete audit trail of all action runs
- **Artifact Collection**: Automatic collection and versioning of build artifacts
- **Real-time Logs**: Live streaming of action execution output
- **Statistics**: Success rates, duration tracking, and performance metrics

### 📋 **Project Management**
- **Issues**: Full issue tracking with status management
- **Pull Requests**: Branch comparison, merging, and review workflows
- **Comments**: Threaded discussions on issues and PRs
- **Activity Feed**: Real-time updates on repository activity
- **Notifications**: Email and in-app notifications (coming soon)

### 🔗 **Integrations**
- **GitHub Sync**: Bidirectional synchronization with GitHub repositories
- **OAuth Support**: Login with GitHub, GitLab, or custom OAuth providers
- **Webhook Support**: Trigger actions from external services
- **API Access**: RESTful API for automation and integrations

### 📊 **System Monitoring**
- **Real-time Metrics**: CPU, memory, and disk usage tracking
- **Container Management**: Docker container status and control
- **Alert System**: Resource threshold notifications
- **Admin Dashboard**: Comprehensive system overview

## 🏗️ Architecture

### Technology Stack
- **Backend**: Go 1.21+ with TheSkyscape DevTools MVC framework
- **Frontend**: HTMX + Alpine.js + DaisyUI v5
- **Database**: SQLite3 with FTS5 for full-text search
- **Container Runtime**: Docker 24+
- **Authentication**: JWT with httpOnly cookies
- **File Storage**: Local filesystem with optional S3 support

### Project Structure
```
workspace/
├── controllers/         # MVC Controllers (split by domain)
│   ├── repos.go        # Repository management
│   ├── actions.go      # CI/CD actions
│   ├── issues.go       # Issue tracking
│   ├── pullrequests.go # Pull request management
│   ├── workspaces.go   # Development environments
│   ├── integrations.go # External integrations
│   ├── monitoring.go   # System monitoring
│   ├── settings.go     # Settings management
│   ├── home.go         # Dashboard
│   └── public.go       # Public access
├── models/             # Database models and repositories
│   ├── repository.go   # Git repository model
│   ├── action.go       # CI/CD action model
│   ├── action_run.go   # Action execution history
│   ├── action_artifact.go # Build artifacts
│   ├── issue.go        # Issue tracking
│   ├── pullrequest.go  # Pull requests
│   ├── comment.go      # Comments
│   ├── coder.go        # Coder service handler
│   ├── coding.go       # Git operations
│   ├── file_search.go  # FTS5 search
│   └── permission.go   # Access control
├── services/           # Business logic and external services
│   ├── sandbox.go     # Docker sandbox management
│   └── coder.go       # VS Code service management
├── views/             # HTML templates with HTMX
│   ├── partials/      # Reusable components
│   ├── repo-*.html    # Repository views
│   ├── monitoring*.html # Monitoring views
│   └── *.html         # Other page templates
└── internal/          # Internal packages
    ├── coding/        # Git server implementation
    └── search/        # FTS5 search implementation
```

### Database Schema
- **repositories**: Git repository metadata with FTS5 search
- **actions**: CI/CD workflow definitions
- **action_runs**: Execution history with metrics
- **action_artifacts**: Build artifacts with versioning
- **issues**: Issue tracking with status management
- **pull_requests**: PR management and merging
- **comments**: Threaded discussions on issues/PRs
- **activities**: Repository activity feed
- **users**: User accounts and authentication
- **access_tokens**: API token management
- **permissions**: Role-based access control
- **settings**: Repository and user preferences
- **file_search**: FTS5 full-text search index

## 🚦 Getting Started

### Prerequisites
- Go 1.21 or later
- Docker 24+ with compose support
- Git 2.40+
- Make (for build automation)

### Installation

1. **Clone the repository**
```bash
git clone https://github.com/The-Skyscape/workspace.git
cd workspace
```

2. **Set environment variables**
```bash
export AUTH_SECRET="your-secret-key-here"  # Required for JWT signing
export PORT=5000                           # Optional, defaults to 5000
```

3. **Build the application**
```bash
make clean && make
```

4. **Run locally**
```bash
./build/workspace
```

5. **Access the application**
```
http://localhost:5000
```

### Docker Deployment

```bash
docker build -t skyscape-workspace .
docker run -d \
  -p 5000:5000 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v skyscape-data:/data \
  -e AUTH_SECRET="your-secret-key" \
  skyscape-workspace
```

### Production Deployment

For production deployments, use the launch-app tool from DevTools:

```bash
cd ../devtools && make build
./build/launch-app deploy \
  --name skyscape-prod \
  --domain git.yourdomain.com \
  --binary ../workspace/build/workspace
```

## 🔧 Configuration

### Environment Variables
- `AUTH_SECRET` (required): JWT signing secret
- `PORT`: Application port (default: 5000)
- `THEME`: DaisyUI theme (default: corporate)
- `DATABASE_PATH`: SQLite database location (default: ./workspace.db)
- `REPOS_PATH`: Git repository storage (default: ./repos)
- `WORKSPACE_TIMEOUT`: Idle workspace timeout in minutes (default: 30)

### SSL Configuration
- `CONGO_SSL_FULLCHAIN`: Path to SSL certificate
- `CONGO_SSL_PRIVKEY`: Path to SSL private key

### Cloud Provider Settings
- `DIGITAL_OCEAN_API_KEY`: For DigitalOcean deployments
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`: For AWS deployments
- `GCP_PROJECT_ID`, `GCP_SERVICE_ACCOUNT_KEY`: For GCP deployments

## 📚 API Documentation

### Authentication
All API endpoints require authentication via JWT token in httpOnly cookie.

```bash
# Login
POST /auth/login
{
  "email": "user@example.com",
  "password": "password"
}

# Logout
POST /auth/logout
```

### Repositories
```bash
# List repositories
GET /api/repos

# Create repository
POST /api/repos
{
  "name": "my-repo",
  "description": "Repository description",
  "visibility": "private"
}

# Get repository
GET /api/repos/{id}

# Delete repository
DELETE /api/repos/{id}
```

### Actions
```bash
# List actions for repository
GET /api/repos/{id}/actions

# Create action
POST /api/repos/{id}/actions
{
  "title": "Build and Test",
  "type": "manual",
  "command": "npm test && npm build",
  "branch": "main",
  "artifact_paths": "dist/, coverage/"
}

# Run action
POST /api/repos/{id}/actions/{actionId}/run

# Get action history
GET /api/repos/{id}/actions/{actionId}/runs
```

## 🧪 Testing

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
go test -tags=integration ./...
```

### Load Testing
```bash
# Using Apache Bench
ab -n 1000 -c 10 http://localhost:5000/

# Using hey
hey -n 1000 -c 10 http://localhost:5000/
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style
- Follow Go standard formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for exported functions
- Write tests for new features

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with [TheSkyscape DevTools](https://github.com/The-Skyscape/devtools)
- UI powered by [DaisyUI](https://daisyui.com) and [HTMX](https://htmx.org)
- Icons from [Heroicons](https://heroicons.com)
- Code highlighting by [Prism.js](https://prismjs.com)

## 📞 Support

- **Documentation**: [docs.skyscape.dev](https://docs.skyscape.dev)
- **Issues**: [GitHub Issues](https://github.com/The-Skyscape/workspace/issues)
- **Email**: support@skyscape.dev
- **Discord**: [Join our community](https://discord.gg/skyscape)

## 🚀 Roadmap

### Q1 2025
- [ ] Real-time collaboration features
- [ ] Advanced CI/CD pipeline editor
- [ ] Kubernetes deployment support
- [ ] Mobile app for iOS/Android

### Q2 2025
- [ ] AI-powered code review
- [ ] Integrated monitoring and logging
- [ ] Multi-region deployment support
- [ ] Enterprise SSO integration

### Future
- [ ] GitOps workflow support
- [ ] Infrastructure as Code templates
- [ ] Marketplace for actions and templates
- [ ] Advanced analytics and insights

---

**Built with ❤️ by The Skyscape Team**