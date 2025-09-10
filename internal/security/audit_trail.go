package security

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
	"workspace/models"
)

// AuditTrail manages audit logging with buffering and batch writes
type AuditTrail struct {
	mu          sync.RWMutex
	buffer      []*models.AuditLog
	bufferSize  int
	flushTicker *time.Ticker
	stopCh      chan struct{}
	retention   time.Duration
}

// NewAuditTrail creates a new audit trail manager
func NewAuditTrail() *AuditTrail {
	at := &AuditTrail{
		bufferSize: 100,
		buffer:     make([]*models.AuditLog, 0, 100),
		retention:  30 * 24 * time.Hour, // 30 days default
		stopCh:     make(chan struct{}),
	}

	// Start background flush routine
	at.flushTicker = time.NewTicker(5 * time.Second)
	go at.backgroundFlush()

	// Start cleanup routine
	go at.backgroundCleanup()

	return at
}

// LogEvent logs an audit event
func (at *AuditTrail) LogEvent(ctx context.Context, event models.AuditEventType, userID, userEmail, resourceType, resourceID, action, details string, success bool) {
	// Extract request info from context if available
	var ipAddress, userAgent string
	if req, ok := ctx.Value("request").(*http.Request); ok {
		ipAddress = getClientIP(req)
		userAgent = req.UserAgent()
	}

	at.mu.Lock()
	defer at.mu.Unlock()

	log := &models.AuditLog{
		Timestamp:    time.Now(),
		EventType:    event,
		UserID:       userID,
		UserEmail:    userEmail,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Details:      details,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Severity:     models.DetermineSeverity(event),
		Success:      success,
	}

	at.buffer = append(at.buffer, log)

	// Flush if buffer is full
	if len(at.buffer) >= at.bufferSize {
		at.flush()
	}
}

// flush writes buffered logs to database
func (at *AuditTrail) flush() {
	if len(at.buffer) == 0 {
		return
	}

	// Make a copy of buffer
	logs := make([]*models.AuditLog, len(at.buffer))
	copy(logs, at.buffer)
	at.buffer = at.buffer[:0]

	// Write logs to database
	for _, auditLog := range logs {
		if _, err := models.AuditLogs.Insert(auditLog); err != nil {
			log.Printf("Failed to write audit log: %v", err)
		}
	}
}

// Flush forces immediate write of buffered logs
func (at *AuditTrail) Flush() {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.flush()
}

// backgroundFlush periodically flushes the buffer
func (at *AuditTrail) backgroundFlush() {
	for {
		select {
		case <-at.flushTicker.C:
			at.Flush()
		case <-at.stopCh:
			return
		}
	}
}

// backgroundCleanup periodically removes old logs
func (at *AuditTrail) backgroundCleanup() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := models.CleanOldAuditLogs(at.retention); err != nil {
				log.Printf("Failed to clean old audit logs: %v", err)
			}
		case <-at.stopCh:
			return
		}
	}
}

// Stop stops the background routines
func (at *AuditTrail) Stop() {
	at.Flush()
	close(at.stopCh)
	at.flushTicker.Stop()
}

// SetRetention sets the retention period for audit logs
func (at *AuditTrail) SetRetention(retention time.Duration) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.retention = retention
}

// Query queries audit logs with filters - delegates to models
func (at *AuditTrail) Query(filter models.AuditFilter) ([]*models.AuditLog, error) {
	return models.QueryAuditLogs(filter)
}

// GetUserActivity gets audit logs for a specific user - delegates to models
func (at *AuditTrail) GetUserActivity(userID string, limit int) ([]*models.AuditLog, error) {
	return models.GetUserActivity(userID, limit)
}

// GetSecurityEvents gets security-related audit logs - delegates to models
func (at *AuditTrail) GetSecurityEvents(severity models.AuditSeverity, limit int) ([]*models.AuditLog, error) {
	return models.GetSecurityEvents(severity, limit)
}

// ComplianceReport represents a compliance report
type ComplianceReport struct {
	StartTime        time.Time
	EndTime          time.Time
	TotalEvents      int
	SecurityEvents   int
	FailedLogins     int
	SuccessfulLogins int
	DataAccess       int
	DataModification int
	UsersByActivity  map[string]int
	EventsByType     map[string]int
	EventsBySeverity map[string]int
}

// GenerateComplianceReport generates a compliance report
func (at *AuditTrail) GenerateComplianceReport(startTime, endTime time.Time) (*ComplianceReport, error) {
	logs, err := models.QueryAuditLogs(models.AuditFilter{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}

	report := &ComplianceReport{
		StartTime:        startTime,
		EndTime:          endTime,
		TotalEvents:      len(logs),
		UsersByActivity:  make(map[string]int),
		EventsByType:     make(map[string]int),
		EventsBySeverity: make(map[string]int),
	}

	// Analyze logs
	for _, log := range logs {
		// Count by event type
		report.EventsByType[string(log.EventType)]++

		// Count by severity
		severityStr := fmt.Sprintf("%d", log.Severity)
		report.EventsBySeverity[severityStr]++

		// Count by user
		if log.UserEmail != "" {
			report.UsersByActivity[log.UserEmail]++
		}

		// Count specific events
		switch log.EventType {
		case models.AuditEventLogin:
			if log.Success {
				report.SuccessfulLogins++
			}
		case models.AuditEventLoginFailed:
			report.FailedLogins++
		case models.AuditEventSecurityViolation, models.AuditEventAccessDenied:
			report.SecurityEvents++
		case models.AuditEventRepoAccessed:
			report.DataAccess++
		case models.AuditEventRepoModified, models.AuditEventRepoPushed:
			report.DataModification++
		}
	}

	return report, nil
}

// Global audit trail instance
var Trail *AuditTrail

// InitializeAuditTrail initializes the global audit trail
func InitializeAuditTrail() {
	Trail = NewAuditTrail()
	log.Println("Audit trail initialized")
}

// Helper functions for common audit logging
func LogLogin(ctx context.Context, userID, userEmail string, success bool) {
	if Trail != nil {
		eventType := models.AuditEventLogin
		if !success {
			eventType = models.AuditEventLoginFailed
		}
		Trail.LogEvent(ctx, eventType, userID, userEmail, "auth", "", "login", "", success)
	}
}

func LogRepoAccess(ctx context.Context, userID, userEmail, repoID, action string) {
	if Trail != nil {
		Trail.LogEvent(ctx, models.AuditEventRepoAccessed, userID, userEmail, "repository", repoID, action, "", true)
	}
}

func LogSecurityEvent(ctx context.Context, userID, userEmail, details string) {
	if Trail != nil {
		Trail.LogEvent(ctx, models.AuditEventSecurityViolation, userID, userEmail, "security", "", "violation", details, false)
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}
