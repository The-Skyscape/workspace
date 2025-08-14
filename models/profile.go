package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

// Profile represents a user's public profile settings
type Profile struct {
	application.Model
	
	UserID      string `json:"user_id"`      // Foreign key to User
	Bio         string `json:"bio"`           // Biography/description
	Title       string `json:"title"`         // Professional title
	Website     string `json:"website"`       // Personal website URL
	GitHub      string `json:"github"`        // GitHub username
	Twitter     string `json:"twitter"`       // Twitter handle
	LinkedIn    string `json:"linkedin"`      // LinkedIn profile
	ShowEmail   bool   `json:"show_email"`    // Whether to display email publicly
	ShowStats   bool   `json:"show_stats"`    // Whether to show repository stats
}

// Table returns the database table name
func (*Profile) Table() string { return "profiles" }

// GetAdminProfile returns the profile for the admin user (first user)
func GetAdminProfile() (*Profile, error) {
	// Get the first user (admin)
	users, err := Auth.Users.Search("ORDER BY ID ASC LIMIT 1")
	if err != nil || len(users) == 0 {
		return nil, err
	}
	
	adminUser := users[0]
	
	// Try to get existing profile
	profiles, err := Profiles.Search("WHERE UserID = ?", adminUser.ID)
	if err != nil || len(profiles) == 0 {
		// Create default profile if none exists
		profile := &Profile{
			Model:     DB.NewModel(""),
			UserID:    adminUser.ID,
			Bio:       "Welcome to my Skyscape instance",
			Title:     "Software Developer",
			ShowEmail: false,
			ShowStats: false,
		}
		
		// Insert the default profile
		profile, err = Profiles.Insert(profile)
		if err != nil {
			return nil, err
		}
		return profile, nil
	}
	
	return profiles[0], nil
}

// UpdateAdminProfile updates the admin user's profile
func UpdateAdminProfile(updates map[string]interface{}) (*Profile, error) {
	profile, err := GetAdminProfile()
	if err != nil {
		return nil, err
	}
	
	// Update fields if provided
	if bio, ok := updates["bio"].(string); ok {
		profile.Bio = bio
	}
	if title, ok := updates["title"].(string); ok {
		profile.Title = title
	}
	if website, ok := updates["website"].(string); ok {
		profile.Website = website
	}
	if github, ok := updates["github"].(string); ok {
		profile.GitHub = github
	}
	if twitter, ok := updates["twitter"].(string); ok {
		profile.Twitter = twitter
	}
	if linkedin, ok := updates["linkedin"].(string); ok {
		profile.LinkedIn = linkedin
	}
	if showEmail, ok := updates["show_email"].(bool); ok {
		profile.ShowEmail = showEmail
	}
	if showStats, ok := updates["show_stats"].(bool); ok {
		profile.ShowStats = showStats
	}
	
	// Save to database
	err = Profiles.Update(profile)
	if err != nil {
		return nil, err
	}
	
	return profile, nil
}