import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { Subscription, SubscriptionStatus } from './api.ts'
import { accessState, planLine, returnReference, subscriptionActions } from './subscriptionView.ts'

const sub = (status: SubscriptionStatus, complimentary = false): Subscription => ({
  status,
  hasAccess: status === 'active' || status === 'cancelling',
  complimentary,
  paidThrough: null,
  plan: complimentary ? null : { name: 'Monthly', amount: 99, currency: 'ZAR', interval: 'monthly' },
})

test('each status offers its own buttons', () => {
  assert.deepEqual(subscriptionActions(sub('none')), ['subscribe'])
  assert.deepEqual(subscriptionActions(sub('active')), ['updateCard', 'cancel'])
  assert.deepEqual(subscriptionActions(sub('cancelling')), ['updateCard'])
  assert.deepEqual(subscriptionActions(sub('payment_failed')), ['payNow', 'updateCard'])
  assert.deepEqual(subscriptionActions(sub('ended')), ['subscribe'])
})

test('a complimentary member has nothing to do, whatever the status', () => {
  for (const s of ['none', 'active', 'cancelling', 'payment_failed', 'ended'] as const) {
    assert.deepEqual(subscriptionActions(sub(s, true)), [])
  }
})

test('the return reference is read from reference, then trxref', () => {
  assert.equal(returnReference('?trxref=a&reference=a'), 'a')
  assert.equal(returnReference('?trxref=b'), 'b')
  assert.equal(returnReference(''), null)
  assert.equal(returnReference('?reference='), null)
})

test('the plan reads as price and interval', () => {
  assert.equal(planLine({ name: 'Monthly', amount: 99, currency: 'ZAR', interval: 'monthly' }), 'R 99.00 / month')
  assert.equal(planLine({ name: 'Yearly', amount: 1200.5, currency: 'USD', interval: 'annually' }), 'USD 1200.50 / year')
  assert.equal(planLine(null), '')
})

test('paid but still locked means access is being switched on', () => {
  assert.equal(accessState(sub('active'), true), 'switching')
  assert.equal(accessState(sub('cancelling'), true), 'switching')
  assert.equal(accessState(sub('active'), false), 'unlocked')
  assert.equal(accessState(sub('payment_failed'), true), 'none')
  assert.equal(accessState(sub('none'), false), 'none')
})
