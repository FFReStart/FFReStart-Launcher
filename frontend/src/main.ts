import './style.css';
import logoURL from '../../GameLauncher/images/ReStartTransparentLogo.png';
import heroesURL from '../../GameLauncher/images/FFReStartMenuBG.png';
import backgroundURL from '../../GameLauncher/images/newloginbackground.png';

document.querySelector<HTMLDivElement>('#app')!.innerHTML = `
<div class="launcher"><div class="backdrop" style="background-image:url('${backgroundURL}')"></div><div class="shade"></div><div class="circuit circuit-a"></div><div class="circuit circuit-b"></div>
<main class="frame"><header class="topbar"><div class="identity"><img src="${logoURL}" alt="FFReStart logo"><div><h1>FFReSTART</h1><p>FUSIONFALL COMMUNITY LAUNCHER</p></div></div><div class="header-actions">
<section id="music-panel" class="micro-panel" aria-label="Launcher music controls" data-tooltip="Music loads from your private local app-data folder."><div class="micro-label"><span>MUSIC</span><output id="volume-label">35%</output></div><div class="music-row"><button id="mute" class="small-button">MUTE</button><input id="volume" aria-label="Launcher music volume" type="range" min="0" max="100" value="35"></div></section>
<section class="account-chip"><span class="pilot-icon">◎</span><div><strong id="account-title">GUEST PILOT</strong><small id="account-detail">Offline play is ready</small></div><button id="signout" class="small-button hidden">SIGN OUT</button></section></div></header>
<div class="content"><section class="hero-zone"><img class="heroes" src="${heroesURL}" alt="FusionFall heroes"><div class="hero-copy"><span>WELCOME BACK, HERO</span><h2>THE FIGHT FOR THE<br>FUTURE STARTS HERE</h2></div>
<section class="auth-panel"><p class="section-kicker">PILOT AUTHORIZATION</p><h3>OPTIONAL MULTIPLAYER SIGN-IN</h3><p id="auth-copy">Play offline without an account.</p><div class="auth-actions"><button id="signin" class="secondary">SIGN IN IN BROWSER</button><button id="device-signin" class="secondary">SIGN IN WITH A CODE</button></div><p id="auth-status" class="inline-status" role="status"></p></section></section>
<section class="mission-panel"><div class="panel-heading"><div><p class="section-kicker">MISSION CONTROL</p><h2>GAME STATUS</h2></div><span id="version" class="version">—</span></div>
<div class="status-card"><span id="status-dot" class="status-dot"></span><div><h3 id="status-title">CHECKING GAME FILES</h3><p id="status-detail" role="status">Reading the installed version…</p><div id="progress-track" class="progress-track hidden"><span id="progress"></span></div></div></div>
<button id="play" class="play" disabled>CHECKING…</button><button id="update" class="secondary hidden" style="width:100%;margin-top:7px;padding:7px">CHECK FOR UPDATES</button><p class="offline-note"><span>✓</span> Offline singleplayer never needs sign-in or a network connection.</p>
<div class="utility-row"><button id="discord" class="secondary" data-tooltip="Open the community Discord">DISCORD</button><button id="support" class="secondary" data-tooltip="Open FFReStart support">SUPPORT</button><button id="files" class="secondary" data-tooltip="Open the game installation folder">GAME FILES</button></div><div class="divider"></div>
<div class="folder-row"><div><span>INSTALL FOLDER</span><p id="install-path"></p></div><div><button id="default-folder" class="small-button" data-tooltip="Restore the default app-data folder">DEFAULT</button><button id="change-folder" class="small-button" data-tooltip="Choose where signed game files are installed">CHANGE</button><button id="settings" class="icon-button" data-tooltip="Launcher settings" aria-label="Launcher settings">⚙</button></div></div></section></div></main>
<section id="setup" class="modal hidden" role="dialog" aria-modal="true"><div class="modal-card"><p class="section-kicker">FIRST-RUN SETUP</p><h2>READY YOUR LAUNCHER</h2><p>FFReStart installs signed game builds into a folder you control. Accounts are optional and offline play always remains available.</p><div class="setup-path"><span>INSTALL LOCATION</span><strong id="setup-path"></strong></div><div class="modal-actions"><button id="setup-change" class="secondary">CHOOSE ANOTHER FOLDER</button><button id="setup-continue" class="play">CONTINUE</button></div></div></section>
<section id="settings-modal" class="modal hidden" role="dialog" aria-modal="true"><div class="modal-card compact"><button id="settings-close" class="modal-close" aria-label="Close">×</button><p class="section-kicker">LAUNCHER SETTINGS</p><h2>MISSION PREFERENCES</h2><p>Preferences are stored by the native launcher backend.</p><label class="setting-line"><span>Autoplay music</span><input id="settings-mute" type="checkbox"></label><button id="settings-folder" class="secondary full">CHANGE INSTALL FOLDER</button></div></section>
<audio id="theme" src="/launcher/audio/launcher-main-theme.mp3" loop preload="auto"></audio></div>`;

