package pluginpack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSeededAndList(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "database" {
		t.Fatalf("list=%v", list)
	}
	if !list[0].Enabled {
		t.Fatal("expected database enabled by default")
	}
	marker := filepath.Join(PackSourceDir(dir, "database"), "USER_EDIT.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != "keep" {
		t.Fatalf("user edit overwritten: %q %v", b, err)
	}
	info, err := m.SetEnabled("database", false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Enabled {
		t.Fatal("expected disabled")
	}
}

func TestEnsureSeededUpgradesNewerSeedVersion(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	manPath := PackManifestPath(dir, "database")
	b, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatal(err)
	}
	// 假装已安装更旧版本，并留下用户文件
	old := []byte("name: database\nversion: 0.0.1\nprovides:\n  products:\n    - id: database\n")
	if err := os.WriteFile(manPath, old, 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(PackSourceDir(dir, "database"), "USER_EDIT.txt")
	if err := os.WriteFile(marker, []byte("gone"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected user edit removed on upgrade, err=%v", err)
	}
	man, err := m.ReadManifest("database")
	if err != nil {
		t.Fatal(err)
	}
	if compareSemver(man.Version, "0.0.1") <= 0 {
		t.Fatalf("expected seed version > 0.0.1, got %s", man.Version)
	}
	_ = b
}

func TestCompareSemver(t *testing.T) {
	if compareSemver("0.2.0", "0.1.0") <= 0 {
		t.Fatal("0.2.0 should be greater")
	}
	if compareSemver("0.1.0", "0.1.0") != 0 {
		t.Fatal("equal")
	}
	if compareSemver("0.1.0", "0.2.0") >= 0 {
		t.Fatal("0.1.0 should be less")
	}
}

func TestGatePermissions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	base := map[string]bool{
		"execute_sql":     true,
		"list_tables":     true,
		"shell_run":       true,
		"list_connections": true,
	}
	gated := m.GatePermissions(base)
	if !gated["execute_sql"] || !gated["list_tables"] || !gated["list_connections"] {
		t.Fatalf("enabled pack should keep db caps: %v", gated)
	}
	if !gated["shell_run"] {
		t.Fatal("host-native shell_run must stay")
	}
	if _, err := m.SetEnabled("database", false); err != nil {
		t.Fatal(err)
	}
	gated = m.GatePermissions(base)
	if gated["execute_sql"] || gated["list_tables"] || gated["list_connections"] {
		t.Fatalf("disabled pack should gate db caps: %v", gated)
	}
	if !gated["shell_run"] {
		t.Fatal("host-native shell_run must stay when pack off")
	}
}
