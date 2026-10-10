package pluginpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Manager 管理用户 plugins 目录、lock 与 seed。
type Manager struct {
	dataDir string
	mu      sync.Mutex
}

// NewManager 创建插件包管理器。
func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

// DataDir 返回宿主数据目录。
func (m *Manager) DataDir() string {
	return m.dataDir
}

// EnsureSeeded 将内置 seed 拷到用户 plugins 目录。
// 同版本不覆盖用户编辑；seed 版本更高时整包覆盖并清空旧 UI 产物（平台升级）。
func (m *Manager) EnsureSeeded() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(PluginsDir(m.dataDir), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(RunDir(m.dataDir), 0o700); err != nil {
		return err
	}
	// 平台协议 SSOT：与 seed 无关，每次确保落盘
	if err := m.ensureProtocolUnlocked(); err != nil {
		return fmt.Errorf("protocol: %w", err)
	}

	entries, err := fs.ReadDir(seedFS, "seed")
	if err != nil {
		return fmt.Errorf("read seed: %w", err)
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return err
	}
	changed := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		packID := e.Name()
		seedMan, err := readSeedManifest(packID)
		if err != nil {
			return fmt.Errorf("seed manifest %s: %w", packID, err)
		}
		dst := PackSourceDir(m.dataDir, packID)
		manifest := filepath.Join(dst, "pack.yml")
		needCopy := false
		if _, err := os.Stat(manifest); err != nil {
			needCopy = true
		} else if installed, err := m.readManifestUnlocked(packID); err == nil {
			if compareSemver(seedMan.Version, installed.Version) > 0 {
				needCopy = true
			}
		} else {
			needCopy = true
		}
		if needCopy {
			// 升级前快照，便于回滚用户改过的版本
			if _, err := os.Stat(manifest); err == nil {
				if snap, err := m.snapshotUnlocked(packID, reasonPreUpgrade, "before seed "+seedMan.Version); err == nil {
					prev := lock.Packs[packID]
					prev.ActiveReleaseID = snap.ID
					lock.Packs[packID] = prev
				}
			}
			_ = os.RemoveAll(dst)
			if err := copySeedPack(packID, dst); err != nil {
				return fmt.Errorf("seed pack %s: %w", packID, err)
			}
			_ = os.RemoveAll(PackUIDir(m.dataDir, packID))
			prev, had := lock.Packs[packID]
			entry := LockEntry{Enabled: true, Version: seedMan.Version}
			if had {
				entry.Enabled = prev.Enabled
				// 新 seed 尚未构建；保留历史 active 仅作回滚指针无意义，清空
				entry.ActiveReleaseID = ""
			}
			lock.Packs[packID] = entry
			changed = true
			continue
		}
		if _, ok := lock.Packs[packID]; !ok {
			lock.Packs[packID] = LockEntry{Enabled: true, Version: seedMan.Version}
			changed = true
		}
	}
	if changed {
		return m.writeLockUnlocked(lock)
	}
	return nil
}

// List 列出已安装 pack。
func (m *Manager) List() ([]PackInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	root := PluginsDir(m.dataDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []PackInfo{}, nil
		}
		return nil, err
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return nil, err
	}
	var out []PackInfo
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := m.packInfoUnlocked(e.Name(), lock)
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get 获取单个 pack。
func (m *Manager) Get(packID string) (PackInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lock, err := m.readLockUnlocked()
	if err != nil {
		return PackInfo{}, err
	}
	return m.packInfoUnlocked(packID, lock)
}

// ReadManifest 读取 pack.yml。
func (m *Manager) ReadManifest(packID string) (Manifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readManifestUnlocked(packID)
}

