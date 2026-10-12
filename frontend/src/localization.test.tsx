import { render, screen, within, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, afterEach, vi } from 'vitest';
import { App } from './App';
import type { EngineAPI } from './engine';
const readyEngine: EngineAPI = { getStatus: async () => ({enabled: true, ready: true, phase: 'READY', received: 0, total: 0}), download: async () => {}, cancel: async () => {} };
import type { NetworkView, ProfileAPI, ProfileView } from './types';
import { dictionaries, translate } from './i18n';
import { diagnosticError } from './diagnostics';

const preference = 'browserruntime.ui.language';
const profile: ProfileView = { id: 'br_test', name: 'Research 工作', browser: 'CloakBrowser', network: 'US · Los Angeles', status: 'Stopped', launch_ready: true };
const network: NetworkView = { mode: 'proxy', scheme: 'http', host: 'proxy.example', port: 8080, username: 'synthetic-user', has_password: true, status: 'Ready', locality: { exit_ip: '203.0.113.80', country_code: 'US', city: 'Los Angeles', timezone: 'America/Los_Angeles', locale: 'en-US' } };
function api(items = [profile]): ProfileAPI {
  return { listProfiles: vi.fn().mockResolvedValue(items), createProfile: vi.fn().mockResolvedValue(profile), renameProfile: vi.fn().mockResolvedValue(profile), deleteProfile: vi.fn(), launchProfile: vi.fn(), openProfile: vi.fn(), stopProfile: vi.fn(), getNetwork: vi.fn().mockResolvedValue(network), setDirect: vi.fn(), setProxy: vi.fn(), testNetwork: vi.fn().mockResolvedValue(network) };
}
beforeEach(() => localStorage.clear());
afterEach(() => { cleanup(); localStorage.clear(); });

test('defaults to English, switches both ways and persists only the UI language', async () => {
  const backend = api(); const user = userEvent.setup();
  render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  expect(await screen.findByRole('heading', { name: 'Profiles' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: '中文' }));
  expect(screen.getByRole('heading', { name: '浏览器环境' })).toBeInTheDocument();
  expect(screen.getByPlaceholderText('搜索浏览器环境')).toBeInTheDocument();
  expect(localStorage.getItem(preference)).toBe('zh-CN');
  expect(localStorage.length).toBe(1);
  await user.click(screen.getByRole('button', { name: 'English' }));
  expect(screen.getByRole('heading', { name: 'Profiles' })).toBeInTheDocument();
  expect(localStorage.getItem(preference)).toBe('en-US');
  for (const method of [backend.setDirect, backend.setProxy, backend.testNetwork, backend.renameProfile, backend.launchProfile]) expect(method).not.toHaveBeenCalled();
});

test('restores language after remount and does not translate profile names or locality', async () => {
  const backend = api(); const user = userEvent.setup();
  const first = render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  await user.click(screen.getByRole('button', { name: '中文' })); first.unmount();
  render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  expect(await screen.findByRole('button', { name: 'Research 工作' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'English' })).toBeInTheDocument();
  expect(screen.getByText('已停止')).toBeInTheDocument();
  expect(screen.getByText('US · Los Angeles')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Research 工作' }));
  expect(screen.getByRole('tab', { name: '概览' })).toBeInTheDocument();
  expect(screen.getByText('浏览器身份')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: '网络' }));
  expect(await screen.findByLabelText('主机地址')).toHaveValue('proxy.example');
  expect(screen.getByLabelText('用户名')).toHaveValue('synthetic-user');
  expect(screen.getByLabelText('密码')).toHaveValue('');
  expect(screen.getByText('已就绪')).toBeInTheDocument();
  for (const value of ['203.0.113.80', 'US', 'Los Angeles', 'America/Los_Angeles', 'en-US']) expect(screen.getByText(value)).toBeInTheDocument();
  expect(screen.getByText('浏览器语言区域')).toBeInTheDocument();
  expect(backend.setProxy).not.toHaveBeenCalled(); expect(backend.testNetwork).not.toHaveBeenCalled();
  expect(network.locality?.locale).toBe('en-US');
});

