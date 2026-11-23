import { create } from "zustand";
import { quests as questsApi, type QuestBoard } from "@/api/quests";

/** The Quests page's board, loaded when it opens; the gateway's `quests_update` replaces it when a quest finishes. */
interface QuestsStore {
  board: QuestBoard | null;
  loading: boolean;
  error: unknown;
  load: () => Promise<void>;
  set: (b: QuestBoard) => void;
  reset: () => void;
}

export const useQuests = create<QuestsStore>()((set, get) => ({
  board: null,
  loading: false,
  error: null,
  load: async () => {
    if (get().loading) return;
    set({ loading: true, error: null });
    try {
      set({ board: await questsApi.get(), loading: false });
    } catch (error) {
      set({ error, loading: false });
    }
  },
  set: (board) => set({ board }),
  reset: () => set({ board: null, loading: false, error: null }),
}));
