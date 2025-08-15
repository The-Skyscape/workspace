package models

// Repository operations - kept for backward compatibility
// New code should use CreateRepository from repository.go instead

func NewRepo(repoID, name string) (repo *Repository, err error) {
	// This function is deprecated - use CreateRepository instead
	// Creating repository with empty description, private visibility, and no user
	return CreateRepository(name, "", "private", "")
}