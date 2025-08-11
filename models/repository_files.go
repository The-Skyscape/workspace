package models

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// FileNode represents a file or directory in the repository
type FileNode struct {
	Name     string
	Path     string
	Type     string // "file" or "dir"
	Size     int64
	Mode     string
	Hash     string
	ModTime  time.Time // Last modification time from git history
	Content  string // Only for files when requested
}

// File represents a single file with content
type File struct {
	Path     string
	Name     string
	Content  string
	Size     int64
	IsBinary bool
	Language string
}

// GetFileTree returns the file tree for a given path
func (r *Repository) GetFileTree(branch, path string) ([]*FileNode, error) {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Check if branch exists
	if !r.BranchExists(branch) {
		return []*FileNode{}, nil
	}
	
	// Prepare path
	if path == "" || path == "/" {
		path = "."
	}
	
	// List files using git ls-tree
	args := []string{"ls-tree", "--long", branch}
	if path != "." {
		args = append(args, "--", path+"/")
	}
	
	stdout, stderr, err := r.Git(args...)
	if err != nil {
		if strings.Contains(stderr.String(), "not a valid object") {
			return []*FileNode{}, nil
		}
		return nil, errors.Wrap(err, stderr.String())
	}
	
	var nodes []*FileNode
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		// Parse ls-tree output: mode type hash size name
		parts := strings.Fields(line)
		if len(parts) >= 5 {
			mode := parts[0]
			nodeType := parts[1]
			hash := parts[2]
			size := int64(0)
			name := strings.Join(parts[4:], " ")
			
			// Parse size for files
			if nodeType == "blob" {
				if sz, err := strconv.Atoi(parts[3]); err == nil {
					size = int64(sz)
				}
			}
			
			// Store the full path for navigation
			fullPath := name
			
			// Clean up the name for display
			displayName := name
			if path != "." && strings.HasPrefix(name, path+"/") {
				displayName = strings.TrimPrefix(name, path+"/")
			}
			
			node := &FileNode{
				Name: filepath.Base(displayName),
				Path: fullPath,  // Keep full path for navigation
				Mode: mode,
				Hash: hash,
				Size: size,
			}
			
			if nodeType == "tree" {
				node.Type = "dir"
			} else {
				node.Type = "file"
			}
			
			nodes = append(nodes, node)
		}
	}
	
	// Get modification times for each node
	for i, node := range nodes {
		if node.Type == "file" {
			modTime, err := r.GetFileModTime(branch, node.Path)
			if err == nil && !modTime.IsZero() {
				nodes[i].ModTime = modTime
			} else {
				// Fallback to current time if we can't get git history
				nodes[i].ModTime = time.Now()
			}
		} else {
			// For directories, use current time
			nodes[i].ModTime = time.Now()
		}
	}
	
	// Sort directories first, then by name
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type == "dir" && nodes[j].Type != "dir" {
			return true
		}
		if nodes[i].Type != "dir" && nodes[j].Type == "dir" {
			return false
		}
		return nodes[i].Name < nodes[j].Name
	})
	
	return nodes, nil
}

// GetFile retrieves a file's content
func (r *Repository) GetFile(branch, path string) (*File, error) {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Check if file exists
	if !r.FileExists(branch, path) {
		return nil, errors.New("file not found")
	}
	
	// Get file content
	stdout, stderr, err := r.Git("show", branch+":"+path)
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}
	
	content := stdout.String()
	
	// Check if binary
	isBinary := false
	if strings.Contains(content, "\x00") {
		isBinary = true
	}
	
	// Get file info
	infoOut, _, err := r.Git("ls-tree", "--long", branch, path)
	size := int64(len(content))
	if err == nil {
		parts := strings.Fields(infoOut.String())
		if len(parts) >= 4 {
			if sz, err := strconv.Atoi(parts[3]); err == nil {
				size = int64(sz)
			}
		}
	}
	
	file := &File{
		Path:     path,
		Name:     filepath.Base(path),
		Content:  content,
		Size:     size,
		IsBinary: isBinary,
		Language: getLanguageFromExtension(filepath.Ext(path)),
	}
	
	return file, nil
}

