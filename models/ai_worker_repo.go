package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

// AIWorkerRepo represents the many-to-many relationship between workers and repositories
type AIWorkerRepo struct {
	application.Model
	WorkerID string // AI Worker ID
	RepoID   string // Repository ID
}

// Table returns the database table name
func (*AIWorkerRepo) Table() string { return "ai_worker_repos" }

// GetRepositoriesForWorker returns all repositories associated with a worker
func GetRepositoriesForWorker(workerID string) ([]*Repository, error) {
	// Get all worker-repo associations
	workerRepos, err := AIWorkerRepos.Search("WHERE WorkerID = ?", workerID)
	if err != nil {
		return nil, err
	}

	// Fetch each repository
	var repos []*Repository
	for _, wr := range workerRepos {
		repo, err := Repositories.Get(wr.RepoID)
		if err == nil && repo != nil {
			repos = append(repos, repo)
		}
	}

	return repos, nil
}

// AddRepositoryToWorker associates a repository with a worker
func AddRepositoryToWorker(workerID, repoID string) error {
	workerRepo := &AIWorkerRepo{
		WorkerID: workerID,
		RepoID:   repoID,
	}
	_, err := AIWorkerRepos.Insert(workerRepo)
	return err
}

// RemoveRepositoryFromWorker removes a repository association
func RemoveRepositoryFromWorker(workerID, repoID string) error {
	workerRepos, err := AIWorkerRepos.Search("WHERE WorkerID = ? AND RepoID = ?", workerID, repoID)
	if err != nil {
		return err
	}
	
	for _, wr := range workerRepos {
		if err := AIWorkerRepos.Delete(wr); err != nil {
			return err
		}
	}
	
	return nil
}