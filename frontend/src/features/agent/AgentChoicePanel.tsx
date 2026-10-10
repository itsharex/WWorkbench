import { useEffect, useMemo, useState } from 'react'
import {
  formatChoiceSendText,
  type AgentChoiceQuestion,
  type ChoiceAnswer,
} from './agentChoice'
import { useI18n } from '../../i18n'
import { pressProps } from '../../components/compat'

export type ChoiceSubmitAnswer = {
  pendingId?: string
  keys: string[]
  text: string
}

interface Props {
  questions: AgentChoiceQuestion[]
  /** 历史气泡 / 忙碌时不可点 */
  disabled?: boolean
  /** 嵌入助手气泡：无独立边框，随消息区滚动 */
  inline?: boolean
  /** 点选后提交：offer_choices 续跑，或围栏 choice 转成用户消息 */
  onSubmit: (answers: ChoiceSubmitAnswer[]) => void
}

/** AgentChoicePanel 渲染助手挂出的可点选项。 */
export function AgentChoicePanel({ questions, disabled, inline, onSubmit }: Props) {
  const { t } = useI18n()
  const [answers, setAnswers] = useState<Record<number, ChoiceAnswer>>({})

  const qKey = questions.map((q) => `${q.n}:${q.id || q.prompt}:${q.mode}`).join('|')
  useEffect(() => {
    const next: Record<number, ChoiceAnswer> = {}
    for (const q of questions) {
      next[q.n] = { keys: [], text: '' }
    }
    setAnswers(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qKey])

  const sendTexts = useMemo(() => {
    const out: string[] = []
    for (const q of questions) {
      const a = answers[q.n]
      if (!a) continue
      const text = formatChoiceSendText(q, a)
      if (text) out.push(text)
    }
    return out
  }, [questions, answers])

  const canSendAll = sendTexts.length === questions.length && sendTexts.every(Boolean) && !disabled
  const needBatch = questions.some((q) => q.mode === 'multi') || questions.length > 1

  const toSubmitPayload = (next: Record<number, ChoiceAnswer>): ChoiceSubmitAnswer[] =>
    questions.map((q) => {
      const a = next[q.n] ?? { keys: [], text: '' }
      const text = (a.text ?? '').trim() || formatChoiceSendText(q, a)
      return {
        pendingId: q.id,
        keys: a.keys ?? [],
        text,
      }
    })

  const submitNow = (next: Record<number, ChoiceAnswer>) => {
    if (disabled) return
    onSubmit(toSubmitPayload(next))
  }

  const pickSingle = (q: AgentChoiceQuestion, key: string) => {
    if (disabled) return
    if (needBatch) {
      setAnswers((prev) => ({ ...prev, [q.n]: { keys: [key], text: '' } }))
      return
    }
    submitNow({ ...answers, [q.n]: { keys: [key], text: '' } })
  }

  const toggleMulti = (q: AgentChoiceQuestion, key: string) => {
    if (disabled) return
    setAnswers((prev) => {
      const cur = prev[q.n] ?? { keys: [], text: '' }
      const keys = [...(cur.keys ?? [])]
      const i = keys.indexOf(key)
      if (i >= 0) keys.splice(i, 1)
      else keys.push(key)
      return { ...prev, [q.n]: { ...cur, keys } }
    })
  }

  const submitText = (q: AgentChoiceQuestion) => {
    if (disabled) return
    const text = formatChoiceSendText(q, answers[q.n] ?? {})
    if (!text) return
    if (needBatch) {
      setAnswers((prev) => ({
        ...prev,
        [q.n]: { ...(prev[q.n] ?? { keys: [] }), text },
      }))
      return
    }
    submitNow({ ...answers, [q.n]: { keys: [], text } })
  }

  const isSelected = (q: AgentChoiceQuestion, key: string) => {
    const keys = answers[q.n]?.keys || []
    if (q.mode === 'multi') return keys.includes(key)
    return keys[0] === key
  }

  return (
    <div className={`agent-choice${inline ? ' agent-choice-inline' : ''}${disabled ? ' is-disabled' : ''}`}>
      <div className="agent-choice-head">{t('agent.choiceTitle')}</div>
      {questions.map((q) => (
        <div key={`${q.n}-${q.id || q.prompt}`} className="agent-choice-q">
          <div className="agent-choice-prompt">
            <span className="agent-choice-n">{q.n}.</span>
            {q.prompt}
          </div>
          {q.mode === 'text' ? (
            <div className="agent-choice-text-row">
              <input
                className="agent-choice-input"
                type="text"
                placeholder={q.placeholder || t('agent.choicePlaceholder')}
                disabled={disabled}
                value={answers[q.n]?.text || ''}
                onChange={(e) =>
                  setAnswers((prev) => ({
                    ...prev,
                    [q.n]: { ...(prev[q.n] ?? { keys: [] }), text: e.target.value },
                  }))
                }
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    submitText(q)
                  }
                }}
              />
              <button
                type="button"
                className="wn-btn wn-btn-xs"
                disabled={disabled || !(answers[q.n]?.text || '').trim()}
                {...pressProps(() => submitText(q), {
                  disabled: disabled || !(answers[q.n]?.text || '').trim(),
                })}
              >
                {t('agent.choiceConfirm')}
              </button>
            </div>
          ) : (
            <div className="agent-choice-opts">
              {q.options.map((opt) => (
                <button
                  key={opt.key}
                  type="button"
                  className={`agent-choice-opt${isSelected(q, opt.key) ? ' is-on' : ''}${
                    q.mode === 'multi' ? ' is-multi' : ''
                  }`}
                  disabled={disabled}
                  title={opt.label}
                  {...pressProps(
                    () => (q.mode === 'multi' ? toggleMulti(q, opt.key) : pickSingle(q, opt.key)),
                    { disabled },
                  )}
                >
                  <span className="agent-choice-key">{opt.key}</span>
                  <span className="agent-choice-label">{opt.label}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      ))}
      {needBatch && (
        <div className="agent-choice-foot">
          <button
            type="button"
            className="wn-btn wn-btn-xs wn-btn-primary"
            disabled={!canSendAll}
            {...pressProps(() => submitNow(answers), { disabled: !canSendAll })}
          >
            {t('agent.choiceConfirm')}
            {sendTexts.length ? `（${sendTexts.length}/${questions.length}）` : ''}
          </button>
        </div>
      )}
    </div>
  )
}
