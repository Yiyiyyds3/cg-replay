(function(){
  'use strict';
  var KEY='ucgrReviewGallery.v1';
  var LABEL={good:'好',minor:'小问题',severe:'严重',todo:'待查','':'未判定'};
  var BOM=String.fromCharCode(0xFEFF);
  var CRLF=String.fromCharCode(13,10);
  var LF=String.fromCharCode(10);
  var state={v:1,items:{}};
  var storageOK=true;
  try{
    var raw=localStorage.getItem(KEY);
    if(raw){var p=JSON.parse(raw); if(p&&p.items){state=p;}}
  }catch(e){storageOK=false;}

  var secs=[].slice.call(document.querySelectorAll('section.game'));
  var cur=0, filterOn=false, rafPending=false;
  var topFilter='all', stickH=200, tocTouched=false;

  function save(){
    try{localStorage.setItem(KEY,JSON.stringify(state));}
    catch(e){storageOK=false;showWarn();}
  }
  function showWarn(){
    var el=document.getElementById('storage-warn');
    if(el){el.hidden=false;}
  }
  function item(s){return state.items[s.getAttribute('data-gid')]||null;}
  function visibleSeqs(){
    return secs.filter(function(s){return !s.classList.contains('hidden');});
  }
  function idxOf(s){return secs.indexOf(s);}

  /* ---------- 顶部筛选：纯前端，只改可见性 ---------- */
  function attrNum(s,name){
    var v=s.getAttribute(name);
    if(v===null||v===''){return NaN;}
    var n=parseInt(v,10);
    return isNaN(n)?NaN:n;
  }
  function matchTop(s){
    if(topFilter==='all'){return true;}
    if(topFilter==='export'){return (s.getAttribute('data-src')||'')==='export';}
    if(topFilter==='source'){return (s.getAttribute('data-src')||'')!=='export';}
    if(topFilter==='big'){return attrNum(s,'data-cg')>200;}
    if(topFilter==='sev2'){return attrNum(s,'data-sev')>=2;}
    return true;
  }
  function isHidden(s){
    if(filterOn&&item(s)&&item(s).v){return true;}
    return !matchTop(s);
  }
  function refreshCount(){
    var el=document.getElementById('st-visible');
    if(el){el.textContent=visibleSeqs().length;}
  }
  function syncTOC(){
    [].forEach.call(document.querySelectorAll('.tocitem'),function(a){
      var s=document.getElementById(a.getAttribute('data-target'));
      if(!s){return;}
      if(s.classList.contains('hidden')){a.classList.add('off');}
      else{a.classList.remove('off');}
    });
  }
  function applyVisibility(){
    secs.forEach(function(s){
      if(isHidden(s)){s.classList.add('hidden');}
      else{s.classList.remove('hidden');}
    });
    var list=visibleSeqs();
    if(cur>=list.length){cur=list.length?list.length-1:0;}
    refreshCount();
    syncTOC();
  }

  function paintVerdict(s){
    var it=item(s);
    var v=(it&&it.v)||'';
    var chip=s.querySelector('.vchip');
    if(chip){chip.setAttribute('data-v',v); chip.textContent=(LABEL[v]||'未判定');}
    var sel=s.querySelector('.vsel');
    if(sel&&sel.value!==v){sel.value=v;}
    var note=s.querySelector('.vnote');
    if(note&&document.activeElement!==note){note.value=(it&&it.note)||'';}
    var jb=document.querySelector('.jchip[data-idx="'+idxOf(s)+'"]');
    if(jb){ if(v){jb.classList.add('done');}else{jb.classList.remove('done');} }
  }

  function doneCount(){
    var n=0;
    secs.forEach(function(s){var it=item(s); if(it&&it.v){n++;}});
    return n;
  }

  function stats(){
    var d=document.getElementById('st-done');
    var done=doneCount();
    if(d){d.textContent=done+'/'+secs.length;}
    var bar=document.getElementById('progbar');
    if(bar){bar.style.width=(secs.length?(done/secs.length*100):0)+'%';}
    var list=visibleSeqs();
    var c=document.getElementById('st-cur');
    if(c){c.textContent='#'+(list.length?Math.min(cur,list.length-1)+1:0);}
    var old=document.querySelector('.jchip.cur');
    if(old){old.classList.remove('cur');}
    var s=list[cur];
    if(s){
      var btn=document.querySelector('.jchip[data-idx="'+idxOf(s)+'"]');
      if(btn){btn.classList.add('cur');}
    }
  }

  function setActive(i,scroll){
    var list=visibleSeqs();
    if(!list.length){secs.forEach(function(s){s.classList.remove('active');});stats();return;}
    if(i<0){i=0;}
    if(i>=list.length){i=list.length-1;}
    cur=i;
    secs.forEach(function(s){s.classList.remove('active');});
    var s=list[i];
    s.classList.add('active');
    if(scroll!==false){s.scrollIntoView({block:'start',behavior:'smooth'});}
    stats();
  }

  function onScroll(){
    var list=visibleSeqs();
    if(!list.length){return;}
    var pick=0;
    for(var i=0;i<list.length;i++){
      var top=list[i].getBoundingClientRect().top;
      if(top-(stickH+8)<=0){pick=i;}else{break;}
    }
    if(pick!==cur){
      cur=pick;
      secs.forEach(function(x){x.classList.remove('active');});
      list[pick].classList.add('active');
      stats();
    }
  }

  function setVerdict(s,v){
    var it=item(s)||{};
    it.v=v;
    var note=s.querySelector('.vnote');
    if(note&&note.value!==it.note){it.note=note.value;}
    if(!it.note){it.note='';}
    it.ts=Date.now();
    state.items[s.getAttribute('data-gid')]=it;
    save();
    paintVerdict(s);
    applyVisibility();
    stats();
  }

  /* ---------- 目录导航：贴顶 + 折叠 ---------- */
  var tocPanel=document.getElementById('toc-panel');
  var tocToggle=document.getElementById('toc-toggle');
  function tocOpen(){return !!(tocPanel&&!tocPanel.hidden);}
  function syncStick(){
    var hd=document.querySelector('.topbar');
    stickH=hd?Math.round(hd.getBoundingClientRect().height):200;
    document.documentElement.style.setProperty('--stick-h',(stickH+14)+'px');
  }
  function setTOC(open){
    if(!tocPanel||!tocToggle){return;}
    tocPanel.hidden=!open;
    tocToggle.setAttribute('aria-expanded',open?'true':'false');
    syncStick();
  }
  function autoFoldTOC(){
    if(tocTouched||!tocOpen()){return;}
    var y=window.pageYOffset||document.documentElement.scrollTop||0;
    if(y>180){setTOC(false);}
  }
  if(tocToggle){
    tocToggle.addEventListener('click',function(){tocTouched=true;setTOC(!tocOpen());});
  }

  function setTopFilter(f){
    topFilter=f;
    [].forEach.call(document.querySelectorAll('.fbtn'),function(b){
      if(b.getAttribute('data-f')===f){b.classList.add('on');}
      else{b.classList.remove('on');}
    });
    applyVisibility();
    cur=0;
    setActive(0,false);
  }
  function setOnlyUnjudged(on){
    filterOn=on;
    var bf=document.getElementById('btn-filter');
    if(bf){
      if(on){bf.classList.add('on');}else{bf.classList.remove('on');}
      bf.textContent=on?'显示全部':'只看未判定';
    }
    applyVisibility();
    cur=0;
    setActive(0,false);
  }

  var tocEl=document.getElementById('toc');
  if(tocEl){
    tocEl.addEventListener('click',function(ev){
      var a=ev.target;
      while(a&&a!==tocEl&&!(a.classList&&a.classList.contains('tocitem'))){a=a.parentNode;}
      if(!a||a===tocEl||!a.getAttribute){return;}
      var s=document.getElementById(a.getAttribute('data-target'));
      if(!s){return;}
      ev.preventDefault();
      if(isHidden(s)){
        if(topFilter!=='all'){setTopFilter('all');}
        if(filterOn){setOnlyUnjudged(false);}
      }
      var list=visibleSeqs();
      var i=list.indexOf(s);
      setActive(i<0?0:i);
      tocTouched=true;
      setTOC(false);
    });
  }

  function copyText(text,btn){
    function done(ok){
      if(!btn){return;}
      var old=btn.textContent;
      btn.textContent=ok?'已复制':'复制失败';
      setTimeout(function(){btn.textContent=old;},1200);
    }
    function fallback(){
      try{
        var ta=document.createElement('textarea');
        ta.value=text;
        ta.style.position='fixed';
        ta.style.left='-9999px';
        document.body.appendChild(ta);
        ta.select();
        var ok=document.execCommand('copy');
        document.body.removeChild(ta);
        done(ok);
      }catch(e){done(false);}
    }
    if(navigator.clipboard&&navigator.clipboard.writeText){
      navigator.clipboard.writeText(text).then(function(){done(true);},fallback);
    }else{fallback();}
  }

  secs.forEach(function(s){
    paintVerdict(s);
    var sel=s.querySelector('.vsel');
    if(sel){sel.addEventListener('change',function(){setVerdict(s,sel.value);});}
    var note=s.querySelector('.vnote');
    if(note){
      var t=null;
      note.addEventListener('input',function(){
        clearTimeout(t);
        t=setTimeout(function(){
          var it=item(s)||{v:''};
          it.note=note.value;
          it.ts=Date.now();
          state.items[s.getAttribute('data-gid')]=it;
          save();
        },250);
      });
    }
    var cp=s.querySelector('.copy');
    if(cp){cp.addEventListener('click',function(){copyText(s.getAttribute('data-cgdir'),cp);});}
    s.addEventListener('mouseenter',function(){
      var list=visibleSeqs();
      var i=list.indexOf(s);
      if(i>=0&&i!==cur){
        cur=i;
        secs.forEach(function(x){x.classList.remove('active');});
        s.classList.add('active');
        stats();
      }
    });
  });

  document.addEventListener('keydown',function(ev){
    var tag=(ev.target&&ev.target.tagName)||'';
    if(tag==='INPUT'||tag==='SELECT'||tag==='TEXTAREA'){return;}
    if(ev.ctrlKey||ev.metaKey||ev.altKey){return;}
    var k=ev.key;
    if(k==='ArrowLeft'){setActive(cur-1);ev.preventDefault();return;}
    if(k==='ArrowRight'){setActive(cur+1);ev.preventDefault();return;}
    if(k==='j'||k==='J'){window.scrollBy(0,Math.round(window.innerHeight*0.85));ev.preventDefault();return;}
    if(k==='k'||k==='K'){window.scrollBy(0,-Math.round(window.innerHeight*0.85));ev.preventDefault();return;}
    if(k>='1'&&k<='4'){
      var map={'1':'good','2':'minor','3':'severe','4':'todo'};
      var s=visibleSeqs()[cur];
      if(s){setVerdict(s,map[k]);}
      ev.preventDefault();
    }
  });

  [].forEach.call(document.querySelectorAll('.jchip'),function(b){
    b.addEventListener('click',function(){
      var target=secs[parseInt(b.getAttribute('data-idx'),10)];
      if(!target){return;}
      var list=visibleSeqs();
      var i=list.indexOf(target);
      setActive(i<0?0:i);
    });
  });

  [].forEach.call(document.querySelectorAll('.fbtn'),function(b){
    b.addEventListener('click',function(){setTopFilter(b.getAttribute('data-f'));});
  });

  window.addEventListener('scroll',function(){
    if(rafPending){return;}
    rafPending=true;
    requestAnimationFrame(function(){rafPending=false;onScroll();autoFoldTOC();});
  },{passive:true});

  window.addEventListener('resize',function(){syncStick();});
  window.addEventListener('load',function(){syncStick();});

  var bf=document.getElementById('btn-filter');
  if(bf){
    bf.addEventListener('click',function(){setOnlyUnjudged(!filterOn);});
  }

  var bc=document.getElementById('btn-clear');
  if(bc){
    bc.addEventListener('click',function(){
      if(!confirm('清空本机保存的全部判定与备注？（已导出的 CSV 不受影响）')){return;}
      state={v:1,items:{}};
      save();
      secs.forEach(function(s){
        var sel=s.querySelector('.vsel');
        if(sel){sel.value='';}
        var nt=s.querySelector('.vnote');
        if(nt){nt.value='';}
        paintVerdict(s);
      });
      applyVisibility();
      stats();
    });
  }

  function csvCell(v){
    v=(v===null||v===undefined)?'':String(v);
    var need=(v.indexOf('"')>=0)||(v.indexOf(',')>=0)||(v.indexOf(LF)>=0)||(v.indexOf(CRLF)>=0);
    if(need){return '"'+v.split('"').join('""')+'"';}
    return v;
  }
  function pad(n){return (n<10?'0':'')+n;}
  function stamp(){
    var d=new Date();
    return ''+d.getFullYear()+pad(d.getMonth()+1)+pad(d.getDate())+'-'+pad(d.getHours())+pad(d.getMinutes());
  }

  var bcsv=document.getElementById('btn-csv');
  if(bcsv){
    bcsv.addEventListener('click',function(){
      var rows=[['游戏','组','批次','引擎','图片数','体积字节','cg目录','结论','备注','更新时间ISO']];
      secs.forEach(function(s){
        var it=item(s)||{};
        rows.push([
          s.getAttribute('data-name'),
          s.getAttribute('data-group'),
          s.getAttribute('data-batch'),
          s.getAttribute('data-engine'),
          s.getAttribute('data-found'),
          s.getAttribute('data-bytes'),
          s.getAttribute('data-cgdir'),
          LABEL[it.v||'']||'',
          it.note||'',
          it.ts?new Date(it.ts).toISOString():''
        ]);
      });
      var lines=rows.map(function(r){return r.map(csvCell).join(',');});
      var text=BOM+lines.join(CRLF)+CRLF;
      var blob=new Blob([text],{type:'text/csv;charset=utf-8'});
      var a=document.createElement('a');
      a.href=URL.createObjectURL(blob);
      a.download='review-verdicts-'+stamp()+'.csv';
      document.body.appendChild(a);
      a.click();
      setTimeout(function(){URL.revokeObjectURL(a.href); a.remove();},1500);
    });
  }

  if(!storageOK){showWarn();}
  applyVisibility();
  if(secs.length){setActive(0,false);}
  stats();
  syncStick();
})();