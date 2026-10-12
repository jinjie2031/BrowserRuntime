// Only exact application-owned messages can cross the error rendering boundary.
// Never echo arbitrary errors: they may contain saved credentials unknown to UI.
const safeMessages = new Set([
  'Browser is still running outside this manager. Close it manually before continuing.',
  'Unable to confirm the browser has stopped. Operation blocked.',
  'Profile is in use by another manager. Close it before continuing.',
  'profile already has a running browser',
  'invalid network configuration',
  'network configuration is missing; explicitly configure network',
  'proxy password is required',
  'proxy secret is unavailable',
  'network connection test failed',
  'invalid network locality',
  'network locality is unavailable',
  'network operation could not be completed',
  'stop browser before changing or testing network',
  'Port must be between 1 and 65535.',
  'running profile cannot be deleted',
  'invalid profile name',
  'profile name already exists',
  'invalid profile ID',
  'profile not found',
  'invalid profile metadata',
]);

export function diagnosticError(reason: unknown): string {
  const message = reason instanceof Error ? reason.message : reason;
  return typeof message === 'string' && safeMessages.has(message)
    ? message
    : 'The operation could not be completed.';
}
