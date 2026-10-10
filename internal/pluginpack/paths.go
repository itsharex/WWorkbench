package pluginpack

import "path/filepath"

// PluginsDir 可写 pack 源码根目录：~/.wworkbench/plugins。
func PluginsDir(dataDir string) string {
	return filepath.Join(dataDir, "plugins")
}

// RunDir 构建产物根目录：~/.wworkbench/run。
func RunDir(dataDir string) string {
	return filepath.Join(dataDir, "run")
}

// ReleasesDir 不可变发布快照根：~/.wworkbench/releases。
func ReleasesDir(dataDir string) string {
	return filepath.Join(dataDir, "releases")
}

// LockPath 启用与构建钉扎文件：~/.wworkbench/plugins/lock.json。
func LockPath(dataDir string) string {
	return filepath.Join(PluginsDir(dataDir), "lock.json")
}

// PackSourceDir 单个 pack 源码目录。
func PackSourceDir(dataDir, packID string) string {
	return filepath.Join(PluginsDir(dataDir), packID)
}

// PackRunDir 单个 pack 运行产物目录。
func PackRunDir(dataDir, packID string) string {
	return filepath.Join(RunDir(dataDir), packID)
}

// PackUIDir 单个 pack UI 构建产物目录。
func PackUIDir(dataDir, packID string) string {
	return filepath.Join(PackRunDir(dataDir, packID), "ui")
}

// PackReleasesDir 某 pack 的全部 release。
func PackReleasesDir(dataDir, packID string) string {
	return filepath.Join(ReleasesDir(dataDir), packID)
}

// PackReleaseDir 单个 release 目录。
func PackReleaseDir(dataDir, packID, releaseID string) string {
	return filepath.Join(PackReleasesDir(dataDir, packID), releaseID)
}

// PackManifestPath pack.yml 路径。
func PackManifestPath(dataDir, packID string) string {
	return filepath.Join(PackSourceDir(dataDir, packID), "pack.yml")
}

// PackUISourceDir pack UI 源码目录（src/ui）。
func PackUISourceDir(dataDir, packID string) string {
	return filepath.Join(PackSourceDir(dataDir, packID), "src", "ui")
}
