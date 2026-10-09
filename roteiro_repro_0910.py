#!/usr/bin/env python3
"""Smoke: gerente abre #/pessoal — coleta erro REAL do console (repro bug 09/10)."""
import json
import sys
import time
import urllib.request

BASE = "http://127.0.0.1:10083"
sys.path.insert(0, '/opt/data/workspace/projetos/sci/e2e_ctx')
from driver_cdp import CDP, elemento_no_centro  # noqa: E402

INSTR = """
window.__log = [];
(function(){
  const of = console.error, ow = console.warn;
  console.error = function(...a){ try{window.__log.push('E: ' + a.map(x => (x && x.stack) ? x.stack : String(x)).join(' '));}catch(_){}; of.apply(console, a); };
  console.warn = function(...a){ try{window.__log.push('W: ' + a.map(String).join(' '));}catch(_){}; ow.apply(console, a); };
  window.addEventListener('unhandledrejection', e => { try{window.__log.push('REJ: ' + ((e.reason && (e.reason.stack || e.reason.message)) || String(e.reason)));}catch(_){}; });
  window.addEventListener('error', e => { try{window.__log.push('ERR: ' + e.message + ' @ ' + e.filename + ':' + e.lineno);}catch(_){}; });
});
"""


def reconectar(cdp):
    for _ in range(25):
        try:
            with urllib.request.urlopen("http://127.0.0.1:9223/json/list") as r:
                alvos = json.loads(r.read())
            pags = [a for a in alvos if a.get("type") == "page"]
            if pags:
                cdp._conectar(pags[0]["webSocketDebuggerUrl"])
                return
        except Exception:
            time.sleep(0.3)
    raise RuntimeError("reconexão falhou")


cdp = CDP()
cdp.viewport(1280, 900)
cdp.ir(BASE + "/?login=1")
time.sleep(0.5)
reconectar(cdp)
cdp.cmd("Runtime.enable")
st = cdp.js("""fetch('/api/login', {method:'POST', headers:{'Content-Type':'application/json','X-SCI':'1'}, body: JSON.stringify({login:'ger_repro', senha:'senha-ger'})}).then(x => x.status)""", aguardar=True)
print("login status:", st)

# navega à raiz autenticada
cdp.ir(BASE + "/")
time.sleep(1.0)
reconectar(cdp)
cdp.cmd("Runtime.enable")
cdp.js(INSTR)
for _ in range(30):
    if cdp.js("!!(document.body && document.body.innerText && document.body.innerText.toUpperCase().includes('PESSOAL'))"):
        break
    time.sleep(0.3)
print("SIDEBAR:", (cdp.js("document.body.innerText.slice(0, 400)") or "").replace("\n", " | "))

# garante instrumentação (sobrevive se houve reload)
cdp.js(INSTR)

# clique REAL no item Pessoal da sidebar
pos = None
for sel in ('a[href="#/pessoal"]', '[data-hash="#/pessoal"]', '[data-rota="#/pessoal"]'):
    pos = elemento_no_centro(cdp, sel)
    if pos:
        print("clique via", sel, pos)
        break
if not pos:
    r = cdp.js("""(() => {
      const els = [...document.querySelectorAll('a, button, [data-hash], [data-rota], .sci-sidebar *')];
      const el = els.find(e => e.children.length <= 1 && (e.innerText || '').trim().toUpperCase() === 'PESSOAL');
      if (!el) return null;
      const b = el.getBoundingClientRect();
      return JSON.stringify({x: b.left + b.width/2, y: b.top + b.height/2});
    })()""")
    d = json.loads(r) if r else None
    if d:
        pos = (d["x"], d["y"])
        print("clique via varredura de texto", pos)
if pos:
    cdp.clicar(*pos)
else:
    print("SEM ITEM PESSOAL — fallback location.hash")
    cdp.js("location.hash = '#/pessoal'")

# espera desfecho: view montada, erro ou toast
fim = None
v = None
for _ in range(40):
    v = cdp.js("""(() => {
      const app = document.querySelector('#app');
      const txt = app ? app.innerText.slice(0, 200) : '';
      const toast = [...document.querySelectorAll('.toast, [class*="toast"]')].map(t => t.innerText).join(' // ');
      return JSON.stringify({hash: location.hash, app: txt, toast});
    })()""")
    d = json.loads(v)
    tabp = cdp.js("document.querySelector('#tabP') ? 'Y' : 'N'")
    if d["toast"] or 'erro' in d["app"].lower() or 'módulo ausente' in d["app"].lower() or tabp == 'Y':
        fim = d
        break
    time.sleep(0.3)
print("DESFECHO:", json.dumps(fim, ensure_ascii=False) if fim else v)
print("ME:", cdp.js("JSON.stringify({papel: window.ME && window.ME.papel, fg: window.ME && window.ME.funcoes_grupo})"))
log = cdp.js("JSON.stringify(window.__log)")
print("CONSOLE:", (json.loads(log) if log else "[]")[:3000])
cdp.foto("/tmp/sci_repro/pessoal_final.png")
cdp.fechar()
print("FIM")
