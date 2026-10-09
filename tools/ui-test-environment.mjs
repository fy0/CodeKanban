/** Disposable real Go backend + production UI. No existing database or browser is touched.
 * Build output/ui-audit-v3/codekanban-check.exe (or set CK_TEST_BINARY), and ui/dist first.
 * Empty sessions are metadata fixtures; this helper NEVER dispatches a model message.
 */
import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, mkdirSync, writeFileSync, rmSync, cpSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { delay } from './ui-browser.mjs';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve)); return port;
}
export async function createTestEnvironment({ sessionCount = 48 } = {}) {
  if (!Number.isInteger(sessionCount) || sessionCount < 1 || sessionCount > 1200) throw new Error('Fixture sessionCount must be 1..1200');
  const binary = process.env.CK_TEST_BINARY || path.join(root, 'output/ui-audit-v3/codekanban-check' + (process.platform === 'win32' ? '.exe' : ''));
  if (!existsSync(binary) || !existsSync(path.join(root, 'ui/dist/index.html'))) throw new Error('Build the test binary and production UI first');
  const temporary = mkdtempSync(path.join(os.tmpdir(), 'codekanban-regression-'));
  const home = path.join(temporary, 'home'); const repo = path.join(temporary, 'fixture-repo');
  for (const dir of [home, repo, path.join(home, 'AppData/Roaming'), path.join(home, 'AppData/Local')]) mkdirSync(dir, { recursive: true });
  // Explicitly pass only process/platform essentials; credentials and provider configuration are not inherited.
  const env = {};
  for (const key of ['PATH','Path','SystemRoot','SYSTEMROOT','WINDIR','COMSPEC','ComSpec','PATHEXT','TEMP','TMP','LANG','LC_ALL','NUMBER_OF_PROCESSORS']) if (process.env[key]) env[key] = process.env[key];
  Object.assign(env, { HOME:home, USERPROFILE:home, APPDATA:path.join(home,'AppData/Roaming'), LOCALAPPDATA:path.join(home,'AppData/Local'), XDG_CONFIG_HOME:path.join(home,'.config'), CODEX_HOME:path.join(home,'.codex'), CLAUDE_CONFIG_DIR:path.join(home,'.claude'), GIT_CONFIG_NOSYSTEM:'1', HTTP_PROXY:'http://127.0.0.1:1', HTTPS_PROXY:'http://127.0.0.1:1', NO_PROXY:'127.0.0.1,localhost' });
  const git = (...args) => {
    const result = spawnSync('git', args, { cwd:repo, env, encoding:'utf8', windowsHide:true, timeout:15000 });
    if (result.status !== 0) throw new Error(`Fixture git failed: ${args[0]} (${result.status})`);
  };
  let backend; let backendError;
  const close = async () => {
    if (backend && backend.exitCode === null) {
      backend.kill();
      for (let i=0;i<30 && backend.exitCode===null;i++) await delay(100);
    }
    try { rmSync(temporary,{recursive:true,force:true,maxRetries:8,retryDelay:250}); } catch { console.warn('A temporary fixture directory remains locked; no project data was modified.'); }
  };
  try {
    git('init','-b','main');
    writeFileSync(path.join(repo,'README.md'),'# UI regression fixture\n\nDisposable test project.\n');
    git('add','README.md'); git('-c','user.name=UI Test','-c','user.email=ui-test@example.invalid','commit','-m','Initialize disposable fixture');
    writeFileSync(path.join(repo,'README.md'),'# UI regression fixture\n\nA sample change for the real Git changes panel.\n');
    // The real static mount expects <uiOverwrite>/static/index.html.
    cpSync(path.join(root,'ui/dist'),path.join(temporary,'static'),{recursive:true});
    const port = await freePort(); const origin = `http://127.0.0.1:${port}`;
    writeFileSync(path.join(temporary,'config.yaml'), `disableAutoOpenBrowser: true\nprintConfig: false\nlogLevel: error\nuiOverwrite: ${JSON.stringify(temporary.replaceAll('\\','/'))}\nterminal:\n  shell:\n    windows: 'cmd.exe'\n`);
    backend = spawn(binary,['--bind','127.0.0.1','--port',String(port)],{cwd:temporary,env,stdio:'ignore',windowsHide:true});
    backend.on('error',error=>{backendError=error;});
    const request = async (route, data, method = data === undefined ? 'GET' : 'POST') => {
      const response = await fetch(origin + '/api/v1' + route, { method, ...(data === undefined ? {} : { headers:{'Content-Type':'application/json'},body:JSON.stringify(data) }), signal:AbortSignal.timeout(15000) });
      const body = await response.json();
      if (!response.ok) throw new Error(`Fixture API ${method} ${route}: ${response.status} ${JSON.stringify(body).slice(0,600)}`);
      return body.body || body;
    };
    let ready = false;
    for (let i=0;i<150;i++) {
      if (backendError) throw backendError;
      if (backend.exitCode !== null) throw new Error(`Disposable backend exited: ${backend.exitCode}`);
      try { const status = await request('/auth/status'); if (status.item?.enabled) throw new Error('Unexpected authentication configuration in fresh fixture'); ready=true; break; } catch (error) { if (error.message.includes('Unexpected authentication')) throw error; }
      await delay(100);
    }
    if (!ready) throw new Error('Disposable backend did not become ready');
    const { item:project } = await request('/projects/create',{name:'CodeKanban UI Lab',path:repo,description:'Disposable regression fixture',hidePath:true});
    // Project creation queues worktree discovery. Observe it rather than racing a second sync.
    let worktree;
    for (let attempt=0;attempt<100;attempt++) {
      const {items}=await request(`/projects/${project.id}/worktrees`);
      worktree=(items||[]).find(w=>w.isMain)||items?.[0];
      if(worktree)break;
      await delay(100);
    }
    if (!worktree) throw new Error('Fixture main worktree missing');
    const names = ['检查主题切换后的草稿保留','优化中文标题搜索与快速切换','回归终端字体和深色配色','检查文件预览与 Git 差异','整理工作树生命周期','修复移动端输入法遮挡','Review streaming markdown edge cases','校验备份导入与主题继承'];
    const sessions = [];
    for (let offset=0;offset<sessionCount;offset+=6) {
      const batch = await Promise.all(Array.from({length:Math.min(6,sessionCount-offset)},async (_,j)=>{
        const i=offset+j;
        const { item } = await request(`/projects/${project.id}/web-sessions`,{worktreeId:worktree.id,agent:'codex',claudeRuntime:'',model:'',reasoningEffort:'',workflowMode:'default',permissionLevel:'standard',autoRetryEnabled:false,title:`${String(i+1).padStart(4,'0')} · ${names[i%names.length]}`});
        return item;
      })); sessions.push(...batch);
    }
    return { root, temporary, repo, origin, request, project, worktree, sessions, close };
  } catch(error) { await close(); throw error; }
}
