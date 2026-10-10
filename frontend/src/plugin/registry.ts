import type { ComponentType } from 'react'
import { PRODUCT_VIEWS } from '../shell/productViews'
import type { ProductId } from '../shell/products'

/** Pack overlay：覆盖 builtin 产品视图（例如动态加载的 pack UI）。 */
const overlays = new Map<ProductId, ComponentType>()

/** registerPackOverlay 注册 pack 产品视图覆盖。 */
export function registerPackOverlay(id: ProductId, view: ComponentType) {
  overlays.set(id, view)
}

/** clearPackOverlay 清除覆盖，回退 builtin。 */
export function clearPackOverlay(id: ProductId) {
  overlays.delete(id)
}

/** getProductView 解析产品视图：pack overlay 优先，否则 builtin。 */
export function getProductView(id: ProductId): ComponentType {
  return overlays.get(id) ?? PRODUCT_VIEWS[id]
}

/** listRegisteredProductIds 全部已注册产品线 id。 */
export function listRegisteredProductIds(): ProductId[] {
  return Object.keys(PRODUCT_VIEWS) as ProductId[]
}

/** resolveProductViews 当前生效的产品视图表（供 AppShell 渲染）。 */
export function resolveProductViews(): Record<ProductId, ComponentType> {
  const out = { ...PRODUCT_VIEWS } as Record<ProductId, ComponentType>
  for (const [id, view] of overlays) {
    out[id] = view
  }
  return out
}
