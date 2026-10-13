(() => {
  const page = location.pathname;
  const labels = [['/settings','General'],['/holidays','Holidays'],['/dates','Important Dates'],['/games','Games']];
  const header = document.createElement('header'); header.className = 'shell-header';
  const brand = document.createElement('a'); brand.className = 'brand'; brand.href='/settings'; brand.append('ZZZ – Wallpaper Engine ');
  const accent = document.createElement('b'); accent.textContent='Companion Settings'; brand.append(accent); header.append(brand);
  let main = document.querySelector('main');
  if (!main) { main=document.createElement('main'); for(const child of [...document.body.children]) if(!['SCRIPT','LINK','STYLE'].includes(child.tagName)) main.append(child); }
  main.querySelectorAll('a[href="/settings"]').forEach(link => link.remove());
  main.querySelector('.sub')?.remove();
  const heading = main.querySelector('h1');
  if (heading) { heading.className='visually-hidden'; heading.nextElementSibling?.matches('p:not([id]):not([role])') && heading.nextElementSibling.remove(); }
  main.querySelectorAll('#settings label small').forEach(hint=>{ hint.parentElement.title=hint.textContent; hint.classList.add('visually-hidden'); });
  if(page==='/settings') for(const link of [...main.querySelectorAll('p > a')]) if(labels.some(([path])=>path===link.getAttribute('href'))) link.parentElement.remove();
    document.title = `${labels.find(([path])=>path===page)?.[1] || 'General'} · ZZZ Companion Settings`;
  if (page === '/settings') {
    const form=main.querySelector('#settings');
    const groups=[
      ['Startup & Updates',[['startup','Start with Windows'],['autoUpdate','Automatic Updates']]],
      ['YouTube Playback',[['height','Video Resolution'],['transcodeHeight','Live Stream Resolution'],['channel','YouTube Downloader Channel']]],
      ['Connection',[['port','Companion Port']]]
    ];
    for(const [title,fields] of groups){
      const section=document.createElement('section');section.className='settings-group';
      const heading=document.createElement('h2');heading.className='group-title';heading.textContent=title;section.append(heading);
      for(const [id,label] of fields){const row=document.getElementById(id).closest('label');row.querySelector('span').firstChild.textContent=label;section.append(row);}
      form.insertBefore(section,form.querySelector('button[type=submit]'));
    }
    form.querySelector('.section')?.remove();
  }
  const shell=document.createElement('div'); shell.className='shell';
  const nav=document.createElement('nav'); nav.className='shell-nav'; nav.setAttribute('aria-label','Companion pages');
  const current=Math.max(0,labels.findIndex(([path])=>path===page));
  const arrow=direction=>{
    const button=document.createElement('button'); button.type='button'; button.className=`nav-arrow ${direction<0?'up':'down'}`;
    button.setAttribute('aria-label',direction<0?'Previous category':'Next category');
    button.addEventListener('click',()=>{ location.href=labels[(current+direction+labels.length)%labels.length][0]; });
    return button;
  };
  const items=document.createElement('div');items.className='nav-items';
  for(const [path,label] of labels){const link=document.createElement('a');link.href=path;link.textContent=label;if(page===path)link.setAttribute('aria-current','page');items.append(link);}
  nav.append(arrow(-1),items,arrow(1));shell.append(nav,main);
  const footer=document.createElement('footer');footer.className='shell-footer';footer.textContent=`Version ${companionBuildInfo.version} · Build ${companionBuildInfo.build}`;
  document.body.querySelector('.bar')?.remove();document.body.prepend(header,shell);document.body.append(footer);
})();