test('Chinese create, rename and delete dialogs keep user names unchanged', async () => {
  localStorage.setItem(preference, 'zh-CN');
  const backend = api(); const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  await screen.findByText('Research 工作');
  await user.click(screen.getByRole('button', { name: '+ 新建环境' }));
  let dialog = screen.getByRole('dialog', { name: '新建环境' });
  await user.type(within(dialog).getByLabelText('环境名称'), 'Work 原名');
  await user.click(within(dialog).getByRole('button', { name: '创建' }));
  await waitFor(() => expect(backend.createProfile).toHaveBeenCalledWith('Work 原名'));
  await user.click(screen.getByLabelText('Research 工作的更多操作'));
  await user.click(screen.getByRole('button', { name: '重命名' }));
  dialog = screen.getByRole('dialog', { name: '重命名环境' });
  expect(within(dialog).getByLabelText('环境名称')).toHaveValue('Research 工作');
  await user.click(within(dialog).getByRole('button', { name: '取消' }));
  await user.click(screen.getByLabelText('Research 工作的更多操作'));
  await user.click(screen.getByRole('button', { name: '删除' }));
  dialog = screen.getByRole('alertdialog', { name: '删除环境？' });
  expect(dialog).toHaveTextContent('此操作无法撤销'); expect(dialog).toHaveTextContent('Research 工作');
});

test('Chinese errors are safe and switch immediately while already displayed', async () => {
  const backend = api(); vi.mocked(backend.listProfiles).mockRejectedValue(new Error('invalid profile name'));
  const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  expect(await screen.findByRole('alert')).toHaveTextContent('invalid profile name');
  await user.click(screen.getByRole('button', { name: '中文' }));
  expect(screen.getByRole('alert')).toHaveTextContent('环境名称无效');
});

test('Chinese network validation remains presentation-only', async () => {
  localStorage.setItem(preference, 'zh-CN'); const backend = api();
  vi.mocked(backend.setProxy).mockRejectedValue(new Error('proxy password is required'));
  const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  await user.click(await screen.findByRole('button', { name: 'Research 工作' })); await user.click(screen.getByRole('tab', { name: '网络' }));
  await screen.findByLabelText('密码'); await user.click(screen.getByRole('button', { name: '保存' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('需要代理密码');
  expect(screen.getByLabelText('密码')).toHaveAttribute('placeholder', '已保存密码，留空以保留');
  await user.clear(screen.getByLabelText('端口')); await user.click(screen.getByRole('button', { name: '保存' }));
  expect(screen.getByRole('alert')).toHaveTextContent('端口必须在 1 到 65535 之间。');
});

test('Chinese credential-bearing errors never reach the alert', async () => {
  localStorage.setItem(preference, 'zh-CN'); const backend = api();
  const username = 'BR_USERNAME_MUST_NOT_LEAK'; const password = 'BR_PASSWORD_MUST_NOT_LEAK';
  vi.mocked(backend.listProfiles).mockRejectedValue(`http://${username}:${password}@proxy.example`);
  render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  const output = (await screen.findByRole('alert')).textContent || '';
  expect(output).toBe('操作未能完成。'); expect(output.includes(username) || output.includes(password) || output.includes('@')).toBe(false);
});

test('unknown stored language defaults to English', async () => {
  localStorage.setItem(preference, 'unsupported'); render(<App engineAPI={readyEngine} api={api([])} pollInterval={60_000} />);
  expect(await screen.findByRole('heading', { name: 'No profiles' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '中文' })).toBeInTheDocument();
});

test('Chinese running state and network controls preserve the existing lock', async () => {
  localStorage.setItem(preference, 'zh-CN'); const backend = api([{ ...profile, status: 'Running' }]);
  const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  expect(await screen.findByText('运行中')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '打开' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '停止' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Research 工作' })); await user.click(screen.getByRole('tab', { name: '网络' }));
  await screen.findByLabelText('主机地址');
  expect(screen.getByRole('button', { name: '保存' })).toBeDisabled();
  expect(screen.getByRole('button', { name: '测试连接' })).toBeDisabled();
  expect(screen.getByText('请先停止浏览器，再修改或测试网络。')).toBeInTheDocument();
  expect(backend.setProxy).not.toHaveBeenCalled(); expect(backend.setDirect).not.toHaveBeenCalled(); expect(backend.testNetwork).not.toHaveBeenCalled();
});

test('Chinese launch readiness and empty states are translated', async () => {
  localStorage.setItem(preference, 'zh-CN'); const backend = api([{ ...profile, network: 'Configuration Error', launch_ready: false, launch_blocked_reason: 'Network configuration unavailable; explicitly configure network' }]);
  const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  expect(await screen.findByText('配置错误')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '启动' })).toBeDisabled();
  expect(screen.getByText('网络配置不可用，请明确配置网络')).toBeInTheDocument();
  await user.type(screen.getByPlaceholderText('搜索浏览器环境'), 'missing');
  expect(screen.getByRole('heading', { name: '暂无浏览器环境' })).toBeInTheDocument();
});

test('translation dictionaries are complete, nonempty, and retain all English copy', () => {
  const en = dictionaries['en-US']; const zh = dictionaries['zh-CN'];
  expect(Object.keys(zh).sort()).toEqual(Object.keys(en).sort());
  expect(Object.values(zh).every(value => value.trim().length > 0)).toBe(true);
  expect(Object.entries(en).every(([key, value]) => key === 'Switch language' || key === value)).toBe(true);
  for (const [key, value] of Object.entries({ Starting: '正在启动', Stopping: '正在停止', 'Needs Test': '待测试', 'Connection Failed': '连接失败', 'Loading profiles…': '正在加载浏览器环境…', 'Loading network…': '正在加载网络…' })) expect(translate('zh-CN', key)).toBe(value);
});

test('translation keeps the strict diagnostic allowlist and never echoes unknown input', () => {
  const username = 'BR_USERNAME_MUST_NOT_LEAK'; const password = 'BR_PASSWORD_MUST_NOT_LEAK';
  for (const language of ['en-US', 'zh-CN'] as const) {
    for (const input of [`http://${username}:${password}@proxy.example`, { username, password }, 'constructor']) {
      const result = translate(language, diagnosticError(input));
      expect(result.includes(username) || result.includes(password) || result.includes('@')).toBe(false);
    }
  }
  expect(translate('zh-CN', 'constructor')).toBe('操作未能完成。');
});

test('native presentation receives only validated language enums, no Profile/Network data', async () => {
  const setter = vi.fn().mockResolvedValue(undefined);
  const original = Object.getOwnPropertyDescriptor(window, 'go');
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { UILanguage: { SetLanguage: setter } } } });
  try {
    const backend = api(); const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
    await waitFor(() => expect(setter).toHaveBeenLastCalledWith('en-US'));
    await user.click(screen.getByRole('button', { name: '中文' }));
    await waitFor(() => expect(setter).toHaveBeenLastCalledWith('zh-CN'));
    expect(document.documentElement.lang).toBe('zh-CN');
    for (const method of [backend.setDirect, backend.setProxy, backend.testNetwork, backend.renameProfile, backend.launchProfile]) expect(method).not.toHaveBeenCalled();
  } finally {
    if (original) Object.defineProperty(window, 'go', original); else Reflect.deleteProperty(window, 'go');
  }
});

