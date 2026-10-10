import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api/client'
import { IconPlay, IconRefresh, IconSettings } from '../../components/Icons'
import { ConfirmDialog } from '../../components/ConfirmDialog'
import { pressProps } from '../../components/compat'
import { LoadingPane } from '../../components/LoadingHost'
import { withLoading } from '../../stores/loadingStore'
import { useAppStore } from '../../stores/appStore'
import { useI18n } from '../../i18n'
import type { PluginPackBuildResult, PluginPackInfo, PluginPackReleaseInfo } from '../../api/pluginPack'
import { bumpPackUI } from '../../plugin/packEvents'

const LOADING_KEY = 'plugins.list'

/** PluginsWorkbench 插件包：启用 / 构建 / 版本历史 / 回滚。 */
export function PluginsWorkbench() {
  const { t } = useI18n()
  const setStatusMessage = useAppStore((s) => s.setStatusMessage)
  const setActiveProduct = useAppStore((s) => s.setActiveProduct)
  const [packs, setPacks] = useState<PluginPackInfo[]>([])
  const [hostCap, setHostCap] = useState('')
  const [protocolPath, setProtocolPath] = useState('')
  const [buildingId, setBuildingId] = useState<string | null>(null)
  const [lastBuild, setLastBuild] = useState<PluginPackBuildResult | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [releases, setReleases] = useState<Record<string, PluginPackReleaseInfo[]>>({})
  const [rollbackTarget, setRollbackTarget] = useState<{ packId: string; release: PluginPackReleaseInfo } | null>(
    null,
  )
  const [rolling, setRolling] = useState(false)

  const refresh = useCallback(async () => {
    try {
      await withLoading(
        LOADING_KEY,
        async () => {
          const [list, proto] = await Promise.all([api.listPluginPacks(), api.getPluginProtocol()])
          setPacks(list)
          setHostCap(proto.hostCapVersion || '')
          setProtocolPath(proto.path || '')
        },
        { label: t('plugins.loading') },
      )
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    }
  }, [setStatusMessage, t])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const loadReleases = async (packId: string) => {
    try {
      const list = await api.listPluginPackReleases(packId)
      setReleases((prev) => ({ ...prev, [packId]: list }))
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    }
  }

  const toggleVersions = async (pack: PluginPackInfo) => {
    if (expandedId === pack.id) {
      setExpandedId(null)
      return
    }
    setExpandedId(pack.id)
    await loadReleases(pack.id)
  }

  const toggleEnabled = async (pack: PluginPackInfo) => {
    try {
      const next = await api.setPluginPackEnabled(pack.id, !pack.enabled)
      setPacks((prev) => prev.map((p) => (p.id === next.id ? next : p)))
      setStatusMessage(
        next.enabled ? t('plugins.enabled', { id: next.id }) : t('plugins.disabled', { id: next.id }),
      )
      bumpPackUI(next.id)
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    }
  }

  const reveal = async (path: string) => {
    try {
      await api.revealPluginPackPath(path)
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    }
  }

  const buildUI = async (pack: PluginPackInfo) => {
    setBuildingId(pack.id)
    setLastBuild(null)
    try {
      const result = await api.buildPluginPackUI(pack.id)
      setLastBuild(result)
      setStatusMessage(
        t('plugins.buildOk', {
          id: result.packId,
          fp: result.buildFingerprint.slice(0, 12),
          dir: result.uiDir,
        }),
      )
      await refresh()
      if (expandedId === pack.id || result.releaseId) {
        await loadReleases(pack.id)
        setExpandedId(pack.id)
      }
      bumpPackUI(pack.id)
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    } finally {
      setBuildingId(null)
    }
  }

  const snapshot = async (pack: PluginPackInfo) => {
    try {
      const info = await api.snapshotPluginPack(pack.id)
      setStatusMessage(t('plugins.snapshotOk', { id: info.id }))
      await loadReleases(pack.id)
      setExpandedId(pack.id)
      await refresh()
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    }
  }

  const confirmRollback = async () => {
    if (!rollbackTarget) return
    setRolling(true)
    try {
      const next = await api.rollbackPluginPack(rollbackTarget.packId, rollbackTarget.release.id)
      setPacks((prev) => prev.map((p) => (p.id === next.id ? next : p)))
      setStatusMessage(t('plugins.rollbackOk', { id: rollbackTarget.release.id }))
      await loadReleases(rollbackTarget.packId)
      bumpPackUI(rollbackTarget.packId)
      setRollbackTarget(null)
    } catch (e) {
      setStatusMessage(e instanceof Error ? e.message : String(e))
    } finally {
      setRolling(false)
    }
  }

  const reasonLabel = (reason: string) => {
    switch (reason) {
      case 'build':
        return t('plugins.reasonBuild')
      case 'pre-upgrade':
        return t('plugins.reasonPreUpgrade')
      case 'pre-rollback':
        return t('plugins.reasonPreRollback')
      case 'manual':
        return t('plugins.reasonManual')
      default:
        return reason
    }
  }

  return (
    <div className="product-workbench plugins-workbench">
      <header className="product-toolbar plugins-toolbar">
        <div className="product-actions">
          <button type="button" className="wn-btn wn-btn-sm wn-btn-tool" {...pressProps(() => void refresh())}>
            <IconRefresh size={14} /> {t('plugins.refresh')}
          </button>
          {protocolPath ? (
            <button
              type="button"
              className="wn-btn wn-btn-sm wn-btn-tool"
              {...pressProps(() => void reveal(protocolPath))}
            >
              {t('plugins.protocolOpen')}
            </button>
          ) : null}
          <span className="product-toolbar-status">
            {t('plugins.title')}
            {hostCap ? ` · ${t('plugins.hostCap', { v: hostCap })}` : ''}
          </span>
        </div>
      </header>
      <div className="product-body plugins-body">
        {lastBuild ? (
          <div className="plugins-build-banner" role="status">
            <strong>{t('plugins.buildSuccessTitle')}</strong>
            <p>{t('plugins.buildSuccessBody', { id: lastBuild.packId })}</p>
            <p>
              <code>{lastBuild.uiDir}</code>
            </p>
            {lastBuild.releaseId ? (
              <p className="plugins-build-files">
                {t('plugins.releaseSaved')}: <code>{lastBuild.releaseId}</code>
              </p>
            ) : null}
            <div className="plugins-build-actions">
              <button
                type="button"
                className="wn-btn wn-btn-sm wn-btn-primary"
                {...pressProps(() => void reveal(lastBuild.uiDir))}
              >
                {t('plugins.revealRun')}
              </button>
              {lastBuild.packId === 'database' ? (
                <button
                  type="button"
                  className="wn-btn wn-btn-sm wn-btn-tool"
                  {...pressProps(() => setActiveProduct('database'))}
                >
                  {t('plugins.openDatabase')}
                </button>
              ) : null}
            </div>
          </div>
        ) : null}
        <LoadingPane loadingKey={LOADING_KEY}>
          {packs.length === 0 ? (
            <div className="pane-empty">{t('plugins.empty')}</div>
          ) : (
            <ul className="plugins-list">
              {packs.map((pack) => (
                <li key={pack.id} className="plugins-card">
                  <div className="plugins-card-head">
                    <IconSettings size={18} />
                    <div className="plugins-card-titles">
                      <strong>{pack.name}</strong>
                      <span className="plugins-meta">
                        v{pack.version}
                        {pack.uiBuilt ? ` · ${t('plugins.uiBuilt')}` : ` · ${t('plugins.uiMissing')}`}
                        {pack.capOk === false
                          ? ` · ${t('plugins.capBad')}`
                          : pack.requiresWwCap
                            ? ` · ${t('plugins.capOk')}`
                            : ''}
                        {pack.releaseCount
                          ? ` · ${t('plugins.releaseCount', { n: pack.releaseCount })}`
                          : ''}
                      </span>
                      {pack.capDetail ? (
                        <span className="plugins-meta plugins-cap-detail" title={pack.capDetail}>
                          {pack.capDetail}
                        </span>
                      ) : null}
                    </div>
                    <label className="plugins-toggle">
                      <input
                        type="checkbox"
                        checked={pack.enabled}
                        onChange={() => void toggleEnabled(pack)}
                      />
                      <span>{pack.enabled ? t('plugins.on') : t('plugins.off')}</span>
                    </label>
                  </div>
                  <dl className="plugins-details">
                    <div>
                      <dt>{t('plugins.sourcePath')}</dt>
                      <dd>
                        <code>{pack.sourcePath}</code>{' '}
                        <button
                          type="button"
                          className="wn-btn wn-btn-xs wn-linkish"
                          {...pressProps(() => void reveal(pack.sourcePath))}
                        >
                          {t('plugins.reveal')}
                        </button>
                      </dd>
                    </div>
                    <div>
                      <dt>{t('plugins.uiPath')}</dt>
                      <dd>
                        <code>{pack.uiPath || `${pack.runPath}/ui`}</code>{' '}
                        {pack.uiBuilt ? (
                          <button
                            type="button"
                            className="wn-btn wn-btn-xs wn-linkish"
                            {...pressProps(() => void reveal(pack.uiPath || `${pack.runPath}/ui`))}
                          >
                            {t('plugins.reveal')}
                          </button>
                        ) : null}
                      </dd>
                    </div>
                    {pack.activeReleaseId ? (
                      <div>
                        <dt>{t('plugins.activeRelease')}</dt>
                        <dd>
                          <code>{pack.activeReleaseId}</code>
                        </dd>
                      </div>
                    ) : null}
                    {pack.capabilities?.length ? (
                      <div>
                        <dt>{t('plugins.capabilities')}</dt>
                        <dd>{pack.capabilities.join(', ')}</dd>
                      </div>
                    ) : null}
                  </dl>
                  <div className="plugins-card-actions">
                    <button
                      type="button"
                      className="wn-btn wn-btn-sm wn-btn-primary"
                      disabled={buildingId === pack.id}
                      {...pressProps(() => void buildUI(pack), { disabled: buildingId === pack.id })}
                    >
                      <IconPlay size={14} />{' '}
                      {buildingId === pack.id ? t('plugins.building') : t('plugins.build')}
                    </button>
                    <button
                      type="button"
                      className="wn-btn wn-btn-sm wn-btn-tool"
                      {...pressProps(() => void snapshot(pack))}
                    >
                      {t('plugins.snapshot')}
                    </button>
                    <button
                      type="button"
                      className="wn-btn wn-btn-sm wn-btn-tool"
                      {...pressProps(() => void toggleVersions(pack))}
                    >
                      {expandedId === pack.id ? t('plugins.hideVersions') : t('plugins.showVersions')}
                    </button>
                  </div>
                  {expandedId === pack.id ? (
                    <div className="plugins-releases">
                      <div className="plugins-releases-head">{t('plugins.versionsTitle')}</div>
                      {(releases[pack.id] ?? []).length === 0 ? (
                        <p className="plugins-hint">{t('plugins.versionsEmpty')}</p>
                      ) : (
                        <ul className="plugins-release-list">
                          {(releases[pack.id] ?? []).map((rel) => (
                            <li key={rel.id} className={`plugins-release${rel.active ? ' is-active' : ''}`}>
                              <div className="plugins-release-main">
                                <code>{rel.id}</code>
                                <span className="plugins-meta">
                                  v{rel.version} · {reasonLabel(rel.reason)}
                                  {rel.hasUI ? ` · UI` : ''}
                                  {rel.active ? ` · ${t('plugins.releaseActive')}` : ''}
                                </span>
                                <span className="plugins-meta">{rel.createdAt}</span>
                              </div>
                              <div className="plugins-release-actions">
                                <button
                                  type="button"
                                  className="wn-btn wn-btn-xs wn-linkish"
                                  {...pressProps(() => void reveal(rel.path))}
                                >
                                  {t('plugins.reveal')}
                                </button>
                                <button
                                  type="button"
                                  className="wn-btn wn-btn-xs wn-btn-tool"
                                  disabled={rel.active || rolling}
                                  {...pressProps(
                                    () => setRollbackTarget({ packId: pack.id, release: rel }),
                                    { disabled: rel.active || rolling },
                                  )}
                                >
                                  {t('plugins.rollback')}
                                </button>
                              </div>
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </LoadingPane>
      </div>
      <ConfirmDialog
        open={rollbackTarget != null}
        title={t('plugins.rollbackTitle')}
        message={
          rollbackTarget
            ? t('plugins.rollbackMsg', {
                pack: rollbackTarget.packId,
                id: rollbackTarget.release.id,
              })
            : undefined
        }
        confirmLabel={rolling ? t('plugins.rollingBack') : t('plugins.rollback')}
        danger
        onConfirm={() => void confirmRollback()}
        onCancel={() => {
          if (!rolling) setRollbackTarget(null)
        }}
      />
    </div>
  )
}
