/* SCI — front vanilla (sem build, sem framework). Hash routing. v9.11.2 */
'use strict';
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const CICLO = ['presente', 'atraso', 'falta', 'justificada'];
let ME = null, DESTINOS = [];
const rotuloPapel = p => p === 'admin' ? 'ADMIN' : p === 'gerente' ? 'GERENTE' : 'OPERADOR';

async function api(path, opts = {}) {
  opts.headers = Object.assign({ 'Content-Type': 'application/json', 'X-SCI': '1' }, opts.headers || {});
  if ((opts.method || 'GET') === 'GET') delete opts.headers['X-SCI'];
  let r;
  try { r = await fetch(path, opts); }
  catch (e) { toast('Servidor inacessível — verifique a rede', 'erro'); throw e; }
  if (r.status === 401 && !path.includes('/login')) { ME = null; location.hash = '#/login'; throw new Error('não autenticado'); }
  const json = (r.headers.get('content-type') || '').includes('json');
  const data = json ? await r.json() : await r.text();
  if (!r.ok) { toast((data && data.erro) || ('Falha ' + r.status), 'erro'); throw new Error('falha'); }
  return data;
}
function toast(msg, tipo = 'ok') {
  const d = document.createElement('div');
  d.className = 'toast' + (tipo === 'erro' ? ' erro' : '');
  d.textContent = msg;
  $('#toasts').appendChild(d);
  setTimeout(() => d.remove(), 3800);
}
const pill = s => `<span class="pill p-${s}">${s}</span>`;

/* ---------- LOGIN ---------- */
function viewLogin() {
  $('#topbar').classList.add('oculto');
  $('#app').innerHTML = `
  <div class="login-box">
    <div class="marca"><span class="logo">SCI</span></div>
    <div class="cartao">
      <div class="campo"><label>Usuário</label><input id="lg" autocomplete="username"></div>
      <div class="campo"><label>Senha</label><input id="sn" type="password" autocomplete="current-password"></div>
      <button class="primario" id="btEntrar">Entrar</button>
    </div></div>`;
  const entrar = async () => {
    try {
      const r = await api('/api/login', { method: 'POST', body: JSON.stringify({ login: $('#lg').value.trim(), senha: $('#sn').value }) });
      ME = r.usuario; toast('Bem-vindo, ' + ME.login);
      location.hash = ME.papel === 'admin' ? '#/admin' : '#/hoje';
      rotear(); // hash pode não mudar se já estava lá
    } catch (e) {}
  };
  $('#btEntrar').onclick = entrar;
  $('#sn').onkeydown = e => { if (e.key === 'Enter') entrar(); };
}