test.each([
  ['ExternalRunning', 'Browser is still running outside this manager. Close it manually before continuing.', '浏览器仍在其他进程中运行，请先手动关闭。'],
  ['InUse', 'Profile is in use by another manager. Close it before continuing.', '环境正被其他管理器使用，请先关闭后再继续。'],
  ['Unknown', 'Unable to confirm the browser has stopped. Operation blocked.', '无法确认浏览器已停止，操作已阻止。'],
] as const)('unsafe occupancy %s blocks actions and remains bilingual', async (status, reason, chinese) => {
  const backend = api([{ ...profile, status, launch_ready: false, launch_blocked_reason: reason }]);
  const user = userEvent.setup(); render(<App engineAPI={readyEngine} api={backend} pollInterval={60_000} />);
  expect(await screen.findByText(reason)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Launch' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Open' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Stop' })).not.toBeInTheDocument();
  await user.click(screen.getByLabelText('More actions for Research 工作'));
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: '中文' }));
  expect(screen.getByText(chinese)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Research 工作' }));
  await user.click(screen.getByRole('tab', { name: '网络' }));
  expect(await screen.findByRole('button', { name: '保存' })).toBeDisabled();
  expect(screen.getByRole('button', { name: '测试连接' })).toBeDisabled();
  expect(screen.getByLabelText('主机地址')).toBeDisabled();
  expect(within(screen.getByRole('dialog')).getByText(chinese)).toBeInTheDocument();
  expect(backend.setDirect).not.toHaveBeenCalled(); expect(backend.setProxy).not.toHaveBeenCalled(); expect(backend.testNetwork).not.toHaveBeenCalled();
});
