import { api } from '../api/client'
import { packGlobalName, type PackUIModule } from './sdk'

export type LoadPackUIResult =
  | { ok: true; mod: PackUIModule }
  | { ok: false; error: string }

/**
 * loadPackUI 读取 run/<id>/ui 产物并以经典 script 注入（IIFE）。
 * WKWebView 对 blob: ESM import() 不可靠；对 globalThis 上由 var 创建的属性也不能 delete。
 */
export async function loadPackUI(packId: string, entry = 'index.js'): Promise<LoadPackUIResult> {
  try {
    if (typeof globalThis.WWHost === 'undefined') {
      return { ok: false, error: 'WWHost 未注入（Host installWWHost 未执行）' }
    }
    const info = await api.getPluginPack(packId)
    if (!info.enabled) {
      return { ok: false, error: `pack ${packId} disabled` }
    }
    if (!info.uiBuilt) {
      return { ok: false, error: `pack ${packId} UI not built` }
    }
    let js = await api.readPluginPackUIFile(packId, entry)
    if (!js || !js.trim()) {
      return { ok: false, error: `empty UI file: ${entry}` }
    }
    js = js.replace(/\/\/# sourceMappingURL=.*$/gm, '')

    const globalName = packGlobalName(packId)
    // 勿 delete：IIFE 的 var 挂在 globalThis 上通常不可配置，delete 会抛 Unable to delete property。
    // 重新执行脚本即可覆盖同名绑定。
    const script = document.createElement('script')
    script.type = 'text/javascript'
    script.setAttribute('data-ww-pack', packId)
    script.text = js
    document.documentElement.appendChild(script)
    script.remove()

    const exported = (globalThis as Record<string, unknown>)[globalName]
    if (exported == null) {
      return {
        ok: false,
        error: `IIFE 未挂载 globalThis.${globalName}（请用 formats:['iife'] name:'${globalName}' 重建）`,
      }
    }

    const mod = normalizePackExport(exported)
    if (!mod.default) {
      return {
        ok: false,
        error: `${globalName} 已挂载但无 default 导出（请 export default 产品视图）`,
      }
    }
    return { ok: true, mod }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    console.warn(`[plugin] loadPackUI(${packId}) failed`, err)
    return { ok: false, error: msg }
  }
}

function normalizePackExport(exported: unknown): PackUIModule {
  if (typeof exported === 'function') {
    return { default: exported as PackUIModule['default'] }
  }
  if (exported && typeof exported === 'object') {
    return exported as PackUIModule
  }
  return {}
}

/** resolvePackEntryFile 从 manifest entry（如 ui/index.js）取文件名。 */
export function resolvePackEntryFile(uiEntry?: string): string {
  if (!uiEntry) return 'index.js'
  const parts = uiEntry.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || 'index.js'
}
