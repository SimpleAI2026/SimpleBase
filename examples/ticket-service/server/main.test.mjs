import { test } from 'node:test'
import { strict as assert } from 'node:assert'
import { validateTicket } from './validation.mjs'
test('ticket form validates title and priority', () => {
  assert.deepEqual(validateTicket({ title: ' 网络故障 ', priority: 1 }), { title: '网络故障', priority: 1 })
  for (const input of [null, { title: 'x', priority: 1 }, { title: 'issue', priority: 5 }]) assert.throws(() => validateTicket(input))
})
