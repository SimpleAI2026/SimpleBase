import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { validSlot } from './validation.mjs'
test('slot ids are scoped identifiers', () => {
  assert.equal(validSlot({ slotId: 'demo-slot' }), 'demo-slot')
  for (const input of [null, { slotId: '../admin' }, { slotId: '' }]) assert.throws(() => validSlot(input))
})
