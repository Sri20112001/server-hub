import { useState } from "react";

const LS_KEY = "serverhub.alerts";

export interface Alerts {
  deployFinished: boolean;
  healthChanged: boolean;
  highUsage: boolean;
}

function loadAlerts(): Alerts {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) return { deployFinished: true, healthChanged: true, highUsage: false, ...JSON.parse(raw) };
  } catch {
    /* fall through */
  }
  return { deployFinished: true, healthChanged: true, highUsage: false };
}

export function useAlerts() {
  const [alerts, setAlerts] = useState<Alerts>(loadAlerts);

  const flip = (key: keyof Alerts) => {
    setAlerts((a) => {
      const nextAlerts = { ...a, [key]: !a[key] };
      localStorage.setItem(LS_KEY, JSON.stringify(nextAlerts));
      return nextAlerts;
    });
  };

  return { alerts, flip };
}
