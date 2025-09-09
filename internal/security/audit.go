package security

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
	"workspace/models"
	
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/application"
)

// AuditEventType represents the type of audit event
type AuditEventType string

const (
	// Authentication events
	AuditLogin           AuditEventType = "auth.login"
	AuditLogout          AuditEventType = "auth.logout"
	AuditLoginFailed     AuditEventType = "auth.login_failed"
	AuditPasswordChange  AuditEventType = "auth.password_change"
	AuditPasswordReset   AuditEventType = "auth.password_reset"
	
	// Repository events
	AuditRepoCreate      AuditEventType = "repo.create"
	AuditRepoDelete      AuditEventType = "repo.delete"
	AuditRepoUpdate      AuditEventType = "repo.update"
	AuditRepoAccess      AuditEventType = "repo.access"
	AuditRepoClone       AuditEventType = "repo.clone"
	AuditRepoPush        AuditEventType = "repo.push"
	
	// File events
	AuditFileCreate      AuditEventType = "file.create"
	AuditFileUpdate      AuditEventType = "file.update"
	AuditFileDelete      AuditEventType = "file.delete"
	AuditFileRead        AuditEventType = "file.read"
	
	// Security events
	AuditSecurityScan    AuditEventType = "security.scan"
	AuditSecurityAlert   AuditEventType = "security.alert"
	AuditAccessDenied    AuditEventType = "security.access_denied"
	AuditSuspiciousActivity AuditEventType = "security.suspicious"
	
	// AI events
	AuditAIQuery         AuditEventType = "ai.query"
	AuditAIToolUse       AuditEventType = "ai.tool_use"
	AuditAIAutoAction    AuditEventType = "ai.auto_action"
	
	// Admin events
	AuditUserCreate      AuditEventType = "admin.user_create"
	AuditUserDelete      AuditEventType = "admin.user_delete"
	AuditUserUpdate      AuditEventType = "admin.user_update"
	AuditSettingsChange  AuditEventType = "admin.settings_change"
	AuditBackup          AuditEventType = "admin.backup"
	AuditRestore         AuditEventType = "admin.restore"
)

