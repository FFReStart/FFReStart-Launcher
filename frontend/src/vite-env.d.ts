/// <reference types="vite/client" />

interface LauncherBackend {
  GamePath(): Promise<string>;
  GetGameStatus(): Promise<{ installed: boolean; message: string }>;
  InstallOrUpdate(): Promise<void>;
  SetGamePath(path: string): Promise<void>;
  PlayOffline(): Promise<void>;
}

interface Window {
  go?: { main?: { App?: LauncherBackend } };
}
