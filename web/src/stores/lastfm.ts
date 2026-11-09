import { create } from "zustand";
import { connections, type LastfmState } from "@/api/connections";
import { useSettings } from "@/stores/settings";

/** The Last.fm account linked to this one, for Settings, Activity. */
interface LastfmStore {
  state: LastfmState | null;
  set: (state: LastfmState) => void;
  load: () => Promise<void>;
  clear: () => void;
}

export const useLastfm = create<LastfmStore>()((set) => ({
  state: null,
  set: (state) => {
    set({ state });
    alignShare();
  },
  load: async () => {
    try {
      useLastfm.getState().set(await connections.lastfm());
    } catch {
      /* an older server without the route, or offline: nothing linked to show */
    }
  },
  clear: () => set({ state: null }),
}));

/** The share switch covers this too, and what Last.fm says is shown by the server, so it is told to match. */
function alignShare(): void {
  const linked = useLastfm.getState().state?.linked;
  const st = useSettings.getState();
  if (!linked || !st.loaded) return;
  const want = st.settings.shareActivity !== false;
  if (linked.show === want) return;
  void connections
    .setLastfmShow(want)
    .then((next) => useLastfm.setState({ state: next }))
    .catch(() => undefined);
}

let installed = false;

export function installLastfm(): void {
  if (installed) return;
  installed = true;
  useSettings.subscribe((s, prev) => {
    if (s.loaded !== prev.loaded || s.settings.shareActivity !== prev.settings.shareActivity) alignShare();
  });
}
