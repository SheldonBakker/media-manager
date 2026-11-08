import assert from 'node:assert/strict'
import { test } from 'node:test'
import { resetBody, resetErrors, type ResetFields } from './passwordResetForm.ts'

const filled: ResetFields = {
  code: '123456',
  password: 'correct horse battery',
  confirmPassword: 'correct horse battery',
}

// Each row: the field changed, its new value, whether the form should pass.
function rows(name: string, field: keyof ResetFields, cases: [string, boolean][]) {
  test(name, () => {
    for (const [value, ok] of cases) {
      const errors = resetErrors({ ...filled, [field]: value })
      assert.equal(errors[field] === null, ok, `${JSON.stringify(value)} -> ${errors[field]}`)
    }
  })
}

test('a filled-in form passes', () => {
  assert.deepEqual(resetErrors(filled), { code: null, password: null, confirmPassword: null })
})

rows('code: exactly 6 digits once spaces are taken out', 'code', [
  ['', false],
  ['123456', true],
  ['123 456', true],
  [' 123 456 ', true],
  ['12345', false],
  ['1234567', false],
  ['12a456', false],
])

rows('password: required, 12 to 128 characters', 'password', [
  ['', false],
  [' '.repeat(12), false],
  ['a'.repeat(11), false],
  ['a'.repeat(12), true],
  ['a'.repeat(128), true],
  ['a'.repeat(129), false],
])

rows('confirmPassword: required and the same as the password', 'confirmPassword', [
  ['', false],
  ['correct horse battery', true],
  ['correct horse batter', false],
])

test('the body trims the address, sends the code without spaces and the password as typed', () => {
  assert.deepEqual(resetBody(' thandi@example.com ', { code: ' 123 456 ', password: ' padded password ', confirmPassword: ' padded password ' }), {
    email: 'thandi@example.com',
    code: '123456',
    password: ' padded password ',
  })
})
