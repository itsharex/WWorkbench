# WWorkbench 插件协议（平台 SSOT）

版本：`ww-cap 0.54`（与 AppVersion 的 major.minor 对齐；**破坏 WWHost 表面**时才升次版本）。

本文件由 Host 写入 `~/.wworkbench/plugins/PROTOCOL.md`，**不以某个 pack 为中心**。  
Agent：先 `get_plugin_protocol`，再改具体 pack；勿 `list_skills` 找 bridge。

## 1. 包形态

```
~/.wworkbench/plugins/
  PROTOCOL.md          ← 本文件（平台维护）
  lock.json
  <packId>/
    pack.yml           ← 清单（见 §2）
    HOST.md            ← 可选：该 pack 特有说明（指向本协议）
    README.md
    src/ui/            ← Vite 工程
~/.wworkbench/run/<packId>/ui/   ← 构建产物 IIFE
~/.wworkbench/releases/<packId>/<id>/  ← 快照
```

## 2. pack.yml

```yaml
name: database
version: 0.2.5                 # pack 自身 semver
requires:
  ww-cap: ">=0.54"             # 宿主契约下限（支持 >=X.Y / X.Y）
provides:
  capabilities:                # 禁用本 pack 时这些能力对 Agent 强制关闭
    - execute_sql
  products:
    - id: database
contributes:
  ui:
    product: database
    entry: ui/index.js         # 相对 run/<packId>/
```

Host 加载时校验字段完整性，并计算 `capOk`（不满足则前馈标明，不静默当可用）。

## 3. WWHost 运行时契约

Pack UI = IIFE，与 Host **共用一份 React**。

| 全局 | 来源 |
|------|------|
| `React` / `ReactDOM` | Host |
| `WWHost` | Host `installWWHost()` |
| `WWPack_<packId>` | `run/<packId>/ui/index.js` |

分层（唯一表面，TypeScript：`frontend/src/plugin/host.ts`）：

```ts
import { api, i18n, store, nav, workbench, ui, features, util } from '@wworkbench/host'
```

| 层 | 用途 |
|----|------|
| `api` | 连库 / SQL / 会话 |
| `i18n` | 文案 |
| `store` | 应用状态 / loading |
| `nav` | 打开终端 / SFTP / 笔记、Agent 草稿 |
| `workbench` | Capability / 资产雷达 |
| `ui` | 通用控件 / icons / pressProps |
| `features` | SqlEditor、ObjectTree… |
| `util` | 小工具 |

缺能力 → **扩展 Host `installWWHost`**，禁止把 React/Host 组件打进 pack。

## 4. 构建契约（vite）

- `formats: ['iife']`，`name: 'WWPack_<packId>'`（仅 `[A-Za-z0-9_]`）
- `external: ['react','react-dom','@wworkbench/host']`
- `globals: { react:'React', 'react-dom':'ReactDOM', '@wworkbench/host':'WWHost' }`

## 5. Agent 改 UI 流程

1. `get_plugin_protocol`（本文件）
2. `list_plugin_packs` / `get_plugin_pack`
3. `read_plugin_pack_file` — 大文件 `startLine` + `maxLines`，跟 `hasMore`/`nextStartLine`
4. **优先** `edit_plugin_pack_file(oldString,newString)`；仅新建/整文件重写才 `write_plugin_pack_file`
5. `build_plugin_pack_ui` → 自动 release 快照

禁止：用 `list_connections` / `execute_sql`「改插件界面」；禁止 `list_skills` 找 bridge。

## 6. 版本与回滚

- 每次成功 `build_plugin_pack_ui` → `releases/<packId>/<id>/`
- seed 升级前 / 回滚前自动快照
- `list_plugin_pack_releases` → `rollback_plugin_pack`

## 7. ww-cap 策略

| 变更 | 是否升 HostCap |
|------|----------------|
| 新增 WWHost 可选层/字段（旧 pack 仍能跑） | 可保持，建议文档注明 |
| 删除/改名层、改 external/globals、改加载方式 | **必须**升 major.minor（如 0.54→0.55） |
| 仅 App patch（修 bug） | 不升 HostCap |

pack 的 `requires.ww-cap: ">=0.54"` 表示：宿主 `HostCapVersion` 的 major.minor ≥ 0.54。
