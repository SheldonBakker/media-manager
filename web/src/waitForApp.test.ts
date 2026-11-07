import assert from 'node:assert/strict'
import { test } from 'node:test'
import { waitForApp } from './waitForApp.ts'

// A fake clock: sleeping moves time forward.
function clock() {
  let t = 0
  return { now: () => t, sleep: async (ms: number) => void (t += ms) }
}

test('waiting for the app: it is not believed before the minimum time', async () => {
  const c = clock()
  const seen: number[] = []
  const result = await waitForApp({
    ...c,
    fetchVersion: async () => {
      seen.push(c.now())
      return '1.2.0' // the old process still answers at first
    },
    minMs: 4000,
  })
  assert.equal(result, 'back')
  assert.ok(seen[0] < 4000 && c.now() >= 4000)
})

test('waiting for the app: it keeps waiting through the gap while it restarts', async () => {
  const c = clock()
  const answers = ['1.1.0', null, null, null, '1.2.0']
  let i = 0
  const result = await waitForApp({ ...c, expectVersion: '1.2.0', minMs: 0, fetchVersion: async () => answers[Math.min(i++, answers.length - 1)] })
  assert.equal(result, 'back')
  assert.equal(i, 5, 'the old version must not count as back')
})

test('waiting for the app: it gives up in the end', async () => {
  const c = clock()
  const result = await waitForApp({ ...c, fetchVersion: async () => null, maxMs: 20000 })
  assert.equal(result, 'timeout')
  assert.ok(c.now() > 20000 && c.now() <= 24000)
})
