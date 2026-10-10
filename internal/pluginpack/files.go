package pluginpack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxPackFileBytes = 256 * 1024

// 读文件分段：对齐 Agent 工具结果 ~16k 字上下文上限，按行翻页。
const (
	defaultPackReadLines = 220
	maxPackReadLines     = 400
)

var packFileExtOK = map[string]struct{}{
	".tsx": {}, ".ts": {}, ".jsx": {}, ".js": {}, ".css": {},
	".json": {}, ".yml": {}, ".yaml": {}, ".md": {}, ".html": {},
}

// SourceSlice pack 源码分段读取结果。
type SourceSlice struct {
	Content      string `json:"content"`
	AbsPath      string `json:"absPath"`
	StartLine    int    `json:"startLine"`
	EndLine      int    `json:"endLine"`
	TotalLines   int    `json:"totalLines"`
	HasMore      bool   `json:"hasMore"`
	NextStartLine int   `json:"nextStartLine,omitempty"`
}

func validPackID(packID string) error {
	id := strings.TrimSpace(packID)
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return fmt.Errorf("无效 pack id")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("无效 pack id")
	}
	return nil
}

func (m *Manager) resolveSourceRel(packID, rel string) (string, error) {
	if err := validPackID(packID); err != nil {
		return "", err
	}
	root := PackSourceDir(m.dataDir, packID)
	if _, err := os.Stat(filepath.Join(root, "pack.yml")); err != nil {
		return "", fmt.Errorf("pack %s 不存在", packID)
	}
	clean := filepath.Clean("/" + strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/"))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." || strings.Contains(clean, "..") {
		return "", fmt.Errorf("无效相对路径")
	}
	full := filepath.Join(root, filepath.FromSlash(clean))
	relCheck, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		return "", fmt.Errorf("路径越出 pack 源码目录")
	}
	return full, nil
}

// ListSourceFiles 列出 pack 源码相对路径（跳过 node_modules / .git）。
func (m *Manager) ListSourceFiles(packID string) ([]string, error) {
	if err := validPackID(packID); err != nil {
		return nil, err
	}
	root := PackSourceDir(m.dataDir, packID)
	if _, err := os.Stat(filepath.Join(root, "pack.yml")); err != nil {
		return nil, fmt.Errorf("pack %s 不存在", packID)
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == "node_modules" || name == ".git" || name == "dist" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// ReadSourceFile 读取 pack 源码文件全文（UTF-8 文本）。
func (m *Manager) ReadSourceFile(packID, rel string) (content, abs string, err error) {
	full, err := m.resolveSourceRel(packID, rel)
	if err != nil {
		return "", "", err
	}
	st, err := os.Stat(full)
	if err != nil {
		return "", "", fmt.Errorf("读文件失败: %w", err)
	}
	if st.Size() > maxPackFileBytes {
		return "", "", fmt.Errorf("文件超过 %d 字节，请缩小后再读", maxPackFileBytes)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", "", err
	}
	if !utf8.Valid(b) {
		return "", "", fmt.Errorf("不是 UTF-8 文本")
	}
	return string(b), full, nil
}

// ReadSourceSlice 按行分段读取（startLine 从 1；maxLines<=0 用默认）。
// 大文件必须用本方法翻页，避免整文件顶满 Agent 工具上下文上限。
func (m *Manager) ReadSourceSlice(packID, rel string, startLine, maxLines int) (SourceSlice, error) {
	content, abs, err := m.ReadSourceFile(packID, rel)
	if err != nil {
		return SourceSlice{}, err
	}
	lines := splitFileLines(content)
	total := len(lines)
	if total == 0 {
		return SourceSlice{AbsPath: abs, StartLine: 1, EndLine: 0, TotalLines: 0}, nil
	}
	if startLine <= 0 {
		startLine = 1
	}
	if startLine > total {
		return SourceSlice{}, fmt.Errorf("startLine=%d 超出文件行数 %d", startLine, total)
	}
	if maxLines <= 0 {
		maxLines = defaultPackReadLines
	}
	if maxLines > maxPackReadLines {
		maxLines = maxPackReadLines
	}
	from := startLine - 1
	to := from + maxLines
	if to > total {
		to = total
	}
	chunk := strings.Join(lines[from:to], "\n")
	out := SourceSlice{
		Content:    chunk,
		AbsPath:    abs,
		StartLine:  startLine,
		EndLine:    to,
		TotalLines: total,
		HasMore:    to < total,
	}
	if out.HasMore {
		out.NextStartLine = to + 1
	}
	return out, nil
}

func splitFileLines(content string) []string {
	if content == "" {
		return nil
	}
	// 保留最后空行语义：Split 后若以 \n 结尾会多一个空串
	return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
}

// WriteSourceFile 写入 pack 源码文件（仅允许源码扩展名）。
func (m *Manager) WriteSourceFile(packID, rel, content string) (abs string, err error) {
	full, err := m.resolveSourceRel(packID, rel)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(full))
	if _, ok := packFileExtOK[ext]; !ok {
		return "", fmt.Errorf("不允许写入扩展名 %s", ext)
	}
	if len(content) > maxPackFileBytes {
		return "", fmt.Errorf("内容超过 %d 字节", maxPackFileBytes)
	}
	if !utf8.ValidString(content) {
		return "", fmt.Errorf("内容不是合法 UTF-8")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return "", err
	}
	return full, nil
}

// PatchResult 局部替换结果。
type PatchResult struct {
	AbsPath       string `json:"absPath"`
	Replacements  int    `json:"replacements"`
	BytesBefore   int    `json:"bytesBefore"`
	BytesAfter    int    `json:"bytesAfter"`
}

// PatchSourceFile 在现有源码上做精确字符串替换（大文件首选，避免整文件写回）。
// oldString 必须非空；默认仅允许唯一匹配，replaceAll=true 时替换全部。
func (m *Manager) PatchSourceFile(packID, rel, oldString, newString string, replaceAll bool) (PatchResult, error) {
	if oldString == "" {
		return PatchResult{}, fmt.Errorf("oldString 不能为空")
	}
	content, _, err := m.ReadSourceFile(packID, rel)
	if err != nil {
		return PatchResult{}, err
	}
	// 统一换行再匹配，写回时用 \n
	norm := strings.ReplaceAll(content, "\r\n", "\n")
	oldN := strings.ReplaceAll(oldString, "\r\n", "\n")
	newN := strings.ReplaceAll(newString, "\r\n", "\n")
	count := strings.Count(norm, oldN)
	if count == 0 {
		return PatchResult{}, fmt.Errorf("oldString 在文件中未找到；请用 read_plugin_pack_file 核对原文后重试（含缩进/换行）")
	}
	if count > 1 && !replaceAll {
		return PatchResult{}, fmt.Errorf("oldString 匹配 %d 处；请扩大上下文使唯一，或设 replaceAll=true", count)
	}
	var next string
	if replaceAll {
		next = strings.ReplaceAll(norm, oldN, newN)
	} else {
		next = strings.Replace(norm, oldN, newN, 1)
	}
	n := count
	if !replaceAll {
		n = 1
	}
	written, err := m.WriteSourceFile(packID, rel, next)
	if err != nil {
		return PatchResult{}, err
	}
	return PatchResult{
		AbsPath:      written,
		Replacements: n,
		BytesBefore:  len(norm),
		BytesAfter:   len(next),
	}, nil
}
