package pluginpack

import "embed"

// seedFS 内置数据库 pack 源码（EnsureSeeded：同版本保留用户编辑，更高版本整包覆盖）。
//
//go:embed all:seed
var seedFS embed.FS