// FileExists checks if a file exists in a branch
func (r *Repository) FileExists(branch, path string) bool {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	if branch == "" {
		return false // No branches in repository
	}
	
	// Check if object exists and is a blob (file)
	stdout, _, err := r.Git("cat-file", "-t", branch+":"+path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(stdout.String()) == "blob"
}

// IsDirectory checks if a path is a directory in a branch
func (r *Repository) IsDirectory(branch, path string) bool {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	if branch == "" {
		return false // No branches in repository
	}
	
	// Check if object exists and is a tree (directory)
	stdout, _, err := r.Git("cat-file", "-t", branch+":"+path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(stdout.String()) == "tree"
}

// GetREADME finds and returns the README file
func (r *Repository) GetREADME(branch string) (*File, error) {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Common README filenames
	readmeFiles := []string{
		"README.md",
		"readme.md",
		"README.MD",
		"README",
		"readme",
		"README.txt",
		"readme.txt",
		"README.rst",
		"readme.rst",
	}
	
	for _, filename := range readmeFiles {
		if r.FileExists(branch, filename) {
			return r.GetFile(branch, filename)
		}
	}
	
	return nil, nil
}

// WriteFile creates or updates a file in the bare repository
func (r *Repository) WriteFile(branch, path, content, message, authorName, authorEmail string) error {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Set default author if not provided
	if authorName == "" {
		authorName = "Skyscape User"
	}
	if authorEmail == "" {
		authorEmail = "user@skyscape.local"
	}
	
	// Get the current commit hash for the branch
	stdout, _, err := r.Git("rev-parse", branch)
	if err != nil {
		// Branch doesn't exist, create initial commit
		return r.createInitialCommit(branch, path, content, message, authorName, authorEmail)
	}
	currentCommit := strings.TrimSpace(stdout.String())
	
	// Write content to object database
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = r.Path()
	cmd.Stdin = strings.NewReader(content)
	hashOut, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to write object")
	}
	blobHash := strings.TrimSpace(string(hashOut))
	
	// Get the current tree
	treeOut, _, err := r.Git("cat-file", "-p", currentCommit)
	if err != nil {
		return errors.Wrap(err, "failed to read commit")
	}
	
	// Extract tree hash from commit
	lines := strings.Split(treeOut.String(), "\n")
	var treeHash string
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			treeHash = strings.TrimPrefix(line, "tree ")
			break
		}
	}
	
	// Read tree into index
	_, _, err = r.Git("read-tree", treeHash)
	if err != nil {
		return errors.Wrap(err, "failed to read tree")
	}
	
	// Update index with new file
	_, _, err = r.Git("update-index", "--add", "--cacheinfo", "100644", blobHash, path)
	if err != nil {
		return errors.Wrap(err, "failed to update index")
	}
	
	// Write tree from index
	treeOut2, _, err := r.Git("write-tree")
	if err != nil {
		return errors.Wrap(err, "failed to write tree")
	}
	newTreeHash := strings.TrimSpace(treeOut2.String())
	
	// Create commit
	commitCmd := exec.Command("git", "commit-tree", newTreeHash, "-p", currentCommit, "-m", message)
	commitCmd.Dir = r.Path()
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName,
		"GIT_COMMITTER_EMAIL="+authorEmail,
	)
	commitOut, err := commitCmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to create commit")
	}
	newCommit := strings.TrimSpace(string(commitOut))
	
	// Update branch reference
	_, _, err = r.Git("update-ref", "refs/heads/"+branch, newCommit)
	if err != nil {
		return errors.Wrap(err, "failed to update branch")
	}
	
	// Update repository activity
	r.UpdateLastActivity()
	
	return nil
}

// createInitialCommit creates the first commit in an empty repository
func (r *Repository) createInitialCommit(branch, path, content, message, authorName, authorEmail string) error {
	// Write content to object database
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = r.Path()
	cmd.Stdin = strings.NewReader(content)
	hashOut, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to write initial object")
	}
	blobHash := strings.TrimSpace(string(hashOut))
	
	// Create index from scratch
	_, _, err = r.Git("update-index", "--add", "--cacheinfo", "100644", blobHash, path)
	if err != nil {
		return errors.Wrap(err, "failed to create initial index")
	}
	
	// Write tree
	treeOut, _, err := r.Git("write-tree")
	if err != nil {
		return errors.Wrap(err, "failed to write initial tree")
	}
	treeHash := strings.TrimSpace(treeOut.String())
	
	// Create commit without parent
	commitCmd := exec.Command("git", "commit-tree", treeHash, "-m", message)
	commitCmd.Dir = r.Path()
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName,
		"GIT_COMMITTER_EMAIL="+authorEmail,
	)
	commitOut, err := commitCmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to create initial commit")
	}
	newCommit := strings.TrimSpace(string(commitOut))
	
	// Create and update branch
	_, _, err = r.Git("update-ref", "refs/heads/"+branch, newCommit)
	if err != nil {
		return errors.Wrap(err, "failed to create branch")
	}
	
	// Set as default branch if this is the first branch
	if r.DefaultBranch == "" {
		r.DefaultBranch = branch
		Repositories.Update(r)
	}
	
	// Update repository state
	r.State = StateActive
	r.UpdateLastActivity()
	Repositories.Update(r)
	
	return nil
}

