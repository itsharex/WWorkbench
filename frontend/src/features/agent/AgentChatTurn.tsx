import { memo, startTransition, useEffect, useRef, useState } from 'react'
import type { AgentChatLine } from '../../stores/agentStore'
import { useAgentStore } from '../../stores/agentStore'
import { AgentMessageContent } from './AgentMessageContent'
import { AgentRichContent } from './AgentRichContent'
import { AgentTurnTools } from './AgentTurnTools'
import type { ChoiceSubmitAnswer } from './AgentChoicePanel'

interface Props {
  line: AgentChatLine
  threadId: string
  isLastAssistant: boolean
  /** 本轮是否显示角色名（一轮助手回复只显示一次「助手」）。 */
  showRoleLabel?: boolean
  choiceDisabled: boolean
  hideInlineChoice?: boolean
  skillCatalog: Record<string, string>
  t: (key: string, params?: Record<string, string | number>) => string
  onChoiceSubmit?: (answers: ChoiceSubmitAnswer[]) => void
}

function historyRef(threadId: string, seq?: number): string {
  const tid = threadId.trim()
  if (!tid) return seq && seq > 0 ? `#${seq}` : ''
  if (seq && seq > 0) return `${tid}#${seq}`
  return tid
}

function shortThread(threadId: string): string {
  const t = threadId.trim()
  if (t.length <= 12) return t
  return `${t.slice(0, 8)}…`
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

/** 整段突至时的匀速追稿；有 token 流时直接跟 draft，不走打字机。 */
const STREAM_CPS = 96

/**
 * 助手正文：流式期即渲染 Markdown；无 delta 整段到达时匀速追终稿（仍是 MD）。
 */
function AssistantRevealBody({
  lineId,
  finalContent,
  live,
  choiceDisabled,
  hideInlineChoice,
  skillIds,
  skillCatalog,
  onChoiceSubmit,
}: {
  lineId: string
  finalContent: string
  /** 正在收 token，或收尾后仍在匀速揭示 */
  live: boolean
  choiceDisabled: boolean
  hideInlineChoice?: boolean
  skillIds?: string[]
  skillCatalog: Record<string, string>
  onChoiceSubmit?: (answers: ChoiceSubmitAnswer[]) => void
}) {
  const draft = useAgentStore((s) => (s.streamingLineId === lineId ? s.streamingDraft : ''))
  const streaming = useAgentStore((s) => s.streamingLineId === lineId)
  const clearRevealing = useAgentStore((s) => s.clearRevealing)
  const [shown, setShown] = useState('')
  const [play, setPlay] = useState(false)
  const shownRef = useRef('')
  const lastTsRef = useRef(0)
  const carryRef = useRef(0)
  const lastPaintRef = useRef(0)
  shownRef.current = shown

  const paintShown = (next: string, force = false) => {
    shownRef.current = next
    const now = performance.now()
    // 流式 MD 解析略重：合并到约 32ms 一帧，避免每 token 全量 marked
    if (!force && now - lastPaintRef.current < 32) return
    lastPaintRef.current = now
    startTransition(() => setShown(next))
  }

  // 有 token：正文紧跟 draft，边收边渲染 Markdown
  useEffect(() => {
    if (!streaming) return
    setPlay(true)
    if (draft) paintShown(draft, true)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- paintShown 稳定闭包不必要
  }, [streaming, draft])

  // 进入 live / 切到收尾揭示：对齐前缀，禁止整段跳稿
  useEffect(() => {
    if (!live) return
    setPlay(true)
    if (streaming) return
    const goal = finalContent || ''
    const cur = shownRef.current
    if (goal && cur && !goal.startsWith(cur)) {
      let i = 0
      while (i < cur.length && i < goal.length && cur[i] === goal[i]) i += 1
      paintShown(goal.slice(0, i), true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, streaming, finalContent])

  // 无 delta 整段到达：匀速追终稿（已是 Markdown 渲染）
  useEffect(() => {
    if (!play || streaming) return
    let raf = 0
    let alive = true
    lastTsRef.current = 0
    carryRef.current = 0

    const tick = (ts: number) => {
      if (!alive) return
      const goal = finalContent
      const cur = shownRef.current

      if (!goal) {
        raf = requestAnimationFrame(tick)
        return
      }

      if (cur === goal) {
        paintShown(goal, true)
        setPlay(false)
        clearRevealing(lineId)
        return
      }

      if (!goal.startsWith(cur)) {
        let i = 0
        while (i < cur.length && i < goal.length && cur[i] === goal[i]) i += 1
        paintShown(goal.slice(0, i), true)
        raf = requestAnimationFrame(tick)
        return
      }

      if (!lastTsRef.current) lastTsRef.current = ts
      const dt = Math.min(50, ts - lastTsRef.current)
      lastTsRef.current = ts
      const lag = goal.length - cur.length
      const cps = Math.min(320, Math.max(STREAM_CPS, lag / 5))
      const maxStep = lag > 400 ? 24 : lag > 120 ? 12 : 6
      carryRef.current += (cps * dt) / 1000
      let step = Math.floor(carryRef.current)
      if (step < 1) {
        raf = requestAnimationFrame(tick)
        return
      }
      carryRef.current -= step
      if (step > maxStep) step = maxStep
      if (step > lag) step = lag
      paintShown(goal.slice(0, cur.length + step))
      raf = requestAnimationFrame(tick)
    }

    raf = requestAnimationFrame(tick)
    return () => {
      alive = false
      cancelAnimationFrame(raf)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [play, streaming, lineId, finalContent, clearRevealing])

  const revealing = play || live
  const streamText = streaming ? draft || shown : shown
  if (revealing && (streamText.length > 0 || live)) {
    return (
      <div className="agent-rich-stream">
        {streamText ? <AgentRichContent content={streamText} defer /> : null}
        <span className="agent-stream-caret" aria-hidden />
      </div>
    )
  }

  return (
    <AgentMessageContent
      content={finalContent}
      role="assistant"
      skillIds={skillIds}
      skillLabels={skillCatalog}
      choiceDisabled={choiceDisabled}
      hideInlineChoice={hideInlineChoice}
      onChoiceSubmit={onChoiceSubmit}
    />
  )
}

/** AgentChatTurn 单条对话回合（memo 隔离历史消息）。 */
export const AgentChatTurn = memo(function AgentChatTurn({
  line,
  threadId,
  isLastAssistant,
  showRoleLabel = true,
  choiceDisabled,
  hideInlineChoice,
  skillCatalog,
  t,
  onChoiceSubmit,
}: Props) {
  const streamingLineId = useAgentStore((s) => s.streamingLineId)
  const revealingLineId = useAgentStore((s) => s.revealingLineId)
  const live = streamingLineId === line.id || revealingLineId === line.id
  const href = historyRef(threadId, line.seq)
  const hasTools = Boolean(line.tools && line.tools.length)
  const toolsLive = live || Boolean(line.tools?.some((s) => s.status === 'running'))
  // 工具前正文：已有工具时不再用流式行渲染（流式进 contentAfter）
  const showBefore =
    line.role === 'system' ||
    Boolean(line.content.trim()) ||
    Boolean(line.images?.length) ||
    Boolean(line.skillIds?.length) ||
    (live && !hasTools)
  const showAfter =
    line.role === 'assistant' && (Boolean(line.contentAfter?.trim()) || (live && hasTools))
  const showHistId = Boolean(href) && (line.role === 'user' || showRoleLabel)
  const showMeta =
    (line.role === 'assistant' || line.role === 'user') && (showRoleLabel || showHistId)

  return (
    <div className={`agent-turn agent-turn-${line.role}`}>
      {showMeta && (
        <div className="agent-turn-meta">
          {showRoleLabel && line.role === 'assistant' && (
            <span className="agent-turn-label">{t('agent.assistantLabel')}</span>
          )}
          {showRoleLabel && line.role === 'user' && (
            <span className="agent-turn-label agent-turn-label-user">{t('agent.youLabel')}</span>
          )}
          {showHistId ? (
            <button
              type="button"
              className="agent-turn-histid"
              title={t('agent.copyHistoryId')}
              onClick={() => void copyText(href)}
            >
              {line.seq && line.seq > 0 ? (
                <>
                  <span className="agent-turn-histid-thread">{shortThread(threadId)}</span>
                  <span className="agent-turn-histid-seq">#{line.seq}</span>
                </>
              ) : (
                <span className="agent-turn-histid-thread">{shortThread(threadId)}</span>
              )}
            </button>
          ) : null}
        </div>
      )}
      {/* 时间线：工具前正文 → 工具 → 工具后续写 */}
      {showBefore && (
        <div
          className={
            line.role === 'system'
              ? 'agent-bubble agent-bubble-system'
              : `agent-bubble agent-bubble-${line.role}`
          }
        >
          {line.role === 'assistant' ? (
            hasTools ? (
              <AgentMessageContent
                content={line.content}
                role="assistant"
                skillIds={line.skillIds}
                skillLabels={skillCatalog}
                choiceDisabled
                hideInlineChoice
              />
            ) : (
              <AssistantRevealBody
                lineId={line.id}
                finalContent={line.content}
                live={live}
                choiceDisabled={choiceDisabled || !isLastAssistant}
                hideInlineChoice={hideInlineChoice}
                skillIds={line.skillIds}
                skillCatalog={skillCatalog}
                onChoiceSubmit={onChoiceSubmit}
              />
            )
          ) : (
            <AgentMessageContent
              content={line.content}
              role={line.role}
              mentions={line.mentions}
              skillIds={line.skillIds}
              skillLabels={skillCatalog}
              images={line.images}
            />
          )}
        </div>
      )}
      {line.role === 'assistant' && hasTools ? (
        <AgentTurnTools tools={line.tools!} live={toolsLive} />
      ) : null}
      {showAfter && (
        <div className="agent-bubble agent-bubble-assistant">
          <AssistantRevealBody
            lineId={line.id}
            finalContent={line.contentAfter || ''}
            live={live && hasTools}
            choiceDisabled={choiceDisabled || !isLastAssistant}
            hideInlineChoice={hideInlineChoice}
            skillIds={undefined}
            skillCatalog={skillCatalog}
            onChoiceSubmit={onChoiceSubmit}
          />
        </div>
      )}
    </div>
  )
})
