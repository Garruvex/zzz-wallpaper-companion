(() => {
  // Three tabs along the top right, like the game's, and a side menu for the current one. The side
  // menu lists either the tab's own pages (Calendar: custom dates and holidays, each still its own
  // server page) or the sections of its one page (General, Game), shown one at a time.
  const groups = [
    { label: 'General', path: '/settings', paths: ['/settings'], rows: true, sections: [['startup', 'Startup & Updates'], ['youtube', 'YouTube Playback'], ['connection', 'Connection']] },
    { label: 'Calendar', path: '/dates', paths: ['/dates', '/holidays'], tabs: [['/dates', 'Custom Date'], ['/holidays', 'Holiday']] },
    { label: 'Game', path: '/games', paths: ['/games'], sections: [['library', 'Library'], ['add', 'Add Games'], ['controller', 'Controller'], ['keyboard', 'Keyboard']] },
  ];
  const groupOf = path => groups.find(group => group.paths.includes(path));
  const isTab = path => Boolean(groupOf(path));
  // Tabs are styled by the kit's selection states (.zzz-sel); its script adds the release blink.
  const kit = import('/ui/zzz/zzz_ui.js').catch(() => null);
  const selectable = nodes => kit.then(module => module && nodes.forEach(node => module.makeSelectable(node)));
  let page = location.pathname;
  const header = document.createElement('header'); header.className = 'shell-header';
  const brand = document.createElement('a'); brand.className = 'brand'; brand.href='/settings'; brand.append('ZZZ – Wallpaper Engine ');
  const accent = document.createElement('b'); accent.textContent='Companion Settings'; brand.append(accent); header.append(brand);
  const top = document.createElement('nav'); top.className = 'top-tabs'; top.setAttribute('aria-label', 'Companion sections');
  for (const group of groups) {
    const link = document.createElement('a'); link.href = group.path;
    // The highlight is its own layer behind the text, so it can swell while the text stays still.
    const shape = document.createElement('i'); shape.className = 'tab-shape'; shape.setAttribute('aria-hidden', 'true');
    const text = document.createElement('span'); text.textContent = group.label; link.append(shape, text); top.append(link);
  }
  header.append(top);
  // Every tab is its own server page, so each works on its own and keeps its
  // own markup. In the window they share one shell: changing tab fetches the
  // next page and swaps its content, styles and script in, so the header and
  // menu stay put and nothing flashes.
  const content = (source, path) => {
    let main = source.querySelector('main');
    if (!main) { main=source.createElement('main'); for(const child of [...source.body.children]) if(!['SCRIPT','LINK','STYLE'].includes(child.tagName)) main.append(child); }
    main.querySelectorAll('a[href="/settings"]').forEach(link => link.remove());
    main.querySelector('.sub')?.remove();
    const heading = main.querySelector('h1');
    if (heading) { heading.className='visually-hidden'; heading.nextElementSibling?.matches('p:not([id]):not([role])') && heading.nextElementSibling.remove(); }
    if(path==='/settings') for(const link of [...main.querySelectorAll('p > a')]) if(isTab(link.getAttribute('href'))) link.parentElement.remove();
    if (path === '/settings') {
      const form=main.querySelector('#settings');
      const groups=[
        ['startup','Startup & Updates',[['startup','Start with Windows'],['autoUpdate','Automatic Updates']]],
        ['youtube','YouTube Playback',[['height','Video Resolution'],['transcodeHeight','Live Stream Resolution'],['channel','YouTube Downloader Channel']]],
        ['connection','Connection',[['port','Companion Port']]]
      ];
      for(const [key,title,fields] of groups){
        const section=source.createElement('section');section.className='settings-group';section.dataset.section=key;
        const heading=source.createElement('h2');heading.className='group-title';heading.textContent=title;section.append(heading);
        for(const [id,label] of fields){const row=main.querySelector('#'+id).closest('label');row.querySelector('span').firstChild.textContent=label;section.append(row);}
        form.insertBefore(section,form.querySelector('button[type=submit]'));
      }
      form.querySelector('.section')?.remove();
    }
    if (path === '/games') {
      // Four sections: the library, adding games (the drop zone and its notes), the controller and the keyboard.
      const make = key => { const section = main.ownerDocument.createElement('section'); section.dataset.section = key; return section; };
      const library = make('library'), add = make('add'), controller = make('controller'), keyboard = make('keyboard');
      main.querySelector('#drop').before(library, add);
      main.querySelector('#pad').before(controller);
      controller.append(companionCard(main.ownerDocument, 'Controller', { body: [main.querySelector('#pad')] }));
      main.querySelector('#keys').before(keyboard);
      keyboard.append(companionCard(main.ownerDocument, 'Keyboard', { body: [main.querySelector('#keys')] }));
      add.append(...[main.querySelector('#drop'), main.querySelector('p.note')].filter(Boolean));
      library.append(...[main.querySelector('#shelfNote'), main.querySelector('.library-columns'), main.querySelector('#games')].filter(Boolean));
      add.append(companionCard(main.ownerDocument, 'Add game files', { body: [...add.childNodes] }));
    }
    // Settings are the game's settings rows: one pill per setting, its control at the right and its
    // help text in small grey under the name.
    if (path === '/settings') for (const row of main.querySelectorAll('#settings label')) row.classList.add('zzz-row');
    // Forms are cards too: a bar naming the form over its rows.
    for (const [selector, title] of [['#dateForm', 'Add a date'], ['#form', 'Location']]) {
      const form = main.querySelector(selector);
      if (!form) continue;
      const doc = main.ownerDocument, bar = doc.createElement('div'), name = doc.createElement('span'), body = doc.createElement('div');
      bar.className = 'zzz-item-bar'; name.className = 'zzz-item-title'; name.textContent = title; bar.append(name);
      body.className = 'zzz-item-body'; body.append(...form.childNodes);
      form.classList.add('zzz-item'); form.append(bar, body);
    }
    // No title card on top: the side menu already names the section, and the game's own cards carry a
    // subject (a character), not the menu entry again. General keeps the small grey section labels the
    // game's settings screens have.
    return main;
  };
  let target = null; // the tab being opened, so quick arrow presses keep stepping
  let main = content(document, page);
  let styles = [...document.querySelectorAll('style')];
  const shell=document.createElement('div'); shell.className='shell';
  const nav=document.createElement('nav'); nav.className='shell-nav'; nav.setAttribute('aria-label','Companion pages');
  // While a tab is opening, the menus already show it (target), so a click answers at once.
  const currentGroup=()=>groupOf(target||page)||groups[0];
  const chosen={}; // the section shown on each page
  const sectionOf=()=>{const list=currentGroup().sections||[];return list.some(([key])=>key===chosen[page])?chosen[page]:list[0]?.[0];};
  const showSection=()=>{const key=sectionOf();if(key)for(const node of main.querySelectorAll('[data-section]'))node.hidden=node.dataset.section!==key;};
  const arrow=direction=>{
    const button=document.createElement('button'); button.type='button'; button.className=`nav-arrow ${direction<0?'up':'down'}`;
    button.setAttribute('aria-label',direction<0?'Previous page':'Next page');
    button.addEventListener('click',()=>{
      const group=currentGroup();
      if(group.sections){
        const list=group.sections,at=Math.max(0,list.findIndex(([key])=>key===sectionOf()));
        chosen[page]=list[(at+direction+list.length)%list.length][0];mark();return;
      }
      const pages=group.tabs||[];
      const at=Math.max(0,pages.findIndex(([path])=>path===(target||page)));
      if(pages.length)show(pages[(at+direction+pages.length)%pages.length][0], true);
    });
    return button;
  };
  const items=document.createElement('div');items.className='nav-items';
  // The side menu lists the current tab's pages or sections; a tab with neither has none.
  const renderSide=()=>{
    const group=currentGroup(),section=sectionOf();
    items.replaceChildren();
    for(const [path,label] of group.tabs||[]){
      const link=document.createElement('a');link.href=path;link.className=path===(target||page)?'zzz-sel is-selected':'zzz-sel';link.textContent=label;
      if(path===(target||page))link.setAttribute('aria-current','page');
      items.append(link);
    }
    for(const [key,label] of group.sections||[]){
      const link=document.createElement('a');link.href='#'+key;link.dataset.section=key;link.className=key===section?'zzz-sel is-selected':'zzz-sel';link.textContent=label;
      if(key===section)link.setAttribute('aria-current','true');
      items.append(link);
    }
    selectable([...items.children]);
    shell.classList.toggle('no-side',!group.tabs&&!group.sections);
    if(!target)showSection();
  };
  const mark=()=>{
    const group=currentGroup();
    [...top.children].forEach((link,index)=>{
      const current=groups[index]===group;
      link.classList.toggle('is-selected',current);
      if(current)link.setAttribute('aria-current','page');else link.removeAttribute('aria-current');
    });
    renderSide();
    const sub=group.tabs?.find(([path])=>path===page)?.[1]||group.sections?.find(([key])=>key===sectionOf())?.[1];
    document.title = `${[group.label,sub].filter(Boolean).join(' · ')} · ZZZ Companion Settings`;
  };
  nav.append(arrow(-1),items,arrow(1));shell.append(nav,main);
  mark();

  const footer=document.createElement('footer');footer.className='shell-footer';footer.textContent=`Version ${companionBuildInfo.version} · Build ${companionBuildInfo.build}`;
  document.body.querySelector('.bar')?.remove();document.body.prepend(header,shell);document.body.append(footer);

  let visit = 0;
  async function show(path, push) {
    if (!isTab(path) || path === page) return;
    const token = ++visit;
    target = path;
    mark();
    let source;
    try {
      // A companion that doesn't answer within 5 s gets a normal page load instead.
      const response = await fetch(path, { cache: 'no-store', signal: AbortSignal.timeout(5000) });
      if (!response.ok) throw Error(response.statusText);
      source = new DOMParser().parseFromString(await response.text(), 'text/html');
    } catch { location.href = path; return; }
    if (token !== visit) return;
    target = null;
    // The page being left stops its timers and listeners (see zzz:leave in the pages).
    document.dispatchEvent(new Event('zzz:leave'));
    companionControls.teardown(footer);
    const scripts = [...source.querySelectorAll('script:not([src])')].map(script => { script.remove(); return script.textContent; });
    const next = content(source, path);
    const nextStyles = [...source.querySelectorAll('style')];
    // Page styles go before the shared theme, as they do in each page, so the theme still wins.
    const theme = document.querySelector('link[href="/ui/theme.css"]');
    styles.forEach(style => style.remove()); styles = nextStyles.map(style => theme.parentNode.insertBefore(document.adoptNode(style), theme));
    const adopted = document.adoptNode(next);
    main.replaceWith(adopted); main = adopted;
    page = path; mark();
    if (push) history.pushState(null, '', path);
    // Each page's script runs in its own scope, so their top-level names don't collide.
    for (const code of scripts) {
      const script = document.createElement('script');
      script.textContent = `(() => {\n${code}\n})();`;
      document.body.appendChild(script).remove();
    }
    companionControls.enhance();
    scrollTo(0, 0);
  }
  document.addEventListener('click', event => {
    const link = event.target.closest?.('a[href]');
    if (!link || link.target || event.defaultPrevented || event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    if (link.dataset.section && link.closest('.nav-items')) { event.preventDefault(); chosen[page] = link.dataset.section; mark(); return; }
    const url = new URL(link.href, location.href);
    if (url.origin !== location.origin || !isTab(url.pathname)) return;
    // The top tab you are already on stays put; its side menu changes the page.
    if (link.closest('.top-tabs') && groupOf(url.pathname) === groupOf(page)) { event.preventDefault(); return; }
    event.preventDefault(); show(url.pathname, true);
  });
  addEventListener('popstate', () => show(location.pathname, false));
})();
