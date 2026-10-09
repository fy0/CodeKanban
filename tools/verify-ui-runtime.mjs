/** Real production UI regression, isolated Go/SQLite/Git fixtures and a clean Chromium profile.
 * No LLM messages are submitted. Run after building the UI and disposable backend binary.
 */
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { createTestEnvironment } from './ui-test-environment.mjs';
import { launchBrowser, delay } from './ui-browser.mjs';
const count = Number(process.env.CK_TEST_SESSIONS || 1000);
const report = { scope:'Production Vue UI + real disposable Go backend/SQLite/Git; no model execution', sessionCount:count, checks:[], exceptions:[], startedAt:new Date().toISOString() };
let env, browser, page;
const $store = name => `document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get(${JSON.stringify(name)})`;
async function check(name, action) {
  const start=Date.now();
  try { const evidence=await action();report.checks.push({name,passed:true,durationMs:Date.now()-start,evidence}); }
  catch(error) { report.checks.push({name,passed:false,durationMs:Date.now()-start,error:error.message}); }
  console.log(name,report.checks.at(-1).passed?'PASS':'FAIL');
}
async function appearance(id, mode='light') {
  await page.evaluate(`(() => {const a=${$store('appearance')};a.selectTheme(${JSON.stringify(id)});a.setMode(${JSON.stringify(mode)});if(${JSON.stringify(id)}==='classic')${$store('settings')}.selectPreset(${JSON.stringify(mode)});})()`);
  await delay(120);
}
const geometry = `(() => {
 const rect=s=>{const e=document.querySelector(s);if(!e)return null;const r=e.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right};};
 const rows=[...document.querySelectorAll('.session-sidebar-item')];
 const title=document.querySelector('.session-sidebar-item-title');
 return {projects:rect('.project-sidebar-shell'),main:rect('.web-session-panel .panel-content'),sessions:rect('.session-sidebar'),rowCount:rows.length,rowHeights:rows.map(e=>e.getBoundingClientRect().height),slotHeights:[...document.querySelectorAll('.session-sidebar-virtual-row')].map(e=>e.getBoundingClientRect().height),visibleRows:rows.filter(e=>{const r=e.getBoundingClientRect(),s=document.querySelector('.session-sidebar-list').getBoundingClientRect();return r.top>=s.top&&r.bottom<=s.bottom;}).length,titleWhiteSpace:title&&getComputedStyle(title).whiteSpace,overflow:document.documentElement.scrollWidth>innerWidth+1};})()`;
