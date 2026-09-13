/// <reference types="vite/client" />

interface LauncherBackend {
  GamePath(): Promise<string>;
  GetGameStatus(): Promise<{ installed: boolean; message: string }>;
  InstallOrUpdate(): Promise<void>;
  SetGamePath(path: string): Promise<void>;
  PlayOffline(): Promise<void>;
  GetLauncherState(): Promise<LauncherState>;
  CompleteSetup(): Promise<void>;
  ChooseInstallDirectory(): Promise<LauncherState>;
  ResetInstallDirectory(): Promise<LauncherState>;
  SaveMusicPreferences(volume: number, muted: boolean): Promise<void>;
  OpenCommunity(): void;
  OpenSupport(): void;
  OpenGameFiles(): void;
  SignInBrowser(): Promise<void>;
  SignInWithCode(): Promise<void>;
  SignOut(): Promise<void>;
}

interface GameStatus { installed:boolean; busy:boolean; message:string; title:string; version:string; progress:number }
interface LauncherState { game:GameStatus; installDirectory:string; defaultInstallDirectory:string; setupComplete:boolean; musicVolume:number; musicMuted:boolean; musicAvailable:boolean; multiplayerConfigured:boolean; signedIn:boolean }
interface DevicePrompt { verificationUri:string; verificationUriComplete:string; userCode:string }

interface Window {
  go?: { main?: { App?: LauncherBackend } };
  runtime?: { EventsOn(name:string, callback:(...data:any[])=>void):()=>void };
}