/* ---------- CONFERÊNCIA DE PESSOAL (iniciar → verificar → fechar) ---------- */
let ESTADO = null;
async function viewHoje() {
  navAtiva('#/hoje');
  $('#app').innerHTML = '<div class="carregando">Carregando efetivo…</div>';
  const d = await api('/api/conferencia/hoje');
  if (!DESTINOS.length) { try { DESTINOS = await api('/api/catalogo/destinos'); } catch (e) { DESTINOS = []; } }
  DESTINOS = DESTINOS.filter(x => x.ativo === 1 || x.ativo === true);
  const est = {}, dest = {}, obs = {}, verif = new Set(), temComentario = {};
  ESTADO = { c: d.conferencia, pessoas: d.pessoas, est, dest, obs, verif, temComentario };
  if (d.conferencia) for (const [pid, v] of Object.entries(d.conferencia.estados)) {
    est[pid] = v.situacao;
    dest[pid] = v.destino_id;
    if (v.observacao) obs[pid] = v.observacao;
    verif.add(+pid);
  }
  ESTADO = { c: d.conferencia, pessoas: d.pessoas, est, dest, obs, verif, temComentario };
  renderHoje();
}
function renderHoje(filtro = '') {
  const e = ESTADO, semC = !e.c;
  const f = filtro.trim().toLowerCase();
  const porSetor = {};
  e.pessoas.filter(p => !f || p.nome_guerra.toLowerCase().includes(f) || p.nome_completo.toLowerCase().includes(f))
    .forEach(p => { (porSetor[p.setor || 'Sem setor'] = porSetor[p.setor || 'Sem setor'] || []).push(p); });
  let listas = '';
  for (const setor of Object.keys(porSetor).sort()) {
    listas += `<div class="grupo-setor"><h4>${esc(setor)} · ${porSetor[setor].length}</h4><div class="lista-pessoa">` +
      porSetor[setor].map(p => {
        const sit = e.est[p.id] || 'presente';
        const sel = (sit === 'justificada')
          ? `<select class="sel-destino" data-id="${p.id}"><option value="">destino…</option>` +
            DESTINOS.map(dx => `<option value="${dx.id}" ${e.dest[p.id] == dx.id ? 'selected' : ''}>${esc(dx.nome)}</option>`).join('') + '</select>'
          : '';
        const optSit = s => `<option value="${s}" ${sit === s ? 'selected' : ''}>${{presente:'Presente', atraso:'Atraso', falta:'Falta', justificada:'Justificada'}[s]}</option>`;
        return `<div class="pessoa ${e.verif.has(p.id) ? 'verificado' : ''}" data-id="${p.id}">
          <input type="checkbox" class="chk" data-id="${p.id}" ${e.verif.has(p.id) ? 'checked' : ''} title="verifiquei esta pessoa">
          <span class="nome"><b>${esc(p.nome_guerra)}</b><small>${esc(p.nome_completo)}${p.funcao ? ' · ' + esc(p.funcao) : ''}${e.obs[p.id] ? ' · 📝' : ''}${e.temComentario[p.id] ? ' · 💬' : ''}</small></span>
          <select class="sel-situacao" data-id="${p.id}" title="situação">${optSit('presente')}${optSit('atraso')}${optSit('falta')}${optSit('justificada')}</select>
          ${sel}<button type="button" class="fantasma bt-coment" data-id="${p.id}" title="comentários" style="min-height:36px;padding:4px 8px">💬</button></div>`;
      }).join('') + '</div></div>';
  }
  const banner = semC
    ? `<div class="cartao"><p style="color:var(--tx2)">Nenhuma conferência aberta. Ao iniciar, a data e o horário de Brasília são registrados automaticamente.</p>
       <div style="display:flex;gap:8px;align-items:end;margin-top:10px">
         <button class="primario" id="btIniciar" style="min-height:44px">▶ Iniciar conferência</button></div></div>`
    : `<div class="cartao" style="display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap">
       <span>${pill('aberta')} <b>Conferência #${e.c.id}</b> · aberta em ${e.c.data} às ${fmtHora(e.c.criada_em)}${e.c.local ? ` · ${esc(e.c.local)}` : ''} · situação pelo menu de cada militar, ✅ para verificar</span>
       <button class="perigo" id="btFechar">✕ Fechar conferência</button></div>`;
  $('#app').innerHTML = `<h2>Conferência de pessoal</h2>${banner}
    <div class="barra-fixa">
      <input id="busca" placeholder="buscar nome…">
      <button class="primario" id="btFecharBarra" ${semC ? 'disabled' : ''}>✕ FECHAR CONFERÊNCIA</button>
    </div>
    <div id="lista">${semC ? '' : listas}</div>
    <div id="listaConf"><div class="carregando">…</div></div>`;
  if (semC) {
    $('#btIniciar').onclick = async () => {
      try {
        await api('/api/conferencia/iniciar', { method: 'POST', body: JSON.stringify({}) });
        toast('Conferência iniciada — data e hora de Brasília registradas');
        viewHoje();
      } catch (err) {}
    };
    carregarListaConf();
    return;
  }
  const contSpan = () => {
    const c = { presente: 0, atraso: 0, falta: 0, justificada: 0 };
    e.pessoas.forEach(p => c[e.est[p.id] || 'presente']++);
    return `<b>${e.verif.size}/${e.pessoas.length}</b> verificados`;
  };
  const atualizar = () => { document.querySelector('.barra-fixa').innerHTML =
    `<input id="busca" placeholder="buscar nome…" value="${esc(f)}">` +
    `<span class="cont">${contSpan()}</span>` +
    `<button class="perigo" id="btFecharBarra">✕ FECHAR CONFERÊNCIA</button>`;
    ligarBarra();
  };
  const ligarBarra = () => {
    $('#busca').oninput = ev => renderHoje(ev.target.value);
    $('#btFecharBarra').onclick = fecharConferencia;
  };
  atualizar();
  // v9.10.3: situação por DROP-DOWN (não cíclico) — mudar para falta/justificada abre modal
  document.querySelectorAll('.sel-situacao').forEach(s => s.onchange = () => {
    const id = +s.dataset.id;
    const novo = s.value;
    const atual = ESTADO.est[id] || 'presente';
    if (novo === atual) return;
    ESTADO.est[id] = novo;
    ESTADO.verif.delete(id);
    if (novo === 'falta' || novo === 'justificada') modalLancamento(id, novo, () => renderHoje($('#busca').value));
    else renderHoje($('#busca').value);
  });
  document.querySelectorAll('.pessoa .chk').forEach(ch => ch.onchange = () => {
    const id = +ch.dataset.id;
    if (ch.checked) ESTADO.verif.add(id); else ESTADO.verif.delete(id);
    ch.closest('.pessoa').classList.toggle('verificado', ch.checked);
    atualizar();
  });
  document.querySelectorAll('.sel-destino').forEach(s => s.onchange = () => { ESTADO.dest[+s.dataset.id] = +s.value || null; });
  document.querySelectorAll('.bt-coment').forEach(b => b.onclick = ev => {
    ev.stopPropagation();
    modalComentarios(+b.dataset.id);
  });
  $('#btFechar').onclick = fecharConferencia;
  carregarListaConf();
}
/* --- lista cronológica embutida (v9.4): abertas primeiro (novas→antigas), depois fechadas --- */
async function carregarListaConf() {
  const alvo = $('#listaConf');
  if (!alvo) return;
  try {
    const lista = await api('/api/conferencia/lista');
    const linha = c => `
      <tr data-cid="${c.id}"><td class="num"><b>#${c.id}</b></td>
      <td>${c.status === 'aberta' ? 'Aberta' : 'Fechada'}</td>
      <td>${c.status === 'aberta' ? fmtHora(c.criada_em) : fmtHora(c.fechada_em)}</td>
      <td><b>${c.data}</b></td>
      <td>${esc(c.grupo || '—')}</td>
      <td>${esc(c.criado_por || '—')}</td>
      <td class="num">${c.lancamentos}</td>
      <td>${c.status === 'fechada'
        ? `<a href="/api/conferencia/${c.id}/relatorio.pdf" target="_blank"><button class="primario" style="min-height:36px;padding:8px 12px">Relatório PDF</button></a>`
        : `<span style="color:var(--ambar);font-size:13px">em andamento</span>`}</td></tr>`;
    const porData = (a, b) => b.data.localeCompare(a.data) || b.id - a.id;
    const abertas = lista.filter(c => c.status === 'aberta').sort(porData);
    const fechadas = lista.filter(c => c.status === 'fechada').sort(porData);
    const tabela = (titulo, itens) => `
      <h3 style="margin:14px 0 8px">${titulo} (${itens.length})</h3>
      <div class="cartao"><div class="rolagem"><table>
      <thead><tr><th class="num">ID</th><th>Status</th><th>Horário</th><th>Data</th><th>Grupo</th><th>Operador</th><th class="num">Lanç.</th><th>Relatório</th></tr></thead>
      <tbody>${itens.map(linha).join('') || '<tr><td colspan="8"><span class="vazio">nenhuma</span></td></tr>'}</tbody></table></div></div>`;
    alvo.innerHTML = `<h2 style="margin-top:22px">Histórico de conferências</h2>
      <div class="cartao" style="margin-bottom:10px"><div class="campo" style="margin:0"><label>Pesquisar por ID da conferência</label><input id="fConfID" placeholder="ex.: 3"></div></div>` +
      tabela('Abertas', abertas) + tabela('Fechadas', fechadas) +
      `<p style="color:var(--tx2);font-size:12px">O relatório PDF só é gerado para conferências fechadas.</p>`;
    $('#fConfID').oninput = () => {
      const q = $('#fConfID').value.trim().replace('#', '');
      document.querySelectorAll('#listaConf tr[data-cid]').forEach(tr => {
        tr.style.display = !q || tr.dataset.cid === q ? '' : 'none';
      });
    };
  } catch (e) { alvo.innerHTML = ''; }
}
async function modalComentarios(pessoaId) {
  const p = ESTADO.pessoas.find(x => x.id === pessoaId);
  const cid = ESTADO.c.id;
  const div = document.createElement('div');
  div.className = 'modal-mask';
  div.innerHTML = `<div class="modal" style="max-width:460px">
    <h3>💬 Comentários — ${esc(p.nome_guerra)}</h3>
    <div id="cmLista" style="max-height:220px;overflow:auto;margin-bottom:10px">
      <span class="vazio">carregando…</span></div>
    <div class="campo"><label>Novo comentário</label>
      <textarea id="cmNovo" rows="2" placeholder="registre aqui…"></textarea></div>
    <div class="modal-acoes">
      <button class="fantasma" id="cmX">Fechar</button>
      <button class="primario" id="cmGo">Adicionar</button></div></div>`;
  document.body.appendChild(div);
  const fechar = () => div.remove();
  div.onclick = ev => { if (ev.target === div) fechar(); };
  div.querySelector('#cmX').onclick = fechar;
  div.addEventListener('keydown', ev => { if (ev.key === 'Escape') fechar(); });
  const carregar = async () => {
    try {
      const lista = await api(`/api/comentarios/${cid}`);
      const meus = lista.filter(c => c.pessoa === p.nome_guerra);
      ESTADO.temComentario[pessoaId] = meus.length > 0;
      $('#cmLista').innerHTML = meus.length ? meus.map(c =>
        `<div class="cartao" style="padding:8px;margin-bottom:6px">
         <small style="color:var(--tx2)">#${c.ordem} · ${esc(c.operador)} · ${(c.datahora || '').slice(0, 16).replace('T', ' ')}</small>
         <div>${esc(c.comentario)}</div></div>`).join('') : '<span class="vazio">sem comentários</span>';
      renderHoje($('#busca') ? $('#busca').value : '');
    } catch (e) { $('#cmLista').innerHTML = '<span class="vazio">falha ao carregar</span>'; }
  };
  await carregar();
  div.querySelector('#cmGo').onclick = async () => {
    const txt = div.querySelector('#cmNovo').value.trim();
    if (!txt) { toast('Escreva o comentário', 'erro'); return; }
    try {
      await api('/api/comentarios', { method: 'POST', body: JSON.stringify({ conferencia_id: cid, pessoa_id: pessoaId, comentario: txt }) });
      div.querySelector('#cmNovo').value = '';
      toast('Comentário adicionado');
      await carregar();
    } catch (e) {}
  };
}
function modalLancamento(id, sit, aoSalvar) {
  const p = ESTADO.pessoas.find(x => x.id === id);
  const e = ESTADO;
  const div = document.createElement('div');
  div.className = 'modal-mask';
  div.innerHTML = `
    <div class="modal">
      <h3>${sit === 'falta' ? 'Falta' : 'Justificada'} — ${esc(p.nome_guerra)}</h3>
      ${sit === 'justificada' ? `<div class="campo"><label>Destino (obrigatório)</label>
        <select id="mDest"><option value="">— escolher —</option>${DESTINOS.filter(x => x.ativo === 1 || x.ativo === true).map(dx =>
          `<option value="${dx.id}" ${e.dest[id] == dx.id ? 'selected' : ''}>${esc(dx.nome)}</option>`).join('')}</select></div>` : ''}
      <div class="campo"><label>Motivo / observação</label><textarea id="mObs" rows="3" placeholder="ex.: não compareceu, sem contato…">${esc(e.obs[id] || '')}</textarea></div>
      <div class="modal-acoes">
        <button class="fantasma" id="mCancel">Cancelar</button>
        <button class="primario" id="mOk">Gravar</button>
      </div></div>`;
  document.body.appendChild(div);
  const fechar = () => div.remove();
  div.onclick = ev => { if (ev.target === div) fechar(); };
  div.querySelector('#mCancel').onclick = fechar;
  div.querySelector('#mOk').onclick = () => {
    const destino = div.querySelector('#mDest') ? (+div.querySelector('#mDest').value || null) : null;
    const obs = div.querySelector('#mObs').value.trim();
    if (sit === 'justificada' && !destino) { toast('Justificada exige destino', 'erro'); return; }
    e.dest[id] = destino; e.obs[id] = obs;
    fechar();
    if (aoSalvar) aoSalvar();
  };
  div.addEventListener('keydown', ev => { if (ev.key === 'Escape') fechar(); });
  const ta = div.querySelector('#mObs');
  if (ta) ta.focus();
}
async function fecharConferencia() {
  const e = ESTADO;
  const semDest = e.pessoas.filter(p => (e.est[p.id] || 'presente') === 'justificada' && !e.dest[p.id]);
  if (semDest.length) { toast('Justificada exige destino: ' + semDest.map(p => p.nome_guerra).join(', '), 'erro'); return; }
  const naoVerif = e.pessoas.filter(p => !e.verif.has(p.id)).map(p => p.nome_guerra);
  let msg = `Fecha a conferência e grava ${e.pessoas.length} lançamentos?`;
  if (naoVerif.length) msg = `ATENÇÃO: ${naoVerif.length} sem verificação (${naoVerif.join(', ')}).\n\n` + msg;
  if (!confirm(msg)) return;
  const lanc = e.pessoas.map(p => ({ pessoa_id: p.id, situacao: e.est[p.id] || 'presente', destino_id: e.dest[p.id] || null, observacao: e.obs[p.id] || null }));
  try {
    const r = await api('/api/conferencia/fechar', { method: 'POST', body: JSON.stringify({ id: e.c.id, lancamentos: lanc }) });
    toast(`Conferência fechada — ${r.gravados} lançamentos gravados`);
    location.hash = '#/hoje';
    location.reload(); // v9.10.2 (ordem Tenente): fechar = recarga completa da página
  } catch (err) {}
}

/* ---------- MEU USUÁRIO (gerente/operador — R3: admin não tem esta aba) ---------- */
async function viewPerfil() {
  navAtiva('#/perfil');
  $('#app').innerHTML = '<div class="carregando">…</div>';
  const d = await api('/api/perfil');
  const u = d.usuario;
  $('#app').innerHTML = `<h2>Meu usuário</h2>
    <div class="cartao" style="max-width:640px">
      <div class="form-linha">
        <div class="campo"><label>Login</label><input value="${esc(u.login)}" disabled></div>
        <div class="campo"><label>Função na conta</label><input value="${esc(rotuloPapel(u.papel))}" disabled></div></div>
      <div class="form-linha">
        <div class="campo"><label>Grupo</label><input value="${esc(d.grupo || '— (sem grupo)')}" disabled></div>
        <div class="campo"><label>Setor</label><input value="${esc(d.setor || '—')}" disabled></div>
        <div class="campo"><label>Função</label><input value="${esc(d.funcao || '—')}" disabled></div></div>
      <div class="form-linha">
        <div class="campo"><label>Nome de guerra</label><input id="pfNg" value="${esc(u.nome_guerra || '')}"></div>
        <div class="campo"><label>Nome completo</label><input id="pfNc" value="${esc(u.nome_completo || '')}"></div></div>
      <button class="primario" id="pfGo">Salvar perfil</button>
      <p style="color:var(--tx2);font-size:12px">Setor e função vêm do grupo. Senha: menu do seu nome no topo direito.</p></div>`;
  $('#pfGo').onclick = async () => {
    const ng = $('#pfNg').value.trim(), nc = $('#pfNc').value.trim();
    if (!ng || !nc) { toast('Preencha os dois nomes', 'erro'); return; }
    await api('/api/perfil', { method: 'PATCH', body: JSON.stringify({ nome_guerra: ng, nome_completo: nc }) });
    ME.nome_guerra = ng; ME.nome_completo = nc;
    montarNav();
    toast('Perfil salvo');
  };
}

