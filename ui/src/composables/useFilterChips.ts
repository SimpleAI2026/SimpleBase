import { ref } from 'vue'
import { pushHistory } from '@/utils/bi-filter'
import type { FilterChip } from '@/utils/bi-filter'

/**
 * 看板的全局条件：表达式草稿 / 已应用的表达式 / 历史 / 联动 chip。
 *
 * 时间范围（range）与时间字段由页面持有——它们同时参与 `globalFilter` 的组合与「清除」按钮，
 * 这里不碰，页面的 `clearFilters` 在调用 `reset()` 之后自己把 range 恢复。
 */
export function useFilterChips() {
  const draft = ref('')
  const applied = ref('')
  const history = ref<string[]>([])
  const chips = ref<FilterChip[]>([])

  /** 把草稿提交为生效条件，并记入历史（去重 / 截断见 pushHistory）。 */
  function apply() {
    applied.value = draft.value.trim()
    history.value = pushHistory(history.value, applied.value)
  }

  /** 往草稿末尾追加字段名：非空且不以空格结尾时用 && 连接。 */
  function appendField(name: string) {
    draft.value += (draft.value && !draft.value.endsWith(' ') ? ' && ' : '') + name
  }

  /** 历史项回填并立即应用。 */
  function useHistory(filter: string) {
    draft.value = filter
    apply()
  }

  function addChip(chip: FilterChip) {
    if (!chips.value.some((c) => c.id === chip.id)) chips.value = [...chips.value, chip]
  }

  function removeChip(id: string) {
    chips.value = chips.value.filter((c) => c.id !== id)
  }

  /** 清掉表达式与 chip（历史保留）：「清除条件」按钮用。 */
  function clear() {
    draft.value = ''
    applied.value = ''
    chips.value = []
  }

  /** 数据源切换：连历史一起清（历史里的表达式引用的是上一个数据源的字段）。 */
  function reset() {
    clear()
    history.value = []
  }

  return { draft, applied, history, chips, apply, appendField, useHistory, addChip, removeChip, clear, reset }
}
