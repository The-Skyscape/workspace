package models

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"golang.org/x/crypto/ssh"
)

// SSHKey represents an SSH public key for Git authentication
type SSHKey struct {
	application.Model
	UserID      string    // User who owns this key
	Name        string    // User-friendly name for the key
	PublicKey   string    // Full public key in OpenSSH format
	Fingerprint string    // SHA256 fingerprint for identification
	LastUsedAt  time.Time // Last time this key was used for authentication
}

func (*SSHKey) Table() string { return "ssh_keys" }

func init() {
	// Create indexes for ssh_keys table
	go func() {
		SSHKeys.Index("UserID")
		SSHKeys.Index("Fingerprint")
		SSHKeys.Index("LastUsedAt")
	}()
}

// ParseSSHKey validates and parses an SSH public key
func ParseSSHKey(keyData string) (*ssh.PublicKey, error) {
	// Trim whitespace and ensure proper formatting
	keyData = strings.TrimSpace(keyData)
	
	// Parse the public key
	publicKey, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(keyData))
	if err != nil {
		return nil, fmt.Errorf("invalid SSH key format: %v", err)
	}
	
	// Store comment if provided (often contains email or identifier)
	_ = comment // Can be used for default name if needed
	
	return &publicKey, nil
}

// GenerateFingerprint creates a SHA256 fingerprint for an SSH key
func GenerateFingerprint(publicKey ssh.PublicKey) string {
	hash := sha256.Sum256(publicKey.Marshal())
	fingerprint := base64.StdEncoding.EncodeToString(hash[:])
	// Format as SHA256:base64 (GitHub/GitLab style)
	return fmt.Sprintf("SHA256:%s", strings.TrimRight(fingerprint, "="))
}

// CreateSSHKey validates and creates a new SSH key
func CreateSSHKey(userID, name, keyData string) (*SSHKey, error) {
	// Parse and validate the key
	publicKey, err := ParseSSHKey(keyData)
	if err != nil {
		return nil, err
	}
	
	// Generate fingerprint
	fingerprint := GenerateFingerprint(*publicKey)
	
	// Check for duplicate keys
	existing, err := SSHKeys.Search("WHERE Fingerprint = ?", fingerprint)
	if err == nil && len(existing) > 0 {
		return nil, fmt.Errorf("this SSH key is already registered")
	}
	
	// Auto-generate name if not provided
	if name == "" {
		// Extract comment from key if available
		parts := strings.Fields(keyData)
		if len(parts) >= 3 {
			name = parts[2] // Often contains email or identifier
		} else {
			name = fmt.Sprintf("SSH Key %s", time.Now().Format("2006-01-02"))
		}
	}
	
	// Create the SSH key record
	sshKey := &SSHKey{
		Model:       DB.NewModel(""),
		UserID:      userID,
		Name:        name,
		PublicKey:   strings.TrimSpace(keyData),
		Fingerprint: fingerprint,
		LastUsedAt:  time.Time{}, // Never used yet
	}
	
	return SSHKeys.Insert(sshKey)
}

// GetUserSSHKeys returns all SSH keys for a user
func GetUserSSHKeys(userID string) ([]*SSHKey, error) {
	return SSHKeys.Search("WHERE UserID = ? ORDER BY CreatedAt DESC", userID)
}

// ValidateSSHKey checks if a public key matches any registered keys
func ValidateSSHKey(publicKey ssh.PublicKey) (*SSHKey, error) {
	fingerprint := GenerateFingerprint(publicKey)
	
	keys, err := SSHKeys.Search("WHERE Fingerprint = ?", fingerprint)
	if err != nil || len(keys) == 0 {
		return nil, fmt.Errorf("SSH key not found")
	}
	
	key := keys[0]
	
	// Update last used timestamp
	key.LastUsedAt = time.Now()
	if err := SSHKeys.Update(key); err != nil {
		// Log error but don't fail authentication
		fmt.Printf("Failed to update SSH key last used: %v\n", err)
	}
	
	return key, nil
}

// GetSSHKeyByFingerprint retrieves a key by its fingerprint
func GetSSHKeyByFingerprint(fingerprint string) (*SSHKey, error) {
	keys, err := SSHKeys.Search("WHERE Fingerprint = ?", fingerprint)
	if err != nil || len(keys) == 0 {
		return nil, fmt.Errorf("SSH key not found")
	}
	return keys[0], nil
}

// DeleteSSHKey removes an SSH key
func DeleteSSHKey(keyID, userID string) error {
	key, err := SSHKeys.Get(keyID)
	if err != nil {
		return err
	}
	
	// Verify ownership
	if key.UserID != userID {
		return fmt.Errorf("unauthorized")
	}
	
	return SSHKeys.Delete(key)
}

// CountUserSSHKeys returns the number of SSH keys for a user
func CountUserSSHKeys(userID string) (int, error) {
	keys, err := SSHKeys.Search("WHERE UserID = ?", userID)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}