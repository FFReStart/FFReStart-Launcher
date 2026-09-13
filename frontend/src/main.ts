import './style.css';

document.querySelector<HTMLDivElement>('#app')!.innerHTML = `
  <main class="shell">
    <section class="brand">
      <span class="eyebrow">FF:ReStart</span>
      <h1>Your world.<br />Your way.</h1>
      <p>Play singleplayer entirely offline. An unavailable update service never keeps you from your game.</p>
    </section>
    <section class="panel" aria-labelledby="launch-title">
      <div class="mode"><span class="status-dot"></span>Offline singleplayer</div>
      <h2 id="launch-title">Game status</h2>
      <p id="update-status" class="update-status">Checking the installed version…</p>
      <button id="install" class="secondary">Install or Update</button>
      <button id="play" class="play" disabled>Play offline</button>
      <p id="status" role="status">No account or network connection required.</p>
      <details class="settings">
        <summary>Development settings</summary>
        <label for="game-path">Game executable override</label>
        <input id="game-path" type="text" spellcheck="false" autocomplete="off" placeholder="Leave blank to use the installed version" />
        <button id="save" class="secondary">Save override</button>
      </details>
    </section>
  </main>`;

const backend = window.go?.main?.App;
const path = document.querySelector<HTMLInputElement>('#game-path')!;
const status = document.querySelector<HTMLParagraphElement>('#status')!;
const play = document.querySelector<HTMLButtonElement>('#play')!;
const install = document.querySelector<HTMLButtonElement>('#install')!;
const updateStatus = document.querySelector<HTMLParagraphElement>('#update-status')!;

const refreshStatus = async () => {
  if (!backend) return;
  const game = await backend.GetGameStatus();
  updateStatus.textContent = game.message;
  play.disabled = !game.installed;
};

if (backend) {
  backend.GamePath().then((value) => { path.value = value; }).catch(() => {});
  refreshStatus().catch(() => { updateStatus.textContent = 'Unable to read the installed version.'; });
}

document.querySelector<HTMLButtonElement>('#save')!.addEventListener('click', async () => {
  try {
    if (!backend) throw new Error('Launcher backend is unavailable');
    await backend.SetGamePath(path.value);
    status.textContent = path.value.trim() ? 'Development override saved for this session.' : 'Using the installed current version.';
    await refreshStatus();
  } catch (error) {
    status.textContent = String(error);
  }
});

install.addEventListener('click', async () => {
  install.disabled = true;
  status.textContent = 'Installing the latest signed game version…';
  try {
    if (!backend) throw new Error('Launcher backend is unavailable');
    await backend.InstallOrUpdate();
    status.textContent = 'Install or update complete.';
    await refreshStatus();
  } catch (error) {
    status.textContent = String(error);
    await refreshStatus().catch(() => {});
  } finally {
    install.disabled = false;
  }
});

play.addEventListener('click', async () => {
  play.disabled = true;
  try {
    if (!backend) throw new Error('Launcher backend is unavailable');
    await backend.PlayOffline();
    status.textContent = 'Game started in offline mode.';
  } catch (error) {
    status.textContent = String(error);
  } finally {
    play.disabled = false;
  }
});
