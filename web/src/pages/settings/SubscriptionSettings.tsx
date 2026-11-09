import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, ApiError, type Subscription } from '../../api'
import ConfirmDialog from '../../components/ConfirmDialog'
import Icon from '../../components/Icon'
import { useToast } from '../../components/Toast'
import { locale, t } from '../../i18n'
import { planLine, returnReference, subscriptionActions } from '../../subscriptionView'

const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e))
const dateText = (iso: string) => new Date(iso).toLocaleDateString(locale())

// The sentence under the status: what the person has and until when.
function statusLine(s: Subscription): string {
  if (s.complimentary) return t('Your access is free.')
  const until = s.paidThrough ? dateText(s.paidThrough) : ''
  switch (s.status) {
    case 'active':
      return until ? t('Your subscription is active. Next payment on {date}.', { date: until }) : t('Your subscription is active.')
    case 'cancelling':
      return until ? t('Your subscription is cancelled. Access until {date}.', { date: until }) : t('Your subscription is cancelled.')
    case 'payment_failed':
      return t('Your last payment did not go through. Pay now or update your card to keep your access.')
    case 'ended':
      return t('Your subscription has ended. Subscribe to get your access back.')
    default:
      return t('You do not have a subscription yet. Subscribe to start using Mediarium.')
  }
}

// Settings > Account & Access > Subscription: the plan, the card, and
// subscribing, paying a failed payment or cancelling. Payment itself happens
// on Paystack's pages; this page only sends people there and back.
export default function SubscriptionSettings() {
  const location = useLocation()
  const navigate = useNavigate()
  const toast = useToast()
  const [sub, setSub] = useState<Subscription | null>(null)
  const [problem, setProblem] = useState('')
  const [busy, setBusy] = useState(false)
  const [asking, setAsking] = useState(false)
  // The payment reference this load already sent, so React's double effect run
  // in development (and a re-render before the address is cleaned) sends it once.
  const confirmed = useRef<string | null>(null)

  const load = useCallback(async () => {
    try {
      setSub(await api.getSubscription())
    } catch (e) {
      setProblem(errorText(e))
    }
  }, [])

  useEffect(() => {
    const reference = returnReference(location.search)
    if (!reference) {
      // After a return the confirm answer is already showing; do not replace it or its message.
      if (confirmed.current === null) void load()
      return
    }
    if (confirmed.current === reference) return
    confirmed.current = reference
    setBusy(true)
    api
      .confirmCheckout(reference)
      .then((s) => {
        setSub(s)
        setProblem('')
      })
      .catch((e) => {
        setProblem(errorText(e))
        // The payment may have gone through even so (a second tab, a late notice): show what the server now says.
        if (e instanceof ApiError && e.status === 409) void load()
      })
      .finally(() => {
        setBusy(false)
        navigate(location.pathname, { replace: true })
      })
  }, [location.search, location.pathname, navigate, load])

  async function run(job: () => Promise<void>) {
    setBusy(true)
    try {
      await job()
    } catch (e) {
      setProblem(errorText(e))
      // An earlier payment went through: the subscription is already active.
      if (e instanceof ApiError && e.code === 'StartCheckout.AlreadyPaid') void load()
    } finally {
      setBusy(false)
    }
  }

  const pay = () =>
    run(async () => {
      setProblem('')
      window.location.href = (await api.startCheckout()).paymentUrl
    })

  const updateCard = () =>
    run(async () => {
      setProblem('')
      // Open the tab first, while this is still the click, or the browser blocks it; the link is fetched fresh each time and never kept.
      const tab = window.open('', '_blank')
      try {
        const { url } = await api.manageLink()
        if (tab) {
          tab.opener = null
          tab.location.href = url
        } else {
          window.location.href = url
        }
      } catch (e) {
        tab?.close()
        throw e
      }
    })

  const cancel = () =>
    run(async () => {
      setProblem('')
      setAsking(false)
      setSub(await api.cancelSubscription())
      toast.success(t('Your subscription is cancelled.'))
    })

  const actions = sub ? subscriptionActions(sub) : []
  const plan = sub ? planLine(sub.plan) : ''
  const until = sub?.paidThrough ? dateText(sub.paidThrough) : ''

  return (
    <div className="settings-stack">
      <fieldset className="group">
        <legend>
          <Icon name="star" size={14} /> {t('Your subscription')}
        </legend>
        {!sub && !problem && <p style={{ color: 'var(--text-dim)', marginTop: 0 }}>{t('Loading')}</p>}
        {sub && (
          <>
            <p style={{ marginTop: 0 }}>{statusLine(sub)}</p>
            {plan && (
              <p style={{ color: 'var(--text-dim)' }}>
                {sub.plan?.name} · {plan}
              </p>
            )}
            {sub.status === 'cancelling' && until && <p style={{ color: 'var(--text-dim)' }}>{t('You can subscribe again after {date}.', { date: until })}</p>}
          </>
        )}
        {problem && (
          <p role="alert" style={{ color: 'var(--danger)' }}>
            {problem}
          </p>
        )}
        {actions.length > 0 && (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {actions.includes('subscribe') && (
              <button className="primary" onClick={() => void pay()} disabled={busy}>
                {t('Subscribe')}
              </button>
            )}
            {actions.includes('payNow') && (
              <button className="primary" onClick={() => void pay()} disabled={busy}>
                {t('Pay now')}
              </button>
            )}
            {actions.includes('updateCard') && (
              <button onClick={() => void updateCard()} disabled={busy}>
                {t('Update card')}
              </button>
            )}
            {actions.includes('cancel') && (
              <button className="danger-ghost" onClick={() => setAsking(true)} disabled={busy}>
                {t('Cancel subscription')}
              </button>
            )}
          </div>
        )}
      </fieldset>
      {asking && (
        <ConfirmDialog
          title={t('Cancel your subscription?')}
          confirmLabel={t('Cancel subscription')}
          danger
          onConfirm={() => void cancel()}
          onCancel={() => setAsking(false)}
        >
          {until ? t('Your access lasts until {date}. After that you will need to subscribe again.', { date: until }) : t('Your access lasts until the end of the period you paid for. After that you will need to subscribe again.')}
        </ConfirmDialog>
      )}
    </div>
  )
}
