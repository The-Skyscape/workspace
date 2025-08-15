package models

import (
	"fmt"
	"path/filepath"
	"strings"
	
	"github.com/The-Skyscape/devtools/pkg/database"
)

// FileSearchResult represents a search result from the FTS index
type FileSearchResult struct {
	RepoID     string  `json:"repo_id"`
	FilePath   string  `json:"file_path"`
	FileName   string  `json:"file_name"`
	Language   string  `json:"language"`
	Content    string  `json:"content"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

// FileSearch provides full-text search functionality for repository files
type FileSearch struct{}

var Search *FileSearch

// InitFileSearch initializes the FTS5 table for file search
func InitFileSearch() error {
	Search = &FileSearch{}
	
	// Create FTS5 virtual table for file indexing
	query := `
	CREATE VIRTUAL TABLE IF NOT EXISTS file_search USING fts5(
		repo_id UNINDEXED,
		file_path UNINDEXED,
		file_name,
		language UNINDEXED,
		content,
		tokenize = 'porter unicode61'
	);`
	
	if err := DB.Query(query).Exec(); err != nil {
		return fmt.Errorf("failed to create FTS5 table: %w", err)
	}
	
	// Create regular table for metadata that FTS5 doesn't handle well
	metaQuery := `
	CREATE TABLE IF NOT EXISTS file_metadata (
		rowid INTEGER PRIMARY KEY,
		repo_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		file_name TEXT NOT NULL,
		language TEXT,
		size INTEGER,
		last_indexed TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repo_id, file_path)
	);`
	
	if err := DB.Query(metaQuery).Exec(); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}
	
	// Create index for faster lookups
	indexQuery := `CREATE INDEX IF NOT EXISTS idx_file_metadata_repo ON file_metadata(repo_id);`
	if err := DB.Query(indexQuery).Exec(); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	
	return nil
}

// IndexFile adds or updates a file in the search index
func (fs *FileSearch) IndexFile(repoID, filePath, content string) error {
	fileName := filepath.Base(filePath)
	language := getLanguageFromPath(filePath)
	
	// Get the underlying connection for transaction support
	// Note: FTS5 requires transactions, so we need the underlying sql.DB
	iter := DB.Query("SELECT 1")
	tx, err := iter.Conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Check if file already exists
	var existingRowID int64
	err = tx.QueryRow(`
		SELECT rowid FROM file_metadata 
		WHERE repo_id = ? AND file_path = ?
	`, repoID, filePath).Scan(&existingRowID)
	
	if err != nil && err.Error() == "sql: no rows in result set" {
		// Insert new file
		result, err := tx.Exec(`
			INSERT INTO file_metadata (repo_id, file_path, file_name, language, size)
			VALUES (?, ?, ?, ?, ?)
		`, repoID, filePath, fileName, language, len(content))
		if err != nil {
			return fmt.Errorf("failed to insert metadata: %w", err)
		}
		
		rowID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		
		// Insert into FTS table
		_, err = tx.Exec(`
			INSERT INTO file_search (rowid, repo_id, file_path, file_name, language, content)
			VALUES (?, ?, ?, ?, ?, ?)
		`, rowID, repoID, filePath, fileName, language, content)
		if err != nil {
			return fmt.Errorf("failed to insert into FTS: %w", err)
		}
	} else if err == nil {
		// Update existing file
		_, err = tx.Exec(`
			UPDATE file_metadata 
			SET size = ?, last_indexed = CURRENT_TIMESTAMP
			WHERE rowid = ?
		`, len(content), existingRowID)
		if err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
		
		// Update FTS table
		_, err = tx.Exec(`
			UPDATE file_search 
			SET content = ?
			WHERE rowid = ?
		`, content, existingRowID)
		if err != nil {
			return fmt.Errorf("failed to update FTS: %w", err)
		}
	} else {
		return err
	}
	
	return tx.Commit()
}

// DeleteFile removes a file from the search index
func (fs *FileSearch) DeleteFile(repoID, filePath string) error {
	// Get the underlying connection for transaction support
	iter := DB.Query("SELECT 1")
	tx, err := iter.Conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Get rowid
	var rowID int64
	err = tx.QueryRow(`
		SELECT rowid FROM file_metadata 
		WHERE repo_id = ? AND file_path = ?
	`, repoID, filePath).Scan(&rowID)
	
	if err != nil {
		return err
	}
	
	// Delete from both tables
	_, err = tx.Exec(`DELETE FROM file_search WHERE rowid = ?`, rowID)
	if err != nil {
		return err
	}
	
	_, err = tx.Exec(`DELETE FROM file_metadata WHERE rowid = ?`, rowID)
	if err != nil {
		return err
	}
	
	return tx.Commit()
}

// DeleteRepository removes all files for a repository from the index
func (fs *FileSearch) DeleteRepository(repoID string) error {
	// Get the underlying connection for transaction support
	iter := DB.Query("SELECT 1")
	tx, err := iter.Conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Get all rowids for the repository
	rows, err := tx.Query(`SELECT rowid FROM file_metadata WHERE repo_id = ?`, repoID)
	if err != nil {
		return err
	}
	defer rows.Close()
	
	var rowIDs []int64
	for rows.Next() {
		var rowID int64
		if err := rows.Scan(&rowID); err != nil {
			return err
		}
		rowIDs = append(rowIDs, rowID)
	}
	
	// Delete from FTS table
	for _, rowID := range rowIDs {
		_, err = tx.Exec(`DELETE FROM file_search WHERE rowid = ?`, rowID)
		if err != nil {
			return err
		}
	}
	
	// Delete from metadata table
	_, err = tx.Exec(`DELETE FROM file_metadata WHERE repo_id = ?`, repoID)
	if err != nil {
		return err
	}
	
	return tx.Commit()
}

// SearchInRepo searches for files within a specific repository
func (fs *FileSearch) SearchInRepo(repoID, query string, limit int) ([]*FileSearchResult, error) {
	if fs == nil {
		return nil, fmt.Errorf("file search not initialized")
	}

	var results []*FileSearchResult
	err := DB.Query(`
		SELECT 
			fs.repo_id,
			fs.file_path,
			fs.file_name,
			fs.language,
			snippet(file_search, 4, '<mark>', '</mark>', '...', 20) as snippet,
			bm25(file_search) as score
		FROM file_search fs
		WHERE fs.repo_id = ? AND file_search MATCH ?
		ORDER BY score DESC
		LIMIT ?
	`, repoID, query, limit).All(func(scan database.ScanFunc) error {
		var r FileSearchResult
		err := scan(&r.RepoID, &r.FilePath, &r.FileName, &r.Language, &r.Snippet, &r.Score)
		if err != nil {
			return nil // Continue on scan errors
		}
		results = append(results, &r)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}

	return results, nil
}

// SearchGlobal searches across all repositories accessible to the user
func (fs *FileSearch) SearchGlobal(query, userID string, limit int) ([]*FileSearchResult, error) {
	if fs == nil {
		return nil, fmt.Errorf("file search not initialized")
	}

	// First get accessible repository IDs
	// For simplicity, we'll search all public repos and user's private repos
	var results []*FileSearchResult
	err := DB.Query(`
		SELECT 
			fs.repo_id,
			fs.file_path,
			fs.file_name,
			fs.language,
			snippet(file_search, 4, '<mark>', '</mark>', '...', 20) as snippet,
			bm25(file_search) as score
		FROM file_search fs
		JOIN repositories r ON fs.repo_id = r.ID
		WHERE file_search MATCH ? 
		  AND (r.Visibility = 'public' OR r.UserID = ?)
		ORDER BY score DESC
		LIMIT ?
	`, query, userID, limit).All(func(scan database.ScanFunc) error {
		var r FileSearchResult
		err := scan(&r.RepoID, &r.FilePath, &r.FileName, &r.Language, &r.Snippet, &r.Score)
		if err != nil {
			return nil // Continue on scan errors
		}
		results = append(results, &r)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("global search query failed: %w", err)
	}

	return results, nil
}

// SearchFiles performs a full-text search across all indexed files
func (fs *FileSearch) SearchFiles(query string, repoID string, limit int) ([]FileSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	
	var results []FileSearchResult
	var err error
	
	// Build the search query
	if repoID != "" {
		err = DB.Query(`
			SELECT 
				fs.repo_id,
				fs.file_path,
				fs.file_name,
				fs.language,
				snippet(file_search, 4, '<mark>', '</mark>', '...', 32) as snippet,
				bm25(file_search) as score
			FROM file_search fs
			WHERE file_search MATCH ? AND fs.repo_id = ?
			ORDER BY score
			LIMIT ?
		`, query, repoID, limit).All(func(scan database.ScanFunc) error {
			var result FileSearchResult
			err := scan(&result.RepoID, &result.FilePath, &result.FileName, &result.Language, &result.Snippet, &result.Score)
			if err != nil {
				return err
			}
			results = append(results, result)
			return nil
		})
	} else {
		err = DB.Query(`
			SELECT 
				fs.repo_id,
				fs.file_path,
				fs.file_name,
				fs.language,
				snippet(file_search, 4, '<mark>', '</mark>', '...', 32) as snippet,
				bm25(file_search) as score
			FROM file_search fs
			WHERE file_search MATCH ?
			ORDER BY score
			LIMIT ?
		`, query, limit).All(func(scan database.ScanFunc) error {
			var result FileSearchResult
			err := scan(&result.RepoID, &result.FilePath, &result.FileName, &result.Language, &result.Snippet, &result.Score)
			if err != nil {
				return err
			}
			results = append(results, result)
			return nil
		})
	}
	
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	
	return results, nil
}

// SearchFilesByPath searches for files by path pattern
func (fs *FileSearch) SearchFilesByPath(pattern string, repoID string, limit int) ([]FileSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	
	var results []FileSearchResult
	var err error
	
	if repoID != "" {
		err = DB.Query(`
			SELECT 
				repo_id,
				file_path,
				file_name,
				language,
				'' as snippet,
				0 as score
			FROM file_metadata
			WHERE file_path LIKE ? AND repo_id = ?
			ORDER BY file_path
			LIMIT ?
		`, "%"+pattern+"%", repoID, limit).All(func(scan database.ScanFunc) error {
			var result FileSearchResult
			err := scan(&result.RepoID, &result.FilePath, &result.FileName, &result.Language, &result.Snippet, &result.Score)
			if err != nil {
				return err
			}
			results = append(results, result)
			return nil
		})
	} else {
		err = DB.Query(`
			SELECT 
				repo_id,
				file_path,
				file_name,
				language,
				'' as snippet,
				0 as score
			FROM file_metadata
			WHERE file_path LIKE ?
			ORDER BY file_path
			LIMIT ?
		`, "%"+pattern+"%", limit).All(func(scan database.ScanFunc) error {
			var result FileSearchResult
			err := scan(&result.RepoID, &result.FilePath, &result.FileName, &result.Language, &result.Snippet, &result.Score)
			if err != nil {
				return err
			}
			results = append(results, result)
			return nil
		})
	}
	
	if err != nil {
		return nil, err
	}
	
	return results, nil
}

// GetIndexedFileCount returns the number of indexed files for a repository
func (fs *FileSearch) GetIndexedFileCount(repoID string) (int, error) {
	var count int
	err := DB.Query(`
		SELECT COUNT(*) FROM file_metadata WHERE repo_id = ?
	`, repoID).Scan(&count)
	return count, err
}

// getLanguageFromPath determines the programming language from file extension
func getLanguageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	
	languages := map[string]string{
		".go":     "go",
		".js":     "javascript",
		".ts":     "typescript",
		".jsx":    "javascript",
		".tsx":    "typescript",
		".py":     "python",
		".rb":     "ruby",
		".java":   "java",
		".c":      "c",
		".cpp":    "cpp",
		".cs":     "csharp",
		".php":    "php",
		".rs":     "rust",
		".swift":  "swift",
		".kt":     "kotlin",
		".scala":  "scala",
		".r":      "r",
		".m":      "objc",
		".mm":     "objcpp",
		".pl":     "perl",
		".sh":     "bash",
		".bash":   "bash",
		".zsh":    "zsh",
		".fish":   "fish",
		".ps1":    "powershell",
		".lua":    "lua",
		".vim":    "vim",
		".md":     "markdown",
		".markdown": "markdown",
		".rst":    "rst",
		".txt":    "text",
		".json":   "json",
		".xml":    "xml",
		".yaml":   "yaml",
		".yml":    "yaml",
		".toml":   "toml",
		".ini":    "ini",
		".cfg":    "cfg",
		".conf":   "conf",
		".html":   "html",
		".htm":    "html",
		".css":    "css",
		".scss":   "scss",
		".sass":   "sass",
		".less":   "less",
		".sql":    "sql",
		".dockerfile": "dockerfile",
		".gitignore": "gitignore",
		".env":    "env",
	}
	
	if lang, ok := languages[ext]; ok {
		return lang
	}
	return "text"
}