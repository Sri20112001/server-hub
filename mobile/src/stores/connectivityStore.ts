import { create } from "zustand";
import NetInfo, {
  type NetInfoState,
  type NetInfoSubscription,
} from "@react-native-community/netinfo";

interface ConnectivityState {
  /** null until the first NetInfo reading arrives. */
  isConnected: boolean | null;
  isInternetReachable: boolean | null;
  startListening: () => void;
  stopListening: () => void;
}

let subscription: NetInfoSubscription | null = null;

function snapshot(state: NetInfoState) {
  return {
    isConnected: state.isConnected,
    isInternetReachable: state.isInternetReachable,
  };
}

export const useConnectivityStore = create<ConnectivityState>((set) => ({
  isConnected: null,
  isInternetReachable: null,

  startListening: () => {
    if (subscription) return;
    // Seed immediately so the banner never waits on the first event.
    void NetInfo.fetch().then((state) => set(snapshot(state)));
    subscription = NetInfo.addEventListener((state) => set(snapshot(state)));
  },

  stopListening: () => {
    subscription?.();
    subscription = null;
  },
}));

/** Offline when NetInfo says disconnected (null = unknown = assume online). */
export function useIsOffline(): boolean {
  const isConnected = useConnectivityStore((s) => s.isConnected);
  return isConnected === false;
}
