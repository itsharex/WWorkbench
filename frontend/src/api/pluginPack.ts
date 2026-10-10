/** 插件包 API 类型（与 Go pluginpack.* 对齐）。 */

export interface PluginPackInfo {
  id: string
  name: string
  version: string
  enabled: boolean
  sourcePath: string
  runPath: string
  uiPath: string
  uiBuilt: boolean
  uiEntry?: string
  uiFiles?: string[]
  buildFingerprint?: string
  builtAt?: string
  activeReleaseId?: string
  releaseCount?: number
  capabilities: string[]
  products: string[]
  requiresWwCap?: string
  hostCapVersion?: string
  capOk?: boolean
  capDetail?: string
  manifestIssues?: string[]
}

export interface PluginProtocolInfo {
  hostCapVersion: string
  path: string
  content: string
}

export interface PluginPackBuildResult {
  packId: string
  uiDir: string
  entry: string
  uiFiles?: string[]
  buildFingerprint: string
  builtAt: string
  releaseId?: string
}

export interface PluginPackReleaseInfo {
  id: string
  packId: string
  version: string
  createdAt: string
  reason: string
  note?: string
  buildFingerprint?: string
  hasUI: boolean
  active: boolean
  path: string
}
