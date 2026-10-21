(() => {
  const labels = [['/settings','General'],['/holidays','Holidays'],['/dates','Important Dates'],['/games','Games']];
  const isTab = path => labels.some(([tab]) => tab === path);
  let page = location.pathname;
  const header = document.createElement('header'); header.className = 'shell-header';
  const brand = document.createElement('a'); brand.className = 'brand'; brand.href='/settings'; brand.append('ZZZ – Wallpaper Engine ');
  const accent = document.createElement('b'); accent.textContent='Companion Settings'; brand.append(accent); header.append(brand);
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
    main.querySelectorAll('#settings label small').forEach(hint=>{ hint.parentElement.title=hint.textContent; hint.classList.add('visually-hidden'); });
    if(path==='/settings') for(const link of [...main.querySelectorAll('p > a')]) if(isTab(link.getAttribute('href'))) link.parentElement.remove();
    if (path === '/settings') {
      const form=main.querySelector('#settings');
      const groups=[
        ['Startup & Updates',[['startup','Start with Windows'],['autoUpdate','Automatic Updates']]],
        ['YouTube Playback',[['height','Video Resolution'],['transcodeHeight','Live Stream Resolution'],['channel','YouTube Downloader Channel']]],
        ['Connection',[['port','Companion Port']]]
      ];
      for(const [title,fields] of groups){
        const section=source.createElement('section');section.className='settings-group';
        const heading=source.createElement('h2');heading.className='group-title';heading.textContent=title;section.append(heading);
        for(const [id,label] of fields){const row=main.querySelector('#'+id).closest('label');row.querySelector('span').firstChild.textContent=label;section.append(row);}
        form.insertBefore(section,form.querySelector('button[type=submit]'));
      }
      form.querySelector('.section')?.remove();
    }
    return main;
  };
  let main = content(document, page);
  let styles = [...document.querySelectorAll('style')];
  const shell=document.createElement('div'); shell.className='shell';
  const nav=document.createElement('nav'); nav.className='shell-nav'; nav.setAttribute('aria-label','Companion pages');
  const arrow=direction=>{
    const button=document.createElement('button'); button.type='button'; button.className=`nav-arrow ${direction<0?'up':'down'}`;
    button.setAttribute('aria-label',direction<0?'Previous category':'Next category');
    button.addEventListener('click',()=>{ const current=Math.max(0,labels.findIndex(([path])=>path===page)); show(labels[(current+direction+labels.length)%labels.length][0], true); });
    return button;
  };
  const items=document.createElement('div');items.className='nav-items';
  for(const [path,label] of labels){const link=document.createElement('a');link.href=path;link.textContent=label;items.append(link);}
  const mark=()=>{
    for(const link of items.children){if(link.getAttribute('href')===page)link.setAttribute('aria-current','page');else link.removeAttribute('aria-current');}
    document.title = `${labels.find(([path])=>path===page)?.[1] || 'General'} · ZZZ Companion Settings`;
  };
  mark();
  nav.append(arrow(-1),items,arrow(1));shell.append(nav,main);
  const footer=document.createElement('footer');footer.className='shell-footer';footer.textContent=`Version ${companionBuildInfo.version} · Build ${companionBuildInfo.build}`;
  document.body.querySelector('.bar')?.remove();document.body.prepend(header,shell);document.body.append(footer);

  let visit = 0;
  async function show(path, push) {
    if (!isTab(path) || path === page) return;
    const token = ++visit;
    let source;
    try {
      const response = await fetch(path, { cache: 'no-store' });
      if (!response.ok) throw Error(response.statusText);
      source = new DOMParser().parseFromString(await response.text(), 'text/html');
    } catch { location.href = path; return; }
    if (token !== visit) return;
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
    const url = new URL(link.href, location.href);
    if (url.origin !== location.origin || !isTab(url.pathname)) return;
    event.preventDefault(); show(url.pathname, true);
  });
  addEventListener('popstate', () => show(location.pathname, false));
})();
