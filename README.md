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

### 🤖 **AI Integration** (Pro Tier)
- **Intelligent Automation**: AI manages your code 24/7 with proactive features
- **Chat Assistant**: Repository-aware conversational AI with 21+ tools
- **Automatic Issue Triage**: Smart labeling, prioritization, and analysis
- **PR Review Automation**: Code analysis, suggestions, and auto-approval
- **Event-Driven Actions**: Responds automatically to repository events
- **Local Execution**: Llama 3.2:3b runs on your infrastructure for privacy
- **No API Keys**: No external dependencies or rate limits
- **Activity Tracking**: Complete audit trail of all AI actions
- **Configurable**: Control automation aggressiveness and response delays
- **Standard Tier**: Works great with external AI tools (Claude CLI, GitHub Copilot)

### 🔗 **Integrations**
- **GitHub Sync**: Bidirectional synchronization with GitHub repositories
- **OAuth Support**: Login with GitHub, GitLab, or custom OAuth providers
- **Webhook Support**: Trigger actions from external services
- **HTMX Integration**: Dynamic UI updates without full page reloads
- **HATEOAS Design**: Hypermedia-driven application state

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
- Go 1.24.5 or later
- Docker 24+ (for sandbox and coder services)
- Git 2.40+
- Make (for build automation)

## 💡 Workspace Tiers

### Standard Workspace ($80/month or self-hosted)
- 2 vCPUs, 4GB RAM, 160GB storage
- Full Git, IDE, CI/CD features
- Perfect for use with external AI tools (Claude CLI, GitHub Copilot)
- Most cost-effective AI-assisted development workflow

### Pro Workspace ($800/month)
- 4 vCPUs, 16GB RAM, 320GB storage  
- RTX 4000 Ada GPU with 20GB VRAM
- Full AI automation suite (AI_ENABLED=true)
- Proactive issue triage and PR reviews
- Daily repository health reports
- Smart handling of stale issues
- No API keys or rate limits needed
- AI manages your code 24/7

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

Note: The application will create its data directory at `~/.skyscape/` on first run.

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
  -v ~/.skyscape:/root/.skyscape \
  -e AUTH_SECRET="your-secret-key" \
  skyscape-workspace
```

### Production Deployment

For production deployments, use the launch-app tool from DevTools:

```bash
# Build the workspace application
cd workspace && make clean && make

# Build and use the launch-app tool
cd ../devtools && make -B 
export DIGITAL_OCEAN_API_KEY="your-token"
./build/launch-app deploy \
  --name skyscape-prod \
  --domain git.yourdomain.com \
  --binary ../workspace/build/workspace
```

This will:
- Create a DigitalOcean droplet
- Install Docker and configure the server
- Deploy your application as a container
- Set up SSL with Let's Encrypt

## 🔧 Configuration

### Environment Variables
- `AUTH_SECRET` (required): JWT signing secret for authentication
- `PORT`: Application port (default: 5000)
- `PREFIX`: URL prefix for the application (default: empty)
- `THEME`: DaisyUI theme (default: corporate)
- `DATA_DIR`: Custom data directory (default: ~/.skyscape)
- `AI_ENABLED`: Enable OpenAI GPT features ("true" for Pro tier, "false" for Standard)
  - Automatically set during deployment based on infrastructure
  - Controls whether AI services start and UI features are shown

### Data Storage
All application data is stored in `~/.skyscape/` by default:
- **Database**: `~/.skyscape/workspace.db` (SQLite)
- **Repositories**: `~/.skyscape/repos/`
- **Artifacts**: Stored as BLOBs in the database

### SSL Configuration (for launch-app deployments)
- `SKYSCAPE_SSL_FULLCHAIN`: Path to SSL certificate
- `SKYSCAPE_SSL_PRIVKEY`: Path to SSL private key

### Cloud Provider Settings (for DevTools launch-app)
- `DIGITAL_OCEAN_API_KEY`: For DigitalOcean deployments
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`: For AWS deployments (planned)
- `GCP_PROJECT_ID`, `GCP_SERVICE_ACCOUNT_KEY`: For GCP deployments (planned)

## 📚 Application Routes (HATEOAS)

Skyscape follows the HATEOAS (Hypermedia as the Engine of Application State) model using HTMX for dynamic interactions. All routes return HTML responses that drive the application state.

### Authentication
All routes require authentication via JWT token in httpOnly cookie.

```
GET  /signin                 # Sign in page
POST /auth/signin            # Process sign in (returns HTML with redirect)
GET  /signup                 # Sign up page  
POST /auth/signup            # Process sign up (returns HTML with redirect)
POST /auth/signout           # Sign out (returns HTML with redirect)
```

