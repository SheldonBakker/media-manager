import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, ApiError, isLocked, type Subscription } from '../../api'
import { useAuth } from '../../AuthContext'
import ConfirmDialog from '../../components/ConfirmDialog'
import Icon from '../../components/Icon'
import { useToast } from '../../components/Toast'
import { locale, t } from '../../i18n'
import { accessState, planLine, refusal, returnReference, subscriptionActions, type RefusalTone } from '../../subscriptionView'

const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e))
const refusalOf = (e: unknown, reading: boolean) => (e instanceof ApiError ? refusal(e.status, e.code, reading) : refusal(0, undefined, reading))
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
  const { user, setUser } = useAuth()
  const [sub, setSub] = useState<Subscription | null>(null)
  // What the last load or action had to say, and how it is shown.
  const [notice, setNotice] = useState<{ text: string; tone: RefusalTone } | null>(null)
  const [busy, setBusy] = useState(false)
  const [asking, setAsking] = useState(false)
  // The payment reference this load already sent, so React's double effect run
  // in development (and a re-render before the address is cleaned) sends it once.
  const confirmed = useRef<string | null>(null)

  // Reads the account again. Once it is no longer locked the app swaps to its
  // full layout by itself; until then the page says access is being switched on.
  const syncUser = useCallback(async () => {
    try {
      setUser(await api.me())
    } catch (e) {
      setNotice({ text: errorText(e), tone: 'error' })
    }
  }, [setUser])

  const load = useCallback(
    async (syncAccess = false) => {
      setNotice(null)
      try {
        const s = await api.getSubscription()
        setSub(s)
        if (syncAccess && s.hasAccess) await syncUser()
      } catch (e) {
        const { tone } = refusalOf(e, true)
        if (tone === 'calm') {
          setSub(null)
          setNotice({ text: t('This account has no subscription.'), tone })
        } else {
          setNotice({ text: errorText(e), tone })
        }
      }
    },
    [syncUser],
  )

  // An action the server turned down. A 409 means the subscription changed
  // meanwhile (a payment in another tab, a late notice from Paystack), so the
  // page shows what the server now says before the message.
  const turnedDown = useCallback(
    async (e: unknown) => {
      const { tone, reload } = refusalOf(e, false)
      if (reload) await load(true)
      setNotice({ text: errorText(e), tone })
    },
    [load],
  )

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
    setNotice(null)
    api
      .confirmCheckout(reference)
      .then(async (s) => {
        setSub(s)
        navigate(location.pathname, { replace: true })
        if (s.hasAccess) await syncUser()
      })
      .catch((e) => {
        navigate(location.pathname, { replace: true })
        return turnedDown(e)
      })
      .finally(() => setBusy(false))
  }, [location.search, location.pathname, navigate, load, syncUser, turnedDown])

  // Coming back to the tab (from Paystack's card page, say) reads the
  // subscription again, and lets a locked account in once it has paid. A
  // message stays: closing the card tab after a failed link brings the focus
  // back too, and that message is still news. It goes only when the page had
  // nothing else to show and now has.
  const locked = isLocked(user)
  const shown = sub !== null
  useEffect(() => {
    const refresh = async () => {
      try {
        const s = await api.getSubscription()
        setSub(s)
        if (!shown) setNotice(null)
        if (locked && s.hasAccess) await syncUser()
      } catch {
        // Keep what the page shows; Try again is there when it shows nothing.
      }
    }
    const onVisible = () => {
      if (document.visibilityState === 'visible') void refresh()
    }
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('focus', onVisible)
    return () => {
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('focus', onVisible)
    }
  }, [shown, locked, syncUser])

  async function run(job: () => Promise<void>) {
    setBusy(true)
    setNotice(null)
    try {
      await job()
    } catch (e) {
      await turnedDown(e)
    } finally {
      setBusy(false)
    }
  }

  const pay = () =>
    run(async () => {
      window.location.href = (await api.startCheckout()).paymentUrl
    })

  const updateCard = () =>
    run(async () => {
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
      setAsking(false)
      setSub(await api.cancelSubscription())
      toast.success(t('Your subscription is cancelled.'))
    })

  const actions = sub ? subscriptionActions(sub) : []
  const plan = sub ? planLine(sub.plan) : ''
  const switching = sub ? accessState(sub, locked) === 'switching' : false
  const until = sub?.paidThrough ? dateText(sub.paidThrough) : ''

  return (
    <div className="settings-stack">
      <fieldset className="group">
        <legend>
          <Icon name="star" size={14} /> {t('Your subscription')}
        </legend>
        {!sub && !notice && <p style={{ color: 'var(--text-dim)', marginTop: 0 }}>{t('Loading')}</p>}
        {notice?.tone === 'calm' && <p style={{ marginTop: 0 }}>{notice.text}</p>}
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
        {switching && (
          <>
            <p>{t('Your subscription is active. Your access is being switched on, which can take a minute.')}</p>
            <button onClick={() => void run(syncUser)} disabled={busy}>
              {t('Check again')}
            </button>
          </>
        )}
        {notice?.tone === 'info' && (
          <div className="notice notice-info" role="status">
            {notice.text}
          </div>
        )}
        {notice?.tone === 'error' && (
          <p role="alert" style={{ color: 'var(--danger)' }}>
            {notice.text}
          </p>
        )}
        {!sub && notice?.tone === 'error' && (
          <button onClick={() => void run(() => load())} disabled={busy}>
            {t('Try again')}
          </button>
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
