package workbenchtools

import (
	"context"
	"encoding/json"
	"strings"

	"WWorkbench/internal/workbench"

	"github.com/google/uuid"
)

func pluginPacksOrFail(d *Deps) ToolResult {
	if d == nil || d.PluginPacks == nil {
		return Fail("插件包管理器未就绪")
	}
	return ToolResult{OK: true}
}

func toolListPluginPacks(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	_ = raw
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	list, err := d.PluginPacks.List()
	if err != nil {
		return Fail(err.Error())
	}
	return OKData(list)
}

type packIDArgs struct {
	PackID string `json:"packId"`
}

func toolGetPluginPack(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packIDArgs
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.PackID) == "" {
		return Fail("需要 packId（如 database）")
	}
	info, err := d.PluginPacks.Get(strings.TrimSpace(in.PackID))
	if err != nil {
		return Fail(err.Error())
	}
	return OKData(info)
}

func toolGetPluginProtocol(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	_ = raw
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	path, content, hostCap, err := d.PluginPacks.ReadProtocol()
	if err != nil {
		return Fail(err.Error())
	}
	return OKData(map[string]interface{}{
		"hostCapVersion": hostCap,
		"path":           path,
		"content":        content,
		"hint":           "改任何 pack UI 前先读本协议；各 pack 的 HOST.md 只是补充，不是 SSOT",
	})
}

func toolListPluginPackFiles(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packIDArgs
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.PackID) == "" {
		return Fail("需要 packId（如 database）")
	}
	files, err := d.PluginPacks.ListSourceFiles(strings.TrimSpace(in.PackID))
	if err != nil {
		return Fail(err.Error())
	}
	return OKData(map[string]interface{}{
		"packId":     strings.TrimSpace(in.PackID),
		"files":      files,
		"uiHint":     "协议 get_plugin_protocol；本 pack 补充 HOST.md；产品页 src/ui/src/DatabaseWorkbench.tsx",
		"sourceRoot": "相对 ~/.wworkbench/plugins/" + strings.TrimSpace(in.PackID),
	})
}

type packFileArgs struct {
	PackID    string `json:"packId"`
	RelPath   string `json:"relPath"`
	Content   string `json:"content"`
	StartLine int    `json:"startLine"`
	MaxLines  int    `json:"maxLines"`
}

func toolReadPluginPackFile(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packFileArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	rel := strings.TrimSpace(in.RelPath)
	if packID == "" || rel == "" {
		return Fail("需要 packId 与 relPath（相对 pack 源码根，如 src/ui/src/DatabaseWorkbench.tsx）")
	}
	slice, err := d.PluginPacks.ReadSourceSlice(packID, rel, in.StartLine, in.MaxLines)
	if err != nil {
		return Fail(err.Error())
	}
	out := map[string]interface{}{
		"packId":     packID,
		"relPath":    rel,
		"absPath":    slice.AbsPath,
		"content":    slice.Content,
		"startLine":  slice.StartLine,
		"endLine":    slice.EndLine,
		"totalLines": slice.TotalLines,
		"hasMore":    slice.HasMore,
	}
	if slice.HasMore {
		out["nextStartLine"] = slice.NextStartLine
		out["hint"] = "文件未读完；继续调用 read_plugin_pack_file，传入 startLine=nextStartLine（可选 maxLines，默认220，最大400）。不要用 recall_resource 拼全文。"
	}
	return OKData(out)
}

type packEditArgs struct {
	PackID      string `json:"packId"`
	RelPath     string `json:"relPath"`
	OldString   string `json:"oldString"`
	NewString   string `json:"newString"`
	ReplaceAll  bool   `json:"replaceAll"`
}

func toolEditPluginPackFile(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packEditArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	rel := strings.TrimSpace(in.RelPath)
	if packID == "" || rel == "" {
		return Fail("需要 packId 与 relPath")
	}
	if in.OldString == "" {
		return Fail("oldString 不能为空（须是文件中的原文片段）")
	}
	preview := map[string]interface{}{
		"packId":     packID,
		"relPath":    rel,
		"replaceAll": in.ReplaceAll,
		"oldHead":    truncate(in.OldString, 160),
		"newHead":    truncate(in.NewString, 160),
		"oldBytes":   len(in.OldString),
		"newBytes":   len(in.NewString),
	}
	return Confirm(uuid.NewString(), "局部编辑插件源码 "+packID+"/"+rel, preview)
}

// EditPluginPackFileConfirmed 用户确认后对 pack 源码做精确替换。
func EditPluginPackFileConfirmed(ctx context.Context, d *Deps, raw string) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packEditArgs
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	rel := strings.TrimSpace(in.RelPath)
	res, err := d.PluginPacks.PatchSourceFile(packID, rel, in.OldString, in.NewString, in.ReplaceAll)
	if err != nil {
		return Fail(err.Error())
	}
	if d.Radar != nil {
		d.Radar.EmitAsset(workbench.RadarDomainPluginPack, "", workbench.RadarOpUpdate, packID, "", packID+" 源码已局部编辑", false)
	}
	return OKData(map[string]interface{}{
		"ok":           true,
		"packId":       packID,
		"relPath":      rel,
		"absPath":      res.AbsPath,
		"replacements": res.Replacements,
		"bytesBefore":  res.BytesBefore,
		"bytesAfter":   res.BytesAfter,
		"next":         "再调用 build_plugin_pack_ui 才能在界面看到变化",
	})
}

