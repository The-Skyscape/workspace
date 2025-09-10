package models

import (
	"fmt"
	"log"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/database"
)

// AuditEventType represents the type of audit event
type AuditEventType string

const (
	// Authentication events
	AuditEventLogin           AuditEventType = "auth.login"
	AuditEventLogout          AuditEventType = "auth.logout"
	AuditEventLoginFailed     AuditEventType = "auth.login_failed"
	AuditEventPasswordChanged AuditEventType = "auth.password_changed"
	AuditEventPasswordReset   AuditEventType = "auth.password_reset"
	
	// Repository events
	AuditEventRepoCreated  AuditEventType = "repo.created"
	AuditEventRepoDeleted  AuditEventType = "repo.deleted"
	AuditEventRepoAccessed AuditEventType = "repo.accessed"
	AuditEventRepoModified AuditEventType = "repo.modified"
	AuditEventRepoCloned   AuditEventType = "repo.cloned"
	AuditEventRepoPushed   AuditEventType = "repo.pushed"
	
	// Issue events
	AuditEventIssueCreated AuditEventType = "issue.created"
	AuditEventIssueUpdated AuditEventType = "issue.updated"
	AuditEventIssueClosed  AuditEventType = "issue.closed"
	
	// PR events
	AuditEventPRCreated  AuditEventType = "pr.created"
	AuditEventPRMerged   AuditEventType = "pr.merged"
	AuditEventPRRejected AuditEventType = "pr.rejected"
	
	// Security events
	AuditEventSecurityViolation AuditEventType = "security.violation"
	AuditEventAccessDenied      AuditEventType = "security.access_denied"
	AuditEventTokenCreated      AuditEventType = "security.token_created"
	AuditEventTokenRevoked      AuditEventType = "security.token_revoked"
	
	// Admin events
	AuditEventUserCreated      AuditEventType = "admin.user_created"
	AuditEventUserDeleted      AuditEventType = "admin.user_deleted"
	AuditEventUserModified     AuditEventType = "admin.user_modified"
	AuditEventPermissionGranted AuditEventType = "admin.permission_granted"
	AuditEventPermissionRevoked AuditEventType = "admin.permission_revoked"
)

// AuditSeverity represents the severity level of an audit event
type AuditSeverity int

const (
	AuditSeverityInfo     AuditSeverity = 0
	AuditSeverityWarning  AuditSeverity = 1
	AuditSeverityCritical AuditSeverity = 2
)

// AuditLog represents an audit log entry
type AuditLog struct {
	application.Model
	Timestamp    time.Time      `json:"timestamp"`
	EventType    AuditEventType `json:"event_type"`
	UserID       string         `json:"user_id,omitempty"`
	UserEmail    string         `json:"user_email,omitempty"`
	ResourceType string         `json:"resource_type,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Action       string         `json:"action"`
	Details      string         `json:"details,omitempty"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	Severity     AuditSeverity  `json:"severity"`
	Success      bool           `json:"success"`
}

func (*AuditLog) Table() string { return "audit_logs" }

// AuditLogs is the audit logs collection
var AuditLogs = database.Manage(DB, new(AuditLog))

// LogEvent creates a new audit log entry
func LogAuditEvent(eventType AuditEventType, userID, userEmail, resourceType, resourceID, action, details, ipAddress, userAgent string, success bool) error {
	auditLog := &AuditLog{
		Timestamp:    time.Now(),
		EventType:    eventType,
		UserID:       userID,
		UserEmail:    userEmail,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Details:      details,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Severity:     DetermineSeverity(eventType),
		Success:      success,
	}
	
	_, err := AuditLogs.Insert(auditLog)
	if err != nil {
		log.Printf("Failed to create audit log: %v", err)
		return err
	}
	
	return nil
}

// DetermineSeverity determines the severity based on event type
func DetermineSeverity(eventType AuditEventType) AuditSeverity {
	switch eventType {
	case AuditEventLoginFailed, AuditEventAccessDenied:
		return AuditSeverityWarning
	case AuditEventSecurityViolation, AuditEventTokenRevoked:
		return AuditSeverityCritical
	default:
		return AuditSeverityInfo
	}
}

// AuditFilter represents filter criteria for querying audit logs
type AuditFilter struct {
	UserID       string
	EventType    AuditEventType
	Severity     AuditSeverity
	ResourceID   string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
}

// QueryAuditLogs queries audit logs with filters
func QueryAuditLogs(filter AuditFilter) ([]*AuditLog, error) {
	query := "WHERE 1=1"
	var args []any
	
	if filter.UserID != "" {
		query += " AND UserID = ?"
		args = append(args, filter.UserID)
	}
	
	if filter.EventType != "" {
		query += " AND EventType = ?"
		args = append(args, filter.EventType)
	}
	
	if filter.Severity > 0 {
		query += " AND Severity = ?"
		args = append(args, filter.Severity)
	}
	
	if filter.ResourceID != "" {
		query += " AND ResourceID = ?"
		args = append(args, filter.ResourceID)
	}
	
	if !filter.StartTime.IsZero() {
		query += " AND Timestamp >= ?"
		args = append(args, filter.StartTime)
	}
	
	if !filter.EndTime.IsZero() {
		query += " AND Timestamp <= ?"
		args = append(args, filter.EndTime)
	}
	
	// Add ordering and limit
	query += " ORDER BY Timestamp DESC"
	
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	
	return AuditLogs.Search(query, args...)
}

// GetUserActivity gets audit logs for a specific user
func GetUserActivity(userID string, limit int) ([]*AuditLog, error) {
	return QueryAuditLogs(AuditFilter{
		UserID: userID,
		Limit:  limit,
	})
}

// GetSecurityEvents gets security-related audit logs
func GetSecurityEvents(severity AuditSeverity, limit int) ([]*AuditLog, error) {
	query := `WHERE EventType LIKE 'security.%' 
	          AND Severity >= ? 
	          ORDER BY Timestamp DESC 
	          LIMIT ?`
	
	return AuditLogs.Search(query, severity, limit)
}

// CleanOldAuditLogs removes logs older than the specified duration
func CleanOldAuditLogs(retention time.Duration) error {
	cutoff := time.Now().Add(-retention)
	query := "DELETE FROM audit_logs WHERE Timestamp < ?"
	
	err := DB.Query(query, cutoff).Exec()
	if err != nil {
		log.Printf("Failed to clean old audit logs: %v", err)
		return err
	}
	
	return nil
}