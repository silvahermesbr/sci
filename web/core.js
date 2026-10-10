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
function fotoValida(s) {
  if (!s || typeof s !== 'string') return false;
  return /^data:image\/(?:png|jpeg|webp);base64,[A-Za-z0-9+/=\s]+$/.test(s.trim());
}
window.esc = esc;
window.fotoValida = fotoValida;
window.formatarTamanhoBytes = formatarTamanhoBytes;

/* ---------- usuário da sessão ---------- */
let ME = null;

function funcaoNomeContem(u, agulha) {
  if (!u || typeof u.funcao_nome !== 'string') return false;
  return u.funcao_nome.toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '').includes(agulha);
}

// v1.6.0 Fase 2 (poderes seguem o CONTEXTO ATIVO): os gestores derivam do
// PAPEL ATIVO da sessão (ME.papel — a linha materializada em usuario_papeis
// que a migração v45 criou para o legado e a designação sincroniza). O
// fallback heurístico por funcoes_grupo/nome fica APENAS para papel vazio
// (sessão legada pré-v45): com papel ativo, cargo NÃO acrescenta mais — quem
// tem os dois contextos troca no dropdown (POST /api/sessao/contexto).

// cadeiraLegadoDePapel: deriva o CONTEXTO ('enc_pessoal'/'enc_material') para
// sessão de papel vazio. Pela chave imutável quando há catálogo; com as duas
// cadeiras, enc_pessoal vence (conferência > material); sem catálogo (pré-key),
// o nome da função decide — 'material' antes do genérico 'encarregado'.
function cadeiraLegadoDePapel(u) {
  if (!u) return '';
  const ch = Array.isArray(u.funcoes_grupo) ? u.funcoes_grupo : null;
  const temChave = k => !!ch && ch.some(f => f && f.chave === k);
  if (temChave('enc_pessoal')) return 'enc_pessoal';
  if (temChave('enc_material')) return 'enc_material';
  if (ch) return ''; // com catálogo de cadeiras, chave ausente = não é enc
  if (funcaoNomeContem(u, 'material')) return 'enc_material';
  if (funcaoNomeContem(u, 'encarregado') || funcaoNomeContem(u, 'pessoal') || funcaoNomeContem(u, 'auxiliar')) return 'enc_pessoal';
  return '';
}

function ehEncPessoalUsuario(u) {
  if (!u) return false;
  if (u.papel === 'enc_pessoal') return true;
  if (u.papel) return false; // contexto ativo manda — cargo não acrescenta
  return cadeiraLegadoDePapel(u) === 'enc_pessoal';
}

function ehEncMaterialUsuario(u) {
  if (!u) return false;
  if (u.papel === 'enc_material') return true;
  if (u.papel) return false; // contexto ativo manda — cargo não acrescenta
  return cadeiraLegadoDePapel(u) === 'enc_material';
}

// v367: papel de sistema NÃO é mais apagado pelo cargo. v1.6.0 Fase 2: a
// derivação legada de papel vazio NÃO produz mais o genérico 'encarregado' —
// produz o CONTEXTO da cadeira ('enc_pessoal' vence quando as duas existem),
// que tem ramos próprios na sidebar (montarShell) e nos portões do rotear.
function definirUsuario(u) {
  if (u && !u.papel) {
    const legado = cadeiraLegadoDePapel(u);
    if (legado) u.papel = legado;
  }
  ME = u; window.SCI_ME = u; window.ME = u;
  window.ehEncPessoal = () => ehEncPessoalUsuario(ME);
  window.ehEncMaterial = () => ehEncMaterialUsuario(ME);
  window.gestorPessoal = () => !!(ME && (ME.papel === 'gerente' || (window.ehEncPessoal && window.ehEncPessoal())));
  window.gestorMaterial = () => !!(ME && (ME.papel === 'gerente' || (window.ehEncMaterial && window.ehEncMaterial())));
  // v1.6.0 Fase 2: ehEncarregado NÃO mistura mais enc_material — é só o
  // contexto 'enc_pessoal' (o string legado 'encarregado' ainda passa para
  // sessões antigas em memória). O módulo do material é do contexto enc_material.
  window.ehEncarregado = () => !!(ME && (ME.papel === 'encarregado' || ME.papel === 'enc_pessoal'));
}
window.definirUsuario = definirUsuario;

function rotuloPapel(p) {
  if (p === 'admin') return 'ADMIN';
  if (p === 'gerente') return 'GERENTE';
  if (p === 'chefe_setor') return 'CHEFE DE SETOR';
  if (p === 'encarregado') return 'ENCARREGADO';
  // v1.6.0 F1: cadeiras enc_* materializam linha em usuario_papeis — o dropdown
  // de contexto passa a exibi-las como papel (rótulo próprio, não mais OPERADOR).
  if (p === 'enc_pessoal') return 'ENCARREGADO DE PESSOAL';
  if (p === 'enc_material') return 'ENCARREGADO DE MATERIAL';
  if (p === 'sem_funcao') return 'SEM FUNÇÃO';
  return 'OPERADOR';
}
window.rotuloPapel = rotuloPapel;

function rotaInicial() {
  const p = ME && ME.papel;
  if (p === 'admin') return '#/admin';
  // ordem 04/10 — usuário normal (sem papel do sistema) não tem módulos:
  if (['gerente', 'operador', 'chefe_setor'].includes(p)) return '#/hoje';
  if (p === 'encarregado') return '#/hoje'; // onda 05/10: papel-conf derivado (função encarregado)
  // v1.6.0 F1: contexto enc_* materializado — cada cadeira aterrissa no seu módulo
  if (p === 'enc_pessoal') return '#/hoje';
  if (p === 'enc_material') return '#/material';
  // v1.6.0 Fase 5 — conta SEM FUNÇÃO (e papel vazio sem cadeira legada
  // derivável): aterrissa na página de bloqueio dedicada (não no genérico).
  if (p === 'sem_funcao' || !p) return '#/bloqueio';
  return '#/sem-modulo';
}

