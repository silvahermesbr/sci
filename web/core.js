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
  // v9.15.3: se vier ISO com hora (RFC3339), converter para o dia de BRASÍLIA
  if (String(s).length > 10 && String(s).includes('T')) {
    const d = new Date(s);
    if (!isNaN(d)) {
      const f = new Intl.DateTimeFormat('pt-BR', { timeZone: 'America/Sao_Paulo', day: '2-digit', month: '2-digit', year: 'numeric' }).format(d);
      return f;
    }
  }
  const m = String(s).slice(0, 10).match(/^(\d{4})-(\d{2})-(\d{2})$/);
  return m ? m[3] + '/' + m[2] + '/' + m[1] : String(s);
}
window.fmtData = fmtData;

function fmtHora(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  // v9.15.3 (ordem Tenente): horário de BRASÍLIA obrigatório, independente do fuso do dispositivo
  return isNaN(d) ? '—' : d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', timeZone: 'America/Sao_Paulo' });
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

function modal(html) {
  const mask = document.createElement('div');
  mask.className = 'modal-mask';
  if (typeof html === 'string' && html.trim().startsWith('<div class="modal')) {
    mask.innerHTML = html;
  } else {
    const m = document.createElement('div');
    m.className = 'modal';
    m.innerHTML = html;
    mask.appendChild(m);
  }
  document.body.appendChild(mask);

  function fechar(ev) {
    if (ev && ev.key === 'Escape') {
      document.removeEventListener('keydown', fechar, true);
      mask.remove();
    }
  }
  document.addEventListener('keydown', fechar, true);
  mask.addEventListener('click', ev => {
    if (ev.target === mask) {
      document.removeEventListener('keydown', fechar, true);
      mask.remove();
    }
  });

  mask.fechar = () => {
    document.removeEventListener('keydown', fechar, true);
    mask.remove();
  };

  const inp = mask.querySelector('input, select, textarea');
  if (inp) inp.focus();

  return mask;
}
window.modal = modal;

function alerta(msg) {
  toast(msg, 'erro');
}
window.alerta = alerta;

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
  
  const papel = usuario && usuario.papel;
  topbar.innerHTML =
    '<button type="button" id="btBurger" aria-label="menu" aria-expanded="false">' +
      '<span class="burg-x"></span><span class="burg-x"></span><span class="burg-x"></span>' +
    '</button>' +
    '<div class="marca"><span class="logo">' +
      '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 2.5l8 3.2v5.6c0 5-3.4 8.6-8 10.2-4.6-1.6-8-5.2-8-10.2V5.7l8-3.2z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M8.4 12.2l2.5 2.5 4.7-5.2" stroke="currentColor" stroke-width="1.8" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>' +
      ' SCI' +
    '</span><span class="sub">Controle Interno</span></div>' +
    '<div id="tituloMob"></div>' +
    '<nav id="nav"></nav>' +
    '<div id="burgMask" class="oculto"></div>' +
    '<div class="sessao">' +
      '<div id="hubNotificacoes">' +
        '<button id="btSinoNotif" type="button" title="Consciência Situacional & Alertas" class="btn-topbar-sino">' +
          '<svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"></path><path d="M13.73 21a2 2 0 0 1-3.46 0"></path></svg>' +
          '<span id="badgeNotif" class="badge-notif oculto">0</span>' +
        '</button>' +
      '</div>' +
      '<div id="menuUsuario" class="menu-usuario-wrapper">' +
        '<button type="button" id="quem" class="btn-usuario-gatilho" title="Minha Conta">' +
          '<div class="user-avatar-ico">' +
            '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle cx="12" cy="8" r="4" stroke="currentColor" stroke-width="2"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>' +
          '</div>' +
          '<span class="user-login-txt">' + esc(usuario ? usuario.login : '') + '</span>' +
          '<span class="user-papel-tag ' + esc(papel || '') + '">' + rotuloPapel(papel) + '</span>' +
          '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="user-seta"><path d="M6 9l6 6 6-6"/></svg>' +
        '</button>' +
        '<div id="menuUsuarioItens" class="menu-dropdown-custom oculto">' +
          '<div class="menu-dropdown-header">' +
            '<div class="menu-dropdown-user">' + esc(usuario ? (usuario.nome_guerra || usuario.login) : '') + '</div>' +
            '<div class="menu-dropdown-sub">' + esc(usuario ? usuario.login : '') + ' · ' + rotuloPapel(papel) + '</div>' +
          '</div>' +
          '<div class="menu-dropdown-div"></div>' +
          '<button type="button" id="btPerfil" class="menu-dropdown-item' + (papel === 'admin' ? ' oculto' : '') + '">' +
            '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>' +
            '<span>Meu Perfil</span>' +
          '</button>' +
          '<button type="button" id="btSenha" class="menu-dropdown-item">' +
            '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>' +
            '<span>Mudar Senha</span>' +
          '</button>' +
          '<div class="menu-dropdown-div"></div>' +
          '<button type="button" id="btnSair" class="menu-dropdown-item item-sair">' +
            '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>' +
            '<span>Encerrar Sessão</span>' +
          '</button>' +
        '</div>' +
      '</div>' +
    '</div>';

  let itens;
  if (papel === 'admin') {
    itens = [
      ['#/admin', 'ADMIN'],
      ['#/relatorios', 'RELATÓRIOS'],
      ['#/configuracoes', 'CONFIGURAÇÕES']
    ];
  } else {
    itens = [
      ['#/hoje', 'CONFERÊNCIA'],
      ['#/relatorios', 'RELATÓRIOS']
    ];
    if (papel === 'gerente') {
      itens.push(['#/grupos', 'GERENCIAR']);
    }
  }
  $('#nav', topbar).innerHTML = itens.map(it => '<a href="' + it[0] + '">' + it[1] + '</a>').join('');

  const menu = $('#menuUsuarioItens', topbar);
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

  /* --- Hub de Notificações & Consciência Situacional --- */
  const btSino = $('#btSinoNotif', topbar);
  if (btSino) {
    btSino.onclick = ev => {
      ev.stopPropagation();
      modalNotificacoes();
    };
    checarNotificacoesHub();
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
  const nomeSys = window.cfg ? window.cfg('NOME_SISTEMA', 'SCI') : 'SCI';
  const subSys = window.cfg ? window.cfg('SUBTITULO_SISTEMA', 'Controle Interno') : 'Controle Interno';
  const orgTitulo = window.cfg ? window.cfg('TITULO_ORGANIZACAO', '') : '';

  app.innerHTML = `
    <div class="login-wrapper">
      <div class="login-box">
        <div class="login-header">
          <div class="login-logo">
            <svg width="34" height="34" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M12 2.5l8 3.2v5.6c0 5-3.4 8.6-8 10.2-4.6-1.6-8-5.2-8-10.2V5.7l8-3.2z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/>
              <path d="M8.4 12.2l2.5 2.5 4.7-5.2" stroke="currentColor" stroke-width="1.8" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
            <span class="login-nome">${esc(nomeSys)}</span>
          </div>
          <p class="login-sub">${esc(subSys)}${orgTitulo ? ' · ' + esc(orgTitulo) : ''}</p>
        </div>

        <div class="cartao login-cartao">
          <div class="login-cartao-topo">
            <h3 class="login-cartao-titulo">Acesso ao Sistema</h3>
          </div>

          <form id="formLogin" onsubmit="return false;" class="login-form">
            <div class="campo">
              <label for="lg">Login</label>
              <input id="lg" autocomplete="username" placeholder="Digite seu login…" autofocus required>
            </div>
            <div class="campo">
              <label for="sn">Senha</label>
              <input id="sn" type="password" autocomplete="current-password" placeholder="••••••••" required>
            </div>
            <button type="submit" class="primario bt-login" id="btEntrar">
              <span>Entrar</span>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
            </button>
          </form>
        </div>

        <div class="login-rodape">
          <span>Sistema de Controle Interno · Uso Restrito</span>
        </div>
      </div>
    </div>
  `;

  const entrar = async () => {
    const login = $('#lg').value.trim(), senha = $('#sn').value;
    if (!login || !senha) { toast('Informe usuário e senha', 'erro'); return; }
    const btn = $('#btEntrar');
    if (btn) btn.disabled = true;
    try {
      const r = await api('/api/login', { method: 'POST', body: JSON.stringify({ login: login, senha: senha }) });
      definirUsuario(r.usuario);
      montarShell(r.usuario);
      toast('Bem-vindo, ' + r.usuario.login);
      irPara(rotaInicial());
    } catch (e) {
      if (btn) btn.disabled = false;
      const snInp = $('#sn');
      if (snInp) { snInp.value = ''; snInp.focus(); }
    }
  };

  const form = $('#formLogin');
  if (form) form.addEventListener('submit', entrar);
  $('#btEntrar').addEventListener('click', entrar);
}
window.viewLogin = viewLogin;

function animarEntradaView() {
  const app = $('#app');
  if (!app) return;
  app.classList.remove('view-blur-enter');
  void app.offsetWidth; // trigger reflow for smooth restart
  app.classList.add('view-blur-enter');
}
window.animarEntradaView = animarEntradaView;

/* ---------- invocação de view tolerante a módulo ausente ---------- */
function chamarView(nome) {
  const app = garantirApp();
  animarEntradaView();
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


/* ---------- efetivo: estado ATUAL (v9.15) ----------
   Última conferência de cada militar = estado atual. Frescor:
   hoje = ok · >1 dia = amarelo · >1 semana = vermelho · nunca conferido = vermelho. */
window.EfetivoAtualHTML = async function () {
  let d;
  try { d = await api('/api/efetivo_atual'); } catch (e) { return ''; }
  const hoje = d.hoje;
  const dias = s => Math.floor((new Date(hoje + 'T12:00:00') - new Date(s + 'T12:00:00')) / 864e5);
  const frescor = p => {
    if (!p.ultima_data) return 'verm';
    const dd = dias(p.ultima_data);
    if (dd >= 7) return 'verm';
    if (dd >= 1) return 'ama';
    return 'ok';
  };
  const ps = d.pessoas || [];
  // v9.16.2 (ordem Tenente): lista de nomes NÃO aparece na tela — só o necessário p/ filtrar
  // e gerar relatório; o detalhe por militar está no PDF.
  let nOk = 0, nAma = 0, nVerm = 0;
  const porSetor = {};
  ps.forEach(p => {
    const f = frescor(p);
    if (f === 'verm') nVerm++; else if (f === 'ama') nAma++; else nOk++;
    const s = porSetor[p.setor || 'Sem setor'] = porSetor[p.setor || 'Sem setor'] || { total: 0, ok: 0, ama: 0, verm: 0 };
    s.total++; s[f]++;
  });
  const linhas = Object.keys(porSetor).sort().map(s => {
    const q = porSetor[s];
    return `<tr><td>${esc(s)}</td><td class="num">${q.total}</td><td class="num">${q.ok}</td>` +
      `<td class="num">${q.ama ? '<span class="alerta-ama">' + q.ama + '</span>' : '0'}</td>` +
      `<td class="num">${q.verm ? '<span class="alerta-verm">' + q.verm + '</span>' : '0'}</td></tr>`;
  }).join('');
  return `<div class="cartao" style="margin-bottom:14px">
    <h3 style="margin:0 0 4px">EFETIVO — ESTADO ATUAL <small style="color:var(--tx3);font-weight:400">(estado da última conferência de cada militar · detalhes no PDF)</small></h3>
    <p style="color:var(--tx2);font-size:12px;margin:0 0 8px">${ps.length} militares · <span style="color:var(--verde-claro)">● ${nOk} conferidos hoje</span> · <span style="color:var(--ambar-txt)">● ${nAma} há 1+ dia</span> · <span style="color:var(--verm-txt)">● ${nVerm} há 1+ semana ou nunca</span></p>
    <div class="rolagem"><table><thead><tr><th>Setor</th><th class="num">Total</th><th class="num">● Hoje</th><th class="num">● 1+ dia</th><th class="num">● 1+ semana/nunca</th></tr></thead>
    <tbody>${linhas}</tbody></table></div></div>`;
};

/* ---------- router ---------- */
function rotear() {
  garantirApp();
  // v9.14.2: hash pode carregar query (#/conferencia?id=7) — rotear pela BASE (antes de '?')
  const hBruto = location.hash || '';
  const h = hBruto.split('?')[0] || '';
  if (!ME) { viewLogin(); return; }
  montarShell(ME);
  const papel = ME.papel;

  if (h === '' || h === '#' || h === '#/' || h === '#/login') { irPara(rotaInicial()); return; }
  if (h === '#/conferencias') { irPara('#/hoje'); return; }             // listas moram na Conferência
  if (papel === 'admin' && (h === '#/hoje' || h === '#/conferencia')) { irPara('#/admin'); return; } // admin não tem grupo: restrito a gerentes/operadores
  // Módulos ESCALAS e MATERIAL EM RESERVA (ordem Tenente 30/09): fora do ar no front —
  // código preservado no repo; retorno pelo flag MODO_RESERVA=0 nas configurações
  if (h === '#/escalas' || h === '#/material') { irPara(rotaInicial()); return; }

  if (h === '#/hoje') { chamarView('ViewHoje'); return; }
  if (h === '#/conferencia') { chamarView('ViewConferencia'); return; } // edição da conf aberta (v9.14)
  if (h === '#/relatorios') { chamarView('ViewRelatorios'); return; }
  if (h === '#/admin') {
    if (papel === 'admin') chamarView('ViewAdmin');
    else irPara(rotaInicial());
    return;
  }
  if (h === '#/configuracoes') {
    if (papel === 'admin') chamarView('ViewConfiguracoes');
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



/* ---------- v9.16.1: "Processando…" — aguarde global p/ toda ação de escrita ----------
   processar(tarefa, rotulo) → executa tarefa(); mostra overlay c/ disco girando ao iniciar,
   troca para ✅ Pronto! (verde) ou ❌ (vermelho X) ao terminar e fecha sozinho. */
function processar(tarefa, rotulo) {
  return new Promise(resolve => {
    const mask = document.createElement('div');
    mask.className = 'proc-mask';
    mask.innerHTML = '<div class="proc-box">' +
      '<div class="proc-disco"></div>' +
      '<div class="proc-msg">' + esc(rotulo || 'Processando…') + '</div></div>';
    document.body.appendChild(mask);
    let resultado = null, erro = null;
    Promise.resolve()
      .then(() => tarefa())
      .then(r => { resultado = r; })
      .catch(e => { erro = e; })
      .finally(() => {
        const box = mask.querySelector('.proc-box');
        box.innerHTML = erro
          ? '<div class="proc-emoji proc-erro">❌</div><div class="proc-msg">Erro: ' + esc((erro && erro.message) || 'falha na operação') + '</div>'
          : '<div class="proc-emoji proc-ok">✅</div><div class="proc-msg">Pronto!</div>';
        setTimeout(() => {
          mask.remove();
          if (erro) { toast((erro && erro.message) || 'Falha na operação', 'erro'); }
          resolve({ ok: !erro, resultado });
        }, erro ? 2200 : 900);
      });
  });
}
window.processar = processar;

window.CONFIGS = {};
window.cfg = function (k, def) { return (window.CONFIGS && window.CONFIGS[k]) || def; };

function aplicarConfiguracoes(c) {
  if (!c) return;
  window.CONFIGS = c;
  const root = document.documentElement;
  if (c.COR_PRIMARIA) root.style.setProperty('--verde', c.COR_PRIMARIA);
  if (c.COR_PRIMARIA_CLARO) root.style.setProperty('--verde-claro', c.COR_PRIMARIA_CLARO);
  if (c.COR_PRIMARIA_ESCURO) root.style.setProperty('--verdeesc', c.COR_PRIMARIA_ESCURO);
  if (c.NOME_SISTEMA) {
    document.title = c.NOME_SISTEMA + (c.TITULO_ORGANIZACAO ? ' — ' + c.TITULO_ORGANIZACAO : '');
    const logoEl = document.querySelector('.logo');
    if (logoEl) {
      const svg = logoEl.querySelector('svg');
      logoEl.innerHTML = (svg ? svg.outerHTML : '') + ' ' + esc(c.NOME_SISTEMA);
    }
  }
  if (c.SUBTITULO_SISTEMA) {
    const subEl = document.querySelector('.sub');
    if (subEl) subEl.textContent = c.SUBTITULO_SISTEMA;
  }
}
window.aplicarConfiguracoes = aplicarConfiguracoes;

/* ---------- boot ---------- */
window.SCI_BOOT = function () {
  window.onhashchange = rotear;
  (async () => {
    try {
      const cfgRes = await fetch('/api/configuracoes').then(r => r.json()).catch(() => null);
      if (cfgRes && cfgRes.configuracoes) {
        aplicarConfiguracoes(cfgRes.configuracoes);
      }
    } catch (e) {}

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

/* ---------- Notificações & Consciência Situacional ---------- */
async function checarNotificacoesHub() {
  if (!ME) return;
  try {
    const res = await api('/api/notificacoes');
    const badge = $('#badgeNotif');
    if (!badge) return;
    const n = (res && res.total_atrasadas) || 0;
    if (n > 0) {
      badge.textContent = n > 99 ? '99+' : n;
      badge.classList.remove('oculto');
    } else {
      badge.classList.add('oculto');
    }
  } catch (e) { /* silencioso */ }
}
window.checarNotificacoesHub = checarNotificacoesHub;

// Polling suave a cada 60s
setInterval(() => { if (ME) checarNotificacoesHub(); }, 60000);

function modalNotificacoes() {
  api('/api/notificacoes').then(res => {
    const atrasos = (res && res.cautelas_atrasadas) || [];
    const prazo = (res && res.prazo_horas) || 24;
    const html = `
      <div style="min-width:320px; max-width:540px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:12px">
          <h3 style="margin:0; display:flex; align-items:center; gap:8px">
            <span>🔔</span> <span>Consciência Situacional & Alertas</span>
          </h3>
          <span style="font-size:12px; color:var(--tx3)">SLA Padrão: ${prazo}h</span>
        </div>
        ${atrasos.length ? `
          <div style="color:var(--verm); font-size:13px; font-weight:600; margin-bottom:10px">
            ⚠️ Atenção: ${atrasos.length} cautela(s) excederam o prazo regulamentar de devolução!
          </div>
          <div style="display:flex; flex-direction:column; gap:8px; max-height:360px; overflow-y:auto; margin-bottom:16px">
            ${atrasos.map(a => `
              <div style="background:var(--painel2); border:1px solid ${a.nivel_sensibilidade === 'restrito' ? 'var(--verm)' : 'var(--ambar)'}; border-radius:var(--raio); padding:10px 12px">
                <div style="display:flex; justify-content:space-between; align-items:flex-start">
                  <div>
                    <b style="font-size:13.5px">${esc(a.item_nome)}</b>
                    <span style="font-size:11.5px; color:var(--tx3); margin-left:6px">#${esc(a.codigo_patrimonio)}</span>
                  </div>
                  <span style="font-size:10px; font-weight:700; text-transform:uppercase; padding:2px 6px; border-radius:4px; ${a.nivel_sensibilidade === 'restrito' ? 'background:rgba(239,68,68,0.2); color:var(--verm)' : 'background:rgba(245,158,11,0.2); color:var(--ambar-txt)'}">
                    ${esc(a.nivel_sensibilidade)}
                  </span>
                </div>
                <div style="font-size:12.5px; color:var(--tx2); margin-top:4px">
                  Responsável: <b style="color:var(--tx)">${esc(a.pessoa_nome_guerra)}</b> ·
                  <span style="color:var(--verm); font-weight:600">Fora há ${a.horas_em_uso}h</span>
                </div>
              </div>
            `).join('')}
          </div>
        ` : `
          <div class="vazio" style="padding:24px 0; text-align:center">
            <span style="font-size:28px">🛡️</span>
            <div style="margin-top:8px; font-weight:600">Nenhum alerta pendente</div>
            <p style="font-size:12.5px; color:var(--tx3); margin:4px 0 0">Todas as cautelas de material estão dentro do prazo de SLA configurado.</p>
          </div>
        `}
        <div style="display:flex; justify-content:flex-end; gap:8px">
          ${atrasos.length && ME && ME.papel !== 'admin' ? `
            <button class="primario" id="btIrMaterialNotif" style="font-size:13px; padding:6px 14px">Ir para Balcão de Material</button>
          ` : ''}
          <button class="acao-linha" id="btFecharNotif" style="font-size:13px; padding:6px 14px">Fechar</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    const btFechar = m.querySelector('#btFecharNotif');
    if (btFechar) btFechar.onclick = () => m.remove();
    const btIr = m.querySelector('#btIrMaterialNotif');
    if (btIr) {
      btIr.onclick = () => {
        m.remove();
        irPara('#/material');
      };
    }
  }).catch(() => toast('Falha ao obter notificações', 'erro'));
}
window.modalNotificacoes = modalNotificacoes;

})();
