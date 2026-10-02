package whisperxworker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	_ "github.com/glebarez/go-sqlite"
	"github.com/google/uuid"
)

type checkpointRef struct {
	Path             string  `json:"path"`
	SHA256           string  `json:"sha256"`
	CompletedSeconds float64 `json:"completed_seconds"`
	SegmentCount     int     `json:"segment_count"`
}
type record struct {
	Request         transcription.JobRequest `json:"request"`
	Status          transcription.JobStatus  `json:"status"`
	ResultDigest    string                   `json:"result_digest"`
	AcknowledgedAt  *time.Time               `json:"acknowledged_at"`
	Pruned          bool                     `json:"pruned"`
	Checkpoint      *checkpointRef           `json:"checkpoint"`
	BudgetUsed      time.Duration            `json:"budget_used"`
	RunSince        *time.Time               `json:"run_since"`
	Recoveries      int                      `json:"recoveries"`
	RecoveryOffset  float64                  `json:"recovery_offset"`
	PipelineVersion string                   `json:"pipeline_version"`
}
type store struct {
	db       *sql.DB
	dir      string
	lock     *os.File
	instance string
}

func openStore(dir string) (_ *store, err error) {
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	lock, err := lockState(filepath.Join(dir, "owner.lock"))
	if err != nil {
		return nil, err
	}
	s := &store{dir: dir, lock: lock}
	defer func() {
		if err != nil {
			s.close()
		}
	}()
	s.db, err = sql.Open("sqlite", filepath.Join(dir, "journal.sqlite"))
	if err != nil {
		return nil, err
	}
	s.db.SetMaxOpenConns(1)
	for _, query := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY,value TEXT NOT NULL)", "CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY,state TEXT NOT NULL,acknowledged INTEGER NOT NULL DEFAULT 0,data BLOB NOT NULL)"} {
		if _, err = s.db.Exec(query); err != nil {
			return nil, err
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{"schema_version": "1", "instance_id": uuid.NewString(), "accepting": "1"} {
		if _, err = tx.Exec("INSERT OR IGNORE INTO metadata(key,value) VALUES (?,?)", key, value); err != nil {
			return nil, err
		}
	}
	var version string
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='schema_version'").Scan(&version); err != nil {
		return nil, err
	}
	if version != "1" {
		return nil, errors.New("unsupported worker state schema")
	}
	if err = tx.QueryRow("SELECT value FROM metadata WHERE key='instance_id'").Scan(&s.instance); err != nil {
		return nil, err
	}
	if !transcription.ValidUUID(s.instance) {
		return nil, errors.New("invalid state instance identity")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(dir, "jobs"), 0700); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *store) close() error {
	var err error
	if s.db != nil {
		err = s.db.Close()
	}
	if s.lock != nil {
		err = errors.Join(err, s.lock.Close())
	}
	return err
}
func (s *store) get(id string) (*record, error) {
	var raw []byte
	if err := s.db.QueryRow("SELECT data FROM jobs WHERE id=?", id).Scan(&raw); err != nil {
		return nil, err
	}
	var r record
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Status.JobID != id {
		return nil, errors.New("journal identity mismatch")
	}
	return &r, nil
}
func (s *store) save(r *record, insert bool) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if insert {
		_, err = tx.Exec("INSERT INTO jobs(id,state,acknowledged,data) VALUES (?,?,?,?)", r.Status.JobID, r.Status.State, r.AcknowledgedAt != nil, raw)
	} else {
		var result sql.Result
		result, err = tx.Exec("UPDATE jobs SET state=?,acknowledged=?,data=? WHERE id=?", r.Status.State, r.AcknowledgedAt != nil, raw, r.Status.JobID)
		if err == nil {
			n, e := result.RowsAffected()
			if e != nil {
				return e
			}
			if n != 1 {
				return errors.New("missing journal row")
			}
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *store) list(where string, args ...any) ([]*record, error) {
	rows, err := s.db.Query("SELECT id,data FROM jobs "+where+" ORDER BY rowid", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*record{}
	for rows.Next() {
		var raw []byte
		var id string
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var r record
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		if !transcription.ValidUUID(id) || r.Status.JobID != id {
			return nil, errors.New("invalid persisted job identity")
		}
		result = append(result, &r)
	}
	return result, rows.Err()
}
func (s *store) counts() (transcription.MaintenanceStatus, error) {
	var c transcription.MaintenanceStatus
	var accepting string
	if err := s.db.QueryRow("SELECT value FROM metadata WHERE key='accepting'").Scan(&accepting); err != nil {
		return c, err
	}
	c.Accepting = accepting == "1"
	rows, err := s.db.Query("SELECT state,acknowledged,COUNT(*) FROM jobs GROUP BY state,acknowledged")
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var ack, n int
		if err = rows.Scan(&state, &ack, &n); err != nil {
			return c, err
		}
		switch state {
		case "queued":
			c.Queued += n
		case "running", "cancelling":
			c.Running += n
		case "succeeded":
			if ack == 0 {
				c.UnacknowledgedResults += n
			}
		}
	}
	return c, rows.Err()
}
func (s *store) accepting(value bool) error {
	text := "0"
	if value {
		text = "1"
	}
	_, err := s.db.Exec("UPDATE metadata SET value=? WHERE key='accepting'", text)
	return err
}
func (s *store) jobDir(id string) string     { return filepath.Join(s.dir, "jobs", id) }
func (s *store) resultPath(id string) string { return filepath.Join(s.jobDir(id), "result.json") }

func durableWrite(path string, raw []byte) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { file.Close(); os.Remove(name) }()
	if _, err = file.Write(raw); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("invalid artifact size or type")
	}
	return readLimit(f, limit)
}
