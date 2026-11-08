import { createPortal } from 'react-dom'
import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { api } from '../api'
import { t, tx } from '../i18n'
import { resetBody, resetErrors } from '../passwordResetForm'
import { ACCOUNT_PASSWORD_MAX, ACCOUNT_PASSWORD_MIN, accountEmailError } from '../signupForm'
import { useFocusTrap } from '../useFocusTrap'
import { useResendWait } from '../useResendWait'
import { FieldError, FormProblem, useValidation } from '../useValidation'
import AdminPasswordHelp from './AdminPasswordHelp'
import Icon from './Icon'

const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e))

// "Forgot your password?" on the sign-in page, for accounts kept by the account
// service, in two steps: the email address, then the emailed code with the new
// password. The password changes there, so it changes for everything that signs
// in with it. The server answers the same whether or not an account uses the
// address, so nothing here says one was found. Everything typed lives only in
// this component's state, so closing the dialog (which unmounts it) forgets the
// password and the code.
export default function ForgotPasswordDialog({ onClose }: { onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null)
  useFocusTrap(box)
  const titleId = useId()
  const [step, setStep] = useState<'email' | 'reset' | 'done'>('email')
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  // The address the last code went to.
  const [sentTo, setSentTo] = useState('')
  const wait = useResendWait()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<'' | 'send' | 'reset'>('')
  const ev = useValidation({ email: accountEmailError(email) })
  const fields = { code, password, confirmPassword }
  const rv = useValidation(resetErrors(fields))
  // Going back without changing the address needs no new code.
  const codeSent = sentTo !== '' && sentTo === email.trim()

  const close = useCallback(() => {
    if (!busy) onClose()
  }, [busy, onClose])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && close()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [close])

  // Emails a code to the address. True when the server took the request.
  async function sendCode(): Promise<boolean> {
    const to = email.trim()
    setError('')
    setBusy('send')
    try {
      await api.sendPasswordResetCode(to)
      setSentTo(to)
      setCode('')
      rv.reset()
      wait.start()
      return true
    } catch (err) {
      setError(errorText(err))
      return false
    } finally {
      setBusy('')
    }
  }

  async function submitEmail(e: FormEvent) {
    e.preventDefault()
    if (!ev.attempt()) return
    if (codeSent || (await sendCode())) {
      setError('')
      setStep('reset')
    }
  }

  async function submitReset(e: FormEvent) {
    e.preventDefault()
    if (!rv.attempt()) return
    setError('')
    setBusy('reset')
    try {
      await api.resetPassword(resetBody(sentTo, fields))
      // The code and the new password are not needed any more.
      setCode('')
      setPassword('')
      setConfirmPassword('')
      setStep('done')
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy('')
    }
  }

  function backToEmail() {
    setError('')
    setStep('email')
  }

  // For accounts Mediarium keeps itself, such as the administrator's.
  const adminHelp = (
    <details className="forgot">
      <summary>{t('Administrator account?')}</summary>
      <AdminPasswordHelp emailReset />
    </details>
  )

  return createPortal(
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div ref={box} tabIndex={-1} className="modal forgot-dialog" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <div className="modal-head">
          <h2 id={titleId} style={{ flex: 1, margin: 0 }}>
            {t('Forgot your password?')}
          </h2>
          <button type="button" className="icon-btn" onClick={close} disabled={!!busy} aria-label={t('Close')}>
            <Icon name="x" />
          </button>
        </div>
        {step === 'done' ? (
          <>
            <div className="modal-body" role="status">
              <p style={{ margin: 0 }}>{t('Your password was changed. Sign in with your username and new password. It works for Emby and Mediarium.')}</p>
            </div>
            <div className="modal-foot">
              <button type="button" className="primary" onClick={close} autoFocus>
                {t('Back to sign in')}
              </button>
            </div>
          </>
        ) : step === 'email' ? (
          <form key="email" onSubmit={(e) => void submitEmail(e)} noValidate>
            <div className="modal-body account-form">
              <p style={{ margin: 0 }}>{t("Enter the email address of your account. We'll email you a code to set a new password.")}</p>
              <label>
                {t('Email')}
                <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" autoFocus {...ev.bind('email', email, setEmail)} />
                <FieldError v={ev} name="email" />
              </label>
              {error && <p className="error-text">{error}</p>}
              {adminHelp}
            </div>
            <div className="modal-foot">
              <button type="button" onClick={close} disabled={!!busy}>
                {t('Cancel')}
              </button>
              <button className="primary" type="submit" disabled={!!busy}>
                {busy ? t('Sending…') : codeSent ? t('Continue') : t('Send code')}
              </button>
            </div>
          </form>
        ) : (
          <form key="reset" onSubmit={(e) => void submitReset(e)} noValidate>
            <div className="modal-body account-form">
              <p style={{ margin: 0 }}>
                {tx('If an account uses {email}, we sent it a 6-digit code. Enter it with your new password. The code expires in 15 minutes.', { email: <strong>{sentTo}</strong> })}
              </p>
              <label>
                {t('Six-digit code')}
                <input value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" autoComplete="one-time-code" className="code-input" autoFocus {...rv.bind('code', code, setCode)} />
                <FieldError v={rv} name="code" />
              </label>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px 20px' }}>
                <button type="button" className="link-btn" onClick={() => void sendCode()} disabled={!!busy || wait.seconds > 0}>
                  {busy === 'send' ? t('Sending…') : wait.seconds > 0 ? t('Resend code ({n}s)', { n: wait.seconds }) : t('Resend code')}
                </button>
                <button type="button" className="link-btn" onClick={backToEmail} disabled={!!busy}>
                  {t('Use another email')}
                </button>
              </div>
              <div className="form-cols">
                <label>
                  {t('New password')}
                  <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" {...rv.bind('password')} />
                  <FieldError v={rv} name="password" />
                  <small className="field-hint">{t('{min} to {max} characters.', { min: ACCOUNT_PASSWORD_MIN, max: ACCOUNT_PASSWORD_MAX })}</small>
                </label>
                <label>
                  {t('Confirm new password')}
                  <input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} autoComplete="new-password" {...rv.bind('confirmPassword')} />
                  <FieldError v={rv} name="confirmPassword" />
                </label>
              </div>
              {error && <p className="error-text">{error}</p>}
              <FormProblem v={rv} action="password" />
              {adminHelp}
            </div>
            <div className="modal-foot">
              <button type="button" onClick={close} disabled={!!busy}>
                {t('Cancel')}
              </button>
              <button className="primary" type="submit" disabled={!!busy}>
                {busy === 'reset' ? t('Saving…') : t('Change password')}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>,
    document.body,
  )
}
