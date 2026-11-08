import { createPortal } from 'react-dom'
import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { api, isSignupPending } from '../api'
import { t, tx } from '../i18n'
import { ACCOUNT_PASSWORD_MAX, ACCOUNT_PASSWORD_MIN, emailedCodeError, signupBody, signupErrors } from '../signupForm'
import { useFocusTrap } from '../useFocusTrap'
import { useResendWait } from '../useResendWait'
import { FieldError, FormProblem, useValidation } from '../useValidation'
import Icon from './Icon'

const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e))

// "Create an account" on the sign-in page, in two steps: the person's details,
// then the code emailed to them to confirm the address. The account service
// makes the account: either the person is signed in straight away, or the
// account is still being set up and they sign in with the username it was
// given a few minutes later. Closing then hands that username to the sign-in
// form. Everything typed lives only in this component's state, so closing the
// dialog (which unmounts it) forgets the password and the code.
export default function SignupDialog({ onSignedIn, onClose }: { onSignedIn: () => Promise<void>; onClose: (username?: string) => void }) {
  const box = useRef<HTMLDivElement>(null)
  useFocusTrap(box)
  const titleId = useId()
  const [step, setStep] = useState<'details' | 'code'>('details')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState('')
  const [phoneNumber, setPhoneNumber] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [code, setCode] = useState('')
  // The address the last code went to.
  const [sentTo, setSentTo] = useState('')
  const wait = useResendWait()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<'' | 'send' | 'create'>('')
  // The username of an account that is still being set up.
  const [created, setCreated] = useState('')
  const fields = { firstName, lastName, email, phoneNumber, password, confirmPassword }
  const v = useValidation(signupErrors(fields))
  const cv = useValidation({ code: emailedCodeError(code) })
  // Going back only to fix a name or the phone number needs no new code.
  const codeSent = sentTo !== '' && sentTo === email.trim()

  const close = useCallback(() => {
    if (!busy) onClose(created || undefined)
  }, [busy, created, onClose])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && close()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [close])

  // Emails a code to the address. True when it is on its way.
  async function sendCode(): Promise<boolean> {
    const to = email.trim()
    setError('')
    setBusy('send')
    try {
      await api.sendSignupCode(to)
      setSentTo(to)
      setCode('')
      cv.reset()
      wait.start()
      return true
    } catch (err) {
      setError(errorText(err))
      return false
    } finally {
      setBusy('')
    }
  }

  async function submitDetails(e: FormEvent) {
    e.preventDefault()
    if (!v.attempt()) return
    if (codeSent || (await sendCode())) {
      setError('')
      setStep('code')
    }
  }

  async function submitCode(e: FormEvent) {
    e.preventDefault()
    if (!cv.attempt()) return
    setError('')
    setBusy('create')
    try {
      const res = await api.signup(signupBody(fields, code))
      if (isSignupPending(res)) {
        setCreated(res.username)
        return
      }
      await onSignedIn()
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy('')
    }
  }

  function changeDetails() {
    setError('')
    setStep('details')
  }

  return createPortal(
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div ref={box} tabIndex={-1} className="modal" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <div className="modal-head">
          <h2 id={titleId} style={{ flex: 1, margin: 0 }}>
            {t('Create an account')}
          </h2>
          <button type="button" className="icon-btn" onClick={close} disabled={!!busy} aria-label={t('Close')}>
            <Icon name="x" />
          </button>
        </div>
        {created ? (
          <>
            <div className="modal-body" role="status">
              <p style={{ margin: 0 }}>
                {tx('Your account was created. Your username is {username}. It is still being set up; sign in with that username and your password in a few minutes.', {
                  username: <strong>{created}</strong>,
                })}
              </p>
            </div>
            <div className="modal-foot">
              <button type="button" className="primary" onClick={close} autoFocus>
                {t('Back to sign in')}
              </button>
            </div>
          </>
        ) : step === 'details' ? (
          <form key="details" onSubmit={(e) => void submitDetails(e)} noValidate>
            <div className="modal-body account-form">
              <div className="form-cols">
                <label>
                  {t('First name')}
                  <input value={firstName} onChange={(e) => setFirstName(e.target.value)} autoComplete="given-name" autoFocus {...v.bind('firstName', firstName, setFirstName)} />
                  <FieldError v={v} name="firstName" />
                </label>
                <label>
                  {t('Last name')}
                  <input value={lastName} onChange={(e) => setLastName(e.target.value)} autoComplete="family-name" {...v.bind('lastName', lastName, setLastName)} />
                  <FieldError v={v} name="lastName" />
                </label>
                <label>
                  {t('Email')}
                  <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" {...v.bind('email', email, setEmail)} />
                  <FieldError v={v} name="email" />
                </label>
                <label>
                  {t('Phone number')}
                  <input type="tel" value={phoneNumber} onChange={(e) => setPhoneNumber(e.target.value)} autoComplete="tel" {...v.bind('phoneNumber', phoneNumber, setPhoneNumber)} />
                  <FieldError v={v} name="phoneNumber" />
                  <small className="field-hint">{t('International format, for example +27821234567')}</small>
                </label>
                <label>
                  {t('Password')}
                  <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" {...v.bind('password')} />
                  <FieldError v={v} name="password" />
                  <small className="field-hint">{t('{min} to {max} characters.', { min: ACCOUNT_PASSWORD_MIN, max: ACCOUNT_PASSWORD_MAX })}</small>
                </label>
                <label>
                  {t('Confirm password')}
                  <input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} autoComplete="new-password" {...v.bind('confirmPassword')} />
                  <FieldError v={v} name="confirmPassword" />
                </label>
              </div>
              {error && <p className="error-text">{error}</p>}
              <FormProblem v={v} action="signup" />
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
          <form key="code" onSubmit={(e) => void submitCode(e)} noValidate>
            <div className="modal-body account-form">
              <p style={{ margin: 0 }}>
                {tx('We sent a 6-digit code to {email}. Enter it to create your account. It expires in 15 minutes.', { email: <strong>{sentTo}</strong> })}
              </p>
              <label>
                {t('Six-digit code')}
                <input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  className="code-input"
                  autoFocus
                  {...cv.bind('code', code, setCode)}
                />
                <FieldError v={cv} name="code" />
              </label>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px 20px' }}>
                <button type="button" className="link-btn" onClick={() => void sendCode()} disabled={!!busy || wait.seconds > 0}>
                  {busy === 'send' ? t('Sending…') : wait.seconds > 0 ? t('Resend code ({n}s)', { n: wait.seconds }) : t('Resend code')}
                </button>
                <button type="button" className="link-btn" onClick={changeDetails} disabled={!!busy}>
                  {t('Change details')}
                </button>
              </div>
              {error && <p className="error-text">{error}</p>}
            </div>
            <div className="modal-foot">
              <button type="button" onClick={close} disabled={!!busy}>
                {t('Cancel')}
              </button>
              <button className="primary" type="submit" disabled={!!busy}>
                {busy === 'create' ? t('Creating…') : t('Create account')}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>,
    document.body,
  )
}
