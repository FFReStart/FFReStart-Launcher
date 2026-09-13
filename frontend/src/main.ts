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
      <h2 id="launch-title">Ready to play</h2>
      <label for="game-path">Game executable</label>
      <input id="game-path" type="text" spellcheck="false" autocomplete="off" placeholder="Choose the installed game executable" />
      <button id="save" class="secondary">Save path</button>
      <button id="play" class="play">Play offline</button>
      <p id="status" role="status">No account or network connection required.</p>
    </section>
  </main>`;

const backend = window.go?.main?.App;
const path = document.querySelector<HTMLInputElement>('#game-path')!;
const status = document.querySelector<HTMLParagraphElement>('#status')!;
const play = document.querySelector<HTMLButtonElement>('#play')!;

if (backend) backend.GamePath().then((value) => { path.value = value; }).catch(() => {});

document.querySelector<HTMLButtonElement>('#save')!.addEventListener('click', async () => {
  try {
    if (!backend) throw new Error('Launcher backend is unavailable');
    await backend.SetGamePath(path.value);
    status.textContent = 'Game path saved for this launcher session.';
  } catch (error) {
    status.textContent = String(error);
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
