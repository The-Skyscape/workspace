package models

import (
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/application"
)

// ActionArtifact represents a file artifact saved from an action execution
type ActionArtifact struct {
	application.Model
	ActionID    string    // ID of the action that created this artifact
	RunID       string    // ID of the specific run that created this artifact
	SandboxName string    // Name of the sandbox that generated the artifact
	FileName    string    // Original filename
	FilePath    string    // Path within sandbox
	GroupName   string    // Group name for similar artifacts (e.g., "build.log", "coverage.xml")
	Version     int       // Version number for this artifact group
	ContentType string    // MIME type of the file
	Size        int64     // File size in bytes
	Content     []byte    // BLOB field for file content
	CreatedAt   time.Time // When the artifact was saved
}

// Table returns the database table name
func (*ActionArtifact) Table() string { return "action_artifacts" }

func init() {
	// Create indexes for action artifacts table
	go func() {
		ActionArtifacts.Index("RunID")
		ActionArtifacts.Index("Version")
		ActionArtifacts.Index("CreatedAt DESC")
	}()
}


// GetByAction returns all artifacts for a specific action
func GetArtifactsByAction(actionID string) ([]*ActionArtifact, error) {
	return ActionArtifacts.Search("WHERE ActionID = ? ORDER BY CreatedAt DESC", actionID)
}

// GetBySandbox returns all artifacts from a specific sandbox
func GetArtifactsBySandbox(sandboxName string) ([]*ActionArtifact, error) {
	return ActionArtifacts.Search("WHERE SandboxName = ? ORDER BY CreatedAt DESC", sandboxName)
}

// GetByRun returns all artifacts for a specific run
func GetArtifactsByRun(runID string) ([]*ActionArtifact, error) {
	return ActionArtifacts.Search("WHERE RunID = ? ORDER BY GroupName, Version DESC", runID)
}

// GetGroupedByAction returns artifacts grouped by GroupName for an action
func GetGroupedArtifactsByAction(actionID string) (map[string][]*ActionArtifact, error) {
	artifacts, err := ActionArtifacts.Search("WHERE ActionID = ? ORDER BY GroupName, Version DESC", actionID)
	if err != nil {
		return nil, err
	}
	
	grouped := make(map[string][]*ActionArtifact)
	for _, artifact := range artifacts {
		groupName := artifact.GroupName
		if groupName == "" {
			groupName = artifact.FileName
		}
		grouped[groupName] = append(grouped[groupName], artifact)
	}
	return grouped, nil
}

// GetNextVersion returns the next version number for a group
func GetNextArtifactVersion(actionID, groupName string) (int, error) {
	artifacts, err := ActionArtifacts.Search("WHERE ActionID = ? AND GroupName = ? ORDER BY Version DESC LIMIT 1", actionID, groupName)
	if err != nil {
		return 1, err
	}
	if len(artifacts) == 0 {
		return 1, nil
	}
	return artifacts[0].Version + 1, nil
}

// Save creates a new artifact record
func (a *ActionArtifact) Save() error {
	// Auto-set group name if not provided
	if a.GroupName == "" {
		a.GroupName = a.FileName
	}
	
	// Auto-set version if not provided
	if a.Version == 0 {
		version, err := GetNextArtifactVersion(a.ActionID, a.GroupName)
		if err != nil {
			return err
		}
		a.Version = version
	}
	
	artifact, err := ActionArtifacts.Insert(a)
	if err != nil {
		return err
	}
	a.ID = artifact.ID
	a.CreatedAt = artifact.CreatedAt
	a.UpdatedAt = artifact.UpdatedAt
	return nil
}