### Repository Management
```
GET  /repos                  # List all repositories
GET  /repos/{id}             # View repository
POST /repos/create           # Create new repository (HTMX form submission)
GET  /repos/{id}/files       # Browse repository files
GET  /repos/{id}/commits     # View commit history
GET  /repos/{id}/settings    # Repository settings
POST /repos/{id}/delete      # Delete repository (HTMX action)
```

### CI/CD Actions
```
GET  /repos/{id}/actions                    # List repository actions
POST /repos/{id}/actions/create             # Create new action (HTMX form)
GET  /repos/{id}/actions/{actionId}         # View action details
POST /repos/{id}/actions/{actionId}/run     # Run action (HTMX trigger)
GET  /repos/{id}/actions/{actionId}/logs    # View execution logs
GET  /repos/{id}/actions/{actionId}/history # View run history
GET  /repos/{id}/actions/{actionId}/artifacts # Download artifacts
```

### Issues & Pull Requests
```
GET  /repos/{id}/issues      # List issues
POST /repos/{id}/issues/create # Create issue (HTMX form)
GET  /repos/{id}/issues/{issueId} # View issue
GET  /repos/{id}/prs         # List pull requests
GET  /repos/{id}/prs/{prId}  # View pull request
```

### AI Features (Pro Tier)
```
GET  /ai/chat                # AI chat interface
POST /ai/chat/send           # Send message to AI
GET  /ai/config              # AI configuration panel
POST /ai/config/update       # Update AI settings
GET  /ai/activity            # Recent AI activity
GET  /ai/queue/stats         # Queue statistics
```

### HTMX Partials
These routes return HTML fragments for dynamic updates:
```
GET  /repos/{id}/actions/{actionId}/logs-partial     # Live log streaming
GET  /repos/{id}/actions/{actionId}/artifacts-partial # Artifact list updates
GET  /monitoring/stats                                # Live monitoring stats
POST /repos/{id}/issues/{issueId}/comments           # Add comment (returns HTML)
GET  /ai/activity                                     # AI activity updates
```

## 🧪 Testing

### Unit Tests
```bash
go test ./...
```

### Build Verification
```bash
make clean && make
```

### Manual Testing
```bash
# Start the application
export AUTH_SECRET="test-secret"
./build/workspace

# Test in browser
open http://localhost:5000
```

## 🤝 Contributing

We welcome contributions!

### Development Setup
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes following the patterns in the codebase
4. Test your changes: `make clean && make && go test ./...`
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Style
- Follow Go standard formatting (`go fmt`)
- Use meaningful variable and function names
- Follow existing MVC patterns (see CLAUDE.md for details)
- Add comments for exported functions
- Write tests for new features
- Use HTMX for dynamic UI updates (not REST APIs)

### Key Patterns to Follow
- Controllers use factory functions: `func Name() (string, *Controller)`
- Models implement `Table() string` method
- Templates use unique names (stored in partials/ for sub-views)
- Use `c.Redirect()` not `http.Redirect()` for HTMX compatibility
- All data stored in `~/.skyscape/` directory


## 🙏 Acknowledgments

- Built with [TheSkyscape DevTools](https://github.com/The-Skyscape/devtools)
- UI powered by [DaisyUI](https://daisyui.com) and [HTMX](https://htmx.org)
- Icons from [Heroicons](https://heroicons.com)
- Code highlighting by [Prism.js](https://prismjs.com)

## 📞 Support

- **Documentation**: See CLAUDE.md for development guidance
- **Issues**: [GitHub Issues](https://github.com/The-Skyscape/workspace/issues)
- **Quick Reference**: See QUICK_REFERENCE.md for common tasks

## 🚀 Roadmap

### Completed Features ✓
- [x] Git repository hosting with FTS5 search
- [x] CI/CD Actions with Docker sandboxes
- [x] Issue and PR tracking
- [x] GitHub bidirectional sync
- [x] System monitoring dashboard
- [x] Artifact collection and versioning
- [x] Modular controller architecture

### In Progress
- [ ] Scheduled action triggers
- [ ] Webhook-based action triggers
- [ ] Enhanced artifact management UI

### Planned
- [ ] Real-time collaboration features
- [ ] Advanced CI/CD pipeline editor
- [ ] Kubernetes deployment support
- [x] AI-powered development assistant (OpenAI GPT integration in Pro tier)
- [ ] Multi-region deployment support
- [ ] Enterprise SSO integration
- [ ] GitOps workflow support
- [ ] Marketplace for actions and templates

---

**Built with ❤️ by The Skyscape Team**