// GatePermissions 将「由任一 pack 声明的能力」与启用 pack 对齐：
// 某能力若出现在任意已安装 pack 的 provides.capabilities 中，则仅当至少有一个
// 启用 pack 提供它时才保持 base 中的 true；否则强制 false。
// Host 原生能力（从未被 pack 声明）不受影响。
func (m *Manager) GatePermissions(base map[string]bool) map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	declared := map[string]struct{}{}
	enabled := map[string]struct{}{}
	root := PluginsDir(m.dataDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return base
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return base
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		man, err := m.readManifestUnlocked(e.Name())
		if err != nil {
			continue
		}
		packOn := true
		if ent, ok := lock.Packs[e.Name()]; ok {
			packOn = ent.Enabled
		}
		for _, cap := range man.Provides.Capabilities {
			cap = strings.TrimSpace(cap)
			if cap == "" {
				continue
			}
			declared[cap] = struct{}{}
			if packOn {
				enabled[cap] = struct{}{}
			}
		}
	}
	if len(declared) == 0 {
		return base
	}
	out := make(map[string]bool, len(base))
	for k, v := range base {
		out[k] = v
	}
	for cap := range declared {
		if _, ok := enabled[cap]; !ok {
			out[cap] = false
		}
	}
	return out
}

// SetEnabled 启用/禁用 pack。
func (m *Manager) SetEnabled(packID string, enabled bool) (PackInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	man, err := m.readManifestUnlocked(packID)
	if err != nil {
		return PackInfo{}, err
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return PackInfo{}, err
	}
	entry := lock.Packs[packID]
	entry.Enabled = enabled
	if entry.Version == "" {
		entry.Version = man.Version
	}
	lock.Packs[packID] = entry
	if err := m.writeLockUnlocked(lock); err != nil {
		return PackInfo{}, err
	}
	return m.packInfoUnlocked(packID, lock)
}

// RecordBuild 写入构建指纹到 lock。
func (m *Manager) RecordBuild(packID, fingerprint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	man, err := m.readManifestUnlocked(packID)
	if err != nil {
		return err
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return err
	}
	entry := lock.Packs[packID]
	entry.Version = man.Version
	entry.BuildFingerprint = fingerprint
	entry.BuiltAt = time.Now().UTC().Format(time.RFC3339)
	if _, ok := lock.Packs[packID]; !ok {
		entry.Enabled = true
	}
	lock.Packs[packID] = entry
	return m.writeLockUnlocked(lock)
}

// BuildUI 在 pack src/ui 执行 npm/vite 构建，产物写入 run/<id>/ui。
func (m *Manager) BuildUI(packID string) (BuildResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	man, err := m.readManifestUnlocked(packID)
	if err != nil {
		return BuildResult{}, err
	}
	uiSrc := PackUISourceDir(m.dataDir, packID)
	if st, err := os.Stat(uiSrc); err != nil || !st.IsDir() {
		return BuildResult{}, fmt.Errorf("pack %s has no src/ui", packID)
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return BuildResult{}, fmt.Errorf("npm not found: install Node.js to build pack UI")
	}

	uiOut := PackUIDir(m.dataDir, packID)
	if err := os.MkdirAll(uiOut, 0o700); err != nil {
		return BuildResult{}, err
	}
	// 清空旧产物
	if entries, err := os.ReadDir(uiOut); err == nil {
		for _, e := range entries {
			_ = os.RemoveAll(filepath.Join(uiOut, e.Name()))
		}
	}

	install := exec.Command("npm", "install")
	install.Dir = uiSrc
	install.Env = os.Environ()
	if out, err := install.CombinedOutput(); err != nil {
		return BuildResult{}, fmt.Errorf("npm install: %w\n%s", err, truncate(string(out), 4000))
	}

	// 相对路径给 vite，避免跨盘符问题
	relOut, err := filepath.Rel(uiSrc, uiOut)
	if err != nil {
		relOut = uiOut
	}
	build := exec.Command("npm", "run", "build", "--", "--outDir", relOut, "--emptyOutDir")
	build.Dir = uiSrc
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		return BuildResult{}, fmt.Errorf("npm run build: %w\n%s", err, truncate(string(out), 4000))
	}
	if err := assertPackUIDoesNotBundleReact(uiOut); err != nil {
		return BuildResult{}, err
	}

	fp, err := fingerprintDir(uiOut)
	if err != nil {
		return BuildResult{}, err
	}
	builtAt := time.Now().UTC().Format(time.RFC3339)
	releaseID := ""
	if snap, err := m.snapshotUnlocked(packID, reasonBuild, ""); err == nil {
		releaseID = snap.ID
	}
	lock, err := m.readLockUnlocked()
	if err != nil {
		return BuildResult{}, err
	}
	entry := lock.Packs[packID]
	entry.Version = man.Version
	entry.BuildFingerprint = fp
	entry.BuiltAt = builtAt
	if releaseID != "" {
		entry.ActiveReleaseID = releaseID
	}
	if _, ok := lock.Packs[packID]; !ok {
		entry.Enabled = true
	}
	lock.Packs[packID] = entry
	if err := m.writeLockUnlocked(lock); err != nil {
		return BuildResult{}, err
	}

	entryPath := "index.js"
	if man.Contributes.UI != nil && man.Contributes.UI.Entry != "" {
		entryPath = filepath.Base(man.Contributes.UI.Entry)
	}
	files, _ := listUIFiles(uiOut)
	return BuildResult{
		PackID:           packID,
		UIDir:            uiOut,
		Entry:            entryPath,
		UIFiles:          files,
		BuildFingerprint: fp,
		BuiltAt:          builtAt,
		ReleaseID:        releaseID,
	}, nil
}

