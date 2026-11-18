import { toast } from 'vue-sonner'

/**
 * 复制文本到剪贴板并给出提示（planv6.7 ui-simplify-ux-plan C10）。
 * 返回是否成功，调用方需要时可据此继续。
 */
export async function copyText(text: string, okMessage: string, failMessage: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(okMessage)
    return true
  } catch {
    toast.error(failMessage)
    return false
  }
}
