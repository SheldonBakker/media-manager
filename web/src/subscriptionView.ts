import type { Subscription } from './api.ts'
import { t } from './i18n.ts'

export type SubscriptionAction = 'subscribe' | 'payNow' | 'updateCard' | 'cancel'

// The buttons the Subscription page shows. A complimentary member has nothing
// to pay or cancel; Paystack cannot resume a cancelled subscription, so while
// one is ending the only way back is to subscribe again after it ends.
export function subscriptionActions(s: Subscription): SubscriptionAction[] {
  if (s.complimentary) return []
  switch (s.status) {
    case 'active':
      return ['updateCard', 'cancel']
    case 'cancelling':
      return ['updateCard']
    case 'payment_failed':
      return ['payNow', 'updateCard']
    default:
      return ['subscribe']
  }
}

// The payment reference Paystack puts on the address when it sends the person
// back: `reference`, or `trxref` when that is missing.
export function returnReference(search: string): string | null {
  const q = new URLSearchParams(search)
  return q.get('reference') || q.get('trxref') || null
}

// How often a plan is charged, as the words after the price.
function every(interval: string): string {
  switch (interval) {
    case 'hourly':
      return t('hour')
    case 'daily':
      return t('day')
    case 'weekly':
      return t('week')
    case 'monthly':
      return t('month')
    case 'quarterly':
      return t('quarter')
    case 'biannually':
      return t('6 months')
    case 'annually':
      return t('year')
    default:
      return interval
  }
}

// "R 99.00 / month": the price and how often it is charged, empty for a member
// with no plan.
export function planLine(plan: Subscription['plan']): string {
  if (!plan) return ''
  const money = plan.currency === 'ZAR' ? 'R' : plan.currency
  return `${money} ${plan.amount.toFixed(2)} / ${every(plan.interval)}`
}

export type AccessState = 'none' | 'switching' | 'unlocked'

// Where a member stands once the subscription is known: no access yet, paid
// but the account still reads as locked (access is being switched on), or open.
export function accessState(s: Subscription, locked: boolean): AccessState {
  if (!s.hasAccess) return 'none'
  return locked ? 'switching' : 'unlocked'
}