// ReadUIFile 读取 run/<id>/ui 下相对路径文件内容。
func (m *Manager) ReadUIFile(packID, rel string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := filepath.Clean("/" + strings.ReplaceAll(rel, "\\", "/"))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." || strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid ui path")
	}
	full := filepath.Join(PackUIDir(m.dataDir, packID), filepath.FromSlash(clean))
	root := PackUIDir(m.dataDir, packID)
	relCheck, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		return "", fmt.Errorf("ui path escapes pack run dir")
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HasUIBuild 是否已有可用 UI 构建产物。
func (m *Manager) HasUIBuild(packID string) bool {
	entry := "index.js"
	if man, err := m.ReadManifest(packID); err == nil && man.Contributes.UI != nil && man.Contributes.UI.Entry != "" {
		entry = filepath.Base(man.Contributes.UI.Entry)
	}
	_, err := os.Stat(filepath.Join(PackUIDir(m.dataDir, packID), entry))
	return err == nil
}

func (m *Manager) packInfoUnlocked(packID string, lock *LockFile) (PackInfo, error) {
	man, err := m.readManifestUnlocked(packID)
	if err != nil {
		return PackInfo{}, err
	}
	entry, ok := lock.Packs[packID]
	enabled := true
	if ok {
		enabled = entry.Enabled
	}
	products := make([]string, 0, len(man.Provides.Products))
	for _, p := range man.Provides.Products {
		products = append(products, p.ID)
	}
	uiEntry := ""
	if man.Contributes.UI != nil {
		uiEntry = man.Contributes.UI.Entry
	}
	entryFile := "index.js"
	if uiEntry != "" {
		entryFile = filepath.Base(uiEntry)
	}
	uiDir := PackUIDir(m.dataDir, packID)
	uiBuilt := false
	if _, err := os.Stat(filepath.Join(uiDir, entryFile)); err == nil {
		uiBuilt = true
	}
	var uiFiles []string
	if uiBuilt {
		uiFiles, _ = listUIFiles(uiDir)
	}
	capOK, capDetail := CapCompat(man.Requires.WWCap)
	return PackInfo{
		ID:               packID,
		Name:             man.Name,
		Version:          man.Version,
		Enabled:          enabled,
		SourcePath:       PackSourceDir(m.dataDir, packID),
		RunPath:          PackRunDir(m.dataDir, packID),
		UIPath:           uiDir,
		UIBuilt:          uiBuilt,
		UIEntry:          uiEntry,
		UIFiles:          uiFiles,
		BuildFingerprint: entry.BuildFingerprint,
		BuiltAt:          entry.BuiltAt,
		ActiveReleaseID:  entry.ActiveReleaseID,
		ReleaseCount:     m.countReleasesUnlocked(packID),
		Capabilities:     man.Provides.Capabilities,
		Products:         products,
		RequiresWWCap:    man.Requires.WWCap,
		HostCapVersion:   HostCapVersion,
		CapOK:            capOK,
		CapDetail:        capDetail,
		ManifestIssues:   ManifestIssues(man, packID),
	}, nil
}

