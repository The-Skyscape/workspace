package models

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// PRDiff represents the diff between two branches for a pull request
type PRDiff struct {
	BaseBranch    string
	CompareBranch string
	Files         []DiffFile
	Additions     int
	Deletions     int
	Commits       []*Commit
	CanMerge      bool
	HasConflicts  bool
}

// GetPRDiff returns the diff between two branches for a pull request
func (r *Repository) GetPRDiff(baseBranch, compareBranch string) (*PRDiff, error) {
	if baseBranch == "" || compareBranch == "" {
		return nil, errors.New("both base and compare branches are required")
	}
	
	// Check if branches exist
	if !r.BranchExists(baseBranch) {
		return nil, errors.New("base branch does not exist")
	}
	if !r.BranchExists(compareBranch) {
		return nil, errors.New("compare branch does not exist")
	}
	
	prDiff := &PRDiff{
		BaseBranch:    baseBranch,
		CompareBranch: compareBranch,
		Files:         []DiffFile{},
	}
	
	// Get commits between branches
	commits, err := r.GetCommitsBetween(baseBranch, compareBranch)
	if err == nil {
		prDiff.Commits = commits
	}
	
	// Get diff statistics and content
	// Use three-dot notation to show changes on compare branch since it diverged from base
	stdout, stderr, err := r.Git("diff", "--stat", baseBranch+"..."+compareBranch)
	if err != nil {
		// If diff fails, branches might be unrelated
		return prDiff, errors.Wrap(err, stderr.String())
	}
	
	// Parse diff statistics
	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Parse file changes (format: "file.txt | 10 ++++------")
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) == 2 {
				path := strings.TrimSpace(parts[0])
				changes := strings.TrimSpace(parts[1])
				
				// Count additions and deletions
				additions := strings.Count(changes, "+")
				deletions := strings.Count(changes, "-")
				
				// Determine status
				status := "modified"
				if additions > 0 && deletions == 0 {
					status = "added"
				} else if deletions > 0 && additions == 0 {
					status = "deleted"
				}
				
				diffFile := DiffFile{
					Path:      path,
					Additions: additions,
					Deletions: deletions,
					Status:    status,
				}
				
				prDiff.Files = append(prDiff.Files, diffFile)
				prDiff.Additions += additions
				prDiff.Deletions += deletions
			}
		}
	}
	
	// Check if branches can be merged
	canMerge, hasConflicts := r.CheckMergeability(baseBranch, compareBranch)
	prDiff.CanMerge = canMerge
	prDiff.HasConflicts = hasConflicts
	
	return prDiff, nil
}

// GetPRDiffContent returns the full diff content between two branches
func (r *Repository) GetPRDiffContent(baseBranch, compareBranch string) (string, error) {
	if baseBranch == "" || compareBranch == "" {
		return "", errors.New("both base and compare branches are required")
	}
	
	// Check if branches exist
	if !r.BranchExists(baseBranch) {
		return "", fmt.Errorf("base branch %s does not exist", baseBranch)
	}
	if !r.BranchExists(compareBranch) {
		return "", fmt.Errorf("compare branch %s does not exist", compareBranch)
	}
	
	// Get full diff with context using three-dot notation
	// This shows changes on compare branch since it diverged from base
	stdout, stderr, err := r.Git("diff", baseBranch+"..."+compareBranch)
	if err != nil {
		return "", errors.Wrap(err, stderr.String())
	}
	
	return stdout.String(), nil
}

// CheckMergeability checks if two branches can be merged without conflicts
func (r *Repository) CheckMergeability(baseBranch, compareBranch string) (canMerge bool, hasConflicts bool) {
	// Find merge base
	mergeBase, _, err := r.Git("merge-base", baseBranch, compareBranch)
	if err != nil {
		// No common ancestor, cannot merge
		return false, true
	}
	
	mergeBaseHash := strings.TrimSpace(mergeBase.String())
	
	// Check if fast-forward is possible
	baseHash, _, _ := r.Git("rev-parse", baseBranch)
	if strings.TrimSpace(baseHash.String()) == mergeBaseHash {
		// Fast-forward merge possible, no conflicts
		return true, false
	}
	
	// Try a test merge using merge-tree
	// This simulates a merge without actually changing any refs
	mergeTreeOut, _, err := r.Git("merge-tree", mergeBaseHash, baseBranch, compareBranch)
	if err != nil {
		// Error in merge-tree, assume conflicts
		return false, true
	}
	
	// Check for conflict markers in the output
	output := mergeTreeOut.String()
	if strings.Contains(output, "<<<<<<<") || strings.Contains(output, ">>>>>>>") {
		// Conflicts detected
		return false, true
	}
	
	// No conflicts, merge is possible
	return true, false
}

