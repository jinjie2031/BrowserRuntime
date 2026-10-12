import {
  CreateProfile,
  DeleteProfile,
  LaunchProfile,
  ListProfiles,
  OpenProfile,
  RenameProfile,
  StopProfile,
  GetNetwork,
  SetDirect,
  SetProxy,
  TestNetwork,
} from '../wailsjs/go/app/Service';
import type { NetworkView, ProfileAPI, ProfileView } from './types';

export const backend: ProfileAPI = {
  listProfiles: () => ListProfiles() as Promise<ProfileView[]>,
  createProfile: (name) => CreateProfile(name) as Promise<ProfileView>,
  renameProfile: (id, name) => RenameProfile(id, name) as Promise<ProfileView>,
  deleteProfile: DeleteProfile,
  launchProfile: LaunchProfile,
  openProfile: OpenProfile,
  stopProfile: StopProfile,
  getNetwork: (id) => GetNetwork(id) as Promise<NetworkView>,
  setDirect: (id) => SetDirect(id) as Promise<NetworkView>,
  setProxy: (id, input) => SetProxy(id, input) as Promise<NetworkView>,
  testNetwork: (id) => TestNetwork(id) as Promise<NetworkView>,
};
