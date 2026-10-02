export function parseOrder(input) {
  if (!input || typeof input.productId !== 'string' || !/^[a-z]{1,32}$/.test(input.productId) || !Number.isSafeInteger(input.qty) || input.qty < 1 || input.qty > 10) throw new Error('Invalid product or quantity')
  return { productId: input.productId, qty: input.qty }
}
