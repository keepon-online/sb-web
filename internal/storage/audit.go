package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"miaomiaowu/internal/systemops"
)

// ErrAuditNotFound is returned when an audit record cannot be located.
var ErrAuditNotFound = errors.New("audit record not found")

// migrateOperationAuditTable creates the audit table for OperationPlan executions.
func (r *TrafficRepository) migrateOperationAuditTable() error {
	if r == nil || r.db == nil {
		return fmt.Errorf("traffic repository not initialized")
	}

	const schema = `
	CREATE TABLE IF NOT EXISTS operation_audit (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    plan_name TEXT NOT NULL,
	    dry_run INTEGER NOT NULL DEFAULT 0,
	    status TEXT NOT NULL,
	    username TEXT,
	    started_at TIMESTAMP NOT NULL,
	    finished_at TIMESTAMP NOT NULL,
	    steps_json TEXT NOT NULL,
	    error_message TEXT,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_operation_audit_plan_name ON operation_audit(plan_name);
	CREATE INDEX IF NOT EXISTS idx_operation_audit_status ON operation_audit(status);
	CREATE INDEX IF NOT EXISTS idx_operation_audit_started_at ON operation_audit(started_at);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate operation_audit: %w", err)
	}
	return nil
}

// EnsureOperationAuditTable initializes the audit table on first use.
func (r *TrafficRepository) EnsureOperationAuditTable() error {
	exists, err := r.EnsureTableExists("operation_audit")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return r.migrateOperationAuditTable()
}

// WriteAudit persists an OperationPlan audit record. Implements systemops.AuditWriter.
func (r *TrafficRepository) WriteAudit(ctx context.Context, record systemops.AuditRecord) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("traffic repository not initialized")
	}

	stepsJSON, err := record.MarshalSteps()
	if err != nil {
		return 0, err
	}

	dryRun := 0
	if record.DryRun {
		dryRun = 1
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO operation_audit
		    (plan_name, dry_run, status, username, started_at, finished_at, steps_json, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.PlanName,
		dryRun,
		record.Status,
		nullableString(record.Username),
		record.StartedAt.UTC(),
		record.FinishedAt.UTC(),
		stepsJSON,
		nullableString(record.Error),
	)
	if err != nil {
		return 0, fmt.Errorf("insert audit: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("audit last insert id: %w", err)
	}
	return id, nil
}

// AuditQueryOptions controls the audit listing query.
type AuditQueryOptions struct {
	Limit    int
	Offset   int
	Status   string
	PlanName string
}

// ListAudits returns audit records ordered by id desc.
func (r *TrafficRepository) ListAudits(ctx context.Context, opts AuditQueryOptions) ([]systemops.AuditRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("traffic repository not initialized")
	}

	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, plan_name, dry_run, status, username, started_at, finished_at, steps_json, error_message
		FROM operation_audit
		WHERE 1=1
	`
	args := []any{}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	if opts.PlanName != "" {
		query += " AND plan_name = ?"
		args = append(args, opts.PlanName)
	}
	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audits: %w", err)
	}
	defer rows.Close()

	var records []systemops.AuditRecord
	for rows.Next() {
		rec, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audits: %w", err)
	}
	return records, nil
}

// GetAudit returns a single audit record by id.
func (r *TrafficRepository) GetAudit(ctx context.Context, id int64) (systemops.AuditRecord, error) {
	if r == nil || r.db == nil {
		return systemops.AuditRecord{}, fmt.Errorf("traffic repository not initialized")
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, plan_name, dry_run, status, username, started_at, finished_at, steps_json, error_message
		FROM operation_audit WHERE id = ?
	`, id)

	rec, err := scanAuditRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return systemops.AuditRecord{}, ErrAuditNotFound
	}
	if err != nil {
		return systemops.AuditRecord{}, err
	}
	return rec, nil
}

func scanAuditRow(scanner rowScanner) (systemops.AuditRecord, error) {
	var (
		rec       systemops.AuditRecord
		dryRun    int
		username  sql.NullString
		errMsg    sql.NullString
		stepsJSON string
		started   time.Time
		finished  time.Time
	)
	if err := scanner.Scan(
		&rec.ID,
		&rec.PlanName,
		&dryRun,
		&rec.Status,
		&username,
		&started,
		&finished,
		&stepsJSON,
		&errMsg,
	); err != nil {
		return systemops.AuditRecord{}, err
	}
	rec.DryRun = dryRun != 0
	if username.Valid {
		rec.Username = username.String
	}
	if errMsg.Valid {
		rec.Error = errMsg.String
	}
	rec.StartedAt = started.UTC()
	rec.FinishedAt = finished.UTC()
	if err := unmarshalSteps(stepsJSON, &rec.Steps); err != nil {
		return systemops.AuditRecord{}, err
	}
	return rec, nil
}

func unmarshalSteps(data string, out *[]systemops.StepAudit) error {
	if data == "" || data == "[]" {
		*out = nil
		return nil
	}
	if err := json.Unmarshal([]byte(data), out); err != nil {
		return fmt.Errorf("unmarshal steps: %w", err)
	}
	return nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