/* ---------- GERENCIAR (gerente — v9.4: MEU GRUPO · BANCO DE PESSOAL · OPERADORES · árvore nested READ ONLY) ---------- */
function arvoreHTML(nos, nivel) {
  const pad = nivel * 22;
  return nos.map(n => {
    const temFilhos = n.filhos && n.filhos.length;
    const id = 'no' + n.id + '_' + nivel + Math.random().toString(36).slice(2, 6);
    const rec = n.efetivo_total !== undefined && n.efetivo_total !== n.efetivo;
    const efetTxt = rec
      ? ` · efetivo total: <b style="color:#000">${n.efetivo_total}</b> <small>(próprio ${n.efetivo})</small>`
      : ` · ${n.efetivo} no efetivo`;
    return `<div style="margin-left:${pad}px;padding:5px 8px;border-left:3px solid var(--verde);margin-bottom:4px;background:#f4f8f4;border-radius:0 6px 6px 0">
      ${nivel === 0 && temFilhos ? `<span data-tgl="${id}" style="cursor:pointer;font-weight:700;color:#000">▸ </span>` : ''}
      <b style="color:#000">${esc(n.nome)}</b> <small style="color:#000">#${n.id}</small>
      <code style="background:#e8f0e8;color:#12291b;padding:1px 6px;border-radius:4px;font-weight:700;font-size:11px">${esc(n.codigo)}</code>
      <small style="color:#000">${efetTxt} · ${n.contas} conta(s)</small>
      ${temFilhos ? `<div id="${id}" class="oculto" style="margin-top:4px">${arvoreHTML(n.filhos, nivel + 1)}</div>` : ''}
    </div>`;
  }).join('');
}
function ligarTogglesArvore(raiz) {
  (raiz || document).querySelectorAll('[data-tgl]').forEach(s => s.onclick = () => {
    const alvo = document.getElementById(s.dataset.tgl);
    const aberto = !alvo.classList.contains('oculto');
    alvo.classList.toggle('oculto', aberto);
    s.textContent = aberto ? '▸ ' : '▾ ';
  });
}
async function viewGrupos() {
  navAtiva('#/grupos');
  $('#app').innerHTML = '<div class="carregando">…</div>';
  const [grupos, arvore, pessoas, setores, funcoes, contas, tags] = await Promise.all([
    api('/api/grupos'), api('/api/grupos/arvore'), api('/api/pessoas'),
    api('/api/catalogo/setores'), api('/api/catalogo/funcoes'), api('/api/usuarios'),
    api('/api/catalogo/tags').catch(() => [])]);
  const meus = grupos.filter(g => g.id === ME.grupo_id);
  const ativos = l => (l || []).filter(x => x.ativo === 1 || x.ativo === true);
  const optSetores = ativos(setores), optFuncoes = ativos(funcoes);
  const operadores = contas.filter(c => c.grupo_id === ME.grupo_id && c.papel === 'operador');

  /* --- formulário de militar (criar/editar) --- */
  const formPessoa = `
    <div class="cartao"><h3 style="margin-top:0">Cadastrar / editar militar</h3>
      <input type="hidden" id="pId">
      <div class="form-linha"><div class="campo"><label>Nome de guerra</label><input id="pNg"></div>
      <div class="campo"><label>Nome completo</label><input id="pNc"></div></div>
      <div class="form-linha"><div class="campo"><label>Setor</label><select id="pSetor"><option value="">—</option>${optSetores.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
      <div class="campo"><label>Função</label><select id="pFuncao"><option value="">—</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
      <div class="campo"><label>Status</label><select id="pStatus"><option value="ativo">ativo</option><option value="inativo">inativo</option></select></div></div>
      <button class="primario" id="pSalvar">Salvar</button>
      <h3 style="margin-top:16px">Adição em lote — cole as linhas e importe</h3>
      <p style="color:var(--tx2);font-size:12px;margin:4px 0">Formato (1 por linha, separado por ponto-e-vírgula): <code>nome de guerra ; nome completo ; setor ; função</code> — setor e função são opcionais e devem já existir no catálogo.</p>
      <textarea id="csv" rows="5" placeholder="SILVA;José da Silva;Comando;Motorista&#10;SOUSA;Maria de Sousa;Serviços&#10;PERES;Bruno Peres"></textarea>
      <button class="acao-linha" id="csvGo" style="margin-top:8px">Importar linhas</button></div>`;

  /* --- tabela de pessoal (checkbox p/ operações batch — v9.7) --- */
  const linhasP = (pessoas.pessoas || []).map(p =>
    `<tr data-p='${esc(JSON.stringify(p))}'><td><input type="checkbox" class="chkP" data-id="${p.id}"></td>
     <td class="num">#${p.id}</td><td><b>${esc(p.nome_guerra)}</b></td><td>${esc(p.nome_completo)}</td>
     <td>${esc(p.setor)}${p.setor_id ? ` <small style="color:#000">#${p.setor_id}</small>` : ''}</td>
     <td>${esc(p.funcao)}${p.funcao_id ? ` <small style="color:#000">#${p.funcao_id}</small>` : ''}</td>
     <td>${pill(p.status === 'ativo' ? 'presente' : 'justificada')} ${p.status}</td>
     <td><button class="acao-linha" data-edit="${p.id}">editar</button>
     <button class="acao-linha" data-excP="${p.id}" data-nome="${esc(p.nome_guerra)}">excluir</button></td></tr>`).join('');

  /* --- operadores --- */
  /* --- abas do Gerenciar (v9.5): Pessoal · Tags · Grupos · Operadores --- */
  $('#app').innerHTML = `<h2>Gerenciar</h2>
    <div class="abas" id="abasGer">
      <button data-g="pessoal" class="ativo">Pessoal</button>
      <button data-g="tags">Tags</button>
      <button data-g="grupos">Grupos</button>
      <button data-g="operadores">Operadores</button></div>
    <div id="gerPessoal">
      ${formPessoa}
      <div class="cartao"><h3 style="margin-top:0">BANCO DE PESSOAL (${(pessoas.pessoas || []).length})</h3>
        <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:8px">
          <label style="font-size:13px"><input type="checkbox" id="chkTodosP"> todos</label>
          <button class="primario" id="btEditLote" disabled>✏️ Editar selecionados (<span id="nSel">0</span>)</button>
          <button class="perigo" id="btExcLote" disabled>Excluir selecionados (<span id="nSel2">0</span>)</button>
          <span style="color:var(--tx2);font-size:12px">com histórico de conferência: exclusão vira inativo (histórico preservado)</span></div>
        <div class="rolagem"><table><thead><tr><th></th><th>Guerra</th><th>Completo</th><th>Setor</th><th>Função</th><th>Status</th><th></th></tr></thead>
        <tbody id="tabP">${linhasP || '<tr><td colspan="7"><span class="vazio">nenhum militar cadastrado</span></td></tr>'}</tbody></table></div></div>
    </div>
    <div id="gerTags" class="oculto">
      <div class="cartao"><h3 style="margin-top:0">TAGS do meu grupo — Setores · Funções · Tags</h3>
        <p style="color:var(--tx2);font-size:12px;margin:4px 0">Itens próprios do grupo + <b>herdados dos grupos de cima</b> (o que existe acima vale aqui; o que o grupo cria não sobe). ✕ exclui (em uso → desativa), ⏸ desativa preservando histórico.</p>
        <div id="catGer">…</div>
        <div class="form-linha" style="margin-top:10px">
          <div class="campo"><label>Catálogo</label><select id="cgT"><option value="setores">Setores</option><option value="funcoes">Funções</option><option value="tags">Tags</option><option value="destinos">Destinos</option></select></div>
          <div class="campo"><label>Novo item</label><input id="cgN" placeholder="nome"></div>
          <div class="campo" style="align-self:end"><button class="primario" id="cgGo">Adicionar</button></div></div></div>
      <div class="cartao"><h3 style="margin-top:0">SUBORDINAÇÃO — árvore do meu grupo (leitura; organização definida pela Administração)</h3>
        <div id="arvore">${(arvore && arvore.length) ? arvoreHTML(arvore, 0) : '<span class="vazio">sem grupos</span>'}</div></div>
    </div>
    <div id="gerGrupos" class="oculto">
      <div class="cartao"><h3 style="margin-top:0">MEU GRUPO</h3>` +
    (meus.map(g => `<div style="margin-bottom:8px">
      • <b>${esc(g.nome)}</b> <code style="background:#e8f0e8;color:#12291b;padding:1px 6px;border-radius:4px;font-weight:700">${esc(g.codigo)}</code>
      — ${g.efetivo} no efetivo · ${g.contas} conta(s)</div>`).join('') || '<span class="vazio">nenhum grupo</span>') + `</div>
      <div class="cartao"><h3 style="margin-top:0">SUBORDINAÇÃO — árvore do meu grupo (leitura)</h3>
        <div id="arvore2">${(arvore && arvore.length) ? arvoreHTML(arvore, 0) : '<span class="vazio">sem grupos</span>'}</div></div>
    </div>
    <div id="gerOperadores" class="oculto">
      <div class="cartao"><h3 style="margin-top:0">OPERADORES do meu grupo (${operadores.length})</h3>
      <div class="campo" style="margin-bottom:8px"><label>Filtrar operadores</label><input id="fOp" placeholder="buscar login…"></div>
      <div class="form-linha"><div class="campo"><label>Login do operador</label><input id="opLogin"></div>
      <div class="campo"><label>Senha (mín. 8)</label><input id="opSenha" type="password"></div>
      <div class="campo" style="align-self:end"><button class="primario" id="opGo">Criar operador</button></div></div>
      <div class="rolagem" style="margin-top:10px"><table><thead><tr><th>ID</th><th>Login</th><th>Status</th><th>Criada</th><th>Ações</th></tr></thead>
      <tbody>${operadores.map(o => `
        <tr data-login="${esc(o.login)}"><td class="num">#${o.id}</td><td><b>${esc(o.login)}</b></td><td>${o.ativo ? 'ativa' : 'desativada'}</td><td>${(o.criado_em || '').slice(0, 10)}</td>
        <td><button class="acao-linha" data-senha="${o.id}" data-login="${esc(o.login)}">senha</button>
        <button class="acao-linha" data-mv="${o.id}" data-login="${esc(o.login)}">mover</button>
        <button class="acao-linha" data-exc="${o.id}" data-login="${esc(o.login)}">excluir</button></td></tr>`).join('') || '<tr><td colspan="5"><span class="vazio">nenhum operador</span></td></tr>'}</tbody></table></div></div>
    </div>`;

  /* --- alternância de abas --- */
  const mostrarGer = qual => {
    ['pessoal', 'tags', 'grupos', 'operadores'].forEach(k => { $('#ger' + k[0].toUpperCase() + k.slice(1)).classList.toggle('oculto', k !== qual); });
    document.querySelectorAll('#abasGer button').forEach(b => b.classList.toggle('ativo', b.dataset.g === qual));
  };
  document.querySelectorAll('#abasGer button').forEach(b => b.onclick = () => mostrarGer(b.dataset.g));
  mostrarGer('pessoal');
  ligarTogglesArvore($('#app'));

  /* --- salvar militar (criar/editar) --- */
  $('#pSalvar').onclick = async () => {
    const corpo = { nome_guerra: $('#pNg').value.trim(), nome_completo: $('#pNc').value.trim(),
      setor_id: +$('#pSetor').value || null, funcao_id: +$('#pFuncao').value || null, status: $('#pStatus').value };
    if (!corpo.nome_guerra || !corpo.nome_completo) { toast('Nomes obrigatórios', 'erro'); return; }
    const id = $('#pId').value;
    if (id) await api('/api/pessoas/' + id, { method: 'PATCH', body: JSON.stringify(corpo) });
    else await api('/api/pessoas', { method: 'POST', body: JSON.stringify(corpo) });
    toast('Salvo'); viewGrupos();
  };
  /* --- aba TAGS: catálogos do grupo (setores/funções/tags) --- */
  const rotCat = { setores: 'Setores', funcoes: 'Funções', tags: 'Tags', destinos: 'Destinos' };
  const carregarCats = async () => {
    const cont = $('#catGer');
    if (!cont) return;
    let html = '';
    for (const t of ['setores', 'funcoes', 'tags', 'destinos']) {
      let lista = [];
      try { lista = await api('/api/catalogo/' + t); } catch (e) {}
      html += `<div class="cat-bloco"><b>${rotCat[t]}</b><div class="cat-itens">` +
        (lista.length ? lista.map(x => `<span class="cat-item"><small style="color:#000;font-weight:700">#${x.id}</small> ${esc(x.nome)}${x.ativo ? '' : ' <i>(inativo)</i>'}
          <button class="cat-x" data-ct="${t}" data-cid="${x.id}" title="excluir">✕</button>
          ${x.ativo ? `<button class="cat-off" data-ct="${t}" data-cid="${x.id}" title="desativar">⏸</button>` : ''}</span>`).join(' ')
          : '<span class="vazio">— vazio —</span>') + '</div></div>';
    }
    cont.innerHTML = html;
    cont.querySelectorAll('.cat-x').forEach(b => b.onclick = async () => {
      if (!confirm('Excluir este item?')) return;
      try { await api(`/api/catalogo/${b.dataset.ct}/${b.dataset.cid}`, { method: 'DELETE' }); toast('Excluído'); carregarCats(); } catch (e) {}
    });
    cont.querySelectorAll('.cat-off').forEach(b => b.onclick = async () => {
      try { await api(`/api/catalogo/${b.dataset.ct}/${b.dataset.cid}?modo=desativar`, { method: 'DELETE' }); toast('Desativado'); carregarCats(); } catch (e) {}
    });
  };
  $('#cgGo').onclick = async () => {
    const t = $('#cgT').value, nome = $('#cgN').value.trim();
    if (!nome) { toast('Informe o nome do item', 'erro'); return; }
    await api('/api/catalogo/' + t, { method: 'POST', body: JSON.stringify({ nome }) });
    toast('Item adicionado'); $('#cgN').value = ''; carregarCats();
  };
  carregarCats();
  /* --- exclusão de militar (individual + batch com checkboxes — v9.7) --- */
  const excPessoa = async (id, nome) => {
    if (!confirm(`Excluir "${nome}" do banco de pessoal? (com histórico de conferência virará inativo)`)) return;
    try {
      const r = await api(`/api/pessoas/${id}`, { method: 'DELETE' });
      toast(r.desativado ? 'Desativado (histórico preservado)' : 'Excluído');
      viewGrupos();
    } catch (e) {}
  };
  document.querySelectorAll('[data-excP]').forEach(b => b.onclick = () => excPessoa(b.dataset.excP, b.dataset.nome));
  const selCount = () => document.querySelectorAll('.chkP:checked').length;
  const refreshSel = () => {
    const n = selCount();
    $('#nSel').textContent = n;
    $('#nSel2').textContent = n;
    $('#btExcLote').disabled = n === 0;
    $('#btEditLote').disabled = n === 0;
  };
  document.querySelectorAll('.chkP').forEach(ch => ch.onchange = refreshSel);
  $('#chkTodosP').onchange = () => {
    document.querySelectorAll('.chkP').forEach(ch => { ch.checked = $('#chkTodosP').checked; });
    refreshSel();
  };
  $('#btExcLote').onclick = async () => {
    const ids = [...document.querySelectorAll('.chkP:checked')].map(c => +c.dataset.id);
    if (!ids.length) return;
    if (!confirm(`Excluir ${ids.length} militar(es)? (com histórico de conferência virarão inativos)`)) return;
    let ok = 0, des = 0, falha = 0;
    for (const id of ids) {
      try {
        const r = await api(`/api/pessoas/${id}`, { method: 'DELETE' });
        r.desativado ? des++ : ok++;
      } catch (e) { falha++; }
    }
    toast(`Excluídos: ${ok} · desativados: ${des}${falha ? ' · falhas: ' + falha : ''}`, falha && !ok ? 'erro' : 'ok');
    if (ok || des) viewGrupos();
  };
  /* --- edição em lote: modal único aplica setor/função/status aos selecionados (v9.11.2) --- */
  $('#btEditLote').onclick = () => {
    const sel = [...document.querySelectorAll('.chkP:checked')].map(c => +c.dataset.id);
    if (!sel.length) { toast('Selecione ao menos um militar', 'erro'); return; }
    const div = document.createElement('div');
    div.className = 'modal-mask';
    div.innerHTML = `<div class="modal"><h3>Editar em lote — ${sel.length} militar(es)</h3>
      <p style="color:var(--tx2);font-size:12px;margin:4px 0">Campos em <b>manter</b> não são alterados. Aplica a todos os selecionados.</p>
      <div class="form-linha">
        <div class="campo"><label>Setor</label><select id="lSetor"><option value="">(manter)</option>${optSetores.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
        <div class="campo"><label>Função</label><select id="lFuncao"><option value="">(manter)</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
        <div class="campo"><label>Status</label><select id="lStatus"><option value="">(manter)</option><option value="ativo">ativo</option><option value="inativo">inativo</option></select></div></div>
      <div class="modal-acoes"><button class="fantasma" id="lX">Cancelar</button>
      <button class="primario" id="lGo">Aplicar a ${sel.length}</button></div></div>`;
    document.body.appendChild(div);
    div.querySelector('#lX').onclick = () => div.remove();
    div.onclick = ev => { if (ev.target === div) div.remove(); };
    div.querySelector('#lGo').onclick = async () => {
      const sid = div.querySelector('#lSetor').value;
      const fid = div.querySelector('#lFuncao').value;
      const st = div.querySelector('#lStatus').value;
      if (!sid && !fid && !st) { toast('Nada para alterar — todos em manter', 'erro'); return; }
      let ok = 0, falha = 0;
      for (const id of sel) {
        const p = (pessoas.pessoas || []).find(x => x.id === id);
        if (!p) { falha++; continue; }
        const corpo = { nome_guerra: p.nome_guerra, nome_completo: p.nome_completo,
          setor_id: sid ? +sid : p.setor_id, funcao_id: fid ? +fid : p.funcao_id,
          status: st || p.status };
        try { await api('/api/pessoas/' + id, { method: 'PATCH', body: JSON.stringify(corpo) }); ok++; }
        catch (e) { falha++; }
      }
      toast(`Aplicado a ${ok} militar(es)${falha ? ' · falhas: ' + falha : ''}`, falha && !ok ? 'erro' : 'ok');
      div.remove();
      if (ok) viewGrupos();
    };
  };
  /* --- adição em lote --- */
  $('#csvGo').onclick = async () => {
    const linhas = $('#csv').value.split('\n').map(l => l.trim()).filter(Boolean);
    if (!linhas.length) { toast('Cole ao menos uma linha', 'erro'); return; }
    let ok = 0, falha = 0;
    for (const l of linhas) {
      const [ng, nc, st, fn] = l.split(';').map(x => (x || '').trim());
      if (!ng || !nc) { falha++; continue; }
      const sid = (optSetores.find(s => s.nome.toLowerCase() === (st || '').toLowerCase()) || {}).id || null;
      const fid = (optFuncoes.find(s => s.nome.toLowerCase() === (fn || '').toLowerCase()) || {}).id || null;
      try { await api('/api/pessoas', { method: 'POST', body: JSON.stringify({ nome_guerra: ng, nome_completo: nc, setor_id: sid, funcao_id: fid, status: 'ativo' }) }); ok++; }
      catch (e) { falha++; }
    }
    toast(`${ok} importado(s)${falha ? ' · ' + falha + ' linha(s) com falha' : ''}`, falha && !ok ? 'erro' : 'ok');
    if (ok) viewGrupos();
  };
  /* --- editar militar (clica na linha) --- */
  document.querySelectorAll('#tabP tr[data-p]').forEach(tr => {
    tr.querySelector('[data-edit]').onclick = ev => {
      ev.stopPropagation();
      const p = JSON.parse(tr.dataset.p);
      $('#pId').value = p.id; $('#pNg').value = p.nome_guerra; $('#pNc').value = p.nome_completo;
      $('#pSetor').value = p.setor_id || ''; $('#pFuncao').value = p.funcao_id || ''; $('#pStatus').value = p.status;
      window.scrollTo({ top: 0, behavior: 'smooth' });
    };
  });
  /* --- operadores: criar, senha, mover, excluir + filtro --- */
  $('#opGo').onclick = async () => {
    const login = $('#opLogin').value.trim(), senha = $('#opSenha').value;
    if (!login || senha.length < 8) { toast('Login e senha (mín. 8) obrigatórios', 'erro'); return; }
    await api('/api/usuarios', { method: 'POST', body: JSON.stringify({ login, senha, papel: 'operador' }) });
    toast('Operador criado'); viewGrupos();
  };
  $('#fOp').oninput = () => {
    const q = $('#fOp').value.trim().toLowerCase();
    document.querySelectorAll('#gerOperadores tbody tr[data-login]').forEach(tr => {
      tr.style.display = !q || tr.dataset.login.toLowerCase().includes(q) ? '' : 'none';
    });
  };
  const optsMoverGer = `<option value="">— destino (dentro da sua hierarquia) —</option>` +
    grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');
  document.querySelectorAll('#gerOperadores [data-mv]').forEach(b => b.onclick = () => {
    const div = document.createElement('div');
    div.className = 'modal-mask';
    div.innerHTML = `<div class="modal"><h3>Mover conta — ${b.dataset.login}</h3>
      <div class="campo"><label>Grupo de destino</label><select id="mG">${optsMoverGer}</select></div>
      <p style="color:var(--tx2);font-size:12px">Permitido apenas entre o seu grupo e seus subordinados.</p>
      <div class="modal-acoes"><button class="fantasma" id="mX">Cancelar</button>
      <button class="primario" id="mGo">Mover</button></div></div>`;
    document.body.appendChild(div);
    div.querySelector('#mX').onclick = () => div.remove();
    div.onclick = ev => { if (ev.target === div) div.remove(); };
    div.querySelector('#mGo').onclick = async () => {
      const g = div.querySelector('#mG').value;
      if (!g) { toast('Escolha o grupo de destino', 'erro'); return; }
      try {
        await api(`/api/usuarios/${b.dataset.mv}/mover`, { method: 'PATCH', body: JSON.stringify({ grupo_id: +g }) });
        toast('Conta movida'); div.remove(); viewGrupos();
      } catch (e) {}
    };
  });
  document.querySelectorAll('[data-senha]').forEach(b => b.onclick = () => {
    const div = document.createElement('div');
    div.className = 'modal-mask';
    div.innerHTML = `<div class="modal"><h3>Redefinir senha — ${b.dataset.login}</h3>
      <div class="campo"><label>Nova senha (mín. 8)</label><input type="password" id="rN"></div>
      <div class="modal-acoes"><button class="fantasma" id="rX">Cancelar</button>
      <button class="primario" id="rGo">Redefinir</button></div></div>`;
    document.body.appendChild(div);
    div.querySelector('#rX').onclick = () => div.remove();
    div.onclick = ev => { if (ev.target === div) div.remove(); };
    div.querySelector('#rGo').onclick = async () => {
      const n = div.querySelector('#rN').value;
      if (n.length < 8) { toast('Mínimo 8 caracteres', 'erro'); return; }
      await api(`/api/usuarios/${b.dataset.senha}/senha`, { method: 'POST', body: JSON.stringify({ senha: n }) });
      toast('Senha redefinida'); div.remove();
    };
  });
  document.querySelectorAll('[data-exc]').forEach(b => b.onclick = async () => {
    if (!confirm(`Excluir a conta "${b.dataset.login}"?`)) return;
    try {
      const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
      toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
      viewGrupos();
    } catch (e) {}
  });
}

/* ---------- LISTA DE CONFERÊNCIAS ---------- */
function fmtHora(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return isNaN(d) ? '—' : d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
}
async function viewConferencias() {
  // v9.4: aba consolidada — o histórico vive dentro da própria Conferência (#/hoje)
  location.hash = '#/hoje';
}

/* ---------- RELATÓRIOS ---------- */
function dataLocal(d) { return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); }
function semanaDe(isoDia) {
  const d = new Date(isoDia + 'T12:00:00');
  const dow = (d.getDay() + 6) % 7;
  const ini = new Date(d.getTime() - dow * 864e5);
  const fim = new Date(ini.getTime() + 6 * 864e5);
  return [dataLocal(ini), dataLocal(fim)];
}
function fimDoMes(ym) {
  const [y, m] = ym.split('-').map(Number);
  return dataLocal(new Date(y, m, 0));
}
function renderRelatorio(b, titulo) {
  const res = `<div class="resumo">
    <div class="caixa"><b>${b.convocacoes}</b><span>conferências</span></div>
    <div class="caixa"><b>${b.efetivo_ativo}</b><span>efetivo</span></div>
    <div class="caixa"><b>${b.presentes}</b><span>presentes</span></div>
    <div class="caixa"><b>${b.atrasos}</b><span>atrasos</span></div>
    <div class="caixa"><b>${b.falta}</b><span>faltas</span></div>
    <div class="caixa"><b>${b.justificadas}</b><span>justificadas</span></div>
    <div class="caixa"><b>${b.total_faltas ?? ((b.falta || 0) + (b.justificadas || 0))}</b><span>faltas tot. (J+NJ)</span></div>
    <div class="caixa"><b>${b.pct_geral}%</b><span>% válidas (P+A)</span></div>
    <div class="caixa" style="border-color:var(--verde)"><b>${b.pct_pronto}%</b><span>ef. pronto</span></div></div>`;
  const linhas = (b.pessoas || []).map(p =>
    `<tr><td class="num">#${p.antiguidade ?? ''}</td><td><b>${esc(p.nome_guerra)}</b></td>
     <td>${esc(p.funcao || '—')}${p.funcao_id ? ` <small style="color:#000">#${p.funcao_id}</small>` : ''}</td>
     <td>${esc(p.setor)}</td><td>${esc(p.grupo || '—')}</td><td class="num">${p.presencas}</td>
     <td class="num">${p.atrasos}</td><td class="num">${p.faltas}</td><td class="num">${p.justificadas}</td></tr>`).join('');
  const forms = (b.formaturas || []).map(f =>
    `<tr><td>${f.data}</td><td>${esc(f.tipo)}</td><td>${esc(f.hora || '—')}</td><td>${pill(f.status === 'fechada' ? 'presente' : 'atraso')} ${f.status}</td>
     <td class="num">${f.presentes}</td><td class="num">${f.faltas}</td></tr>`).join('');
  return `${res}<div class="cartao"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
    <h3 style="margin:0">${titulo} — ${b.convocacoes} conferências no período</h3>
    <a href="/api/relatorio.pdf?de=${b.De}&ate=${b.Ate}${$('#fGrupo') && $('#fGrupo').value ? `&grupo=${$('#fGrupo').value}` : ''}" target="_blank"><button class="primario">ABRIR PDF</button></a></div>
    ${forms ? `<div class="rolagem" style="margin-bottom:12px"><table><thead><tr><th>Data</th><th>Tipo</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead><tbody>${forms}</tbody></table></div>` : ''}
    <div class="rolagem"><table><thead><tr><th class="num">Antig.</th><th>Nome</th><th>Função</th><th>Setor</th><th>Grupo</th><th class="num">Pres.</th><th class="num">Atraso</th>
    <th class="num">Falta</th><th class="num">Just.</th></tr></thead><tbody>${linhas}</tbody></table></div></div>`;
}
async function viewRelatorios() {
  navAtiva('#/relatorios');
  const hoje = new Date();
  const [seg, dom] = semanaDe(dataLocal(hoje));
  let gSel = '';
  try {
    const gs = await api('/api/grupos');
    if (ME.papel === 'admin') {
      gSel = `<div class="campo"><label>Grupo</label><select id="fGrupo"><option value="">Todos</option>` +
        gs.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('') + `</select></div>`;
    } else {
      // v9.4 (ordem Tenente): escopos = Meu grupo + Subordinados / Meu Grupo / cada subordinado
      const meuG = gs.find(g => g.id === ME.grupo_id);
      const subs = (meuG && meuG.subordinados_ids) || [];
      if (subs.length) {
        gSel = `<div class="campo"><label>Escopo</label><select id="fGrupo">
          <option value="">Meu grupo + subordinados</option>
          <option value="${ME.grupo_id}">Meu grupo (somente)</option>` +
          subs.map((id, i) => `<option value="${id}">Somente: ${esc(meuG.subordinados[i])}</option>`).join('') +
          `</select></div>`;
      }
    }
  } catch (e) {}
  $('#app').innerHTML = `<h2>Relatórios</h2>
    <div class="cartao">
      ${gSel ? `<div class="form-linha" style="margin-bottom:8px">${gSel}</div>` : ''}
      <div class="abas" id="modos">
        <button data-m="dia" class="ativo">Dia</button>
        <button data-m="semana">Semana</button>
        <button data-m="ano">Ano</button>
        <button data-m="livre">Período livre</button></div>
      <div class="form-linha" style="grid-template-columns:1fr auto;align-items:end">
        <div id="entrada"></div>
        <button class="primario" id="btGerar" style="min-height:44px">Gerar</button></div></div>
    <div id="saida"><div class="carregando">Gerando dia atual…</div></div>`;
  let modo = 'dia';
  const entrada = () => {
    const el = $('#entrada');
    if (modo === 'dia') el.innerHTML = `<div class="campo"><label>Dia específico</label><input type="date" id="fDia" value="${dataLocal(hoje)}"></div>`;
    else if (modo === 'semana') el.innerHTML = `<div class="campo"><label>Semana (escolha qualquer dia dela)</label><input type="date" id="fDia" value="${dataLocal(hoje)}"></div>`;
    else if (modo === 'ano') el.innerHTML = `<div class="campo"><label>Ano</label><input type="number" id="fAno" min="2000" max="2100" value="${hoje.getFullYear()}"></div>`;
    else el.innerHTML = `<div class="campo"><label>De — até</label><div style="display:flex;gap:6px"><input type="date" id="fDe" value="${dataLocal(new Date(Date.now() - 29 * 864e5))}"><input type="date" id="fAte" value="${dataLocal(hoje)}"></div></div>`;
  };
  const periodos = () => {
    if (modo === 'dia') { const d = $('#fDia').value; return [d, d]; }
    if (modo === 'semana') return semanaDe($('#fDia').value);
    if (modo === 'ano') { const y = $('#fAno').value; return [y + '-01-01', y + '-12-31']; }
    return [$('#fDe').value, $('#fAte').value];
  };
  const rotulos = { dia: 'Dia', semana: 'Semana', ano: 'Ano', livre: 'Período' };
  const gerar = async () => {
    const [de, ate] = periodos();
    if (!de || !ate) { toast('Escolha a data', 'erro'); return; }
    $('#saida').innerHTML = '<div class="carregando">Gerando…</div>';
    try {
      const gq = $('#fGrupo') && $('#fGrupo').value ? `&grupo=${$('#fGrupo').value}` : '';
      const b = await api(`/api/relatorio?de=${de}&ate=${ate}${gq}`);
      b.De = de; b.Ate = ate;
      $('#saida').innerHTML = renderRelatorio(b, `${rotulos[modo]} ${de} a ${ate}`);
    } catch (e) { $('#saida').innerHTML = ''; }
  };
  entrada();
  gerar();
  document.querySelectorAll('#modos button').forEach(b => b.onclick = () => {
    modo = b.dataset.m;
    document.querySelectorAll('#modos button').forEach(x => x.classList.toggle('ativo', x === b));
    entrada();
  });
  $('#btGerar').onclick = gerar;
}

/* ---------- DASHBOARD (leitura; admin não tem ferramentas de conferência — R11) ---------- */
async function viewDashboard() {
  navAtiva('#/hoje');
  const hoje = new Date();
  const [seg] = semanaDe(dataLocal(hoje));
  const gq = ME.papel === 'admin' ? '' : '';
  const b = await api(`/api/relatorio?de=${seg}&ate=${dataLocal(hoje)}${gq}`);
  b.De = seg; b.Ate = dataLocal(hoje);
  $('#app').innerHTML = `<h2>Dashboard</h2>
    <p style="color:var(--tx2);font-size:13px">Panorama da semana — leitura. A conferência de pessoal fica com o gerente e o operador.</p>` +
    renderRelatorio(b, 'Semana em curso');
}

/* ---------- ABA ADMIN (só admin — R7/R11): Usuários · Grupos · Backup ---------- */
let abaAdmin = 'usuarios';
async function viewAdmin() {
  navAtiva('#/admin');
  const abas = ['usuarios', 'grupos', 'backup'];
  $('#app').innerHTML = `<h2>Administração</h2>
    <div class="abas">${abas.map(a => `<button data-a="${a}" class="${abaAdmin === a ? 'ativo' : ''}">${a}</button>`).join('')}</div>
    <div id="adm"><div class="carregando">…</div></div>`;
  document.querySelectorAll('.abas button').forEach(b => b.onclick = () => { abaAdmin = b.dataset.a; viewAdmin(); });
  if (abaAdmin === 'usuarios') await admUsuarios();
  else if (abaAdmin === 'grupos') await admGrupos();
  else admBackup();
}

/* --- Usuários (admin): SOMENTE tabela — mover · senha · excluir (v9.5: sem criar conta, sem admin) + filtros --- */
async function admUsuarios() {
  const [lista, grupos] = await Promise.all([api('/api/usuarios'), api('/api/grupos')]);
  const gerenciaveis = lista.filter(u => u.papel !== 'admin');
  const nomeGrupo = gid => (grupos.find(g => g.id === gid) || {}).nome || '—';
  const optsGrupos = `<option value="">— destino —</option>` + grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');
  $('#adm').innerHTML = `<div class="cartao">
    <div class="form-linha" style="margin-bottom:8px">
      <div class="campo" style="flex:1"><label>Filtrar por login</label><input id="fULogin" placeholder="buscar login…"></div>
      <div class="campo"><label>Grupo</label><select id="fUGrupo"><option value="">todos</option>${grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('')}</select></div>
      <div class="campo"><label>Status</label><select id="fUAtivo"><option value="">todas</option><option value="1">ativa</option><option value="0">desativada</option></select></div>
    </div>
    <div class="rolagem"><table id="tabU"><thead><tr><th>ID</th><th>Login</th><th>Papel</th><th>Grupo</th><th>Status</th><th>Criada</th><th>Ações</th></tr></thead>
    <tbody>${gerenciaveis.map(u => `<tr data-login="${esc(u.login)}" data-gid="${u.grupo_id || 0}" data-ativo="${u.ativo ? 1 : 0}">
      <td class="num">#${u.id}</td><td><b>${esc(u.login)}</b></td><td>${rotuloPapel(u.papel)}</td><td>${esc(nomeGrupo(u.grupo_id))}</td>
      <td>${u.ativo ? 'ativa' : 'desativada'}</td><td>${(u.criado_em || '').slice(0, 10)}</td>
      <td><button class="acao-linha" data-id="${u.id}" data-login="${esc(u.login)}">🔑 senha</button>
      <button class="acao-linha" data-mv="${u.id}" data-login="${esc(u.login)}" data-papel="${u.papel}">➡ mover</button>
      <button class="acao-linha" data-exc="${u.id}" data-login="${esc(u.login)}">excluir</button></td></tr>`).join('')}</tbody></table></div>
    <p style="color:var(--tx2);font-size:12px;margin-top:8px">Mover: admin transfere entre grupos quaisquer; gerente só dentro da própria hierarquia. Contas com histórico são desativadas ao excluir, o resto é removido.</p></div>`;
  // filtros
  const aplicarFiltro = () => {
    const q = $('#fULogin').value.trim().toLowerCase();
    const gid = $('#fUGrupo').value;
    const at = $('#fUAtivo').value;
    document.querySelectorAll('#tabU tbody tr').forEach(tr => {
      const ok = (!q || tr.dataset.login.toLowerCase().includes(q)) &&
                 (!gid || tr.dataset.gid === gid) &&
                 (at === '' || tr.dataset.ativo === at);
      tr.style.display = ok ? '' : 'none';
    });
  };
  $('#fULogin').oninput = aplicarFiltro;
  $('#fUGrupo').onchange = aplicarFiltro;
  $('#fUAtivo').onchange = aplicarFiltro;
  document.querySelectorAll('#adm button[data-id]').forEach(b => b.onclick = () => {
    const div = document.createElement('div');
    div.className = 'modal-mask';
    div.innerHTML = `<div class="modal"><h3>Redefinir senha — ${b.dataset.login}</h3>
      <div class="campo"><label>Nova senha (mín. 8)</label><input type="password" id="rN"></div>
      <div class="campo"><label>Confirmar</label><input type="password" id="rC"></div>
      <div class="modal-acoes"><button class="fantasma" id="rX">Cancelar</button>
      <button class="primario" id="rGo">Redefinir</button></div></div>`;
    document.body.appendChild(div);
    div.querySelector('#rX').onclick = () => div.remove();
    div.onclick = ev => { if (ev.target === div) div.remove(); };
    div.querySelector('#rGo').onclick = async () => {
      const n = div.querySelector('#rN').value, c = div.querySelector('#rC').value;
      if (n.length < 8) { toast('Mínimo 8 caracteres', 'erro'); return; }
      if (n !== c) { toast('Senhas não conferem', 'erro'); return; }
      await api(`/api/usuarios/${b.dataset.id}/senha`, { method: 'POST', body: JSON.stringify({ senha: n }) });
      toast('Senha redefinida'); div.remove();
    };
  });
  document.querySelectorAll('#adm button[data-mv]').forEach(b => b.onclick = () => {
    const div = document.createElement('div');
    div.className = 'modal-mask';
    div.innerHTML = `<div class="modal"><h3>Mover conta — ${b.dataset.login} (${rotuloPapel(b.dataset.papel)})</h3>
      <div class="campo"><label>Grupo de destino</label><select id="mG">${optsGrupos}</select></div>
      <p style="color:var(--tx2);font-size:12px">Se for o único gerente do grupo de origem, virará operador no destino e o operador mais antigo assumirá o grupo.</p>
      <div class="modal-acoes"><button class="fantasma" id="mX">Cancelar</button>
      <button class="primario" id="mGo">Mover</button></div></div>`;
    document.body.appendChild(div);
    div.querySelector('#mX').onclick = () => div.remove();
    div.onclick = ev => { if (ev.target === div) div.remove(); };
    div.querySelector('#mGo').onclick = async () => {
      const g = div.querySelector('#mG').value;
      if (!g) { toast('Escolha o grupo de destino', 'erro'); return; }
      try {
        const r = await api(`/api/usuarios/${b.dataset.mv}/mover`, { method: 'PATCH', body: JSON.stringify({ grupo_id: +g }) });
        toast(r.rebaixado ? 'Movido e rebaixado a operador (grupo de origem recebeu novo gerente)' : 'Conta movida');
        div.remove(); admUsuarios();
      } catch (e) {}
    };
  });
  document.querySelectorAll('#adm button[data-exc]').forEach(b => b.onclick = async () => {
    if (!confirm(`Excluir a conta "${b.dataset.login}"?`)) return;
    try {
      const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
      toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
      admUsuarios();
    } catch (e) {}
  });
}

/* --- Grupos: criar grupo+gerente, trocar gerente, subordinação (R7/R8) + árvore NESTED (v9.4) --- */
async function admGrupos() {
  const [grupos, contas, arvore] = await Promise.all([api('/api/grupos'), api('/api/usuarios'), api('/api/grupos/arvore')]);
  const gerenteDe = {};
  contas.filter(c => c.papel === 'gerente' && c.ativo && c.grupo_id).forEach(c => { gerenteDe[c.grupo_id] = c.login; });
  const optsContas = gid => contas.filter(c => c.grupo_id === gid && c.papel !== 'admin' && c.ativo)
    .map(c => `<option value="${esc(c.login)}">${esc(c.login)} (${rotuloPapel(c.papel)})</option>`).join('');
  let linhas = grupos.map(g => {
    const sups = g.superiores_ids || [];
    return `<div class="cartao" data-g="${g.id}" style="margin-bottom:10px">
      <h3 style="margin:0 0 6px">${esc(g.nome)} <code style="background:#e8f0e8;color:#12291b;padding:1px 6px;border-radius:4px;font-weight:700">${esc(g.codigo)}</code></h3>
      <p style="margin:0 0 6px;color:var(--tx2);font-size:13px">gerente: <b id="ger-${g.id}">${esc(gerenteDe[g.id] || '— sem gerente —')}</b> · ${g.efetivo} no efetivo · ${g.contas} conta(s)</p>
      <div class="form-linha">
        <div class="campo"><label>Trocar gerente — promover conta</label><select id="tg-${g.id}">${optsContas(g.id)}</select></div>
        <div class="campo"><label>Subordinado a</label><select id="sub-${g.id}">
          <option value="">— sem superior —</option>
          ${grupos.filter(x => x.id !== g.id).map(x => `<option value="${x.id}" ${sups.includes(x.id) ? 'selected' : ''}>${esc(x.nome)}</option>`).join('')}
        </select></div>
      </div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button class="primario" data-trocar="${g.id}">Trocar gerente</button>
        <button class="acao-linha" data-remover="${g.id}">Remover subordinação</button>
        <button class="perigo" data-excluir="${g.id}" data-nome="${esc(g.nome)}">Excluir grupo</button>
      </div></div>`;
  }).join('');
  const noHTML = (n, nivel) => {
    const temFilhos = n.filhos && n.filhos.length;
    const id = 'admNo' + n.id + '_' + Math.random().toString(36).slice(2, 6);
    const rec = n.efetivo_total !== undefined && n.efetivo_total !== n.efetivo;
    const efetTxt = rec
      ? `<small style="color:#000"> · efetivo total: <b style="color:#000">${n.efetivo_total}</b> <small>(próprio ${n.efetivo})</small></small>`
      : `<small style="color:#000"> · ${n.efetivo} no efetivo</small>`;
    return `<div style="margin-left:${nivel * 24}px;padding:6px 10px;border-left:3px solid var(--verde);margin-bottom:5px;background:#f4f8f4;border-radius:0 6px 6px 0">
      ${temFilhos ? `<span data-tgl="${id}" style="cursor:pointer;font-weight:700;color:#000">▸ </span>` : ''}
      <b style="color:#000;cursor:${temFilhos ? 'pointer' : 'default'}" data-tgl="${temFilhos ? id : ''}">${esc(n.nome)}</b>
      <small style="color:#000">#${n.id}</small>
      <code style="background:#e8f0e8;color:#12291b;padding:1px 6px;border-radius:4px;font-weight:700;font-size:11px">${esc(n.codigo)}</code>
      <small style="color:#000"> · gerente: <b style="color:#000">${esc(gerenteDe[n.id] || '—')}</b> · ${efetTxt} · ${n.contas} conta(s)</small>
      ${temFilhos ? `<div id="${id}" class="oculto" style="margin-top:4px">${n.filhos.map(f => noHTML(f, nivel + 1)).join('')}</div>` : ''}
    </div>`;
  };
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Hierarquia dos grupos (subordinados indentados)</h3>
      <div class="campo" style="margin-bottom:8px"><label>Filtrar grupos</label><input id="fGNome" placeholder="buscar grupo…"></div>
      <div id="arvoreAdm">${(arvore && arvore.length) ? arvore.map(n => noHTML(n, 0)).join('') : '<span class="vazio">nenhum grupo</span>'}</div></div>
    <div class="cartao"><h3 style="margin-top:0">Criar grupo — nasce com gerente</h3>
    <div class="form-linha">
      <div class="campo"><label>Nome do grupo</label><input id="gNome" placeholder="ex.: 1ª Cia"></div>
      <div class="campo"><label>Login do gerente</label><input id="gLogin"></div>
      <div class="campo"><label>Senha do gerente (mín. 8)</label><input id="gSenha" type="password"></div>
      <div class="campo"><label>Nome de guerra do gerente</label><input id="gGuerra"></div></div>
    <button class="primario" id="gGo">Criar grupo</button>
    <p style="color:var(--tx2);font-size:12px">O grupo exige gerente no ato da criação — sem gerente, sem grupo.</p></div>
    <div class="cartao"><div class="campo" style="margin-bottom:8px"><label>Filtrar grupos (painel)</label><input id="fGPainel" placeholder="buscar grupo…"></div>
    ${linhas || '<span class="vazio">nenhum grupo</span>'}</div>`;
  $('#gGo').onclick = async () => {
    const nome = $('#gNome').value.trim(), login = $('#gLogin').value.trim(),
          senha = $('#gSenha').value, guerra = $('#gGuerra').value.trim();
    if (!nome || !login || senha.length < 8 || !guerra) { toast('Nome do grupo + login + senha (mín. 8) + nome de guerra do gerente', 'erro'); return; }
    const r = await api('/api/grupos', { method: 'POST', body: JSON.stringify({ nome, login, senha, nome_guerra: guerra }) });
    toast(`Grupo criado — código ${r.codigo}, gerente ${login}`); viewAdmin();
  };
  // filtros de grupo: árvore (esconde nós que não casam, mantendo ancestral visível) e painel de cartões
  const casarArvore = (n, q) => {
    const eu = n.nome.toLowerCase().includes(q) || n.codigo.toLowerCase().includes(q);
    const filhos = (n.filhos || []).map(f => casarArvore(f, q)).filter(Boolean);
    if (!eu && !filhos.length) return null;
    return { ...n, filhos };
  };
  const ligarFiltroArvore = () => {
    const render = nos => { $('#arvoreAdm').innerHTML = nos.length ? nos.map(n => noHTML(n, 0)).join('') : '<span class="vazio">nenhum grupo casa com a busca</span>'; ligarTogglesArvore($('#arvoreAdm')); };
    $('#fGNome').oninput = () => {
      const q = $('#fGNome').value.trim().toLowerCase();
      if (!q) { render(arvore || []); return; }
      // busca: mostra EXPANDIDO o caminho até o nó que casa (ancestrais visíveis)
      const mostrar = n => {
        const eu = n.nome.toLowerCase().includes(q) || n.codigo.toLowerCase().includes(q);
        const filhos = (n.filhos || []).map(f => mostrar(f)).filter(Boolean);
        if (!eu && !filhos.length) return null;
        const aberto = filhos.length > 0;
        const node = { ...n, filhos: filhos.length ? filhos : (n.filhos || []) };
        node._abrir = aberto;
        return node;
      };
      const filtrada = (arvore || []).map(mostrar).filter(Boolean);
      render(filtrada);
      // expandir automaticamente os ramos com resultado
      $('#arvoreAdm').querySelectorAll('.oculto').forEach(d => d.classList.remove('oculto'));
      $('#arvoreAdm').querySelectorAll('[data-tgl]').forEach(s => { if (s.dataset.tgl) s.textContent = '▾ '; });
    };
    $('#fGPainel').oninput = () => {
      const q = $('#fGPainel').value.trim().toLowerCase();
      document.querySelectorAll('#adm div.cartao[data-g]').forEach(c => {
        c.style.display = !q || c.querySelector('h3').textContent.toLowerCase().includes(q) ? '' : 'none';
      });
    };
  };
  ligarFiltroArvore();
  ligarTogglesArvore($('#adm'));
  document.querySelectorAll('#adm button[data-trocar]').forEach(b => b.onclick = async () => {
    const gid = b.dataset.trocar;
    const login = $('#tg-' + gid) ? $('#tg-' + gid).value : '';
    if (!login) { toast('Escolha a conta a promover', 'erro'); return; }
    try {
      await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login }) });
      toast('Gerente trocado — o anterior virou operador'); viewAdmin();
    } catch (e) {}
  });
  document.querySelectorAll('#adm button[data-remover]').forEach(b => b.onclick = async () => {
    const gid = +b.dataset.remover;
    const sel = $('#sub-' + gid);
    if (!sel || !sel.value) { toast('Este grupo não tem superior', 'erro'); return; }
    try {
      await api('/api/admin/grupos/vinculo?superior_id=' + sel.value + '&subordinado_id=' + gid, { method: 'DELETE', body: '{}' });
      toast('Subordinação removida'); viewAdmin();
    } catch (e) {}
  });
  document.querySelectorAll('#adm button[data-excluir]').forEach(b => b.onclick = () => {
    const gid = b.dataset.excluir, gnome = b.dataset.nome;
    const div = document.createElement('div');
    div.className = 'modal-mask';
    div.innerHTML = `<div class="modal"><h3>Excluir grupo — ${gnome}</h3>
      <p style="color:var(--tx2);font-size:13px;margin:6px 0">Exclusão normal: só se o grupo estiver vazio.</p>
      <div class="modal-acoes"><button class="fantasma" id="mX">Cancelar</button>
      <button class="perigo" id="mGo">Tentar exclusão normal</button></div>
      <div style="border-top:1px solid #ccc;margin-top:12px;padding-top:10px">
        <p style="color:#a00;font-size:13px;margin:4px 0"><b>Exclusão FORÇADA</b> — remove o grupo INTEIRO mesmo com contas e pessoal (tudo é apagado). Grupos com histórico de conferências são sempre preservados. Requer sua senha de admin.</p>
        <div class="campo"><label>Senha de admin</label><input type="password" id="fSenha"></div>
        <div class="modal-acoes"><button class="perigo" id="mForce">Excluir forçadamente</button></div>
      </div></div>`;
    document.body.appendChild(div);
    div.querySelector('#mX').onclick = () => div.remove();
    div.onclick = ev => { if (ev.target === div) div.remove(); };
    div.querySelector('#mGo').onclick = async () => {
      try {
        await api(`/api/grupos/${gid}`, { method: 'DELETE' });
        toast('Grupo excluído'); div.remove(); viewAdmin();
      } catch (e) {}
    };
    div.querySelector('#mForce').onclick = async () => {
      const senha = div.querySelector('#fSenha').value;
      if (!senha) { toast('Digite a senha de admin', 'erro'); return; }
      try {
        const r = await api(`/api/grupos/${gid}?forcar=1`, { method: 'DELETE', body: JSON.stringify({ senha }) });
        toast(`Grupo excluído forçadamente — ${r.contas_removidas} conta(s), ${r.pessoas_removidas} pessoa(s) removidas`);
        div.remove(); viewAdmin();
      } catch (e) {}
    };
  });
  document.querySelectorAll('#adm select[id^="sub-"]').forEach(sel => sel.onchange = async () => {
    const gid = sel.id.split('-')[1];
    if (!sel.value) { return; } // remover é pelo botão
    try {
      await api('/api/admin/grupos/vinculo', { method: 'POST', body: JSON.stringify({ superior_id: +sel.value, subordinado_id: +gid }) });
      toast('Subordinação gravada'); viewAdmin();
    } catch (e) {}
  });
}

