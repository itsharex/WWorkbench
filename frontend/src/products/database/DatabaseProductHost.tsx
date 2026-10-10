import { useEffect, useState, type ComponentType } from 'react'
import { useI18n } from '../../i18n'
import { ErrorBoundary } from '../../components/ErrorBoundary'
import { pressProps } from '../../components/compat'
import { loadPackUI, resolvePackEntryFile } from '../../plugin/loadPackUI'
import { subscribePackUI } from '../../plugin/packEvents'
import { subscribeWorkbenchChanged } from '../../workbench/workbenchRadar'
import { api } from '../../api/client'
import { useAppStore } from '../../stores/appStore'

/**
 * DatabaseProductHost 只加载 database pack UI（无 builtin 双轨）。
 * 未构建 / 禁用 / 加载失败时给出引导，不回退 Host 内置工作台。
 */
export function DatabaseProductHost() {
  const { t } = useI18n()
  const setActiveProduct = useAppStore((s) => s.setActiveProduct)
  const [PackView, setPackView] = useState<ComponentType | null>(null)
  const [ready, setReady] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [uiPath, setUiPath] = useState('')
  const [uiBuilt, setUiBuilt] = useState(false)
  const [enabled, setEnabled] = useState(true)
  const [tick, setTick] = useState(0)

  useEffect(() => subscribePackUI((id) => {
    if (id === 'database') setTick((n) => n + 1)
  }), [])

  useEffect(
    () =>
      subscribeWorkbenchChanged((ev) => {
        if (ev.domain === 'plugin.pack') setTick((n) => n + 1)
      }),
    [],
  )

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setReady(false)
      setLoadError('')
      setPackView(null)
      try {
        const pack = await api.getPluginPack('database')
        if (cancelled) return
        setUiPath(pack.uiPath || `${pack.runPath}/ui`)
        setUiBuilt(pack.uiBuilt)
        setEnabled(pack.enabled)
        if (!pack.enabled) {
          setLoadError(t('plugins.packDisabled'))
          return
        }
        if (!pack.uiBuilt) {
          setLoadError(t('plugins.needBuild'))
          return
        }
        const result = await loadPackUI('database', resolvePackEntryFile(pack.uiEntry))
        if (cancelled) return
        if (!result.ok) {
          setLoadError(`${t('plugins.loadFailed')} · ${result.error}`)
          return
        }
        const Comp = result.mod.default
        if (!Comp) {
          setLoadError(t('plugins.loadFailed') + ' · no default export')
          return
        }
        setPackView(() => Comp)
      } catch (e) {
        if (!cancelled) {
          setLoadError(e instanceof Error ? e.message : String(e))
        }
      } finally {
        if (!cancelled) setReady(true)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [tick, t])

  if (!ready) {
    return <div className="pane-empty">{t('shell.loadingProduct')}</div>
  }

  if (PackView) {
    return (
      <div className="pack-ui-host" data-ww-pack-host="database">
        <ErrorBoundary key={tick}>
          <PackView />
        </ErrorBoundary>
      </div>
    )
  }

  return (
    <div className="pane-empty pack-ui-missing">
      <p>{loadError || t('plugins.needBuild')}</p>
      {uiPath ? (
        <p className="pack-ui-missing-path">
          <code>{uiPath}</code>
          {!uiBuilt ? ` · ${t('plugins.uiMissing')}` : null}
          {!enabled ? ` · ${t('plugins.off')}` : null}
        </p>
      ) : null}
      <button
        type="button"
        className="wn-btn wn-btn-sm"
        {...pressProps(() => setActiveProduct('plugins'))}
      >
        {t('plugins.openPlugins')}
      </button>
    </div>
  )
}
