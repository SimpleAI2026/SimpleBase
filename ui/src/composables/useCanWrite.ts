import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'

/**
 * 控制台写入口的统一判定：系统（admin）项目与只读角色都隐藏。
 *
 * 前端只读只是体验层，后端才是真正的保护；这里只收敛「哪些入口该显示」的规则。
 */
export function useCanWrite() {
  const { isAdmin } = storeToRefs(useProjectStore())
  const auth = useAuthStore()
  return computed(() => !isAdmin.value && auth.canWrite)
}
