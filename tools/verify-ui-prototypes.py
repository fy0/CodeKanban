"""Browser verification of dependency-free previews. No backend, agents or credentials.
Requires Python Playwright and Chromium. Set CHROMIUM_PATH if needed.
Uses set_content to stay offline, including in locked-down browser environments.
"""
import json, os, re
from pathlib import Path
from urllib.parse import urlsplit, unquote
from playwright.sync_api import sync_playwright
ROOT = Path(__file__).resolve().parents[1]
PREVIEW = ROOT / 'ui-prototypes'
OUT = ROOT / 'docs/ui'
PAGES = json.loads((PREVIEW / 'coverage.json').read_text(encoding='utf-8'))['pages']
SKINS = ['classic', 'studio', 'forge', 'tide']
css = '\n'.join((PREVIEW / ('shared/' + name)).read_text(encoding='utf-8') for name in ['app.css','workspace.css','designs.css'])
tokens = (PREVIEW / 'shared/tokens.js').read_text(encoding='utf-8')
app = (PREVIEW / 'shared/app.js').read_text(encoding='utf-8')
results = {'rendered': [], 'dialogs': [], 'interactions': [], 'geometry': [], 'gallery': [], 'terminalContrast': [], 'errors': []}

def mount(browser, skin, view, width=1440, mode=None):
    page = browser.new_page(viewport={'width': width, 'height': 1000 if width > 700 else 844}, device_scale_factor=1)
    page.set_default_timeout(5000)
    page.on('pageerror', lambda error: results['errors'].append(f'{skin}/{view}/{width}: {error}'))
    page.set_content(f'<!doctype html><html lang="zh-CN"><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body data-skin="{skin}" data-page="{view}"><div id="app"></div></body></html>')
    page.add_style_tag(content=css)
    page.add_script_tag(content=tokens)
    page.add_script_tag(content=app)
    if mode and page.locator('html').get_attribute('data-scheme') != mode:
        # Dispatch the real action even on narrow screens where the toolbar is hidden.
        page.evaluate("""() => {const b=document.createElement('button');b.dataset.action='mode-toggle';document.body.append(b);b.click();b.remove();}""")
    return page

