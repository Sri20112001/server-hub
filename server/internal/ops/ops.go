// Package ops tracks long-running control-plane work (deploys, restarts,
// backups …) as Operation rows with stage checklists, so the UI can poll
// one endpoint instead of every feature inventing its own progress.
package ops

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"serverhub/internal/database"
)

type Stage struct {
	Name  string `json:"name"`
	State string `json:"state"` // pending | active | done | failed | skipped
}

type Operation struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	TargetType  string  `json:"targetType"`
	TargetID    string  `json:"targetId"`
	Status      string  `json:"status"` // QUEUED | RUNNING | SUCCESS | FAILED | CANCELLED
	Stage       string  `json:"stage"`
	Stages      []Stage `json:"stages"`
	InitiatedBy string  `json:"initiatedBy"`
	Error       string  `json:"error,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   string  `json:"startedAt,omitempty"`
	CompletedAt string  `json:"completedAt,omitempty"`
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "op_" + hex.EncodeToString(b)
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// Create inserts a QUEUED operation and returns it.
func Create(db *database.DB, opType, targetType, targetID, initiatedBy string, stages []string) (*Operation, error) {
	op := &Operation{
		ID:          newID(),
		Type:        opType,
		TargetType:  targetType,
		TargetID:    targetID,
		Status:      "QUEUED",
		InitiatedBy: initiatedBy,
		CreatedAt:   time.Now().UTC().Format("2006-01-02 15:04:05"),
	}
	for _, s := range stages {
		op.Stages = append(op.Stages, Stage{Name: s, State: "pending"})
	}
	raw, _ := json.Marshal(op.Stages)
	_, err := db.Exec(`INSERT INTO operations (id,type,target_type,target_id,status,stages,initiated_by,created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		op.ID, op.Type, op.TargetType, op.TargetID, op.Status, string(raw), op.InitiatedBy, op.CreatedAt)
	if err != nil {
		return nil, err
	}
	return op, nil
}

// Start marks RUNNING and activates the first stage.
func Start(db *database.DB, id string) error {
	_, err := db.Exec(`UPDATE operations SET status='RUNNING', started_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

// SetStage marks stages[:idx] done, stages[idx] active (or failed), rest pending.
func SetStage(db *database.DB, id string, idx int, failed bool, label string) error {
	op, err := Get(db, id)
	if err != nil {
		return err
	}
	for i := range op.Stages {
		switch {
		case i < idx:
			op.Stages[i].State = "done"
		case i == idx:
			if failed {
				op.Stages[i].State = "failed"
			} else {
				op.Stages[i].State = "active"
			}
		default:
			op.Stages[i].State = "pending"
		}
	}
	stage := label
	if stage == "" && idx >= 0 && idx < len(op.Stages) {
		stage = op.Stages[idx].Name
	}
	raw, _ := json.Marshal(op.Stages)
	_, err = db.Exec(`UPDATE operations SET stages=?, stage=? WHERE id=?`, string(raw), stage, id)
	return err
}

// Finish marks terminal status and completes remaining stages accordingly.
func Finish(db *database.DB, id, status, errMsg string) error {
	op, err := Get(db, id)
	if err != nil {
		return err
	}
	for i := range op.Stages {
		if op.Stages[i].State == "active" || op.Stages[i].State == "pending" {
			if status == "SUCCESS" {
				op.Stages[i].State = "done"
			} else if op.Stages[i].State == "active" {
				op.Stages[i].State = "failed"
			}
		}
	}
	raw, _ := json.Marshal(op.Stages)
	_, err = db.Exec(`UPDATE operations SET status=?, error=?, stages=?, completed_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, errMsg, string(raw), id)
	return err
}

// Get fetches one operation.
func Get(db *database.DB, id string) (*Operation, error) {
	var op Operation
	var stages string
	var created, started, completed sql.NullString
	err := db.QueryRow(`SELECT id,type,target_type,target_id,status,stage,stages,
		initiated_by,error,created_at,started_at,completed_at FROM operations WHERE id=?`, id).
		Scan(&op.ID, &op.Type, &op.TargetType, &op.TargetID, &op.Status, &op.Stage,
			&stages, &op.InitiatedBy, &op.Error, &created, &started, &completed)
	if err != nil {
		return nil, err
	}
	op.CreatedAt, op.StartedAt, op.CompletedAt = nullStr(created), nullStr(started), nullStr(completed)
	_ = json.Unmarshal([]byte(stages), &op.Stages)
	if op.Stages == nil {
		op.Stages = []Stage{}
	}
	return &op, nil
}

// List returns the newest operations first.
func List(db *database.DB, limit int) ([]Operation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`SELECT id,type,target_type,target_id,status,stage,stages,
		initiated_by,error,created_at,started_at,completed_at FROM operations
		ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operation{}
	for rows.Next() {
		var op Operation
		var stages string
		var created, started, completed sql.NullString
		if err := rows.Scan(&op.ID, &op.Type, &op.TargetType, &op.TargetID, &op.Status,
			&op.Stage, &stages, &op.InitiatedBy, &op.Error,
			&created, &started, &completed); err == nil {
			op.CreatedAt, op.StartedAt, op.CompletedAt = nullStr(created), nullStr(started), nullStr(completed)
			_ = json.Unmarshal([]byte(stages), &op.Stages)
			if op.Stages == nil {
				op.Stages = []Stage{}
			}
			out = append(out, op)
		}
	}
	return out, nil
}
