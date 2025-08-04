package models

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

type GitRepo struct {
	application.Model
	Name        string
	Description string
	Visibility  string
	UserID      string // Owner of the repository
}

func (*GitRepo) Table() string { return "code_repos" }

func (repo *GitRepo) Path() string {
	return filepath.Join(database.DataDir(), "repos", repo.ID)
}

func (repo *GitRepo) IsEmpty(branch string) bool {
	_, _, err := repo.Run("rev-parse", "--verify", branch)
	if err != nil {
		return true
	}

	stdout, _, err := repo.Run("rev-list", "--count", branch)
	if err != nil {
		return true
	}
	count, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		return true
	}
	return count == 0
}

func (repo *GitRepo) Open(branch, path string) (*GitBlob, error) {
	if path != "" && path[0] == '/' {
		path = path[1:]
	}
	isDir, err := repo.isDir(branch, path)
	if err != nil {
		return nil, err
	}

	return &GitBlob{Repo: repo, IsDirectory: isDir, Exists: true, Branch: branch, Path: path}, nil
}

func (repo *GitRepo) Blobs(branch, path string) (blobs []*GitBlob, err error) {
	stdout, stderr, err := repo.Run("ls-tree", branch, filepath.Join(".", path)+"/")
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}

	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if parts := strings.Fields(line); len(parts) >= 4 {
			blobs = append(blobs, &GitBlob{
				Repo:        repo,
				Branch:      branch,
				Exists:      true,
				IsDirectory: parts[1] == "tree",
				Path:        parts[3],
			})
		}
	}

	sort.Slice(blobs, func(i, j int) bool {
		if blobs[i].IsDirectory && !blobs[j].IsDirectory {
			return true
		}
		if !blobs[i].IsDirectory && blobs[j].IsDirectory {
			return false
		}
		return blobs[i].Path < blobs[j].Path
	})

	return blobs, nil
}

func (repo *GitRepo) Run(args ...string) (stdout bytes.Buffer, stderr bytes.Buffer, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo.Path()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return stdout, stderr, cmd.Run()
}

func (repo *GitRepo) isDir(branch, path string) (bool, error) {
	if path == "" || path == "." {
		return true, nil
	}

	stdout, stderr, err := repo.Run("ls-tree", branch, filepath.Join(".", path))
	if err != nil {
		return false, errors.Wrap(err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return false, errors.New("no such file or directory")
	}

	parts := strings.Fields(output)
	return parts[1] == "tree", nil
}

// GitCommit represents a commit in the repository
type GitCommit struct {
	Hash      string
	Message   string
	Author    string
	Email     string
	Date      string
	ShortHash string
}

// GetCommits retrieves recent commits from the repository
func (repo *GitRepo) GetCommits(branch string, limit int) ([]*GitCommit, error) {
	if branch == "" {
		branch = "HEAD"
	}
	
	// Check if repository has any commits
	if repo.IsEmpty(branch) {
		return []*GitCommit{}, nil
	}
	
	args := []string{"log", "--oneline", "--pretty=format:%H|%s|%an|%ae|%ad", "--date=format:%Y-%m-%d %H:%M:%S"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	args = append(args, branch)

	stdout, stderr, err := repo.Run(args...)
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}

	var commits []*GitCommit
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			commit := &GitCommit{
				Hash:      parts[0],
				Message:   parts[1],
				Author:    parts[2],
				Email:     parts[3],
				Date:      parts[4],
				ShortHash: parts[0][:8],
			}
			commits = append(commits, commit)
		}
	}

	return commits, nil
}

// GitBranch represents a branch in the repository
type GitBranch struct {
	Name      string
	IsDefault bool
	IsCurrent bool
	LastCommit string
}

