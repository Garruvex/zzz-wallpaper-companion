(() => {
  const adapters = [];
  let opened = null;
  let serial = 0;
  const syncAll = () => adapters.forEach(control => control.sync());
  const rowName = input => {
    const label=input.closest('label');
    const span=label?.querySelector(':scope > span');
    return (span?.firstChild?.textContent || [...(label?.childNodes || [])].filter(n=>n.nodeType===Node.TEXT_NODE).map(n=>n.textContent).join('') || input.id).trim();
  };
  function dropdown(input) {
    const checkbox=input.type==='checkbox';
    const label=input.closest('label');
    if(checkbox && label && input.parentElement!==label)label.append(input);
    const wrapper=document.createElement('div'); wrapper.className='zzz-select';
    const trigger=document.createElement('button');trigger.type='button';trigger.className='zzz-select-trigger';
    trigger.setAttribute('role','combobox');trigger.setAttribute('aria-label',rowName(input));trigger.setAttribute('aria-haspopup','listbox');trigger.setAttribute('aria-expanded','false');
    const text=document.createElement('span');trigger.append(text);
    const popup=document.createElement('div');popup.id=`zzz-options-${++serial}`;popup.className='zzz-options';popup.hidden=true;popup.setAttribute('role','listbox');popup.setAttribute('aria-label',rowName(input));popup.tabIndex=-1;
    trigger.setAttribute('aria-controls',popup.id);
    input.hidden=true;input.after(wrapper);wrapper.append(trigger);document.body.append(popup);
    let choices=[],active=0,signature='';
    const value=()=>checkbox?String(input.checked):input.value;
    const data=()=>checkbox?[{value:'false',text:'Disable',disabled:false},{value:'true',text:'Enable',disabled:false}]:[...input.options].map(option=>({value:option.value,text:option.textContent,disabled:option.disabled || option.parentElement.disabled}));
    const mark=()=>{
      [...popup.children].forEach((option,index)=>{option.classList.toggle('is-active',index===active);option.setAttribute('aria-selected',String(choices[index].value===value()));});
      popup.setAttribute('aria-activedescendant',`${popup.id}-${active}`);
      trigger.setAttribute('aria-activedescendant',`${popup.id}-${active}`);
      popup.children[active]?.scrollIntoView({block:'nearest'});
    };
    const place=()=>{
      const rect=trigger.getBoundingClientRect(); const bottom=document.querySelector('.settings-dock')?.getBoundingClientRect().top || innerHeight; const available=Math.max(100,Math.max(rect.top,bottom-rect.bottom)-20);
      popup.style.width=`${rect.width}px`;popup.style.maxHeight=`${Math.min(340,available)}px`;
      popup.style.left=`${Math.max(8,Math.min(rect.left,innerWidth-rect.width-8))}px`;
      const height=popup.offsetHeight;
      popup.style.top=`${rect.bottom+height+10<=bottom?rect.bottom+8:Math.max(8,rect.top-height-8)}px`;
    };
    const close=(focus=false)=>{popup.hidden=true;wrapper.classList.remove('is-open');trigger.setAttribute('aria-expanded','false');trigger.removeAttribute('aria-activedescendant');if(opened===control)opened=null;if(focus)trigger.focus();};
    const choose=index=>{
      if(input.disabled || choices[index]?.disabled)return;
      if(checkbox)input.checked=choices[index].value==='true';else input.value=choices[index].value;
      input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));sync();close(true);
    };
    const sync=()=>{
      choices=data();trigger.disabled=input.disabled;text.textContent=choices.find(option=>option.value===value())?.text || 'Choose…';
      const next=JSON.stringify(choices);
      if(next!==signature){signature=next;popup.replaceChildren();choices.forEach((choice,index)=>{const option=document.createElement('div');option.id=`${popup.id}-${index}`;option.className='zzz-option';option.setAttribute('role','option');option.setAttribute('aria-disabled',String(Boolean(choice.disabled)));option.textContent=choice.text;option.addEventListener('click',event=>{event.preventDefault();choose(index)});popup.append(option)});}
      if(input.disabled)close();
      [...popup.children].forEach((option,index)=>option.setAttribute('aria-selected',String(choices[index].value===value())));
    };
    const show=()=>{
      if(input.disabled)return;
      opened?.close();sync();active=Math.max(0,choices.findIndex(option=>option.value===value()));
      popup.hidden=false;wrapper.classList.add('is-open');trigger.setAttribute('aria-expanded','true');opened=control;place();popup.focus();mark();
    };
    const move=direction=>{for(let count=0;count<choices.length;count++){active=(active+direction+choices.length)%choices.length;if(!choices[active].disabled)break;}mark();};
    const key=event=>{
      if(['ArrowDown','ArrowUp','Home','End','Enter',' '].includes(event.key))event.preventDefault();
      if(event.key==='Escape'){event.preventDefault();close(true);return;}
      if(event.key==='Tab'){close(true);return;}
      if(popup.hidden && ['ArrowDown','ArrowUp','Enter',' '].includes(event.key)){show();return;}
      if(event.key==='ArrowDown')move(1);else if(event.key==='ArrowUp')move(-1);
      else if(event.key==='Home'){active=choices.findIndex(option=>!option.disabled);mark();}
      else if(event.key==='End'){active=choices.findLastIndex(option=>!option.disabled);mark();}
      else if(event.key==='Enter'||event.key===' ')choose(active);
      else if(event.key.length===1){const index=choices.findIndex(option=>!option.disabled&&option.text.toLowerCase().startsWith(event.key.toLowerCase()));if(index>=0){active=index;mark();}}
    };
    trigger.addEventListener('click',event=>{event.preventDefault();popup.hidden?show():close(true)});
    trigger.addEventListener('keydown',key);popup.addEventListener('keydown',key);input.addEventListener('change',sync);
    const control={sync,close,wrapper,popup,trigger};adapters.push(control);
    new MutationObserver(sync).observe(input,{subtree:true,childList:true,attributes:true,attributeFilter:['disabled','selected','checked'],characterData:true});sync();
  }
  // A checkbox is the game's ON / OFF switch (the one on the shelf): a button that follows the hidden
  // checkbox, so the pages keep reading and setting input.checked as before.
  function toggle(input){
    const label=input.closest('label');
    if(label && input.parentElement!==label)label.append(input);
    const button=document.createElement('button');button.type='button';button.className='zzz-switch';
    button.setAttribute('role','switch');button.setAttribute('aria-label',rowName(input));
    const text=document.createElement('span');button.append(text);
    input.hidden=true;input.after(button);
    const sync=()=>{button.classList.toggle('is-on',input.checked);button.setAttribute('aria-checked',String(input.checked));text.textContent=input.checked?'ON':'OFF';button.disabled=input.disabled;};
    button.addEventListener('click',()=>{
      if(input.disabled)return;
      input.checked=!input.checked;
      input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));sync();
    });
    input.addEventListener('change',sync);
    new MutationObserver(sync).observe(input,{attributes:true,attributeFilter:['disabled','checked']});
    adapters.push({sync,close(){},wrapper:button,popup:button,trigger:button});sync();
  }
  document.addEventListener('pointerdown',event=>{if(opened&&!opened.wrapper.contains(event.target)&&!opened.popup.contains(event.target))opened.close();});
  document.addEventListener('change',()=>queueMicrotask(syncAll));
  document.addEventListener('reset',()=>queueMicrotask(syncAll));
  document.addEventListener('zzz:sync',syncAll);
  addEventListener('resize',()=>opened?.close());addEventListener('scroll',event=>{if(opened&&event.target!==opened.popup)opened.close();},true);

  // The pills come from the shared UI kit (ui/zzz, copied from the wallpaper), loaded once.
  const kit=import('/ui/zzz/zzz_ui.js');
  let generation=0;
  // A pill that does nothing while aria-disabled is "true".
  function pill(createPill,label,kind,action){
    const node=createPill(document,kind,label,()=>{if(node.getAttribute('aria-disabled')!=='true')action();});
    return node;
  }
  const follow=(node,native)=>{const set=()=>{node.setAttribute('aria-disabled',String(native.disabled));node.classList.toggle('is-disabled',native.disabled);};new MutationObserver(set).observe(native,{attributes:true,attributeFilter:['disabled']});set();};
  // The shell runs enhance() for each tab it shows and teardown() when it leaves one.
  async function enhance(){
  const visit=++generation;
  document.querySelectorAll('select').forEach(dropdown);
  document.querySelectorAll('input[type=checkbox]').forEach(toggle);
  const {createPill}=await kit;
  if(visit!==generation)return;
  for(const form of document.querySelectorAll('#settings,#form,#dateForm')){
    const native=form.querySelector('button[type=submit],button:not([type])');if(!native)continue;
    const dock=document.createElement('footer');dock.className='settings-dock';dock.setAttribute('aria-label','Settings actions');
    const actions=document.createElement('div');actions.className='settings-actions';dock.append(actions);document.body.append(dock);document.body.classList.add('has-settings-dock');
    const status=document.getElementById('status');
    const save=pill(createPill,'Save','confirm',()=>form.requestSubmit(native));native.hidden=true;actions.append(save);follow(save,native);
    if(form.id==='settings'){
      const defaults=companionBuildInfo.defaults;
      const reset=pill(createPill,'Reset','reset',()=>{
        const mapping={port:'port',height:'maxHeight',transcodeHeight:'transcodeHeight',channel:'updateChannel',startup:'launchOnStartup',autoUpdate:'autoUpdate'};
        for(const [id,key] of Object.entries(mapping)){const input=document.getElementById(id);if(input.type==='checkbox')input.checked=defaults[key];else input.value=defaults[key];}
        document.dispatchEvent(new Event('zzz:sync'));companionSay('Defaults restored. Press Save to apply.');
      });actions.prepend(reset);follow(reset,native);
    }
    if(status){status.setAttribute('role','status');dock.prepend(status);}
    const version=document.querySelector('.shell-footer');if(version)dock.append(version);
  }
  }
  // The version footer goes back to the page before the dock that holds it is removed.
  function teardown(footer){
    generation++;opened?.close();adapters.length=0;
    if(footer)document.body.append(footer);
    document.querySelectorAll('.zzz-options,.settings-dock').forEach(node=>node.remove());
    document.body.classList.remove('has-settings-dock');
  }
  window.companionControls={enhance,teardown};
  enhance();
})();
