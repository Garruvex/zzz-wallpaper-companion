(() => {
  const page = location.pathname;
  const labels = [['/settings','Settings'],['/games','Game Station'],['/dates','Important Dates'],['/holidays','Public Holidays']];
  const header = document.createElement('header'); header.className = 'shell-header';
  const brand = document.createElement('a'); brand.className = 'brand'; brand.href='/settings'; brand.append('ZZZ / ');
  const accent = document.createElement('b'); accent.textContent='COMPANION'; brand.append(accent);
  const tag = document.createElement('span'); tag.className='shell-tag'; tag.textContent='YOUR DESKTOP. YOUR SIGNAL.'; header.append(brand,tag);
  let main = document.querySelector('main');
  if (!main) { main=document.createElement('main'); for(const child of [...document.body.children]) if(!['SCRIPT','LINK','STYLE'].includes(child.tagName)) main.append(child); }
  main.querySelectorAll('a[href="/settings"]').forEach(link => link.remove());
  if(page==='/settings') for(const link of [...main.querySelectorAll('p > a')]) if(labels.some(([path])=>path===link.getAttribute('href'))) link.parentElement.remove();
  const shell=document.createElement('div'); shell.className='shell';
  const nav=document.createElement('nav'); nav.className='shell-nav'; nav.setAttribute('aria-label','Companion pages');
  const caption=document.createElement('div'); caption.className='nav-caption'; caption.textContent='CONTROL CENTER'; nav.append(caption);
  for(const [path,label] of labels){const link=document.createElement('a');link.href=path;link.textContent=label;if(page===path)link.setAttribute('aria-current','page');nav.append(link);}
  const note=document.createElement('div');note.className='nav-footer';note.textContent='ZZZ WALLPAPER / LOCAL';nav.append(note);
  shell.append(nav,main);
  const footer=document.createElement('footer');footer.className='shell-footer';footer.textContent='ZZZ WALLPAPER COMPANION // CONNECTED TO YOUR WORLD';
  document.body.querySelector('.bar')?.remove();document.body.prepend(header,shell);document.body.append(footer);
})();
