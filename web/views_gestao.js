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

  let abaAdmin = 'dashboard'; // sub-aba corrente do painel admin (persiste na sessão)
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

  const codigoChip = c => `<code style="background:rgba(16,185,129,0.12);color:var(--verde-claro);padding:2px 6px;border-radius:4px;font-weight:700;font-size:11px;border:1px solid rgba(16,185,129,0.25)">${esc(c)}</code>`;

  let FOCO_GRUPO_ID = null;

  function buscarNoPorId(nos, id) {
    if (!nos || !nos.length) return null;
    for (const n of nos) {
      if (n.id === id) return n;
      const achou = buscarNoPorId(n.filhos, id);
      if (achou) return achou;
    }
    return null;
  }

  function obterCaminhoAncestrais(nos, id, caminho = []) {
    if (!nos || !nos.length) return null;
    for (const n of nos) {
      const novoCaminho = [...caminho, n];
      if (n.id === id) return novoCaminho;
      const achou = obterCaminhoAncestrais(n.filhos, id, novoCaminho);
      if (achou) return achou;
    }
    return null;
  }

  // Card interativo de nó da árvore com expansão e foco recursivo (Obsidian Graph View Style)
  const noCardHTML = (n, nivel, comGerente, arvoreTotal) => {
    const temFilhos = !!(n.filhos && n.filhos.length);
    const idCollapse = 'noArv_' + n.id + '_' + nivel + '_' + Math.random().toString(36).slice(2, 6);
    const rec = n.efetivo_total !== undefined && n.efetivo_total !== n.efetivo;
    const efetTxt = rec
      ? `<span style="color:var(--verde-claro);font-weight:700">${n.efetivo_total}</span> <small style="color:var(--tx3)">(próprio: ${n.efetivo})</small>`
      : `<span style="font-weight:600">${n.efetivo}</span>`;

    const isFocado = FOCO_GRUPO_ID === n.id;
    const margemEsq = Math.min(nivel * 20, 160);

    const corBordaEsq = isFocado ? 'var(--verde-claro)' : nivel === 0 ? '#10b981' : temFilhos ? '#3b82f6' : '#8b5cf6';
    const tagNivel = nivel === 0 ? '🏛️ Unidade Raiz' : `🌲 Nível ${nivel + 1}`;

    return `
      <div class="grupo-node-card" data-gid="${n.id}"
           style="margin-left:${margemEsq}px; margin-bottom:12px; background:linear-gradient(135deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%), var(--painel2); border:1px solid ${isFocado ? 'var(--verde-claro)' : 'var(--borda)'}; border-left:5px solid ${corBordaEsq}; border-radius:10px; padding:14px 16px; box-shadow:${isFocado ? '0 0 16px rgba(16,185,129,0.25)' : '0 3px 12px rgba(0,0,0,0.25)'}; transition:all 0.2s ease">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px">
          <div style="display:flex; align-items:center; gap:10px; flex:1; min-width:250px">
            ${temFilhos
              ? `<button type="button" data-tgl="${idCollapse}" style="background:var(--painel3); border:1px solid var(--borda); color:var(--tx); border-radius:6px; width:28px; height:28px; cursor:pointer; font-weight:bold; display:flex; align-items:center; justify-content:center; padding:0; flex-shrink:0; font-size:13px; transition:transform 0.15s ease" title="Expandir/Recolher subgrupos">▾</button>`
              : `<span style="width:28px; display:inline-block; text-align:center; color:var(--tx3); font-size:14px; flex-shrink:0">•</span>`}
            
            <div style="cursor:pointer" data-focargrupo="${n.id}" title="Clique para expandir e focar nesta unidade e seus subordinados">
              <div style="display:flex; align-items:center; gap:6px; flex-wrap:wrap">
                <span style="font-size:15px; font-weight:700; color:${isFocado ? 'var(--verde-claro)' : 'var(--tx)'}; letter-spacing:0.2px">
                  ${esc(n.nome)}
                </span>
                <span style="font-size:11px; color:var(--tx3); font-family:monospace">#${n.id}</span>
                <span>${codigoChip(n.codigo)}</span>
                <span style="font-size:10.5px; font-weight:600; padding:1px 6px; border-radius:4px; background:rgba(255,255,255,0.06); color:var(--tx3); border:1px solid var(--borda)">${tagNivel}</span>
              </div>
            </div>
          </div>

          <div style="display:flex; align-items:center; gap:6px; font-size:12px; color:var(--tx2); flex-wrap:wrap">
            ${comGerente ? `<span style="background:var(--painel3); padding:4px 9px; border-radius:6px; border:1px solid var(--borda); display:flex; align-items:center; gap:4px">👤 <b>${esc(n.gerente || 'Sem gerente')}</b></span>` : ''}
            <span style="background:var(--painel3); padding:4px 9px; border-radius:6px; border:1px solid var(--borda)">👥 Efetivo: ${efetTxt}</span>
            <span style="background:var(--painel3); padding:4px 9px; border-radius:6px; border:1px solid var(--borda)">🔑 ${n.contas || 0} conta(s)</span>
            ${temFilhos ? `<span style="background:rgba(59,130,246,0.15); color:#60a5fa; padding:4px 9px; border-radius:6px; border:1px solid rgba(59,130,246,0.3); font-weight:600">🌲 ${n.filhos.length} subgrupo(s)</span>` : ''}
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:6px; margin-top:10px; border-top:1px solid rgba(255,255,255,0.06); padding-top:10px; flex-wrap:wrap">
          <button class="acao-linha" style="font-size:12px; padding:4px 10px; font-weight:600" data-focargrupo="${n.id}">🔍 Focar / Drilldown</button>
          <button class="acao-linha" style="font-size:12px; padding:4px 10px" data-novosub="${n.id}" data-nome="${esc(n.nome)}">+ Subgrupo</button>
          <button class="acao-linha" style="font-size:12px; padding:4px 10px" data-trocarger="${n.id}" data-nome="${esc(n.nome)}">👤 Trocar Gerente</button>
          <button class="acao-linha" style="font-size:12px; padding:4px 10px" data-subordinar="${n.id}" data-nome="${esc(n.nome)}">⛓️ Subordinação</button>
          <button class="acao-linha" style="font-size:12px; padding:4px 10px" data-contas="${n.id}" data-nome="${esc(n.nome)}">📊 Auditar</button>
          <button class="acao-linha perigo" style="font-size:12px; padding:4px 10px" data-excluir="${n.id}" data-nome="${esc(n.nome)}">🗑️ Excluir</button>
        </div>

        ${temFilhos ? `
          <div id="${idCollapse}" class="subgrupos-container" style="margin-top:10px; border-left:2px dashed rgba(16,185,129,0.35); padding-left:10px; transition:all 0.2s ease">
            ${n.filhos.map(f => noCardHTML(f, nivel + 1, comGerente, arvoreTotal)).join('')}
          </div>
        ` : ''}
      </div>
    `;
  };

  const arvoreHTML = (nos, comGerente) => {
    if (!nos || !nos.length) return '<div class="vazio" style="padding:16px">Nenhum grupo cadastrado na estrutura.</div>';
    
    let htmlBreadcrumb = '';
    let nosParaExibir = nos;

    if (FOCO_GRUPO_ID) {
      const noFocado = buscarNoPorId(nos, FOCO_GRUPO_ID);
      const caminho = obterCaminhoAncestrais(nos, FOCO_GRUPO_ID) || [];
      if (noFocado) {
        nosParaExibir = [noFocado];
        htmlBreadcrumb = `
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; background:linear-gradient(90deg, rgba(16,185,129,0.12) 0%, rgba(16,185,129,0.03) 100%); border:1px solid rgba(16,185,129,0.35); border-radius:10px; padding:12px 16px; margin-bottom:16px">
            <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap; font-size:13.5px">
              <span style="color:var(--tx3); font-weight:600">📍 Visualização Focada:</span>
              <button class="acao-linha" style="font-size:12px; padding:3px 10px; border-radius:6px" data-focargrupo="0">🏛️ Raiz Geral</button>
              ${caminho.map((p, idx) => `
                <span style="color:var(--tx3)">›</span>
                <button class="acao-linha ${idx === caminho.length - 1 ? 'primario' : ''}" style="font-size:12px; padding:3px 10px; border-radius:6px" data-focargrupo="${p.id}">
                  ${esc(p.nome)}
                </button>
              `).join('')}
            </div>
            <button class="acao-linha" data-focargrupo="0" style="font-size:12px; padding:4px 12px">🌐 Ver Estrutura Completa</button>
          </div>
        `;
      }
    }

    return htmlBreadcrumb + nosParaExibir.map(n => noCardHTML(n, 0, comGerente, nos)).join('');
  };

  function ligarToggles(raiz) {
    (raiz || document).querySelectorAll('[data-tgl]').forEach(s => s.onclick = (e) => {
      e.stopPropagation();
      const alvo = document.getElementById(s.dataset.tgl);
      if (!alvo) return;
      const aberto = alvo.style.display !== 'none';
      alvo.style.display = aberto ? 'none' : 'block';
      s.textContent = aberto ? '▸' : '▾';
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
    const abas = [['dashboard', 'Dashboard Global'], ['grupos', 'Estrutura & Grupos'], ['backup', 'Sistema & Backup']];
    $('#app').innerHTML = `<h2>Administração do Sistema</h2>
      <div class="abas">${abas.map(([k, t]) => `<button data-a="${k}" class="${abaAdmin === k ? 'ativo' : ''}">${t}</button>`).join('')}</div>
      <div id="adm"><div class="carregando">…</div></div>`;
    document.querySelectorAll('.abas button').forEach(b => b.onclick = () => { abaAdmin = b.dataset.a; window.ViewAdmin(); });
    if (abaAdmin === 'dashboard') await admDashboard();
    else if (abaAdmin === 'grupos') await admGrupos();
    else admBackup();
  };

  async function admDashboard() {
    try {
      const [grupos, contas, confs] = await Promise.all([api('/api/grupos'), api('/api/usuarios'), api('/api/conferencia/lista').catch(()=>[])]);
      const efetivoTotal = grupos.reduce((acc, g) => acc + (g.efetivo || 0), 0);
      const ativos = contas.filter(c => c.ativo).length;
      const gerentesFaltando = grupos.filter(g => !contas.some(c => c.grupo_id === g.id && c.papel === 'gerente')).length;
      $('#adm').innerHTML = `
        <div class="resumo" style="margin-bottom:14px">
          <div class="caixa"><b>${grupos.length}</b><span>Grupos</span></div>
          <div class="caixa"><b>${efetivoTotal}</b><span>Total de Pessoal (Banco)</span></div>
          <div class="caixa"><b>${contas.length}</b><span>Usuários Registrados</span></div>
          <div class="caixa"><b>${confs.length}</b><span>Conferências</span></div>
        </div>
        <div class="cartao">
          <h3 style="margin-top:0">Saúde da Estrutura</h3>
          <ul style="color:var(--tx2);padding-left:20px;line-height:1.8">
            <li>Proteção de Dados: <span style="color:var(--verde-claro)">Backups Automáticos em Background (Ativo)</span></li>
            <li>Alertas de Gestão: <b style="color:${gerentesFaltando > 0 ? 'var(--verm-txt)' : 'var(--verde-claro)'}">${gerentesFaltando} grupos sem gerente</b></li>
            <li>Controle de Acesso: Bloqueio contra Força Bruta ativo (Limiter em memória isolada).</li>
          </ul>
        </div>`;
    } catch (e) {
      $('#adm').innerHTML = '<span class="vazio">Falha ao carregar dashboard.</span>';
    }
  }

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

  /* --- ADMIN › grupos: árvore nested colapsável + foco recursivo + ações completas --- */
  async function admGrupos() {
    const [grupos, contas, arvore] = await Promise.all([api('/api/grupos'), api('/api/usuarios'), api('/api/grupos/arvore')]);
    const gerenteDe = {};
    contas.filter(c => c.papel === 'gerente' && c.ativo && c.grupo_id).forEach(c => { gerenteDe[c.grupo_id] = c.login; });
    marcarGerente(arvore, gerenteDe);

    const renderConteudo = () => {
      $('#adm').innerHTML = `
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
            <div>
              <h3 style="margin:0 0 4px">Estrutura & Hierarquia Organizacional</h3>
              <p style="color:var(--tx2); font-size:13px; margin:0">Navegue, expanda e foque em unidades e subgrupos recursivamente.</p>
            </div>
            <div style="display:flex; gap:8px; flex-wrap:wrap">
              ${FOCO_GRUPO_ID ? `<button class="acao-linha" id="btVoltarTodos">🌐 Ver Toda a Estrutura</button>` : ''}
              <button class="primario" id="btNovoGrupoRaiz">+ Novo Grupo Raiz</button>
            </div>
          </div>

          <div class="campo" style="margin-bottom:14px">
            <input id="fGNome" placeholder="Filtrar por nome do grupo ou código (#ABC123)…">
          </div>

          <div id="arvoreAdm" style="display:flex; flex-direction:column; gap:2px">
            ${arvoreHTML(arvore, true)}
          </div>
        </div>
      `;

      ligarEventosArvore();
    };

    const ligarEventosArvore = () => {
      ligarToggles($('#arvoreAdm'));

      // Botões de focar em um nó
      $('#arvoreAdm').querySelectorAll('[data-focargrupo]').forEach(b => {
        b.onclick = (e) => {
          e.stopPropagation();
          const gid = +b.dataset.focargrupo;
          FOCO_GRUPO_ID = gid > 0 ? gid : null;
          renderConteudo();
        };
      });

      if ($('#btVoltarTodos')) {
        $('#btVoltarTodos').onclick = () => {
          FOCO_GRUPO_ID = null;
          renderConteudo();
        };
      }

      // Botão de Novo Grupo Raiz
      $('#btNovoGrupoRaiz').onclick = () => modalCriarGrupo(null);

      // Botões de Novo Subgrupo
      $('#arvoreAdm').querySelectorAll('[data-novosub]').forEach(b => {
        b.onclick = () => {
          const supId = +b.dataset.novosub;
          const supNome = b.dataset.nome;
          modalCriarGrupo(supId, supNome);
        };
      });

      // Botões de Trocar Gerente
      $('#arvoreAdm').querySelectorAll('[data-trocarger]').forEach(b => {
        b.onclick = () => {
          const gid = +b.dataset.trocarger;
          const gnome = b.dataset.nome;
          modalTrocarGerente(gid, gnome);
        };
      });

      // Botões de Subordinação
      $('#arvoreAdm').querySelectorAll('[data-subordinar]').forEach(b => {
        b.onclick = () => {
          const gid = +b.dataset.subordinar;
          const gnome = b.dataset.nome;
          modalGerenciarSubordinacao(gid, gnome);
        };
      });

      // Botões de Auditar Contas
      $('#arvoreAdm').querySelectorAll('[data-contas]').forEach(b => {
        b.onclick = () => {
          const gid = +b.dataset.contas;
          const nome = b.dataset.nome;
          modalAuditarContas(gid, nome);
        };
      });

      // Botões de Excluir
      $('#arvoreAdm').querySelectorAll('[data-excluir]').forEach(b => {
        b.onclick = () => {
          const gid = +b.dataset.excluir;
          const gnome = b.dataset.nome;
          modalExcluirGrupo(gid, gnome);
        };
      });

      // Busca na árvore
      $('#fGNome').oninput = () => {
        const q = $('#fGNome').value.trim().toLowerCase();
        if (!q) {
          $('#arvoreAdm').innerHTML = arvoreHTML(arvore, true);
          ligarEventosArvore();
          return;
        }
        const mostrar = n => {
          const eu = n.nome.toLowerCase().includes(q) || (n.codigo || '').toLowerCase().includes(q);
          const filhos = (n.filhos || []).map(f => mostrar(f)).filter(Boolean);
          if (!eu && !filhos.length) return null;
          return { ...n, filhos: filhos.length ? filhos : (n.filhos || []) };
        };
        const filtrada = (arvore || []).map(mostrar).filter(Boolean);
        $('#arvoreAdm').innerHTML = arvoreHTML(filtrada, true);
        $('#arvoreAdm').querySelectorAll('.subgrupos-container').forEach(d => { d.style.display = 'block'; });
        $('#arvoreAdm').querySelectorAll('[data-tgl]').forEach(s => { s.textContent = '▾'; });
        ligarEventosArvore();
      };
    };

    function modalCriarGrupo(superiorId, superiorNome) {
      const html = `
        <div class="modal" style="max-width:520px">
          <h3 style="margin-top:0">${superiorId ? `Criar Subgrupo subordinado a "${esc(superiorNome)}"` : 'Criar Novo Grupo Raiz'}</h3>
          <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Cada grupo exige um gerente no ato da criação para assegurar a cadeia de comando.</p>
          
          <div class="campo" style="margin-bottom:8px">
            <label>Nome da Unidade / Subgrupo *</label>
            <input id="gNome" placeholder="ex.: 1ª Companhia / 1º Pelotão">
          </div>
          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Login do Gerente *</label>
              <input id="gLogin" placeholder="ex.: silva.gerente">
            </div>
            <div class="campo" style="flex:1">
              <label>Nome de Guerra *</label>
              <input id="gGuerra" placeholder="ex.: SILVA">
            </div>
          </div>
          <div class="campo" style="margin-bottom:14px">
            <label>Senha do Gerente (mín. 8 caracteres) *</label>
            <input id="gSenha" type="password" placeholder="••••••••">
          </div>

          <div style="display:flex; gap:8px; justify-content:flex-end">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button class="primario" id="gGo">Criar Unidade</button>
          </div>
        </div>
      `;
      const m = modal(html);
      m.querySelector('#gGo').onclick = async () => {
        const nome = m.querySelector('#gNome').value.trim();
        const login = m.querySelector('#gLogin').value.trim();
        const senha = m.querySelector('#gSenha').value;
        const guerra = m.querySelector('#gGuerra').value.trim();

        if (!nome || !login || senha.length < 8 || !guerra) {
          toast('Preencha todos os campos obrigatórios (senha mín. 8 caracteres)', 'erro');
          return;
        }
        try {
          const r = await api('/api/grupos', {
            method: 'POST',
            body: JSON.stringify({ nome, login, senha, nome_guerra: guerra })
          });
          const novoId = r.id;
          if (superiorId && novoId) {
            await api('/api/admin/grupos/vinculo', {
              method: 'POST',
              body: JSON.stringify({ superior_id: superiorId, subordinado_id: novoId })
            });
          }
          toast(`Unidade criada com sucesso (código ${r.codigo})`);
          m.remove();
          window.ViewAdmin();
        } catch (e) {}
      };
    }

    function modalTrocarGerente(gid, gnome) {
      const contasGrupo = contas.filter(c => c.grupo_id === gid && c.papel !== 'admin' && c.ativo);
      const html = `
        <div class="modal" style="max-width:480px">
          <h3 style="margin-top:0">👤 Trocar Gerente — ${esc(gnome)}</h3>
          <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Selecione uma conta existente deste grupo para ser promovida a Gerente.</p>
          
          <div class="campo" style="margin-bottom:14px">
            <label>Conta a ser promovida</label>
            <select id="mSelNovaConta">
              <option value="">— Selecione uma conta do grupo —</option>
              ${contasGrupo.map(c => `<option value="${esc(c.login)}">${esc(c.login)} (${rotuloPapel(c.papel)}) - ${esc(c.nome_guerra || c.login)}</option>`).join('')}
            </select>
          </div>

          <div style="display:flex; justify-content:flex-end; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button class="primario" id="mBtnTrocarGer">Promover a Gerente</button>
          </div>
        </div>
      `;
      const m = modal(html);
      m.querySelector('#mBtnTrocarGer').onclick = async () => {
        const login = m.querySelector('#mSelNovaConta').value;
        if (!login) { toast('Selecione a conta para promover', 'erro'); return; }
        try {
          await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login }) });
          toast('Gerente atualizado com sucesso!');
          m.remove();
          window.ViewAdmin();
        } catch (e) {}
      };
    }

    function modalGerenciarSubordinacao(gid, gnome) {
      const gAtual = grupos.find(x => x.id === gid);
      const sups = (gAtual && gAtual.superiores_ids) || [];
      const supAtualId = sups[0] || null;

      const html = `
        <div class="modal" style="max-width:500px">
          <h3 style="margin-top:0">⛓️ Subordinação — ${esc(gnome)}</h3>
          <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Defina a qual unidade superior este grupo reporta na cadeia de comando.</p>

          <div class="campo" style="margin-bottom:14px">
            <label>Unidade Superior (Comando Direto)</label>
            <select id="mSelSuperior">
              <option value="">— Sem Superior (Nível Raiz) —</option>
              ${grupos.filter(x => x.id !== gid).map(x => `<option value="${x.id}" ${x.id === supAtualId ? 'selected' : ''}>${esc(x.nome)} (#${x.codigo})</option>`).join('')}
            </select>
          </div>

          <div style="display:flex; justify-content:space-between; align-items:center; margin-top:16px">
            ${supAtualId ? `<button type="button" class="perigo" id="mBtnRemoverSub">Desvincular Superior</button>` : '<div></div>'}
            <div style="display:flex; gap:8px">
              <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
              <button class="primario" id="mBtnSalvarSub">Salvar Subordinação</button>
            </div>
          </div>
        </div>
      `;
      const m = modal(html);

      m.querySelector('#mBtnSalvarSub').onclick = async () => {
        const novoSupId = +m.querySelector('#mSelSuperior').value;
        try {
          if (supAtualId && supAtualId !== novoSupId) {
            await api(`/api/admin/grupos/vinculo?superior_id=${supAtualId}&subordinado_id=${gid}`, { method: 'DELETE', body: '{}' });
          }
          if (novoSupId > 0) {
            await api('/api/admin/grupos/vinculo', { method: 'POST', body: JSON.stringify({ superior_id: novoSupId, subordinado_id: gid }) });
          }
          toast('Subordinação atualizada');
          m.remove();
          window.ViewAdmin();
        } catch (e) {}
      };

      if (m.querySelector('#mBtnRemoverSub')) {
        m.querySelector('#mBtnRemoverSub').onclick = async () => {
          try {
            await api(`/api/admin/grupos/vinculo?superior_id=${supAtualId}&subordinado_id=${gid}`, { method: 'DELETE', body: '{}' });
            toast('Subordinação removida');
            m.remove();
            window.ViewAdmin();
          } catch (e) {}
        };
      }
    }

    function modalAuditarContas(gid, nome) {
      const cGrupo = contas.filter(c => c.grupo_id === gid);
      const html = `
        <div class="modal" style="max-width:800px; max-height:85vh; display:flex; flex-direction:column">
          <h3 style="margin-top:0">📊 Contas & Usuários: ${esc(nome)}</h3>
          <div style="overflow-y:auto; flex:1">
            <table style="width:100%; margin-top:8px">
              <thead><tr><th>ID</th><th>Login</th><th>Nome de Guerra</th><th>Papel</th><th>Status</th></tr></thead>
              <tbody>
                ${cGrupo.length ? cGrupo.map(c => `
                  <tr>
                    <td>#${c.id}</td>
                    <td><b>${esc(c.login)}</b></td>
                    <td>${esc(c.nome_guerra || '—')}</td>
                    <td>${rotuloPapel(c.papel)}</td>
                    <td><span style="color:${c.ativo ? 'var(--verde-claro)' : 'var(--tx3)'}; font-weight:700">${c.ativo ? 'Ativa' : 'Desativada'}</span></td>
                  </tr>
                `).join('') : '<tr><td colspan="5"><span class="vazio">Nenhuma conta cadastrada neste grupo.</span></td></tr>'}
              </tbody>
            </table>
          </div>
          <div style="margin-top:16px; text-align:right; border-top:1px solid var(--borda); padding-top:10px">
            <button class="primario" onclick="this.closest('.modal-mask').remove()">Fechar</button>
          </div>
        </div>
      `;
      modal(html);
    }

    function modalExcluirGrupo(gid, gnome) {
      const div = modal(`
        <div class="modal-inner">
          <h3>Excluir grupo — ${esc(gnome)}</h3>
          <p style="color:var(--tx2);font-size:13px;margin:6px 0">Exclusão normal: só é permitida se o grupo estiver vazio.</p>
          <div class="modal-acoes">
            <button class="fantasma" id="mX">Cancelar</button>
            <button class="perigo" id="mGo">Tentar exclusão normal</button>
          </div>
          <div style="border-top:1px solid var(--borda);margin-top:12px;padding-top:10px">
            <p style="color:var(--verm);font-size:13px;margin:4px 0"><b>Exclusão FORÇADA</b> — remove o grupo INTEIRO mesmo com contas e pessoal (tudo é apagado). Grupos com histórico de conferências NUNCA são excluídos. Requer sua senha de admin.</p>
            <div class="campo"><label>Senha de admin</label><input type="password" id="fSenha"></div>
            <div class="modal-acoes"><button class="perigo" id="mForce">Excluir forçadamente</button></div>
          </div>
          <div style="border-top:1px solid var(--verm);margin-top:12px;padding-top:10px">
            <p style="color:var(--verm);font-size:13px;margin:4px 0"><b>☢️ MODO NUKE</b> — exclusão TOTAL e IRREVERSÍVEL: apaga o grupo, contas, pessoal, catálogos e <b>TODO o histórico de conferências</b> (presenças e comentários). Sem backup de seleção — restaure pelo backup do sistema se algo der errado.</p>
            <div class="modal-acoes"><button class="perigo" id="mNuke">☢️ NUKE — apagar TUDO</button></div>
          </div>
        </div>
      `);
      if (!div) return;
      div.querySelector('#mX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#mGo').onclick = async () => {
        try {
          await api(`/api/grupos/${gid}`, { method: 'DELETE' });
          toast('Grupo excluído');
          div.fechar && div.fechar();
          window.ViewAdmin();
        } catch (e) {}
      };
      div.querySelector('#mForce').onclick = async () => {
        const senha = div.querySelector('#fSenha').value;
        if (!senha) { toast('Digite a senha de admin', 'erro'); return; }
        try {
          const r = await api(`/api/grupos/${gid}?forcar=1`, { method: 'DELETE', body: JSON.stringify({ senha }) });
          toast(`Grupo excluído forçadamente — ${r.contas_removidas} conta(s), ${r.pessoas_removidas} pessoa(s) removidas`);
          div.fechar && div.fechar();
          window.ViewAdmin();
        } catch (e) {}
      };
      // ordem Tenente 30/09: NUKE = DUPLO modal de verificação antes do pedido
      div.querySelector('#mNuke').onclick = async () => {
        if (!(await confirmar(`☢️ Quer fazer isso mesmo? Isto APAGA "${gnome}" — contas, pessoal, catálogos e TODO o histórico de conferências. Não tem volta.`))) return;
        modalNukeFinal(gid, gnome, div);
      };
    }

    /* 2º modal do NUKE (ordem Tenente 30/09): "TEM CERTEZA?" + senha de admin */
    function modalNukeFinal(gid, gnome, divAnterior) {
      const div = modal(`
        <div class="modal-inner">
          <h3 style="color:var(--verm)">TEM CERTEZA?</h3>
          <p style="color:var(--tx2);font-size:13px;margin:6px 0">Última confirmação: <b>${esc(gnome)}</b> será APAGADO com todo o histórico, para sempre. Digite sua senha de admin para autorizar.</p>
          <div class="campo"><label>Senha de admin</label><input type="password" id="fSenhaNuke" autocomplete="off"></div>
          <div class="modal-acoes">
            <button class="fantasma" id="mNX">Cancelar</button>
            <button class="perigo" id="mNGo">☢️ APAGAR TUDO AGORA</button>
          </div>
        </div>
      `);
      if (!div) return;
      div.querySelector('#mNX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#mNGo').onclick = async () => {
        const senha = div.querySelector('#fSenhaNuke').value;
        if (!senha) { toast('Digite a senha de admin', 'erro'); return; }
        try {
          const r = await api(`/api/grupos/${gid}?nuke=1`, { method: 'DELETE', body: JSON.stringify({ senha }) });
          toast(`☢️ Grupo apagado (NUKE) — ${r.contas_removidas} conta(s), ${r.pessoas_removidas} pessoa(s), ${r.conferencias_removidas} conferência(s)`);
          if (divAnterior && divAnterior.fechar) divAnterior.fechar();
          div.fechar && div.fechar();
          window.ViewAdmin();
        } catch (e) {}
      };
      const inp = div.querySelector('#fSenhaNuke');
      if (inp) inp.focus();
    }

    renderConteudo();
  }

  /* --- ADMIN › backup: gerar download .db + IMPORTAR upload com confirmação --- */
  function admBackup() {
    const anoAtual = new Date().getFullYear();
    $('#adm').innerHTML = `
      <div class="cartao"><h3 style="margin-top:0">Backup Integral do Sistema</h3>
        <p style="color:var(--tx2);font-size:13px">O SCI faz backup automático a cada conferência fechada e no boot
        (<code>VACUUM INTO</code> + SHA-256 + MANIFEST). Aqui você força uma cópia agora — o download do .db começa em seguida.</p>
        <button class="primario" id="bkGo">Fazer backup agora</button>
        <pre id="bkOut" style="margin-top:10px;color:var(--tx2);font-size:12px"></pre>
      </div>

      <!-- Exportação Granular e Retenção Anual (Dual Mode) -->
      <div class="cartao">
        <h3 style="margin-top:0; display:flex; align-items:center; gap:8px">
          <span>📦</span> <span>Exportação Avançada & Retenção Anual de Dados</span>
        </h3>
        <p style="color:var(--tx2);font-size:13px; margin-bottom:14px">
          Extraia recortes específicos dos registros do sistema com total flexibilidade (exercício anual, período personalizado ou módulo).
          Você pode escolher entre formato analítico aberto (JSON para auditoria/BI) ou banco de dados relacional frio (SQLite desanexado).
        </p>

        <div class="form-linha" style="gap:10px; margin-bottom:12px; flex-wrap:wrap">
          <div class="campo" style="max-width:140px">
            <label>Ano de Exercício</label>
            <input type="number" id="fExpAno" value="${anoAtual}" min="2020" max="2050">
          </div>
          <div class="campo" style="max-width:160px">
            <label>Data Inicial (Opcional)</label>
            <input type="date" id="fExpDe">
          </div>
          <div class="campo" style="max-width:160px">
            <label>Data Final (Opcional)</label>
            <input type="date" id="fExpAte">
          </div>
          <div class="campo" style="flex:1; min-width:200px">
            <label>Módulo / Tabela Alvo</label>
            <select id="fExpTabela">
              <option value="tudo">Todos os Módulos (Conferências + Reserva de Material)</option>
              <option value="conferencias">Apenas Conferências de Pessoal & Presenças</option>
              <option value="material">Apenas Reserva de Material & Cautelas</option>
            </select>
          </div>
        </div>

        <div style="display:flex; gap:10px; flex-wrap:wrap; margin-top:16px">
          <button class="primario" id="btExpJSON" style="box-shadow: 0 2px 8px rgba(16,185,129,0.25)">
            📥 Exportar Relatório Analítico (JSON)
          </button>
          <button class="primario" id="btExpSQLite" style="background:#1e293b; border-color:#334155; box-shadow: 0 2px 8px rgba(0,0,0,0.3)">
            💾 Exportar Base Histórica Desanexada (SQLite .db)
          </button>
        </div>
      </div>

      <div class="cartao"><h3 style="margin-top:0">Importar backup (.db)</h3>
        <p style="color:var(--tx2);font-size:13px">Valida o arquivo (SQLite + integridade + versão de schema), grava um backup de segurança do estado atual e troca o banco. Arquivo inválido é rejeitado sem tocar em nada.</p>
        <input type="file" id="bkArq" accept=".db">
        <button class="primario" id="bkImp" style="margin-top:8px">Importar</button>
        <pre id="bkImpOut" style="margin-top:10px;color:var(--tx2);font-size:12px"></pre>
      </div>
    `;

    $('#bkGo').onclick = async () => {
      const r = await api('/api/backup', { method: 'POST', body: '{}' });
      $('#bkOut').textContent = `${r.arquivo}\nsha256: ${r.sha256}\n→ download iniciado no navegador`;
      toast('Backup gerado — download iniciado');
      window.location.href = '/api/backup/download?nome=' + encodeURIComponent(r.arquivo.split('/').pop());
    };

    const obterUrlExport = (tipo) => {
      const ano = $('#fExpAno').value.trim();
      const de = $('#fExpDe').value.trim();
      const ate = $('#fExpAte').value.trim();
      const tabela = $('#fExpTabela').value;
      const params = new URLSearchParams({ tipo: tipo, tabela: tabela });
      if (de && ate) {
        params.set('de', de);
        params.set('ate', ate);
      } else if (ano) {
        params.set('ano', ano);
      }
      return '/api/export?' + params.toString();
    };

    $('#btExpJSON').onclick = () => {
      toast('Gerando exportação analítica JSON…');
      window.location.href = obterUrlExport('json');
    };

    $('#btExpSQLite').onclick = () => {
      toast('Gerando base SQLite desanexada…');
      window.location.href = obterUrlExport('sqlite');
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
       <td>${esc(p.setor || 'INDEFINIDO')}</td>
       <td>${esc(p.funcao || 'INDEFINIDO')}</td>
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
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; margin-bottom:12px">
            <div>
              <h3 style="margin:0 0 4px">Catálogos & TAGS Organizacionais</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:0">Gerencie Tags, Setores, Funções e Destinos. Superiores podem editar itens próprios e de subordinados; itens de superiores são somente leitura (🔒).</p>
            </div>
          </div>

          <!-- Formulário de Adição no Topo -->
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:14px; margin-bottom:16px">
            <h4 style="margin:0 0 10px; font-size:13.5px; display:flex; align-items:center; gap:6px">
              <span>➕</span> <span>Adicionar Novo Item ao Catálogo</span>
            </h4>
            <div class="form-linha" style="align-items:flex-end">
              <div class="campo" style="width:160px; margin:0">
                <label>Tipo de Catálogo</label>
                <select id="cgT">
                  <option value="tags">🏷️ Tags</option>
                  <option value="setores">🏢 Setores / Seções</option>
                  <option value="funcoes">💼 Funções</option>
                  <option value="destinos">📍 Destinos</option>
                </select>
              </div>
              <div class="campo" style="flex:1; margin:0">
                <label>Nome do Item</label>
                <input id="cgN" placeholder="ex.: MISSÃO EXTERNA, Comandante, Almoxarifado…">
              </div>
              <div class="campo" id="cgCorWrap" style="width:100px; margin:0">
                <label>Cor (Tag)</label>
                <input type="color" id="cgCor" value="#10b981" style="width:100%; height:40px; padding:2px; cursor:pointer">
              </div>
              <div class="campo" id="cgSiglaWrap" style="width:110px; margin:0; display:none">
                <label>Sigla</label>
                <input id="cgSigla" placeholder="ex.: 1º PEL">
              </div>
              <div class="campo" style="margin:0">
                <button class="primario" id="cgGo" style="min-height:40px; padding:0 22px">Adicionar</button>
              </div>
            </div>
          </div>

          <div id="catGer"><div class="carregando">Carregando catálogos…</div></div>
        </div>
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
      document.querySelectorAll('.abas button[data-g]').forEach(x => x.classList.toggle('ativo', x.dataset.g === abaGer));
      if (abaGer === 'tags') carregarCats(); // v9.16.9: carrega ao clicar (não só na 1ª renderização)
    });
    ligarToggles($('#app'));

    /* --- salvar militar (criar/editar) --- */
    $('#pSalvar').onclick = async () => {
      const corpo = { nome_guerra: $('#pNg').value.trim(), nome_completo: $('#pNc').value.trim(),
        setor_id: +$('#pSetor').value || null, funcao_id: +$('#pFuncao').value || null, status: $('#pStatus').value };
      if (!corpo.nome_guerra || !corpo.nome_completo) { toast('Nomes obrigatórios', 'erro'); return; }
      const id = $('#pId').value;
      const r = await processar(() => id
        ? api('/api/pessoas/' + id, { method: 'PATCH', body: JSON.stringify(corpo) })
        : api('/api/pessoas', { method: 'POST', body: JSON.stringify(corpo) }),
        id ? 'Salvando alterações…' : 'Cadastrando militar…');
      if (r.ok) window.ViewGrupos();
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

    /* --- aba TAGS: 4 catálogos com nova doutrina hierárquica --- */
    const rotCat = { tags: 'Tags', setores: 'Setores / Seções', funcoes: 'Funções', destinos: 'Destinos' };
    
    // Toggle de campos no form do topo
    const cgTSel = $('#cgT');
    if (cgTSel) {
      cgTSel.onchange = () => {
        const val = cgTSel.value;
        const corWrap = $('#cgCorWrap');
        const siglaWrap = $('#cgSiglaWrap');
        if (corWrap) corWrap.style.display = val === 'tags' ? 'block' : 'none';
        if (siglaWrap) siglaWrap.style.display = val === 'setores' ? 'block' : 'none';
      };
    }

    const carregarCats = async () => {
      const cont = $('#catGer');
      if (!cont) return;
      const tipos = ['tags', 'setores', 'funcoes', 'destinos'];
      const meuGid = (window.ME && window.ME.grupo_id) || null;
      
      // Coletar IDs de grupos subordinados a partir da árvore
      const coletarSubordinados = (no, acc = new Set()) => {
        if (!no) return acc;
        if (Array.isArray(no)) {
          no.forEach(n => coletarSubordinados(n, acc));
          return acc;
        }
        if (no.id && no.id !== meuGid) acc.add(no.id);
        if (no.filhos) no.filhos.forEach(f => coletarSubordinados(f, acc));
        return acc;
      };
      const subsIds = coletarSubordinados(arvore);
      const mapGrupos = {};
      (grupos || []).forEach(g => { mapGrupos[g.id] = g.nome; });

      const cache = { setores: setoresCat, funcoes: funcoesCat };
      const resultados = await Promise.all(tipos.map(t =>
        Array.isArray(cache[t])
          ? Promise.resolve([t, cache[t]])
          : api('/api/catalogo/' + t).then(l => [t, l]).catch(() => [t, []])));
      
      let html = '';
      for (const [t, lista] of resultados) {
        const minhas = lista.filter(x => x.grupo_id === meuGid);
        const subordinadas = lista.filter(x => x.grupo_id && subsIds.has(x.grupo_id));
        const herdadas = lista.filter(x => x.grupo_id !== meuGid && (!x.grupo_id || !subsIds.has(x.grupo_id)));

        const nivelTag = x => {
          let n = 0, pai = x.pai_id;
          while (pai != null) { const p = lista.find(y => y.id === pai); if (!p) break; n++; pai = p.pai_id; }
          return n;
        };

        const chipCorTag = x => t === 'tags' && x.cor
          ? `<span style="display:inline-block; width:12px; height:12px; border-radius:50%; background:${esc(x.cor)}; margin-right:6px; vertical-align:middle; border:1px solid rgba(255,255,255,0.2)"></span>`
          : '';

        const linhaLista = (x, tipoPermissao, nomeOrigem) => {
          const podeGerenciar = tipoPermissao === 'meu' || tipoPermissao === 'subordinado';
          const recuo = (t === 'tags' && tipoPermissao === 'meu') ? `margin-left:${nivelTag(x) * 18}px` : '';

          return `
            <div class="cat-linha ${!podeGerenciar ? 'herdado' : ''}" style="${recuo}; display:flex; align-items:center; gap:8px; padding:8px 10px; margin-bottom:4px; background:var(--painel2); border:1px solid var(--borda); border-radius:6px">
              <small class="num" style="width:34px; color:var(--tx3)">#${x.id}</small>
              <div style="flex:1; display:flex; align-items:center; gap:6px; flex-wrap:wrap">
                ${chipCorTag(x)}
                <span style="font-weight:600">${esc(x.nome)}</span>
                ${x.sigla ? `<code style="font-size:11px; padding:1px 5px; background:var(--painel3); border-radius:4px; color:var(--tx2)">${esc(x.sigla)}</code>` : ''}
                ${!x.ativo ? '<i style="color:var(--tx3); font-size:12px">(inativo)</i>' : ''}
                ${tipoPermissao === 'subordinado' ? `<span style="font-size:10.5px; padding:1px 6px; border-radius:4px; background:rgba(59,130,246,0.15); color:#60a5fa; border:1px solid rgba(59,130,246,0.3)">🌲 ${esc(nomeOrigem || 'Subordinado')}</span>` : ''}
              </div>
              <div style="display:flex; align-items:center; gap:6px">
                ${podeGerenciar ? `
                  <button class="acao-linha" style="font-size:11.5px; padding:2px 8px" data-editcat="${t}" data-cid="${x.id}" data-nome="${esc(x.nome)}" data-cor="${esc(x.cor || '')}" data-sigla="${esc(x.sigla || '')}">editar</button>
                  <button class="acao-linha perigo" style="font-size:11.5px; padding:2px 8px" data-delcat="${t}" data-cid="${x.id}">excluir</button>
                ` : `
                  <span style="color:var(--tx3); font-size:11.5px; display:inline-flex; align-items:center; gap:3px">
                    🔒 ${esc(nomeOrigem || 'Superior')}
                  </span>
                `}
              </div>
            </div>
          `;
        };

        let corpo = '';
        
        // 1. Do Meu Grupo
        corpo += `
          <div class="cat-secao" style="font-weight:700; font-size:12px; color:var(--verde-claro); margin:10px 0 6px; display:flex; align-items:center; gap:6px">
            <span>🛡️ DO MEU GRUPO (${minhas.length})</span>
          </div>
          ${minhas.length ? minhas.sort((a, b) => (a.antiguidade ?? 999) - (b.antiguidade ?? 999) || (a.id - b.id)).map(x => linhaLista(x, 'meu')).join('') : '<span class="vazio" style="padding:6px 0; display:block">— Nenhum item próprio criado —</span>'}
        `;

        // 2. De Subordinados (Superiores têm permissão total de edição/exclusão)
        if (subordinadas.length) {
          corpo += `
            <div class="cat-secao" style="font-weight:700; font-size:12px; color:#60a5fa; margin:14px 0 6px; display:flex; align-items:center; gap:6px">
              <span>🌲 DE GRUPOS SUBORDINADOS (${subordinadas.length})</span>
              <small style="font-weight:400; color:var(--tx3)">— Você pode editar/gerenciar</small>
            </div>
            ${subordinadas.map(x => linhaLista(x, 'subordinado', mapGrupos[x.grupo_id] || ('Grupo #' + x.grupo_id))).join('')}
          `;
        }

        // 3. Herdados de Superiores / Globais (Somente Leitura)
        if (herdadas.length) {
          corpo += `
            <div class="cat-secao" style="font-weight:700; font-size:12px; color:var(--tx3); margin:14px 0 6px; display:flex; align-items:center; gap:6px">
              <span>🔒 HERDADO DE GRUPOS SUPERIORES / GLOBAL (${herdadas.length})</span>
              <small style="font-weight:400; color:var(--tx3)">— Somente Leitura</small>
            </div>
            ${herdadas.map(x => linhaLista(x, 'superior', x.grupo_id ? (mapGrupos[x.grupo_id] || 'Grupo #' + x.grupo_id) : 'Global')).join('')}
          `;
        }

        const btAnt = (t === 'tags' || t === 'setores' || t === 'funcoes') && minhas.length >= 2
          ? `<button class="fantasma" data-ant="${t}" style="min-height:30px; padding:4px 10px; font-size:12px">⚖ Definir antiguidade</button>` : '';

        html += `
          <div class="cat-bloco" style="margin-bottom:20px; background:var(--painel); border:1px solid var(--borda); border-radius:8px; padding:12px 14px">
            <div style="display:flex; justify-content:space-between; align-items:center; gap:8px; border-bottom:1px solid var(--borda); padding-bottom:8px; margin-bottom:8px">
              <b style="font-size:14.5px">${rotCat[t]}</b>
              ${btAnt}
            </div>
            ${corpo}
          </div>
        `;
      }

      cont.innerHTML = html;

      /* --- handlers: editar / excluir / desativar --- */
      cont.querySelectorAll('[data-editcat]').forEach(b => b.onclick = () => {
        const t = b.dataset.editcat, cid = b.dataset.cid, nome = b.dataset.nome, cor = b.dataset.cor, sigla = b.dataset.sigla;
        const div = modal(`
          <div class="modal-inner" style="max-width:440px">
            <h3 style="margin-top:0">Editar ${rotCat[t]}</h3>
            <div class="campo"><label>Nome</label><input id="edNome" value="${esc(nome)}"></div>
            ${t === 'tags' ? `<div class="campo"><label>Cor da Tag</label><input type="color" id="edCor" value="${esc(cor || '#10b981')}" style="width:100%; height:40px; padding:2px; cursor:pointer"></div>` : ''}
            ${t === 'setores' ? `<div class="campo"><label>Sigla</label><input id="edSigla" value="${esc(sigla || '')}"></div>` : ''}
            <div class="modal-acoes">
              <button class="fantasma" id="edX">Cancelar</button>
              <button class="primario" id="edGo">Salvar Alterações</button>
            </div>
          </div>
        `);
        div.querySelector('#edNome').focus();
        div.querySelector('#edX').onclick = () => div.fechar();
        div.querySelector('#edGo').onclick = async () => {
          const novoNome = div.querySelector('#edNome').value.trim();
          if (!novoNome) { toast('Informe o nome', 'erro'); return; }
          const corpo = { nome: novoNome };
          if (t === 'tags' && div.querySelector('#edCor')) corpo.cor = div.querySelector('#edCor').value;
          if (t === 'setores' && div.querySelector('#edSigla')) corpo.sigla = div.querySelector('#edSigla').value.trim();

          const r = await processar(() => api(`/api/catalogo/${t}/${cid}`, { method: 'PATCH', body: JSON.stringify(corpo) }), 'Salvando alterações…');
          if (r.ok) {
            toast('Item atualizado com sucesso!');
            div.fechar();
            if (t === 'setores' || t === 'funcoes') setoresCat = funcoesCat = null;
            carregarCats();
          }
        };
      });

      cont.querySelectorAll('[data-delcat]').forEach(b => b.onclick = async () => {
        if (!(await confirmar('Excluir este item do catálogo?'))) return;
        const r = await processar(() => api(`/api/catalogo/${b.dataset.delcat}/${b.dataset.cid}`, { method: 'DELETE' }), 'Excluindo item…');
        if (r.ok) {
          toast('Item excluído com sucesso');
          if (b.dataset.delcat === 'setores' || b.dataset.delcat === 'funcoes') setoresCat = funcoesCat = null;
          carregarCats();
        }
      });

      cont.querySelectorAll('[data-ant]').forEach(bt => bt.onclick = () => {
        const t = bt.dataset.ant;
        const lista = (resultados.find(r => r[0] === t) || [null, []])[1]
          .filter(x => x.grupo_id === meuGid);
        if (lista.length < 2) return;
        const raiz = lista.filter(x => x.pai_id == null);
        const empilhar = (nos, acc) => nos
          .sort((a, b) => (a.antiguidade ?? 999) - (b.antiguidade ?? 999) || (a.id - b.id))
          .forEach(x => { acc.push(x); empilhar(lista.filter(y => y.pai_id === x.id), acc); });
        let ordem = []; empilhar(raiz, ordem);
        lista.filter(x => !ordem.includes(x)).forEach(x => ordem.push(x));
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
      const payload = { nome };
      if (t === 'tags') payload.cor = $('#cgCor').value;
      if (t === 'setores') payload.sigla = $('#cgSigla').value.trim();

      const r = await processar(() => api('/api/catalogo/' + t, { method: 'POST', body: JSON.stringify(payload) }), 'Adicionando item…');
      if (r.ok) {
        toast('Item adicionado com sucesso!');
        $('#cgN').value = '';
        if ($('#cgSigla')) $('#cgSigla').value = '';
        setoresCat = funcoesCat = null;
        carregarCats();
      }
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
