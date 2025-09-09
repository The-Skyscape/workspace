package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupConfig holds backup configuration
type BackupConfig struct {
	// Paths to backup
	DatabasePath   string
	ReposPath      string
	SecretsPath    string
	UploadsPath    string
	
	// Backup destination
	BackupDir      string
	
	// Retention settings
	RetentionDays  int
	MaxBackups     int
	
	// Schedule
	Schedule       string // cron expression
	Enabled        bool
}

// DefaultBackupConfig returns default backup configuration
func DefaultBackupConfig() *BackupConfig {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/root"
	}
	
	return &BackupConfig{
		DatabasePath:  filepath.Join(homeDir, ".skyscape", "workspace.db"),
		ReposPath:     filepath.Join(homeDir, ".skyscape", "repos"),
		SecretsPath:   filepath.Join(homeDir, ".skyscape", "vault"),
		UploadsPath:   filepath.Join(homeDir, ".skyscape", "uploads"),
		BackupDir:     filepath.Join(homeDir, ".skyscape", "backups"),
		RetentionDays: 30,
		MaxBackups:    10,
		Schedule:      "0 2 * * *", // Daily at 2 AM
		Enabled:       true,
	}
}

// BackupManager handles backup operations
type BackupManager struct {
	config *BackupConfig
	stopCh chan struct{}
}

// NewBackupManager creates a new backup manager
func NewBackupManager(config *BackupConfig) *BackupManager {
	if config == nil {
		config = DefaultBackupConfig()
	}
	
	return &BackupManager{
		config: config,
		stopCh: make(chan struct{}),
	}
}

// CreateBackup creates a full backup
func (bm *BackupManager) CreateBackup() (string, error) {
	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(bm.config.BackupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}
	
	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("workspace-backup-%s.tar.gz", timestamp)
	backupPath := filepath.Join(bm.config.BackupDir, backupName)
	
	// Create backup file
	file, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()
	
	// Create gzip writer
	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()
	
	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()
	
	// Backup database
	log.Printf("Backing up database: %s", bm.config.DatabasePath)
	if err := bm.addFileToTar(tarWriter, bm.config.DatabasePath, "database/workspace.db"); err != nil {
		log.Printf("Warning: Failed to backup database: %v", err)
	}
	
	// Backup repositories
	log.Printf("Backing up repositories: %s", bm.config.ReposPath)
	if err := bm.addDirectoryToTar(tarWriter, bm.config.ReposPath, "repos"); err != nil {
		log.Printf("Warning: Failed to backup repositories: %v", err)
	}
	
	// Backup secrets (vault)
	log.Printf("Backing up secrets: %s", bm.config.SecretsPath)
	if err := bm.addDirectoryToTar(tarWriter, bm.config.SecretsPath, "vault"); err != nil {
		log.Printf("Warning: Failed to backup secrets: %v", err)
	}
	
	// Backup uploads
	log.Printf("Backing up uploads: %s", bm.config.UploadsPath)
	if err := bm.addDirectoryToTar(tarWriter, bm.config.UploadsPath, "uploads"); err != nil {
		log.Printf("Warning: Failed to backup uploads: %v", err)
	}
	
	// Add backup metadata
	metadata := fmt.Sprintf(`Backup created: %s
Version: 1.0
Type: Full Backup
`, time.Now().Format(time.RFC3339))
	
	if err := bm.addStringToTar(tarWriter, metadata, "backup.info"); err != nil {
		log.Printf("Warning: Failed to add metadata: %v", err)
	}
	
	log.Printf("Backup created successfully: %s", backupPath)
	
	// Clean old backups
	if err := bm.cleanOldBackups(); err != nil {
		log.Printf("Warning: Failed to clean old backups: %v", err)
	}
	
	return backupPath, nil
}

// addFileToTar adds a single file to the tar archive
func (bm *BackupManager) addFileToTar(tw *tar.Writer, sourcePath, targetPath string) error {
	// Check if file exists
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Skip if doesn't exist
		}
		return err
	}
	
	// Open source file
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Create tar header
	header := &tar.Header{
		Name:    targetPath,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}
	
	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	
	// Copy file content
	_, err = io.Copy(tw, file)
	return err
}

// addDirectoryToTar recursively adds a directory to the tar archive
func (bm *BackupManager) addDirectoryToTar(tw *tar.Writer, sourcePath, targetPath string) error {
	// Check if directory exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil // Skip if doesn't exist
	}
	
	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Create relative path
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		
		// Create target path
		tarPath := filepath.Join(targetPath, relPath)
		
		// Create header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = tarPath
		
		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		
		// If it's a file, write content
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			
			_, err = io.Copy(tw, file)
			return err
		}
		
		return nil
	})
}

