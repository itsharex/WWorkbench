package app

import (
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"

	"WWorkbench/internal/errno"
	"WWorkbench/internal/pluginpack"
	"WWorkbench/internal/workbench"
)

// ListPluginPacks 列出已安装插件包。
func (s *Service) ListPluginPacks() ApiResult[[]pluginpack.PackInfo] {
	if s.pluginPacks == nil {
		return OkResult([]pluginpack.PackInfo{})
	}
	list, err := s.pluginPacks.List()
	if err != nil {
		return ErrResult[[]pluginpack.PackInfo](err)
	}
	if list == nil {
		list = []pluginpack.PackInfo{}
	}
	return OkResult(list)
}

// GetPluginPack 获取单个插件包摘要。
func (s *Service) GetPluginPack(packID string) ApiResult[pluginpack.PackInfo] {
	if s.pluginPacks == nil {
		return ErrResult[pluginpack.PackInfo](errPluginPackUnavailable)
	}
	info, err := s.pluginPacks.Get(packID)
	if err != nil {
		return ErrResult[pluginpack.PackInfo](err)
	}
	return OkResult(info)
}

// PluginProtocolDO 平台插件协议摘要（给前端/调试）。
type PluginProtocolDO struct {
	HostCapVersion string `json:"hostCapVersion"`
	Path           string `json:"path"`
	Content        string `json:"content"`
}

// GetPluginProtocol 读取平台 Host↔Pack 协议 SSOT。
func (s *Service) GetPluginProtocol() ApiResult[PluginProtocolDO] {
	if s.pluginPacks == nil {
		return ErrResult[PluginProtocolDO](errPluginPackUnavailable)
	}
	path, content, hostCap, err := s.pluginPacks.ReadProtocol()
	if err != nil {
		return ErrResult[PluginProtocolDO](err)
	}
	return OkResult(PluginProtocolDO{
		HostCapVersion: hostCap,
		Path:           path,
		Content:        content,
	})
}

// SetPluginPackEnabled 启用/禁用插件包。
func (s *Service) SetPluginPackEnabled(packID string, enabled bool) ApiResult[pluginpack.PackInfo] {
	if s.pluginPacks == nil {
		return ErrResult[pluginpack.PackInfo](errPluginPackUnavailable)
	}
	info, err := s.pluginPacks.SetEnabled(packID, enabled)
	if err != nil {
		return ErrResult[pluginpack.PackInfo](err)
	}
	if s.radar != nil {
		product := ""
		if len(info.Products) > 0 {
			product = info.Products[0]
		}
		label := info.ID + " disabled"
		if enabled {
			label = info.ID + " enabled"
		}
		s.radar.EmitAsset(workbench.RadarDomainPluginPack, product, workbench.RadarOpUpdate, info.ID, "", label, true)
	}
	return OkResult(info)
}

// GetPluginPackSourcePath 返回 pack 可写源码目录。
func (s *Service) GetPluginPackSourcePath(packID string) ApiResult[string] {
	if s.pluginPacks == nil {
		return ErrResult[string](errPluginPackUnavailable)
	}
	info, err := s.pluginPacks.Get(packID)
	if err != nil {
		return ErrResult[string](err)
	}
	return OkResult(info.SourcePath)
}

// BuildPluginPackUI 在 pack src/ui 执行 npm/vite，产物写入 run/<id>/ui。
func (s *Service) BuildPluginPackUI(packID string) ApiResult[pluginpack.BuildResult] {
	if s.pluginPacks == nil {
		return ErrResult[pluginpack.BuildResult](errPluginPackUnavailable)
	}
	result, err := s.pluginPacks.BuildUI(packID)
	if err != nil {
		return ErrResult[pluginpack.BuildResult](err)
	}
	if s.radar != nil {
		s.radar.EmitAsset(workbench.RadarDomainPluginPack, "", workbench.RadarOpUpdate, result.PackID, "", result.PackID+" UI built", true)
	}
	return OkResult(result)
}

// ListPluginPackReleases 列出 pack 历史版本（新→旧）。
func (s *Service) ListPluginPackReleases(packID string) ApiResult[[]pluginpack.ReleaseInfo] {
	if s.pluginPacks == nil {
		return OkResult([]pluginpack.ReleaseInfo{})
	}
	list, err := s.pluginPacks.ListReleases(packID)
	if err != nil {
		return ErrResult[[]pluginpack.ReleaseInfo](err)
	}
	if list == nil {
		list = []pluginpack.ReleaseInfo{}
	}
	return OkResult(list)
}

// SnapshotPluginPack 手动为当前工作区打快照。
func (s *Service) SnapshotPluginPack(packID string) ApiResult[pluginpack.ReleaseInfo] {
	if s.pluginPacks == nil {
		return ErrResult[pluginpack.ReleaseInfo](errPluginPackUnavailable)
	}
	info, err := s.pluginPacks.Snapshot(packID, "manual", "")
	if err != nil {
		return ErrResult[pluginpack.ReleaseInfo](err)
	}
	return OkResult(info)
}

// RollbackPluginPack 回滚到指定 release（源码 + UI）。
func (s *Service) RollbackPluginPack(packID, releaseID string) ApiResult[pluginpack.PackInfo] {
	if s.pluginPacks == nil {
		return ErrResult[pluginpack.PackInfo](errPluginPackUnavailable)
	}
	info, err := s.pluginPacks.Rollback(packID, releaseID)
	if err != nil {
		return ErrResult[pluginpack.PackInfo](err)
	}
	if s.radar != nil {
		product := ""
		if len(info.Products) > 0 {
			product = info.Products[0]
		}
		s.radar.EmitAsset(workbench.RadarDomainPluginPack, product, workbench.RadarOpUpdate, info.ID, "", info.ID+" rollback "+releaseID, true)
	}
	return OkResult(info)
}

// ReadPluginPackUIFile 读取 run/<id>/ui 下相对路径文件内容（供前端 blob ESM）。
func (s *Service) ReadPluginPackUIFile(packID, rel string) ApiResult[string] {
	if s.pluginPacks == nil {
		return ErrResult[string](errPluginPackUnavailable)
	}
	content, err := s.pluginPacks.ReadUIFile(packID, rel)
	if err != nil {
		return ErrResult[string](err)
	}
	return OkResult(content)
}

// RevealPluginPackPath 在系统文件管理器中打开路径（源码或产物目录）。
func (s *Service) RevealPluginPackPath(path string) ApiResult[bool] {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrResult[bool](errno.New(errno.CodeInvalidArg, "路径为空", ""))
	}
	if _, err := os.Stat(path); err != nil {
		return ErrResult[bool](errno.Wrap(errno.CodeInvalidArg, "路径不存在", err))
	}
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return ErrResult[bool](errno.Wrap(errno.CodeStoreFailed, "打开文件管理器失败", err))
	}
	return OkResult(true)
}

var errPluginPackUnavailable = &pluginPackError{msg: "plugin pack manager not initialized"}

type pluginPackError struct{ msg string }

func (e *pluginPackError) Error() string { return e.msg }
