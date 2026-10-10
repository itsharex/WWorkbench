# database pack 说明

**平台插件协议（SSOT）不在本文件。**

Agent / 开发者请先：

1. `get_plugin_protocol` → 读 `~/.wworkbench/plugins/PROTOCOL.md`（WWHost 分层、pack.yml、构建、ww-cap）
2. 再改本 pack 产品 UI：`src/ui/src/DatabaseWorkbench.tsx`（大文件用 `read_plugin_pack_file` 分段）

## 本 pack 要点

- `pack.yml`：`provides.capabilities` 含 execute_sql / list_tables 等；**禁用本 pack** 时这些能力对 Agent 强制不可用
- UI 全局名：`WWPack_database`
- 局部改：`edit_plugin_pack_file` → `build_plugin_pack_ui`

禁止：`list_skills` 找 bridge；禁止用 `execute_sql`「改插件显示」。
