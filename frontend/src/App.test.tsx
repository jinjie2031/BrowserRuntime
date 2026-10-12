import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { App } from './App';
import type { EngineAPI } from './engine';
const readyEngine: EngineAPI = { getStatus: async () => ({enabled: true, ready: true, phase: 'READY', received: 0, total: 0}), download: async () => {}, cancel: async () => {} };
import type { ProfileAPI, ProfileView } from './types';

const stopped: ProfileView = { id: 'br_stopped', name: 'Research', browser: 'CloakBrowser', network: 'Direct', status: 'Stopped', launch_ready: true };
const running: ProfileView = { id: 'br_running', name: 'Operations', browser: 'CloakBrowser', network: 'US · Los Angeles', status: 'Running', launch_ready: true };

test('missing portable engine blocks launch without hiding profiles or leaking diagnostics', async () => {
  const engineAPI: EngineAPI = { ...readyEngine, getStatus: async () => ({ enabled: true, ready: false, phase: 'MISSING', received: 0, total: 0 }) };
  const api = fakeAPI([stopped]);
  render(<App engineAPI={engineAPI} api={api} pollInterval={60_000} />);
  expect(await screen.findByText('Research')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Launch' })).toBeDisabled();
  expect(await screen.findByRole('button', { name: 'Download and install' })).toBeEnabled();
  expect(api.launchProfile).not.toHaveBeenCalled();
});

function fakeAPI(profiles: ProfileView[] = []): ProfileAPI {
  return {
    listProfiles: vi.fn().mockResolvedValue(profiles),
    createProfile: vi.fn().mockResolvedValue(stopped),
    renameProfile: vi.fn().mockResolvedValue(stopped),
    deleteProfile: vi.fn().mockResolvedValue(undefined),
    launchProfile: vi.fn().mockResolvedValue(undefined),
    openProfile: vi.fn().mockResolvedValue(undefined),
    stopProfile: vi.fn().mockResolvedValue(undefined),
    getNetwork: vi.fn().mockResolvedValue({ mode: 'direct', status: 'Direct', has_password: false }),
    setDirect: vi.fn(),
    setProxy: vi.fn(),
    testNetwork: vi.fn(),
  };
}

test('renders an empty profile list', async () => {
  render(<App engineAPI={readyEngine} api={fakeAPI()} pollInterval={60_000} />);
  expect(await screen.findByRole('heading', { name: 'No profiles' })).toBeInTheDocument();
});

test('opens the two-tab profile drawer and closes it', async () => {
  const user = userEvent.setup();
  render(<App engineAPI={readyEngine} api={fakeAPI([stopped])} pollInterval={60_000} />);
  await user.click(await screen.findByRole('button', { name: 'Research' }));
  const drawer = screen.getByRole('dialog', { name: 'Research' });
  expect(within(drawer).getAllByRole('tab')).toHaveLength(2);
  expect(within(drawer).getByText('Browser Identity')).toBeInTheDocument();
  expect(within(drawer).getByText('Stable')).toBeInTheDocument();
  await user.click(within(drawer).getByRole('tab', { name: 'Network' }));
  expect(await within(drawer).findByLabelText('Direct')).toBeChecked();
  await user.click(within(drawer).getByRole('button', { name: 'Close profile' }));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

test('blocks unresolved and failed launch buttons with a reason, enables ready profiles', async () => {
  const profiles: ProfileView[] = [
    { ...stopped, launch_ready: false, launch_blocked_reason: 'Test network before launch' },
    { ...stopped, id: 'br_failed', name: 'Failed', network: 'Connection Failed', launch_ready: false, launch_blocked_reason: 'Test network before launch' },
    { ...stopped, id: 'br_ready', name: 'Ready', network: 'US · Los Angeles', launch_ready: true },
  ];
  render(<App engineAPI={readyEngine} api={fakeAPI(profiles)} pollInterval={60_000} />);
  const buttons = await screen.findAllByRole('button', { name: 'Launch' });
  expect(buttons[0]).toBeDisabled();
  expect(buttons[1]).toBeDisabled();
  expect(buttons[2]).toBeEnabled();
  expect(screen.getAllByText('Test network before launch')).toHaveLength(2);
});

test('renders profiles, filters search, and switches lifecycle actions', async () => {
  const api = fakeAPI([stopped, running]);
  const user = userEvent.setup();
  render(<App engineAPI={readyEngine} api={api} pollInterval={60_000} />);
  expect(await screen.findByText('Research')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Launch' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();

  await user.type(screen.getByPlaceholderText('Search Profiles'), 'research');
  expect(screen.getByText('Research')).toBeInTheDocument();
  expect(screen.queryByText('Operations')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Launch' }));
  await waitFor(() => expect(api.launchProfile).toHaveBeenCalledWith(stopped.id));
});

test('creates a profile from the modal', async () => {
  const api = fakeAPI();
  const user = userEvent.setup();
  render(<App engineAPI={readyEngine} api={api} pollInterval={60_000} />);
  await screen.findByRole('heading', { name: 'No profiles' });
  await user.click(screen.getByRole('button', { name: '+ New Profile' }));
  const dialog = screen.getByRole('dialog', { name: 'New Profile' });
  await user.type(within(dialog).getByLabelText('Profile Name'), 'Work');
  await user.click(within(dialog).getByRole('button', { name: 'Create' }));
  await waitFor(() => expect(api.createProfile).toHaveBeenCalledWith('Work'));
});

test('renames and confirms deletion from the More menu', async () => {
  const api = fakeAPI([stopped]);
  const user = userEvent.setup();
  render(<App engineAPI={readyEngine} api={api} pollInterval={60_000} />);
  await screen.findByText('Research');
  await user.click(screen.getByLabelText('More actions for Research'));
  await user.click(screen.getByRole('button', { name: 'Rename' }));
  const renameDialog = screen.getByRole('dialog', { name: 'Rename Profile' });
  const input = within(renameDialog).getByLabelText('Profile Name');
  await user.clear(input);
  await user.type(input, 'Personal');
  await user.click(within(renameDialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.renameProfile).toHaveBeenCalledWith(stopped.id, 'Personal'));

  await user.click(screen.getByLabelText('More actions for Research'));
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  const deleteDialog = screen.getByRole('alertdialog', { name: 'Delete Profile?' });
  await user.click(within(deleteDialog).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(api.deleteProfile).toHaveBeenCalledWith(stopped.id));
});

test('delegates running Open and Stop actions', async () => {
  const api = fakeAPI([running]);
  const user = userEvent.setup();
  render(<App engineAPI={readyEngine} api={api} pollInterval={60_000} />);
  await screen.findByText('Operations');
  await user.click(screen.getByRole('button', { name: 'Open' }));
  await user.click(screen.getByRole('button', { name: 'Stop' }));
  await waitFor(() => expect(api.openProfile).toHaveBeenCalledWith(running.id));
  await waitFor(() => expect(api.stopProfile).toHaveBeenCalledWith(running.id));
});

test('renders backend errors', async () => {
  const api = fakeAPI();
  vi.mocked(api.listProfiles).mockRejectedValue(new Error('Unable to load profiles'));
  render(<App engineAPI={readyEngine} api={api} pollInterval={60_000} />);
  expect(await screen.findByRole('alert')).toHaveTextContent('The operation could not be completed.');
});

test('missing network configuration stays visible and cannot launch', async () => {
  const broken = { ...stopped, network: 'Configuration Error', launch_ready: false, launch_blocked_reason: 'Network configuration unavailable; explicitly configure network' };
  render(<App engineAPI={readyEngine} api={fakeAPI([broken])} pollInterval={60_000} />);
  expect(await screen.findByText('Configuration Error')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Launch' })).toBeDisabled();
  expect(screen.getByText(broken.launch_blocked_reason)).toBeInTheDocument();
});

test('does not echo credential-bearing application errors', async () => {
  const username = 'BR_USERNAME_MUST_NOT_LEAK';
  const password = 'BR_PASSWORD_MUST_NOT_LEAK';
  const api = fakeAPI();
  vi.mocked(api.listProfiles).mockRejectedValue(`http://${username}:${password}@proxy.example:8080`);
  render(<App engineAPI={readyEngine} api={api} pollInterval={60_000} />);
  const output = (await screen.findByRole('alert')).textContent || '';
  expect(output.includes(username) || output.includes(password) || output.includes('@')).toBe(false);
});

test('polls profile status and switches actions', async () => {
  vi.useFakeTimers();
  const api = fakeAPI([stopped]);
  vi.mocked(api.listProfiles)
    .mockResolvedValueOnce([stopped])
    .mockResolvedValue([running]);
  try {
    render(<App engineAPI={readyEngine} api={api} pollInterval={1500} />);
    await act(async () => { await Promise.resolve(); });
    expect(screen.getByRole('button', { name: 'Launch' })).toBeInTheDocument();
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();
  } finally { vi.useRealTimers(); }
});
