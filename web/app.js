/* SCI — front vanilla (sem build, sem framework). Hash routing. v9.3 */
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
  ESTADO = { c: d.conferencia, pessoas: d.pessoas, est, dest, obs, verif };
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
        return `<div class="pessoa ${e.verif.has(p.id) ? 'verificado' : ''}" data-id="${p.id}">
          <input type="checkbox" class="chk" data-id="${p.id}" ${e.verif.has(p.id) ? 'checked' : ''} title="verifiquei esta pessoa">
          <span class="nome clicavel"><b>${esc(p.nome_guerra)}</b><small>${esc(p.nome_completo)}${p.funcao ? ' · ' + esc(p.funcao) : ''}${e.obs[p.id] ? ' · 📝' : ''}${e.temComentario[p.id] ? ' · 💬' : ''}</small></span>
          ${sel}<button type="button" class="fantasma bt-coment" data-id="${p.id}" title="comentários" style="min-height:36px;padding:4px 8px">💬</button>${pill(sit)}</div>`;
      }).join('') + '</div></div>';
  }
  const banner = semC
    ? `<div class="cartao"><p style="color:var(--tx2)">Nenhuma conferência aberta. Ao iniciar, a data e o horário de Brasília são registrados automaticamente.</p>
       <div style="display:flex;gap:8px;align-items:end;margin-top:10px;flex-wrap:wrap">
         <div class="campo" style="margin:0"><label>Local (opcional)</label><input id="cLocal" placeholder="ex.: Praça" style="max-width:160px"></div>
         <button class="primario" id="btIniciar" style="min-height:44px">▶ Iniciar conferência</button></div>
       <p style="margin-top:10px"><a href="#/conferencias" style="color:var(--verde)">Ver lista de conferências →</a></p></div>`
    : `<div class="cartao" style="display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap">
       <span>${pill('aberta')} <b>Conferência de pessoal</b> · aberta em ${e.c.data} às ${fmtHora(e.c.criada_em)} · toque no militar p/ ciclar, ✅ para verificar</span>
       <button class="perigo" id="btFechar">✕ Fechar conferência</button></div>
       <p style="color:var(--tx2);font-size:12px;margin:0 0 10px"><a href="#/conferencias" style="color:var(--verde)">Lista de conferências →</a></p>`;
  $('#app').innerHTML = `<h2>Conferência de pessoal</h2>${banner}
    <div class="barra-fixa">
      <input id="busca" placeholder="buscar nome…">
      <button class="primario" id="btFecharBarra" ${semC ? 'disabled' : ''}>✕ FECHAR CONFERÊNCIA</button>
    </div>
    <div id="lista">${semC ? '' : listas}</div>`;
  if (semC) {
    $('#btIniciar').onclick = async () => {
      try {
        await api('/api/conferencia/iniciar', { method: 'POST', body: JSON.stringify({ local: $('#cLocal').value }) });
        toast('Conferência iniciada — data e hora de Brasília registradas');
        viewHoje();
      } catch (err) {}
    };
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
  document.querySelectorAll('.pessoa .clicavel, .pessoa .pill').forEach(el => el.onclick = ev => {
    const card = ev.target.closest('.pessoa');
    const id = +card.dataset.id;
    const atual = ESTADO.est[id] || 'presente';
    const novo = CICLO[(CICLO.indexOf(atual) + 1) % CICLO.length];
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
    location.hash = '#/conferencias';
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
        <div class="campo"><label>Setor (herdado)</label><input value="${esc(d.setor || '—')}" disabled></div>
        <div class="campo"><label>Função (herdada)</label><input value="${esc(d.funcao || '—')}" disabled></div></div>
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

/* ---------- GRUPOS (gerente/operador: visão do próprio grupo) ---------- */
async function viewGrupos() {
  navAtiva('#/grupos');
  $('#app').innerHTML = '<div class="carregando">…</div>';
  const grupos = await api('/api/grupos');
  let html = '<h2>Meu grupo</h2>';
  const meus = grupos.filter(g => g.id === ME.grupo_id);
  html += `<div class="cartao"><h3 style="margin-top:0">Dados do grupo</h3>` +
    (meus.map(g => `<div style="margin-bottom:10px">
      • <b>${esc(g.nome)}</b> <code style="background:#e8f0e8;color:#12291b;padding:1px 6px;border-radius:4px;font-weight:700">${esc(g.codigo)}</code>
      — ${g.efetivo} no efetivo · ${g.contas} conta(s)
      ${g.subordinados && g.subordinados.length ? `<br><small style="color:var(--tx2);margin-left:14px">subordinados: ${g.subordinados.map(esc).join(', ')}</small>` : ''}
      ${g.superiores && g.superiores.length ? `<br><small style="color:var(--tx2);margin-left:14px">superior: ${g.superiores.map(esc).join(', ')}</small>` : ''}
    </div>`).join('') || '<span class="vazio">nenhum grupo</span>') + '</div>';
  html += `<div class="cartao"><h3 style="margin-top:0">Criar operador do meu grupo</h3>
      <div class="form-linha"><div class="campo"><label>Login</label><input id="opLogin"></div>
      <div class="campo"><label>Senha (mín. 8)</label><input id="opSenha" type="password"></div></div>
      <button class="primario" id="opGo">Criar operador</button></div>`;
  $('#app').innerHTML = html;
  $('#opGo').onclick = async () => {
    const login = $('#opLogin').value.trim(), senha = $('#opSenha').value;
    if (!login || senha.length < 8) { toast('Login e senha (mín. 8) obrigatórios', 'erro'); return; }
    await api('/api/usuarios', { method: 'POST', body: JSON.stringify({ login, senha, papel: 'operador' }) });
    toast('Operador criado');
    viewGrupos();
  };
}

/* ---------- LISTA DE CONFERÊNCIAS ---------- */
function fmtHora(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return isNaN(d) ? '—' : d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
}
async function viewConferencias() {
  navAtiva('#/conferencias');
  $('#app').innerHTML = '<div class="carregando">Carregando…</div>';
  const lista = await api('/api/conferencia/lista');
  const linhas = lista.map(c => `
    <tr><td><b>${c.data}</b>${c.local ? ` <small style="color:var(--tx2)">${esc(c.local)}</small>` : ''}</td>
    <td>${pill(c.status === 'fechada' ? 'presente' : 'atraso')} ${c.status}</td>
    <td>${c.status === 'aberta' ? fmtHora(c.criada_em) : fmtHora(c.fechada_em)}</td>
    <td>${esc(c.criado_por || '—')}</td>
    <td class="num">${c.lancamentos}</td>
    <td>${c.status === 'fechada'
      ? `<a href="/api/conferencia/${c.id}/relatorio.pdf" target="_blank"><button class="primario" style="min-height:36px;padding:8px 12px">Relatório PDF</button></a>`
      : `<a href="#/hoje" style="color:var(--ambar);font-size:13px">em andamento →</a>`}</td></tr>`).join('');
  $('#app').innerHTML = `<h2>Conferências</h2>
    <div class="cartao"><div class="rolagem"><table>
    <thead><tr><th>Data</th><th>Status</th><th>Criada às / Fechada às</th><th>Operador</th><th class="num">Lanç.</th><th>Relatório</th></tr></thead>
    <tbody>${linhas}</tbody></table></div>
    <p style="color:var(--tx2);font-size:12px;margin-top:8px">O relatório PDF só é gerado para conferências <b>fechadas</b>.</p></div>`;
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
    `<tr><td><b>${esc(p.nome_guerra)}</b></td><td>${esc(p.setor)}</td><td class="num">${p.presencas}</td>
     <td class="num">${p.atrasos}</td><td class="num">${p.faltas}</td><td class="num">${p.justificadas}</td>
     <td class="num">${pill(p.pct >= 90 ? 'presente' : p.pct >= 70 ? 'atraso' : 'falta')} ${p.pct}%</td></tr>`).join('');
  const forms = (b.formaturas || []).map(f =>
    `<tr><td>${f.data}</td><td>${esc(f.tipo)}</td><td>${esc(f.hora || '—')}</td><td>${pill(f.status === 'fechada' ? 'presente' : 'atraso')} ${f.status}</td>
     <td class="num">${f.presentes}</td><td class="num">${f.faltas}</td></tr>`).join('');
  return `${res}<div class="cartao"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
    <h3 style="margin:0">${titulo} — ${b.convocacoes} conferências no período</h3>
    <a href="/api/relatorio.pdf?de=${b.De}&ate=${b.Ate}${$('#fGrupo') && $('#fGrupo').value ? `&grupo=${$('#fGrupo').value}` : ''}" target="_blank"><button class="primario">ABRIR PDF</button></a></div>
    ${forms ? `<div class="rolagem" style="margin-bottom:12px"><table><thead><tr><th>Data</th><th>Tipo</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead><tbody>${forms}</tbody></table></div>` : ''}
    <div class="rolagem"><table><thead><tr><th>Nome</th><th>Setor</th><th class="num">Pres.</th><th class="num">Atraso</th>
    <th class="num">Falta</th><th class="num">Just.</th><th class="num">%</th></tr></thead><tbody>${linhas}</tbody></table></div></div>`;
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
      const meuG = gs.find(g => g.id === ME.grupo_id);
      if (meuG && meuG.subordinados_ids && meuG.subordinados_ids.length) {
        gSel = `<div class="campo"><label>Escopo</label><select id="fGrupo"><option value="">Meu grupo + subordinados</option>` +
          meuG.subordinados_ids.map((id, i) => `<option value="${id}">Somente: ${esc(meuG.subordinados[i])}</option>`).join('') + `</select></div>`;
      }
    }
  } catch (e) {}
  $('#app').innerHTML = `<h2>Relatórios</h2>
    <div class="cartao">
      ${gSel ? `<div class="form-linha" style="margin-bottom:8px">${gSel}</div>` : ''}
      <div class="abas" id="modos">
        <button data-m="semana" class="ativo">Semana</button>
        <button data-m="dia">Dia</button>
        <button data-m="mes">Mês</button>
        <button data-m="livre">Período livre</button></div>
      <div class="form-linha" style="grid-template-columns:1fr auto;align-items:end">
        <div id="entrada"></div>
        <button class="primario" id="btGerar" style="min-height:44px">Gerar</button></div></div>
    <div id="saida"><div class="carregando">Gerando semana atual…</div></div>`;
  let modo = 'semana';
  const entrada = () => {
    const el = $('#entrada');
    if (modo === 'semana') el.innerHTML = `<div class="campo"><label>Semana (escolha qualquer dia dela)</label><input type="date" id="fDia" value="${dataLocal(hoje)}"></div>`;
    else if (modo === 'dia') el.innerHTML = `<div class="campo"><label>Dia específico</label><input type="date" id="fDia" value="${dataLocal(hoje)}"></div>`;
    else if (modo === 'mes') el.innerHTML = `<div class="campo"><label>Mês</label><input type="month" id="fMes" value="${dataLocal(hoje).slice(0, 7)}"></div>`;
    else el.innerHTML = `<div class="campo"><label>De — até</label><div style="display:flex;gap:6px"><input type="date" id="fDe" value="${dataLocal(new Date(Date.now() - 29 * 864e5))}"><input type="date" id="fAte" value="${dataLocal(hoje)}"></div></div>`;
  };
  const periodos = () => {
    if (modo === 'semana') return semanaDe($('#fDia').value);
    if (modo === 'dia') { const d = $('#fDia').value; return [d, d]; }
    if (modo === 'mes') { const m = $('#fMes').value; return [m + '-01', fimDoMes(m)]; }
    return [$('#fDe').value, $('#fAte').value];
  };
  const rotulos = { semana: 'Semana', dia: 'Dia', mes: 'Mês', livre: 'Período' };
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

/* --- Usuários: criar, redefinir senha, EXCLUIR com confirmação (R6) --- */
async function admUsuarios() {
  const lista = await api('/api/usuarios');
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Criar conta</h3>
    <div class="form-linha"><div class="campo"><label>Login</label><input id="uLogin"></div>
    <div class="campo"><label>Senha (mín. 8)</label><input id="uSenha" type="password"></div>
    <div class="campo"><label>Papel</label><select id="uPapel"><option value="gerente">gerente</option><option value="admin">admin</option></select></div></div>
    <button class="primario" id="uCriar">Criar conta</button>
    <p style="color:var(--tx2);font-size:12px">Operadores nascem dentro do grupo (o gerente cria os do seu). Excluir: contas com histórico são desativadas, o resto é removido.</p></div>
    <div class="cartao"><table><thead><tr><th>Login</th><th>Papel</th><th>Grupo</th><th>Status</th><th>Criada</th><th></th></tr></thead>
    <tbody>${lista.map(u => `<tr><td><b>${esc(u.login)}</b></td><td>${rotuloPapel(u.papel)}</td><td>${u.grupo_id || '—'}</td>
      <td>${u.ativo ? 'ativa' : 'desativada'}</td><td>${(u.criado_em || '').slice(0, 10)}</td>
      <td><button class="acao-linha" data-id="${u.id}" data-login="${esc(u.login)}">🔑 senha</button>
      ${u.papel !== 'admin' ? `<button class="acao-linha" data-exc="${u.id}" data-login="${esc(u.login)}">excluir</button>` : ''}</td></tr>`).join('')}</tbody></table></div>`;
  $('#uCriar').onclick = async () => {
    await api('/api/usuarios', { method: 'POST', body: JSON.stringify({ login: $('#uLogin').value.trim(), senha: $('#uSenha').value, papel: $('#uPapel').value }) });
    toast('Conta criada'); viewAdmin();
  };
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
  document.querySelectorAll('#adm button[data-exc]').forEach(b => b.onclick = async () => {
    if (!confirm(`Excluir a conta "${b.dataset.login}"?`)) return;
    try {
      const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
      toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
      viewAdmin();
    } catch (e) {}
  });
}

/* --- Grupos: criar grupo+gerente, trocar gerente, subordinação (R7/R8) --- */
async function admGrupos() {
  const [grupos, contas] = await Promise.all([api('/api/grupos'), api('/api/usuarios')]);
  const gerenteDe = {};
  contas.filter(c => c.papel === 'gerente' && c.ativo && c.grupo_id).forEach(c => { gerenteDe[c.grupo_id] = c.login; });
  const optsContas = gid => contas.filter(c => c.grupo_id === gid && c.papel !== 'admin' && c.ativo)
    .map(c => `<option value="${esc(c.login)}">${esc(c.login)} (${rotuloPapel(c.papel)})</option>`).join('');
  let linhas = grupos.map(g => {
    const subs = g.subordinados_ids || [], sups = g.superiores_ids || [];
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
      </div></div>`;
  }).join('');
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Criar grupo — nasce com gerente</h3>
    <div class="form-linha">
      <div class="campo"><label>Nome do grupo</label><input id="gNome" placeholder="ex.: 1ª Cia"></div>
      <div class="campo"><label>Login do gerente</label><input id="gLogin"></div>
      <div class="campo"><label>Senha do gerente (mín. 8)</label><input id="gSenha" type="password"></div>
      <div class="campo"><label>Nome de guerra do gerente</label><input id="gGuerra"></div></div>
    <button class="primario" id="gGo">Criar grupo</button>
    <p style="color:var(--tx2);font-size:12px">O grupo exige gerente no ato da criação — sem gerente, sem grupo.</p></div>
    ${linhas || '<div class="cartao"><span class="vazio">nenhum grupo</span></div>'}`;
  $('#gGo').onclick = async () => {
    const nome = $('#gNome').value.trim(), login = $('#gLogin').value.trim(),
          senha = $('#gSenha').value, guerra = $('#gGuerra').value.trim();
    if (!nome || !login || senha.length < 8 || !guerra) { toast('Nome do grupo + login + senha (mín. 8) + nome de guerra do gerente', 'erro'); return; }
    const r = await api('/api/grupos', { method: 'POST', body: JSON.stringify({ nome, login, senha, nome_guerra: guerra }) });
    toast(`Grupo criado — código ${r.codigo}, gerente ${login}`); viewAdmin();
  };
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
  // R11: admin = DASHBOARD - ADMIN - RELATÓRIOS; R2: sem abas de conferência
  let itens;
  if (ME && ME.papel === 'admin') {
    itens = [['#/hoje', 'Dashboard'], ['#/admin', 'Admin'], ['#/relatorios', 'Relatórios']];
  } else {
    itens = [['#/hoje', 'Conferência'], ['#/conferencias', 'Conferências'], ['#/relatorios', 'Relatórios']];
    if (ME && ME.papel === 'gerente') itens.push(['#/grupos', 'Grupos']);
    if (ME && ME.papel !== 'admin') itens.push(['#/perfil', 'Meu usuário']); // R3
  }
  $('#nav').innerHTML = itens.map(([h, t]) => `<a href="${h}">${t}</a>`).join('');
}
function ligarMenuUsuario() { // R13
  const menu = $('#menuUsuarioItens');
  $('#quem').onclick = ev => { ev.stopPropagation(); menu.classList.toggle('oculto'); };
  document.addEventListener('click', ev => { if (!menu.classList.contains('oculto') && !$('#menuUsuario').contains(ev.target)) menu.classList.add('oculto'); });
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
  else if (h === '#/conferencias') viewConferencias();
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
