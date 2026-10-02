import test from 'node:test'
import assert from 'node:assert/strict'
import { validateItem } from './validation.mjs'

test('item titles are validated', () => {
  assert.equal(validateItem({ title: ' hello ' }), 'hello')
  assert.throws(() => validateItem({ title: '' }))
  assert.throws(() => validateItem({}))
  assert.throws(() => validateItem({ title: 'x'.repeat(81) }))
})
