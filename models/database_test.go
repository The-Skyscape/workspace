// +build test

package models

import (
	"os"
	"testing"
)

func init() {
	// During tests, skip database initialization
	// Tests will call SetupTestDB which handles initialization
	if testing.Testing() {
		// Set a flag to prevent services from starting
		os.Setenv("SKIP_SERVICE_INIT", "true")
	}
}