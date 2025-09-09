package backup

import (
	"log"
	"sync"
	"time"
)

// BackupScheduler handles automated backup scheduling
type BackupScheduler struct {
	manager  *BackupManager
	schedule string
	enabled  bool
	mu       sync.RWMutex
	stopCh   chan struct{}
	wg       sync.WaitGroup
	lastRun  time.Time
	nextRun  time.Time
}

// NewBackupScheduler creates a new backup scheduler
func NewBackupScheduler(config *BackupConfig) *BackupScheduler {
	return &BackupScheduler{
		manager:  NewBackupManager(config),
		schedule: config.Schedule,
		enabled:  config.Enabled,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the backup scheduler
func (bs *BackupScheduler) Start() {
	bs.mu.Lock()
	if !bs.enabled {
		bs.mu.Unlock()
		log.Println("Backup scheduler is disabled")
		return
	}
	bs.mu.Unlock()
	
	log.Println("Starting backup scheduler")
	
	// Calculate next run time (default: daily at 2 AM)
	bs.calculateNextRun()
	
	bs.wg.Add(1)
	go bs.run()
}

// Stop stops the backup scheduler
func (bs *BackupScheduler) Stop() {
	log.Println("Stopping backup scheduler")
	close(bs.stopCh)
	bs.wg.Wait()
}

// run is the main scheduler loop
func (bs *BackupScheduler) run() {
	defer bs.wg.Done()
	
	// Run initial backup if never run before
	if bs.lastRun.IsZero() {
		log.Println("Running initial backup")
		bs.runBackup()
	}
	
	// Create ticker for checking schedule
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-bs.stopCh:
			return
		case <-ticker.C:
			bs.mu.RLock()
			enabled := bs.enabled
			nextRun := bs.nextRun
			bs.mu.RUnlock()
			
			if enabled && time.Now().After(nextRun) {
				bs.runBackup()
				bs.calculateNextRun()
			}
		}
	}
}

// runBackup executes a backup
func (bs *BackupScheduler) runBackup() {
	bs.mu.Lock()
	bs.lastRun = time.Now()
	bs.mu.Unlock()
	
	log.Println("Starting scheduled backup")
	
	backupPath, err := bs.manager.CreateBackup()
	if err != nil {
		log.Printf("Scheduled backup failed: %v", err)
		return
	}
	
	log.Printf("Scheduled backup completed: %s", backupPath)
}

// calculateNextRun calculates the next backup time
func (bs *BackupScheduler) calculateNextRun() {
	now := time.Now()
	
	// Parse schedule (simplified - assumes daily at specific hour)
	// Format: "0 2 * * *" means 2:00 AM daily
	// For production, use a proper cron parser
	
	// Default to 2 AM tomorrow
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, now.Location())
	
	// If it's before 2 AM today, run today
	todayRun := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	if now.Before(todayRun) {
		next = todayRun
	}
	
	bs.mu.Lock()
	bs.nextRun = next
	bs.mu.Unlock()
	
	log.Printf("Next backup scheduled for: %s", next.Format(time.RFC3339))
}

// TriggerBackup manually triggers a backup
func (bs *BackupScheduler) TriggerBackup() (string, error) {
	log.Println("Manual backup triggered")
	return bs.manager.CreateBackup()
}

// RestoreBackup restores from a backup
func (bs *BackupScheduler) RestoreBackup(backupPath string) error {
	return bs.manager.RestoreBackup(backupPath)
}

// ListBackups returns available backups
func (bs *BackupScheduler) ListBackups() ([]BackupInfo, error) {
	return bs.manager.ListBackups()
}

// GetStatus returns the scheduler status
func (bs *BackupScheduler) GetStatus() SchedulerStatus {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	
	return SchedulerStatus{
		Enabled:  bs.enabled,
		LastRun:  bs.lastRun,
		NextRun:  bs.nextRun,
		Schedule: bs.schedule,
	}
}

// SetEnabled enables or disables the scheduler
func (bs *BackupScheduler) SetEnabled(enabled bool) {
	bs.mu.Lock()
	bs.enabled = enabled
	bs.mu.Unlock()
	
	if enabled {
		log.Println("Backup scheduler enabled")
		bs.calculateNextRun()
	} else {
		log.Println("Backup scheduler disabled")
	}
}

// SchedulerStatus holds scheduler status information
type SchedulerStatus struct {
	Enabled  bool
	LastRun  time.Time
	NextRun  time.Time
	Schedule string
}

// Global backup scheduler instance
var Scheduler *BackupScheduler

// InitializeBackupScheduler initializes the global backup scheduler
func InitializeBackupScheduler() {
	config := DefaultBackupConfig()
	Scheduler = NewBackupScheduler(config)
	Scheduler.Start()
	log.Println("Backup scheduler initialized")
}