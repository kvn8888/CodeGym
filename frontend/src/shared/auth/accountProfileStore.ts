import { create } from 'zustand';

import { api } from '../api/client';
import type { UserProfile } from '../api/types';

type AccountProfileState = {
  profile: UserProfile | null;
  loading: boolean;
  error: string | null;
  loadProfile: (force?: boolean) => Promise<UserProfile | null>;
  updateDisplayName: (displayName: string) => Promise<UserProfile>;
  resetProfile: () => void;
};

let requestVersion = 0;

export const useAccountProfile = create<AccountProfileState>((set, get) => ({
  profile: null,
  loading: false,
  error: null,

  loadProfile: async (force = false) => {
    const current = get();
    if (current.loading || (!force && current.profile)) {
      return current.profile;
    }

    const version = ++requestVersion;
    set({ loading: true, error: null });

    try {
      const profile = await api.get<UserProfile>('/me');
      if (version === requestVersion) {
        set({ profile, loading: false, error: null });
      }
      return profile;
    } catch (error) {
      if (version === requestVersion) {
        set({
          loading: false,
          error: error instanceof Error ? error.message : 'Could not load account profile.',
        });
      }
      return null;
    }
  },

  updateDisplayName: async (displayName) => {
    try {
      const profile = await api.patch<UserProfile>('/me', { display_name: displayName });
      set({ profile, error: null });
      return profile;
    } catch (error) {
      set({ error: error instanceof Error ? error.message : 'Could not save settings.' });
      throw error;
    }
  },

  resetProfile: () => {
    requestVersion += 1;
    set({ profile: null, loading: false, error: null });
  },
}));
