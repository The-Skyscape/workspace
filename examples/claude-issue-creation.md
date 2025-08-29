# Claude AI Assistant - Issue Creation Example

This example demonstrates how the Claude AI assistant can analyze code and create issues in your repository.

## Features

### 1. Real-time JSON Streaming
- Claude runs as a persistent process with `--input-format=stream-json --output-format=stream-json`
- Bidirectional communication via named pipes
- Real-time updates shown in the UI

### 2. Tool Usage
Claude has access to these tools in the sandbox:
- `Bash(git:*)` - Git operations (commit, push, etc.)
- `Bash(cd:*)` - Navigate directories
- `Bash(ls:*)` - List files
- `Bash(cat:*)` - Read files
- `Edit` - Modify files
- `Read` - Read file contents
- `Write` - Create new files

### 3. Git Configuration
Each sandbox is configured with:
```bash
git config --global user.name "User Name"
git config --global user.email "user@example.com"
git config --global push.default current
```

## Example Prompts

### Analyze Code for Issues
```
Please analyze the code in /workspace/repos/[repo-name] and identify:
1. Security vulnerabilities
2. Performance bottlenecks
3. Code quality issues
4. Missing documentation

For each issue found, create a detailed GitHub issue with:
- Clear title
- Problem description
- Suggested fix
- Priority level
```

### Create Issues from Analysis
```
Based on your analysis, please create issues for the top 5 problems you found.
Use git to create issue files in .github/ISSUES/ directory with the format:
- Filename: issue-YYYY-MM-DD-HH-MM-SS.md
- Content: Issue title and description in markdown

Then commit and push these issues.
```

### Automated Code Review
```
Review the recent changes in this repository:
1. Check git log for recent commits
2. Analyze the changes for potential issues
3. Create issues for any problems found
4. Suggest improvements
```

## Implementation Details

### Worker Initialization
When a worker is created, it:
1. Clones selected repositories with access tokens
2. Configures git for push operations
3. Installs Claude CLI
4. Starts Claude in streaming JSON mode
5. Creates named pipes for communication

### Streaming Architecture
```
User Input → HTMX Form → Go Handler → Named Pipe → Claude CLI
                                                        ↓
Browser ← SSE Events ← Go Handler ← Named Pipe ← Claude Response
```

### Message Flow
1. User types message in chat interface
2. JavaScript sends message and opens SSE connection
3. Go handler creates StreamHandler
4. StreamHandler sends JSON to Claude via named pipe
5. Claude processes and responds with JSON messages
6. StreamHandler reads responses and forwards to SSE
7. JavaScript updates UI in real-time

## Security Considerations

- Claude runs with `--dangerously-skip-permissions` only in sandboxed environment
- Each worker has isolated Docker container
- Git operations use access tokens with limited scope
- All operations are logged and auditable

## Future Enhancements

1. **WebSocket Support**: Replace SSE with WebSocket for true bidirectional communication
2. **Issue Templates**: Pre-defined templates for common issue types
3. **Batch Operations**: Analyze multiple repos and create issues in bulk
4. **Integration with Issue Tracker**: Direct API integration instead of git-based issues
5. **Custom Tools**: Add project-specific tools for Claude to use