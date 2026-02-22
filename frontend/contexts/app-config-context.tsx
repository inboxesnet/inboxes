"use client";

import { createContext, useContext, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface AppConfig {
  commercial: boolean;
  apiUrl: string;
  wsUrl: string;
}

const defaultConfig: AppConfig = { commercial: false, apiUrl: "", wsUrl: "" };

const AppConfigContext = createContext<AppConfig>(defaultConfig);

export function useAppConfig() {
  return useContext(AppConfigContext);
}

export function AppConfigProvider({ children }: { children: React.ReactNode }) {
  const [config, setConfig] = useState<AppConfig>(defaultConfig);

  useEffect(() => {
    api.get<{ commercial?: boolean; api_url?: string; ws_url?: string }>("/api/config")
      .then((data) =>
        setConfig({
          commercial: data.commercial ?? false,
          apiUrl: data.api_url ?? "",
          wsUrl: data.ws_url ?? "",
        })
      )
      .catch(() => {});
  }, []);

  return (
    <AppConfigContext.Provider value={config}>
      {children}
    </AppConfigContext.Provider>
  );
}
