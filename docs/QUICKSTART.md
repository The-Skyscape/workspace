# Skyscape Quickstart Guide

Welcome to Skyscape! This guide will help you get up and running in just 5 minutes.

## 🚀 Choose Your Path

### Option 1: Skyscape Hosted (Recommended for Quick Start)

Get started immediately with our managed hosting:

1. **Sign Up** at [theskyscape.com/signup](https://www.theskyscape.com/signup)
2. **Create Your First Repository**
3. **Launch a VS Code Workspace**
4. **Start Building!**

That's it! No server setup, no configuration needed.

### Option 2: Self-Hosted (Full Control)

Deploy Skyscape on your own infrastructure:

```bash
# 1. Clone the repository
git clone https://github.com/The-Skyscape/workspace.git
cd workspace

# 2. Set required environment variable
export AUTH_SECRET="your-secret-key-here"

# 3. Build the application (ALWAYS use Makefile)
make clean && make

# 4. Run Skyscape
./build/workspace
```

Visit `http://localhost:5000` and you're ready to go!

## 📦 Your First Repository

### Creating a Repository

1. Click **"New Repository"** on your dashboard
2. Choose a name (e.g., "my-awesome-project")
3. Select visibility (public or private)
4. Click **"Create Repository"**

### Cloning Your Repository

```bash
# HTTPS (recommended for beginners)
git clone https://your-skyscape-domain/repos/USERNAME/REPO_NAME.git

# SSH (after adding your SSH key)
git clone git@your-skyscape-domain:USERNAME/REPO_NAME.git
```

### Pushing Your First Commit

```bash
cd REPO_NAME
echo "# My Awesome Project" > README.md
git add README.md
git commit -m "Initial commit"
git push origin main
```

## 💻 Cloud Workspaces

### Launching VS Code in Your Browser

1. Navigate to your repository
2. Click **"Open in VS Code"**
3. Wait ~30 seconds for your workspace to initialize
4. Start coding with full VS Code features!

### Workspace Features

- ✅ Full terminal access
- ✅ Git integration
- ✅ Extension support
- ✅ Docker access
- ✅ Persistent storage
- ✅ Multiple workspaces

## 🔄 Migrating from GitHub

### One-Click Import

1. Go to **Settings → Integrations**
2. Click **"Connect GitHub"**
3. Authorize Skyscape
4. Select repositories to import
5. Click **"Import Selected"**

Your entire repository history, branches, and issues will be imported!

### Manual Migration

```bash
# 1. Clone from GitHub
git clone --mirror https://github.com/USERNAME/REPO.git

# 2. Push to Skyscape
cd REPO.git
git remote set-url origin https://your-skyscape/repos/USERNAME/REPO.git
git push --mirror
```

## 🤖 CI/CD Actions

### Creating Your First Action

1. Navigate to your repository
2. Go to **Actions** tab
3. Click **"New Action"**
4. Configure:
   ```yaml
   Name: Build and Test
   Command: npm install && npm test
   Trigger: Manual
   Branch: main
   ```
5. Click **"Create Action"**

### Running Actions

- **Manual**: Click "Run" button
- **Scheduled**: Set cron expression (coming soon)
- **On Push**: Automatic trigger (coming soon)

### Example Actions

**Node.js Project**:
```bash
npm install && npm test && npm run build
```

**Python Project**:
```bash
pip install -r requirements.txt && pytest
```

**Go Project**:
```bash
go test ./... && go build
```

## 🔐 Access Control

### Adding Collaborators

1. Go to **Repository Settings**
2. Click **"Collaborators"**
3. Enter username or email
4. Select permission level:
   - **Read**: View only
   - **Write**: Push access
   - **Admin**: Full control

### SSH Keys

Add your SSH key for secure Git access:

1. Go to **User Settings**
2. Click **"SSH Keys"**
3. Paste your public key
4. Save

Generate a new SSH key:
```bash
ssh-keygen -t ed25519 -C "your-email@example.com"
cat ~/.ssh/id_ed25519.pub
```

## 📊 Issue Tracking

### Creating Issues

1. Navigate to repository
2. Click **"Issues"** tab
3. Click **"New Issue"**
4. Fill in:
   - Title
   - Description (Markdown supported)
   - Labels
   - Assignee

### Issue Workflow

1. **Open**: New issue
2. **In Progress**: Being worked on
3. **Review**: Ready for review
4. **Closed**: Completed

## 🎯 Best Practices

### Repository Organization

```
my-project/
├── README.md           # Project overview
├── .skyscape/         
│   └── actions.yml     # CI/CD configuration
├── docs/               # Documentation
├── src/                # Source code
└── tests/              # Test files
```

### Workspace Tips

- **Save Often**: Workspaces persist, but save important work
- **Use Extensions**: Install VS Code extensions as needed
- **Terminal Multiplexer**: Use `tmux` for multiple terminals
- **Resource Monitoring**: Check workspace resources in settings

### Security Best Practices

1. **Never commit secrets** - Use environment variables
2. **Use private repos** for sensitive code
3. **Regular backups** - Export repositories periodically
4. **Strong passwords** - Use unique, complex passwords
5. **2FA** - Enable when available (coming soon)

## 🆘 Troubleshooting

### Common Issues

**Can't push to repository**
- Check your permissions
- Verify your credentials
- Ensure you're on the right branch

**Workspace won't start**
- Check Docker service status
- Verify enough resources available
- Try refreshing the page

**Action failing**
- Check action logs for errors
- Verify command works locally
- Check artifact paths exist

**Can't clone repository**
- Verify repository exists
- Check authentication
- Try HTTPS if SSH fails

### Getting Help

- 📧 Email: support@theskyscape.com
- 💬 Discord: [Join our community](https://discord.gg/skyscape)
- 📖 Docs: [docs.theskyscape.com](https://docs.theskyscape.com)
- 🐛 Issues: [GitHub Issues](https://github.com/The-Skyscape/workspace/issues)

## 🎉 What's Next?

Now that you're up and running:

1. **Import your GitHub repos** - Bring your existing projects
2. **Set up CI/CD** - Automate your workflows
3. **Invite your team** - Collaborate on projects
4. **Explore integrations** - Connect external services
5. **Customize your workspace** - Make it yours

## 💡 Pro Tips

- **Keyboard Shortcuts**: Press `?` in any view for shortcuts
- **Quick Search**: Use `/` to search repositories
- **Command Palette**: `Ctrl+Shift+P` in VS Code
- **Multiple Workspaces**: Open different repos in tabs
- **Git Aliases**: Set up shortcuts for common commands

Welcome to Skyscape - now go build something amazing! 🚀