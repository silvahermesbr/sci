/* SCI — views de GESTÃO (redesign): #/admin · #/grupos (Gerenciar) · #/perfil
   window.ViewAdmin / window.ViewGrupos / window.ViewPerfil (async).
   Consome os helpers globais do core: api, esc, toast, fmtData, fmtHora, pill,
   abrirModal (insere .modal-mask no body e devolve o elemento), confirmar(msg) →
   Promise<boolean>, navAtiva(hash). Vanilla, sem build, sem CDN.
   Comportamento copiado do protótipo web/app.js (v9.11.2) — nada pode sumir. */
'use strict';
(function () {
  const $ = s => document.querySelector(s);
  const rotuloPapel = p => p === 'admin' ? 'ADMIN' : p === 'gerente' ? 'GERENTE' : 'OPERADOR';
  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const ativosDe = l => (l || []).filter(x => x.ativo === 1 || x.ativo === true);

  let abaAdmin = 'usuarios'; // sub-aba corrente do painel admin (persiste na sessão)
  let abaGer = 'pessoal';    // sub-aba corrente do Gerenciar

  /* ---------- blocos compartilhados ---------- */

  // abrirModal (core) insere o .modal-mask no body e devolve o elemento; aqui só
  // completamos com fechar por ESC / clique fora, como nos modais do protótipo.
  function modal(html) {
    // v9.16.2 FIX: abrirModal devolve {fechar, mask, modal}; expor o ELEMENTO mask com helper fechar
    const h = abrirModal(html);
    if (!h) return null;
    const m = h.mask;
    m.fechar = h.fechar;
    m.querySelector = sel => h.modal.querySelector(sel);
    m.querySelectorAll = sel => h.modal.querySelectorAll(sel);
    m.addEventListener('click', ev => { if (ev.target === m) h.fechar(); });
    return m;
  }

  const codigoChip = c => `<code style="background:#e8f0e8;color:#12291b;padding:1px 6px;border-radius:4px;font-weight:700;font-size:11px">${esc(c)}</code>`;

  // nó da árvore nested — raiz-only toggle (▸/▾), efetivo total recursivo 'total (próprio X)'
  const noHTML = (n, nivel, comGerente) => {
    const temFilhos = !!(n.filhos && n.filhos.length);
    const id = 'noArv' + n.id + '_' + nivel + '_' + Math.random().toString(36).slice(2, 6);
    const rec = n.efetivo_total !== undefined && n.efetivo_total !== n.efetivo;
    const efetTxt = rec
      ? ` · efetivo total: <b style="color:#000">${n.efetivo_total}</b> <small style="color:#000">(próprio ${n.efetivo})</small>`
      : ` · <span style="color:#000">${n.efetivo} no efetivo</span>`;
    return `<div style="margin-left:${nivel * 22}px;padding:5px 8px;border-left:3px solid var(--verde);margin-bottom:4px;background:#f4f8f4;border-radius:0 6px 6px 0">` +
      (temFilhos ? `<span data-tgl="${id}" style="cursor:pointer;font-weight:700;color:#000">▸ </span>` : '') +
      `<b style="color:#000">${esc(n.nome)}</b> <small style="color:#000">#${n.id}</small> ` + codigoChip(n.codigo) +
      `<small style="color:#000">${comGerente ? ` · gerente: <b style="color:#000">${esc(n.gerente || '—')}</b>` : ''}${efetTxt} · ${n.contas} conta(s)</small>` +
      (temFilhos ? `<div id="${id}" class="oculto" style="margin-top:4px">${n.filhos.map(f => noHTML(f, nivel + 1, comGerente)).join('')}</div>` : '') +
      `</div>`;
  };
  const arvoreHTML = (nos, comGerente) => (nos && nos.length ? nos.map(n => noHTML(n, 0, comGerente)).join('') : '<span class="vazio">nenhum grupo</span>');

  function ligarToggles(raiz) {
    (raiz || document).querySelectorAll('[data-tgl]').forEach(s => s.onclick = () => {
      const alvo = document.getElementById(s.dataset.tgl);
      if (!alvo) return;
      const aberto = !alvo.classList.contains('oculto');
      alvo.classList.toggle('oculto', aberto);
      s.textContent = aberto ? '▸ ' : '▾ ';
    });
  }

  // Redefinir senha (admin usa confirmação; operadores do gerente, campo único)
  function modalSenha(id, login, comConfirmar, aoFim) {
    const div = modal(`<div class="modal-inner"><h3>Redefinir senha — ${esc(login)}</h3>
      <div class="campo"><label>Nova senha (mín. 8)</label><input type="password" id="rN"></div>
      ${comConfirmar ? '<div class="campo"><label>Confirmar</label><input type="password" id="rC"></div>' : ''}
      <div class="modal-acoes"><button class="fantasma" id="rX">Cancelar</button>
      <button class="primario" id="rGo">Redefinir</button></div></div>`);
    if (!div) return;
    div.querySelector('#rX').onclick = () => div.fechar && div.fechar();
    div.querySelector('#rGo').onclick = async () => {
      const n = div.querySelector('#rN').value;
      const c = comConfirmar ? div.querySelector('#rC').value : n;
      if (n.length < 8) { toast('Mínimo 8 caracteres', 'erro'); return; }
      if (n !== c) { toast('Senhas não conferem', 'erro'); return; }
      await api(`/api/usuarios/${id}/senha`, { method: 'POST', body: JSON.stringify({ senha: n }) });
      toast('Senha redefinida'); div.fechar && div.fechar();
      if (aoFim) aoFim();
    };
  }

  // Mover conta entre grupos (admin: quaisquer; gerente: backend limita à hierarquia)
  function modalMover(id, login, optsGrupos, aviso, aoFim) {
    const div = modal(`<div class="modal-inner"><h3>Mover conta — ${esc(login)}</h3>
      <div class="campo"><label>Grupo de destino</label><select id="mG">${optsGrupos}</select></div>
      ${aviso ? `<p style="color:var(--tx2);font-size:12px">${aviso}</p>` : ''}
      <div class="modal-acoes"><button class="fantasma" id="mX">Cancelar</button>
      <button class="primario" id="mGo">Mover</button></div></div>`);
    if (!div) return;
    div.querySelector('#mX').onclick = () => div.fechar && div.fechar();
    div.querySelector('#mGo').onclick = async () => {
      const g = div.querySelector('#mG').value;
      if (!g) { toast('Escolha o grupo de destino', 'erro'); return; }
      try {
        const r = await api(`/api/usuarios/${id}/mover`, { method: 'PATCH', body: JSON.stringify({ grupo_id: +g }) });
        toast(r.rebaixado ? 'Movido e rebaixado a operador (grupo de origem recebeu novo gerente)' : 'Conta movida');
        div.fechar && div.fechar();
        if (aoFim) aoFim();
      } catch (e) {}
    };
  }

  const marcarGerente = (nos, gerenteDe) => (nos || []).forEach(n => {
    n.gerente = gerenteDe[n.id] || '';
    marcarGerente(n.filhos || [], gerenteDe);
  });

  /* =====================================================================
     #/admin — ADMINISTRAÇÃO (só admin): Usuários · Grupos · Backup
     ===================================================================== */
  window.ViewAdmin = async function () {
    const eu = quem();
    if (!eu || eu.papel !== 'admin') { location.hash = '#/hoje'; return; }
    navAtiva('#/admin');
    const abas = [['usuarios', 'Usuários'], ['grupos', 'Grupos'], ['backup', 'Backup']];
    $('#app').innerHTML = `<h2>Administração</h2>
      <div class="abas">${abas.map(([k, t]) => `<button data-a="${k}" class="${abaAdmin === k ? 'ativo' : ''}">${t}</button>`).join('')}</div>
      <div id="adm"><div class="carregando">…</div></div>`;
    document.querySelectorAll('.abas button').forEach(b => b.onclick = () => { abaAdmin = b.dataset.a; window.ViewAdmin(); });
    if (abaAdmin === 'usuarios') await admUsuarios();
    else if (abaAdmin === 'grupos') await admGrupos();
    else admBackup();
  };

  /* --- ADMIN › usuários: SOMENTE tabela (ID·Login·Papel·Grupo·Status·Criada·Ações) + filtros --- */
  async function admUsuarios() {
    const [lista, grupos] = await Promise.all([api('/api/usuarios'), api('/api/grupos')]);
    const gerenciaveis = lista.filter(u => u.papel !== 'admin'); // sem criação de conta; admin não se lista
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
        <td>${u.ativo ? 'ativa' : 'desativada'}</td><td>${fmtData(u.criado_em)}</td>
        <td><button class="acao-linha" data-id="${u.id}" data-login="${esc(u.login)}">senha</button>
        <button class="acao-linha" data-mv="${u.id}" data-login="${esc(u.login)}" data-papel="${u.papel}">mover</button>
        <button class="acao-linha" data-exc="${u.id}" data-login="${esc(u.login)}">excluir</button></td></tr>`).join('')
        || '<tr><td colspan="7"><span class="vazio">nenhuma conta</span></td></tr>'}</tbody></table></div>
      <p style="color:var(--tx2);font-size:12px;margin-top:8px">Mover transfere a conta entre grupos quaisquer; se for o único gerente da origem, vira operador no destino e o operador mais antigo assume o grupo. Contas com histórico são desativadas ao excluir; o resto é removido.</p></div>`;
    const aplicarFiltro = () => {
      const q = $('#fULogin').value.trim().toLowerCase();
      const gid = $('#fUGrupo').value;
      const at = $('#fUAtivo').value;
      document.querySelectorAll('#tabU tbody tr[data-login]').forEach(tr => {
        const ok = (!q || tr.dataset.login.toLowerCase().includes(q)) &&
                   (!gid || tr.dataset.gid === gid) &&
                   (at === '' || tr.dataset.ativo === at);
        tr.style.display = ok ? '' : 'none';
      });
    };
    $('#fULogin').oninput = aplicarFiltro;
    $('#fUGrupo').onchange = aplicarFiltro;
    $('#fUAtivo').onchange = aplicarFiltro;
    document.querySelectorAll('#adm button[data-id]').forEach(b => b.onclick = () =>
      modalSenha(b.dataset.id, b.dataset.login, true, () => {}));
    document.querySelectorAll('#adm button[data-mv]').forEach(b => b.onclick = () =>
      modalMover(b.dataset.mv, b.dataset.login, optsGrupos,
        'Se for o único gerente do grupo de origem, virará operador no destino e o operador mais antigo assumirá o grupo.',
        () => admUsuarios()));
    document.querySelectorAll('#adm button[data-exc]').forEach(b => b.onclick = async () => {
      if (!(await confirmar(`Excluir a conta "${b.dataset.login}"?`))) return;
      try {
        const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
        toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
        admUsuarios();
      } catch (e) {}
    });
  }

  /* --- ADMIN › grupos: árvore nested colapsável + criar (EXIGE gerente) + trocar gerente + subordinação + excluir --- */
  async function admGrupos() {
    const [grupos, contas, arvore] = await Promise.all([api('/api/grupos'), api('/api/usuarios'), api('/api/grupos/arvore')]);
    const gerenteDe = {};
    contas.filter(c => c.papel === 'gerente' && c.ativo && c.grupo_id).forEach(c => { gerenteDe[c.grupo_id] = c.login; });
    marcarGerente(arvore, gerenteDe);
    const optsContas = gid => contas.filter(c => c.grupo_id === gid && c.papel !== 'admin' && c.ativo)
      .map(c => `<option value="${esc(c.login)}">${esc(c.login)} (${rotuloPapel(c.papel)})</option>`).join('');
    const linhas = grupos.map(g => {
      const sups = g.superiores_ids || [];
      return `<div class="cartao" data-g="${g.id}" style="margin-bottom:10px">
        <h3 style="margin:0 0 6px">${esc(g.nome)} ${codigoChip(g.codigo)}</h3>
        <p style="margin:0 0 6px;color:var(--tx2);font-size:13px">gerente: <b>${esc(gerenteDe[g.id] || '— sem gerente —')}</b> · ${g.efetivo} no efetivo · ${g.contas} conta(s)</p>
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
    $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Hierarquia dos grupos (subordinados indentados)</h3>
        <div class="campo" style="margin-bottom:8px"><label>Filtrar grupos</label><input id="fGNome" placeholder="buscar grupo…"></div>
        <div id="arvoreAdm">${arvoreHTML(arvore, true)}</div></div>
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
      toast(`Grupo criado — código ${r.codigo}, gerente ${login}`); window.ViewAdmin();
    };
    // filtro da árvore: mantém ancestral de resultado visível e EXPANDE os ramos achados
    const renderArv = nos => {
      $('#arvoreAdm').innerHTML = arvoreHTML(nos, true);
      ligarToggles($('#arvoreAdm'));
    };
    $('#fGNome').oninput = () => {
      const q = $('#fGNome').value.trim().toLowerCase();
      if (!q) { renderArv(arvore || []); return; }
      const mostrar = n => {
        const eu = n.nome.toLowerCase().includes(q) || (n.codigo || '').toLowerCase().includes(q);
        const filhos = (n.filhos || []).map(f => mostrar(f)).filter(Boolean);
        if (!eu && !filhos.length) return null;
        return { ...n, filhos: filhos.length ? filhos : (n.filhos || []) };
      };
      renderArv((arvore || []).map(mostrar).filter(Boolean));
      $('#arvoreAdm').querySelectorAll('.oculto').forEach(d => d.classList.remove('oculto'));
      $('#arvoreAdm').querySelectorAll('[data-tgl]').forEach(s => { s.textContent = '▾ '; });
    };
    $('#fGPainel').oninput = () => {
      const q = $('#fGPainel').value.trim().toLowerCase();
      document.querySelectorAll('#adm div.cartao[data-g]').forEach(c => {
        c.style.display = !q || c.querySelector('h3').textContent.toLowerCase().includes(q) ? '' : 'none';
      });
    };
    ligarToggles($('#adm'));
    document.querySelectorAll('#adm button[data-trocar]').forEach(b => b.onclick = async () => {
      const gid = b.dataset.trocar;
      const sel = $('#tg-' + gid);
      const login = sel ? sel.value : '';
      if (!login) { toast('Escolha a conta a promover', 'erro'); return; }
      try {
        await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login }) });
        toast('Gerente trocado — o anterior virou operador'); window.ViewAdmin();
      } catch (e) {}
    });
    document.querySelectorAll('#adm button[data-remover]').forEach(b => b.onclick = async () => {
      const gid = +b.dataset.remover;
      const sel = $('#sub-' + gid);
      if (!sel || !sel.value) { toast('Este grupo não tem superior', 'erro'); return; }
      try {
        await api('/api/admin/grupos/vinculo?superior_id=' + sel.value + '&subordinado_id=' + gid, { method: 'DELETE', body: '{}' });
        toast('Subordinação removida'); window.ViewAdmin();
      } catch (e) {}
    });
    document.querySelectorAll('#adm select[id^="sub-"]').forEach(sel => sel.onchange = async () => {
      const gid = sel.id.split('-')[1];
      if (!sel.value) return; // remover é pelo botão
      try {
        await api('/api/admin/grupos/vinculo', { method: 'POST', body: JSON.stringify({ superior_id: +sel.value, subordinado_id: +gid }) });
        toast('Subordinação gravada'); window.ViewAdmin();
      } catch (e) {}
    });
    document.querySelectorAll('#adm button[data-excluir]').forEach(b => b.onclick = () => {
      const gid = b.dataset.excluir, gnome = b.dataset.nome;
      const div = modal(`<div class="modal-inner"><h3>Excluir grupo — ${esc(gnome)}</h3>
        <p style="color:var(--tx2);font-size:13px;margin:6px 0">Exclusão normal: só se o grupo estiver vazio.</p>
        <div class="modal-acoes"><button class="fantasma" id="mX">Cancelar</button>
        <button class="perigo" id="mGo">Tentar exclusão normal</button></div>
        <div style="border-top:1px solid var(--borda);margin-top:12px;padding-top:10px">
          <p style="color:var(--verm);font-size:13px;margin:4px 0"><b>Exclusão FORÇADA</b> — remove o grupo INTEIRO mesmo com contas e pessoal (tudo é apagado). Grupos com histórico de conferências NUNCA são excluídos, nem forçadamente. Requer sua senha de admin.</p>
          <div class="campo"><label>Senha de admin</label><input type="password" id="fSenha"></div>
          <div class="modal-acoes"><button class="perigo" id="mForce">Excluir forçadamente</button></div>
        </div></div>`);
      if (!div) return;
      div.querySelector('#mX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#mGo').onclick = async () => {
        try {
          await api(`/api/grupos/${gid}`, { method: 'DELETE' });
          toast('Grupo excluído'); div.fechar && div.fechar(); window.ViewAdmin();
        } catch (e) {}
      };
      div.querySelector('#mForce').onclick = async () => {
        const senha = div.querySelector('#fSenha').value;
        if (!senha) { toast('Digite a senha de admin', 'erro'); return; }
        try {
          const r = await api(`/api/grupos/${gid}?forcar=1`, { method: 'DELETE', body: JSON.stringify({ senha }) });
          toast(`Grupo excluído forçadamente — ${r.contas_removidas} conta(s), ${r.pessoas_removidas} pessoa(s) removidas`);
          div.fechar && div.fechar(); window.ViewAdmin();
        } catch (e) {}
      };
    });
  }

  /* --- ADMIN › backup: gerar download .db + IMPORTAR upload com confirmação --- */
  function admBackup() {
    $('#adm').innerHTML = `<div class="cartao"><h3 style="margin-top:0">Backup</h3>
      <p style="color:var(--tx2);font-size:13px">O SCI faz backup automático a cada conferência fechada e no boot
      (<code>VACUUM INTO</code> + SHA-256 + MANIFEST). Aqui você força uma cópia agora — o download do .db começa em seguida.</p>
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
      if (!(await confirmar(`Importar "${f.name}"? O banco atual será substituído (antes, um backup de segurança é gravado).`))) return;
      const fd = new FormData();
      fd.append('arquivo', f);
      try {
        const r = await fetch('/api/backup/importar', { method: 'POST', headers: { 'X-SCI': '1' }, body: fd });
        const j = await r.json();
        if (!r.ok) { toast(j.erro || 'Falha na importação', 'erro'); $('#bkImpOut').textContent = 'REJEITADO: ' + (j.erro || r.status); return; }
        $('#bkImpOut').textContent = `Importado (schema ${j.schema}). Backup de segurança: ${j.seguranca}`;
        toast('Backup importado — entre novamente com as credenciais do arquivo importado');
        try { if (typeof ME !== 'undefined') ME = null; } catch (e) {}
        location.hash = '#/login';
        if (typeof rotear === 'function') rotear(); else location.reload();
      } catch (e) { $('#bkImpOut').textContent = 'falha de rede'; }
    };
  }

  /* =====================================================================
     #/grupos — GERENCIAR (gerente): Pessoal · Tags · Grupos · Operadores
     ===================================================================== */
  let setoresCat = null, funcoesCat = null; // cache compartilhado (v9.16.9)
  window.ViewGrupos = async function () {
    const eu = quem();
    if (!eu || eu.papel !== 'gerente') { location.hash = '#/hoje'; return; }
    navAtiva('#/grupos');
    $('#app').innerHTML = '<div class="carregando">…</div>';
    const [grupos, arvore, pessoas, setores, funcoes, contas] = await Promise.all([
      api('/api/grupos'), api('/api/grupos/arvore'), api('/api/pessoas'),
      api('/api/catalogo/setores'), api('/api/catalogo/funcoes'), api('/api/usuarios')]);
    const meus = grupos.filter(g => g.id === eu.grupo_id);
    const optSetores = ativosDe(setores), optFuncoes = ativosDe(funcoes);
    setoresCat = setores; funcoesCat = funcoes; // cache p/ carregarCats (v9.16.9)
    const operadores = contas.filter(c => c.grupo_id === eu.grupo_id && c.papel === 'operador');

    /* --- formulário de militar (criar/editar) + adição em lote --- */
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

    /* --- banco de pessoal (checkbox por linha p/ operações em lote) --- */
    const linhasP = (pessoas.pessoas || []).map(p =>
      `<tr data-p='${esc(JSON.stringify(p))}'><td><input type="checkbox" class="chkP" data-id="${p.id}"></td>
       <td class="num">#${p.id}</td><td><b>${esc(p.nome_guerra)}</b></td><td>${esc(p.nome_completo)}</td>
       <td>${esc(p.setor)}${p.setor_id ? ` <small style="color:var(--tx2)">#${p.setor_id}</small>` : ''}</td>
       <td>${esc(p.funcao)}${p.funcao_id ? ` <small style="color:var(--tx2)">#${p.funcao_id}</small>` : ''}</td>
       <td>${p.status === 'ativo' ? '<span class="alerta-ok">● ATIVO</span>' : '<span style="color:var(--tx3)">● INATIVO</span>'}</td>
       <td><button class="acao-linha" data-edit="${p.id}">editar</button>
       <button class="acao-linha" data-excP="${p.id}" data-nome="${esc(p.nome_guerra)}">excluir</button></td></tr>`).join('');

    const optsMoverGer = `<option value="">— destino (dentro da sua hierarquia) —</option>` +
      grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');

    $('#app').innerHTML = `<h2>Gerenciar</h2>
      <div class="abas" id="abasGer">
        <button data-g="pessoal" class="${abaGer === 'pessoal' ? 'ativo' : ''}">Pessoal</button>
        <button data-g="tags" class="${abaGer === 'tags' ? 'ativo' : ''}">Tags</button>
        <button data-g="grupos" class="${abaGer === 'grupos' ? 'ativo' : ''}">Grupos</button>
        <button data-g="operadores" class="${abaGer === 'operadores' ? 'ativo' : ''}">Operadores</button></div>
      <div id="gerPessoal" class="${abaGer === 'pessoal' ? '' : 'oculto'}">
        ${formPessoa}
        <div class="cartao"><h3 style="margin-top:0">BANCO DE PESSOAL (${(pessoas.pessoas || []).length})</h3>
          <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:8px">
            <label style="font-size:13px"><input type="checkbox" id="chkTodosP"> todos</label>
            <button class="primario" id="btEditLote" disabled>Editar selecionados (<span id="nSel">0</span>)</button>
            <button class="perigo" id="btExcLote" disabled>Excluir selecionados (<span id="nSel2">0</span>)</button>
            <span style="color:var(--tx2);font-size:12px">com histórico de conferência: exclusão vira inativo (histórico preservado)</span></div>
          <div class="rolagem"><table><thead><tr><th></th><th>ID</th><th>Guerra</th><th>Completo</th><th>Setor</th><th>Função</th><th>Ativo</th><th></th></tr></thead>
          <tbody id="tabP">${linhasP || '<tr><td colspan="8"><span class="vazio">nenhum militar cadastrado</span></td></tr>'}</tbody></table></div></div>
      </div>
      <div id="gerTags" class="${abaGer === 'tags' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">Catálogos do meu grupo — Setores · Funções · Tags · Destinos</h3>
          <p style="color:var(--tx2);font-size:12px;margin:4px 0">Itens próprios do grupo + <b>herdados dos grupos de cima</b> (o que existe acima vale aqui; o que o grupo cria não sobe). ✕ exclui (em uso → desativa), ⏸ desativa preservando histórico.</p>
          <div id="catGer"><div class="carregando">…</div></div>
          <div class="form-linha" style="margin-top:10px">
            <div class="campo"><label>Catálogo</label><select id="cgT"><option value="setores">Setores</option><option value="funcoes">Funções</option><option value="tags">Tags</option><option value="destinos">Destinos</option></select></div>
            <div class="campo"><label>Novo item</label><input id="cgN" placeholder="nome"></div>
            <div class="campo" style="align-self:end"><button class="primario" id="cgGo">Adicionar</button></div></div></div>
      </div>
      <div id="gerGrupos" class="${abaGer === 'grupos' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">MEU GRUPO</h3>` +
        (meus.map(g => `<div style="margin-bottom:8px">• <b>${esc(g.nome)}</b> ${codigoChip(g.codigo)} — ${g.efetivo} no efetivo · ${g.contas} conta(s)</div>`).join('')
          || '<span class="vazio">nenhum grupo</span>') + `</div>
        <div class="cartao"><h3 style="margin-top:0">Subordinação — árvore do meu grupo (leitura; organização definida pela Administração)</h3>
          <div id="arvore2">${arvoreHTML(arvore, false)}</div></div>
      </div>
      <div id="gerOperadores" class="${abaGer === 'operadores' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">OPERADORES do meu grupo (${operadores.length})</h3>
        <div class="form-linha"><div class="campo"><label>Login do operador</label><input id="opLogin"></div>
        <div class="campo"><label>Senha (mín. 8)</label><input id="opSenha" type="password"></div>
        <div class="campo" style="align-self:end"><button class="primario" id="opGo">Criar operador</button></div></div>
        <div class="campo" style="margin-bottom:8px"><label>Filtrar operadores</label><input id="fOp" placeholder="buscar login…"></div>
        <div class="rolagem" style="margin-top:10px"><table><thead><tr><th>ID</th><th>Login</th><th>Status</th><th>Criada</th><th>Ações</th></tr></thead>
        <tbody>${operadores.map(o => `
          <tr data-login="${esc(o.login)}"><td class="num">#${o.id}</td><td><b>${esc(o.login)}</b></td><td>${o.ativo ? 'ativa' : 'desativada'}</td><td>${fmtData(o.criado_em)}</td>
          <td><button class="acao-linha" data-senha="${o.id}" data-login="${esc(o.login)}">senha</button>
          <button class="acao-linha" data-mv="${o.id}" data-login="${esc(o.login)}">mover</button>
          <button class="acao-linha" data-exc="${o.id}" data-login="${esc(o.login)}">excluir</button></td></tr>`).join('')
          || '<tr><td colspan="5"><span class="vazio">nenhum operador</span></td></tr>'}</tbody></table></div></div>
      </div>`;

    /* --- alternância de sub-abas --- */
    document.querySelectorAll('#abasGer button').forEach(b => b.onclick = () => {
      abaGer = b.dataset.g;
      ['pessoal', 'tags', 'grupos', 'operadores'].forEach(k => {
        const el = $('#ger' + k[0].toUpperCase() + k.slice(1));
        if (el) el.classList.toggle('oculto', k !== abaGer);
      });
      document.querySelectorAll('#abasGer button').forEach(x => x.classList.toggle('ativo', x.dataset.g === abaGer));
    });
    ligarToggles($('#app'));

    /* --- salvar militar (criar/editar) --- */
    $('#pSalvar').onclick = async () => {
      const corpo = { nome_guerra: $('#pNg').value.trim(), nome_completo: $('#pNc').value.trim(),
        setor_id: +$('#pSetor').value || null, funcao_id: +$('#pFuncao').value || null, status: $('#pStatus').value };
      if (!corpo.nome_guerra || !corpo.nome_completo) { toast('Nomes obrigatórios', 'erro'); return; }
      const id = $('#pId').value;
      await processar(() => id
        ? api('/api/pessoas/' + id, { method: 'PATCH', body: JSON.stringify(corpo) })
        : api('/api/pessoas', { method: 'POST', body: JSON.stringify(corpo) }),
        id ? 'Salvando alterações…' : 'Cadastrando militar…');
      window.ViewGrupos();
    };

    /* --- adição em lote --- */
    $('#csvGo').onclick = async () => {
      const linhas = $('#csv').value.split('\n').map(l => l.trim()).filter(Boolean);
      if (!linhas.length) { toast('Cole ao menos uma linha', 'erro'); return; }
      let ok = 0, falha = 0;
      await processar(async () => {
        for (const l of linhas) {
          const [ng, nc, st, fn] = l.split(';').map(x => (x || '').trim());
          if (!ng || !nc) { falha++; continue; }
          const sid = (optSetores.find(s => s.nome.toLowerCase() === (st || '').toLowerCase()) || {}).id || null;
          const fid = (optFuncoes.find(s => s.nome.toLowerCase() === (fn || '').toLowerCase()) || {}).id || null;
          try { await api('/api/pessoas', { method: 'POST', body: JSON.stringify({ nome_guerra: ng, nome_completo: nc, setor_id: sid, funcao_id: fid, status: 'ativo' }) }); ok++; }
          catch (e) { falha++; }
        }
      }, `Importando ${linhas.length} linha(s)…`);
      toast(`${ok} importado(s)${falha ? ' · ' + falha + ' linha(s) com falha' : ''}`, falha && !ok ? 'erro' : 'ok');
      if (ok) window.ViewGrupos();
    };

    /* --- exclusão de militar (individual + lote; com histórico → desativa) --- */
    const excPessoa = async (id, nome) => {
      if (!(await confirmar(`Excluir "${nome}" do banco de pessoal? (com histórico de conferência virará inativo)`))) return;
      const r = await processar(() => api(`/api/pessoas/${id}`, { method: 'DELETE' }), `Excluindo ${nome}…`);
      if (r.ok) {
        toast(r.resultado.desativado ? 'Desativado (histórico preservado)' : 'Excluído');
        window.ViewGrupos();
      }
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
      if (!(await confirmar(`Excluir ${ids.length} militar(es)? (com histórico de conferência virarão inativos)`))) return;
      let ok = 0, des = 0, falha = 0;
      await processar(async () => {
        for (const id of ids) {
          try {
            const r = await api(`/api/pessoas/${id}`, { method: 'DELETE' });
            r.desativado ? des++ : ok++;
          } catch (e) { falha++; }
        }
      }, `Excluindo ${ids.length} militar(es)…`);
      toast(`Excluídos: ${ok} · desativados: ${des}${falha ? ' · falhas: ' + falha : ''}`, falha && !ok ? 'erro' : 'ok');
      if (ok || des) window.ViewGrupos();
    };

    /* --- edição em lote: modal único aplica setor/função/status ('(manter)' = não altera) --- */
    $('#btEditLote').onclick = () => {
      const sel = [...document.querySelectorAll('.chkP:checked')].map(c => +c.dataset.id);
      if (!sel.length) { toast('Selecione ao menos um militar', 'erro'); return; }
      const div = modal(`<div class="modal-inner"><h3>Editar em lote — ${sel.length} militar(es)</h3>
        <p style="color:var(--tx2);font-size:12px;margin:4px 0">Campos em <b>(manter)</b> não são alterados. Aplica a todos os selecionados.</p>
        <div class="form-linha">
          <div class="campo"><label>Setor</label><select id="lSetor"><option value="">(manter)</option>${optSetores.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
          <div class="campo"><label>Função</label><select id="lFuncao"><option value="">(manter)</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
          <div class="campo"><label>Status</label><select id="lStatus"><option value="">(manter)</option><option value="ativo">ativo</option><option value="inativo">inativo</option></select></div></div>
        <div class="modal-acoes"><button class="fantasma" id="lX">Cancelar</button>
        <button class="primario" id="lGo">Aplicar a ${sel.length}</button></div></div>`);
      if (!div) return;
      div.querySelector('#lX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#lGo').onclick = async () => {
        const sid = div.querySelector('#lSetor').value;
        const fid = div.querySelector('#lFuncao').value;
        const st = div.querySelector('#lStatus').value;
        if (!sid && !fid && !st) { toast('Nada para alterar — todos em manter', 'erro'); return; }
        div.fechar && div.fechar();
        const r = await processar(async () => {
          let ok = 0; const erros = [];
          for (const id of sel) {
            const p = (pessoas.pessoas || []).find(x => x.id === id);
            if (!p) { erros.push('#' + id + ' não encontrado'); continue; }
            const corpo = { nome_guerra: p.nome_guerra, nome_completo: p.nome_completo,
              setor_id: sid ? +sid : (p.setor_id ?? null), funcao_id: fid ? +fid : (p.funcao_id ?? null),
              status: st || p.status };
            try { await api('/api/pessoas/' + id, { method: 'PATCH', body: JSON.stringify(corpo) }); ok++; }
            catch (e) { erros.push('#' + id + ': ' + (e && e.message || 'falhou')); }
          }
          if (erros.length && !ok) throw new Error(erros[0]);
          return { ok, erros };
        }, `Editando ${sel.length} militar(es)…`);
        if (r.ok) {
          const info = r.resultado;
          toast(`Aplicado a ${info.ok} militar(es)${info.erros.length ? ' · falhas: ' + info.erros.length : ''}`, info.erros.length ? 'erro' : 'ok');
          window.ViewGrupos();
        }
      };
    };

    /* --- editar militar (preenche o formulário do topo) --- */
    document.querySelectorAll('#tabP tr[data-p]').forEach(tr => {
      tr.querySelector('[data-edit]').onclick = ev => {
        ev.stopPropagation();
        const p = JSON.parse(tr.dataset.p);
        $('#pId').value = p.id; $('#pNg').value = p.nome_guerra; $('#pNc').value = p.nome_completo;
        $('#pSetor').value = p.setor_id || ''; $('#pFuncao').value = p.funcao_id || ''; $('#pStatus').value = p.status;
        window.scrollTo({ top: 0, behavior: 'smooth' });
      };
    });

    /* --- aba TAGS: 4 catálogos (herança só desce) --- */
    const rotCat = { setores: 'Setores', funcoes: 'Funções', tags: 'Tags', destinos: 'Destinos' };
    const carregarCats = async () => {
      const cont = $('#catGer');
      if (!cont) return;
      const tipos = ['setores', 'funcoes', 'tags', 'destinos'];
      const meusGrupos = new Set(((window.ME && window.ME.grupo_id) ? [window.ME.grupo_id] : []));
      // v9.16.9: setores/funcoes JÁ vieram no carregamento da view — só tags/destinos vão à rede
      const cache = { setores: setoresCat, funcoes: funcoesCat };
      const resultados = await Promise.all(tipos.map(t =>
        Array.isArray(cache[t])
          ? Promise.resolve([t, cache[t]])
          : api('/api/catalogo/' + t).then(l => [t, l]).catch(() => [t, []])));
      let html = '';
      for (const [t, lista] of resultados) {
        // v9.16.8: HERDADO (grupo superior) vem primeiro, READ ONLY; DO GRUPO vem depois, gerenciável
        const herdadas = lista.filter(x => !meusGrupos.has(x.grupo_id));
        const minhas = lista.filter(x => meusGrupos.has(x.grupo_id));
        const chipHerd = x => `<span class="cat-item herdado" title="herdado de grupo superior — somente leitura">` +
          `<small style="color:var(--tx2);font-weight:700">#${x.id}</small> ${esc(x.nome)}${x.ativo ? '' : ' <i>(inativo)</i>'}</span>`;
        const chipMeu = x => `<span class="cat-item"><small style="color:var(--tx2);font-weight:700">#${x.id}</small> ${esc(x.nome)}${x.ativo ? '' : ' <i>(inativo)</i>'}
            <button class="cat-x" data-ct="${t}" data-cid="${x.id}" title="excluir">✕</button>
            ${x.ativo ? `<button class="cat-off" data-ct="${t}" data-cid="${x.id}" title="desativar">⏸</button>` : ''}</span>`;
        let corpo = '';
        if (herdadas.length) corpo += `<div class="cat-secao">HERDADO <small>(de grupos superiores — somente leitura, sempre mais antigas)</small></div>` +
          `<div class="cat-itens">${herdadas.map(chipHerd).join(' ')}</div>`;
        corpo += `<div class="cat-secao">DO MEU GRUPO</div>`;
        if (t === 'tags' && minhas.length) {
          const arv = (pai, nivel) => minhas
            .filter(x => (x.pai_id ?? null) === (pai ?? null))
            .sort((a, b) => (a.nome || '').localeCompare(b.nome || '', 'pt'))
            .map(x => {
              const temFilhos = minhas.some(y => y.pai_id === x.id);
              return `<div class="cat-arv-item" draggable="true" data-tagid="${x.id}" style="margin-left:${nivel * 22}px">` +
                `<span class="cat-arv-dot"></span><span class="cat-item" draggable="false"><small style="color:var(--tx2);font-weight:700">#${x.id}</small> ${esc(x.nome)}${x.ativo ? '' : ' <i>(inativo)</i>'}
                <button class="cat-x" data-ct="tags" data-cid="${x.id}" title="excluir">✕</button>
                ${x.ativo ? `<button class="cat-off" data-ct="tags" data-cid="${x.id}" title="desativar">⏸</button>` : ''}</span></div>` + arv(x.id, nivel + 1);
            }).join('');
          corpo += `<div class="cat-arv" id="arvTags">${arv(null, 0)}</div>
            <div class="cat-arv-raiz" id="arvRaiz">⟲ soltar aqui para voltar à raiz</div>`;
        } else {
          corpo += `<div class="cat-itens">${minhas.length ? minhas.map(chipMeu).join(' ') : '<span class="vazio">— nenhuma —</span>'}</div>`;
        }
        // botão de antiguidade (D&D) — só para catálogos com 2+ itens do grupo
        const btAnt = minhas.length >= 2
          ? `<button class="fantasma" data-ant="${t}" style="min-height:34px;padding:6px 12px">⚖ Definir antiguidade</button>` : '';
        html += `<div class="cat-bloco"><div style="display:flex;justify-content:space-between;align-items:center;gap:8px">
          <b>${rotCat[t]}</b>${btAnt}</div>${corpo}</div>`;
      }
      cont.innerHTML = html;
      /* --- handlers: excluir / desativar --- */
      cont.querySelectorAll('.cat-x').forEach(b => b.onclick = async () => {
        if (!(await confirmar('Excluir este item?'))) return;
        const r = await processar(() => api(`/api/catalogo/${b.dataset.ct}/${b.dataset.cid}`, { method: 'DELETE' }), 'Excluindo item…');
        if (r.ok) { toast('Excluído'); if (b.dataset.ct === 'setores' || b.dataset.ct === 'funcoes') setoresCat = funcoesCat = null; carregarCats(); }
      });
      cont.querySelectorAll('.cat-off').forEach(b => b.onclick = async () => {
        const r = await processar(() => api(`/api/catalogo/${b.dataset.ct}/${b.dataset.cid}?modo=desativar`, { method: 'DELETE' }), 'Desativando item…');
        if (r.ok) { toast('Desativado'); carregarCats(); }
      });
      /* --- drag & drop hierarquia de tags (só itens do meu grupo) --- */
      let dragId = null;
      cont.querySelectorAll('.cat-arv-item').forEach(item => {
        item.addEventListener('dragstart', ev => { dragId = +item.dataset.tagid; ev.dataTransfer.effectAllowed = 'move'; });
        item.addEventListener('dragover', ev => { ev.preventDefault(); item.classList.add('drop-alvo'); });
        item.addEventListener('dragleave', () => item.classList.remove('drop-alvo'));
        item.addEventListener('drop', async ev => {
          ev.preventDefault(); ev.stopPropagation();
          item.classList.remove('drop-alvo');
          const pai = +item.dataset.tagid;
          if (!dragId || dragId === pai) return;
          const r = await processar(() => api(`/api/catalogo/tags/${dragId}/pai`, { method: 'PATCH', body: JSON.stringify({ pai_id: pai }) }), 'Movendo tag…');
          if (r.ok) { toast('Tag subordinada'); carregarCats(); }
          dragId = null;
        });
      });
      const raiz = cont.querySelector('#arvRaiz');
      if (raiz) {
        raiz.addEventListener('dragover', ev => { ev.preventDefault(); raiz.classList.add('drop-alvo'); });
        raiz.addEventListener('dragleave', () => raiz.classList.remove('drop-alvo'));
        raiz.addEventListener('drop', async ev => {
          ev.preventDefault();
          raiz.classList.remove('drop-alvo');
          if (!dragId) return;
          const r = await processar(() => api(`/api/catalogo/tags/${dragId}/pai`, { method: 'PATCH', body: JSON.stringify({ pai_id: null }) }), 'Movendo tag…');
          if (r.ok) { toast('Tag voltou à raiz'); carregarCats(); }
          dragId = null;
        });
      }
      /* --- v9.16.8: modal de ANTIGUIDADE por categoria (drag & drop ordenável) --- */
      cont.querySelectorAll('[data-ant]').forEach(bt => bt.onclick = () => {
        const t = bt.dataset.ant;
        const lista = (resultados.find(r => r[0] === t) || [null, []])[1]
          .filter(x => meusGrupos.has(x.grupo_id));
        if (lista.length < 2) return;
        let ordem = lista.slice().sort((a, b) => (a.nome || '').localeCompare(b.nome || '', 'pt'));
        const div = modal(`<div class="modal-inner"><h3>Antiguidade — ${rotCat[t]}</h3>
          <p style="color:var(--tx2);font-size:12px;margin:4px 0">Arraste para ordenar: o primeiro é o mais antigo.</p>
          <div id="antLista" style="display:flex;flex-direction:column;gap:6px;max-height:50vh;overflow:auto"></div>
          <div class="modal-acoes"><button class="fantasma" id="antX">Cancelar</button>
          <button class="primario" id="antGo">Salvar ordem</button></div></div>`);
        const desenhar = () => {
          div.querySelector('#antLista').innerHTML = ordem.map((x, i) =>
            `<div class="ant-item" draggable="true" data-antid="${x.id}">
               <span class="num" style="width:26px;text-align:center;font-weight:800">${i + 1}</span>
               <span style="flex:1">${esc(x.nome)}</span>
               <span style="color:var(--tx3);cursor:grab">⋮⋮</span></div>`).join('');
          div.querySelectorAll('.ant-item').forEach(item => {
            item.addEventListener('dragstart', ev => { ev.dataTransfer.setData('text/plain', item.dataset.antid); item.classList.add('drop-alvo'); });
            item.addEventListener('dragend', () => item.classList.remove('drop-alvo'));
            item.addEventListener('dragover', ev => { ev.preventDefault(); item.style.borderTop = '2px solid var(--verde)'; });
            item.addEventListener('dragleave', () => { item.style.borderTop = ''; });
            item.addEventListener('drop', ev => {
              ev.preventDefault();
              item.style.borderTop = '';
              const movId = +ev.dataTransfer.getData('text/plain');
              const alvoId = +item.dataset.antid;
              if (movId === alvoId) return;
              const de = ordem.findIndex(x => x.id === movId);
              const para = ordem.findIndex(x => x.id === alvoId);
              const [mov] = ordem.splice(de, 1);
              ordem.splice(para, 0, mov);
              desenhar();
            });
          });
        };
        desenhar();
        div.querySelector('#antX').onclick = () => div.fechar();
        div.querySelector('#antGo').onclick = async () => {
          const ids = [...div.querySelectorAll('.ant-item')].map(e => +e.dataset.antid);
          const r = await processar(async () => {
            for (let i = 0; i < ids.length; i++) {
              await api(`/api/catalogo/${t}/${ids[i]}/pai`, { method: 'PATCH', body: JSON.stringify({ pai_id: null, antiguidade: i + 1 }) });
            }
          }, 'Salvando antiguidade…');
          if (r.ok) { toast('Antiguidade salva'); carregarCats(); }
        };
      });
    };
    $('#cgGo').onclick = async () => {
      const t = $('#cgT').value, nome = $('#cgN').value.trim();
      if (!nome) { toast('Informe o nome do item', 'erro'); return; }
      const r = await processar(() => api('/api/catalogo/' + t, { method: 'POST', body: JSON.stringify({ nome }) }), 'Adicionando item…');
      if (r.ok) { toast('Item adicionado'); $('#cgN').value = ''; setoresCat = funcoesCat = null; carregarCats(); }
    };
    if (abaGer === 'tags') carregarCats(); else $('#gerTags').addEventListener('renderTags', carregarCats, { once: true });

    /* --- operadores: criar (acima da tabela) + senha/mover/excluir + filtro --- */
    $('#opGo').onclick = async () => {
      const login = $('#opLogin').value.trim(), senha = $('#opSenha').value;
      if (!login || senha.length < 8) { toast('Login e senha (mín. 8) obrigatórios', 'erro'); return; }
      const r = await processar(() => api('/api/usuarios', { method: 'POST', body: JSON.stringify({ login, senha, papel: 'operador' }) }), `Criando operador ${login}…`);
      if (r.ok) { toast('Operador criado'); window.ViewGrupos(); }
    };
    $('#fOp').oninput = () => {
      const q = $('#fOp').value.trim().toLowerCase();
      document.querySelectorAll('#gerOperadores tbody tr[data-login]').forEach(tr => {
        tr.style.display = !q || tr.dataset.login.toLowerCase().includes(q) ? '' : 'none';
      });
    };
    document.querySelectorAll('#gerOperadores [data-mv]').forEach(b => b.onclick = () =>
      modalMover(b.dataset.mv, b.dataset.login, optsMoverGer,
        'Permitido apenas entre o seu grupo e seus subordinados.', () => window.ViewGrupos()));
    document.querySelectorAll('[data-senha]').forEach(b => b.onclick = () =>
      modalSenha(b.dataset.senha, b.dataset.login, false, () => {}));
    document.querySelectorAll('[data-exc]').forEach(b => b.onclick = async () => {
      if (!(await confirmar(`Excluir a conta "${b.dataset.login}"?`))) return;
      try {
        const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
        toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
        window.ViewGrupos();
      } catch (e) {}
    });
  };

  /* =====================================================================
     #/perfil — MEU USUÁRIO (não-admin): leitura + nome guerra/completo editáveis
     ===================================================================== */
  window.ViewPerfil = async function () {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    if (eu.papel === 'admin') { location.hash = '#/admin'; return; }
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
      const atual = quem();
      if (atual) { atual.nome_guerra = ng; atual.nome_completo = nc; }
      if (typeof montarNav === 'function') montarNav();
      toast('Perfil salvo');
    };
  };
})();
