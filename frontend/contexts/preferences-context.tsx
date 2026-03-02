"use client";

import { createContext, useContext, useState, useEffect, useCallback } from "react";
import { api } from "@/lib/api";

interface Preferences {
  stripTrackingParams: boolean;
}

interface PreferencesContextValue extends Preferences {
  updatePreference: (key: keyof Preferences, value: boolean) => void;
}

const defaultPreferences: Preferences = {
  stripTrackingParams: true,
};

const PreferencesContext = createContext<PreferencesContextValue>({
  ...defaultPreferences,
  updatePreference: () => {},
});

export function usePreferences() {
  return useContext(PreferencesContext);
}

export function PreferencesProvider({ children }: { children: React.ReactNode }) {
  const [prefs, setPrefs] = useState<Preferences>(defaultPreferences);

  useEffect(() => {
    api
      .get<Record<string, unknown>>("/api/users/me/preferences")
      .then((data) => {
        setPrefs({
          stripTrackingParams: data.strip_tracking_params !== false,
        });
      })
      .catch(() => {});
  }, []);

  const updatePreference = useCallback((key: keyof Preferences, value: boolean) => {
    setPrefs((prev) => ({ ...prev, [key]: value }));

    const apiKey = key === "stripTrackingParams" ? "strip_tracking_params" : key;
    api.patch("/api/users/me/preferences", { [apiKey]: value }).catch(() => {
      // Revert on failure
      setPrefs((prev) => ({ ...prev, [key]: !value }));
    });
  }, []);

  return (
    <PreferencesContext.Provider value={{ ...prefs, updatePreference }}>
      {children}
    </PreferencesContext.Provider>
  );
}
