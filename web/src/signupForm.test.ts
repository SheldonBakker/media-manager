import assert from 'node:assert/strict'
import { test } from 'node:test'
import { emailedCodeError, signupBody, signupErrors, type SignupFields } from './signupForm.ts'

const filled: SignupFields = {
  firstName: 'Thandi',
  lastName: 'Nkosi',
  email: 'thandi@example.com',
  phoneNumber: '+27821234567',
  password: 'correct horse battery',
  confirmPassword: 'correct horse battery',
}

// Each row: the field changed, its new value, whether the form should pass.
function rows(name: string, field: keyof SignupFields, cases: [string, boolean][], extra: Partial<SignupFields> = {}) {
  test(name, () => {
    for (const [value, ok] of cases) {
      const errors = signupErrors({ ...filled, ...extra, [field]: value })
      assert.equal(errors[field] === null, ok, `${JSON.stringify(value)} -> ${errors[field]}`)
    }
  })
}

test('a filled-in form passes', () => {
  assert.deepEqual(Object.values(signupErrors(filled)), [null, null, null, null, null, null])
})

for (const field of ['firstName', 'lastName'] as const) {
  rows(`${field}: required and at most 100 characters`, field, [
    ['', false],
    ['   ', false],
    ['A', true],
    ['a'.repeat(100), true],
    ['  ' + 'a'.repeat(100) + '  ', true],
    ['a'.repeat(101), false],
  ])
}

rows('email: required, valid and at most 254 characters', 'email', [
  ['', false],
  ['  ', false],
  ['name@example.com', true],
  ['  name@example.com  ', true],
  ['name@example', false],
  ['name example.com', false],
  ['a'.repeat(242) + '@example.com', true],
  ['a'.repeat(243) + '@example.com', false],
])

rows('phoneNumber: required, international format', 'phoneNumber', [
  ['', false],
  ['+27821234567', true],
  [' +27821234567 ', true],
  ['+12345678', true],
  ['+123456789012345', true],
  ['+1234567', false],
  ['+1234567890123456', false],
  ['0821234567', false],
  ['+0821234567', false],
  ['+27 82 123 4567', false],
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
  ['correct horse battery ', false],
])

test('code: exactly 6 digits once spaces are taken out', () => {
  const cases: [string, boolean][] = [
    ['', false],
    ['      ', false],
    ['123456', true],
    [' 123456 ', true],
    ['123 456', true],
    ['000000', true],
    ['12345', false],
    ['1234567', false],
    ['12a456', false],
    ['12-456', false],
    ['１２３４５６', false],
  ]
  for (const [value, ok] of cases) {
    const got = emailedCodeError(value)
    assert.equal(got === null, ok, `${JSON.stringify(value)} -> ${got}`)
  }
})

test('the body trims the details, sends the password as typed and the code without spaces', () => {
  const body = signupBody(
    {
      firstName: '  Thandi ',
      lastName: ' Nkosi  ',
      email: ' thandi@example.com ',
      phoneNumber: ' +27821234567 ',
      password: ' padded password ',
      confirmPassword: ' padded password ',
    },
    ' 123 456 ',
  )
  assert.deepEqual(body, {
    firstName: 'Thandi',
    lastName: 'Nkosi',
    email: 'thandi@example.com',
    phoneNumber: '+27821234567',
    password: ' padded password ',
    code: '123456',
  })
})