// GetBranches retrieves all branches from the repository
func (repo *GitRepo) GetBranches() ([]*GitBranch, error) {
	// Check if repository has any commits first
	if repo.IsEmpty("HEAD") {
		// Return just the default branch for empty repos
		return []*GitBranch{{
			Name:      "main",
			IsDefault: true,
			IsCurrent: true,
			LastCommit: "",
		}}, nil
	}

	// Get all branches
	stdout, stderr, err := repo.Run("branch", "-a", "--format=%(refname:short)|%(HEAD)|%(objectname:short)")
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}

	var branches []*GitBranch
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 3 {
			branchName := parts[0]
			isCurrent := parts[1] == "*"
			lastCommit := parts[2]

			// Skip remote tracking branches for now (origin/branch)
			if strings.HasPrefix(branchName, "origin/") {
				continue
			}

			// Determine if this is the default branch (main or master)
			isDefault := branchName == "main" || branchName == "master"

			branches = append(branches, &GitBranch{
				Name:       branchName,
				IsDefault:  isDefault,
				IsCurrent:  isCurrent,
				LastCommit: lastCommit,
			})
		}
	}

	// Sort branches: current first, then default, then alphabetically
	sort.Slice(branches, func(i, j int) bool {
		if branches[i].IsCurrent && !branches[j].IsCurrent {
			return true
		}
		if !branches[i].IsCurrent && branches[j].IsCurrent {
			return false
		}
		if branches[i].IsDefault && !branches[j].IsDefault {
			return true
		}
		if !branches[i].IsDefault && branches[j].IsDefault {
			return false
		}
		return branches[i].Name < branches[j].Name
	})

	return branches, nil
}

// CreateBranch creates a new branch from the current HEAD
func (repo *GitRepo) CreateBranch(branchName string) error {
	if branchName == "" {
		return errors.New("branch name cannot be empty")
	}

	// Check if repository is empty
	if repo.IsEmpty("HEAD") {
		return errors.New("cannot create branch in empty repository")
	}

	// Create the branch
	_, stderr, err := repo.Run("checkout", "-b", branchName)
	if err != nil {
		return errors.Wrap(err, "failed to create branch: "+stderr.String())
	}

	return nil
}

// SwitchBranch switches to an existing branch
func (repo *GitRepo) SwitchBranch(branchName string) error {
	if branchName == "" {
		return errors.New("branch name cannot be empty")
	}

	// Switch to the branch
	_, stderr, err := repo.Run("checkout", branchName)
	if err != nil {
		return errors.Wrap(err, "failed to switch to branch: "+stderr.String())
	}

	return nil
}

// MergeBranch merges a source branch into a target branch
func (repo *GitRepo) MergeBranch(sourceBranch, targetBranch string) error {
	if sourceBranch == "" || targetBranch == "" {
		return errors.New("source and target branch names cannot be empty")
	}

	if sourceBranch == targetBranch {
		return errors.New("cannot merge branch into itself")
	}

	// Check if both branches exist
	branches, err := repo.GetBranches()
	if err != nil {
		return errors.Wrap(err, "failed to get branches")
	}

	var sourceExists, targetExists bool
	for _, branch := range branches {
		if branch.Name == sourceBranch {
			sourceExists = true
		}
		if branch.Name == targetBranch {
			targetExists = true
		}
	}

	if !sourceExists {
		return errors.New("source branch '" + sourceBranch + "' does not exist")
	}
	if !targetExists {
		return errors.New("target branch '" + targetBranch + "' does not exist")
	}

	// Switch to target branch
	err = repo.SwitchBranch(targetBranch)
	if err != nil {
		return errors.Wrap(err, "failed to switch to target branch")
	}

	// Perform the merge
	_, stderr, err := repo.Run("merge", "--no-ff", "-m", "Merge branch '"+sourceBranch+"' into '"+targetBranch+"'", sourceBranch)
	if err != nil {
		// Check if it's a merge conflict
		if strings.Contains(stderr.String(), "CONFLICT") {
			return errors.New("merge conflict detected - manual resolution required")
		}
		return errors.Wrap(err, "failed to merge branches: "+stderr.String())
	}

	return nil
}

// CanMergeBranch checks if a source branch can be merged into a target branch without conflicts
func (repo *GitRepo) CanMergeBranch(sourceBranch, targetBranch string) (bool, error) {
	if sourceBranch == "" || targetBranch == "" {
		return false, errors.New("source and target branch names cannot be empty")
	}

	if sourceBranch == targetBranch {
		return false, errors.New("cannot merge branch into itself")
	}

	// Test merge without actually committing
	_, stderr, err := repo.Run("merge-tree", "$(git merge-base "+targetBranch+" "+sourceBranch+")", targetBranch, sourceBranch)
	if err != nil {
		return false, errors.Wrap(err, "failed to check merge compatibility: "+stderr.String())
	}

	// If stderr is empty, merge should be clean
	return stderr.String() == "", nil
}