// addStringToTar adds a string as a file to the tar archive
func (bm *BackupManager) addStringToTar(tw *tar.Writer, content, filename string) error {
	header := &tar.Header{
		Name:    filename,
		Size:    int64(len(content)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	
	_, err := tw.Write([]byte(content))
	return err
}

// RestoreBackup restores from a backup file
func (bm *BackupManager) RestoreBackup(backupPath string) error {
	// Open backup file
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()
	
	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()
	
	// Create tar reader
	tarReader := tar.NewReader(gzReader)
	
	// Create restore directory
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/root"
	}
	restoreDir := filepath.Join(homeDir, ".skyscape")
	
	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}
		
		// Determine target path
		var targetPath string
		if strings.HasPrefix(header.Name, "database/") {
			targetPath = filepath.Join(restoreDir, strings.TrimPrefix(header.Name, "database/"))
		} else if strings.HasPrefix(header.Name, "repos/") {
			targetPath = filepath.Join(restoreDir, header.Name)
		} else if strings.HasPrefix(header.Name, "vault/") {
			targetPath = filepath.Join(restoreDir, header.Name)
		} else if strings.HasPrefix(header.Name, "uploads/") {
			targetPath = filepath.Join(restoreDir, header.Name)
		} else {
			// Skip metadata files
			continue
		}
		
		// Create directory if needed
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}
			continue
		}
		
		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}
		
		// Create file
		outFile, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", targetPath, err)
		}
		
		// Copy content
		if _, err := io.Copy(outFile, tarReader); err != nil {
			outFile.Close()
			return fmt.Errorf("failed to extract file %s: %w", targetPath, err)
		}
		
		outFile.Close()
		
		// Set permissions
		if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
			log.Printf("Warning: Failed to set permissions for %s: %v", targetPath, err)
		}
		
		log.Printf("Restored: %s", targetPath)
	}
	
	log.Printf("Backup restored successfully from: %s", backupPath)
	return nil
}

// ListBackups returns a list of available backups
func (bm *BackupManager) ListBackups() ([]BackupInfo, error) {
	files, err := os.ReadDir(bm.config.BackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}
	
	var backups []BackupInfo
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".tar.gz") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			
			backups = append(backups, BackupInfo{
				Name:     file.Name(),
				Path:     filepath.Join(bm.config.BackupDir, file.Name()),
				Size:     info.Size(),
				Created:  info.ModTime(),
			})
		}
	}
	
	return backups, nil
}

// cleanOldBackups removes backups older than retention period
func (bm *BackupManager) cleanOldBackups() error {
	backups, err := bm.ListBackups()
	if err != nil {
		return err
	}
	
	// Remove backups older than retention period
	cutoff := time.Now().Add(-time.Duration(bm.config.RetentionDays) * 24 * time.Hour)
	for _, backup := range backups {
		if backup.Created.Before(cutoff) {
			log.Printf("Removing old backup: %s", backup.Name)
			if err := os.Remove(backup.Path); err != nil {
				log.Printf("Warning: Failed to remove old backup %s: %v", backup.Name, err)
			}
		}
	}
	
	// Keep only MaxBackups most recent
	if len(backups) > bm.config.MaxBackups {
		// Sort by creation time (newest first)
		// Simple bubble sort for small lists
		for i := 0; i < len(backups)-1; i++ {
			for j := i + 1; j < len(backups); j++ {
				if backups[i].Created.Before(backups[j].Created) {
					backups[i], backups[j] = backups[j], backups[i]
				}
			}
		}
		
		// Remove excess backups
		for i := bm.config.MaxBackups; i < len(backups); i++ {
			log.Printf("Removing excess backup: %s", backups[i].Name)
			if err := os.Remove(backups[i].Path); err != nil {
				log.Printf("Warning: Failed to remove excess backup %s: %v", backups[i].Name, err)
			}
		}
	}
	
	return nil
}

// BackupInfo holds information about a backup
type BackupInfo struct {
	Name    string
	Path    string
	Size    int64
	Created time.Time
}

// FormatSize formats bytes as human-readable string
func (bi BackupInfo) FormatSize() string {
	const unit = 1024
	if bi.Size < unit {
		return fmt.Sprintf("%d B", bi.Size)
	}
	div, exp := int64(unit), 0
	for n := bi.Size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bi.Size)/float64(div), "KMGTPE"[exp])
}