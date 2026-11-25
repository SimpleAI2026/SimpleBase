import { toast } from 'vue-sonner'

/** 老浏览器 / 非安全上下文没有 Clipboard API 时的降级：临时 textarea + execCommand('copy')。 */
function copyViaExecCommand(text: string): boolean {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  try {
    ta.select()
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    ta.remove()
  }
}

/**
 * 复制文本到剪贴板并给出提示。
 * 先用 Clipboard API，不可用 / 被拒时降级 execCommand；两者都失败才提示失败。
 * 成功文案可延迟求值，失败文案回调接收原始错误；返回是否成功。
 */
export async function copyText(
  text: string,
  okMessage: string | (() => string),
  failMessage: string | ((error: unknown) => string)
): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
  } catch (error) {
    if (!copyViaExecCommand(text)) {
      toast.error(typeof failMessage === 'function' ? failMessage(error) : failMessage)
      return false
    }
  }
  toast.success(typeof okMessage === 'function' ? okMessage() : okMessage)
  return true
}
