package models

import (
	"strings"
	
	"github.com/The-Skyscape/devtools/pkg/application"
)

// TagCategory represents the category of a tag
type TagCategory string

const (
	TagCategoryBug         TagCategory = "bug"
	TagCategoryEnhancement TagCategory = "enhancement"
	TagCategoryPriority    TagCategory = "priority"
	TagCategoryStatus      TagCategory = "status"
	TagCategoryComponent   TagCategory = "component"
	TagCategoryEffort      TagCategory = "effort"
	TagCategorySecurity    TagCategory = "security"
	TagCategoryCustom      TagCategory = "custom"
)

// TagDefinition defines metadata for a tag
type TagDefinition struct {
	application.Model
	Name        string      // Unique tag name
	Category    TagCategory // Tag category
	Color       string      // Hex color code
	Description string      // Tag description
	RepoID      string      // Optional: repo-specific tag
	IsSystem    bool        // System-defined vs user-defined
	SortOrder   int         // Display order
}

func (*TagDefinition) Table() string { return "tag_definitions" }

// IssueLabel represents the many-to-many relationship between issues and tags
type IssueLabel struct {
	application.Model
	IssueID string // References Issue.ID
	TagID   string // References TagDefinition.ID
	AddedBy string // User who added the label
}

func (*IssueLabel) Table() string { return "issue_labels" }

// Issue returns the issue this label belongs to
func (l *IssueLabel) Issue() (*Issue, error) {
	if l.IssueID == "" {
		return nil, nil
	}
	return Issues.Get(l.IssueID)
}

// Tag returns the tag definition for this label
func (l *IssueLabel) Tag() (*TagDefinition, error) {
	if l.TagID == "" {
		return nil, nil
	}
	return TagDefinitions.Get(l.TagID)
}

// AddedByUser returns the user who added this label
func (l *IssueLabel) AddedByUser() (*User, error) {
	if l.AddedBy == "" {
		return nil, nil
	}
	return Users.Get(l.AddedBy)
}

func init() {
	// Create indexes for tag_definitions table
	go func() {
		TagDefinitions.Index("Name")
		TagDefinitions.Index("Category")
		TagDefinitions.Index("RepoID")
		TagDefinitions.Index("IsSystem")
		TagDefinitions.Index("SortOrder")
		// Unique constraint on name within a repo (or globally if RepoID is empty)
		TagDefinitions.Index("Name, RepoID")
	}()
	
	// Create indexes for issue_labels table
	go func() {
		IssueLabels.Index("IssueID")
		IssueLabels.Index("TagID")
		IssueLabels.Index("AddedBy")
		// Unique constraint to prevent duplicate labels
		IssueLabels.Index("IssueID, TagID")
	}()
}

