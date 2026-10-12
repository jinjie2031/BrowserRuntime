import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { EnginePanel } from './EnginePanel';
import { LanguageProvider } from '../i18n';
import type { EngineAPI, EngineStatus } from '../engine';

function api(phase: string): EngineAPI {
 return { getStatus: vi.fn().mockResolvedValue({ enabled: true, ready: false, phase, received: 1024, total: 2048 } satisfies EngineStatus), download: vi.fn().mockResolvedValue(undefined), cancel: vi.fn().mockResolvedValue(undefined) };
}
test('missing engine prompts download; cancel leaves launch blocked and supports retry', async()=>{
 const backend=api('MISSING');const ready=vi.fn();const user=userEvent.setup();
 render(<LanguageProvider><EnginePanel api={backend} onReady={ready}/></LanguageProvider>);
 expect(await screen.findByText('CloakBrowser engine is not installed')).toBeVisible();
 await user.click(screen.getByRole('button',{name:'Cancel'}));
 expect(ready).toHaveBeenCalledWith(false);
 await user.click(screen.getByRole('button',{name:'Download and install'}));
 expect(backend.download).toHaveBeenCalledOnce();
});
test('progress is accessible and can be cancelled',async()=>{
 const backend=api('DOWNLOADING');const user=userEvent.setup();
 render(<LanguageProvider><EnginePanel api={backend} onReady={vi.fn()}/></LanguageProvider>);
 expect(await screen.findByRole('progressbar')).toHaveAttribute('value','1024');
 await user.click(screen.getByRole('button',{name:'Cancel download'}));expect(backend.cancel).toHaveBeenCalledOnce();
});
test('failed download uses safe bilingual text, not backend diagnostics',async()=>{
 localStorage.setItem('browserruntime.ui.language','zh-CN');
 const backend=api('FAILED');
 render(<LanguageProvider><EnginePanel api={backend} onReady={vi.fn()}/></LanguageProvider>);
 expect(await screen.findByText('官方引擎下载失败，请重试。')).toBeVisible();
 expect(screen.getByRole('button',{name:'重试下载'})).toBeEnabled();
});
test('ready engine hides prompt; status failure fails closed without raw error',async()=>{
 const backend=api('READY');const ready=vi.fn();
 backend.getStatus=vi.fn().mockRejectedValue(new Error('https://canary-user:canary-password@example.invalid'));
 render(<LanguageProvider><EnginePanel api={backend} onReady={ready}/></LanguageProvider>);
 await waitFor(()=>expect(ready).toHaveBeenCalledWith(false));
 expect(document.body.textContent).not.toContain('canary-user');
 expect(document.body.textContent).not.toContain('canary-password');
});