func toolWritePluginPackFile(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packFileArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	rel := strings.TrimSpace(in.RelPath)
	if packID == "" || rel == "" {
		return Fail("需要 packId 与 relPath")
	}
	if in.Content == "" {
		return Fail("content 不能为空")
	}
	preview := map[string]interface{}{
		"packId":  packID,
		"relPath": rel,
		"bytes":   len(in.Content),
		"head":    truncate(in.Content, 240),
		"hint":    "大文件局部改请用 edit_plugin_pack_file（oldString/newString）",
	}
	return Confirm(uuid.NewString(), "整文件写入插件源码 "+packID+"/"+rel, preview)
}

// WritePluginPackFileConfirmed 用户确认后写入 pack 源码。
func WritePluginPackFileConfirmed(ctx context.Context, d *Deps, raw string) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packFileArgs
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	rel := strings.TrimSpace(in.RelPath)
	abs, err := d.PluginPacks.WriteSourceFile(packID, rel, in.Content)
	if err != nil {
		return Fail(err.Error())
	}
	if d.Radar != nil {
		d.Radar.EmitAsset(workbench.RadarDomainPluginPack, "", workbench.RadarOpUpdate, packID, "", packID+" 源码已改", false)
	}
	return OKData(map[string]interface{}{
		"ok":      true,
		"packId":  packID,
		"relPath": rel,
		"absPath": abs,
		"next":    "再调用 build_plugin_pack_ui 才能在界面看到变化",
	})
}

func toolBuildPluginPackUI(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packIDArgs
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.PackID) == "" {
		return Fail("需要 packId")
	}
	packID := strings.TrimSpace(in.PackID)
	return Confirm(uuid.NewString(), "构建插件 UI "+packID+"（npm/vite，写入 ~/.wworkbench/run/"+packID+"/ui）", map[string]interface{}{
		"packId": packID,
	})
}

// BuildPluginPackUIConfirmed 用户确认后构建 pack UI。
func BuildPluginPackUIConfirmed(ctx context.Context, d *Deps, raw string) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packIDArgs
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	result, err := d.PluginPacks.BuildUI(packID)
	if err != nil {
		return Fail(err.Error())
	}
	if d.Radar != nil {
		product := "database"
		if packID != "database" {
			product = "plugins"
		}
		d.Radar.EmitAsset(workbench.RadarDomainPluginPack, product, workbench.RadarOpUpdate, packID, "", packID+" UI 已构建", true)
	}
	return OKData(result)
}

type rollbackArgs struct {
	PackID    string `json:"packId"`
	ReleaseID string `json:"releaseId"`
}

func toolListPluginPackReleases(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in packIDArgs
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.PackID) == "" {
		return Fail("需要 packId")
	}
	list, err := d.PluginPacks.ListReleases(strings.TrimSpace(in.PackID))
	if err != nil {
		return Fail(err.Error())
	}
	return OKData(map[string]interface{}{
		"releases": list,
		"hint":     "回滚用 rollback_plugin_pack(packId, releaseId)；构建会自动打快照",
	})
}

func toolRollbackPluginPack(ctx context.Context, d *Deps, raw json.RawMessage) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in rollbackArgs
	if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.PackID) == "" || strings.TrimSpace(in.ReleaseID) == "" {
		return Fail("需要 packId 与 releaseId（先 list_plugin_pack_releases）")
	}
	packID := strings.TrimSpace(in.PackID)
	releaseID := strings.TrimSpace(in.ReleaseID)
	return Confirm(uuid.NewString(), "回滚插件 "+packID+" → "+releaseID+"（先快照当前，再恢复源码/UI）", map[string]interface{}{
		"packId":    packID,
		"releaseId": releaseID,
	})
}

// RollbackPluginPackConfirmed 用户确认后回滚。
func RollbackPluginPackConfirmed(ctx context.Context, d *Deps, raw string) ToolResult {
	_ = ctx
	if r := pluginPacksOrFail(d); !r.OK {
		return r
	}
	var in rollbackArgs
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return Fail("参数无效")
	}
	packID := strings.TrimSpace(in.PackID)
	releaseID := strings.TrimSpace(in.ReleaseID)
	info, err := d.PluginPacks.Rollback(packID, releaseID)
	if err != nil {
		return Fail(err.Error())
	}
	if d.Radar != nil {
		product := ""
		if len(info.Products) > 0 {
			product = info.Products[0]
		}
		d.Radar.EmitAsset(workbench.RadarDomainPluginPack, product, workbench.RadarOpUpdate, packID, "", packID+" 已回滚 "+releaseID, true)
	}
	return OKData(map[string]interface{}{
		"pack": info,
		"next": "界面应已切换；若 UI 快照缺失则需再 build_plugin_pack_ui",
	})
}
