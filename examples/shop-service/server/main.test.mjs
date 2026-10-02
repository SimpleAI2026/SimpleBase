import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { parseOrder } from './validation.mjs'

test('order input validates ids and quantity', () => {
  assert.deepEqual(parseOrder({ productId: 'tea', qty: 2 }), { productId: 'tea', qty: 2 })
  for (const value of [{ productId: 'tea;DROP', qty: 1 }, { productId: 'tea', qty: 0 }, { productId: 'tea', qty: 1.5 }]) assert.throws(() => parseOrder(value))
})