// CreateFile creates a new file in the repository
func (r *Repository) CreateFile(branch, path, content, message, authorName, authorEmail string) error {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Check if file already exists
	if r.FileExists(branch, path) {
		return errors.New("file already exists")
	}
	
	return r.WriteFile(branch, path, content, message, authorName, authorEmail)
}

// UpdateFile updates an existing file
func (r *Repository) UpdateFile(branch, path, content, message, authorName, authorEmail string) error {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Check if file exists
	if !r.FileExists(branch, path) {
		return errors.New("file not found")
	}
	
	return r.WriteFile(branch, path, content, message, authorName, authorEmail)
}

// DeleteFile removes a file from the repository
func (r *Repository) DeleteFile(branch, path, message, authorName, authorEmail string) error {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Check if file exists
	if !r.FileExists(branch, path) {
		return errors.New("file not found")
	}
	
	// Set default author if not provided
	if authorName == "" {
		authorName = "Skyscape User"
	}
	if authorEmail == "" {
		authorEmail = "user@skyscape.local"
	}
	
	// Get the current commit hash for the branch
	stdout, _, err := r.Git("rev-parse", branch)
	if err != nil {
		return errors.Wrap(err, "branch not found")
	}
	currentCommit := strings.TrimSpace(stdout.String())
	
	// Get the current tree
	treeOut, _, err := r.Git("cat-file", "-p", currentCommit)
	if err != nil {
		return errors.Wrap(err, "failed to read commit")
	}
	
	// Extract tree hash from commit
	lines := strings.Split(treeOut.String(), "\n")
	var treeHash string
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			treeHash = strings.TrimPrefix(line, "tree ")
			break
		}
	}
	
	// Read tree into index
	_, _, err = r.Git("read-tree", treeHash)
	if err != nil {
		return errors.Wrap(err, "failed to read tree")
	}
	
	// Remove file from index
	_, _, err = r.Git("update-index", "--remove", path)
	if err != nil {
		return errors.Wrap(err, "failed to remove from index")
	}
	
	// Write tree from index
	treeOut2, _, err := r.Git("write-tree")
	if err != nil {
		return errors.Wrap(err, "failed to write tree")
	}
	newTreeHash := strings.TrimSpace(treeOut2.String())
	
	// Create commit
	commitCmd := exec.Command("git", "commit-tree", newTreeHash, "-p", currentCommit, "-m", message)
	commitCmd.Dir = r.Path()
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName,
		"GIT_COMMITTER_EMAIL="+authorEmail,
	)
	commitOut, err := commitCmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to create commit")
	}
	newCommit := strings.TrimSpace(string(commitOut))
	
	// Update branch reference
	_, _, err = r.Git("update-ref", "refs/heads/"+branch, newCommit)
	if err != nil {
		return errors.Wrap(err, "failed to update branch")
	}
	
	// Update repository activity
	r.UpdateLastActivity()
	
	return nil
}

// getLanguageFromExtension returns the programming language based on file extension
func getLanguageFromExtension(ext string) string {
	languages := map[string]string{
		".go":     "go",
		".js":     "javascript",
		".ts":     "typescript",
		".jsx":    "javascript",
		".tsx":    "typescript",
		".py":     "python",
		".rb":     "ruby",
		".java":   "java",
		".c":      "c",
		".cpp":    "cpp",
		".cs":     "csharp",
		".php":    "php",
		".rs":     "rust",
		".swift":  "swift",
		".kt":     "kotlin",
		".scala":  "scala",
		".r":      "r",
		".m":      "objc",
		".mm":     "objcpp",
		".pl":     "perl",
		".sh":     "bash",
		".bash":   "bash",
		".zsh":    "zsh",
		".fish":   "fish",
		".ps1":    "powershell",
		".lua":    "lua",
		".vim":    "vim",
		".md":     "markdown",
		".markdown": "markdown",
		".rst":    "rst",
		".txt":    "text",
		".json":   "json",
		".xml":    "xml",
		".yaml":   "yaml",
		".yml":    "yaml",
		".toml":   "toml",
		".ini":    "ini",
		".cfg":    "cfg",
		".conf":   "conf",
		".html":   "html",
		".htm":    "html",
		".css":    "css",
		".scss":   "scss",
		".sass":   "sass",
		".less":   "less",
		".sql":    "sql",
		".dockerfile": "dockerfile",
		".Dockerfile": "dockerfile",
		".gitignore": "gitignore",
		".env":    "env",
	}
	
	if lang, ok := languages[strings.ToLower(ext)]; ok {
		return lang
	}
	return "text"
}