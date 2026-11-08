// The checks and the details sent for "Forgot your password?" on the sign-in
// page. The password is changed in the account service, so the code and new
// password follow the same rules as sign-up (signupForm.ts). Free of React so
// it can be tested with plain node (see passwordResetForm.test.ts).
import type { PasswordReset } from './api.ts'
import { codeWithoutSpaces, emailedCodeError, newPasswordErrors } from './signupForm.ts'
import type { Message } from './validate.ts'

// The second step of the form. The first is the email address the code goes to.
export interface ResetFields {
  code: string
  password: string
  confirmPassword: string
}

/** The message for each field, or null when it is fine, in the order the form shows them. */
export function resetErrors(f: ResetFields): Record<keyof ResetFields, Message> {
  return { code: emailedCodeError(f.code), ...newPasswordErrors(f.password, f.confirmPassword) }
}

/** What is sent: the address the code went to, the code without spaces, and the password exactly as typed. */
export function resetBody(email: string, f: ResetFields): PasswordReset {
  return { email: email.trim(), code: codeWithoutSpaces(f.code), password: f.password }
}
