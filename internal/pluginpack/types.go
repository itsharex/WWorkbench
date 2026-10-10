package pluginpack

// Manifest 对应 pack.yml。
type Manifest struct {
	Name     string            `yaml:"name" json:"name"`
	Version  string            `yaml:"version" json:"version"`
	Requires ManifestRequires  `yaml:"requires" json:"requires"`
	Provides ManifestProvides  `yaml:"provides" json:"provides"`
	Entry    string            `yaml:"entry,omitempty" json:"entry,omitempty"`
	Contributes ManifestContrib `yaml:"contributes" json:"contributes"`
}

// ManifestRequires 宿主能力约束。
type ManifestRequires struct {
	WWCap string `yaml:"ww-cap" json:"wwCap"`
}

// ManifestProvides pack 声明的能力与产品线。
type ManifestProvides struct {
	Capabilities []string         `yaml:"capabilities" json:"capabilities"`
	Products     []ManifestProduct `yaml:"products" json:"products"`
}

// ManifestProduct pack 贡献的产品线。
type ManifestProduct struct {
	ID string `yaml:"id" json:"id"`
}

// ManifestContrib pack 贡献物。
type ManifestContrib struct {
	UI *ManifestUI `yaml:"ui,omitempty" json:"ui,omitempty"`
}

// ManifestUI 前端 UI 贡献（构建产物相对 run/<id>/）。
type ManifestUI struct {
	Product string `yaml:"product" json:"product"`
	Entry   string `yaml:"entry" json:"entry"`
}

// LockFile ~/.wworkbench/plugins/lock.json。
type LockFile struct {
	Packs map[string]LockEntry `json:"packs"`
}

// LockEntry 单个 pack 的启用与构建钉扎。
type LockEntry struct {
	Enabled          bool   `json:"enabled"`
	Version          string `json:"version"`
	BuildFingerprint string `json:"buildFingerprint,omitempty"`
	BuiltAt          string `json:"builtAt,omitempty"`
	ActiveReleaseID  string `json:"activeReleaseId,omitempty"`
}

// PackInfo 前端/API 可见的 pack 摘要。
type PackInfo struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Enabled          bool     `json:"enabled"`
	SourcePath       string   `json:"sourcePath"`
	RunPath          string   `json:"runPath"`
	UIPath           string   `json:"uiPath"`
	UIBuilt          bool     `json:"uiBuilt"`
	UIEntry          string   `json:"uiEntry,omitempty"`
	UIFiles          []string `json:"uiFiles,omitempty"`
	BuildFingerprint string   `json:"buildFingerprint,omitempty"`
	BuiltAt          string   `json:"builtAt,omitempty"`
	ActiveReleaseID  string   `json:"activeReleaseId,omitempty"`
	ReleaseCount     int      `json:"releaseCount,omitempty"`
	Capabilities     []string `json:"capabilities"`
	Products         []string `json:"products"`
	RequiresWWCap  string   `json:"requiresWwCap,omitempty"`
	HostCapVersion string   `json:"hostCapVersion,omitempty"` // 宿主 ww-cap major.minor
	CapOK          bool     `json:"capOk"`                     // requires.ww-cap 是否满足
	CapDetail      string   `json:"capDetail,omitempty"`       // 提前说明兼容现状
	ManifestIssues []string `json:"manifestIssues,omitempty"`  // pack.yml 结构问题
}

// BuildResult UI 构建结果。
type BuildResult struct {
	PackID           string   `json:"packId"`
	UIDir            string   `json:"uiDir"`
	Entry            string   `json:"entry"`
	UIFiles          []string `json:"uiFiles,omitempty"`
	BuildFingerprint string   `json:"buildFingerprint"`
	BuiltAt          string   `json:"builtAt"`
	ReleaseID        string   `json:"releaseId,omitempty"`
}

// ReleaseInfo 一次不可变快照（源码 + 可选 UI）。
type ReleaseInfo struct {
	ID               string `json:"id"`
	PackID           string `json:"packId"`
	Version          string `json:"version"`
	CreatedAt        string `json:"createdAt"`
	Reason           string `json:"reason"`
	Note             string `json:"note,omitempty"`
	BuildFingerprint string `json:"buildFingerprint,omitempty"`
	HasUI            bool   `json:"hasUI"`
	Active           bool   `json:"active"`
	Path             string `json:"path"`
}

// releaseMeta 落盘 meta.json。
type releaseMeta struct {
	ID               string `json:"id"`
	PackID           string `json:"packId"`
	Version          string `json:"version"`
	CreatedAt        string `json:"createdAt"`
	Reason           string `json:"reason"`
	Note             string `json:"note,omitempty"`
	BuildFingerprint string `json:"buildFingerprint,omitempty"`
	HasUI            bool   `json:"hasUI"`
}
