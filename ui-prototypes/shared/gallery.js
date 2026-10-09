/* Local manifests only. Text is never interpreted as HTML. */
(() => {
  const parent = document.querySelector('#collection');
  const make = (tag, cls, text) => {
    const element = document.createElement(tag);
    if (cls) element.className = cls;
    if (text) element.textContent = text;
    return element;
  };
  for (const theme of window.CK_THEMES) {
    const card = make('article', 'card');
    const preview = make('a', `preview ${theme.id}`);
    preview.href = `${theme.id}/workspace.html`;
    preview.setAttribute('aria-label', `浏览 ${theme.nameEn} 工作区`);
    const colors = theme.colors[theme.defaultMode === 'dark' ? 'dark' : 'light'];
    for (const [key, source] of Object.entries({bg:'canvas',panel:'surface',ink:'text',line:'border',accent:'accent'})) preview.style.setProperty(`--${key}`, colors[source]);
    const left = make('div', 'left');
    left.append(make('small', '', 'PROJECTS'), make('b', '', 'CK'));
    for (let i=0; i<4; i++) left.append(make('i'));
    const center = make('div', 'center');
    center.append(make('small', '', 'WORKSPACE'), make('b', '', 'Build something remarkable.'), make('div', 'bubble', 'A focused place to think.'), make('i'), make('i'));
    const right = make('div', 'right');
    right.append(make('small', '', 'SESSIONS'), make('i', 'active'), make('i'), make('i'), make('i'));
    preview.append(left, center, right);
    const info = make('div', 'info');
    info.append(make('h2', '', theme.name), make('p', '', theme.description));
    const links = make('div', 'links');
    for (const [page, label] of [['workspace','打开工作区 →'],['projects','项目总览'],['settings','主题设置'],['components','弹窗与状态']]) {
      const link = make('a', '', label);
      link.href = `${theme.id}/${page}.html`;
      links.append(link);
    }
    info.append(links);
    card.append(preview, info);
    parent.append(card);
  }
})();