// CreateSystemTags creates default system tags
func CreateSystemTags() error {
	systemTags := []TagDefinition{
		// Bug-related
		{Name: "bug", Category: TagCategoryBug, Color: "#d73a4a", Description: "Something isn't working", IsSystem: true, SortOrder: 1},
		{Name: "regression", Category: TagCategoryBug, Color: "#e11d48", Description: "Previously working functionality is broken", IsSystem: true, SortOrder: 2},
		
		// Enhancement-related
		{Name: "enhancement", Category: TagCategoryEnhancement, Color: "#a2eeef", Description: "New feature or request", IsSystem: true, SortOrder: 10},
		{Name: "feature", Category: TagCategoryEnhancement, Color: "#7c3aed", Description: "New functionality", IsSystem: true, SortOrder: 11},
		{Name: "improvement", Category: TagCategoryEnhancement, Color: "#3b82f6", Description: "Improvement to existing functionality", IsSystem: true, SortOrder: 12},
		
		// Priority-related
		{Name: "critical", Category: TagCategoryPriority, Color: "#b91c1c", Description: "Critical priority", IsSystem: true, SortOrder: 20},
		{Name: "high", Category: TagCategoryPriority, Color: "#dc2626", Description: "High priority", IsSystem: true, SortOrder: 21},
		{Name: "medium", Category: TagCategoryPriority, Color: "#f59e0b", Description: "Medium priority", IsSystem: true, SortOrder: 22},
		{Name: "low", Category: TagCategoryPriority, Color: "#84cc16", Description: "Low priority", IsSystem: true, SortOrder: 23},
		
		// Status-related
		{Name: "wontfix", Category: TagCategoryStatus, Color: "#ffffff", Description: "This will not be worked on", IsSystem: true, SortOrder: 30},
		{Name: "duplicate", Category: TagCategoryStatus, Color: "#cfd3d7", Description: "This issue or pull request already exists", IsSystem: true, SortOrder: 31},
		{Name: "invalid", Category: TagCategoryStatus, Color: "#e4e669", Description: "This doesn't seem right", IsSystem: true, SortOrder: 32},
		{Name: "blocked", Category: TagCategoryStatus, Color: "#d93f0b", Description: "Blocked by another issue or dependency", IsSystem: true, SortOrder: 33},
		{Name: "ready", Category: TagCategoryStatus, Color: "#0e8a16", Description: "Ready to be worked on", IsSystem: true, SortOrder: 34},
		
		// Component-related
		{Name: "documentation", Category: TagCategoryComponent, Color: "#0075ca", Description: "Improvements or additions to documentation", IsSystem: true, SortOrder: 40},
		{Name: "testing", Category: TagCategoryComponent, Color: "#fbca04", Description: "Related to tests", IsSystem: true, SortOrder: 41},
		{Name: "ui", Category: TagCategoryComponent, Color: "#e99695", Description: "User interface", IsSystem: true, SortOrder: 42},
		{Name: "api", Category: TagCategoryComponent, Color: "#5319e7", Description: "API-related", IsSystem: true, SortOrder: 43},
		{Name: "database", Category: TagCategoryComponent, Color: "#006b75", Description: "Database-related", IsSystem: true, SortOrder: 44},
		
		// Effort-related
		{Name: "good first issue", Category: TagCategoryEffort, Color: "#7057ff", Description: "Good for newcomers", IsSystem: true, SortOrder: 50},
		{Name: "easy", Category: TagCategoryEffort, Color: "#c2e0c6", Description: "Easy to implement", IsSystem: true, SortOrder: 51},
		{Name: "hard", Category: TagCategoryEffort, Color: "#f9d0c4", Description: "Difficult to implement", IsSystem: true, SortOrder: 52},
		
		// Security-related
		{Name: "security", Category: TagCategorySecurity, Color: "#d1242f", Description: "Security-related issue", IsSystem: true, SortOrder: 60},
		{Name: "vulnerability", Category: TagCategorySecurity, Color: "#b60205", Description: "Security vulnerability", IsSystem: true, SortOrder: 61},
	}
	
	for _, tag := range systemTags {
		// Check if tag already exists
		existing, err := TagDefinitions.Search("WHERE Name = ? AND IsSystem = ?", tag.Name, true)
		if err != nil {
			return err
		}
		if len(existing) == 0 {
			if _, err := TagDefinitions.Insert(&tag); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// GetOrCreateTag gets an existing tag or creates a new one
func GetOrCreateTag(name string, repoID string) (*TagDefinition, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, nil
	}
	
	// Check for existing tag (repo-specific first, then global)
	tags, err := TagDefinitions.Search("WHERE Name = ? AND (RepoID = ? OR RepoID = '') ORDER BY RepoID DESC LIMIT 1", name, repoID)
	if err != nil {
		return nil, err
	}
	
	if len(tags) > 0 {
		return tags[0], nil
	}
	
	// Create new custom tag
	tag := &TagDefinition{
		Name:     name,
		Category: TagCategoryCustom,
		Color:    "#808080", // Default gray for custom tags
		RepoID:   repoID,
		IsSystem: false,
	}
	
	return TagDefinitions.Insert(tag)
}

// AddLabelToIssue adds a label to an issue
func AddLabelToIssue(issueID, tagID, userID string) error {
	// Check if label already exists
	existing, err := IssueLabels.Search("WHERE IssueID = ? AND TagID = ?", issueID, tagID)
	if err != nil {
		return err
	}
	
	if len(existing) == 0 {
		_, err = IssueLabels.Insert(&IssueLabel{
			IssueID: issueID,
			TagID:   tagID,
			AddedBy: userID,
		})
		return err
	}
	
	return nil
}

// RemoveLabelFromIssue removes a label from an issue
func RemoveLabelFromIssue(issueID, tagID string) error {
	// Note: SQL field names are PascalCase
	labels, err := IssueLabels.Search("WHERE IssueID = ? AND TagID = ?", issueID, tagID)
	if err != nil {
		return err
	}
	
	if len(labels) == 0 {
		// No labels found to delete - this might be OK
		return nil
	}
	
	for _, label := range labels {
		if err := IssueLabels.Delete(label); err != nil {
			return err
		}
	}
	
	return nil
}

// GetIssueLabels returns all labels for an issue
func GetIssueLabels(issueID string) ([]*TagDefinition, error) {
	labels, err := IssueLabels.Search("WHERE IssueID = ?", issueID)
	if err != nil {
		return nil, err
	}
	
	tags := make([]*TagDefinition, 0, len(labels))
	for _, label := range labels {
		tag, err := TagDefinitions.Get(label.TagID)
		if err == nil && tag != nil {
			tags = append(tags, tag)
		}
	}
	
	return tags, nil
}

// GetIssuesByTagID returns all issue IDs with a specific tag
func GetIssuesByTagID(tagID string) ([]string, error) {
	labels, err := IssueLabels.Search("WHERE TagID = ?", tagID)
	if err != nil {
		return nil, err
	}
	
	issueIDs := make([]string, len(labels))
	for i, label := range labels {
		issueIDs[i] = label.IssueID
	}
	
	return issueIDs, nil
}