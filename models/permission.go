package models

import "github.com/The-Skyscape/devtools/pkg/application"

type Permission struct {
	application.Model
	RepoID string
	UserID string
	Role   string // "read", "write", "admin"
}

func (*Permission) Table() string { return "permissions" }