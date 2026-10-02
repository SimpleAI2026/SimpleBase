export function validSlot(input) {
  if (!input || typeof input.slotId !== 'string' || !/^[a-z0-9-]{1,40}$/.test(input.slotId)) throw new Error('Invalid slot')
  return input.slotId
}