// CanMergeBranch checks if a branch can be merged into another
func (r *Repository) CanMergeBranch(sourceBranch, targetBranch string) (bool, error) {
	if sourceBranch == "" || targetBranch == "" {
		return false, errors.New("both source and target branches are required")
	}
	
	// Check if branches exist
	if !r.BranchExists(sourceBranch) {
		return false, fmt.Errorf("source branch %s does not exist", sourceBranch)
	}
	if !r.BranchExists(targetBranch) {
		return false, fmt.Errorf("target branch %s does not exist", targetBranch)
	}
	
	canMerge, _ := r.CheckMergeability(targetBranch, sourceBranch)
	return canMerge, nil
}

// MergeBranch merges one branch into another
func (r *Repository) MergeBranch(sourceBranch, targetBranch, message, authorName, authorEmail string) error {
	if sourceBranch == "" || targetBranch == "" {
		return errors.New("both source and target branches are required")
	}
	
	// Set defaults
	if message == "" {
		message = fmt.Sprintf("Merge branch '%s' into %s", sourceBranch, targetBranch)
	}
	if authorName == "" {
		authorName = "Skyscape User"
	}
	if authorEmail == "" {
		authorEmail = "user@skyscape.local"
	}
	
	// Check if branches exist
	if !r.BranchExists(sourceBranch) {
		return fmt.Errorf("source branch %s does not exist", sourceBranch)
	}
	if !r.BranchExists(targetBranch) {
		return fmt.Errorf("target branch %s does not exist", targetBranch)
	}
	
	// Get commit hashes
	sourceHash, _, err := r.Git("rev-parse", sourceBranch)
	if err != nil {
		return errors.Wrap(err, "failed to get source branch commit")
	}
	sourceCommit := strings.TrimSpace(sourceHash.String())
	
	targetHash, _, err := r.Git("rev-parse", targetBranch)
	if err != nil {
		return errors.Wrap(err, "failed to get target branch commit")
	}
	targetCommit := strings.TrimSpace(targetHash.String())
	
	// Find merge base
	mergeBase, _, err := r.Git("merge-base", targetBranch, sourceBranch)
	if err != nil {
		return errors.Wrap(err, "no common ancestor found")
	}
	mergeBaseHash := strings.TrimSpace(mergeBase.String())
	
	// Check if fast-forward is possible
	if targetCommit == mergeBaseHash {
		// Fast-forward merge - just update the ref
		_, _, err = r.Git("update-ref", "refs/heads/"+targetBranch, sourceCommit)
		if err != nil {
			return errors.Wrap(err, "failed to fast-forward merge")
		}
		
		r.UpdateLastActivity()
		return nil
	}
	
	// Need to create a merge commit
	// First, check for conflicts using merge-tree
	mergeTreeOut, _, err := r.Git("merge-tree", mergeBaseHash, targetCommit, sourceCommit)
	if err != nil {
		return errors.Wrap(err, "failed to test merge")
	}
	
	// Check for conflicts
	if strings.Contains(mergeTreeOut.String(), "<<<<<<<") {
		return errors.New("merge conflicts detected - cannot auto-merge")
	}
	
	// Perform the actual merge using read-tree
	// Read the target tree into index
	targetTreeOut, _, err := r.Git("cat-file", "-p", targetCommit)
	if err != nil {
		return errors.Wrap(err, "failed to read target commit")
	}
	
	// Extract tree hash from commit
	lines := strings.Split(targetTreeOut.String(), "\n")
	var targetTree string
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			targetTree = strings.TrimPrefix(line, "tree ")
			break
		}
	}
	
	// Get source tree
	sourceTreeOut, _, err := r.Git("cat-file", "-p", sourceCommit)
	if err != nil {
		return errors.Wrap(err, "failed to read source commit")
	}
	
	var sourceTree string
	lines = strings.Split(sourceTreeOut.String(), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			sourceTree = strings.TrimPrefix(line, "tree ")
			break
		}
	}
	
	// Perform three-way merge
	_, _, err = r.Git("read-tree", "-m", mergeBaseHash, targetTree, sourceTree)
	if err != nil {
		return errors.Wrap(err, "failed to merge trees")
	}
	
	// Write the merged tree
	mergedTree, _, err := r.Git("write-tree")
	if err != nil {
		return errors.Wrap(err, "failed to write merged tree")
	}
	mergedTreeHash := strings.TrimSpace(mergedTree.String())
	
	// Create merge commit with two parents
	cmd := exec.Command("git", "commit-tree", mergedTreeHash, 
		"-p", targetCommit, "-p", sourceCommit, "-m", message)
	cmd.Dir = r.Path()
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName,
		"GIT_COMMITTER_EMAIL="+authorEmail,
	)
	
	mergeCommitOut, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to create merge commit")
	}
	mergeCommit := strings.TrimSpace(string(mergeCommitOut))
	
	// Update target branch to point to merge commit
	_, _, err = r.Git("update-ref", "refs/heads/"+targetBranch, mergeCommit)
	if err != nil {
		return errors.Wrap(err, "failed to update branch reference")
	}
	
	r.UpdateLastActivity()
	return nil
}