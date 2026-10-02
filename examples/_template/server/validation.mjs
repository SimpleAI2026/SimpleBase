export function validateItem(input) {
  const title = typeof input?.title === 'string' ? input.title.trim() : ''
  if (!title || title.length > 80) throw new Error('Invalid title')
  return title
}
