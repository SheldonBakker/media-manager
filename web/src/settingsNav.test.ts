import assert from 'node:assert/strict'
import { test } from 'node:test'
import { settingsGroupsFor, settingsNavFor } from './settingsNav.ts'

const SUB = '/settings/subscription'
const paths = (admin: boolean, viewer: boolean, available: boolean, locked: boolean) => settingsNavFor(admin, viewer, { available, locked }).map((p) => p.to)

test('a locked member sees only profile, security and subscription', () => {
  assert.deepEqual(paths(false, false, true, true), ['/settings/profile', '/settings/security', SUB])
})

test('an unlocked member gets the subscription page next to the normal ones', () => {
  const normal = paths(false, false, false, false)
  assert.ok(!normal.includes(SUB))
  assert.deepEqual(paths(false, false, true, false).filter((p) => p !== SUB), normal)
  assert.ok(paths(false, false, true, false).includes(SUB))
})

test('an administrator sees the subscription page only when it is available', () => {
  assert.ok(!paths(true, false, false, false).includes(SUB))
  assert.ok(paths(true, false, true, false).includes(SUB))
})

test('the subscription page sits in Account & Access after Security', () => {
  const group = settingsGroupsFor(false, false, { available: true, locked: true })
  assert.deepEqual(group.map((g) => g.pages.map((p) => p.to)), [['/settings/profile', '/settings/security', SUB]])
})
