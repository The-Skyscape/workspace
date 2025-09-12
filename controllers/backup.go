package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"workspace/internal/backup"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// BackupController handles backup and recovery operations
type BackupController struct {
	application.Controller
}

// Backup is the factory function for the backup controller
func Backup() (string, *BackupController) {
	return "backup", &BackupController{}
}

// Setup registers backup routes
func (b *BackupController) Setup(app *application.App) {
	b.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	// Admin-only backup management
	adminRequired := func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			b.Redirect(w, r, "/signin")
			return false
		}
		if !user.IsAdmin {
			b.Redirect(w, r, "/settings/profile")
			return false
		}
		return true
	}

	// Backup management page
	http.Handle("GET /settings/backup", app.Serve("settings-backup.html", adminRequired))

	// API endpoints
	http.Handle("POST /backup/create", app.ProtectFunc(b.createBackup, auth.AdminOnly))
	http.Handle("POST /backup/restore", app.ProtectFunc(b.restoreBackup, auth.AdminOnly))
	http.Handle("GET /backup/list", app.ProtectFunc(b.listBackups, auth.AdminOnly))
	http.Handle("GET /backup/status", app.ProtectFunc(b.getStatus, auth.AdminOnly))
	http.Handle("POST /backup/toggle", app.ProtectFunc(b.toggleScheduler, auth.AdminOnly))

	// HTMX partials
	http.Handle("GET /backup/partial/list", app.ProtectFunc(b.getBackupListPartial, auth.AdminOnly))
	http.Handle("GET /backup/partial/status", app.ProtectFunc(b.getStatusPartial, auth.AdminOnly))
}

// Handle prepares the controller for each request
func (b BackupController) Handle(req *http.Request) application.IController {
	b.Request = req
	return &b
}

// GetSchedulerStatus returns the backup scheduler status for templates
func (b *BackupController) GetSchedulerStatus() backup.SchedulerStatus {
	if backup.Scheduler != nil {
		return backup.Scheduler.GetStatus()
	}
	return backup.SchedulerStatus{}
}

// GetBackupList returns the list of available backups for templates
func (b *BackupController) GetBackupList() []backup.BackupInfo {
	if backup.Scheduler != nil {
		backups, err := backup.Scheduler.ListBackups()
		if err != nil {
			return []backup.BackupInfo{}
		}
		return backups
	}
	return []backup.BackupInfo{}
}

// createBackup creates a manual backup
func (b *BackupController) createBackup(w http.ResponseWriter, r *http.Request) {
	if backup.Scheduler == nil {
		b.RenderError(w, r, errors.New("Backup system not initialized"))
		return
	}

	backupPath, err := backup.Scheduler.TriggerBackup()
	if err != nil {
		b.RenderError(w, r, fmt.Errorf("backup failed: %w", err))
		return
	}

	// Return success message
	b.Render(w, r, "backup-success.html", map[string]any{
		"Message": fmt.Sprintf("Backup created successfully: %s", backupPath),
	})
}

// restoreBackup restores from a backup
func (b *BackupController) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if backup.Scheduler == nil {
		b.RenderError(w, r, errors.New("Backup system not initialized"))
		return
	}

	// Get backup path from form
	backupPath := r.FormValue("backup_path")
	if backupPath == "" {
		b.RenderError(w, r, errors.New("No backup selected"))
		return
	}

	// Perform restore
	if err := backup.Scheduler.RestoreBackup(backupPath); err != nil {
		b.RenderError(w, r, fmt.Errorf("restore failed: %w", err))
		return
	}

	// Return success message
	b.Render(w, r, "backup-success.html", map[string]any{
		"Message": "Backup restored successfully. Please restart the application.",
	})
}

// listBackups returns the list of available backups as JSON
func (b *BackupController) listBackups(w http.ResponseWriter, r *http.Request) {
	if backup.Scheduler == nil {
		http.Error(w, "Backup system not initialized", http.StatusServiceUnavailable)
		return
	}

	backups, err := backup.Scheduler.ListBackups()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backups)
}

// getStatus returns the scheduler status as JSON
func (b *BackupController) getStatus(w http.ResponseWriter, r *http.Request) {
	if backup.Scheduler == nil {
		http.Error(w, "Backup system not initialized", http.StatusServiceUnavailable)
		return
	}

	status := backup.Scheduler.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// toggleScheduler enables or disables the backup scheduler
func (b *BackupController) toggleScheduler(w http.ResponseWriter, r *http.Request) {
	if backup.Scheduler == nil {
		b.RenderError(w, r, errors.New("Backup system not initialized"))
		return
	}

	// Get current status
	status := backup.Scheduler.GetStatus()

	// Toggle
	backup.Scheduler.SetEnabled(!status.Enabled)

	// Return updated status partial
	b.getStatusPartial(w, r)
}

// getBackupListPartial returns the backup list as HTML partial
func (b *BackupController) getBackupListPartial(w http.ResponseWriter, r *http.Request) {
	backups := b.GetBackupList()

	b.Render(w, r, "backup-list.html", map[string]any{
		"Backups": backups,
	})
}

// getStatusPartial returns the scheduler status as HTML partial
func (b *BackupController) getStatusPartial(w http.ResponseWriter, r *http.Request) {
	status := b.GetSchedulerStatus()

	b.Render(w, r, "backup-status.html", map[string]any{
		"Status": status,
	})
}
