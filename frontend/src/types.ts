export type ProfileStatus = 'Running' | 'Stopped' | 'Starting' | 'Stopping' | 'ExternalRunning' | 'InUse' | 'Unknown';

export interface ProfileView {
  id: string;
  name: string;
  browser: string;
  network: string;
  status: ProfileStatus;
  launch_ready: boolean;
  launch_blocked_reason?: string;
}

export type NetworkStatus = 'Direct' | 'Needs Test' | 'Ready' | 'Connection Failed';

export interface LocalityView {
  exit_ip: string;
  country_code: string;
  city?: string;
  timezone: string;
  locale: string;
}

export interface NetworkView {
  mode: 'direct' | 'proxy';
  scheme?: 'http' | 'https' | 'socks5';
  host?: string;
  port?: number;
  username?: string;
  has_password: boolean;
  status: NetworkStatus;
  locality?: LocalityView;
}

// Write-only replacement; existing secrets are never returned in NetworkView.
export interface SetProxyInput {
  scheme: 'http' | 'https' | 'socks5';
  host: string;
  port: number;
  username: string;
  password: string;
}

export interface ProfileAPI {
  listProfiles(): Promise<ProfileView[]>;
  createProfile(name: string): Promise<ProfileView>;
  renameProfile(id: string, name: string): Promise<ProfileView>;
  deleteProfile(id: string): Promise<void>;
  launchProfile(id: string): Promise<void>;
  openProfile(id: string): Promise<void>;
  stopProfile(id: string): Promise<void>;
  getNetwork(id: string): Promise<NetworkView>;
  setDirect(id: string): Promise<NetworkView>;
  setProxy(id: string, input: SetProxyInput): Promise<NetworkView>;
  testNetwork(id: string): Promise<NetworkView>;
}
