import { diagnosticError } from './diagnostics';

const username = 'BR_USERNAME_MUST_NOT_LEAK';
const password = 'BR_PASSWORD_MUST_NOT_LEAK';

test('diagnostic errors accept only fixed safe messages, never arbitrary payloads', () => {
  for (const reason of [
    new Error(`Proxy connection failed for http://${username}:${password}@proxy.example.com:8080`),
    `validation username=${username} password=${password}`,
    { username, password, secret: 'opaque DPAPI data' },
    new Error(`network connection test failed: ${username}`),
  ]) {
    const output = diagnosticError(reason);
    expect(output.includes(username) || output.includes(password) || output.includes('@')).toBe(false);
    expect(output).toBe('The operation could not be completed.');
  }
  expect(diagnosticError('proxy password is required')).toBe('proxy password is required');
});
