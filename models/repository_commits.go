package models

import (
	"bytes"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Commit represents a git commit
type Commit struct {
	Hash      string
	Message   string
	Author    string
	Email     string
	Date      time.Time
	ShortHash string
}

// Diff represents changes in a commit
type Diff struct {
	Files     []DiffFile
	Additions int
	Deletions int
}

// DiffFile represents changes in a single file
type DiffFile struct {
	Path      string
	Additions int
	Deletions int
	Status    string // "added", "modified", "deleted"
}

// GetCommits retrieves commit history
func (r *Repository) GetCommits(branch string, limit int) ([]*Commit, error) {
	if branch == "" {
		branch = r.GetDefaultBranch()
	}

	// Check if repository has any commits
	if r.IsEmpty() {
		return []*Commit{}, nil
	}

	// Build git log command
	args := []string{
		"log",
		"--pretty=format:%H|%h|%s|%an|%ae|%ad",
		"--date=iso",
	}

	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	args = append(args, branch)

	stdout, stderr, err := r.Git(args...)
	if err != nil {
		if strings.Contains(stderr.String(), "does not have any commits") {
			return []*Commit{}, nil
		}
		return nil, errors.Wrap(err, stderr.String())
	}

	var commits []*Commit
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 6 {
			date, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[5])

			commit := &Commit{
				Hash:      parts[0],
				ShortHash: parts[1],
				Message:   parts[2],
				Author:    parts[3],
				Email:     parts[4],
				Date:      date,
			}

			commits = append(commits, commit)
		}
	}

	return commits, nil
}

// GetCommit retrieves a specific commit
func (r *Repository) GetCommit(hash string) (*Commit, error) {
	stdout, stderr, err := r.Git(
		"show",
		"--pretty=format:%H|%h|%s|%an|%ae|%ad",
		"--date=iso",
		"--no-patch",
		hash,
	)
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}

	line := strings.TrimSpace(stdout.String())
	parts := strings.Split(line, "|")

	if len(parts) >= 6 {
		date, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[5])

		return &Commit{
			Hash:      parts[0],
			ShortHash: parts[1],
			Message:   parts[2],
			Author:    parts[3],
			Email:     parts[4],
			Date:      date,
		}, nil
	}

	return nil, errors.New("invalid commit format")
}

// GetCommitDiff retrieves the diff for a commit
func (r *Repository) GetCommitDiff(hash string) (*Diff, error) {
	// Get diff statistics
	stdout, _, err := r.Git("diff", "--stat", hash+"^", hash)
	if err != nil {
		stderr := &bytes.Buffer{}
		// Might be the first commit
		stdout, stderr, err = r.Git("diff", "--stat", "--root", hash)
		if err != nil {
			return nil, errors.Wrap(err, stderr.String())
		}
	}

	diff := &Diff{
		Files: []DiffFile{},
	}

	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse file changes
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) == 2 {
				path := strings.TrimSpace(parts[0])
				changes := strings.TrimSpace(parts[1])

				additions := strings.Count(changes, "+")
				deletions := strings.Count(changes, "-")

				// Determine status based on the diff
				status := "modified"
				if strings.Contains(changes, "Bin") {
					status = "binary"
				}

				diffFile := DiffFile{
					Path:      path,
					Additions: additions,
					Deletions: deletions,
					Status:    status,
				}

				diff.Files = append(diff.Files, diffFile)
				diff.Additions += additions
				diff.Deletions += deletions
			}
		}
	}

	// Get more detailed status for each file
	nameStatusOut, _, _ := r.Git("diff", "--name-status", hash+"^", hash)
	if nameStatusOut.String() == "" {
		// Try for first commit
		nameStatusOut, _, _ = r.Git("diff", "--name-status", "--root", hash)
	}
	
	statusLines := strings.Split(nameStatusOut.String(), "\n")
	statusMap := make(map[string]string)
	for _, line := range statusLines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			status := ""
			switch parts[0] {
			case "A":
				status = "added"
			case "M":
				status = "modified"
			case "D":
				status = "deleted"
			case "R":
				status = "renamed"
			case "C":
				status = "copied"
			}
			statusMap[parts[1]] = status
		}
	}
	
	// Update file statuses
	for i := range diff.Files {
		if status, ok := statusMap[diff.Files[i].Path]; ok {
			diff.Files[i].Status = status
		}
	}

	return diff, nil
}

// GetCommitDiffContent retrieves the full patch content for a commit
func (r *Repository) GetCommitDiffContent(hash string) (string, error) {
	// Get full diff with patch
	stdout, _, err := r.Git("diff", hash+"^", hash)
	if err != nil {
		// Might be the first commit
		stdout, _, err = r.Git("diff", "--root", hash)
		if err != nil {
			return "", err
		}
	}
	
	return stdout.String(), nil
}

// GetCommitCount returns the number of commits in a branch
func (r *Repository) GetCommitCount(branch string) (int, error) {
	if branch == "" {
		branch = "HEAD"
	}

	stdout, stderr, err := r.Git("rev-list", "--count", branch)
	if err != nil {
		// Might be empty repository
		if strings.Contains(stderr.String(), "does not have any commits") ||
			strings.Contains(stderr.String(), "bad revision") {
			return 0, nil
		}
		return 0, errors.Wrap(err, stderr.String())
	}

	count, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		return 0, err
	}

	return count, nil
}

// GetLatestCommit returns the most recent commit
func (r *Repository) GetLatestCommit(branch string) (*Commit, error) {
	commits, err := r.GetCommits(branch, 1)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return nil, errors.New("no commits found")
	}

	return commits[0], nil
}

// GetCommitsBetween returns commits between two references
func (r *Repository) GetCommitsBetween(from, to string) ([]*Commit, error) {
	if from == "" || to == "" {
		return nil, errors.New("both from and to references are required")
	}

	stdout, stderr, err := r.Git(
		"log",
		"--pretty=format:%H|%h|%s|%an|%ae|%ad",
		"--date=iso",
		from+".."+to,
	)
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}

	var commits []*Commit
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 6 {
			date, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[5])

			commit := &Commit{
				Hash:      parts[0],
				ShortHash: parts[1],
				Message:   parts[2],
				Author:    parts[3],
				Email:     parts[4],
				Date:      date,
			}

			commits = append(commits, commit)
		}
	}

	return commits, nil
}
