/** Real Chromium file:// checks for the offline collection. No dependencies or backend. */
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { launchBrowser, delay } from './ui-browser.mjs';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const out=path.join(root,'docs/ui/validation-v3');mkdirSync(out,{recursive:true});
const report={scope:'Offline static UI in Chromium, not backend E2E',renders:0,dialogs:0,compactChecks:0,errors:[],startedAt:new Date().toISOString()};
const themes=['classic','studio','forge','tide'];
const views=['projects','workspace','terminal','files','changes','branches','settings','guide','components','login'];
const browser=await launchBrowser();
try {
  const p=await browser.cdp.page();browser.cdp.on('Runtime.exceptionThrown',(e,s)=>{if(s===p.sessionId)report.errors.push({type:'exception',message:e.exceptionDetails.exception?.description||e.exceptionDetails.text});});
  for(const mode of ['light','dark']) {
    const {identifier}=await p.command('Page.addScriptToEvaluateOnNewDocument',{source:`for(const t of ${JSON.stringify(themes)})localStorage.setItem('ck-prototype:'+t,JSON.stringify({mode:${JSON.stringify(mode)}}));`});
    for(const width of [1440,1024,390]) {
      await p.viewport(width,width===390?844:900);
      for(const theme of themes)for(const view of views) {
        const label=`${theme}/${mode}/${width}/${view}`;
        try {
          await p.navigate(pathToFileURL(path.join(root,'ui-prototypes',theme,view+'.html')).href);
          await p.wait(`document.body.dataset.page===${JSON.stringify(view)} && document.querySelector('#demo-dialog')`);await delay(20);
          assert.equal(await p.evaluate('document.documentElement.scrollWidth>innerWidth+1'),false,'Whole-page horizontal overflow');
          assert.equal(await p.evaluate('document.documentElement.dataset.scheme'),mode);
          if(['workspace','files','changes','terminal'].includes(view)&&width>680) {
            const g=await p.evaluate(`(()=>{const r=s=>document.querySelector(s).getBoundingClientRect();const a=r('[data-region=projects]'),b=r('[data-region=main]'),c=r('[data-region=sessions]');return {left:a.right,centerX:b.x,centerRight:b.right,right:c.x,rightEdge:c.right,heights:[...document.querySelectorAll('.session-row')].map(e=>e.getBoundingClientRect().height),titleWrap:getComputedStyle(document.querySelector('.session-row .session-title')).whiteSpace};})()`);
            assert.ok(g.left<=g.centerX+1&&g.centerRight<=g.right+1&&g.rightEdge<=width+1);
            assert.ok(g.heights.every(h=>Math.abs(h-34)<1));assert.equal(g.titleWrap,'nowrap');report.compactChecks++;
          }
          if(theme==='studio'&&view==='workspace'&&width===1440)await p.screenshot(path.join(out,`studio-${mode}-prototype.png`));
          report.renders++;
        } catch(error) {report.errors.push({label,error:error.message});}
      }
    }
    await p.command('Page.removeScriptToEvaluateOnNewDocument',{identifier});
  }
  await p.viewport(1440);
  for(const theme of themes) {
    await p.navigate(pathToFileURL(path.join(root,'ui-prototypes',theme,'components.html')).href);await p.wait("document.querySelector('.component-card')");
    const actions=await p.evaluate("[...document.querySelectorAll('.component-card [data-action]')].map(e=>e.dataset.action)");
    for(const action of actions) {
      try {
        await p.click(`.component-card [data-action="${action}"]`);await p.wait("document.querySelector('#demo-dialog').open");
        await p.command('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});await p.command('Input.dispatchKeyEvent',{type:'keyUp',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});
        await p.wait("!document.querySelector('#demo-dialog').open");report.dialogs++;
      } catch(error){report.errors.push({theme,action,error:error.message});}
    }
  }
} finally {await browser.close();}
report.finishedAt=new Date().toISOString();writeFileSync(path.join(out,'static.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report,null,2));process.exitCode=report.errors.length?1:0;
