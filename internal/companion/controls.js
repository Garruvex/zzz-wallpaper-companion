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
      const rect=trigger.getBoundingClientRect(); const available=Math.max(100,Math.max(rect.top,innerHeight-rect.bottom)-20);
      popup.style.width=`${rect.width}px`;popup.style.maxHeight=`${Math.min(340,available)}px`;
      popup.style.left=`${Math.max(8,Math.min(rect.left,innerWidth-rect.width-8))}px`;
      const height=popup.offsetHeight;
      popup.style.top=`${rect.bottom+height+10<=innerHeight?rect.bottom+8:Math.max(8,rect.top-height-8)}px`;
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
  document.querySelectorAll('select,input[type=checkbox]').forEach(dropdown);
  document.addEventListener('pointerdown',event=>{if(opened&&!opened.wrapper.contains(event.target)&&!opened.popup.contains(event.target))opened.close();});
  document.addEventListener('change',()=>queueMicrotask(syncAll));
  document.addEventListener('reset',()=>queueMicrotask(syncAll));
  document.addEventListener('zzz:sync',syncAll);
  addEventListener('resize',()=>opened?.close());addEventListener('scroll',event=>{if(opened&&event.target!==opened.popup)opened.close();},true);

  // Reuse the approved wallpaper pill component, including its cap geometry.
  const icons={save:'<svg viewBox="0 0 22 20" aria-hidden="true"><path d="M2.5 10.5L8.5 16.5L22.15 0.35" stroke="#000" stroke-width="4.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>',reset:'<svg viewBox="0 0 20 20" aria-hidden="true"><path d="M3.5 3.5L16.5 16.5M16.5 3.5L3.5 16.5" stroke="#000" stroke-width="4.5" stroke-linecap="round" fill="none"/></svg>'};
  function pill(label,kind,action){
    const node=document.createElement('div');node.className=`zzz-pill ${kind==='save'?'is-confirm':'is-cancel'}`;node.setAttribute('role','button');node.tabIndex=0;
    node.innerHTML=`<span class="zzz-pill-cap"><span class="zzz-pill-disc">${icons[kind]}</span></span><span class="zzz-pill-label"></span>`;
    node.querySelector('.zzz-pill-label').textContent=label;
    const invoke=()=>{if(node.getAttribute('aria-disabled')!=='true')action();};node.addEventListener('click',invoke);node.addEventListener('keydown',event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();invoke();}});return node;
  }
  for(const form of document.querySelectorAll('#settings,#form,#dateForm')){
    const native=form.querySelector('button[type=submit],button:not([type])');if(!native)continue;
    const actions=document.createElement('div');actions.className='settings-actions';form.append(actions);
    const status=document.getElementById('status');
    const save=pill('Save','save',()=>form.requestSubmit(native));native.hidden=true;actions.append(save);
    const disabled=()=>save.setAttribute('aria-disabled',String(native.disabled));new MutationObserver(disabled).observe(native,{attributes:true,attributeFilter:['disabled']});disabled();
    if(form.id==='settings'){
      const defaults=companionBuildInfo.defaults;
      const reset=pill('Reset','reset',()=>{
        const mapping={port:'port',height:'maxHeight',transcodeHeight:'transcodeHeight',channel:'updateChannel',startup:'launchOnStartup',autoUpdate:'autoUpdate'};
        for(const [id,key] of Object.entries(mapping)){const input=document.getElementById(id);if(input.type==='checkbox')input.checked=defaults[key];else input.value=defaults[key];}
        document.dispatchEvent(new Event('zzz:sync'));status.textContent='Defaults restored. Press Save to apply.';
      });actions.prepend(reset);
      const busy=()=>reset.setAttribute('aria-disabled',String(native.disabled));new MutationObserver(busy).observe(native,{attributes:true,attributeFilter:['disabled']});busy();
    }
    if(status)actions.after(status);
  }
})();