try {
  env=await createTestEnvironment({sessionCount:count});browser=await launchBrowser();page=await browser.cdp.page();
  const out=path.join(env.root,'docs/ui/validation-v3');mkdirSync(out,{recursive:true});
  browser.cdp.on('Runtime.exceptionThrown',(event,sid)=>{if(sid===page.sessionId)report.exceptions.push(event.exceptionDetails.exception?.description||event.exceptionDetails.text);});
  await page.viewport(1440,900);await page.command('Network.enable');
  await page.navigate(env.origin+'/#/project/'+env.project.id);
  await page.wait("document.querySelector('.session-sidebar-item') && document.querySelector('[contenteditable=true]')",40000);
  await delay(500);
  await check('real backend lists all fixture sessions',async()=>{
    const n=await page.evaluate(`${$store('web-session')}.getSessions(${JSON.stringify(env.project.id)}).length`);assert.equal(n,count);return {loaded:n};
  });
  await page.click('[contenteditable=true]');
  await page.command('Input.insertText',{text:'主题切换保留的测试草稿 / Draft stays here.'});
  await page.evaluate("void (window.__ckCheck={root:document.querySelector('.project-workspace'),panel:document.querySelector('.web-session-panel'),editor:document.querySelector('[contenteditable=true]')})");
  const base=await page.evaluate(geometry);
  for(const id of ['classic','studio','forge','tide'])for(const mode of ['light','dark']) {
    await check(`${id}/${mode}: compact rows, fixed regions and draft continuity`,async()=>{
      await appearance(id,mode);const g=await page.evaluate(geometry);
      assert.equal(g.overflow,false);assert.ok(g.projects.right<=g.main.x+1&&g.main.right<=g.sessions.x+1);assert.ok(g.rowCount<100,'Virtual list must not render all 1000 rows');
      assert.ok(g.visibleRows>=Math.min(15,count),'Keep a dense, title-first sidebar');
      assert.ok(await page.evaluate("(()=>{const e=document.querySelector('.session-sidebar-count');return e.scrollWidth<=e.clientWidth;})()"),'Large session counts must fit their badge');
      assert.ok(g.rowHeights.every(h=>Math.abs(h-34)<1));assert.ok(g.slotHeights.every(h=>Math.abs(h-38)<1));
      assert.equal(g.titleWhiteSpace,'nowrap');assert.equal(g.sessions.width,base.sessions.width);assert.equal(g.projects.width,base.projects.width);
      const stable=await page.evaluate("window.__ckCheck.root===document.querySelector('.project-workspace')&&window.__ckCheck.panel===document.querySelector('.web-session-panel')&&window.__ckCheck.editor===document.querySelector('[contenteditable=true]')&&window.__ckCheck.editor.innerText.includes('Draft stays here.')");
      assert.ok(stable,'Theme change remounted the workspace or lost the draft');
      await page.screenshot(path.join(out,`${id}-${mode}-workspace.png`));return g;
    });
  }
  await check('large typography and comfortable density never enlarge session rows',async()=>{
    await appearance('studio');
    try {
      await page.evaluate(`${$store('appearance')}.updateLayout({fontSize:20,lineHeight:2,spacing:32,radius:24,density:'comfortable',borderWidth:2})`);await delay(150);
      const g=await page.evaluate(geometry);assert.ok(g.rowHeights.every(h=>Math.abs(h-34)<1));assert.ok(g.slotHeights.every(h=>Math.abs(h-38)<1));assert.equal(g.titleWhiteSpace,'nowrap');return {rowHeight:34,slotHeight:38};
    } finally {await page.evaluate(`${$store('appearance')}.resetOverrides()`);}
  });
  await check('1000-session virtual scroll reaches the tail without mounting the full collection',async()=>{
    await page.evaluate("(()=>{const root=document.querySelector('.session-sidebar-list');const s=[root,...root.querySelectorAll('*')].find(e=>/auto|scroll/.test(getComputedStyle(e).overflowY)&&e.scrollHeight-e.clientHeight>100);if(!s)throw Error('No scrollable virtual list');window.__ckScroller=s;s.scrollTop=s.scrollHeight;s.dispatchEvent(new Event('scroll'));})()");await delay(500);
    const result=await page.evaluate("({rendered:document.querySelectorAll('.session-sidebar-item').length,scrollTop:window.__ckScroller.scrollTop,tail:[...document.querySelectorAll('.session-sidebar-main')].some(e=>e.getAttribute('aria-label')?.startsWith('0001'))})");
    assert.ok(result.rendered<100);assert.ok(result.scrollTop>10000);assert.ok(result.tail);return result;
  });
  await check('search by title and switch with the keyboard',async()=>{
    await page.click('.session-sidebar-search-input input');await page.command('Input.insertText',{text:'0500'});
    await page.wait("document.querySelector('.session-sidebar-main')?.getAttribute('aria-label')?.startsWith('0500')",15000);
    const target=await page.evaluate("document.querySelector('.session-sidebar-main').getAttribute('aria-label')");
    await delay(350);
    await page.evaluate("document.querySelector('.session-sidebar-main').focus()");
    // Enter needs its character event as well as the key event to invoke native button activation.
    await page.command('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',text:'\r',windowsVirtualKeyCode:13,nativeVirtualKeyCode:13});
    await page.command('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
    await delay(500);
    assert.equal(await page.evaluate(`${$store('web-session')}.getActiveSession(${JSON.stringify(env.project.id)}).title`),target,'Native Enter did not switch the session');
    await page.wait("document.querySelector('.session-sidebar-main[aria-current=true]')?.getAttribute('aria-label')?.startsWith('0500')",5000);
    await page.click('.session-sidebar-search-input input');await page.command('Input.dispatchKeyEvent',{type:'keyDown',key:'a',code:'KeyA',modifiers:2,windowsVirtualKeyCode:65});await page.command('Input.dispatchKeyEvent',{type:'keyUp',key:'a',code:'KeyA',modifiers:2,windowsVirtualKeyCode:65});await page.command('Input.dispatchKeyEvent',{type:'keyDown',key:'Backspace',code:'Backspace',windowsVirtualKeyCode:8});await page.command('Input.dispatchKeyEvent',{type:'keyUp',key:'Backspace',code:'Backspace',windowsVirtualKeyCode:8});return {selected:target};
  });
  await check('Theme Studio renders, switches by card click and rejects malformed imports',async()=>{
    await page.evaluate(`${$store('appearance')}.studioOpen=true`);await page.wait("document.querySelector('.ck-theme-card')");
    await page.evaluate("[...document.querySelectorAll('.ck-theme-card')].find(e=>e.innerText.includes('Studio')).click()");
    await page.wait("document.documentElement.dataset.interface==='studio'");
    await page.evaluate("[...document.querySelectorAll('.ck-studio button')].find(e=>e.innerText.includes('导入 JSON')).click()");await page.wait("document.querySelector('.ck-json-label textarea')");await delay(400);
    await page.click('.ck-json-label textarea');await page.command('Input.insertText',{text:'{"schemaVersion":1,"id":"bad-theme","name":"Bad","extends":"missing"}'});
    assert.ok(await page.evaluate("document.querySelector('.ck-json-label textarea').value.includes('missing')"),'Import textarea did not receive the test input');
    await page.evaluate("[...document.querySelectorAll('.ck-import-modal button')].find(e=>e.innerText.includes('校验')).click()");
    await page.wait("document.querySelector('.ck-import-modal')?.textContent.includes('Unknown theme: missing')",5000);
    assert.equal(await page.evaluate(`${$store('appearance')}.themeId`),'studio');
    await page.command('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});await page.command('Input.dispatchKeyEvent',{type:'keyUp',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});
    await page.evaluate(`${$store('appearance')}.studioOpen=false`);await delay(300);return {invalidImportApplied:false};
  });
  await check('valid theme import previews both schemes before applying and persists after reload',async()=>{
    await page.evaluate(`${$store('appearance')}.studioOpen=true`);await page.wait("document.querySelector('.ck-theme-card')");
    await page.evaluate("[...document.querySelectorAll('.ck-studio button')].find(e=>e.innerText.includes('导入 JSON')).click()");
    await page.wait("document.querySelector('.ck-json-label textarea')");await delay(350);
    await page.click('.ck-json-label textarea');
    await page.command('Input.dispatchKeyEvent',{type:'keyDown',key:'a',code:'KeyA',modifiers:2,windowsVirtualKeyCode:65});await page.command('Input.dispatchKeyEvent',{type:'keyUp',key:'a',code:'KeyA',modifiers:2,windowsVirtualKeyCode:65});
    const imported={schemaVersion:1,id:'test-studio',name:'验证主题',extends:'studio',colors:{light:{accent:'#245B46'}},layout:{radius:9}};
    await page.command('Input.insertText',{text:JSON.stringify(imported)});
    await page.evaluate("[...document.querySelectorAll('.ck-import-modal button')].find(e=>e.innerText.includes('校验')).click()");
    await page.wait("document.querySelectorAll('.ck-import-previews>section').length===2");
    assert.equal(await page.evaluate(`${$store('appearance')}.themeId`),'studio','Preview must not apply a theme');
    await page.evaluate("[...document.querySelectorAll('.ck-import-modal button')].find(e=>e.innerText.includes('确认应用')).click()");
    await page.wait(`${$store('appearance')}.themeId==='test-studio'`);
    assert.equal(await page.evaluate(`${$store('appearance')}.resolved.palette.accent`),'#245B46');
    await page.command('Page.reload');await page.wait("document.querySelector('.session-sidebar-item')");
    assert.equal(await page.evaluate(`${$store('appearance')}.themeId`),'test-studio');
    assert.equal(await page.evaluate(`${$store('appearance')}.resolved.layout.radius`),9);
    await appearance('studio');return {previewSchemes:2,appliedId:'test-studio',reloadRetained:true};
  });
  await check('system mode follows live operating-system appearance changes',async()=>{
    await page.evaluate(`${$store('appearance')}.setMode('system')`);
    for(const mode of ['dark','light']) {await page.command('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:mode}]});await page.wait(`${$store('appearance')}.scheme===${JSON.stringify(mode)}`);}
    await page.command('Emulation.setEmulatedMedia',{features:[]});await appearance('studio');return {light:true,dark:true};
  });
  await check('theme changes synchronize to another real browser tab',async()=>{
    const other=await browser.cdp.page();
    try {
      await other.navigate(env.origin+'/#/');await other.wait("document.querySelector('.project-browser')");
      await other.evaluate(`${$store('appearance')}.selectTheme('forge')`);await page.wait(`${$store('appearance')}.themeId==='forge'`);
      await appearance('studio');await other.wait(`${$store('appearance')}.themeId==='studio'`);return {bidirectionalStorageEvents:true};
    } finally {await other.close();}
  });
  await check('terminal instance survives theme changes with its connection retained',async()=>{
    const {item:terminal}=await env.request(`/projects/${env.project.id}/worktrees/${env.worktree.id}/terminals`,{workingDir:env.repo,title:'UI regression terminal',rows:24,cols:80});
    try {
      await page.evaluate(`${$store('terminal')}.loadSessions(${JSON.stringify(env.project.id)})`);
      await page.click('.workspace-tab-view > .tab-header .tab-item:nth-child(1)');
      await page.wait("document.querySelector('.xterm')",20000);
      const status=`${$store('terminal')}.getSessionById(${JSON.stringify(terminal.id)})?.clientStatus`;
      await page.wait(`${status}==='ready'`,20000);
      let received='';await page.command('Network.enable');
      const remove=browser.cdp.on('Network.webSocketFrameReceived',(event,sid)=>{
        if(sid!==page.sessionId)return;
        // The real terminal protocol wraps base64 PTY bytes inside a JSON data frame.
        try {const frame=JSON.parse(event.response.payloadData);if(frame.type==='data'&&typeof frame.data==='string')received=(received+Buffer.from(frame.data,'base64').toString('utf8')).slice(-131072);}catch {/* Ignore unrelated binary frames. */}
      });
      try {
        await page.evaluate("void (window.__ckTerminal=document.querySelector('.xterm'))");
        for(const id of ['studio','forge','tide','classic']) {await appearance(id);assert.ok(await page.evaluate("window.__ckTerminal===document.querySelector('.xterm')"));assert.equal(await page.evaluate(status),'ready');}
        await page.click('.xterm-screen');
        await page.wait("document.activeElement?.classList.contains('xterm-helper-textarea')");
        await page.command('Input.insertText',{text:process.platform==='win32'?'echo CK_UI_%CD:~-12%':'printf CK_UI_READY'});
        await page.command('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',text:'\r',windowsVirtualKeyCode:13});await page.command('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
        const expected=process.platform==='win32'?'CK_UI_fixture-repo':'CK_UI_READY';
        for(let i=0;i<100&&!received.includes(expected);i++)await delay(100);
        assert.ok(received.includes(expected),'The real shell must respond over its retained WebSocket');
        const {items}=await env.request(`/projects/${env.project.id}/terminals`);assert.ok(items.some(t=>t.id===terminal.id));
        return {sameTerminalDOM:true,sameSessionId:true,clientStatus:'ready',realShellResponse:true};
      } finally {remove();}
    } finally {await env.request(`/projects/${env.project.id}/terminals/${terminal.id}/close`,{});}
  });
  for(const [n,selector] of [['files','.file-manager-panel'],['changes','.git-changes-panel']])await check(`real ${n} panel loads`,async()=>{
    await page.click(`.workspace-tab-view > .tab-header .tab-item:nth-child(${n==='files'?4:3})`);
    await page.wait(`document.querySelector(${JSON.stringify(selector)})`);return {mounted:true};
  });
  for(const [route,selector] of [['/','.project-browser'],['/settings','.general-settings-page'],['/guide','.user-guide-page'],[`/project/${env.project.id}/branches`,'.branch-page']])await check(`production route ${route.includes(env.project.id)?'branches':route}`,async()=>{
    await page.navigate(env.origin+'/#'+route);await page.wait(`document.querySelector(${JSON.stringify(selector)})`);assert.equal(await page.evaluate('document.documentElement.scrollWidth>innerWidth+1'),false);return {mounted:true};
  });
  await check('mobile workspace mounts and retains navigation at 390px',async()=>{
    await page.viewport(390,844);await page.navigate(env.origin+'/#/project/'+env.project.id);await page.wait("document.querySelector('.mobile-bottom-nav')");await appearance('studio');
    assert.equal(await page.evaluate('document.documentElement.scrollWidth>innerWidth+1'),false);
    assert.ok(await page.evaluate("document.querySelectorAll('.mobile-bottom-nav button').length>=5"));await page.screenshot(path.join(out,'studio-mobile.png'));return {width:390};
  });
  await check('real password-gated login renders in all themes and returns to the protected workspace',async()=>{
    // This changes authentication ONLY in the fresh disposable backend created above.
    await page.viewport(1440,900);await page.navigate(env.origin+'/#/settings');await page.wait("document.querySelector('.general-settings-page')");
    await page.evaluate(`${$store('auth')}.updateAccessConfig({accessRules:{bypassIPs:[],bypassDomains:[],forceAuthIPs:['127.0.0.1/32'],forceAuthDomains:[]},proxyHeader:'X-Forwarded-For',trustedProxies:[]})`);
    await page.evaluate(`${$store('auth')}.enablePasswordProtection('UI-fixture-only-842')`);
    await page.evaluate(`${$store('auth')}.logout()`);await page.navigate(env.origin+'/#/project/'+env.project.id);await page.wait("document.querySelector('.login-card')");
    assert.equal(await page.evaluate(`${$store('auth')}.canAccessProtectedContent`),false);
    for(const id of ['classic','studio','forge','tide'])for(const mode of ['light','dark']) {await appearance(id,mode);assert.equal(await page.evaluate('document.documentElement.scrollWidth>innerWidth+1'),false);if(id==='studio'&&mode==='light')await page.screenshot(path.join(out,'studio-login.png'));}
    await appearance('studio');await page.click('.login-card input[type=password]');await page.command('Input.insertText',{text:'UI-fixture-only-842'});await page.click('.login-card .n-button');
    await page.wait("document.querySelector('.project-workspace')",40000);assert.equal(await page.evaluate(`${$store('auth')}.authenticated`),true);return {loginVariants:8,realCookieAuthentication:true,protectedRedirect:true};
  });
  await check('no uncaught page exceptions',async()=>{assert.deepEqual(report.exceptions,[]);return {count:0};});
} catch(error) {report.checks.push({name:'fixture/browser setup',passed:false,error:error.message});}
finally {
  if(browser)await browser.close();if(env)await env.close();
  const out=path.resolve('docs/ui/validation-v3');mkdirSync(out,{recursive:true});report.finishedAt=new Date().toISOString();writeFileSync(path.join(out,'runtime.json'),JSON.stringify(report,null,2)+'\n');
}
const failed=report.checks.filter(c=>!c.passed);console.log(JSON.stringify({passed:report.checks.length-failed.length,failed},null,2));process.exitCode=failed.length?1:0;
