package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

// Profile represents a user's public profile settings
type Profile struct {
	application.Model
	
	UserID      string // Foreign key to User
	Name        string // Display name for public profile
	Email       string // Email address (only shown if ShowEmail is true)
	Avatar      string // Avatar URL or generated avatar
	Description string // Short description/tagline
	Bio         string // Biography/description
	Website     string // Personal website URL
	GitHub      string // GitHub username
	Twitter     string // Twitter handle
	LinkedIn    string // LinkedIn profile
	ShowEmail   bool   // Whether to display email publicly
	ShowStats   bool   // Whether to show repository stats
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

