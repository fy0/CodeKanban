/** Dependency-free Chromium driver for UI regression tests. Uses only a fresh temporary profile.
 * Never attaches to a user's browser, changes an application setting, or installs packages.
 * Protocol reference: https://chromedevtools.github.io/devtools-protocol/
 */
import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync, mkdirSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';

export const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
export class CDP {
  constructor(socket) {
    this.socket = socket; this.sequence = 0; this.pending = new Map(); this.listeners = new Map();
    socket.addEventListener('message', event => {
      const message = JSON.parse(String(event.data));
      if (message.id) {
        const request = this.pending.get(message.id);
        if (!request) return;
        this.pending.delete(message.id); clearTimeout(request.timer);
        message.error ? request.reject(new Error(JSON.stringify(message.error))) : request.resolve(message.result);
      } else for (const listener of this.listeners.get(message.method) || []) listener(message.params, message.sessionId);
    });
    socket.addEventListener('close', () => {
      for (const request of this.pending.values()) { clearTimeout(request.timer); request.reject(new Error('Browser disconnected')); }
      this.pending.clear();
    });
  }
  static async connect(url) {
    const socket = new WebSocket(url);
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => { socket.close(); reject(new Error('Browser connection timeout')); }, 15000);
      socket.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true });
      socket.addEventListener('error', () => { clearTimeout(timer); reject(new Error('Browser connection failed')); }, { once: true });
    });
    return new CDP(socket);
  }
  send(method, params = {}, sessionId) {
    return new Promise((resolve, reject) => {
      const id = ++this.sequence;
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 30000);
      this.pending.set(id, { resolve, reject, timer });
      this.socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  }
  on(method, listener) {
    if (!this.listeners.has(method)) this.listeners.set(method, new Set());
    this.listeners.get(method).add(listener);
    return () => this.listeners.get(method)?.delete(listener);
  }
  async page() {
    const { targetId } = await this.send('Target.createTarget', { url: 'about:blank' });
    const { sessionId } = await this.send('Target.attachToTarget', { targetId, flatten: true });
    const command = (method, params) => this.send(method, params, sessionId);
    await command('Page.enable'); await command('Runtime.enable');
    const evaluate = async expression => {
      const result = await command('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true, userGesture: true });
      if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
      return result.result.value;
    };
    return {
      targetId, sessionId, command, evaluate,
      navigate: url => command('Page.navigate', { url }),
      viewport: (width, height = 900) => command('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: width < 768 }),
      async wait(expression, timeout = 25000) {
        const until = Date.now() + timeout; let last;
        while (Date.now() < until) {
          try { const value = await evaluate(`Boolean(${expression})`); if (value) return true; } catch (error) { last = error; }
          await delay(150);
        }
        throw new Error(`Timed out waiting for ${expression.slice(0,180)}${last ? ': ' + last.message : ''}`);
      },
      async click(selector) {
        const point = await evaluate(`(() => { const e=document.querySelector(${JSON.stringify(selector)}); if(!e)throw Error('Missing element'); e.scrollIntoView({block:'nearest'});const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}; })()`);
        await command('Input.dispatchMouseEvent', { type: 'mousePressed', button: 'left', clickCount: 1, ...point });
        await command('Input.dispatchMouseEvent', { type: 'mouseReleased', button: 'left', clickCount: 1, ...point });
      },
      async screenshot(file) {
        const result = await command('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
        mkdirSync(path.dirname(file), { recursive: true }); writeFileSync(file, Buffer.from(result.data, 'base64'));
      },
      close: () => this.send('Target.closeTarget', { targetId }),
    };
  }
}
export async function launchBrowser() {
  const executable = [process.env.CHROMIUM_PATH, 'C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', '/usr/bin/chromium', '/usr/bin/google-chrome'].find(p => p && existsSync(p));
  if (!executable) throw new Error('Chrome/Edge not found. Set CHROMIUM_PATH; no automatic installation.');
  const profile = mkdtempSync(path.join(os.tmpdir(), 'codekanban-ui-check-'));
  const child = spawn(executable, ['--headless=new', '--remote-debugging-port=0', '--remote-debugging-address=127.0.0.1', `--user-data-dir=${profile}`, '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-component-update', '--disable-sync', '--disable-extensions', 'about:blank'], { stdio: 'ignore', windowsHide: true });
  let launchError; child.on('error', error => { launchError = error; });
  const portFile = path.join(profile, 'DevToolsActivePort');
  let cdp;
  try {
    for (let i=0; i<100 && !existsSync(portFile); i++) { if (launchError) throw launchError; if (child.exitCode !== null) throw new Error('Browser exited during launch'); await delay(100); }
    if (!existsSync(portFile)) throw new Error('Browser debugging endpoint unavailable');
    const [port, endpoint] = readFileSync(portFile, 'utf8').trim().split(/\r?\n/);
    cdp = await CDP.connect(`ws://127.0.0.1:${port}${endpoint}`);
    return { cdp, executable, async close() {
      try { await cdp.send('Browser.close'); } catch { /* Browser may close before replying. */ }
      cdp.socket.close();
      if (child.exitCode === null) child.kill();
      await delay(500);
      try { rmSync(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 }); } catch { /* Windows may briefly retain a lock on this temporary profile. */ }
    } };
  } catch (error) { child.kill(); try { rmSync(profile, { recursive: true, force: true }); } catch {} throw error; }
}
