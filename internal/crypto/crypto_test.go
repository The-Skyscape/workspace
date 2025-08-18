package crypto

import (
	"os"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// Set up test environment
	os.Setenv("AUTH_SECRET", "test-secret-key-for-encryption")
	
	testCases := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"simple text", "hello world"},
		{"GitHub token", "ghp_abcdef123456789"},
		{"special chars", "test!@#$%^&*()_+-=[]{}|;:,.<>?"},
		{"long text", "This is a much longer text that should still be encrypted and decrypted correctly without any issues"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}
			
			// Encrypted should be different from plaintext (unless empty)
			if tc.plaintext != "" && encrypted == tc.plaintext {
				t.Error("Encrypted text is same as plaintext")
			}
			
			// Decrypt
			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}
			
			// Verify match
			if decrypted != tc.plaintext {
				t.Errorf("Decrypted text doesn't match: got %q, want %q", decrypted, tc.plaintext)
			}
		})
	}
}

func TestEncryptDecryptField(t *testing.T) {
	os.Setenv("AUTH_SECRET", "test-secret-key-for-encryption")
	
	// Test EncryptField
	token := "ghp_testtoken123"
	encrypted := EncryptField(token)
	if encrypted == token {
		t.Error("EncryptField should encrypt the token")
	}
	
	// Test DecryptField
	decrypted := DecryptField(encrypted)
	if decrypted != token {
		t.Errorf("DecryptField failed: got %q, want %q", decrypted, token)
	}
	
	// Test backward compatibility - unencrypted data
	plainValue := "unencrypted-value"
	result := DecryptField(plainValue)
	if result != plainValue {
		t.Errorf("DecryptField should return original for unencrypted data: got %q, want %q", result, plainValue)
	}
}