// GitDiff represents a diff between two git references
type GitDiff struct {
	FilePath    string
	Status      string // "added", "modified", "deleted", "renamed"
	Additions   int
	Deletions   int
	Changes     []*GitDiffHunk
	IsBinary    bool
	OldFilePath string // For renamed files
}

// GitDiffHunk represents a chunk of changes in a file
type GitDiffHunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Header   string
	Lines    []*GitDiffLine
}

// GitDiffLine represents a single line in a diff
type GitDiffLine struct {
	Type    string // "add", "delete", "context" 
	Content string
	OldNum  int
	NewNum  int
}

// GetPRDiff gets the diff between two branches (for pull requests)
func (repo *GitRepo) GetPRDiff(baseBranch, compareBranch string) ([]*GitDiff, error) {
	if baseBranch == "" || compareBranch == "" {
		return nil, errors.New("base and compare branch names cannot be empty")
	}

	// Get basic file stats first
	stdout, stderr, err := repo.Run("diff", "--name-status", baseBranch+"..."+compareBranch)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get diff stats: "+stderr.String())
	}

	var diffs []*GitDiff
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			status := parts[0]
			filePath := parts[1]
			
			diff := &GitDiff{
				FilePath: filePath,
				Status:   getStatusName(status),
			}

			// For renamed files
			if strings.HasPrefix(status, "R") && len(parts) >= 3 {
				diff.OldFilePath = filePath
				diff.FilePath = parts[2]
			}

			diffs = append(diffs, diff)
		}
	}

	// Get detailed diff for each file
	for _, diff := range diffs {
		err := repo.populateDiffDetails(diff, baseBranch, compareBranch)
		if err != nil {
			// Log error but continue with other files
			continue
		}
	}

	return diffs, nil
}

// GetCommitDiff gets the diff for a specific commit
func (repo *GitRepo) GetCommitDiff(commitHash string) ([]*GitDiff, error) {
	if commitHash == "" {
		return nil, errors.New("commit hash cannot be empty")
	}

	// Get basic file stats for the commit
	stdout, stderr, err := repo.Run("diff-tree", "--name-status", "-r", commitHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get commit diff stats: "+stderr.String())
	}

	var diffs []*GitDiff
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			status := parts[0]
			filePath := parts[1]
			
			diff := &GitDiff{
				FilePath: filePath,
				Status:   getStatusName(status),
			}

			// For renamed files
			if strings.HasPrefix(status, "R") && len(parts) >= 3 {
				diff.OldFilePath = filePath
				diff.FilePath = parts[2]
			}

			diffs = append(diffs, diff)
		}
	}

	// Get detailed diff for each file
	for _, diff := range diffs {
		err := repo.populateCommitDiffDetails(diff, commitHash)
		if err != nil {
			// Log error but continue with other files
			continue
		}
	}

	return diffs, nil
}

// populateDiffDetails fills in the detailed diff information for a file
func (repo *GitRepo) populateDiffDetails(diff *GitDiff, baseBranch, compareBranch string) error {
	// Get detailed diff for this file
	stdout, stderr, err := repo.Run("diff", "--no-color", "-U3", baseBranch+"..."+compareBranch, "--", diff.FilePath)
	if err != nil {
		return errors.Wrap(err, "failed to get detailed diff: "+stderr.String())
	}

	return repo.parseDiffOutput(diff, stdout.String())
}

// populateCommitDiffDetails fills in the detailed diff information for a commit
func (repo *GitRepo) populateCommitDiffDetails(diff *GitDiff, commitHash string) error {
	// Get detailed diff for this file in the commit
	stdout, stderr, err := repo.Run("show", "--no-color", "-U3", "--format=", commitHash, "--", diff.FilePath)
	if err != nil {
		return errors.Wrap(err, "failed to get commit diff details: "+stderr.String())
	}

	return repo.parseDiffOutput(diff, stdout.String())
}

