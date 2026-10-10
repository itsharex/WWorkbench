package turnctx

import (
	"strings"

	"WWorkbench/internal/model"
)

// ProductDefaultPackID 产品线默认绑定的可写插件包（无则空）。
func ProductDefaultPackID(activeProduct string) string {
	switch strings.TrimSpace(activeProduct) {
	case "database":
		return "database"
	default:
		return ""
	}
}

// PluginPackLookup 解析 pack 工作目录。
type PluginPackLookup func(packID string) (sourcePath, runPath string, uiBuilt bool, ok bool)

// AttachPluginPack 把当前产品线对应的插件工作目录写入上下文（供 Gather 前馈）。
// 前端可只传 pluginPackId；路径由 Host 解析，避免 Agent 猜 ~/workspace/...。
func AttachPluginPack(ctx model.AgentContextDO, lookup PluginPackLookup) model.AgentContextDO {
	packID := strings.TrimSpace(ctx.PluginPackID)
	if packID == "" {
		packID = ProductDefaultPackID(ctx.ActiveProduct)
	}
	if packID == "" {
		return ctx
	}
	ctx.PluginPackID = packID
	if lookup == nil {
		return ctx
	}
	src, run, built, ok := lookup(packID)
	if !ok {
		return ctx
	}
	if strings.TrimSpace(src) != "" {
		ctx.PluginSourcePath = src
	}
	if strings.TrimSpace(run) != "" {
		ctx.PluginRunPath = run
	}
	ctx.PluginUIBuilt = built
	return ctx
}