const $ = <T extends HTMLElement>(selector:string) => document.querySelector<T>(selector)!;
const backend = window.go?.main?.App;
const theme = $('#theme') as HTMLAudioElement;
const volume = $('#volume') as HTMLInputElement;
const mute = $('#mute') as HTMLButtonElement;
let state:LauncherState|undefined;
let saveTimer=0;
const errorText=(error:unknown)=>error instanceof Error?error.message:String(error).replace(/^Error:\s*/, '');

function updateMusic(){ $('#volume-label').textContent=`${Math.round(Number(volume.value))}%`; mute.textContent=theme.muted?'UNMUTE':'MUTE'; ($('#settings-mute') as HTMLInputElement).checked=!theme.muted; }
function render(next:LauncherState){
 state=next; const game=next.game; $('#install-path').textContent=next.installDirectory; ($('#install-path') as HTMLElement).title=next.installDirectory; $('#setup-path').textContent=next.installDirectory;
 $('#version').textContent=game.version||(game.installed?'INSTALLED':'NEW INSTALL'); $('#status-title').textContent=game.title||'CHECKING GAME FILES'; $('#status-detail').textContent=game.message;
 $('#status-dot').className=`status-dot ${game.busy?'busy':game.installed?'ready':'attention'}`; $('#progress-track').classList.toggle('hidden',!game.busy); $('#progress-track').classList.toggle('indeterminate',game.progress<0); ($('#progress') as HTMLElement).style.width=`${Math.max(0,game.progress)}%`;
 const play=$('#play') as HTMLButtonElement; play.disabled=game.busy; play.textContent=game.busy?(game.installed?'UPDATING…':'INSTALLING…'):game.installed?'PLAY OFFLINE':'INSTALL OR UPDATE'; const update=$('#update') as HTMLButtonElement; update.classList.toggle('hidden',!game.installed); update.disabled=game.busy; ($('#change-folder') as HTMLButtonElement).disabled=game.busy; ($('#default-folder') as HTMLButtonElement).disabled=game.busy;
 $('#default-folder').classList.toggle('hidden',next.installDirectory===next.defaultInstallDirectory); $('#setup').classList.toggle('hidden',next.setupComplete);
 volume.value=String(Math.round(next.musicVolume*100)); theme.volume=next.musicVolume; theme.muted=next.musicMuted; updateMusic();
 if(!next.musicAvailable){ ($('#music-panel') as HTMLElement).dataset.tooltip='Run scripts/install-local-music.ps1 to enable local music.'; volume.disabled=true; mute.disabled=true; }
 $('#auth-copy').textContent=next.multiplayerConfigured?'Multiplayer is not yet available until signed tickets and stdin hand-off ship.':'Multiplayer is not configured yet. Set the ZITADEL issuer and public client ID to enable sign-in.';
 ($('#signin') as HTMLButtonElement).disabled=!next.multiplayerConfigured||next.signedIn; ($('#device-signin') as HTMLButtonElement).disabled=!next.multiplayerConfigured||next.signedIn;
 $('#account-title').textContent=next.signedIn?'PILOT SIGNED IN':'GUEST PILOT'; $('#account-detail').textContent=next.signedIn?'Multiplayer hand-off pending':'Offline play is ready'; $('#signout').classList.toggle('hidden',!next.signedIn);
}
async function refresh(){if(backend)render(await backend.GetLauncherState())}
async function saveMusic(){if(backend)await backend.SaveMusicPreferences(Number(volume.value)/100,theme.muted)}
function tryMusic(){if(state?.musicAvailable&&!theme.muted)theme.play().catch(()=>{($('#music-panel') as HTMLElement).dataset.tooltip='Your system paused autoplay. Use a music control to start it.'})}
volume.addEventListener('input',()=>{theme.volume=Number(volume.value)/100;updateMusic();clearTimeout(saveTimer);saveTimer=window.setTimeout(()=>saveMusic().catch(()=>{}),450);tryMusic()});
mute.addEventListener('click',()=>{theme.muted=!theme.muted;updateMusic();tryMusic();saveMusic().catch(()=>{})});
$('#play').addEventListener('click',async()=>{const button=$('#play') as HTMLButtonElement;button.disabled=true;try{if(!backend)throw new Error('Launcher backend unavailable');if(state?.game.installed){await backend.PlayOffline();$('#status-detail').textContent='Game started in offline mode.'}else{await backend.InstallOrUpdate();await refresh()}}catch(error){$('#status-detail').textContent=errorText(error);await refresh().catch(()=>{})}finally{button.disabled=false}});
$('#update').addEventListener('click',async()=>{const button=$('#update') as HTMLButtonElement;button.disabled=true;try{if(!backend)throw new Error('Launcher backend unavailable');await backend.InstallOrUpdate();await refresh()}catch(error){$('#status-detail').textContent=errorText(error);await refresh().catch(()=>{})}finally{button.disabled=false}});
async function chooseFolder(){if(backend)render(await backend.ChooseInstallDirectory())}
for(const id of ['#change-folder','#setup-change','#settings-folder'])$(id).addEventListener('click',()=>chooseFolder().catch(error=>{$('#status-detail').textContent=errorText(error)}));
$('#default-folder').addEventListener('click',async()=>{if(backend)render(await backend.ResetInstallDirectory())}); $('#setup-continue').addEventListener('click',async()=>{if(backend){await backend.CompleteSetup();await refresh();tryMusic()}});
async function signIn(device:boolean){const button=$(device?'#device-signin':'#signin') as HTMLButtonElement;button.disabled=true;$('#auth-status').textContent=device?'Requesting a sign-in code…':'Complete sign-in in your system browser…';try{if(!backend)throw new Error('Launcher backend unavailable');if(device)await backend.SignInWithCode();else await backend.SignInBrowser();$('#auth-status').textContent='Signed in. Multiplayer awaits the secure hand-off.';await refresh()}catch(error){$('#auth-status').textContent=errorText(error)}finally{button.disabled=false}}
$('#signin').addEventListener('click',()=>signIn(false));$('#device-signin').addEventListener('click',()=>signIn(true));$('#signout').addEventListener('click',async()=>{if(backend){await backend.SignOut();$('#auth-status').textContent='Signed out.';await refresh()}});
window.runtime?.EventsOn('auth:device-prompt',(prompt:DevicePrompt)=>{$('#auth-status').textContent=`Enter code ${prompt.userCode} at ${prompt.verificationUri}`});
$('#discord').addEventListener('click',()=>backend?.OpenCommunity());$('#support').addEventListener('click',()=>backend?.OpenSupport());$('#files').addEventListener('click',()=>backend?.OpenGameFiles());
$('#settings').addEventListener('click',()=>$('#settings-modal').classList.remove('hidden'));$('#settings-close').addEventListener('click',()=>$('#settings-modal').classList.add('hidden'));($('#settings-mute') as HTMLInputElement).addEventListener('change',event=>{theme.muted=!(event.target as HTMLInputElement).checked;updateMusic();saveMusic().catch(()=>{});tryMusic()});
document.addEventListener('pointerdown',tryMusic,{once:true});refresh().then(tryMusic).catch(error=>{$('#status-detail').textContent=errorText(error)});
