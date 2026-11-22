import { toast } from 'vue-sonner'

/**
 * 复制文本到剪贴板并给出提示（planv6.7 ui-simplify-ux-plan C10）。
 * 成功文案可延迟求值，失败文案回调接收原始错误；返回是否成功。
 */
export async function copyText(
  text: string,
  okMessage: string | (() => string),
  failMessage: string | ((error: unknown) => string)
): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(typeof okMessage === 'function' ? okMessage() : okMessage)
    return true
  } catch (error) {
    toast.error(typeof failMessage === 'function' ? failMessage(error) : failMessage)
    return false
  }
}
