package coding

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/database"

	"github.com/pkg/errors"
)

func (*GitRepo) Table() string { return "code_repos" }

type GitRepo struct {
	database.Model
	Name        string
	Description string
	Visibility  string
	UserID      string // Owner of the repository
}

func (r *Repository) NewRepo(repoID, name string) (repo *GitRepo, err error) {
	// Create model with custom ID if provided, otherwise generate one
	var model database.Model
	if repoID == "" {
		model = r.repos.DB.NewModel("")
	} else {
		model = r.repos.DB.NewModel(repoID)
	}
	
	repo, err = r.repos.Insert(&GitRepo{
		Model:      model,
		Name:       name,
		Visibility: "private",
		UserID:     "", // Will be set by the caller
	})
	if err != nil {
		return nil, err
	}

	if err = os.Mkdir(repo.Path(), 0755); err != nil {
		return nil, err
	}

	_, _, err = repo.Run("init", "--bare")
	return repo, err
}

func (r *Repository) GetRepo(id string) (*GitRepo, error) {
	return r.repos.Get(id)
}

func (r *Repository) SearchRepos(query string, args ...any) ([]*GitRepo, error) {
	return r.repos.Search(query, args...)
}

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

	return &GitBlob{repo, isDir, true, branch, path}, nil
}

func (repo *GitRepo) Blobs(branch, path string) (blobs []*GitBlob, err error) {
	stdout, stderr, err := repo.Run("ls-tree", branch, filepath.Join(".", path)+"/")
	if err != nil {
		return nil, errors.Wrap(err, stderr.String())
	}

	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if parts := strings.Fields(line); len(parts) >= 4 {
			blobs = append(blobs, &GitBlob{
				repo:   repo,
				Branch: branch,
				Exists: true,
				isDir:  parts[1] == "tree",
				Path:   parts[3],
			})
		}
	}

	sort.Slice(blobs, func(i, j int) bool {
		if blobs[i].isDir && !blobs[j].isDir {
			return true
		}
		if !blobs[i].isDir && blobs[j].isDir {
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
