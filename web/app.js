/* SCI — front vanilla (sem build, sem framework). Hash routing. */
'use strict';
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const CICLO = ['presente', 'atraso', 'falta', 'justificada'];
let ME = null, DESTINOS = [];

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
      location.hash = '#/hoje';
    } catch (e) {}
  };
  $('#btEntrar').onclick = entrar;
  $('#sn').onkeydown = e => { if (e.key === 'Enter') entrar(); };
}

/* ---------- CONFERÊNCIA DE PESSOAL (iniciar → verificar → fechar) ---------- */
let ESTADO = null; // {c:conferencia|null, pessoas:[], est:{}, dest:{}, obs:{}, verif:Set}
async function viewHoje() {
  navAtiva('#/hoje');
  $('#app').innerHTML = '<div class="carregando">Carregando efetivo…</div>';
  const d = await api('/api/conferencia/hoje');
  if (!DESTINOS.length) { try { DESTINOS = await api('/api/catalogo/destinos'); } catch (e) { DESTINOS = []; } }
  DESTINOS = DESTINOS.filter(x => x.ativo === 1 || x.ativo === true);
  const est = {}, dest = {}, obs = {}, verif = new Set();
  if (d.conferencia) for (const [pid, v] of Object.entries(d.conferencia.estados)) {
    est[pid] = v.situacao;
    dest[pid] = v.destino_id;
    if (v.observacao) obs[pid] = v.observacao;
    verif.add(+pid); // já registrado na conferência = verificado
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
          <span class="nome clicavel"><b>${esc(p.nome_guerra)}</b><small>${esc(p.nome_completo)}${p.funcao ? ' · ' + esc(p.funcao) : ''}${e.obs[p.id] ? ' · 📝' : ''}</small></span>
          ${sel}${pill(sit)}</div>`;
      }).join('') + '</div></div>';
  }
  const banner = semC
    ? `<div class="cartao"><p style="color:var(--tx2)">Nenhuma conferência aberta. Inicie a conferência de pessoal — ela fica aberta até você fechá-la.</p>
       <div style="display:flex;gap:8px;align-items:end;margin-top:10px;flex-wrap:wrap">
         <div class="campo" style="margin:0"><label>Data de início</label><input type="date" id="cData" value="${dataLocal(new Date())}"></div>
         <div class="campo" style="margin:0"><label>Local (opcional)</label><input id="cLocal" placeholder="ex.: Praça" style="max-width:160px"></div>
         <button class="primario" id="btIniciar" style="min-height:44px">▶ Iniciar conferência</button></div>
       <p style="margin-top:10px"><a href="#/conferencias" style="color:var(--verde)">Ver lista de conferências →</a></p></div>`
    : `<div class="cartao" style="display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap">
       <span>${pill('aberta')} <b>Conferência de pessoal</b> · aberta em ${e.c.data} às ${(e.c.criada_em || '').slice(11, 16)} · toque no militar p/ ciclar, ✅ para verificar</span>
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
        await api('/api/conferencia/iniciar', { method: 'POST', body: JSON.stringify({ data: $('#cData').value, local: $('#cLocal').value }) });
        toast('Conferência iniciada');
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
    ESTADO.verif.delete(id); // mudou situação → precisa reverificar
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
  $('#btFechar').onclick = fecharConferencia;
}
/* modal de lançamento: falta → motivo/obs; justificada → destino + obs */
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

/* ---------- LISTA DE CONFERÊNCIAS + RELATÓRIO PRÓPRIO ---------- */
async function viewConferencias() {
  navAtiva('#/conferencias');
  $('#app').innerHTML = '<div class="carregando">Carregando…</div>';
  const lista = await api('/api/conferencia/lista');
  const linhas = lista.map(c => `
    <tr><td><b>${c.data}</b>${c.local ? ` <small style="color:var(--tx2)">${esc(c.local)}</small>` : ''}</td>
    <td>${pill(c.status === 'fechada' ? 'presente' : 'atraso')} ${c.status}</td>
    <td>${c.status === 'aberta' ? (c.criada_em || '').slice(11, 16) : ((c.fechada_em || '').slice(11, 16) || '—')}</td>
    <td>${esc(c.criado_por || '—')}</td>
    <td class="num">${c.lancamentos}</td>
    <td>${c.status === 'fechada'
      ? `<a href="/api/conferencia/${c.id}/relatorio.pdf"><button class="primario" style="min-height:36px;padding:8px 12px">Relatório PDF</button></a>`
      : `<a href="#/hoje" style="color:var(--ambar);font-size:13px">em andamento →</a>`}</td></tr>`).join('');
  $('#app').innerHTML = `<h2>Conferências</h2>
    <div class="cartao"><div class="rolagem"><table>
    <thead><tr><th>Data</th><th>Status</th><th>Criada às / Fechada às</th><th>Operador</th><th class="num">Lanç.</th><th>Relatório</th></tr></thead>
    <tbody>${linhas}</tbody></table></div>
    <p style="color:var(--tx2);font-size:12px;margin-top:8px">O relatório PDF só é gerado para conferências <b>fechadas</b> — contém horário de fechamento, horário de geração e o nome do operador.</p></div>`;
}

/* ---------- SEMANA / RELATÓRIOS ---------- */
function dataLocal(d) { return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); }
function semanaDe(isoDia) {
  const d = new Date(isoDia + 'T12:00:00');
  const dow = (d.getDay() + 6) % 7; // seg=0
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
    <div class="caixa"><b>${b.pct_geral}%</b><span>presença</span></div>
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
    <a href="/api/relatorio.pdf?de=${b.De}&ate=${b.Ate}"><button class="primario">BAIXAR PDF</button></a></div>
    ${forms ? `<div class="rolagem" style="margin-bottom:12px"><table><thead><tr><th>Data</th><th>Tipo</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead><tbody>${forms}</tbody></table></div>` : ''}
    <div class="rolagem"><table><thead><tr><th>Nome</th><th>Setor</th><th class="num">Pres.</th><th class="num">Atraso</th>
    <th class="num">Falta</th><th class="num">Just.</th><th class="num">%</th></tr></thead><tbody>${linhas}</tbody></table></div></div>`;
}
async function viewRelatorios() {
  navAtiva('#/relatorios');
  const hoje = new Date();
  const [seg, dom] = semanaDe(dataLocal(hoje));
  $('#app').innerHTML = `<h2>Relatórios</h2>
    <div class="cartao">
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
      const b = await api(`/api/relatorio?de=${de}&ate=${ate}`);
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

/* ---------- ADMIN ---------- */
let abaAdmin = 'pessoas';
async function viewAdmin() {
  navAtiva('#/admin');
  const abas = ['pessoas', 'catálogos', 'usuários', 'backup'];
  $('#app').innerHTML = `<h2>Administração</h2>
    <div class="abas">${abas.map(a => `<button data-a="${a}" class="${abaAdmin === a ? 'ativo' : ''}">${a}</button>`).join('')}</div>
    <div id="adm"><div class="carregando">…</div></div>`;
  document.querySelectorAll('.abas button').forEach(b => b.onclick = () => { abaAdmin = b.dataset.a; viewAdmin(); });
  if (abaAdmin === 'pessoas') await admPessoas();
  else if (abaAdmin === 'catálogos') await admCatalogos();
  else if (abaAdmin === 'usuários') await admUsuarios();
  else admBackup();
}
async function admPessoas() {
  const [pessoas, setores, funcoes] = await Promise.all([
    api('/api/pessoas'), api('/api/catalogo/setores'), api('/api/catalogo/funcoes')]);
  const opts = (lista, sel) => `<option value="">—</option>` + lista.filter(x => x.ativo === 1 || x.ativo === true)
    .map(x => `<option value="${x.id}" ${sel == x.id ? 'selected' : ''}>${esc(x.nome)}</option>`).join('');
  const linhas = pessoas.pessoas.map(p =>
    `<tr data-p='${esc(JSON.stringify(p))}'><td><b>${esc(p.nome_guerra)}</b></td><td>${esc(p.nome_completo)}</td>
     <td>${esc(p.setor)}</td><td>${esc(p.funcao)}</td><td>${pill(p.status === 'ativo' ? 'presente' : 'justificada')} ${p.status}</td></tr>`).join('');
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">${JSON.parse('null') ? '' : ''}Cadastrar / editar militar</h3>
    <input type="hidden" id="pId">
    <div class="form-linha"><div class="campo"><label>Nome de guerra</label><input id="pNg"></div>
    <div class="campo"><label>Nome completo</label><input id="pNc"></div></div>
    <div class="form-linha"><div class="campo"><label>Setor</label><select id="pSetor">${opts(setores)}</select></div>
    <div class="campo"><label>Função</label><select id="pFuncao">${opts(funcoes)}</select></div>
    <div class="campo"><label>Status</label><select id="pStatus"><option value="ativo">ativo</option><option value="inativo">inativo</option><option value="movido">movido</option></select></div></div>
    <button class="primario" id="pSalvar">Salvar</button>
    <h3>Importar em lote (1 por linha: nome de guerra ; nome completo ; setor ; função)</h3>
    <textarea id="csv" rows="4" placeholder="SILVA;José da Silva;Comando;Motorista"></textarea>
    <button class="acao-linha" id="csvGo" style="margin-top:8px">Importar linhas</button></div>
    <div class="cartao"><h3 style="margin-top:0">Efetivo cadastrado (${pessoas.pessoas.length})</h3>
    <div class="rolagem"><table><thead><tr><th>Guerra</th><th>Completo</th><th>Setor</th><th>Função</th><th>Status</th></tr></thead>
    <tbody id="tabP">${linhas}</tbody></table></div></div>`;
  $('#pSalvar').onclick = async () => {
    const corpo = { nome_guerra: $('#pNg').value.trim(), nome_completo: $('#pNc').value.trim(),
      setor_id: +$('#pSetor').value || null, funcao_id: +$('#pFuncao').value || null, status: $('#pStatus').value };
    if (!corpo.nome_guerra || !corpo.nome_completo) { toast('Nomes obrigatórios', 'erro'); return; }
    const id = $('#pId').value;
    if (id) await api('/api/pessoas/' + id, { method: 'PATCH', body: JSON.stringify(corpo) });
    else await api('/api/pessoas', { method: 'POST', body: JSON.stringify(corpo) });
    toast('Salvo'); viewAdmin();
  };
  $('#csvGo').onclick = async () => {
    const linhas = $('#csv').value.split('\n').map(l => l.trim()).filter(Boolean);
    let ok = 0;
    for (const l of linhas) {
      const [ng, nc, st, fn] = l.split(';').map(x => (x || '').trim());
      if (!ng || !nc) continue;
      const sid = (setores.find(s => s.nome.toLowerCase() === (st || '').toLowerCase()) || {}).id || null;
      const fid = (funcoes.find(s => s.nome.toLowerCase() === (fn || '').toLowerCase()) || {}).id || null;
      try { await api('/api/pessoas', { method: 'POST', body: JSON.stringify({ nome_guerra: ng, nome_completo: nc, setor_id: sid, funcao_id: fid, status: 'ativo' }) }); ok++; } catch (e) {}
    }
    toast(`${ok} importados`); if (ok) viewAdmin();
  };
  document.querySelectorAll('#tabP tr').forEach(tr => tr.onclick = () => {
    const p = JSON.parse(tr.dataset.p);
    $('#pId').value = p.id; $('#pNg').value = p.nome_guerra; $('#pNc').value = p.nome_completo;
    $('#pSetor').value = p.setor_id || ''; $('#pFuncao').value = p.funcao_id || ''; $('#pStatus').value = p.status;
    window.scrollTo({ top: 0, behavior: 'smooth' });
  });
}
async function admCatalogos() {
  const tabs = ['setores', 'funcoes', 'destinos', 'tags', 'conferencia_tipos'];
  const rot = { funcoes: 'funções', destinos: 'destinos', tags: 'tags', setores: 'setores', conferencia_tipos: 'tipos de conferência' };
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Catálogos do admin (alimentam os dropdowns)</h3>
    ${tabs.map(t => `<div class="cat-bloco"><b>${rot[t]}</b><div id="cat-${t}" class="cat-itens">…</div>
      <div style="display:flex;gap:6px"><input id="nov-${t}" placeholder="novo item" style="max-width:220px">
      <button class="acao-linha" data-t="${t}">adicionar</button></div></div>`).join('')}
    <p style="color:var(--tx2);font-size:12px;margin-top:8px">✕ exclui de verdade (itens em uso por lançamentos não podem ser excluídos) · ⏸ desativa preservando o histórico.</p></div>`;
  const carregar = async () => {
    for (const t of tabs) {
      const lista = await api('/api/catalogo/' + t);
      $('#cat-' + t).innerHTML = lista.map(x =>
        `<span class="cat-item">${esc(x.nome)}${x.ativo ? '' : ' <i>(inativo)</i>'}
         <button class="cat-x" data-t="${t}" data-id="${x.id}" title="excluir">✕</button>
         ${x.ativo ? `<button class="cat-off" data-t="${t}" data-id="${x.id}" title="desativar">⏸</button>` : ''}</span>`).join(' ') || '<span class="vazio">— vazio —</span>';
    }
    document.querySelectorAll('.cat-x').forEach(b => b.onclick = () => excluirItem(b.dataset.t, b.dataset.id, b));
    document.querySelectorAll('.cat-off').forEach(b => b.onclick = async () => {
      await api(`/api/catalogo/${b.dataset.t}/${b.dataset.id}?modo=desativar`, { method: 'DELETE' });
      toast('Desativado'); await carregar();
    });
  };
  window.excluirItem = async (t, id, btn) => {
    const nome = btn.parentNode.childNodes[0].textContent.trim();
    if (!confirm(`Excluir "${nome}"? Itens em uso por lançamentos/cadastros não podem ser excluídos.`)) return;
    try { await api(`/api/catalogo/${t}/${id}`, { method: 'DELETE' }); toast('Excluído'); await carregar(); } catch (e) {}
  };
  await carregar();
  document.querySelectorAll('#adm button[data-t]').forEach(b => b.onclick = async () => {
    const t = b.dataset.t, nome = $('#nov-' + t).value.trim();
    if (!nome) return;
    await api('/api/catalogo/' + t, { method: 'POST', body: JSON.stringify({ nome }) });
    toast('Adicionado'); await carregar();
  });
}
async function admUsuarios() {
  const lista = await api('/api/usuarios');
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Contas</h3>
    <div class="form-linha"><div class="campo"><label>Login</label><input id="uLogin"></div>
    <div class="campo"><label>Senha</label><input id="uSenha" type="password"></div>
    <div class="campo"><label>Papel</label><select id="uPapel"><option value="usuario">usuário</option><option value="admin">admin</option></select></div></div>
    <button class="primario" id="uCriar">Criar conta</button></div>
    <div class="cartao"><table><thead><tr><th>Login</th><th>Papel</th><th>Criada</th><th></th></tr></thead>
    <tbody>${lista.map(u => `<tr><td><b>${esc(u.login)}</b></td><td>${u.papel}</td><td>${(u.criado_em || '').slice(0, 10)}</td>
      <td><button class="acao-linha" data-id="${u.id}" data-login="${esc(u.login)}">🔑 senha</button></td></tr>`).join('')}</tbody></table></div>`;
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
}
function admBackup() {
  $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Backup definitivo</h3>
    <p style="color:var(--tx2);font-size:13px">O SCI já faz backup automático a cada formatura confirmada e no boot
    (<code>VACUUM INTO</code> + SHA-256 + MANIFEST). Aqui você força uma cópia agora.</p>
    <button class="primario" id="bkGo">Fazer backup agora</button>
    <pre id="bkOut" style="margin-top:10px;color:var(--tx2);font-size:12px"></pre></div>`;
  $('#bkGo').onclick = async () => {
    const r = await api('/api/backup', { method: 'POST', body: '{}' });
    $('#bkOut').textContent = `${r.arquivo}\nsha256: ${r.sha256}\n→ download iniciado no navegador`;
    toast('Backup gerado — download iniciado');
    // ordem do Tenente: gerou → baixa o arquivo no navegador
    window.location.href = '/api/backup/download?nome=' + encodeURIComponent(r.arquivo.split('/').pop());
  };
}

/* ---------- casca ---------- */
function navAtiva(h) { document.querySelectorAll('#nav a').forEach(a => a.classList.toggle('ativo', a.hash === h)); }
function montarNav() {
  $('#topbar').classList.remove('oculto');
  $('#quem').textContent = ME ? ME.login + (ME.papel === 'admin' ? ' · admin' : '') : '';
  const itens = [['#/hoje', 'Conferência'], ['#/conferencias', 'Conferências'], ['#/relatorios', 'Relatórios']];
  if (ME && ME.papel === 'admin') itens.push(['#/admin', 'Admin']);
  $('#nav').innerHTML = itens.map(([h, t]) => `<a href="${h}">${t}</a>`).join('');
  $('#btSenha').onclick = modalSenha; // botão estático no HTML — nunca duplica
  $('#btnSair').onclick = async () => { await api('/api/logout', { method: 'POST', body: '{}' }); ME = null; location.hash = '#/login'; };
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
  if (h === '#/hoje') viewHoje();
  else if (h === '#/conferencias') viewConferencias();
  else if (h === '#/relatorios' || h === '#/semana') viewRelatorios();
  else if (h === '#/admin' && ME.papel === 'admin') viewAdmin();
  else { location.hash = '#/hoje'; }
}
window.onhashchange = rotear;
(async function init() {
  try { const r = await api('/api/me'); ME = r.usuario; } catch (e) {}
  if (!location.hash) location.hash = '#/hoje';
  rotear();
})();
