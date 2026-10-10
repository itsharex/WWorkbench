import { useDeferredValue, useMemo, type MouseEvent } from 'react'
import { markdownToHtml } from '../../utils/markdown'
import { parseAgentBlocks } from '../../utils/agentContent'
import { AgentEChart } from './AgentEChart'
import { useI18n } from '../../i18n'

interface Props {
  content: string
  /** 流式期：用 deferred 内容做 MD，避免每帧同步 marked 卡住输入 */
  defer?: boolean
}

/** AgentRichContent 渲染助手消息（Markdown + ECharts）。 */
export function AgentRichContent({ content, defer = false }: Props) {
  const { t } = useI18n()
  const copyLabel = t('common.copy')
  const copiedLabel = t('common.copied')
  const deferred = useDeferredValue(content)
  const source = defer ? deferred : content
  const blocks = useMemo(() => parseAgentBlocks(source), [source])

  const onClick = (e: MouseEvent) => {
    const btn = (e.target as HTMLElement).closest('.md-code-copy')
    if (!(btn instanceof HTMLElement)) return
    const code = btn.parentElement?.querySelector('code')
    const text = code?.textContent ?? ''
    if (!text) return
    void navigator.clipboard.writeText(text).then(() => {
      btn.setAttribute('title', copiedLabel)
      btn.setAttribute('aria-label', copiedLabel)
      btn.classList.add('is-copied')
      window.setTimeout(() => {
        btn.setAttribute('title', copyLabel)
        btn.setAttribute('aria-label', copyLabel)
        btn.classList.remove('is-copied')
      }, 1200)
    })
  }

  return (
    <div className="agent-rich" data-deferred={defer && deferred !== content ? '1' : undefined}>
      {blocks.map((block, i) => {
        if (block.kind === 'echarts') {
          return (
            <AgentEChart
              key={`chart-${i}`}
              option={block.option}
              error={block.error}
            />
          )
        }
        return (
          <MarkdownBlock
            key={`md-${i}`}
            text={block.text}
            copyLabel={copyLabel}
            onClick={onClick}
          />
        )
      })}
    </div>
  )
}

function MarkdownBlock({
  text,
  copyLabel,
  onClick,
}: {
  text: string
  copyLabel: string
  onClick: (e: MouseEvent) => void
}) {
  const html = useMemo(() => markdownToHtml(text, { copyLabel }), [text, copyLabel])
  if (!html.trim()) return null
  return (
    <div className="agent-md" onClick={onClick} dangerouslySetInnerHTML={{ __html: html }} />
  )
}
