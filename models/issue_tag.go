package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type IssueTag struct {
	application.Model
	IssueID string
	Tag     string
}

func (*IssueTag) Table() string { return "issue_tags" }

func init() {
	// Create indexes for issue_tags table
	go func() {
		IssueTags.Index("IssueID")
		IssueTags.Index("Tag")
		// Composite index for unique constraint
		IssueTags.Index("IssueID, Tag")
	}()
}

// AddTagToIssue adds a tag to an issue (if not already present)
func AddTagToIssue(issueID, tag string) error {
	// Check if tag already exists
	existing, err := IssueTags.Search("WHERE IssueID = ? AND Tag = ?", issueID, tag)
	if err != nil {
		return err
	}
	
	if len(existing) == 0 {
		_, err = IssueTags.Insert(&IssueTag{
			IssueID: issueID,
			Tag:     tag,
		})
		return err
	}
	
	return nil
}

// RemoveTagFromIssue removes a tag from an issue
func RemoveTagFromIssue(issueID, tag string) error {
	tags, err := IssueTags.Search("WHERE IssueID = ? AND Tag = ?", issueID, tag)
	if err != nil {
		return err
	}
	
	for _, t := range tags {
		if err := IssueTags.Delete(t); err != nil {
			return err
		}
	}
	
	return nil
}

// GetIssueTags returns all tags for an issue
func GetIssueTags(issueID string) ([]string, error) {
	tags, err := IssueTags.Search("WHERE IssueID = ? ORDER BY Tag", issueID)
	if err != nil {
		return nil, err
	}
	
	result := make([]string, len(tags))
	for i, tag := range tags {
		result[i] = tag.Tag
	}
	
	return result, nil
}

// GetIssuesByTag returns all issue IDs with a specific tag
func GetIssuesByTag(tag string) ([]string, error) {
	tags, err := IssueTags.Search("WHERE Tag = ?", tag)
	if err != nil {
		return nil, err
	}
	
	result := make([]string, len(tags))
	for i, t := range tags {
		result[i] = t.IssueID
	}
	
	return result, nil
}