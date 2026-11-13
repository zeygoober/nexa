import { create } from "zustand";
import type { Trade } from "@/api/types";

/** Trades as the server last said they stand, by id: the trade window reads them, and the gateway keeps them current. */
interface TradesStore {
  byId: Record<string, Trade>;
  set: (trade: Trade) => void;
}

export const useTrades = create<TradesStore>()((set) => ({
  byId: {},
  set: (trade) => set((s) => ({ byId: { ...s.byId, [trade.id]: trade } })),
}));
