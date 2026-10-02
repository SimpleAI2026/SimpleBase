export function validateTicket(input) {
  if (typeof input?.title !== 'string' || input.title.trim().length < 2 || input.title.length > 120 || !Number.isInteger(input.priority) || input.priority < 1 || input.priority > 3) throw new Error('Invalid ticket')
  return { title: input.title.trim(), priority: input.priority }
}