/* --- Backup: gerar, baixar e IMPORTAR (R9) --- */
function admBackup() {
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Backup</h3>
    <p style="color:var(--tx2);font-size:13px">O SCI faz backup automático a cada conferência fechada e no boot
    (<code>VACUUM INTO</code> + SHA-256 + MANIFEST). Aqui você força uma cópia agora ou restaura um arquivo .db.</p>
    <button class="primario" id="bkGo">Fazer backup agora</button>
    <pre id="bkOut" style="margin-top:10px;color:var(--tx2);font-size:12px"></pre></div>
    <div class="cartao"><h3 style="margin-top:0">Importar backup (.db)</h3>
    <p style="color:var(--tx2);font-size:13px">Valida o arquivo (SQLite + integridade + versão de schema), grava um backup de segurança do estado atual e troca o banco. Arquivo inválido é rejeitado sem tocar em nada.</p>
    <input type="file" id="bkArq" accept=".db">
    <button class="primario" id="bkImp" style="margin-top:8px">Importar</button>
    <pre id="bkImpOut" style="margin-top:10px;color:var(--tx2);font-size:12px"></pre></div>`;
  $('#bkGo').onclick = async () => {
    const r = await api('/api/backup', { method: 'POST', body: '{}' });
    $('#bkOut').textContent = `${r.arquivo}\nsha256: ${r.sha256}\n→ download iniciado no navegador`;
    toast('Backup gerado — download iniciado');
    window.location.href = '/api/backup/download?nome=' + encodeURIComponent(r.arquivo.split('/').pop());
  };
  $('#bkImp').onclick = async () => {
    const f = $('#bkArq').files[0];
    if (!f) { toast('Escolha o arquivo .db', 'erro'); return; }
    if (!confirm(`Importar "${f.name}"? O banco atual será substituído (antes, um backup de segurança é gravado).`)) return;
    const fd = new FormData();
    fd.append('arquivo', f);
    try {
      const r = await fetch('/api/backup/importar', { method: 'POST', headers: { 'X-SCI': '1' }, body: fd });
      const j = await r.json();
      if (!r.ok) { toast(j.erro || 'Falha na importação', 'erro'); $('#bkImpOut').textContent = 'REJEITADO: ' + (j.erro || r.status); return; }
      $('#bkImpOut').textContent = `Importado (schema ${j.schema}). Backup de segurança: ${j.seguranca}`;
      toast('Backup importado — entre novamente com as credenciais do arquivo importado');
      ME = null;
      location.hash = '#/login';
      rotear();
    } catch (e) { $('#bkImpOut').textContent = 'falha de rede'; }
  };
}

/* ---------- casca ---------- */
function navAtiva(h) { document.querySelectorAll('#nav a').forEach(a => a.classList.toggle('ativo', a.hash === h)); }
function montarNav() {
  $('#topbar').classList.remove('oculto');
  $('#quem').textContent = ME ? ME.login + ' · ' + rotuloPapel(ME.papel) : '';
  // R11: admin = DASHBOARD - ADMIN - RELATÓRIOS; sem abas de conferência
  // v9.4: gerente/operador = Conferência (única, com histórico) + Gerenciar + Relatórios
  let itens;
  if (ME && ME.papel === 'admin') {
    itens = [['#/hoje', 'Dashboard'], ['#/admin', 'Admin'], ['#/relatorios', 'Relatórios']];
  } else {
    itens = [['#/hoje', 'Conferência'], ['#/relatorios', 'Relatórios']];
    if (ME && ME.papel === 'gerente') itens.push(['#/grupos', 'Gerenciar']);
  }
  $('#nav').innerHTML = itens.map(([h, t]) => `<a href="${h}">${t}</a>`).join('');
}
function ligarMenuUsuario() { // R13 + v9.4: Meu usuário dentro do dropdown, acima de Mudar senha (admin não tem perfil — R3)
  const menu = $('#menuUsuarioItens');
  $('#btPerfil').classList.toggle('oculto', ME && ME.papel === 'admin');
  $('#quem').onclick = ev => { ev.stopPropagation(); menu.classList.toggle('oculto'); };
  document.addEventListener('click', ev => { if (!menu.classList.contains('oculto') && !$('#menuUsuario').contains(ev.target)) menu.classList.add('oculto'); });
  $('#btPerfil').onclick = () => { menu.classList.add('oculto'); location.hash = '#/perfil'; rotear(); };
  $('#btSenha').onclick = () => { menu.classList.add('oculto'); modalSenha(); };
  $('#btnSair').onclick = async () => {
    menu.classList.add('oculto');
    try { await api('/api/logout', { method: 'POST', body: '{}' }); } catch (e) {}
    ME = null; location.hash = '#/login'; rotear();
  };
}
function modalSenha() {
  const div = document.createElement('div');
  div.className = 'modal-mask';
  div.innerHTML = `
    <div class="modal">
      <h3>Alterar minha senha</h3>
      <div class="campo"><label>Senha atual</label><input type="password" id="mA" autocomplete="current-password"></div>
      <div class="campo"><label>Nova senha (mín. 8)</label><input type="password" id="mN" autocomplete="new-password"></div>
      <div class="campo"><label>Confirmar nova senha</label><input type="password" id="mC" autocomplete="new-password"></div>
      <div class="modal-acoes">
        <button class="fantasma" id="mCancel">Cancelar</button>
        <button class="primario" id="mGo">Salvar</button>
      </div>
    </div>`;
  document.body.appendChild(div);
  const fechar = () => div.remove();
  div.onclick = e => { if (e.target === div) fechar(); };
  div.querySelector('#mCancel').onclick = fechar;
  div.querySelector('#mGo').onclick = async () => {
    const a = div.querySelector('#mA').value, n = div.querySelector('#mN').value, c = div.querySelector('#mC').value;
    if (!a || !n) { toast('Preencha a senha atual e a nova', 'erro'); return; }
    if (n.length < 8) { toast('Nova senha: mínimo 8 caracteres', 'erro'); return; }
    if (n !== c) { toast('As novas senhas não conferem', 'erro'); return; }
    try {
      await api('/api/senha', { method: 'POST', body: JSON.stringify({ atual: a, nova: n }) });
      toast('Senha alterada'); fechar();
    } catch (e) {}
  };
  div.addEventListener('keydown', e => { if (e.key === 'Escape') fechar(); });
  div.querySelector('#mA').focus();
}
async function rotear() {
  const h = location.hash || '#/hoje';
  if (!ME) { viewLogin(); return; }
  montarNav();
  if (ME.papel === 'admin') {
    if (h === '#/hoje') viewDashboard();
    else if (h === '#/admin') viewAdmin();
    else if (h === '#/relatorios' || h === '#/semana') viewRelatorios();
    else location.hash = '#/admin'; // homepage do admin = aba ADMIN
    return;
  }
  if (h === '#/hoje') viewHoje();
  else if (h === '#/conferencias') { location.hash = '#/hoje'; } // v9.4: histórico mora na própria Conferência
  else if (h === '#/grupos' && ME.papel === 'gerente') viewGrupos();
  else if (h === '#/perfil') viewPerfil();
  else if (h === '#/relatorios' || h === '#/semana') viewRelatorios();
  else { location.hash = '#/hoje'; }
}
window.onhashchange = rotear;
(async function init() {
  ligarMenuUsuario();
  try { const r = await api('/api/me'); ME = r.usuario; } catch (e) {}
  if (!location.hash) location.hash = ME && ME.papel === 'admin' ? '#/admin' : '#/hoje';
  rotear();
})();