with sync_playwright() as pw:
    candidates = [os.environ.get('CHROMIUM_PATH', ''), '/usr/bin/chromium']
    for env in ['PROGRAMFILES', 'PROGRAMFILES(X86)', 'LOCALAPPDATA']:
        base = os.environ.get(env)
        if base:
            candidates.extend([str(Path(base)/'Microsoft/Edge/Application/msedge.exe'), str(Path(base)/'Google/Chrome/Application/chrome.exe')])
    executable = next((candidate for candidate in candidates if candidate and Path(candidate).is_file()), None)
    browser = pw.chromium.launch(executable_path=executable, args=['--no-sandbox'] if os.name == 'posix' else [])
    for skin in SKINS:
        for width in [1440, 1024, 390]:
            for mode in ['light', 'dark']:
                for view in PAGES:
                    page = mount(browser, skin, view, width, mode)
                    assert page.locator('#app').inner_text().strip(), f'Empty {skin}/{view}'
                    assert page.locator('html').get_attribute('data-scheme') == mode
                    label = f'{skin}/{view}/{width}/{mode}'
                    if page.evaluate('document.documentElement.scrollWidth > innerWidth + 2'):
                        results['errors'].append(f'Horizontal overflow: {label}')
                    for link in page.locator('a[href]').all():
                        href = link.get_attribute('href') or ''
                        target = urlsplit(href)
                        if target.scheme or not target.path: continue
                        dest = (PREVIEW / skin / unquote(target.path)).resolve()
                        if not dest.is_file(): results['errors'].append(f'Broken link: {label}: {href}')
                    if view in ['workspace','terminal','files','changes'] and width > 700:
                        rects = page.evaluate("""Object.fromEntries(['projects','main','sessions'].map(k => {
                            const e = document.querySelector(`[data-region="${k}"]`);
                            if(!e) return [k, null]; const r=e.getBoundingClientRect();
                            return [k,{x:r.x,right:r.right,width:r.width,height:r.height}];
                        }))""")
                        a,c,r = [rects[k] for k in ['projects','main','sessions']]
                        assert a and c and r, f'Missing region: {label}'
                        assert a['width'] > 100 and r['width'] > 180 and c['width'] > 200, label
                        assert a['right'] <= c['x']+2 and c['right'] <= r['x']+2, f'Region order: {label}'
                        assert r['right'] <= width+2, f'Right sessions outside viewport: {label}'
                        results['geometry'].append({'view':label,'regions':rects})
                    if view == 'terminal' and width == 1440:
                        ratios = page.evaluate(r"""() => {
                          const bg = getComputedStyle(document.querySelector('.terminal')).backgroundColor;
                          const lum = value => value.match(/[\d.]+/g).slice(0,3).map(Number).map(c=>c/255).map(c=>c<=.04045?c/12.92:((c+.055)/1.055)**2.4).reduce((a,c,i)=>a+c*[.2126,.7152,.0722][i],0);
                          const ratio = color => (Math.max(lum(color),lum(bg))+.05)/(Math.min(lum(color),lum(bg))+.05);
                          return ['.prompt','.success-text','.subtle'].map(s=>ratio(getComputedStyle(document.querySelector('.terminal '+s)).color));
                        }""")
                        assert min(ratios) >= 4.5, f'Terminal contrast: {label}: {ratios}'
                        results['terminalContrast'].append({'view':label,'ratios':ratios})
                    results['rendered'].append(label)
                    print('render',label,flush=True)
                    if (view == 'workspace' and width in [1440,390]) or (skin != 'classic' and view in ['projects','settings','login'] and width == 1440):
                        (OUT / 'screenshots').mkdir(exist_ok=True)
                        page.screenshot(path=str(OUT/'screenshots'/f'{skin}-{view}-{width}-{mode}.png'), full_page=True)
                    page.close()
        page = mount(browser, skin, 'components')
        for button in page.locator('.component-card button').all():
            action = button.get_attribute('data-action')
            button.click()
            assert page.locator('dialog[open]').count() == 1, action
            assert page.locator('#dialog-title').inner_text().strip(), action
            page.keyboard.press('Escape')
            assert page.locator('dialog[open]').count() == 0, action
            assert button.evaluate('(el) => el === document.activeElement'), f'Focus was not restored: {action}'
            results['dialogs'].append(f'{skin}/{action}')
            print('dialog',skin,action,flush=True)
        page.close()
    # Stateful mock interactions; these do not send network requests.
    page = mount(browser, 'studio', 'projects')
    page.locator('#project-search').fill('design-system')
    assert page.locator('.project-card:visible').count() == 1
    page.locator('#project-search').fill('does-not-exist')
    assert page.locator('.project-card:visible').count() == 0
    results['interactions'].append('project search and empty results')
    page.close()
    page = mount(browser, 'forge', 'workspace')
    page.locator('#composer-input').fill('Verify local demo interaction')
    page.locator('#composer-form button[type="submit"]').click()
    assert 'Verify local demo interaction' in page.locator('#app').inner_text()
    results['interactions'].append('local conversation send')
    page.close()
    page = mount(browser, 'tide', 'settings')
    for button in page.locator('.settings-nav button').all():
        button.click()
        assert page.locator('#settings-content').inner_text().strip()
    results['interactions'].append('all 10 settings sections')
    page.close()
    for skin in SKINS:
        page = mount(browser, skin, 'workspace')
        page.locator('#workspace-project-search').fill('design-system')
        assert page.locator('[data-project-name]:visible').count() == 1
        page.locator('#session-search').fill('WebSocket')
        assert page.locator('[data-session-title]:visible').count() == 1
        for context in ['file','session','command']:
            page.locator(f'[data-action="context-{context}"]').click()
        assert '@ui/src/themes/core.ts' in page.locator('#composer-input').input_value()
        assert '#主题设计规范' in page.locator('#composer-input').input_value()
        assert '/review' in page.locator('#composer-input').input_value()
        results['interactions'].append(f'{skin}: projects / sessions search and context chips')
        page.close()
        page = mount(browser, skin, 'workspace',390)
        for action in ['workspace-projects','mobile-sessions']:
            trigger=page.locator(f'.work-tabs [data-action="{action}"]')
            trigger.click()
            assert page.locator('dialog[open]').count() == 1
            page.keyboard.press('Escape')
            assert trigger.evaluate('(e) => e === document.activeElement')
        results['interactions'].append(f'{skin}: mobile projects and sessions dialogs, focus restored')
        page.close()
    for width in [1440, 390]:
        page = browser.new_page(viewport={'width':width,'height':1000})
        page.on('pageerror', lambda error: results['errors'].append(f'Gallery: {error}'))
        html = (PREVIEW/'index.html').read_text(encoding='utf-8')
        html = re.sub(r'<script\b[^>]*src=[^>]*></script>', '', html)
        page.set_content(html)
        page.add_script_tag(content=tokens)
        page.add_script_tag(content=(PREVIEW/'shared/gallery.js').read_text(encoding='utf-8'))
        assert page.locator('.card').count() == 4
        assert not page.evaluate('document.documentElement.scrollWidth > innerWidth + 2')
        for link in page.locator('a[href]').all():
            assert (PREVIEW / link.get_attribute('href')).is_file()
        results['gallery'].append(f'gallery/{width}')
        page.close()
    browser.close()
(OUT / 'prototype-test-results.json').write_text(json.dumps(results, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'renders': len(results['rendered']), 'geometry': len(results['geometry']), 'dialogs': len(results['dialogs']), 'interactions': len(results['interactions']), 'gallery': len(results['gallery']), 'terminalContrast': len(results['terminalContrast']), 'errors': results['errors']}, ensure_ascii=False, indent=2))
assert not results['errors'], 'See docs/ui/prototype-test-results.json'
