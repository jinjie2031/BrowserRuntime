import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { NetworkPanel } from './NetworkPanel';
import type { NetworkView, ProfileAPI } from '../types';

const direct: NetworkView = { mode: 'direct', status: 'Direct', has_password: false };
const proxy: NetworkView = { mode: 'proxy', scheme: 'http', host: 'proxy.example', port: 8080, username: 'test-user', has_password: true, status: 'Needs Test' };
const ready: NetworkView = { ...proxy, status: 'Ready', locality: { exit_ip: '203.0.113.80', country_code: 'US', city: 'Los Angeles', timezone: 'America/Los_Angeles', locale: 'en-US' } };

function api(view: NetworkView): ProfileAPI {
  return {
    listProfiles: vi.fn(), createProfile: vi.fn(), renameProfile: vi.fn(), deleteProfile: vi.fn(), launchProfile: vi.fn(), openProfile: vi.fn(), stopProfile: vi.fn(),
    getNetwork: vi.fn().mockResolvedValue(view),
    setDirect: vi.fn().mockResolvedValue(direct),
    setProxy: vi.fn().mockResolvedValue(proxy),
    testNetwork: vi.fn().mockResolvedValue(ready),
  };
}

test.each(['load', 'save', 'test'])('redacts credential-bearing backend errors on %s without hiding the normal username field', async (operation) => {
  const username = 'BR_USERNAME_MUST_NOT_LEAK';
  const password = 'BR_PASSWORD_MUST_NOT_LEAK';
  const backend = api({ ...proxy, username });
  const rejection = new Error(`failure http://${username}:${password}@proxy.example:8080`);
  if (operation === 'load') vi.mocked(backend.getNetwork).mockRejectedValue(rejection);
  if (operation === 'save') vi.mocked(backend.setProxy).mockRejectedValue(rejection);
  if (operation === 'test') vi.mocked(backend.testNetwork).mockRejectedValue(rejection);
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  if (operation !== 'load') {
    await screen.findByLabelText('Username');
    expect((screen.getByLabelText('Username') as HTMLInputElement).value === username).toBe(true);
    await user.click(screen.getByRole('button', { name: operation === 'save' ? 'Save' : 'Test Connection' }));
  }
  const output = (await screen.findByRole('alert')).textContent || '';
  expect(output.includes(username) || output.includes(password) || output.includes('@')).toBe(false);
});

test('loads Direct mode and saves it', async () => {
  const backend = api(direct);
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  expect(await screen.findByLabelText('Direct')).toBeChecked();
  expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Test Connection' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Save' }));
  expect(backend.setDirect).toHaveBeenCalledWith('br_test');
});

test.each(['http', 'https', 'socks5'])('saves %s proxy without loading the stored password', async (scheme) => {
  const backend = api(ready);
  const changed = vi.fn();
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={changed} />);
  const password = await screen.findByLabelText('Password');
  expect(password).toHaveAttribute('type', 'password');
  expect(password).toHaveValue('');
  expect(password).toHaveAttribute('placeholder', 'Saved password — leave blank to keep');
  expect(await screen.findByText('Ready')).toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText('Protocol'), scheme);
  await user.clear(screen.getByLabelText('Host'));
  await user.type(screen.getByLabelText('Host'), 'replacement.example');
  expect(screen.getByText('Needs Test')).toBeInTheDocument();
  expect(screen.queryByText('Los Angeles')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Test Connection' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(backend.setProxy).toHaveBeenCalledWith('br_test', { scheme, host: 'replacement.example', port: 8080, username: 'test-user', password: '' }));
  expect(changed).toHaveBeenCalled();
});

test('tests saved proxy and shows backend locality', async () => {
  const backend = api(proxy);
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  await screen.findByText('Needs Test');
  await user.click(screen.getByRole('button', { name: 'Test Connection' }));
  expect(await screen.findByText('Ready')).toBeInTheDocument();
  for (const text of ['203.0.113.80', 'US', 'Los Angeles', 'America/Los_Angeles', 'en-US']) expect(screen.getByText(text)).toBeInTheDocument();
});

test('renders a failed connection without old locality', async () => {
  const backend = api(ready);
  vi.mocked(backend.testNetwork).mockResolvedValue({ ...proxy, status: 'Connection Failed' });
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  await screen.findByText('Ready');
  await user.click(screen.getByRole('button', { name: 'Test Connection' }));
  expect(await screen.findByText('Connection Failed')).toBeInTheDocument();
  expect(screen.queryByText('Los Angeles')).not.toBeInTheDocument();
});

test('a rejected connection test also clears the displayed Ready state and locality', async () => {
  const backend = api(ready);
  vi.mocked(backend.testNetwork).mockRejectedValue(new Error('network operation could not be completed'));
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  await screen.findByText('Ready');
  await user.click(screen.getByRole('button', { name: 'Test Connection' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('network operation could not be completed');
  expect(screen.getByText('Connection Failed')).toBeInTheDocument();
  expect(screen.queryByText('Los Angeles')).not.toBeInTheDocument();
});

test('switches to Proxy and saves a replacement password then clears the input', async () => {
  const backend = api(direct);
  const user = userEvent.setup();
  render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  await user.click(await screen.findByLabelText('Proxy'));
  await user.type(screen.getByLabelText('Host'), 'proxy.example');
  await user.type(screen.getByLabelText('Port'), '8080');
  await user.type(screen.getByLabelText('Username'), 'test-user');
  await user.type(screen.getByLabelText('Password'), 'synthetic-input-only');
  await user.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(backend.setProxy).toHaveBeenCalled());
  await waitFor(() => expect(screen.getByLabelText('Password')).toHaveValue(''));
});

test('renders backend validation errors and disables editing while running', async () => {
  const backend = api(proxy);
  vi.mocked(backend.setProxy).mockRejectedValue(new Error('proxy password is required'));
  const user = userEvent.setup();
  const { rerender } = render(<NetworkPanel profileID="br_test" api={backend} running={false} onChanged={vi.fn()} />);
  await screen.findByLabelText('Password');
  await user.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('proxy password is required');
  rerender(<NetworkPanel profileID="br_test" api={backend} running onChanged={vi.fn()} />);
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Test Connection' })).toBeDisabled();
});
