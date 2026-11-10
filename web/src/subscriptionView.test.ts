import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { Subscription, SubscriptionStatus } from './api.ts'
import { accessState, isCheckoutReturn, planLine, refusal, subscriptionActions } from './subscriptionView.ts'

const sub = (status: SubscriptionStatus, complimentary = false): Subscription => ({
  status,
  hasAccess: status === 'active' || status === 'cancelling',
  complimentary,
  paidThrough: null,
  plan: complimentary ? null : { name: 'Monthly', amount: 60, currency: 'ZAR', interval: 'monthly' },
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

test('the return from the payment page is recognised by checkout=return', () => {
  assert.equal(isCheckoutReturn('?checkout=return'), true)
  assert.equal(isCheckoutReturn('?checkout=return&trxref=a&reference=a'), true)
  assert.equal(isCheckoutReturn(''), false)
  assert.equal(isCheckoutReturn('?reference=a'), false)
  assert.equal(isCheckoutReturn('?checkout=other'), false)
})

test('the plan reads as price and interval, in the reader\'s locale', () => {
  // Intl keeps a letter symbol off the digits with a no-break space.
  assert.equal(planLine({ name: 'Monthly', amount: 60, currency: 'ZAR', interval: 'monthly' }, 'en'), 'R\u00a060.00 / month')
  assert.equal(planLine({ name: 'Yearly', amount: 1200.5, currency: 'USD', interval: 'annually' }, 'en'), '$1,200.50 / year')
  assert.equal(planLine({ name: 'Monthly', amount: 60, currency: 'ZAR', interval: 'monthly' }, 'en-ZA'), 'R\u00a060,00 / month')
  assert.equal(planLine(null), '')
})

test('paid but still locked means access is being switched on', () => {
  assert.equal(accessState(sub('active'), true), 'switching')
  assert.equal(accessState(sub('cancelling'), true), 'switching')
  assert.equal(accessState(sub('active'), false), 'unlocked')
  assert.equal(accessState(sub('payment_failed'), true), 'none')
  assert.equal(accessState(sub('none'), false), 'none')
})

test('an account the Coordinator does not know simply has no subscription', () => {
  assert.deepEqual(refusal(404, 'Subscription.MemberNotFound', true), { tone: 'calm', reload: false })
  assert.deepEqual(refusal(404, undefined, true), { tone: 'calm', reload: false })
})

test('any other failure to read the subscription is an error', () => {
  assert.deepEqual(refusal(502, undefined, true), { tone: 'error', reload: false })
  assert.deepEqual(refusal(0, undefined, true), { tone: 'error', reload: false })
})

test('a 409 on an action reads the subscription again', () => {
  assert.deepEqual(refusal(409, 'StartCheckout.AlreadyPaid', false), { tone: 'info', reload: true })
  assert.deepEqual(refusal(409, 'StartCheckout.NotAllowed', false), { tone: 'error', reload: true })
  assert.deepEqual(refusal(409, 'CancelSubscription.NotAllowed', false), { tone: 'error', reload: true })
  assert.deepEqual(refusal(409, 'ConfirmCheckout.NotPaid', false), { tone: 'error', reload: true })
})

test('any other failed action is an error and changes nothing on the page', () => {
  assert.deepEqual(refusal(500, undefined, false), { tone: 'error', reload: false })
  assert.deepEqual(refusal(404, undefined, false), { tone: 'error', reload: false })
  assert.deepEqual(refusal(0, undefined, false), { tone: 'error', reload: false })
})
