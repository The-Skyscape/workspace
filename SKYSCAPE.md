# SKYSCAPE.md - AI Assistant Context

This file provides context to the AI assistant about the Skyscape Workspace project.

## Project Overview
Skyscape Workspace is a self-hosted development platform that combines:
- Git repository management (like GitHub)
- Containerized development environments (like Codespaces)
- CI/CD automation (Actions)
- AI-powered coding assistance

## Architecture
- **Backend**: Go with MVC pattern using the DevTools framework
- **Frontend**: HTMX + DaisyUI (server-side rendering, no React/Vue)
- **Database**: SQLite with FTS5 for code search
- **Containers**: Docker for sandboxed execution
- **AI**: Llama 3.2 via Ollama for local AI assistance

## Key Directories
- `controllers/` - HTTP request handlers
- `models/` - Database models and business logic
- `services/` - Container management services
- `views/` - HTML templates
- `internal/ai/tools/` - AI tool implementations

## Coding Conventions
- Use Go standard formatting (`go fmt`)
- Follow MVC pattern strictly
- All database fields use PascalCase (not snake_case)
- Use HTMX for dynamic updates, not client-side JS
- Always validate user input and check permissions
- Use sandboxes for any code execution

## Testing & Quality
- Run `go test ./...` for unit tests
- Use `go vet` for static analysis
- Check `golint` for style issues
- Test in sandbox before production

## Common Commands
- Build: `make clean && make`
- Run locally: `AUTH_SECRET="dev" go run .`
- Deploy: `./devtools/build/launch-app deploy`
- View logs: `docker logs sky-app --tail 50`

## AI Assistant Guidelines
When helping with this codebase:
1. Always use the sandbox for command execution
2. Follow existing patterns and conventions
3. Test changes before suggesting them
4. Use multi-file edits for complex changes
5. Check permissions before operations