package pluginpack

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// HostCapVersion 宿主 WWHost 契约版本（major.minor）。
// 与 AppVersion 的 major.minor 对齐；破坏性变更才递增。
const HostCapVersion = "0.54"

// ProtocolFileName 平台协议落盘文件名（plugins 根下）。
const ProtocolFileName = "PROTOCOL.md"

//go:embed PROTOCOL.md
var protocolFS embed.FS

// ProtocolPath ~/.wworkbench/plugins/PROTOCOL.md。
func ProtocolPath(dataDir string) string {
	return filepath.Join(PluginsDir(dataDir), ProtocolFileName)
}

// ProtocolText 返回内嵌协议正文。
func ProtocolText() (string, error) {
	b, err := protocolFS.ReadFile(ProtocolFileName)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EnsureProtocolFile 把平台 PROTOCOL.md 写入 plugins 根（每次覆盖，属 Host 资产）。
func (m *Manager) EnsureProtocolFile() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureProtocolUnlocked()
}

func (m *Manager) ensureProtocolUnlocked() error {
	text, err := ProtocolText()
	if err != nil {
		return err
	}
	root := PluginsDir(m.dataDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	path := ProtocolPath(m.dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadProtocol 读落盘协议；若缺失则回退内嵌并尝试落盘。
func (m *Manager) ReadProtocol() (path string, content string, hostCap string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path = ProtocolPath(m.dataDir)
	b, readErr := os.ReadFile(path)
	if readErr != nil {
		text, e := ProtocolText()
		if e != nil {
			return path, "", HostCapVersion, e
		}
		_ = m.ensureProtocolUnlocked()
		return path, text, HostCapVersion, nil
	}
	return path, string(b), HostCapVersion, nil
}

// ManifestIssues pack.yml 结构问题（空则结构合格）。
func ManifestIssues(man Manifest, packID string) []string {
	var issues []string
	id := strings.TrimSpace(packID)
	if strings.TrimSpace(man.Name) == "" && id == "" {
		issues = append(issues, "name 不能为空")
	}
	if strings.TrimSpace(man.Version) == "" {
		issues = append(issues, "version 不能为空")
	}
	if man.Contributes.UI != nil {
		ui := man.Contributes.UI
		if strings.TrimSpace(ui.Product) == "" {
			issues = append(issues, "contributes.ui.product 不能为空")
		}
		if strings.TrimSpace(ui.Entry) == "" {
			issues = append(issues, "contributes.ui.entry 不能为空")
		}
	}
	for i, c := range man.Provides.Capabilities {
		if strings.TrimSpace(c) == "" {
			issues = append(issues, fmt.Sprintf("provides.capabilities[%d] 为空", i))
		}
	}
	return issues
}

// CapCompat 宿主契约与 pack requires.ww-cap 是否匹配。
func CapCompat(required string) (ok bool, detail string) {
	req := strings.TrimSpace(required)
	if req == "" {
		return true, "未声明 requires.ww-cap（默认兼容）"
	}
	ok, detail = satisfyWWCap(req, HostCapVersion)
	return ok, detail
}

// satisfyWWCap 支持 ">=X.Y" / "^X.Y" / "X.Y"（精确 major.minor）。
func satisfyWWCap(constraint, host string) (bool, string) {
	c := strings.TrimSpace(constraint)
	h := strings.TrimSpace(host)
	switch {
	case strings.HasPrefix(c, ">="):
		min := strings.TrimSpace(c[2:])
		if compareSemver(h, min) >= 0 {
			return true, fmt.Sprintf("host %s 满足 %s", h, c)
		}
		return false, fmt.Sprintf("host ww-cap=%s 不满足 pack 要求 %s", h, c)
	case strings.HasPrefix(c, "^"):
		base := strings.TrimSpace(c[1:])
		hp, bp := semverParts(h), semverParts(base)
		if len(hp) > 0 && len(bp) > 0 && hp[0] == bp[0] && compareSemver(h, base) >= 0 {
			return true, fmt.Sprintf("host %s 满足 %s", h, c)
		}
		return false, fmt.Sprintf("host ww-cap=%s 不满足 pack 要求 %s", h, c)
	default:
		// 精确到 major.minor（忽略 patch）
		if majorMinor(h) == majorMinor(c) {
			return true, fmt.Sprintf("host %s 匹配 %s", h, c)
		}
		return false, fmt.Sprintf("host ww-cap=%s 与 pack 要求 %s 不一致", h, c)
	}
}

func majorMinor(v string) string {
	p := semverParts(v)
	if len(p) == 0 {
		return strings.TrimSpace(v)
	}
	maj := strconv.Itoa(p[0])
	min := "0"
	if len(p) > 1 {
		min = strconv.Itoa(p[1])
	}
	return maj + "." + min
}
