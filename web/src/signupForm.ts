// The checks and the details sent for "Create an account" on the sign-in page.
// The account is made by the account service, so these are its rules: a form
// that passes here is never refused there for how it is filled in. The email,
// new password and emailed code rules are shared with "Forgot your password?"
// (passwordResetForm.ts). Free of React so it can be tested with plain node
// (see signupForm.test.ts).
import type { SignupDetails } from './api.ts'
import { t } from './i18n.ts'
import { email, firstError, maxLength, passwordLength, passwordsMatch, phone, required, type Message } from './validate.ts'

export const SIGNUP_NAME_MAX = 100
export const ACCOUNT_PASSWORD_MIN = 12
export const ACCOUNT_PASSWORD_MAX = 128

// The first step of the form. The emailed code that confirms the address is the second.
export interface SignupFields extends Omit<SignupDetails, 'code'> {
  confirmPassword: string
}

/** The email address of an account in the account service. */
export function accountEmailError(value: string): Message {
  return firstError(required(value, t('Enter your email address.')), email(value))
}

/** A new password for the account service, and the same typed again. */
export function newPasswordErrors(password: string, again: string): { password: Message; confirmPassword: Message } {
  return {
    password: firstError(
      required(password, t('Choose a password of at least {n} characters.', { n: ACCOUNT_PASSWORD_MIN })),
      passwordLength(password, ACCOUNT_PASSWORD_MIN, ACCOUNT_PASSWORD_MAX),
    ),
    confirmPassword: firstError(required(again, t('Type your password again to make sure it is right.')), passwordsMatch(password, again)),
  }
}

/** The message for each field, or null when it is fine, in the order the form shows them. */
export function signupErrors(f: SignupFields): Record<keyof SignupFields, Message> {
  return {
    firstName: firstError(required(f.firstName, t('Enter your first name.')), maxLength(f.firstName, SIGNUP_NAME_MAX, t('Your first name'))),
    lastName: firstError(required(f.lastName, t('Enter your last name.')), maxLength(f.lastName, SIGNUP_NAME_MAX, t('Your last name'))),
    email: accountEmailError(f.email),
    phoneNumber: firstError(required(f.phoneNumber, t('Enter your phone number.')), phone(f.phoneNumber)),
    ...newPasswordErrors(f.password, f.confirmPassword),
  }
}

/** An emailed code as sent: it may be typed or pasted with spaces in it. */
export const codeWithoutSpaces = (code: string) => code.replace(/\s/g, '')

/** A code the account service emailed: exactly 6 digits. */
export function emailedCodeError(code: string): Message {
  return /^[0-9]{6}$/.test(codeWithoutSpaces(code)) ? null : t('Enter the 6-digit code from the email.')
}

/** What is sent: the details without stray spaces around them, the password exactly as typed, and the code. */
export function signupBody(f: SignupFields, code: string): SignupDetails {
  return {
    firstName: f.firstName.trim(),
    lastName: f.lastName.trim(),
    email: f.email.trim(),
    phoneNumber: f.phoneNumber.trim(),
    password: f.password,
    code: codeWithoutSpaces(code),
  }
}