// parseDiffOutput parses the git diff output into structured data
func (repo *GitRepo) parseDiffOutput(diff *GitDiff, diffOutput string) error {
	if diffOutput == "" {
		return nil
	}

	lines := strings.Split(diffOutput, "\n")
	var currentHunk *GitDiffHunk
	
	for i, line := range lines {
		// Skip header lines
		if strings.HasPrefix(line, "diff --git") || 
		   strings.HasPrefix(line, "index ") ||
		   strings.HasPrefix(line, "+++") ||
		   strings.HasPrefix(line, "---") {
			continue
		}

		// Check for binary file
		if strings.Contains(line, "Binary files") {
			diff.IsBinary = true
			return nil
		}

		// Hunk header
		if strings.HasPrefix(line, "@@") {
			currentHunk = &GitDiffHunk{
				Header: line,
			}
			diff.Changes = append(diff.Changes, currentHunk)
			continue
		}

		// Diff lines
		if currentHunk != nil && i < len(lines) {
			diffLine := &GitDiffLine{
				Content: line,
			}

			if len(line) > 0 {
				switch line[0] {
				case '+':
					diffLine.Type = "add"
					diff.Additions++
				case '-':
					diffLine.Type = "delete"
					diff.Deletions++
				default:
					diffLine.Type = "context"
				}
			} else {
				diffLine.Type = "context"
			}

			currentHunk.Lines = append(currentHunk.Lines, diffLine)
		}
	}

	return nil
}

// getStatusName converts git status codes to readable names
func getStatusName(status string) string {
	switch {
	case status == "A":
		return "added"
	case status == "M":
		return "modified"
	case status == "D":
		return "deleted"
	case strings.HasPrefix(status, "R"):
		return "renamed"
	case strings.HasPrefix(status, "C"):
		return "copied"
	default:
		return "modified"
	}
}

// WriteFile writes content to a file and commits the change
func (repo *GitRepo) WriteFile(branch, path, content, commitMessage, userID string) error {
	// Switch to the target branch
	err := repo.SwitchBranch(branch)
	if err != nil {
		return errors.Wrap(err, "failed to switch to branch")
	}

	// Write file content
	repoPath := repo.Path()
	filePath := filepath.Join(repoPath, path)
	
	// Ensure directory exists
	err = exec.Command("mkdir", "-p", filepath.Dir(filePath)).Run()
	if err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	// Write the file
	_, _, err = repo.Run("bash", "-c", "cat > "+shellescape(filePath)+" <<'EOF'\n"+content+"\nEOF")
	if err != nil {
		return errors.Wrap(err, "failed to write file")
	}

	// Stage the file
	_, _, err = repo.Run("add", path)
	if err != nil {
		return errors.Wrap(err, "failed to stage file")
	}

	// Get user info for commit
	user, err := Auth.Users.Get(userID)
	if err != nil {
		return errors.Wrap(err, "failed to get user info")
	}

	// Commit the change
	_, _, err = repo.Run("commit", "-m", commitMessage, "--author", user.Name+" <"+user.Email+">")
	if err != nil {
		return errors.Wrap(err, "failed to commit changes")
	}

	return nil
}

// DeleteFile removes a file and commits the change
func (repo *GitRepo) DeleteFile(branch, path, commitMessage, userID string) error {
	// Switch to the target branch
	err := repo.SwitchBranch(branch)
	if err != nil {
		return errors.Wrap(err, "failed to switch to branch")
	}

	// Remove the file
	_, _, err = repo.Run("rm", path)
	if err != nil {
		return errors.Wrap(err, "failed to remove file")
	}

	// Get user info for commit
	user, err := Auth.Users.Get(userID)
	if err != nil {
		return errors.Wrap(err, "failed to get user info")
	}

	// Commit the deletion
	_, _, err = repo.Run("commit", "-m", commitMessage, "--author", user.Name+" <"+user.Email+">")
	if err != nil {
		return errors.Wrap(err, "failed to commit changes")
	}

	return nil
}

// shellescape escapes a string for safe use in shell commands
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}