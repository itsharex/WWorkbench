package pluginpack

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxPackReleases   = 20
	releaseMetaFile   = "meta.json"
	releaseSourceDir  = "source"
	releaseUIDir      = "ui"
	reasonBuild       = "build"
	reasonPreUpgrade  = "pre-upgrade"
	reasonPreRollback = "pre-rollback"
	reasonManual      = "manual"
)

// ListReleases 列出 pack 历史版本（新→旧）。
func (m *Manager) ListReleases(packID string) ([]ReleaseInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validPackID(packID); err != nil {
		return nil, err
	}
	return m.listReleasesUnlocked(packID)
}

// Snapshot 手动打快照（一般构建会自动打）。
func (m *Manager) Snapshot(packID, reason, note string) (ReleaseInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validPackID(packID); err != nil {
		return ReleaseInfo{}, err
	}
	if strings.TrimSpace(reason) == "" {
		reason = reasonManual
	}
	info, err := m.snapshotUnlocked(packID, reason, note)
	if err != nil {
		return ReleaseInfo{}, err
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return info, nil
	}
	entry := lock.Packs[packID]
	entry.ActiveReleaseID = info.ID
	lock.Packs[packID] = entry
	_ = m.writeLockUnlocked(lock)
	info.Active = true
	return info, nil
}

