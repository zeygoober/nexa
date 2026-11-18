import { create } from "zustand";
import type { GemWallet, GemsView } from "@/api/gems";
import { gems as gemsApi } from "@/api/gems";

/** The Gems page's view, loaded when it opens; the gateway's `gems_update` keeps the wallet current. */
interface GemsStore {
  view: GemsView | null;
  loading: boolean;
  error: unknown;
  load: () => Promise<void>;
  setView: (v: GemsView) => void;
  setWallet: (w: GemWallet) => void;
  reset: () => void;
}

export const useGems = create<GemsStore>()((set, get) => ({
  view: null,
  loading: false,
  error: null,
  load: async () => {
    if (get().loading) return;
    set({ loading: true, error: null });
    try {
      set({ view: await gemsApi.get(), loading: false });
    } catch (error) {
      set({ error, loading: false });
    }
  },
  setView: (view) => set({ view }),
  setWallet: (wallet) => set((s) => (s.view ? { view: { ...s.view, wallet } } : s)),
  reset: () => set({ view: null, loading: false, error: null }),
}));
