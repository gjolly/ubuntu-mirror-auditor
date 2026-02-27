package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ProbeRecord represents a record in the probe table
type ProbeRecord struct {
	ID             int
	MirrorURL      string
	Time           time.Time
	TestError      *string
	TestFile       string
	Result         bool
	CorruptedFiles *string
}

// DB wraps the SQLite database operations
type DB struct {
	conn *sql.DB
}

// NewDB creates a new database connection and initializes the schema
func NewDB(dbPath string) (*DB, error) {
	slog.Info("Opening database", "path", dbPath)

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}

	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// initSchema creates the probe table if it doesn't exist
func (db *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS probe (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mirror_url TEXT NOT NULL,
		time TIMESTAMP,
		test_error TEXT,
		test_file TEXT,
		result BOOLEAN,
		corrupted_files TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_mirror_url ON probe(mirror_url);
	CREATE INDEX IF NOT EXISTS idx_time ON probe(time);
	`

	_, err := db.conn.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	slog.Debug("Database schema initialized")
	return nil
}

// InsertProbe inserts a new probe record
func (db *DB) InsertProbe(record ProbeRecord) error {
	query := `
		INSERT INTO probe (mirror_url, time, test_error, test_file, result, corrupted_files)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(query,
		record.MirrorURL,
		record.Time,
		record.TestError,
		record.TestFile,
		record.Result,
		record.CorruptedFiles,
	)
	if err != nil {
		return fmt.Errorf("failed to insert probe: %w", err)
	}

	id, _ := result.LastInsertId()
	slog.Debug("Probe record inserted", "id", id, "mirror", record.MirrorURL, "result", record.Result)

	return nil
}

// InitializeMirrors inserts placeholder probe records for mirrors that don't have any probes yet
// This allows GetOldestCheckedMirror to work efficiently without checking each mirror individually
func (db *DB) InitializeMirrors(mirrors []string) error {
	if len(mirrors) == 0 {
		return nil
	}

	slog.Info("Initializing mirrors in database", "count", len(mirrors))

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement for checking existence
	checkStmt, err := tx.Prepare("SELECT COUNT(*) FROM probe WHERE mirror_url = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare check statement: %w", err)
	}
	defer checkStmt.Close()

	// Prepare statement for insertion
	insertStmt, err := tx.Prepare(`
		INSERT INTO probe (mirror_url, time, test_error, test_file, result, corrupted_files)
		VALUES (?, NULL, NULL, NULL, 0, NULL)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer insertStmt.Close()

	initialized := 0
	for _, mirror := range mirrors {
		var count int
		err := checkStmt.QueryRow(mirror).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check mirror %s: %w", mirror, err)
		}

		if count == 0 {
			_, err := insertStmt.Exec(mirror)
			if err != nil {
				return fmt.Errorf("failed to initialize mirror %s: %w", mirror, err)
			}
			initialized++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if initialized > 0 {
		slog.Info("Initialized new mirrors", "count", initialized)
	}

	return nil
}

// GetOldestCheckedMirror returns the mirror that was checked longest ago
// Mirrors with NULL time (never checked) are prioritized and selected randomly
// This assumes InitializeMirrors has been called to populate the database
func (db *DB) GetOldestCheckedMirror(mirrors []string) (string, error) {
	if len(mirrors) == 0 {
		return "", fmt.Errorf("no mirrors provided")
	}

	placeholders := make([]string, len(mirrors))
	args := make([]interface{}, len(mirrors))
	for i, mirror := range mirrors {
		placeholders[i] = "?"
		args[i] = mirror
	}

	// Find the mirror with the oldest check time (NULL times are considered oldest)
	// We use MAX(time) to get the most recent check for each mirror, then order by that
	// For NULL times (never checked), we randomize the order to avoid always checking in the same sequence
	query := fmt.Sprintf(`
		SELECT mirror_url
		FROM (
			SELECT mirror_url, MAX(time) as last_check
			FROM probe
			WHERE mirror_url IN (%s)
			GROUP BY mirror_url
		)
		ORDER BY 
			last_check IS NOT NULL,
			CASE WHEN last_check IS NULL THEN RANDOM() END,
			last_check ASC
		LIMIT 1
	`, strings.Join(placeholders, ","))

	var oldestMirror string
	err := db.conn.QueryRow(query, args...).Scan(&oldestMirror)
	if err != nil {
		return "", fmt.Errorf("failed to get oldest checked mirror: %w", err)
	}

	return oldestMirror, nil
}

// GetAllProbes returns all probe records
func (db *DB) GetAllProbes() ([]ProbeRecord, error) {
	query := `SELECT id, mirror_url, time, test_error, test_file, result, corrupted_files FROM probe ORDER BY time DESC`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query probes: %w", err)
	}
	defer rows.Close()

	var probes []ProbeRecord
	for rows.Next() {
		var p ProbeRecord
		err := rows.Scan(&p.ID, &p.MirrorURL, &p.Time, &p.TestError, &p.TestFile, &p.Result, &p.CorruptedFiles)
		if err != nil {
			return nil, fmt.Errorf("failed to scan probe: %w", err)
		}
		probes = append(probes, p)
	}

	return probes, nil
}

// GetLatestProbesByMirror returns the latest probe for each mirror
func (db *DB) GetLatestProbesByMirror() ([]ProbeRecord, error) {
	query := `
		SELECT p.id, p.mirror_url, p.time, p.test_error, p.test_file, p.result, p.corrupted_files
		FROM probe p
		INNER JOIN (
			SELECT mirror_url, MAX(time) as max_time
			FROM probe
			GROUP BY mirror_url
		) latest ON p.mirror_url = latest.mirror_url AND p.time = latest.max_time
		ORDER BY p.time DESC
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest probes: %w", err)
	}
	defer rows.Close()

	var probes []ProbeRecord
	for rows.Next() {
		var p ProbeRecord
		err := rows.Scan(&p.ID, &p.MirrorURL, &p.Time, &p.TestError, &p.TestFile, &p.Result, &p.CorruptedFiles)
		if err != nil {
			return nil, fmt.Errorf("failed to scan probe: %w", err)
		}
		probes = append(probes, p)
	}

	return probes, nil
}

// GetDistinctMirrorCount returns the count of distinct mirrors that have been tested
func (db *DB) GetDistinctMirrorCount() (int, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(DISTINCT mirror_url) FROM probe").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get distinct mirror count: %w", err)
	}
	return count, nil
}
