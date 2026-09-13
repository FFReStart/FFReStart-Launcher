/// <reference types="vite/client" />

interface LauncherBackend {
  GamePath(): Promise<string>;
  SetGamePath(path: string): Promise<void>;
  PlayOffline(): Promise<void>;
}

interface Window {
  go?: { main?: { App?: LauncherBackend } };
}