// Rollback 恢复到指定 release（会先给当前工作区打 pre-rollback 快照）。
func (m *Manager) Rollback(packID, releaseID string) (PackInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validPackID(packID); err != nil {
		return PackInfo{}, err
	}
	releaseID = strings.TrimSpace(releaseID)
	if releaseID == "" || strings.Contains(releaseID, "..") || strings.ContainsAny(releaseID, `/\`) {
		return PackInfo{}, fmt.Errorf("无效 release id")
	}
	relRoot := PackReleaseDir(m.dataDir, packID, releaseID)
	meta, err := readReleaseMeta(relRoot)
	if err != nil {
		return PackInfo{}, fmt.Errorf("release 不存在: %w", err)
	}
	srcSnap := filepath.Join(relRoot, releaseSourceDir)
	if st, err := os.Stat(srcSnap); err != nil || !st.IsDir() {
		return PackInfo{}, fmt.Errorf("release 缺少源码快照")
	}

	// 回滚前保全当前，便于再次回滚
	if _, err := os.Stat(PackManifestPath(m.dataDir, packID)); err == nil {
		_, _ = m.snapshotUnlocked(packID, reasonPreRollback, "before rollback to "+releaseID)
	}

	dst := PackSourceDir(m.dataDir, packID)
	_ = os.RemoveAll(dst)
	if err := copyTree(srcSnap, dst, nil); err != nil {
		return PackInfo{}, fmt.Errorf("恢复源码: %w", err)
	}

	uiDst := PackUIDir(m.dataDir, packID)
	_ = os.RemoveAll(uiDst)
	uiSnap := filepath.Join(relRoot, releaseUIDir)
	hasUI := false
	if st, err := os.Stat(uiSnap); err == nil && st.IsDir() {
		if err := copyTree(uiSnap, uiDst, nil); err != nil {
			return PackInfo{}, fmt.Errorf("恢复 UI: %w", err)
		}
		hasUI = true
	}

	lock, err := m.readLockUnlocked()
	if err != nil {
		return PackInfo{}, err
	}
	entry := lock.Packs[packID]
	entry.Version = meta.Version
	entry.ActiveReleaseID = meta.ID
	entry.BuildFingerprint = meta.BuildFingerprint
	if hasUI {
		if entry.BuildFingerprint == "" {
			if fp, err := fingerprintDir(uiDst); err == nil {
				entry.BuildFingerprint = fp
			}
		}
		entry.BuiltAt = time.Now().UTC().Format(time.RFC3339)
	} else {
		entry.BuildFingerprint = ""
		entry.BuiltAt = ""
	}
	if _, ok := lock.Packs[packID]; !ok {
		entry.Enabled = true
	}
	lock.Packs[packID] = entry
	if err := m.writeLockUnlocked(lock); err != nil {
		return PackInfo{}, err
	}
	return m.packInfoUnlocked(packID, lock)
}

func (m *Manager) listReleasesUnlocked(packID string) ([]ReleaseInfo, error) {
	root := PackReleasesDir(m.dataDir, packID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []ReleaseInfo{}, nil
		}
		return nil, err
	}
	lock, _ := m.readLockUnlocked()
	active := ""
	if lock != nil {
		active = lock.Packs[packID].ActiveReleaseID
	}
	out := make([]ReleaseInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		meta, err := readReleaseMeta(dir)
		if err != nil {
			continue
		}
		out = append(out, ReleaseInfo{
			ID:               meta.ID,
			PackID:           meta.PackID,
			Version:          meta.Version,
			CreatedAt:        meta.CreatedAt,
			Reason:           meta.Reason,
			Note:             meta.Note,
			BuildFingerprint: meta.BuildFingerprint,
			HasUI:            meta.HasUI,
			Active:           meta.ID == active,
			Path:             dir,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

func (m *Manager) countReleasesUnlocked(packID string) int {
	list, err := m.listReleasesUnlocked(packID)
	if err != nil {
		return 0
	}
	return len(list)
}

// snapshotUnlocked 打快照；调用方须已持锁。
func (m *Manager) snapshotUnlocked(packID, reason, note string) (ReleaseInfo, error) {
	src := PackSourceDir(m.dataDir, packID)
	if _, err := os.Stat(filepath.Join(src, "pack.yml")); err != nil {
		return ReleaseInfo{}, fmt.Errorf("pack %s 不存在", packID)
	}
	man, err := m.readManifestUnlocked(packID)
	if err != nil {
		return ReleaseInfo{}, err
	}
	createdAt := time.Now().UTC()
	id := fmt.Sprintf("%s-%s", sanitizeReleaseToken(man.Version), createdAt.Format("20060102T150405Z"))
	root := PackReleaseDir(m.dataDir, packID, id)
	if _, err := os.Stat(root); err == nil {
		id = fmt.Sprintf("%s-%s", id, createdAt.Format("000000"))
		root = PackReleaseDir(m.dataDir, packID, id)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return ReleaseInfo{}, err
	}
	srcDst := filepath.Join(root, releaseSourceDir)
	skip := map[string]struct{}{
		"node_modules": {},
		".git":         {},
		"dist":         {},
	}
	if err := copyTree(src, srcDst, skip); err != nil {
		_ = os.RemoveAll(root)
		return ReleaseInfo{}, fmt.Errorf("拷贝源码: %w", err)
	}

	hasUI := false
	fp := ""
	uiSrc := PackUIDir(m.dataDir, packID)
	if st, err := os.Stat(uiSrc); err == nil && st.IsDir() {
		if entries, err := os.ReadDir(uiSrc); err == nil && len(entries) > 0 {
			uiDst := filepath.Join(root, releaseUIDir)
			if err := copyTree(uiSrc, uiDst, nil); err != nil {
				_ = os.RemoveAll(root)
				return ReleaseInfo{}, fmt.Errorf("拷贝 UI: %w", err)
			}
			hasUI = true
			fp, _ = fingerprintDir(uiSrc)
		}
	}
	if fp == "" {
		if lock, err := m.readLockUnlocked(); err == nil {
			fp = lock.Packs[packID].BuildFingerprint
		}
	}

	meta := releaseMeta{
		ID:               id,
		PackID:           packID,
		Version:          man.Version,
		CreatedAt:        createdAt.Format(time.RFC3339),
		Reason:           reason,
		Note:             strings.TrimSpace(note),
		BuildFingerprint: fp,
		HasUI:            hasUI,
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.RemoveAll(root)
		return ReleaseInfo{}, err
	}
	if err := os.WriteFile(filepath.Join(root, releaseMetaFile), append(b, '\n'), 0o600); err != nil {
		_ = os.RemoveAll(root)
		return ReleaseInfo{}, err
	}
	_ = m.pruneReleasesUnlocked(packID, maxPackReleases)

	active := false
	if lock, err := m.readLockUnlocked(); err == nil {
		active = lock.Packs[packID].ActiveReleaseID == id
	}
	return ReleaseInfo{
		ID:               meta.ID,
		PackID:           meta.PackID,
		Version:          meta.Version,
		CreatedAt:        meta.CreatedAt,
		Reason:           meta.Reason,
		Note:             meta.Note,
		BuildFingerprint: meta.BuildFingerprint,
		HasUI:            meta.HasUI,
		Active:           active,
		Path:             root,
	}, nil
}

func (m *Manager) pruneReleasesUnlocked(packID string, keep int) error {
	if keep <= 0 {
		keep = maxPackReleases
	}
	list, err := m.listReleasesUnlocked(packID)
	if err != nil || len(list) <= keep {
		return err
	}
	lock, _ := m.readLockUnlocked()
	active := ""
	if lock != nil {
		active = lock.Packs[packID].ActiveReleaseID
	}
	for i := keep; i < len(list); i++ {
		if list[i].ID == active {
			continue
		}
		_ = os.RemoveAll(list[i].Path)
	}
	return nil
}

func readReleaseMeta(root string) (releaseMeta, error) {
	b, err := os.ReadFile(filepath.Join(root, releaseMetaFile))
	if err != nil {
		return releaseMeta{}, err
	}
	var meta releaseMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return releaseMeta{}, err
	}
	if meta.ID == "" {
		meta.ID = filepath.Base(root)
	}
	return meta, nil
}

func sanitizeReleaseToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "0.0.0"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// copyTree 递归拷贝；skipDir 按目录名跳过（任一层）。
func copyTree(src, dst string, skipDir map[string]struct{}) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o700)
		}
		if d.IsDir() {
			if skipDir != nil {
				if _, ok := skipDir[d.Name()]; ok {
					return filepath.SkipDir
				}
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o700)
		}
		if skipDir != nil {
			// 跳过路径中含 node_modules 的文件（防御）
			parts := strings.Split(filepath.ToSlash(rel), "/")
			for _, p := range parts {
				if _, ok := skipDir[p]; ok {
					return nil
				}
			}
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
