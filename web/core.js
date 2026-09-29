/* SCI — core do frontend (vanilla, sem framework, sem build).
   Shell + router + helpers globais compartilhados pelas views.

   Contrato (window.*): api, esc, toast, fmtData, fmtHora, pill, abrirModal,
   confirmar, navAtiva, montarShell, rotear, SCI_BOOT.
   Views esperadas (definidas nos outros scripts, toleradas ausentes):
     window.ViewHoje  window.ViewRelatorios  window.ViewAdmin
     window.ViewGrupos  window.ViewPerfil    (window.ViewDashboard vive aqui)

   index.html (DEV-A) chama DEPOIS dos 3 scripts:
     <script src="/core.js?v=200"></script>
     <script src="/views_conf.js?v=200"></script>
     <script src="/views_gestao.js?v=200"></script>
     <script>SCI_BOOT();</script>

   Tudo vive numa IIFE de propósito: nenhum const/let top-level além dos
   window.*, para não colidir com declarações dos scripts das views. */
(function () {
'use strict';

/* ---------- DOM básico ---------- */
function $(sel, raiz) { return (raiz || document).querySelector(sel); }
function $$(sel, raiz) { return Array.prototype.slice.call((raiz || document).querySelectorAll(sel)); }
window.$ = $;
window.$$ = $$;

function garantirApp() {
  let app = $('#app');
  if (!app) { app = document.createElement('main'); app.id = 'app'; document.body.appendChild(app); }
  return app;
}

/* ---------- escape ---------- */
function esc(s) {
  return String(s === null || s === undefined ? '' : s)
    .replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
window.esc = esc;

/* ---------- usuário da sessão ---------- */
let ME = null;
function definirUsuario(u) { ME = u; window.SCI_ME = u; window.ME = u; }
window.definirUsuario = definirUsuario;

function rotuloPapel(p) { return p === 'admin' ? 'ADMIN' : p === 'gerente' ? 'GERENTE' : 'OPERADOR'; }
window.rotuloPapel = rotuloPapel;

function rotaInicial() { return ME && ME.papel === 'admin' ? '#/admin' : '#/hoje'; }

/* navega p/ hash; se já estiver nele, roteia direto (hashchange não dispara) */
function irPara(hash) {
  if (location.hash === hash) rotear();
  else location.hash = hash;
}

/* ---------- API ---------- */
/* Escrita (não-GET) leva X-SCI: 1 (anti-CSRF). FormData não recebe Content-Type.
   401 (fora do /api/login) → encerra sessão e volta à tela de login.
   Erro JSON {"erro": msg} → toast + throw (err.status disponível). */
async function api(path, opts) {
  opts = opts || {};
  const metodo = String(opts.method || 'GET').toUpperCase();
  const headers = Object.assign({}, opts.headers || {});
  if (!(opts.body instanceof FormData)) headers['Content-Type'] = headers['Content-Type'] || 'application/json';
  if (metodo !== 'GET') headers['X-SCI'] = '1';
  const conf = Object.assign({}, opts, { headers: headers });

  let r;
  try { r = await fetch(path, conf); }
  catch (e) { toast('Servidor inacessível — verifique a rede', 'erro'); throw e; }

  if (r.status === 401 && path.indexOf('/api/login') === -1) {
    definirUsuario(null);
    irPara('#/login');
    const err = new Error('não autenticado'); err.status = 401; throw err;
  }
  const ehJson = (r.headers.get('content-type') || '').indexOf('json') !== -1;
  const data = ehJson ? await r.json().catch(() => null) : await r.text();
  if (!r.ok) {
    const msg = (data && data.erro) || ('Falha ' + r.status);
    toast(msg, 'erro');
    const err = new Error(msg); err.status = r.status; throw err;
  }
  return data;
}
window.api = api;

/* ---------- toast ---------- */
function toast(msg, tipo) {
  let cont = $('#toasts');
  if (!cont) { cont = document.createElement('div'); cont.id = 'toasts'; document.body.appendChild(cont); }
  const d = document.createElement('div');
  d.className = 'toast' + (tipo === 'erro' ? ' erro' : '');
  d.textContent = msg;
  cont.appendChild(d);
  setTimeout(() => d.remove(), 4000);
}
window.toast = toast;

/* ---------- datas ---------- */
function fmtData(s) {
  if (!s) return '—';
  const m = String(s).slice(0, 10).match(/^(\d{4})-(\d{2})-(\d{2})$/);
  return m ? m[3] + '/' + m[2] + '/' + m[1] : String(s);
}
window.fmtData = fmtData;

function fmtHora(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return isNaN(d) ? '—' : d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
}
window.fmtHora = fmtHora;

function dataLocal(d) {
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
}
window.dataLocal = dataLocal;

function semanaDe(isoDia) { // [segunda, domingo] da semana do dia informado
  const d = new Date(isoDia + 'T12:00:00');
  const dow = (d.getDay() + 6) % 7;
  const ini = new Date(d.getTime() - dow * 864e5);
  const fim = new Date(ini.getTime() + 6 * 864e5);
  return [dataLocal(ini), dataLocal(fim)];
}
window.semanaDe = semanaDe;

function fimDoMes(ym) {
  const p = ym.split('-');
  return dataLocal(new Date(+p[0], +p[1], 0));
}
window.fimDoMes = fimDoMes;

/* ---------- pill de situação ---------- */
const ROTULO_SIT = { presente: 'Presente', atraso: 'Atraso', falta: 'Falta', justificada: 'Justificada' };
function pill(situacao) {
  const s = String(situacao === null || situacao === undefined ? '' : situacao);
  const classe = s.toLowerCase().replace(/[^a-z0-9_-]/g, '');
  return '<span class="pill pill-' + classe + '">' + esc(ROTULO_SIT[classe] || s) + '</span>';
}
window.pill = pill;

/* ---------- modal ---------- */
/* abrirModal(html) → {fechar(), mask, modal}. Fecha em Esc, clique na máscara
   ou fechar(); aoFechar opcional dispara uma única vez em qualquer fechamento. */
function abrirModal(html, aoFechar) {
  const mask = document.createElement('div');
  mask.className = 'modal-mask';
  const modal = document.createElement('div');
  modal.className = 'modal';
  modal.innerHTML = html;
  mask.appendChild(modal);
  document.body.appendChild(mask);

  let fechado = false;
  function noEsc(ev) { if (ev.key === 'Escape') fechar(); }
  function fechar() {
    if (fechado) return;
    fechado = true;
    document.removeEventListener('keydown', noEsc, true);
    mask.remove();
    if (typeof aoFechar === 'function') aoFechar();
  }
  mask.addEventListener('click', ev => { if (ev.target === mask) fechar(); });
  document.addEventListener('keydown', noEsc, true);
  const inp = modal.querySelector('input, select, textarea');
  if (inp) inp.focus();
  return { fechar: fechar, mask: mask, modal: modal };
}
window.abrirModal = abrirModal;

/* confirmar(msg) → Promise<bool> (Esc/máscara/Cancelar = false) */
function confirmar(msg) {
  return new Promise(resolve => {
    let decidido = false;
    const decidir = v => { if (decidido) return; decidido = true; resolve(v); h.fechar(); };
    const h = abrirModal(
      '<p>' + esc(msg) + '</p>' +
      '<div class="modal-acoes">' +
        '<button type="button" class="fantasma" data-cfm="0">Cancelar</button>' +
        '<button type="button" class="primario" data-cfm="1">Confirmar</button>' +
      '</div>',
      () => decidir(false));
    $$('[data-cfm]', h.modal).forEach(b => {
      b.addEventListener('click', () => decidir(b.dataset.cfm === '1'));
    });
  });
}
window.confirmar = confirmar;

/* ---------- modal "Mudar senha" (dropdown) ---------- */
function modalSenha() {
  const h = abrirModal(
    '<h3>Alterar minha senha</h3>' +
    '<div class="campo"><label>Senha atual</label><input type="password" id="mA" autocomplete="current-password"></div>' +
    '<div class="campo"><label>Nova senha (mín. 8)</label><input type="password" id="mN" autocomplete="new-password"></div>' +
    '<div class="campo"><label>Confirmar nova senha</label><input type="password" id="mC" autocomplete="new-password"></div>' +
    '<div class="modal-acoes">' +
      '<button type="button" class="fantasma" id="mCancel">Cancelar</button>' +
      '<button type="button" class="primario" id="mGo">Salvar</button>' +
    '</div>');
  const q = s => h.modal.querySelector(s);
  q('#mCancel').addEventListener('click', h.fechar);
  q('#mGo').addEventListener('click', async () => {
    const a = q('#mA').value, n = q('#mN').value, c = q('#mC').value;
    if (!a || !n) { toast('Preencha a senha atual e a nova', 'erro'); return; }
    if (n.length < 8) { toast('Nova senha: mínimo 8 caracteres', 'erro'); return; }
    if (n !== c) { toast('As novas senhas não conferem', 'erro'); return; }
    try {
      await api('/api/senha', { method: 'POST', body: JSON.stringify({ atual: a, nova: n }) });
      toast('Senha alterada');
      h.fechar();
    } catch (e) { /* toast já exibido pelo api() */ }
  });
}
window.modalSenha = modalSenha;

/* ---------- shell (topbar + nav + dropdown do usuário) ---------- */
function montarShell(usuario) {
  definirUsuario(usuario);
  let topbar = $('#topbar');
  if (!topbar) {
    topbar = document.createElement('header');
    topbar.id = 'topbar';
    document.body.prepend(topbar);
  }
  topbar.classList.remove('oculto');
  topbar.innerHTML =
    '<button type="button" id="btBurger" aria-label="menu" aria-expanded="false">' +
      '<span class="burg-x"></span><span class="burg-x"></span><span class="burg-x"></span>' +
    '</button>' +
    '<div class="marca"><span class="logo">SCI</span><span class="sub">Controle Interno</span></div>' +
    '<div id="tituloMob"></div>' +
    '<nav id="nav"></nav>' +
    '<div id="burgMask" class="oculto"></div>' +
    '<div class="sessao"><div id="menuUsuario">' +
      '<button type="button" id="quem" title="conta"></button>' +
      '<div id="menuUsuarioItens" class="oculto">' +
        '<button type="button" id="btPerfil">Meu usuário</button>' +
        '<button type="button" id="btSenha">Mudar senha</button>' +
        '<button type="button" id="btnSair">Logout</button>' +
      '</div>' +
    '</div></div>';

  const papel = usuario && usuario.papel;
  let itens;
  if (papel === 'admin') {
    itens = [['#/dashboard', 'DASHBOARD'], ['#/admin', 'ADMIN'], ['#/relatorios', 'RELATÓRIOS']];
  } else {
    itens = [['#/hoje', 'CONFERÊNCIA'], ['#/relatorios', 'RELATÓRIOS']];
    if (papel === 'gerente') itens.push(['#/grupos', 'GERENCIAR']);
  }
  $('#nav', topbar).innerHTML = itens.map(it => '<a href="' + it[0] + '">' + it[1] + '</a>').join('');
  // botão de usuário GENÉRICO (ícone) — ordem Tenente 29/09: login/função truncavam
  $('#quem', topbar).innerHTML =
    '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">' +
    '<circle cx="12" cy="8" r="4" stroke="currentColor" stroke-width="1.8"/>' +
    '<path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>' +
    '</svg>';
  $('#quem', topbar).title = usuario ? usuario.login + ' · ' + rotuloPapel(papel) : '';

  const menu = $('#menuUsuarioItens', topbar);
  $('#btPerfil', topbar).classList.toggle('oculto', papel === 'admin'); // admin não tem perfil (R3)
  $('#quem', topbar).addEventListener('click', ev => { ev.stopPropagation(); menu.classList.toggle('oculto'); });
  $('#btPerfil', topbar).addEventListener('click', () => { menu.classList.add('oculto'); irPara('#/perfil'); });
  $('#btSenha', topbar).addEventListener('click', () => { menu.classList.add('oculto'); modalSenha(); });
  $('#btnSair', topbar).addEventListener('click', async () => {
    menu.classList.add('oculto');
    try { await api('/api/logout', { method: 'POST', body: '{}' }); } catch (e) { /* sai mesmo assim */ }
    definirUsuario(null);
    irPara('#/login');
  });

  /* --- burger menu mobile (ordem Tenente 29/09): drawer lateral esquerdo --- */
  const burger = $('#btBurger', topbar), mask = $('#burgMask', topbar), navEl = $('#nav', topbar);
  const fecharDrawer = () => {
    navEl.classList.remove('aberta');
    burger.classList.remove('x');
    burger.setAttribute('aria-expanded', 'false');
    mask.classList.add('oculto');
  };
  burger.onclick = ev => {
    ev.stopPropagation();
    const abre = !navEl.classList.contains('aberta');
    if (abre) { // fora do #topbar: stacking context do topbar não limita mais o drawer
      document.body.appendChild(mask);
      document.body.appendChild(navEl);
    }
    navEl.classList.toggle('aberta', abre);
    burger.classList.toggle('x', abre);
    burger.setAttribute('aria-expanded', String(abre));
    mask.classList.toggle('oculto', !abre);
  };
  mask.onclick = fecharDrawer;
  navEl.querySelectorAll('a').forEach(a => a.addEventListener('click', fecharDrawer));
  montarShell.fecharDrawer = fecharDrawer;

  if (!montarShell._foraLigado) { // fecha o dropdown ao clicar fora (uma única vez)
    montarShell._foraLigado = true;
    document.addEventListener('click', ev => {
      const m = $('#menuUsuarioItens'), mu = $('#menuUsuario');
      if (m && mu && !m.classList.contains('oculto') && !mu.contains(ev.target)) m.classList.add('oculto');
    });
  }
  navAtiva(location.hash);
}
window.montarShell = montarShell;

function navAtiva(hash) {
  $$('#nav a').forEach(a => a.classList.toggle('ativo', a.getAttribute('href') === hash));
  const t = document.querySelector('#nav a.ativo');
  const tm = document.querySelector('#tituloMob');
  if (tm) tm.textContent = t ? t.textContent : '';
}
window.navAtiva = navAtiva;

/* ---------- login ---------- */
function viewLogin() {
  const tb = $('#topbar');
  if (tb) tb.classList.add('oculto');
  const app = garantirApp();
  app.innerHTML =
    '<div class="login-box">' +
      '<div class="marca"><span class="logo">SCI</span></div>' +
      '<div class="cartao">' +
        '<div class="campo"><label>Usuário</label><input id="lg" autocomplete="username"></div>' +
        '<div class="campo"><label>Senha</label><input id="sn" type="password" autocomplete="current-password"></div>' +
        '<button type="button" class="primario" id="btEntrar">Entrar</button>' +
      '</div>' +
    '</div>';
  const entrar = async () => {
    const login = $('#lg').value.trim(), senha = $('#sn').value;
    if (!login || !senha) { toast('Informe usuário e senha', 'erro'); return; }
    try {
      const r = await api('/api/login', { method: 'POST', body: JSON.stringify({ login: login, senha: senha }) });
      definirUsuario(r.usuario);
      montarShell(r.usuario);
      toast('Bem-vindo, ' + r.usuario.login);
      irPara(rotaInicial());
    } catch (e) { /* toast já exibido pelo api() */ }
  };
  $('#btEntrar').addEventListener('click', entrar);
  $('#lg').addEventListener('keydown', ev => { if (ev.key === 'Enter') entrar(); });
  $('#sn').addEventListener('keydown', ev => { if (ev.key === 'Enter') entrar(); });
}
window.viewLogin = viewLogin;

/* ---------- invocação de view tolerante a módulo ausente ---------- */
function chamarView(nome) {
  const app = garantirApp();
  const fn = window[nome];
  if (typeof fn !== 'function') {
    app.innerHTML = '<div class="carregando">módulo ausente: ' + esc(nome) + '</div>';
    return;
  }
  try {
    const r = fn();
    if (r && typeof r.catch === 'function') {
      r.catch(err => {
        console.error('[SCI]', nome, err);
        if (!err || err.status !== 401) toast('Falha ao carregar a tela', 'erro');
      });
    }
  } catch (err) {
    console.error('[SCI]', nome, err);
    app.innerHTML = '<div class="carregando">erro ao carregar ' + esc(nome) + '</div>';
  }
}

/* ---------- router ---------- */
function rotear() {
  garantirApp();
  const h = location.hash || '';
  if (!ME) { viewLogin(); return; }
  montarShell(ME);
  const papel = ME.papel;

  if (h === '' || h === '#' || h === '#/' || h === '#/login') { irPara(rotaInicial()); return; }
  if (h === '#/conferencias') { irPara('#/hoje'); return; }             // histórico mora na Conferência
  if (papel === 'admin' && h === '#/hoje') { irPara('#/dashboard'); return; } // admin não vê conferência
  if (papel !== 'admin' && h === '#/dashboard') { irPara(rotaInicial()); return; }

  if (h === '#/hoje') { chamarView('ViewHoje'); return; }
  if (h === '#/conferencia') { chamarView('ViewConferencia'); return; } // edição da conf aberta (v9.14)
  if (h === '#/relatorios') { chamarView('ViewRelatorios'); return; }
  if (h === '#/dashboard') { chamarView('ViewDashboard'); return; }
  if (h === '#/admin') {
    if (papel === 'admin') chamarView('ViewAdmin');
    else irPara(rotaInicial());
    return;
  }
  if (h === '#/grupos') {
    if (papel === 'gerente') chamarView('ViewGrupos');
    else irPara(rotaInicial());
    return;
  }
  if (h === '#/perfil') {
    if (papel !== 'admin') chamarView('ViewPerfil');
    else irPara(rotaInicial());
    return;
  }
  irPara(rotaInicial());
}
window.rotear = rotear;

/* ---------- dashboard do admin (vive no core; ViewDashboard é substituível) ---------- */
function renderRelatorio(b, titulo) {
  const pessoas = (b.pessoas || []).slice()
    .sort((a, c) => String(a.nome_guerra || '').localeCompare(String(c.nome_guerra || ''), 'pt-BR'));
  const resumo =
    '<div class="resumo">' +
      '<div class="caixa"><b>' + b.convocacoes + '</b><span>conferências</span></div>' +
      '<div class="caixa"><b>' + b.efetivo_ativo + '</b><span>efetivo</span></div>' +
      '<div class="caixa"><b>' + b.presentes + '</b><span>presentes</span></div>' +
      '<div class="caixa"><b>' + b.atrasos + '</b><span>atrasos</span></div>' +
      '<div class="caixa"><b>' + b.falta + '</b><span>faltas</span></div>' +
      '<div class="caixa"><b>' + b.justificadas + '</b><span>justificadas</span></div>' +
      '<div class="caixa"><b>' + (b.total_faltas !== undefined && b.total_faltas !== null ? b.total_faltas : ((b.falta || 0) + (b.justificadas || 0))) + '</b><span>faltas tot.</span></div>' +
      '<div class="caixa"><b>' + b.pct_geral + '%</b><span>% válidas (P+A)</span></div>' +
      '<div class="caixa"><b>' + b.pct_pronto + '%</b><span>ef. pronto</span></div>' +
    '</div>';
  const linhasConf = (b.formaturas || []).map(f =>
    '<tr><td>' + esc(fmtData(f.data)) + '</td><td>' + esc(f.tipo) + '</td><td>' + esc(f.hora || '—') + '</td>' +
    '<td>' + pill(f.status) + '</td><td class="num">' + f.presentes + '</td><td class="num">' + f.faltas + '</td></tr>'
  ).join('');
  const linhasPessoas = pessoas.map(p =>
    '<tr><td class="num">' + (p.antiguidade !== undefined && p.antiguidade !== null ? p.antiguidade : '') + '</td>' +
    '<td><b>' + esc(p.nome_guerra) + '</b></td>' +
    '<td>' + esc(p.funcao || '—') + '</td><td>' + esc(p.setor || '—') + '</td><td>' + esc(p.grupo || '—') + '</td>' +
    '<td class="num">' + p.presencas + '</td><td class="num">' + p.atrasos + '</td>' +
    '<td class="num">' + p.faltas + '</td><td class="num">' + p.justificadas + '</td></tr>'
  ).join('');
  const pdf = '/api/relatorio.pdf?de=' + encodeURIComponent(b.De || '') + '&ate=' + encodeURIComponent(b.Ate || '');
  return resumo +
    '<div class="cartao">' +
      '<div class="cartao-topo"><h3>' + esc(titulo) + ' — ' + b.convocacoes + ' conferência(s) no período</h3>' +
      '<a href="' + pdf + '" target="_blank" rel="noopener"><button type="button" class="primario">ABRIR PDF</button></a></div>' +
      (linhasConf
        ? '<div class="rolagem" style="margin-bottom:12px"><table>' +
          '<thead><tr><th>Data</th><th>Tipo</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead>' +
          '<tbody>' + linhasConf + '</tbody></table></div>'
        : '') +
      '<div class="rolagem"><table>' +
        '<thead><tr><th class="num">Antig.</th><th>Nome</th><th>Função</th><th>Setor</th><th>Grupo</th>' +
        '<th class="num">Pres.</th><th class="num">Atraso</th><th class="num">Falta</th><th class="num">Just.</th></tr></thead>' +
        '<tbody>' + (linhasPessoas || '<tr><td colspan="9"><span class="vazio">sem efetivo no período</span></td></tr>') + '</tbody>' +
      '</table></div>' +
    '</div>';
}
window.renderRelatorio = renderRelatorio;

async function viewDashboard() {
  navAtiva('#/dashboard');
  const app = garantirApp();
  app.innerHTML = '<div class="carregando">Carregando painel…</div>';
  const hoje = new Date();
  const periodo = semanaDe(dataLocal(hoje));
  const b = await api('/api/relatorio?de=' + periodo[0] + '&ate=' + dataLocal(hoje));
  b.De = periodo[0];
  b.Ate = dataLocal(hoje);
  app.innerHTML = '<h2>Dashboard</h2>' +
    '<p style="color:var(--tx2);font-size:13px;margin-bottom:10px">Panorama da semana em curso — leitura.</p>' +
    renderRelatorio(b, 'Semana em curso');
}
window.ViewDashboard = viewDashboard;

/* ---------- boot ---------- */
window.SCI_BOOT = function () {
  window.onhashchange = rotear;
  (async () => {
    try {
      const r = await api('/api/me');
      definirUsuario(r && r.usuario);
    } catch (e) { return; } // 401 → api() já abriu o login; falha de rede → toast exibido
    if (!location.hash || location.hash === '#' || location.hash === '#/') {
      location.hash = rotaInicial(); // dispara hashchange → rotear
    } else {
      rotear();
    }
  })();
};

})();