/* navega p/ hash; se já estiver nele, roteia direto (hashchange não dispara) */
function irPara(hash) {
  if (location.hash === hash) {
    if (typeof window.rotear === 'function') window.rotear();
    else rotear();
  } else {
    location.hash = hash;
  }
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
    if (data && data.req_setup && window.showSetupModal) {
      window.showSetupModal();
      const err = new Error('setup pendente'); err.status = 403; throw err;
    }
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
const ROTULO_SIT = { presente: 'Presente', atraso: 'Atraso', falta: 'Falta', justificada: 'Justificada', nao_verificado: 'NÃO VERIFICADO' };
function pill(situacao) {
  const s = String(situacao === null || situacao === undefined ? '' : situacao);
  const classe = s.toLowerCase().replace(/[^a-z0-9_-]/g, '');
  return '<span class="pill pill-' + classe + '">' + esc(ROTULO_SIT[classe] || s) + '</span>';
}
window.pill = pill;

/* ---------- Dropdown Estilizado Obsidian Glassmorphism (v1.5) ---------- */
function criarDropdown(container, opcoes, config = {}) {
  if (!container) return null;
  const valorInicial = config.valorPadrao !== undefined ? config.valorPadrao : (opcoes[0] ? opcoes[0].valor : null);
  let valorAtual = valorInicial;

  container.innerHTML = '';
  container.classList.add('sci-dropdown');

  const trigger = document.createElement('button');
  trigger.type = 'button';
  trigger.className = 'sci-dropdown-trigger ' + (config.classeExtra || '');

  const labelSpan = document.createElement('span');
  labelSpan.className = 'sci-dropdown-label';

  const chevron = document.createElement('span');
  chevron.className = 'sci-dropdown-chevron';
  chevron.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><polyline points="6 9 12 15 18 9"/></svg>';

  trigger.appendChild(labelSpan);
  trigger.appendChild(chevron);

  const menu = document.createElement('div');
  menu.className = 'sci-dropdown-menu ' + (config.alinharDireita ? 'alinhar-direita' : '');
  menu.style.display = 'none';

  const atualizarLabel = () => {
    const opt = opcoes.find(o => String(o.valor) === String(valorAtual)) || opcoes[0];
    if (opt) {
      labelSpan.innerHTML = (opt.icone ? `<span style="margin-right:6px">${opt.icone}</span>` : '') + esc(opt.rotulo || opt.nome || opt.valor);
    } else {
      labelSpan.textContent = config.placeholder || 'Selecione…';
    }
  };

  const renderItens = () => {
    menu.innerHTML = '';
    opcoes.forEach(opt => {
      const it = document.createElement('button');
      it.type = 'button';
      it.className = 'sci-dropdown-item' + (String(opt.valor) === String(valorAtual) ? ' ativo' : '');
      it.innerHTML = (opt.icone ? `<span style="font-size:14px">${opt.icone}</span>` : '') + `<span>${esc(opt.rotulo || opt.nome || opt.valor)}</span>`;
      it.onclick = (e) => {
        e.stopPropagation();
        valorAtual = opt.valor;
        atualizarLabel();
        fechar();
        if (typeof config.onChange === 'function') config.onChange(opt.valor, opt);
      };
      menu.appendChild(it);
    });
  };

  const abrir = () => {
    // Fechar outros dropdowns abertos
    document.querySelectorAll('.sci-dropdown-trigger.aberto').forEach(t => {
      if (t !== trigger) {
        t.classList.remove('aberto');
        const m = t.nextElementSibling;
        if (m) m.style.display = 'none';
      }
    });
    renderItens();
    menu.style.display = 'flex';
    trigger.classList.add('aberto');
  };

  const fechar = () => {
    menu.style.display = 'none';
    trigger.classList.remove('aberto');
  };

  trigger.onclick = (e) => {
    e.stopPropagation();
    if (trigger.classList.contains('aberto')) fechar();
    else abrir();
  };

  document.addEventListener('click', (e) => {
    if (!container.contains(e.target)) fechar();
  });

  container.appendChild(trigger);
  container.appendChild(menu);
  atualizarLabel();

  return {
    getValor: () => valorAtual,
    setValor: (v) => {
      valorAtual = v;
      atualizarLabel();
    },
    setOpcoes: (novasOpcoes) => {
      opcoes = novasOpcoes;
      atualizarLabel();
    }
  };
}
window.criarDropdown = criarDropdown;

/* ---------- Paginação Global (Máx 10 itens por página) ---------- */
function paginarArray(itens, pagina, porPagina) {
  const tamPag = porPagina || 10;
  const total = (itens || []).length;
  const totalPaginas = Math.max(1, Math.ceil(total / tamPag));
  const p = Math.max(1, Math.min(+pagina || 1, totalPaginas));
  const inicio = (p - 1) * tamPag;
  const fim = Math.min(inicio + tamPag, total);
  const dados = (itens || []).slice(inicio, fim);
  return {
    pagina: p,
    totalPaginas: totalPaginas,
    totalItens: total,
    inicio: total === 0 ? 0 : inicio + 1,
    fim: fim,
    dados: dados
  };
}
window.paginarArray = paginarArray;

function renderPaginadorHTML(info, idContainer) {
  if (!info || info.totalItens === 0) return '';
  const { pagina, totalPaginas, inicio, fim, totalItens } = info;
  if (totalPaginas <= 1) {
    return `<div class="paginacao-wrapper"><span class="paginacao-info">Total: <b>${totalItens}</b> registro(s)</span></div>`;
  }
  
  // Algoritmo com ellipsis inteligente (< ..., 2, 3, 4, ... >)
  const range = [];
  const delta = 1;
  for (let i = Math.max(2, pagina - delta); i <= Math.min(totalPaginas - 1, pagina + delta); i++) {
    range.push(i);
  }
  if (pagina - delta > 2) range.unshift('...');
  if (pagina + delta < totalPaginas - 1) range.push('...');
  range.unshift(1);
  if (totalPaginas > 1) range.push(totalPaginas);

  const btnsHtml = range.map(item => {
    if (item === '...') return `<span class="paginacao-ellipsis">…</span>`;
    const ativo = item === pagina ? 'ativo' : '';
    return `<button type="button" class="paginacao-btn ${ativo}" data-pg="${item}">${item}</button>`;
  }).join('');

  return `
    <div class="paginacao-wrapper" ${idContainer ? `data-pagcont="${idContainer}"` : ''}>
      <span class="paginacao-info">Exibindo <b>${inicio}–${fim}</b> de <b>${totalItens}</b> registros</span>
      <div class="paginacao-controles">
        <button type="button" class="paginacao-btn" data-pg="${pagina - 1}" ${pagina <= 1 ? 'disabled' : ''} title="Página anterior">‹</button>
        ${btnsHtml}
        <button type="button" class="paginacao-btn" data-pg="${pagina + 1}" ${pagina >= totalPaginas ? 'disabled' : ''} title="Próxima página">›</button>
      </div>
    </div>
  `;
}
window.renderPaginadorHTML = renderPaginadorHTML;

/* ---------- modal ---------- */
/* abrirModal(html [, aoFechar] [, opcoes]) → {fechar(), mask, modal}. Fecha em Esc,
   clique na máscara ou fechar(); aoFechar opcional dispara uma única vez em qualquer
   fechamento. opcoes.largura (ex. '850px') ajusta o max-width do modal — onda UX
   0510 item 1 (ordem Diretor): modais largos NÃO transbordam (.modal ganhou
   max-height:90vh + overflow-y:auto e a largura vira opção em vez de style inline). */
function abrirModal(html, aoFechar, opcoes) {
  const mask = document.createElement('div');
  mask.className = 'modal-mask';
  const modal = document.createElement('div');
  modal.className = 'modal';
  const largura = opcoes && opcoes.largura ? String(opcoes.largura) : '';
  if (largura) modal.style.maxWidth = largura;
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
function textoContextoUsuario(u) {
  if (!u) return 'Sem papel';
  const papel = rotuloPapel(u.papel);
  const funcao = (u.funcao_nome && u.funcao_nome.trim()) ? u.funcao_nome.trim() : papel;
  const grupo = (u.grupo_nome && u.grupo_nome.trim()) ? u.grupo_nome.trim() : (u.papel === 'admin' ? 'Global' : '');
  const setor = (u.setor_nome && u.setor_nome.trim()) ? u.setor_nome.trim() : '';

  let grupoSetor = '';
  if (grupo && setor) {
    grupoSetor = `${grupo}/${setor}`;
  } else if (grupo) {
    grupoSetor = grupo;
  } else if (setor) {
    grupoSetor = setor;
  } else {
    grupoSetor = (u.papel === 'admin' ? 'Global' : 'Sem grupo');
  }

  return `${grupoSetor} - ${funcao}`;
}

function montarShell(usuario) {
  if (typeof usuario === 'object' && usuario && usuario.setor_id === undefined) { /* login/setup: sem wrapper */ }
definirUsuario(usuario);
  
  // Garantir container estrutural de layout
  let layout = $('#layoutApp');
  if (!layout) {
    layout = document.createElement('div');
    layout.id = 'layoutApp';
    layout.className = 'layout-app';
    document.body.prepend(layout);
  }

  let sidebar = $('#sidebar');
  if (!sidebar) {
    sidebar = document.createElement('aside');
    sidebar.id = 'sidebar';
    sidebar.className = 'sidebar';
    layout.appendChild(sidebar);
  }
  sidebar.classList.remove('oculto');
  if (localStorage.getItem('SCI_SIDEBAR_RECOLHIDO') === '1') {
    sidebar.classList.add('recolhido');
  }

  let viewport = $('#viewport');
  if (!viewport) {
    viewport = document.createElement('div');
    viewport.id = 'viewport';
    viewport.className = 'viewport';
    layout.appendChild(viewport);
    const appEl = $('#app') || garantirApp();
    viewport.appendChild(appEl);
  }

  // Topbar legado é escondido
  const topbarAntiga = $('#topbar');
  if (topbarAntiga) topbarAntiga.classList.add('oculto');

  const papel = usuario && usuario.papel;
  const nomeSys = window.cfg ? window.cfg('NOME_SISTEMA', 'SCI') : 'SCI';
  const subSys = window.cfg ? window.cfg('SUBTITULO_SISTEMA', 'Controle Interno') : 'Controle Interno';
  const tituloOrg = window.cfg ? window.cfg('TITULO_ORGANIZACAO', '') : '';

  // Renderizar o conteúdo da Sidebar
  sidebar.innerHTML = `
    <!-- Topo da Sidebar: Cosméticos e Marca -->
    <div class="sidebar-header">
      <div class="sidebar-marca">
        <span class="sidebar-logo">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 2.5l8 3.2v5.6c0 5-3.4 8.6-8 10.2-4.6-1.6-8-5.2-8-10.2V5.7l8-3.2z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M8.4 12.2l2.5 2.5 4.7-5.2" stroke="currentColor" stroke-width="1.8" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
          <span class="sidebar-logo-txt">${esc(nomeSys)}</span>
        </span>
        <span class="sidebar-sub-txt">${esc(tituloOrg || subSys)}</span>
      </div>
      <button type="button" id="btToggleSidebar" class="btn-sidebar-toggle" title="Recolher / Expandir Menu">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 19l-7-7 7-7m8 14l-7-7 7-7"/></svg>
      </button>
    </div>

    <!-- Card de Usuário & Multi-Funções -->
    <div class="sidebar-usuario-card">
      <div class="sidebar-usuario-topo">
        <div class="sidebar-avatar" id="sbAvatarWrapper">
          ${usuario && usuario.foto_base64 && fotoValida(usuario.foto_base64) ? `<img src="${esc(usuario.foto_base64)}" class="sidebar-avatar-img" alt="Foto">` : `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle cx="12" cy="8" r="4" stroke="currentColor" stroke-width="2"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`}
        </div>
        <div class="sidebar-usuario-info">
          <div class="sidebar-usuario-nome">${esc(usuario ? (usuario.nome_guerra || usuario.nome_completo || '—') : '')}</div>
          <div class="sidebar-usuario-cargo">${esc(usuario && usuario.funcao_nome ? usuario.funcao_nome : rotuloPapel(papel))}</div>
        </div>
        <div class="sidebar-sino-wrapper">
          <button id="sbBtSinoNotif" type="button" title="Mensagens e Notificações" class="btn-sidebar-sino" aria-label="Mensagens e Notificações">
            <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"></path><path d="M13.73 21a2 2 0 0 1-3.46 0"></path></svg>
            <span id="sbBadgeNotif" class="badge-mini oculto">0</span>
            <span id="sbBadgeAvisos" class="badge-mini badge-avisos oculto">0</span>
          </button>
          <div class="sino-hover-card" id="sbSinoHoverCard">
            <div class="sino-hover-header">Resumo de Notificações</div>
            <div class="sino-hover-item" id="sinoHoverMsgItem">
              <span class="sino-hover-icon">📬</span>
              <span id="sinoHoverMsgTxt">0 mensagens não lidas</span>
            </div>
            <div class="sino-hover-item" id="sinoHoverAvisosItem">
              <span class="sino-hover-icon">📢</span>
              <span id="sinoHoverAvisosTxt">0 novos comunicados</span>
            </div>
            <div class="sino-hover-item" id="sinoHoverCautItem">
              <span class="sino-hover-icon">⏰</span>
              <span id="sinoHoverCautTxt">0 alertas de material</span>
            </div>
            <div class="sino-hover-footer">Clique para abrir Mensagens</div>
          </div>
        </div>
      </div>

      <!-- Dropdown Estilizado de Troca de Função / Contexto -->
      <div class="sidebar-contexto-wrapper" id="sbContextoWrapper">
        <button type="button" id="sbBtContexto" class="btn-contexto-gatilho" title="Clique para alternar sua função/grupo ativo">
          <div class="contexto-badge-ativo">
            <span class="contexto-dot"></span>
            <span id="sbTxtFuncaoAtiva" class="contexto-txt">${esc(textoContextoUsuario(usuario))}</span>
          </div>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="contexto-seta"><path d="M6 9l6 6 6-6"/></svg>
        </button>
        <div id="sbMenuContexto" class="menu-contexto-dropdown oculto"></div>
      </div>
    </div>

    <!-- Navegação Vertical Principal -->
    <nav id="sidebarNav" class="sidebar-nav"></nav>

    <!-- Rodapé da Sidebar (Ações e Logout) -->
    <div class="sidebar-rodape">
      <button type="button" id="sbBtPerfil" class="sidebar-btn-rodape" title="Meu Perfil">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
        <span class="sb-lbl">Meu Perfil</span>
      </button>
      <button type="button" id="sbBtSenha" class="sidebar-btn-rodape" title="Alterar Senha">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
        <span class="sb-lbl">Mudar Senha</span>
      </button>
      <button type="button" id="btnSair" class="sidebar-btn-rodape item-sair" title="Encerrar Sessão">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
        <span class="sb-lbl">Sair</span>
      </button>
    </div>
  `;

  // Popular itens de navegação na Sidebar com ícones modernos
  let itens;
  if (papel === 'admin') {
    // Correção Diretor 05/10: o admin TEM o dashboard de admin na sidebar.
    // Onda C2 (05/10): MURAL DE AVISOS no TOPO de todos os papéis — o admin
    // agora lê e participa dos murais de todos os grupos (visão global).
    const svgAvisosAdm = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>';
    itens = [
      ['#/avisos', 'MURAL DE AVISOS', svgAvisosAdm],
      ['#/admin', 'PAINEL ADMIN', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>'],
      ['#/perfil', 'MEU PERFIL', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>'],
      ['#/mensagens', 'EMAIL INTERNO', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>', true]
    ];
  } else {
    // ordem 04/10 — SISTEMAS DISPONÍVEIS POR PAPEL:
    //   Gerente: TODOS do seu grupo (conferência, email, avisos, drive, relatórios, gerenciar)
    //   Chefe de setor: Conferência, Drive, mural de avisos e Email Interno
    //   Operador: Conferência e mural de avisos
    //   Conta sem função do sistema: NADA (login leva ao aviso de módulo indisponível)
    const svgConf = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>';
    const svgMsg = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>';
    const svgAvisos = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>';
    const svgDrive = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
    const svgRel = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>';
    const svgGer = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>';
    const svgPes = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/><line x1="16" y1="3" x2="22" y2="9"/><line x1="22" y1="3" x2="16" y2="9"/></svg>';
    const svgMaterial = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>';
    const svgPerfil = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';
    // v1.6.0 Fase 5: conta SEM FUNÇÃO — sidebar MÍNIMA: sem itens de módulo
    // (perfil/senha/sair são fixos no rodapé; o dropdown de contexto mostra
    // "Nenhuma outra função disponível" com papeis[] vazio). O rotear leva
    // qualquer hash a #/bloqueio.
    if (papel === 'sem_funcao' || !papel) {
      itens = [];
    } else if (papel === 'operador') {
      // v1.6.0 Fase 2: o papel de sistema dá a BASE e pronto — o CARGO (cadeira
      // enc_*) virou CONTEXTO separado no dropdown; quem tem os dois contextos
      // troca e a sidebar remonta pelo papel ativo (sem acréscimo por cargo).
      itens = [
        ['#/avisos', 'MURAL DE AVISOS', svgAvisos],
        ['#/hoje', 'CONFERÊNCIA', svgConf]
      ];
    } else if (papel === 'chefe_setor') {
      // v1.6.0 Fase 2: mesma doutrina do operador — chefia sem acréscimo por cargo.
      itens = [
        ['#/avisos', 'MURAL DE AVISOS', svgAvisos], // onda C2: mural NO TOPO
        ['#/hoje', 'CONFERÊNCIA', svgConf],
        ['#/drive', 'DRIVE LOCAL', svgDrive],
        ['#/mensagens', 'EMAIL INTERNO', svgMsg, true]
      ];
    } else if (papel === 'enc_pessoal') {
      // v1.6.0 Fase 2: contexto PRÓPRIO do encarregado de pessoal (linha
      // materializada ativa) — CONFERÊNCIA (régua de gerente) + PESSOAL.
      // Encarregado NUNCA vê GERENCIAR GRUPO (#/grupos); material é do
      // contexto enc_material (troca no dropdown).
      itens = [
        ['#/avisos', 'MURAL DE AVISOS', svgAvisos], // onda C2: mural NO TOPO
        ['#/hoje', 'CONFERÊNCIA', svgConf],
        ['#/pessoal', 'PESSOAL', svgPes],
        ['#/relatorios', 'RELATÓRIOS', svgRel],
        ['#/perfil', 'MEU PERFIL', svgPerfil]
      ];
    } else if (papel === 'enc_material') {
      // v1.6.0 Fase 2: contexto PRÓPRIO do encarregado de material — SÓ o
      // módulo MATERIAL (sem conferência, nem pessoal; fase 6 endurece o servidor).
      itens = [
        ['#/avisos', 'MURAL DE AVISOS', svgAvisos],
        ['#/material', 'MATERIAL', svgMaterial],
        ['#/perfil', 'MEU PERFIL', svgPerfil]
      ];
    } else if (papel === 'gerente') {
      itens = [
        ['#/avisos', 'MURAL DE AVISOS', svgAvisos], // onda C2: mural NO TOPO
        ['#/hoje', 'CONFERÊNCIA', svgConf],
        ['#/mensagens', 'EMAIL INTERNO', svgMsg, true],
        ['#/drive', 'DRIVE LOCAL', svgDrive],
        ['#/pessoal', 'PESSOAL', svgPes],
        ['#/material', 'MATERIAL', svgMaterial],
        ['#/grupos', 'GERENCIAR GRUPO', svgGer],
        ['#/relatorios', 'RELATÓRIOS', svgRel]
      ];
    } else if (window.ehEncarregado && window.ehEncarregado()) {
      // LEGADO (v1.6.0 Fase 2): ramo composto só alcança sessão com o string
      // antigo 'encarregado' em ME.papel — a derivação nova converte papel
      // vazio nos contextos enc_pessoal/enc_material (ramos acima). Mantido
      // apenas para não engolir sessão legada em memória.
      // Encarregado NUNCA vê GERENCIAR GRUPO (#/grupos).
      // Correção Diretor: TODOS têm leitura do MURAL DE AVISOS (no topo).
      const mapa = new Map();
      const addItem = (rota, rotulo, svg, extra) => {
        if (!mapa.has(rota)) mapa.set(rota, [rota, rotulo, svg, extra]);
      };
      addItem('#/avisos', 'MURAL DE AVISOS', svgAvisos);
      if (window.ehEncPessoal && window.ehEncPessoal()) {
        addItem('#/hoje', 'CONFERÊNCIA', svgConf);
        addItem('#/pessoal', 'PESSOAL', svgPes);
      }
      if (window.ehEncMaterial && window.ehEncMaterial()) {
        addItem('#/material', 'MATERIAL', svgMaterial);
      }
      addItem('#/perfil', 'MEU PERFIL', svgPerfil);
      itens = Array.from(mapa.values());
    } else {
      // P0 onda 05/10 — conta SEM função do sistema: Meu Perfil + Mural de Avisos
      // (correção Diretor: leitura de avisos para TODOS).
      itens = [
        ['#/avisos', 'MURAL DE AVISOS', svgAvisos],
        ['#/perfil', 'MEU PERFIL', '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>']
      ];
    }
  }

  $('#sidebarNav', sidebar).innerHTML = itens.map(it => `
    <a href="${it[0]}">
      ${it[2]}
      <span class="nav-item-lbl">${it[1]}</span>
      ${it[3] ? '<span id="sbNavBadgeMsg" class="badge-mini oculto" style="margin-left:auto">0</span>' : ''}
    </a>
  `).join('');

  // Dropdown de Contexto (Multi-Funções)
  const menuCtx = $('#sbMenuContexto', sidebar);
  const btCtx = $('#sbBtContexto', sidebar);

  const renderDropdownPapeis = () => {
    const papeis = (usuario && usuario.papeis) || [];
    if (papeis.length === 0) {
      menuCtx.innerHTML = '<div style="padding:10px;font-size:12px;color:var(--tx3);text-align:center">Nenhuma outra função disponível</div>';
      return;
    }
    const html = papeis.map(p => {
      const mesmoPapel = usuario.papel_ativo_id === p.id;
      const mesmoSetor = p.setor_id ? (usuario.setor_id === p.setor_id) : true;
      const ehAtivo = mesmoPapel && mesmoSetor;

      const rot = rotuloPapel(p.papel);
      const funcao = (p.funcao_nome && p.funcao_nome.trim()) ? p.funcao_nome.trim() : rot;
      const grupo = (p.grupo_nome && p.grupo_nome.trim()) ? p.grupo_nome.trim() : (p.papel === 'admin' ? 'Global' : 'Sem grupo');
      const setor = (p.setor_nome && p.setor_nome.trim()) ? p.setor_nome.trim() : '';
      const grupoSetor = (grupo && setor) ? `${grupo}/${setor}` : (grupo || setor || 'Global');

      return `
        <div class="menu-contexto-item ${ehAtivo ? 'ativo' : ''}" data-papelid="${p.id}" data-setorid="${p.setor_id || ''}">
          <div style="min-width:0">
            <div style="font-weight:700">${esc(grupoSetor)}</div>
            <div style="font-size:11px;color:var(--tx3)">${esc(funcao)}</div>
          </div>
          ${ehAtivo ? '<span style="font-size:12px">✓</span>' : ''}
        </div>
      `;
    }).join('');
    menuCtx.innerHTML = html;

    menuCtx.querySelectorAll('.menu-contexto-item').forEach(item => {
      item.onclick = async (ev) => {
        ev.stopPropagation();
        const pId = +item.dataset.papelid;
        const sId = item.dataset.setorid ? +item.dataset.setorid : null;
        if (pId === usuario.papel_ativo_id && (!sId || sId === usuario.setor_id)) {
          menuCtx.classList.add('oculto');
          return;
        }
        menuCtx.classList.add('oculto');
        try {
          const res = await api('/api/sessao/contexto', {
            method: 'POST',
            body: JSON.stringify({ papel_id: pId, setor_id: sId })
          });
          if (res && res.usuario) {
            toast('Contexto alterado com sucesso!');
            if (res && res.setor_id !== undefined && res.usuario) res.usuario.setor_id = res.setor_id;
            if (res && res.setor_nome && res.usuario && !res.usuario.setor_nome) res.usuario.setor_nome = res.setor_nome;
            if (res && res.grupo_nome && res.usuario && !res.usuario.grupo_nome) res.usuario.grupo_nome = res.grupo_nome;
            definirUsuario(res.usuario);
            montarShell(res.usuario);
            irPara(rotaInicial());
          }
        } catch (e) {}
      };
    });
  };

  btCtx.onclick = (ev) => {
    ev.stopPropagation();
    renderDropdownPapeis();
    menuCtx.classList.toggle('oculto');
  };

  // Botão Recolher / Expandir Sidebar
  const btToggle = $('#btToggleSidebar', sidebar);
  const atualizarToggleEstado = () => {
    const estaRecolhido = sidebar.classList.contains('recolhido');
    if (btToggle) {
      btToggle.title = estaRecolhido ? 'Mostrar barra lateral (Expandir)' : 'Esconder barra lateral (Recolher)';
      btToggle.setAttribute('aria-label', btToggle.title);
    }
  };
  if (btToggle) {
    btToggle.onclick = () => {
      sidebar.classList.toggle('recolhido');
      const estaRecolhido = sidebar.classList.contains('recolhido');
      localStorage.setItem('SCI_SIDEBAR_RECOLHIDO', estaRecolhido ? '1' : '0');
      atualizarToggleEstado();
    };
    atualizarToggleEstado();
  }

  // Ações do rodapé
  $('#sbBtPerfil', sidebar).onclick = () => irPara('#/perfil');
  $('#sbBtSenha', sidebar).onclick = () => modalSenha();
  $('#btnSair', sidebar).onclick = async () => {
    try { await api('/api/logout', { method: 'POST', body: '{}' }); } catch (e) {}
    definirUsuario(null);
    irPara('#/login');
  };

  // Sino de notificações
  const btSino = $('#sbBtSinoNotif', sidebar);
  if (btSino) {
    btSino.onclick = (ev) => {
      ev.stopPropagation();
      irPara('#/mensagens');
    };
  }

  // Mobile menu toggle
  const btMob = $('#btMobileMenu');
  if (btMob) {
    btMob.onclick = (ev) => {
      ev.stopPropagation();
      // Item 13 (ordem 06/10): drawer reanexado ao document.body ao abrir —
      // dentro do <header>/<div> com transform/overflow o stacking context come
      // o z-index e o drawer nasce por baixo (pitfall já provado neste repo).
      // Guardamos o pai original para devolver a sidebar ao fechar (desktop intocado).
      if (!sidebar.__paiOriginal) sidebar.__paiOriginal = sidebar.parentNode;
      if (sidebar.parentNode !== document.body) document.body.appendChild(sidebar);
      sidebar.classList.toggle('aberto-mobile');
    };
  }
  const mobSino = $('#mobSinoNotif');
  if (mobSino) {
    mobSino.onclick = () => irPara('#/mensagens');
  }

  // Fechar menus ao clicar fora
  if (!montarShell._foraLigado) {
    montarShell._foraLigado = true;
    document.addEventListener('click', (ev) => {
      const m = $('#sbMenuContexto');
      const b = $('#sbBtContexto');
      if (m && !m.classList.contains('oculto') && b && !b.contains(ev.target) && !m.contains(ev.target)) {
        m.classList.add('oculto');
      }
      const sb = $('#sidebar');
      if (sb && sb.classList.contains('aberto-mobile') && !sb.contains(ev.target) && (!btMob || !btMob.contains(ev.target))) {
        // item 13: ao fechar, devolve a sidebar ao layout (veja toggle acima)
        // item 14: devolve na POSIÇÃO ORIGINAL (1º filho do flex) — appendChild
        // jogaria a sidebar para depois da viewport e ela apareceria à DIREITA.
        const layoutSidebar = document.getElementById('layoutApp');
        if (sb.__paiOriginal && sb.parentNode !== sb.__paiOriginal) sb.__paiOriginal.insertBefore(sb, sb.__paiOriginal.firstChild);
        else if (!sb.__paiOriginal && layoutSidebar && sb.parentNode !== layoutSidebar) layoutSidebar.insertBefore(sb, layoutSidebar.firstChild);
        sb.classList.remove('aberto-mobile');
      }
    });
  }

  // Atualizar badges
  if (window.atualizarBadgesMensagens) {
    window.atualizarBadgesMensagens();
  }

  navAtiva(location.hash);
  if (window.CONFIGS && Object.keys(window.CONFIGS).length > 0) {
    aplicarConfiguracoes(window.CONFIGS);
  }
}
window.montarShell = montarShell;

function navAtiva(hash) {
  $$('#sidebarNav a, #nav a').forEach(a => a.classList.toggle('ativo', a.getAttribute('href') === hash));
}
window.navAtiva = navAtiva;

/* ---------- login ---------- */
function viewLogin() {
  const sb = $('#sidebar');
  if (sb) sb.classList.add('oculto');
  const mobBar = $('#mobileBar');
  if (mobBar) mobBar.style.display = 'none';
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
      if (r && r.setor_id !== undefined && r.usuario) r.usuario.setor_id = r.setor_id; // onda 0510
      if (r && r.setor_nome && r.usuario && !r.usuario.setor_nome) r.usuario.setor_nome = r.setor_nome;
      if (r && r.grupo_nome && r.usuario && !r.usuario.grupo_nome) r.usuario.grupo_nome = r.grupo_nome;
      definirUsuario(r.usuario);
      if (r.usuario.precisa_setup && window.showSetupModal) {
        window.showSetupModal();
        if (btn) btn.disabled = false;
        return;
      }
      montarShell(r.usuario);
      toast('Bem-vindo, ' + (r.usuario.nome_guerra || r.usuario.login));
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
function chamarView(nome, ...args) {
  const app = garantirApp();
  animarEntradaView();
  const fn = window[nome];
  if (typeof fn !== 'function') {
    if (typeof window.garantirHash === 'function' && location.hash) {
      window.garantirHash(location.hash).then(() => {
        if (typeof window[nome] === 'function') {
          chamarView(nome, ...args);
        } else {
          app.innerHTML = '<div class="carregando">módulo ausente: ' + esc(nome) + '</div>';
        }
      });
      return;
    }
    app.innerHTML = '<div class="carregando">módulo ausente: ' + esc(nome) + '</div>';
    return;
  }
  try {
    const r = fn(...args);
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

  // v1.6.0 Fase 5 — conta SEM FUNÇÃO: página de bloqueio dedicada. Qualquer
  // OUTRA hash com papel 'sem_funcao' (ou vazio, sem cadeira legada) cai nela;
  // ViewSemModulo continua para papel VÁLIDO com módulo bloqueado.
  if (h === '#/bloqueio') { chamarView('ViewBloqueioSemFuncao'); return; }
  if (papel === 'sem_funcao' || !papel) { irPara('#/bloqueio'); return; }

  if (h === '' || h === '#' || h === '#/' || h === '#/login') { irPara(rotaInicial()); return; }
  if (h === '#/conferencias') { irPara('#/hoje'); return; }             // listas moram na Conferência
  if (papel === 'admin' && (h === '#/hoje' || h === '#/conferencia' || h === '#/calendario' || h === '#/drive' || h === '#/grupos')) {
    irPara('#/admin');
    return;
  } // admin não tem grupo: restrito em dados operacionais de grupo, mas possui caixa de mensagens própria
  // Modos de DESENVOLVIMENTO (ordem 04/10): escalas (1.8),
  // calendário (1.7) e consciência situacional (1.9) — acesso removido
  // across the board; código em acervo para as versões futuras.
  if (h === '#/escalas' || h === '#/calendario' || h === '#/consciencia') {
    chamarView('ViewModuloEmDesenvolvimento'); return;
  }
  // Usuário normal (ordem 04/10): login leva ao aviso de módulo não disponível
  if (h === '#/sem-modulo' || h === '#/perfil') { chamarView(h === '#/perfil' ? 'ViewPerfil' : 'ViewSemModulo'); return; }
  // ordem 04/10 — usuário sem função do sistema: NADA (login → aviso de módulo
  // indisponível; só Meu Perfil fica acessível).
  // P0 onda 05/10: 'admin' NÃO cai no portão de bloqueio — sem isso as rotas
  // #/admin e #/configuracoes (fim da função) ficavam inalcançáveis para o admin,
  // que era capturado aqui e levado a "Módulo não disponível".
  // v1.6.0 F1: o portão GERAL passa a aceitar os contextos enc_* materializados
  // (as portas ESPECÍFICAS de módulo vêm nas fases 2/6 — aqui é só não-engolir
  // o papel novo no bloqueio genérico de "sem módulo").
  if (papel === 'encarregado' || papel === 'enc_pessoal' || papel === 'enc_material') { /* onda 05/10: função de encarregado de pessoal passa no portão */ }
  else if (papel !== 'admin' && !['gerente', 'operador', 'chefe_setor'].includes(papel)) {
    chamarView('ViewSemModulo'); return;
  }
  // ordem 04/10 — portão por papel: operador não tem drive nem email interno;
  // chefe de setor não tem relatórios.
  if (h === '#/drive' && papel === 'operador') { chamarView('ViewSemModulo'); return; }
  if (h === '#/mensagens' && papel === 'operador') { chamarView('ViewSemModulo'); return; }
  // ordem 06/10 (P4) · v1.6.0 Fase 2: relatórios = gerente OU CONTEXTO
  // 'enc_pessoal' ativo (gestorPessoal agora deriva do papel ativo — o cargo
  // sozinho não abre mais; quem tem a cadeira troca no dropdown).
  if (h === '#/relatorios' && papel !== 'gerente' && !(window.gestorPessoal && window.gestorPessoal())) { chamarView('ViewSemModulo'); return; }
  // Onda 10/10 · v1.6.0 Fase 2 — portão do módulo Conferência (#/hoje e
  // #/conferencia): liberado para gerente, operador, chefe_setor e contexto
  // 'enc_pessoal' ATIVO; admin, contexto 'enc_material' e conta sem função
  // são barrados (fase 6 endurece o servidor na leitura).
  if (h === '#/hoje' || h === '#/conferencia') {
    const podeConf = ['gerente', 'operador', 'chefe_setor'].includes(papel) || papel === 'enc_pessoal';
    if (!podeConf) { chamarView('ViewSemModulo'); return; }
  }
  if (h === '#/hoje') { chamarView('ViewHoje'); return; }
  if (h === '#/mensagens') { chamarView('ViewMensagens'); return; }
  if (h === '#/despachos') { chamarView('ViewMensagens', 'despachos'); return; }
  if (h === '#/avisos') { chamarView('ViewAvisos'); return; }
  if (h === '#/conferencia') { chamarView('ViewConferencia'); return; } // edição da conf aberta (v9.14)
  if (h === '#/calendario') { chamarView('ViewCalendario'); return; }
  if (h === '#/drive') { chamarView('ViewDrive'); return; }
  if (h === '#/relatorios') { chamarView('ViewRelatorios'); return; }
  if (h === '#/material') {
    // v1.6.0 Fase 2: gerente OU CONTEXTO 'enc_material' ativo — espelha o
    // authMaterial do servidor (u.Papel == 'enc_material', linha materializada).
    // No contexto operador/chefe/enc_pessoal o mesmo usuário NÃO passa.
    if (papel === 'gerente' || papel === 'enc_material') chamarView('ViewMaterial');
    else chamarView('ViewSemModulo');
    return;
  }
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
  if (h === '#/pessoal') {
    // módulo Pessoal (f2) · v1.6.0 Fase 2: gerente OU CONTEXTO 'enc_pessoal'
    // ativo (podeGestaoPessoal do servidor agora é u.Papel == 'enc_pessoal');
    // servidor valida de novo.
    if (papel === 'gerente' || papel === 'enc_pessoal') chamarView('ViewPessoal');
    else irPara(rotaInicial());
    return;
  }
  if (h === '#/perfil') {
    chamarView('ViewPerfil');
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
        '<thead><tr><th class="num">Antig.</th><th>Nome</th><th>Antiguidade</th><th>Setor</th><th>Grupo</th>' +
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
  try { localStorage.setItem('SCI_CONFIGS', JSON.stringify(c)); } catch (e) {}
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
    const sbLogoTxt = document.querySelector('.sidebar-logo-txt');
    if (sbLogoTxt) sbLogoTxt.textContent = c.NOME_SISTEMA;
    const lgnLogoTxt = document.querySelector('.login-nome');
    if (lgnLogoTxt) lgnLogoTxt.textContent = c.NOME_SISTEMA;
  }
  const orgSub = c.TITULO_ORGANIZACAO || c.SUBTITULO_SISTEMA;
  if (orgSub) {
    const subEl = document.querySelector('.sub');
    if (subEl) subEl.textContent = orgSub;
    const sbSubTxt = document.querySelector('.sidebar-sub-txt');
    if (sbSubTxt) sbSubTxt.textContent = orgSub;
    const lgnSubTxt = document.querySelector('.login-sub');
    if (lgnSubTxt) lgnSubTxt.textContent = orgSub;
  }
}
window.aplicarConfiguracoes = aplicarConfiguracoes;

/* ---------- boot ---------- */
window.SCI_BOOT = function () {
  window.onhashchange = function () {
    if (typeof window.rotear === 'function') window.rotear();
    else rotear();
  };

  // Carregar imediatamente configs locais para evitar flash de cor padrão
  try {
    const cachedCfg = localStorage.getItem('SCI_CONFIGS');
    if (cachedCfg) aplicarConfiguracoes(JSON.parse(cachedCfg));
  } catch (e) {}

  (async () => {
    try {
      const cfgRes = await fetch('/api/configuracoes').then(r => r.json()).catch(() => null);
      if (cfgRes && cfgRes.configuracoes) {
        aplicarConfiguracoes(cfgRes.configuracoes);
      }
    } catch (e) {}

    try {
      const r = await api('/api/me');
      if (r && r.setor_id !== undefined && r.usuario) r.usuario.setor_id = r.setor_id;
      if (r && r.setor_nome && r.usuario && !r.usuario.setor_nome) r.usuario.setor_nome = r.setor_nome;
      if (r && r.grupo_nome && r.usuario && !r.usuario.grupo_nome) r.usuario.grupo_nome = r.grupo_nome;
      definirUsuario(r && r.usuario);
      if (r && r.usuario && r.usuario.precisa_setup && window.showSetupModal) {
        window.showSetupModal();
      }
    } catch (e) {
      definirUsuario(null);
      irPara('#/login');
      return;
    }
    if (!location.hash || location.hash === '#' || location.hash === '#/') {
      location.hash = rotaInicial(); // dispara hashchange → rotear
    } else {
      if (typeof window.rotear === 'function') window.rotear();
      else rotear();
    }
  })();
};

/* ---------- Notificações & Consciência Situacional ---------- */
async function checarNotificacoesHub() {
  if (!ME) return;
  try {
    const res = await api('/api/notificacoes');
    const n = (res && res.total_atrasadas) || 0;
    const txt = n === 1 ? '1 alerta de material' : `${n} alertas de material`;
    const el1 = $('#sinoHoverCautTxt');
    if (el1) el1.textContent = txt;
    const el2 = $('#mobSinoHoverCautTxt');
    if (el2) el2.textContent = txt;
    // AVISOS do grupo (ordem Diretor 07/10): badge azul do sino + linha do hover-card
    const av = (res && res.avisos_pendentes) || 0;
    const elAv1 = $('#sinoHoverAvisosTxt');
    if (elAv1) elAv1.textContent = av === 1 ? '1 novo comunicado' : `${av} novos comunicados`;
    const elAv2 = $('#mobSinoHoverAvisosTxt');
    if (elAv2) elAv2.textContent = av === 1 ? '1 novo comunicado' : `${av} novos comunicados`;
    ['#sbBadgeAvisos', '#mobBadgeAvisos'].forEach(sel => {
      const b = $(sel);
      if (!b) return;
      b.textContent = String(av);
      if (av > 0) b.classList.remove('oculto');
      else b.classList.add('oculto');
    });
  } catch (e) { /* silencioso */ }
}
window.checarNotificacoesHub = checarNotificacoesHub;

// Polling suave a cada 60s
setInterval(() => { if (ME) checarNotificacoesHub(); }, 60000);

/* ---------- Watchdog de Conectividade (Heartbeat > 10s) ---------- */
let ultimoHeartbeatSucesso = Date.now();
let alertaConexaoVisivel = false;

async function checarHeartbeat() {
  const ctrl = new AbortController();
  const tId = setTimeout(() => ctrl.abort(), 4000); // timeout da requisição individual
  try {
    const res = await fetch('/api/ping', { signal: ctrl.signal, cache: 'no-store' });
    clearTimeout(tId);
    if (res.ok) {
      ultimoHeartbeatSucesso = Date.now();
      if (alertaConexaoVisivel) {
        alertaConexaoVisivel = false;
        const banner = document.getElementById('alertaConectividade');
        if (banner) banner.style.display = 'none';
      }
      return;
    }
  } catch (e) {
    clearTimeout(tId);
  }

  // Se falhou ou demorou e já se passaram mais de 10s desde o último sucesso
  if (Date.now() - ultimoHeartbeatSucesso >= 10000) {
    if (!alertaConexaoVisivel) {
      alertaConexaoVisivel = true;
      const banner = document.getElementById('alertaConectividade');
      if (banner) banner.style.display = 'flex';
    }
  }
}

// Verifica heartbeat a cada 3.5 segundos
setInterval(checarHeartbeat, 3500);

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
        ${res.despachos_pendentes ? `
          <div style="background:rgba(239, 68, 68, 0.12); border:1px solid rgba(239, 68, 68, 0.35); border-radius:var(--raio); padding:10px 12px; margin-bottom:10px; display:flex; justify-content:space-between; align-items:center">
            <div>
              <b style="color:var(--verm-txt); font-size:13px">⚖️ ${res.despachos_pendentes} Despacho(s) Pendente(s)</b>
              <div style="font-size:12px; color:var(--tx2)">Demandas com resposta obrigatória aguardando seu atendimento.</div>
            </div>
            <button class="primario" id="btIrDespachosNotif" style="font-size:12px; padding:4px 10px">Ver Despachos</button>
          </div>
        ` : ''}
        ${res.avisos_pendentes ? `
          <div style="background:rgba(59, 130, 246, 0.12); border:1px solid rgba(59, 130, 246, 0.35); border-radius:var(--raio); padding:10px 12px; margin-bottom:10px; display:flex; justify-content:space-between; align-items:center">
            <div>
              <b style="color:#60a5fa; font-size:13px">📢 ${res.avisos_pendentes} Novo(s) Comunicado(s)</b>
              <div style="font-size:12px; color:var(--tx2)">Avisos do comando aguardando confirmação de ciente.</div>
            </div>
            <button class="acao-linha" id="btIrAvisosNotif" style="font-size:12px; padding:4px 10px">Ver Mural</button>
          </div>
        ` : ''}
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
        ` : (!res.despachos_pendentes && !res.avisos_pendentes ? `
          <div class="vazio" style="padding:24px 0; text-align:center">
            <span style="font-size:28px">🛡️</span>
            <div style="margin-top:8px; font-weight:600">Nenhum alerta pendente</div>
            <p style="font-size:12.5px; color:var(--tx3); margin:4px 0 0">Todas as comunicações, despachos e cautelas estão regularizados.</p>
          </div>
        ` : '')}
        <div style="display:flex; justify-content:flex-end; gap:8px">
          ${atrasos.length && ME && ME.papel !== 'admin' ? `
            <button class="primario" id="btIrMaterialNotif" style="font-size:13px; padding:6px 14px">Ir para Balcão de Material</button>
          ` : ''}
          <button class="acao-linha" id="btFecharNotif" style="font-size:13px; padding:6px 14px">Fechar</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    const btDesp = m.querySelector('#btIrDespachosNotif');
    if (btDesp) {
      btDesp.onclick = () => {
        m.remove();
        irPara('#/despachos');
      };
    }
    const btAv = m.querySelector('#btIrAvisosNotif');
    if (btAv) {
      btAv.onclick = () => {
        m.remove();
        irPara('#/avisos');
      };
    }
  }).catch(() => toast('Falha ao obter notificações', 'erro'));
}
window.modalNotificacoes = modalNotificacoes;

window.showSetupModal = function() {
  if (document.getElementById('setupModalGlobal')) return;
  const usuarioAtual = (typeof ME !== 'undefined' && ME) || window.ME || {};
  const html = `
    <div style="text-align:center; padding: 20px 0">
      <h2 style="margin-top:0; color:var(--tx1)">Atualização Obrigatória (v1.2)</h2>
      <p style="color:var(--tx2); margin-bottom:24px; line-height:1.5">
        Para acessar o sistema na versão 1.2, informe seu <b>Nº de Identificação Único</b> (ex: CPF) e cadastre uma nova senha pessoal.
      </p>
      <div class="form-linha" style="text-align:left; max-width:320px; margin:0 auto 16px auto">
        <label style="font-weight:700">Nº de Identificação Único (CPF/ID) *</label>
        <input id="setupLogin" placeholder="ex: 000.000.000-00" value="${esc(usuarioAtual.login || '')}" autocomplete="off" style="width:100%;height:38px">
      </div>
      <div class="form-linha" style="text-align:left; max-width:320px; margin:0 auto 20px auto">
        <label style="font-weight:700">Nova Senha Pessoal (mín. 8 caracteres) *</label>
        <input type="password" id="setupSenha" placeholder="Sua nova senha segura" autocomplete="new-password" style="width:100%;height:38px">
      </div>
      <button class="primario" id="setupBtn" style="width:100%; max-width:320px; padding:12px; font-weight:700">Salvar e Conectar</button>
      <div style="margin-top:14px">
        <button type="button" class="fantasma" id="setupLogoutBtn" style="font-size:12px; color:var(--tx3)">Desconectar / Trocar de Conta</button>
      </div>
    </div>
  `;
  const mask = modal(html);
  if (!mask) return;
  mask.id = 'setupModalGlobal';
  mask.onclick = (e) => e.stopPropagation();

  mask.querySelector('#setupBtn').onclick = async () => {
    const login = mask.querySelector('#setupLogin').value.trim();
    const senha = mask.querySelector('#setupSenha').value;
    if (!login || !senha) return toast('Preencha a identificação e a nova senha', 'erro');
    if (senha.length < 8) return toast('A senha deve ter no mínimo 8 caracteres', 'erro');
    try {
      const res = await api('/api/setup', { method: 'POST', body: JSON.stringify({ novo_login: login, nova_senha: senha }) });
      toast(res.msg || 'Cadastro atualizado com sucesso!');
      setTimeout(() => location.reload(), 1200);
    } catch (e) {}
  };

  const btnLogout = mask.querySelector('#setupLogoutBtn');
  if (btnLogout) {
    btnLogout.onclick = async () => {
      try { await api('/api/logout', { method: 'POST' }); } catch (e) {}
      definirUsuario(null);
      mask.remove();
      irPara('#/login');
      rotear();
    };
  }
};

/* onda 05/10: delegação ao módulo web/editor_rico.js (EditorRico).
   'completa' preserva o conjunto antigo de botões (H2/H3/¶, alinhamentos, limpar). */
window.montarRichEditor = function (container, placeholder, valorInicial) {
  return window.EditorRico && window.EditorRico.init
    ? window.EditorRico.init(container, { placeholder: placeholder, valorInicial: valorInicial, toolbar: 'completa' })
    : null;
};

})();

/* ---------- anexos com integração ao Drive (ordem Diretor 04/10; remake DELIB-0010
   item 5, 05/10): NUNCA mais lista plana — modal é navegador de PASTAS começando na
   RAIZ do drive do usuário (GET /api/drive/itens, sem pasta_id = raiz acessível),
   com breadcrumbs clicáveis, pastas navegáveis e busca local de arquivos. Seleção
   devolve anexos por REFERÊNCIA ({drive_arquivo_id, nome, tipo, tamanho}) — nada
   duplicado. Seleção PERSISTE ao navegar entre pastas. */
window.abrirSeletorDrive = function (opcoes) {
  opcoes = opcoes || {};
  const jaSel = (opcoes.jaSelecionados || []).filter(a => a && a.drive_arquivo_id);
  const sel = new Set(jaSel.map(a => a.drive_arquivo_id));
  const meta = {}; // drive_arquivo_id -> {nome, tipo, tamanho} (acumula o que já foi visto)
  jaSel.forEach(a => { meta[a.drive_arquivo_id] = { nome: a.nome, tipo: a.tipo, tamanho: a.tamanho }; });

  const html = `
    <div class="modal">
      <h3 style="margin-top:0">Anexar do Drive</h3>
      <div id="sdTrilha" style="display:flex;flex-wrap:wrap;align-items:center;gap:4px;font-size:12.5px;margin-bottom:8px;color:var(--tx2)"></div>
      <input type="text" id="sdBusca" placeholder="Filtrar arquivos desta pasta por nome…" style="width:100%;margin-bottom:10px">
      <div id="sdLista" style="max-height:44vh;overflow-y:auto;display:flex;flex-direction:column;gap:6px;min-height:120px"></div>
      <div id="sdRodape" style="margin-top:8px;font-size:12px;color:var(--tx3);min-height:16px"></div>
      <div class="modal-acoes" style="justify-content:flex-end;gap:8px;margin-top:10px">
        <button type="button" class="acao-linha" id="sdCancelar">Cancelar</button>
        <button type="button" class="primario" id="sdConfirmar">Anexar selecionados</button>
      </div>
    </div>`;
  const m = window.abrirModal(html, null, { largura: '620px' });
  const q = s => m.modal.querySelector(s);

  let trilha = [{ id: 0, nome: 'Meu Drive' }];
  let pastasAqui = [];
  let arqsAqui = [];

  function renderTrilha() {
    const t = q('#sdTrilha');
    t.innerHTML = trilha.map((p, i) => {
      const ultimo = i === trilha.length - 1;
      const no = ultimo
        ? `<b style="color:var(--tx)">${esc(p.nome)}</b>`
        : `<a href="javascript:void(0)" data-sdpasta="${p.id}" style="color:var(--tx2);text-decoration:underline">${esc(p.nome)}</a>`;
      return (i ? '<span style="color:var(--tx3)">›</span>' : '') + no;
    }).join('');
    t.querySelectorAll('[data-sdpasta]').forEach(a => { a.onclick = () => navegar(+a.dataset.sdpasta); });
  }

  function renderLista() {
    const termo = (q('#sdBusca').value || '').toLowerCase();
    const arqs = arqsAqui.filter(a => !termo || (a.nome_original || '').toLowerCase().includes(termo));
    const linhas = [];
    (pastasAqui || []).forEach(p => {
      linhas.push(`
        <div class="acao-linha" data-sdentra="${p.id}" style="display:flex;align-items:center;gap:10px;cursor:pointer;padding:8px 10px;border-radius:8px">
          <span style="font-size:16px">📁</span>
          <span style="flex:1"><b>${esc(p.nome)}</b> <small style="color:var(--tx3)">· ${p.qtd_itens || 0} item(ns)</small></span>
          <span style="font-size:11.5px;color:var(--tx3)">abrir ›</span>
        </div>`);
    });
    arqs.forEach(a => {
      meta[a.id] = { nome: a.nome_original, tipo: a.tipo, tamanho: a.tamanho };
      linhas.push(`
        <label class="acao-linha" style="display:flex;align-items:center;gap:10px;cursor:pointer;padding:8px 10px;border-radius:8px">
          <input type="checkbox" data-id="${a.id}" ${sel.has(a.id) ? 'checked' : ''}>
          <span style="font-size:16px">📄</span>
          <span style="flex:1"><b>${esc(a.nome_original)}</b> <small style="color:var(--tx3)">(${formatarTamanhoBytes(a.tamanho)})</small></span>
        </label>`);
    });
    q('#sdLista').innerHTML = linhas.length === 0
      ? '<div style="padding:18px;text-align:center;color:var(--tx3)">Pasta vazia.</div>'
      : linhas.join('');
    q('#sdLista').querySelectorAll('[data-sdentra]').forEach(el => { el.onclick = () => navegar(+el.dataset.sdentra); });
    q('#sdLista').querySelectorAll('input[type=checkbox]').forEach(cb => {
      cb.onchange = () => {
        const id = +cb.dataset.id;
        if (cb.checked) sel.add(id); else sel.delete(id);
        atualizarRodape();
      };
    });
  }

  function atualizarRodape() {
    q('#sdRodape').textContent = sel.size > 0 ? `${sel.size} arquivo(s) selecionado(s)` : '';
  }

  function navegar(pid) {
    q('#sdBusca').value = '';
    q('#sdLista').innerHTML = '<div class="carregando">Carregando…</div>';
    api('/api/drive/itens' + (pid > 0 ? '?pasta_id=' + pid : '')).then(d => {
      trilha = (d && d.breadcrumbs && d.breadcrumbs.length) ? d.breadcrumbs : [{ id: 0, nome: 'Meu Drive' }];
      pastasAqui = (d && d.pastas) || [];
      arqsAqui = (d && d.arquivos) || [];
      renderTrilha();
      renderLista();
      atualizarRodape();
    }).catch(() => {
      q('#sdLista').innerHTML = '<div style="padding:18px;text-align:center;color:var(--verm-txt)">Falha ao carregar a pasta.</div>';
    });
  }

  // Busca NUNCA recria o próprio input (o filtro atua só na lista — foco preservado).
  q('#sdBusca').oninput = renderLista;
  q('#sdCancelar').onclick = () => m.fechar();
  q('#sdConfirmar').onclick = () => {
    const escolhidos = [...sel].map(id => {
      const mm = meta[id] || {};
      return { drive_arquivo_id: id, nome: mm.nome || ('arquivo-' + id), tipo: mm.tipo || '', tamanho: mm.tamanho || 0 };
    });
    m.fechar();
    if (typeof opcoes.onConfirma === 'function') opcoes.onConfirma(escolhidos);
  };
  navegar(0);
};

function formatarTamanhoBytes(b) {
  if (b === undefined || b === null) return '';
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
  return (b / 1048576).toFixed(1) + ' MB';
}

window.enviarArquivoParaDrive = function (file) {
  const fd = new FormData();
  fd.append('arquivo', file);
  return fetch('/api/drive/upload', {
    method: 'POST',
    headers: { 'X-SCI': '1' },
    credentials: 'same-origin',
    body: fd
  }).then(r => {
    if (!r.ok) throw new Error('upload falhou');
    return r.json();
  }).then(meta => ({
    drive_arquivo_id: meta.id, nome: meta.nome_original, tipo: meta.tipo, tamanho: meta.tamanho
  }));
};

/* ---------- módulos em modo de desenvolvimento (ordem 04/10) ---------- */
window.ViewModuloEmDesenvolvimento = function () {
  const app = document.getElementById('app');
  if (!app) return;
  if (window.navAtiva) navAtiva('');
  app.innerHTML = `
    <div class="cartao" style="max-width:560px;margin:60px auto;text-align:center;padding:38px 28px">
      <div style="font-size:44px;margin-bottom:12px">🚧</div>
      <h2 style="margin:0 0 8px">Módulo em desenvolvimento</h2>
      <p style="color:var(--tx2);font-size:13.5px;margin:0 0 4px">Este módulo foi reservado para uma versão futura do SCI.</p>
      <p style="color:var(--tx3);font-size:12.5px;margin:0">Material → 1.6 · Calendário → 1.7 · Escalas → 1.8 · Consciência Situacional → 1.9</p>
      <button type="button" class="primario" style="margin-top:18px;padding:8px 22px" onclick="location.hash='#/hoje'">Voltar ao início</button>
    </div>`;
};

/* ---------- usuário normal: sem módulos (ordem 04/10) ---------- */
window.ViewSemModulo = function () {
  const app = document.getElementById('app');
  if (!app) return;
  if (window.navAtiva) navAtiva('');
  app.innerHTML = `
    <div class="cartao" style="max-width:560px;margin:60px auto;text-align:center;padding:38px 28px">
      <div style="font-size:44px;margin-bottom:12px">🔒</div>
      <h2 style="margin:0 0 8px">Módulo não disponível</h2>
      <p style="color:var(--tx2);font-size:13.5px;margin:0">Sua conta ainda não possui funções no sistema. Procure o encarregado de pessoal do seu grupo para receber uma função.</p>
    </div>`;
};

/* ---------- v1.6.0 Fase 5: conta SEM FUNÇÃO — página de bloqueio ----------
   Destino de login/rotear de toda conta com papel 'sem_funcao' (e papel vazio
   sem cadeira legada): sem módulo algum — o servidor barra os dados com a
   guarda central de escopo (-1 → 403). Sai do bloqueio por SAIR (logout) ou
   quando o gerente/encarregado designar uma cadeira (o próximo login resolve
   o contexto pela linha materializada — Fase 3). */
window.ViewBloqueioSemFuncao = function () {
  const app = document.getElementById('app');
  if (!app) return;
  if (window.navAtiva) navAtiva('');
  app.innerHTML = `
    <div class="cartao" style="max-width:560px;margin:60px auto;text-align:center;padding:38px 28px">
      <div style="font-size:44px;margin-bottom:12px">🚫</div>
      <h2 style="margin:0 0 8px">Você não tem função neste grupo</h2>
      <p style="color:var(--tx2);font-size:13.5px;margin:0 0 20px">Procure o gerente/encarregado para receber uma designação. Quando ela for feita, faça login novamente para entrar.</p>
      <button type="button" class="primario" id="btBloqueioSair" style="padding:9px 26px">SAIR</button>
    </div>`;
  const bt = document.getElementById('btBloqueioSair');
  if (bt) {
    bt.onclick = async () => {
      try { await api('/api/logout', { method: 'POST', body: '{}' }); } catch (e) {}
      definirUsuario(null);
      irPara('#/login');
    };
  }
};