// AuditSeverity represents the severity level of an audit event
type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "info"
	SeverityWarning  AuditSeverity = "warning"
	SeverityError    AuditSeverity = "error"
	SeverityCritical AuditSeverity = "critical"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	application.Model
	EventType   AuditEventType         `json:"event_type"`
	Severity    AuditSeverity          `json:"severity"`
	UserID      string                 `json:"user_id"`
	Username    string                 `json:"username"`
	IPAddress   string                 `json:"ip_address"`
	UserAgent   string                 `json:"user_agent"`
	ResourceID  string                 `json:"resource_id,omitempty"`
	ResourceType string                `json:"resource_type,omitempty"`
	Action      string                 `json:"action"`
	Result      string                 `json:"result"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

func (*AuditLog) Table() string { return "audit_logs" }

// AuditTrail manages audit logging
type AuditTrail struct {
	db          *database.DB
	logs        *database.Repository
	mu          sync.RWMutex
	buffer      []AuditLog
	bufferSize  int
	flushTicker *time.Ticker
	stopCh      chan struct{}
	retention   time.Duration
}

// NewAuditTrail creates a new audit trail manager
func NewAuditTrail(db *database.DB) *AuditTrail {
	at := &AuditTrail{
		db:         db,
		logs:       database.Manage(db, new(AuditLog)),
		bufferSize: 100,
		buffer:     make([]AuditLog, 0, 100),
		stopCh:     make(chan struct{}),
		retention:  90 * 24 * time.Hour, // 90 days retention
	}
	
	// Start background flusher
	at.startFlusher()
	
	// Start retention cleaner
	at.startRetentionCleaner()
	
	return at
}

// Log creates a new audit log entry
func (at *AuditTrail) Log(ctx context.Context, event AuditLog) error {
	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	// Determine severity if not set
	if event.Severity == "" {
		event.Severity = at.determineSeverity(event.EventType)
	}
	
	// Add to buffer
	at.mu.Lock()
	at.buffer = append(at.buffer, event)
	shouldFlush := len(at.buffer) >= at.bufferSize
	at.mu.Unlock()
	
	// Flush if buffer is full
	if shouldFlush {
		go at.flush()
	}
	
	// Log critical events immediately
	if event.Severity == SeverityCritical {
		log.Printf("AUDIT [%s]: %s - User: %s, Resource: %s, Result: %s",
			event.Severity, event.EventType, event.UserID, event.ResourceID, event.Result)
		go at.flush()
	}
	
	return nil
}

// LogSimple creates a simple audit log entry
func (at *AuditTrail) LogSimple(eventType AuditEventType, userID, action, result string) {
	at.Log(context.Background(), AuditLog{
		EventType: eventType,
		UserID:    userID,
		Action:    action,
		Result:    result,
		Timestamp: time.Now(),
	})
}

// LogWithDetails creates an audit log with additional details
func (at *AuditTrail) LogWithDetails(eventType AuditEventType, userID, action, result string, details map[string]interface{}) {
	at.Log(context.Background(), AuditLog{
		EventType: eventType,
		UserID:    userID,
		Action:    action,
		Result:    result,
		Details:   details,
		Timestamp: time.Now(),
	})
}

// LogSecurityEvent logs a security-related event
func (at *AuditTrail) LogSecurityEvent(eventType AuditEventType, severity AuditSeverity, userID, action string, details map[string]interface{}) {
	at.Log(context.Background(), AuditLog{
		EventType: eventType,
		Severity:  severity,
		UserID:    userID,
		Action:    action,
		Result:    "security_event",
		Details:   details,
		Timestamp: time.Now(),
	})
}

// flush writes buffered logs to database
func (at *AuditTrail) flush() {
	at.mu.Lock()
	if len(at.buffer) == 0 {
		at.mu.Unlock()
		return
	}
	
	logs := make([]AuditLog, len(at.buffer))
	copy(logs, at.buffer)
	at.buffer = at.buffer[:0]
	at.mu.Unlock()
	
	// Write to database
	for _, log := range logs {
		if _, err := at.logs.Insert(&log); err != nil {
			// Log error but don't fail - audit logs should not break the app
			fmt.Printf("Failed to write audit log: %v\n", err)
		}
	}
}

// startFlusher starts the background flusher
func (at *AuditTrail) startFlusher() {
	at.flushTicker = time.NewTicker(30 * time.Second)
	
	go func() {
		for {
			select {
			case <-at.flushTicker.C:
				at.flush()
			case <-at.stopCh:
				at.flush() // Final flush
				return
			}
		}
	}()
}

// startRetentionCleaner starts the retention cleaner
func (at *AuditTrail) startRetentionCleaner() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				at.cleanOldLogs()
			case <-at.stopCh:
				return
			}
		}
	}()
}

// cleanOldLogs removes logs older than retention period
func (at *AuditTrail) cleanOldLogs() {
	cutoff := time.Now().Add(-at.retention)
	
	query := "DELETE FROM audit_logs WHERE Timestamp < ?"
	result := at.db.Exec(query, cutoff)
	if result.Error != nil {
		log.Printf("Failed to clean old audit logs: %v", result.Error)
		return
	}
	
	if result.RowsAffected > 0 {
		log.Printf("Cleaned %d old audit logs", result.RowsAffected)
	}
}

// determineSeverity determines the severity based on event type
func (at *AuditTrail) determineSeverity(eventType AuditEventType) AuditSeverity {
	switch eventType {
	case AuditLoginFailed, AuditAccessDenied:
		return SeverityWarning
	case AuditSuspiciousActivity, AuditSecurityAlert:
		return SeverityCritical
	case AuditRepoDelete, AuditUserDelete:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// Query searches audit logs
func (at *AuditTrail) Query(filter AuditFilter) ([]AuditLog, error) {
	query := "SELECT * FROM audit_logs WHERE 1=1"
	args := []interface{}{}
	
	if filter.EventType != "" {
		query += " AND EventType = ?"
		args = append(args, filter.EventType)
	}
	
	if filter.UserID != "" {
		query += " AND UserID = ?"
		args = append(args, filter.UserID)
	}
	
	if filter.Severity != "" {
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
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	
	var logs []AuditLog
	result := at.db.Raw(query, args...).Scan(&logs)
	if result.Error != nil {
		return nil, result.Error
	}
	
	return logs, nil
}

// GetUserActivity gets audit logs for a specific user
func (at *AuditTrail) GetUserActivity(userID string, limit int) ([]AuditLog, error) {
	return at.Query(AuditFilter{
		UserID: userID,
		Limit:  limit,
	})
}

// GetSecurityEvents gets security-related audit logs
func (at *AuditTrail) GetSecurityEvents(severity AuditSeverity, limit int) ([]AuditLog, error) {
	query := `SELECT * FROM audit_logs 
	          WHERE EventType LIKE 'security.%' 
	          AND Severity >= ? 
	          ORDER BY Timestamp DESC 
	          LIMIT ?`
	
	var logs []AuditLog
	result := at.db.Raw(query, severity, limit).Scan(&logs)
	if result.Error != nil {
		return nil, result.Error
	}
	
	return logs, nil
}

// GenerateComplianceReport generates a compliance report
func (at *AuditTrail) GenerateComplianceReport(startTime, endTime time.Time) (*ComplianceReport, error) {
	logs, err := at.Query(AuditFilter{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		return nil, err
	}
	
	report := &ComplianceReport{
		StartTime:    startTime,
		EndTime:      endTime,
		TotalEvents:  len(logs),
		GeneratedAt:  time.Now(),
		EventsByType: make(map[AuditEventType]int),
		EventsBySeverity: make(map[AuditSeverity]int),
		UserActivity: make(map[string]int),
		SecurityEvents: []AuditLog{},
	}
	
	// Analyze logs
	for _, log := range logs {
		report.EventsByType[log.EventType]++
		report.EventsBySeverity[log.Severity]++
		report.UserActivity[log.UserID]++
		
		if strings.HasPrefix(string(log.EventType), "security.") {
			report.SecurityEvents = append(report.SecurityEvents, log)
		}
	}
	
	// Calculate compliance metrics
	report.ComplianceScore = at.calculateComplianceScore(report)
	
	return report, nil
}

// calculateComplianceScore calculates a compliance score based on audit data
func (at *AuditTrail) calculateComplianceScore(report *ComplianceReport) float64 {
	score := 100.0
	
	// Deduct points for security events
	criticalEvents := report.EventsBySeverity[SeverityCritical]
	score -= float64(criticalEvents) * 10
	
	warningEvents := report.EventsBySeverity[SeverityWarning]
	score -= float64(warningEvents) * 2
	
	// Bonus points for regular security scans
	if scans, ok := report.EventsByType[AuditSecurityScan]; ok && scans > 0 {
		score += 5
	}
	
	// Ensure score is between 0 and 100
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}
	
	return score
}

// ExportLogs exports audit logs to JSON
func (at *AuditTrail) ExportLogs(filter AuditFilter) ([]byte, error) {
	logs, err := at.Query(filter)
	if err != nil {
		return nil, err
	}
	
	return json.MarshalIndent(logs, "", "  ")
}

// Stop stops the audit trail manager
func (at *AuditTrail) Stop() {
	close(at.stopCh)
	at.flushTicker.Stop()
	at.flush() // Final flush
}

// AuditFilter represents filters for querying audit logs
type AuditFilter struct {
	EventType   AuditEventType
	Severity    AuditSeverity
	UserID      string
	ResourceID  string
	StartTime   time.Time
	EndTime     time.Time
	Limit       int
}

// ComplianceReport represents a compliance report
type ComplianceReport struct {
	StartTime        time.Time
	EndTime          time.Time
	GeneratedAt      time.Time
	TotalEvents      int
	EventsByType     map[AuditEventType]int
	EventsBySeverity map[AuditSeverity]int
	UserActivity     map[string]int
	SecurityEvents   []AuditLog
	ComplianceScore  float64
}

// Global audit trail instance
var Trail *AuditTrail

// InitializeAuditTrail initializes the global audit trail
func InitializeAuditTrail(db *database.DB) {
	Trail = NewAuditTrail(db)
	log.Println("Audit trail initialized")
}

// Helper functions for common audit logging

// LogLogin logs a successful login
func LogLogin(userID, username, ipAddress string) {
	if Trail != nil {
		Trail.Log(context.Background(), AuditLog{
			EventType: AuditLogin,
			Severity:  SeverityInfo,
			UserID:    userID,
			Username:  username,
			IPAddress: ipAddress,
			Action:    "User logged in",
			Result:    "success",
			Timestamp: time.Now(),
		})
	}
}

// LogLoginFailed logs a failed login attempt
func LogLoginFailed(username, ipAddress, reason string) {
	if Trail != nil {
		Trail.Log(context.Background(), AuditLog{
			EventType: AuditLoginFailed,
			Severity:  SeverityWarning,
			Username:  username,
			IPAddress: ipAddress,
			Action:    "Login attempt",
			Result:    "failed",
			Details: map[string]interface{}{
				"reason": reason,
			},
			Timestamp: time.Now(),
		})
	}
}

// LogRepoAccess logs repository access
func LogRepoAccess(userID, repoID, action string) {
	if Trail != nil {
		Trail.Log(context.Background(), AuditLog{
			EventType:    AuditRepoAccess,
			Severity:     SeverityInfo,
			UserID:       userID,
			ResourceID:   repoID,
			ResourceType: "repository",
			Action:       action,
			Result:       "success",
			Timestamp:    time.Now(),
		})
	}
}

// LogSecurityAlert logs a security alert
func LogSecurityAlert(userID, alertType, description string, details map[string]interface{}) {
	if Trail != nil {
		Trail.Log(context.Background(), AuditLog{
			EventType: AuditSecurityAlert,
			Severity:  SeverityCritical,
			UserID:    userID,
			Action:    alertType,
			Result:    "alert",
			Details:   details,
			Metadata: map[string]interface{}{
				"description": description,
			},
			Timestamp: time.Now(),
		})
	}
}