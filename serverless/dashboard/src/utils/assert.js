export function assert(expr, message, ...params) {
  if (!expr) {
    console.error(message, params)
    throw new Error(message)
  }
}