// listUIFiles 列出 UI 产物文件名（仅一层）。
func listUIFiles(uiDir string) ([]string, error) {
	entries, err := os.ReadDir(uiDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

func (m *Manager) readManifestUnlocked(packID string) (Manifest, error) {
	path := PackManifestPath(m.dataDir, packID)
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read pack.yml: %w", err)
	}
	var man Manifest
	if err := yaml.Unmarshal(b, &man); err != nil {
		return Manifest{}, fmt.Errorf("parse pack.yml: %w", err)
	}
	if man.Name == "" {
		man.Name = packID
	}
	return man, nil
}

func (m *Manager) readLockUnlocked() (*LockFile, error) {
	path := LockPath(m.dataDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LockFile{Packs: map[string]LockEntry{}}, nil
		}
		return nil, err
	}
	var lock LockFile
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("parse lock.json: %w", err)
	}
	if lock.Packs == nil {
		lock.Packs = map[string]LockEntry{}
	}
	return &lock, nil
}

func (m *Manager) writeLockUnlocked(lock *LockFile) error {
	if lock.Packs == nil {
		lock.Packs = map[string]LockEntry{}
	}
	b, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	path := LockPath(m.dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readSeedManifest(packID string) (Manifest, error) {
	path := filepath.ToSlash(filepath.Join("seed", packID, "pack.yml"))
	b, err := fs.ReadFile(seedFS, path)
	if err != nil {
		return Manifest{}, err
	}
	var man Manifest
	if err := yaml.Unmarshal(b, &man); err != nil {
		return Manifest{}, err
	}
	if man.Name == "" {
		man.Name = packID
	}
	return man, nil
}

// compareSemver 比较 a 与 b；a>b 返回 1，相等 0，a<b -1。非严格 semver 时按点分数字比较。
func compareSemver(a, b string) int {
	pa := semverParts(a)
	pb := semverParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var va, vb int
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

func semverParts(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	segs := strings.Split(v, ".")
	out := make([]int, 0, len(segs))
	for _, s := range segs {
		n := 0
		for _, r := range s {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}

func copySeedPack(packID, dst string) error {
	prefix := filepath.ToSlash(filepath.Join("seed", packID))
	return fs.WalkDir(seedFS, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(prefix, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o700)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		in, err := seedFS.Open(path)
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

func fingerprintDir(dir string) (string, error) {
	h := sha256.New()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, path := range files {
		rel, _ := filepath.Rel(dir, path)
		_, _ = io.WriteString(h, rel)
		_, _ = h.Write([]byte{0})
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write(b)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// assertPackUIDoesNotBundleReact 防止 pack 打进第二份 React（Host 渲染 hooks 会崩溃）。
func assertPackUIDoesNotBundleReact(uiDir string) error {
	entries, err := os.ReadDir(uiDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".js") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(uiDir, e.Name()))
		if err != nil {
			return err
		}
		body := string(b)
		if strings.Contains(body, "react.production") || strings.Contains(body, "react.development") {
			return fmt.Errorf(
				"pack UI %s 打进了 React 本体；vite.config 须 external:['react','react-dom'] 且 globals 指向 Host 的 window.React（见 seed vite.config.ts）",
				e.Name(),
			)
		}
	}
	return nil
}
