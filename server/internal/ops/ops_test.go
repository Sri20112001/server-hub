package ops

import (
	"testing"

	"serverhub/internal/testdb"
)

func TestLifecycle(t *testing.T) {
	db := testdb.Open(t)

	op, err := Create(db, "deploy", "project", "1", "admin", []string{"Pull", "Up"})
	if err != nil {
		t.Fatal(err)
	}
	if op.Status != "QUEUED" || len(op.Stages) != 2 {
		t.Fatalf("unexpected op: %+v", op)
	}
	if err := Start(db, op.ID); err != nil {
		t.Fatal(err)
	}
	if err := SetStage(db, op.ID, 0, false, ""); err != nil {
		t.Fatal(err)
	}
	got, err := Get(db, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "RUNNING" || got.Stages[0].State != "active" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if err := Finish(db, op.ID, "SUCCESS", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = Get(db, op.ID)
	if got.Status != "SUCCESS" || got.Stages[1].State != "done" {
		t.Fatalf("unexpected finish: %+v", got)
	}
	list, err := List(db, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if _, err := Get(db, "op_nope"); err == nil {
		t.Fatal("expected error for unknown op")
	}
}
