import { useCallback, useEffect, useRef } from 'react'

/** 距底部小于此值视为贴底；仅用户滚动可取消跟滚。 */
const STICK_PX = 80

/**
 * useAgentChatScroll 对话列表贴底。
 * 流式/工具期用 rAF 轻量跟底（只写 scrollTop）；空闲期靠 contentKey + ResizeObserver。
 */
export function useAgentChatScroll(
  enabled: boolean,
  contentKey: string,
  /** 流式/工具/揭示中：强制连续贴底 */
  forceStick = false,
) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const endRef = useRef<HTMLDivElement>(null)
  const stickToBottomRef = useRef(true)
  const forceStickRef = useRef(forceStick)
  const programmaticRef = useRef(false)
  const pendingForceRef = useRef(false)
  const kickRafRef = useRef(0)
  const followRafRef = useRef(0)

  forceStickRef.current = forceStick

  const onScroll = useCallback(() => {
    // 强制跟底或程序滚动中：不根据位置取消 stick
    if (programmaticRef.current || forceStickRef.current) return
    const el = scrollRef.current
    if (!el) return
    stickToBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_PX
  }, [])

  const applyBottom = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    programmaticRef.current = true
    el.scrollTop = el.scrollHeight
    // 同步清掉标记；若浏览器异步打 onScroll，靠 forceStickRef 挡一档
    programmaticRef.current = false
  }, [])

  const scrollToBottom = useCallback(
    (force = false) => {
      if (force) stickToBottomRef.current = true
      if (!force && !stickToBottomRef.current) return
      applyBottom()
    },
    [applyBottom],
  )

  const pinToBottom = useCallback(() => {
    stickToBottomRef.current = true
    scrollToBottom(true)
  }, [scrollToBottom])

  const scheduleScroll = useCallback(
    (force = false) => {
      if (force) pendingForceRef.current = true
      if (kickRafRef.current) return
      kickRafRef.current = requestAnimationFrame(() => {
        kickRafRef.current = 0
        const f = pendingForceRef.current
        pendingForceRef.current = false
        scrollToBottom(f)
      })
    },
    [scrollToBottom],
  )

  // 活跃回合：每帧跟底，异步 MD/工具撑高也能带着走；代价只是写 scrollTop
  useEffect(() => {
    if (!enabled || !forceStick) return
    stickToBottomRef.current = true
    let alive = true
    const tick = () => {
      if (!alive) return
      applyBottom()
      followRafRef.current = requestAnimationFrame(tick)
    }
    followRafRef.current = requestAnimationFrame(tick)
    return () => {
      alive = false
      if (followRafRef.current) cancelAnimationFrame(followRafRef.current)
      followRafRef.current = 0
      // 结束强制跟底时再贴一次，接住 deferred 排版尾差
      applyBottom()
    }
  }, [enabled, forceStick, applyBottom])

  useEffect(() => {
    if (!enabled) return
    if (forceStick) stickToBottomRef.current = true
    scheduleScroll(forceStick)
  }, [enabled, contentKey, forceStick, scheduleScroll])

  useEffect(() => {
    return () => {
      if (kickRafRef.current) cancelAnimationFrame(kickRafRef.current)
      if (followRafRef.current) cancelAnimationFrame(followRafRef.current)
    }
  }, [])

  useEffect(() => {
    if (!enabled) return
    const el = scrollRef.current
    if (!el) return
    const inner = el.querySelector('.agent-messages-inner')
    if (!inner) return

    const kick = () => {
      if (forceStickRef.current) {
        stickToBottomRef.current = true
        // 强制跟底循环已在跑，无需再 schedule
        return
      }
      if (!stickToBottomRef.current) return
      scheduleScroll(false)
    }

    const ro = new ResizeObserver(kick)
    ro.observe(inner)
    return () => ro.disconnect()
  }, [enabled, scheduleScroll])

  return { scrollRef, endRef, onScroll, scrollToBottom, pinToBottom }
}
