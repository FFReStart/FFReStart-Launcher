import './style.css';

document.querySelector<HTMLDivElement>('#app')!.innerHTML = `
  <main class="shell">
    <section class="brand">
      <span class="eyebrow">FF:ReStart</span>
      <h1>Return to the Future</h1>
      <p>A Wails proof-of-concept launcher. Authentication secrets stay in the Go backend.</p>
    </section>
    <section class="panel" aria-labelledby="launch-title">
      <div><span class="status-dot"></span><span>Services online</span></div>
      <h2 id="launch-title">Ready to launch</h2>
      <button id="signin" class="primary">Sign in</button>
      <button id="device" class="secondary">Sign in with a code</button>
      <button id="play" class="play" disabled>Play</button>
      <p id="status" role="status">Choose a secure sign-in method.</p>
    </section>
  </main>`;

const status = document.querySelector<HTMLParagraphElement>('#status')!;
const play = document.querySelector<HTMLButtonElement>('#play')!;
document.querySelector('#signin')!.addEventListener('click', () => {
  status.textContent = 'System-browser login selected (S256 PKCE + loopback callback).';
  play.disabled = false;
});
document.querySelector('#device')!.addEventListener('click', () => {
  status.textContent = 'Device-code login selected (RFC 8628 polling).';
  play.disabled = false;
});
play.addEventListener('click', () => { status.textContent = 'Launch hand-off will be written to game stdin.'; });
