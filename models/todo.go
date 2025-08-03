package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/database"
)

// Table is location in database to store todos
func (*Todo) Table() string { return "todos" }

// Todo is the model for storing todos
type Todo struct {
	database.Model
	Title       string
	Description string
	Completed   bool
	Priority    string
	DueDate     time.Time
	UserID      string
}