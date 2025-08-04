package models

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GitBlob represents a file or directory in a Git repository
// This is not a database model - it's a wrapper around Git operations
type GitBlob struct {
	Repo        *GitRepo
	IsDirectory bool
	Exists      bool
	Branch      string
	Path        string
}

func (b *GitBlob) Read(v []byte) (int, error) {
	content, err := b.Content()
	if err != nil {
		return 0, err
	}
	return strings.NewReader(content).Read(v)
}

func (blob *GitBlob) Close() error {
	return nil
}

func (blob *GitBlob) Stat() (fs.FileInfo, error) {
	return blob, nil
}

func (blob *GitBlob) Dir() string {
	dir := filepath.Dir(blob.Path)
	if dir == "." {
		return ""
	}

	return dir
}

func (blob *GitBlob) Files() ([]*GitBlob, error) {
	return blob.Repo.Blobs(blob.Branch, blob.Path)
}

func (blob *GitBlob) Content() (string, error) {
	stdout, stderr, err := blob.Repo.Run("show", blob.Branch+":"+blob.Path)
	if err != nil {
		return "", errors.New(stderr.String())
	}

	return stdout.String(), nil
}

func (blob *GitBlob) Lines() ([]string, error) {
	content, err := blob.Content()
	return strings.Split(content, "\n"), err
}

func (blob *GitBlob) Name() string {
	return filepath.Base(blob.Path)
}

func (blob *GitBlob) Size() int64 {
	stdout, _, err := blob.Repo.Run("cat-file", "-s", blob.Branch+":"+blob.Path)
	if err != nil {
		return 0
	}

	size, _ := strconv.ParseInt(strings.TrimSpace(stdout.String()), 10, 64)
	return size
}

func (blob *GitBlob) Mode() fs.FileMode {
	if blob.IsDirectory {
		return fs.ModeDir
	}

	return 0
}

func (*GitBlob) ModTime() time.Time { return time.Now() }
func (b *GitBlob) IsDir() bool      { return b.IsDirectory }
func (*GitBlob) Sys() any           { return nil }

func (blob *GitBlob) FileType() string {
	ext := strings.ToLower(filepath.Ext(blob.Path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp":
		return "image/" + ext[1:]
	default:
		return "text/plain"
	}
}

func (blob *GitBlob) IsImage() bool {
	ext := strings.ToLower(filepath.Ext(blob.Path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp":
		return true
	default:
		return false
	}
}