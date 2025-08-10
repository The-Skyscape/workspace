package models

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
		args = append(args, path+"/")
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
			
			// Clean up the path
			if path != "." && strings.HasPrefix(name, path+"/") {
				name = strings.TrimPrefix(name, path+"/")
			}
			
			node := &FileNode{
				Name: filepath.Base(name),
				Path: name,
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
	
	_, _, err := r.Git("cat-file", "-e", branch+":"+path)
	return err == nil
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

// CreateFile creates a new file in the repository
func (r *Repository) CreateFile(branch, path, content, message, authorName, authorEmail string) error {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}
	
	// Check if file already exists
	if r.FileExists(branch, path) {
		return errors.New("file already exists")
	}
	
	// This would require a working tree, so we need to use git hash-object and update-index
	// For now, return not implemented
	return errors.New("file creation not implemented in bare repository")
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
	
	// This would require a working tree
	return errors.New("file update not implemented in bare repository")
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
	
	// This would require a working tree
	return errors.New("file deletion not implemented in bare repository")
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