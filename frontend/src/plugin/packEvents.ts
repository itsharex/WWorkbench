/** pack UI 变更通知：构建成功后递增，DatabaseProductHost 等可重试加载。 */

type PackUIListener = (packId: string, generation: number) => void

const generations = new Map<string, number>()
const listeners = new Set<PackUIListener>()

/** bumpPackUI 标记某 pack UI 已更新（构建完成）。 */
export function bumpPackUI(packId: string) {
  const next = (generations.get(packId) ?? 0) + 1
  generations.set(packId, next)
  for (const fn of listeners) {
    try {
      fn(packId, next)
    } catch {
      /* ignore listener errors */
    }
  }
}

/** getPackUIGeneration 当前代数。 */
export function getPackUIGeneration(packId: string): number {
  return generations.get(packId) ?? 0
}

/** subscribePackUI 订阅 UI 变更。 */
export function subscribePackUI(fn: PackUIListener): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}
