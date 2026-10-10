import type { ComponentType } from 'react'
import type { WWHostAPI } from './host'

/** Pack UI 模块约定：default 导出产品视图组件。 */
export type PackUIModule = {
  default?: ComponentType
  [key: string]: unknown
}

/** packGlobalName 与 vite lib.name 对齐。 */
export function packGlobalName(packId: string): string {
  return `WWPack_${packId.replace(/[^a-zA-Z0-9_]/g, '_')}`
}

/**
 * Pack 构建契约（平台 SSOT：internal/pluginpack/PROTOCOL.md / get_plugin_protocol）：
 * - formats: ['iife']，name = packGlobalName(id)
 * - external: ['react', 'react-dom', '@wworkbench/host']
 * - globals: { react:'React', 'react-dom':'ReactDOM', '@wworkbench/host':'WWHost' }
 * - Host 表面分层：api / i18n / store / nav / workbench / ui / features / util
 * - ww-cap：HostCapVersion 0.54；pack.yml requires.ww-cap: ">=0.54"
 */
export type PackBuildContract = {
  reactGlobal: 'React'
  reactDomGlobal: 'ReactDOM'
  hostModule: '@wworkbench/host'
  hostGlobal: 'WWHost'
  hostLayers: Array<keyof WWHostAPI>
}

export type { WWHostAPI }

declare global {
  // eslint-disable-next-line no-var
  var WWHost: WWHostAPI | undefined
}

export {}
