package pluginpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSourceSlicePaging(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	rel := "src/ui/src/DatabaseWorkbench.tsx"
	full, err := m.ReadSourceSlice("database", rel, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if full.StartLine != 1 || full.EndLine != 50 {
		t.Fatalf("slice=%+v", full)
	}
	if !full.HasMore || full.NextStartLine != 51 {
		t.Fatalf("expect hasMore next=51 got %+v", full)
	}
	if full.TotalLines < 100 {
		t.Fatalf("totalLines=%d", full.TotalLines)
	}
	if strings.Count(full.Content, "\n")+1 != 50 && len(strings.Split(full.Content, "\n")) != 50 {
		// Split gives line count; empty last line edge cases aside
		n := len(strings.Split(full.Content, "\n"))
		if n != 50 {
			t.Fatalf("content lines=%d", n)
		}
	}

	page2, err := m.ReadSourceSlice("database", rel, full.NextStartLine, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page2.StartLine != 51 {
		t.Fatalf("page2 start=%d", page2.StartLine)
	}

	// 超出
	if _, err := m.ReadSourceSlice("database", rel, full.TotalLines+1, 10); err == nil {
		t.Fatal("expected out of range")
	}

	// maxLines 上限
	big, err := m.ReadSourceSlice("database", rel, 1, 9999)
	if err != nil {
		t.Fatal(err)
	}
	if big.EndLine-big.StartLine+1 > maxPackReadLines {
		t.Fatalf("maxLines not capped: %d", big.EndLine-big.StartLine+1)
	}

	// 小文件一次读完
	readme := filepath.Join(PackSourceDir(dir, "database"), "README.md")
	if err := os.WriteFile(readme, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	small, err := m.ReadSourceSlice("database", "README.md", 1, 220)
	if err != nil {
		t.Fatal(err)
	}
	if small.HasMore || small.TotalLines != 4 { // trailing newline → 4 parts: a,b,c,""
		// "a\nb\nc\n" Split → ["a","b","c",""] = 4
		if small.HasMore {
			t.Fatalf("small should not hasMore: %+v", small)
		}
	}
}

func TestPatchSourceFile(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	rel := "src/ui/src/patch_demo.ts"
	if _, err := m.WriteSourceFile("database", rel, "const a = 1;\nconst b = 2;\nconst a = 1;\n"); err != nil {
		t.Fatal(err)
	}

	if _, err := m.PatchSourceFile("database", rel, "const a = 1;", "const a = 9;", false); err == nil {
		t.Fatal("expected ambiguous match error")
	}

	res, err := m.PatchSourceFile("database", rel, "const b = 2;", "const b = 3;", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Replacements != 1 {
		t.Fatalf("replacements=%d", res.Replacements)
	}
	got, _, err := m.ReadSourceFile("database", rel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "const b = 3;") || strings.Contains(got, "const b = 2;") {
		t.Fatalf("content=%q", got)
	}

	res2, err := m.PatchSourceFile("database", rel, "const a = 1;", "const a = 9;", true)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Replacements != 2 {
		t.Fatalf("replaceAll count=%d", res2.Replacements)
	}
	got2, _, _ := m.ReadSourceFile("database", rel)
	if strings.Count(got2, "const a = 9;") != 2 {
		t.Fatalf("got2=%q", got2)
	}

	if _, err := m.PatchSourceFile("database", rel, "nope", "x", false); err == nil {
		t.Fatal("expected not found")
	}
}
