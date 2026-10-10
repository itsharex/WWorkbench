package pluginpack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotAndRollback(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	// 伪造已构建 UI
	ui := PackUIDir(dir, "database")
	if err := os.MkdirAll(ui, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ui, "index.js"), []byte("var WWPack_database={}"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap1, err := m.Snapshot("database", "manual", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if !snap1.HasUI {
		t.Fatal("expected UI in snapshot")
	}
	marker := filepath.Join(PackSourceDir(dir, "database"), "src", "ui", "src", "EDIT.txt")
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap2, err := m.Snapshot("database", "manual", "v2")
	if err != nil {
		t.Fatal(err)
	}
	list, err := m.ListReleases("database")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("releases=%d", len(list))
	}
	if _, err := m.Rollback("database", snap1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("EDIT.txt should be gone after rollback to snap1, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(ui, "index.js")); err != nil {
		t.Fatal("UI should restore from snap1")
	}
	// 再列：应有 pre-rollback 快照
	list2, err := m.ListReleases("database")
	if err != nil {
		t.Fatal(err)
	}
	foundPre := false
	for _, r := range list2 {
		if r.Reason == reasonPreRollback {
			foundPre = true
			break
		}
	}
	if !foundPre {
		t.Fatal("expected pre-rollback snapshot")
	}
	_ = snap2
}
