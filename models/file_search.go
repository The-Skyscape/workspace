package models

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
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
type FileSearch struct {
	db *sql.DB
}

var Search *FileSearch

// InitFileSearch initializes the FTS5 table for file search
func InitFileSearch(db *sql.DB) error {
	Search = &FileSearch{db: db}
	
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
	
	if _, err := db.Exec(query); err != nil {
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
	
	if _, err := db.Exec(metaQuery); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}
	
	// Create index for faster lookups
	indexQuery := `CREATE INDEX IF NOT EXISTS idx_file_metadata_repo ON file_metadata(repo_id);`
	if _, err := db.Exec(indexQuery); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	
	return nil
}

// IndexFile adds or updates a file in the search index
func (fs *FileSearch) IndexFile(repoID, filePath, content string) error {
	fileName := filepath.Base(filePath)
	language := getLanguageFromPath(filePath)
	
	// Start transaction
	tx, err := fs.db.Begin()
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
	
	if err == sql.ErrNoRows {
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
	tx, err := fs.db.Begin()
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
	tx, err := fs.db.Begin()
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

// SearchFiles performs a full-text search across all indexed files
func (fs *FileSearch) SearchFiles(query string, repoID string, limit int) ([]FileSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	
	var results []FileSearchResult
	var rows *sql.Rows
	var err error
	
	// Build the search query
	searchQuery := fmt.Sprintf(`
		SELECT 
			fs.repo_id,
			fs.file_path,
			fs.file_name,
			fs.language,
			snippet(file_search, 4, '<mark>', '</mark>', '...', 32) as snippet,
			bm25(file_search) as score
		FROM file_search fs
		WHERE file_search MATCH ?
		%s
		ORDER BY score
		LIMIT ?
	`, func() string {
		if repoID != "" {
			return "AND fs.repo_id = ?"
		}
		return ""
	}())
	
	if repoID != "" {
		rows, err = fs.db.Query(searchQuery, query, repoID, limit)
	} else {
		rows, err = fs.db.Query(searchQuery, query, limit)
	}
	
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()
	
	for rows.Next() {
		var result FileSearchResult
		err := rows.Scan(
			&result.RepoID,
			&result.FilePath,
			&result.FileName,
			&result.Language,
			&result.Snippet,
			&result.Score,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	
	return results, nil
}

// SearchFilesByPath searches for files by path pattern
func (fs *FileSearch) SearchFilesByPath(pattern string, repoID string, limit int) ([]FileSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	
	var results []FileSearchResult
	query := `
		SELECT 
			repo_id,
			file_path,
			file_name,
			language,
			'' as snippet,
			0 as score
		FROM file_metadata
		WHERE file_path LIKE ?
		%s
		ORDER BY file_path
		LIMIT ?
	`
	
	var rows *sql.Rows
	var err error
	
	if repoID != "" {
		query = fmt.Sprintf(query, "AND repo_id = ?")
		rows, err = fs.db.Query(query, "%"+pattern+"%", repoID, limit)
	} else {
		query = fmt.Sprintf(query, "")
		rows, err = fs.db.Query(query, "%"+pattern+"%", limit)
	}
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	for rows.Next() {
		var result FileSearchResult
		err := rows.Scan(
			&result.RepoID,
			&result.FilePath,
			&result.FileName,
			&result.Language,
			&result.Snippet,
			&result.Score,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	
	return results, nil
}

// GetIndexedFileCount returns the number of indexed files for a repository
func (fs *FileSearch) GetIndexedFileCount(repoID string) (int, error) {
	var count int
	err := fs.db.QueryRow(`
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