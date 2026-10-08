/* SCI — views de GESTÃO (redesign): #/admin · #/grupos (Gerenciar) · #/perfil
   window.ViewAdmin / window.ViewGrupos / window.ViewPerfil (async).
   Consome os helpers globais do core: api, esc, toast, fmtData, fmtHora, pill,
   abrirModal (insere .modal-mask no body e devolve o elemento), confirmar(msg) →
   Promise<boolean>, navAtiva(hash). Vanilla, sem build, sem CDN.
   Comportamento copiado do protótipo web/app.js (v9.11.2) — nada pode sumir. */
'use strict';
(function () {
  const $ = s => document.querySelector(s);
  const rotuloPapel = p => p === 'admin' ? 'ADMIN' : p === 'gerente' ? 'GERENTE' : p === 'chefe_setor' ? 'CHEFE DE SETOR' : 'OPERADOR';
  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const ativosDe = l => (l || []).filter(x => x.ativo === 1 || x.ativo === true);

  let abaAdmin = 'dashboard'; // sub-aba corrente do painel admin (persiste na sessão)
  let abaGer = 'pessoal';    // sub-aba corrente do Gerenciar

  /* ---------- ordem 06/10 (item 10): controles de TABELA — ordenação + paginação ----------
     window.tblOrdenar / window.tblPaginar (também via window.tabelaControles). Estado por
     tabela (chave em memória do módulo). Aplicar DEPOIS de o tbody ter as linhas renderizadas:
       tblOrdenar('agregado', table, tbody, [ {sel:'td:nth-child(1)', tipo:'txt'}, ... ]);
       tblPaginar('chefes', table, tbody, 20, tituloEl?);
     - tipo 'num': comparação numérica (ids/quantidades); tipo 'txt': localeCompare pt-BR;
     - clique no TH alterna asc/desc com indicador ▲/▼ (th.sorted-asc / th.sorted-desc);
     - paginação 20/página com controles ‹ 1/N › na div .tbl-pag (inserida após a .rolagem);
     - NÃO usar nos RELATÓRIOS (ordenação hierárquica cravada pelo Diretor). */
  const TBL_ESTADO = {}; // chave → { col, asc } | { pag }

  function tblHeaders(table) { return [...table.querySelectorAll('thead th')]; }
  function tblLinhas(tbody) { return [...tbody.querySelectorAll('tr')]; }
  const eLinhaVazio = tr => tr.querySelector('.vazio, .carregando') !== null;
  function tblOrdenarPor(chave, table, tbody, col, tipo, asc) {
    const linhas = tblLinhas(tbody).filter(tr => !eLinhaVazio(tr) && !tr.classList.contains('conf-toggle-detalhe'));
    const valDe = tr => {
      const td = tr.children[col];
      if (!td) return '';
      if (tipo === 'num') {
        const t = parseFloat((td.dataset.v !== undefined ? td.dataset.v : td.textContent).replace(/[^\d.,-]/g, '').replace(',', '.'));
        return isNaN(t) ? '' : t;
      }
      return td.textContent.trim();
    };
    linhas.sort((a, b) => {
      const va = valDe(a), vb = valDe(b);
      let r;
      if (tipo === 'num') r = (parseFloat(va) || 0) - (parseFloat(vb) || 0);
      else r = String(va).localeCompare(String(vb), 'pt', { sensitivity: 'base' }) || String(va).localeCompare(String(vb));
      return asc ? r : -r;
    });
    linhas.forEach(tr => tbody.appendChild(tr));
    tblHeaders(table).forEach((th, i) => {
      th.classList.remove('sorted-asc', 'sorted-desc');
      if (i === col) th.classList.add(asc ? 'sorted-asc' : 'sorted-desc');
    });
    (TBL_ESTADO[chave] = TBL_ESTADO[chave] || {}).col = col;
    TBL_ESTADO[chave].asc = asc;
    TBL_ESTADO[chave].tipo = tipo;
    TBL_ESTADO[chave].pag = 1; // reordenar volta à primeira página
  }
  function tblPaginarPara(chave, table, tbody, tamPag, estado) {
    const todas = tblLinhas(tbody).filter(tr => !eLinhaVazio(tr) && !tr.classList.contains('conf-toggle-detalhe'));
    const total = todas.length;
    const pags = Math.max(1, Math.ceil(total / tamPag));
    estado.pag = Math.min(Math.max(1, estado.pag || 1), pags);
    todas.forEach((tr, i) => {
      const naPag = Math.floor(i / tamPag) === (estado.pag - 1);
      tr.style.display = naPag ? '' : 'none';
    });
    let div = table.parentElement.parentElement.querySelector('.tbl-pag[data-tbl="' + chave + '"]');
    if (!div) {
      div = document.createElement('div');
      div.className = 'tbl-pag';
      div.dataset.tbl = chave;
      const cartao = table.closest('.cartao') || table.parentElement.parentElement;
      cartao.appendChild(div);
    }
    if (total <= tamPag) { div.innerHTML = '<span class="tbl-pag-info">' + total + ' registro(s)</span>'; return; }
    div.innerHTML =
      '<button type="button" class="tbl-pag-bt" data-pg="prev" ' + (estado.pag <= 1 ? 'disabled' : '') + '>‹</button>' +
      '<span class="tbl-pag-info">' + estado.pag + '/' + pags + ' · ' + total + '</span>' +
      '<button type="button" class="tbl-pag-bt" data-pg="next" ' + (estado.pag >= pags ? 'disabled' : '') + '>›</button>';
    div.querySelectorAll('[data-pg]').forEach(bt => {
      bt.onclick = () => { estado.pag += (bt.dataset.pg === 'next' ? 1 : -1); tblPaginarPara(chave, table, tbody, tamPag, estado); };
    });
    (TBL_ESTADO[chave] = TBL_ESTADO[chave] || {}).pag = estado.pag;
  }
  function tblOrdenar(chave, table, tbody, cols) {
    if (!table || !tbody) return;
    const est = TBL_ESTADO[chave] || (TBL_ESTADO[chave] = {});
    tblHeaders(table).forEach((th, i) => {
      const cfg = (cols || [])[i]; // posicional: {tipo:'num'|'txt'} ou null (coluna sem ordenar, ex. Ações)
      if (!cfg) return;
      th.style.cursor = 'pointer';
      th.title = th.title || 'Clique para ordenar';
      if (th.dataset.tblOrdLigado) return;
      th.dataset.tblOrdLigado = '1';
      th.addEventListener('click', () => {
        const e2 = TBL_ESTADO[chave] || (TBL_ESTADO[chave] = {});
        const asc = !(e2.col === i && e2.asc);
        tblOrdenarPor(chave, table, tbody, i, cfg.tipo || 'txt', asc);
        if (e2._repag) e2._repag();
      });
    });
    if (est.col !== undefined) tblOrdenarPor(chave, table, tbody, est.col, est.tipo || 'txt', est.asc);
  }
  function tblPaginar(chave, table, tbody, tamPag) {
    if (!table || !tbody) return;
    tamPag = tamPag || 20;
    const est = TBL_ESTADO[chave] || (TBL_ESTADO[chave] = {});
    est._repag = () => tblPaginarPara(chave, table, tbody, tamPag, est);
    tblPaginarPara(chave, table, tbody, tamPag, est);
  }
  window.tblOrdenar = tblOrdenar;
  window.tblPaginar = tblPaginar;
  window.tabelaControles = (chave, table, tbody, cols, tamPag) => {
    tblOrdenar(chave, table, tbody, cols);
    if (tamPag) tblPaginar(chave, table, tbody, tamPag);
  };


  /* ---------- blocos compartilhados ---------- */

  // abrirModal (core) insere o .modal-mask no body e devolve o elemento; aqui só
  // completa com fechar por ESC / clique fora, como nos modais do protótipo.
  // Onda UX 0510: repassa opcoes (ex. {largura:'850px'}) ao abrirModal do core.
  function modal(html, opcoes) {
    // v9.16.2 FIX: abrirModal devolve {fechar, mask, modal}; expor o ELEMENTO mask com helper fechar
    const h = abrirModal(html, null, opcoes);
    if (!h) return null;
    const m = h.mask;
    m.fechar = h.fechar;
    m.querySelector = sel => h.modal.querySelector(sel);
    m.querySelectorAll = sel => h.modal.querySelectorAll(sel);
    m.addEventListener('click', ev => { if (ev.target === m) h.fechar(); });
    return m;
  }

  function processarFoto1x1(arquivo, callback) {
    if (!arquivo) return;
    if (!arquivo.type.startsWith('image/')) {
      toast('Selecione um arquivo de imagem válido', 'erro');
      return;
    }
    const leitor = new FileReader();
    leitor.onload = (e) => {
      const img = new Image();
      img.onload = () => {
        const canvas = document.createElement('canvas');
        const tam = 320;
        canvas.width = tam;
        canvas.height = tam;
        const ctx = canvas.getContext('2d');
        const minDim = Math.min(img.width, img.height);
        const sx = (img.width - minDim) / 2;
        const sy = (img.height - minDim) / 2;
        ctx.drawImage(img, sx, sy, minDim, minDim, 0, 0, tam, tam);
        const dataUrl = canvas.toDataURL('image/jpeg', 0.85);
        callback(dataUrl);
      };
      img.src = e.target.result;
    };
    leitor.readAsDataURL(arquivo);
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
    
    // Efetivo (próprio vs total da subárvore)
    const recEf = n.efetivo_total !== undefined && n.efetivo_total !== n.efetivo;
    const efetTxt = recEf
      ? `<span style="color:var(--verde-claro);font-weight:700">${n.efetivo_total}</span> <small style="color:var(--tx3)">(próprio: ${n.efetivo})</small>`
      : `<span style="font-weight:600">${n.efetivo}</span>`;

    // Contas (próprias vs total da subárvore)
    const recCt = n.contas_total !== undefined && n.contas_total !== n.contas;
    const contasTxt = recCt
      ? `<span style="color:var(--verde-claro);font-weight:700">${n.contas_total}</span> <small style="color:var(--tx3)">(próprias: ${n.contas || 0})</small>`
      : `<span style="font-weight:600">${n.contas || 0}</span>`;

    // Subgrupos (diretos vs total no ramo)
    const subTot = n.subgrupos_total !== undefined ? n.subgrupos_total : (n.filhos ? n.filhos.length : 0);
    const subTxt = temFilhos
      ? `<span style="background:rgba(59,130,246,0.15); color:#60a5fa; padding:4px 9px; border-radius:6px; border:1px solid rgba(59,130,246,0.3); font-weight:600">🌲 ${n.filhos.length} direto(s) · ${subTot} no total</span>`
      : '';

    const isFocado = FOCO_GRUPO_ID === n.id;
    const margemEsq = Math.min(nivel * 20, 160);

    const corBordaEsq = isFocado ? 'var(--verde-claro)' : nivel === 0 ? '#10b981' : temFilhos ? '#3b82f6' : '#8b5cf6';
    const tagNivel = nivel === 0 ? '🏛️ Unidade Raiz' : `🌲 Nível ${nivel + 1}`;

    // Regra operacional: ao focar em um grupo, olhar apenas até o nível 2 a partir do ponto de observação
    const maxNivel = FOCO_GRUPO_ID ? 2 : 32;
    const podeExibirFilhos = temFilhos && nivel < maxNivel;

    return `
      <div class="grupo-node-card" data-gid="${n.id}"
           style="margin-left:${margemEsq}px; margin-bottom:12px; background:linear-gradient(135deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%), var(--painel2); border:1px solid ${isFocado ? 'var(--verde-claro)' : 'var(--borda)'}; border-left:5px solid ${corBordaEsq}; border-radius:10px; padding:14px 16px; box-shadow:${isFocado ? '0 0 16px rgba(16,185,129,0.25)' : '0 3px 12px rgba(0,0,0,0.25)'}; transition:all 0.2s ease">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px">
          <div style="display:flex; align-items:center; gap:10px; flex:1; min-width:250px">
            ${podeExibirFilhos
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
            ${comGerente ? (n.gerente ? `<span style="background:var(--painel3); padding:4px 9px; border-radius:6px; border:1px solid var(--borda); display:flex; align-items:center; gap:4px">👤 <b>${esc(n.gerente)}</b></span>` : `<span class="badge-sem-gerente">⚠️ Sem Gerente</span>`) : ''}
            <span style="background:var(--painel3); padding:4px 9px; border-radius:6px; border:1px solid var(--borda)">👥 Efetivo: ${efetTxt}</span>
            <span style="background:var(--painel3); padding:4px 9px; border-radius:6px; border:1px solid var(--borda)">🔑 Contas: ${contasTxt}</span>
            ${subTxt}
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:10px; border-top:1px solid rgba(255,255,255,0.06); padding-top:10px; flex-wrap:wrap">
          <button class="acao-linha" style="font-size:12px; padding:4px 12px; font-weight:600" data-focargrupo="${n.id}">🔍 Focar / Drilldown</button>
          <button class="primario" style="font-size:12px; padding:4px 14px; font-weight:600" data-editargrupo="${n.id}">⚙️ Editar</button>
        </div>

        ${podeExibirFilhos ? `
          <div id="${idCollapse}" class="subgrupos-container" style="margin-top:10px; border-left:2px dashed rgba(16,185,129,0.35); padding-left:10px; transition:all 0.2s ease">
            ${n.filhos.map(f => noCardHTML(f, nivel + 1, comGerente, arvoreTotal)).join('')}
          </div>
        ` : (temFilhos && nivel >= maxNivel ? `
          <div style="margin-top:8px; font-size:11.5px; color:var(--tx3); font-style:italic; padding-left:10px">
            ↳ Contém ${n.filhos.length} subgrupo(s) em níveis inferiores (clique em <b>Focar</b> para explorar este ramo).
          </div>
        ` : '')}
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
        const superiorImediato = caminho.length > 1 ? caminho[caminho.length - 2] : null;
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
            ${superiorImediato ? `
              <div style="display:flex; align-items:center; gap:6px; font-size:12px; color:var(--tx2)">
                <span>⬆️ Superior:</span>
                <button class="acao-linha" style="font-size:11.5px; padding:3px 9px" data-focargrupo="${superiorImediato.id}">
                  ${esc(superiorImediato.nome)}
                </button>
              </div>
            ` : ''}
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
    // ITEM 2 (ordem Diretor 06/10): login é sensível — para não-admin o título
    // resolve pelo nome de guerra/completo nas listas da view corrente (admUsuarios
    // ou operadores do grupo); sem lista disponível, cai no login (nunca inventado).
    let titulo = login;
    if ((quem() || {}).papel !== 'admin') {
      const alvo = (typeof lista !== 'undefined' && Array.isArray(lista) && lista.find(x => String(x.login) === String(login)))
        || (typeof operadores !== 'undefined' && Array.isArray(operadores) && operadores.find(x => String(x.login) === String(login)))
        || null;
      if (alvo) titulo = alvo.nome_guerra || alvo.nome_completo || login;
    }
    const div = modal(`<div class="modal-inner"><h3>Redefinir senha — ${esc(titulo)}</h3>
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
    // ITEM 2: mesmo tratamento do modalSenha — não-admin vê nome, não login.
    let tituloMv = login;
    if ((quem() || {}).papel !== 'admin') {
      const alvo = (typeof lista !== 'undefined' && Array.isArray(lista) && lista.find(x => String(x.login) === String(login)))
        || (typeof operadores !== 'undefined' && Array.isArray(operadores) && operadores.find(x => String(x.login) === String(login)))
        || null;
      if (alvo) tituloMv = alvo.nome_guerra || alvo.nome_completo || login;
    }
    const div = modal(`<div class="modal-inner"><h3>Mover conta — ${esc(tituloMv)}</h3>
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
    const abas = [
      ['dashboard', 'Dashboard Global'],
      ['usuarios', 'Usuários & Funções'],
      ['grupos', 'Estrutura & Grupos'],
      ['backup', 'Sistema & Backup']
    ];
    // ordem 04/10: abas do módulo → DROPDOWN estilizado
    $('#app').innerHTML = `<h2>Administração do Sistema</h2>
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:6px">
        <span style="font-size:12px;color:var(--tx2)">Seção:</span>
        <div id="abasAdminDD" style="min-width:230px"></div></div>
      <div id="adm"><div class="carregando">…</div></div>`;
    if (typeof criarDropdown === 'function') {
      criarDropdown($('#abasAdminDD'), abas.map(([k, t]) => ({ valor: k, rotulo: t })), {
        valorPadrao: abaAdmin, onChange: (k) => { abaAdmin = k; window.ViewAdmin(); }
      });
    }
    if (abaAdmin === 'dashboard') await admDashboard();
    else if (abaAdmin === 'usuarios') await admUsuarios();
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

  /* --- ADMIN › usuários: Gestão Completa de Contas, Nomes e Multi-Funções --- */
  async function admUsuarios() {
    const [lista, grupos, funcoesRes, setoresRes] = await Promise.all([
      api('/api/usuarios'),
      api('/api/grupos'),
      api('/api/catalogo/funcoes').catch(() => []),
      api('/api/catalogo/setores').catch(() => [])
    ]);
    const funcoesLista = Array.isArray(funcoesRes) ? funcoesRes : (funcoesRes.funcoes || []);
    const setoresLista = (Array.isArray(setoresRes) ? setoresRes : []).filter(s => s.ativo !== false);
    const nomeGrupo = gid => (grupos.find(g => g.id === gid) || {}).nome || '—';
    // ITEM 2 (ordem Diretor 06/10): login é identificação SENSÍVEL — só o admin
    // vê na UI. Gerente/encarregado/operador veem nome de guerra (ou completo).
    const isAdminViewer = (quem() || {}).papel === 'admin';
    const rotuloConta = u => esc(u.nome_guerra || u.nome_completo || (isAdminViewer ? u.login : '—'));
    const optsGrupos = `<option value="">— Sem grupo (Global / Atribuir depois) —</option>` + grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');

    function formatarNomeUsuario(completo, guerra) {
      const c = (completo || '').trim();
      const g = (guerra || '').trim();
      if (!c && !g) return '<span style="color:var(--tx3)">—</span>';
      if (!c) return `<b>${esc(g)}</b>`;
      if (!g) return esc(c);
      const idx = c.toLowerCase().indexOf(g.toLowerCase());
      if (idx !== -1) {
        return `${esc(c.substring(0, idx))}<b>${esc(c.substring(idx, idx + g.length))}</b>${esc(c.substring(idx + g.length))}`;
      }
      return `${esc(c)} (<b>${esc(g)}</b>)`;
    }

    $('#adm').innerHTML = `
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
          <div>
            <h3 style="margin:0 0 4px">Gestão de Usuários & Multi-Funções</h3>
            <p style="color:var(--tx2); font-size:13px; margin:0">Gerencie logins, nomes militares e atribua múltiplos papéis/funções por unidade (cadeiras de comando e operação).</p>
          </div>
          <button class="primario" id="btNovoUsuario">+ Novo Usuário</button>
        </div>

        <div class="form-linha" style="margin-bottom:12px; gap:8px; flex-wrap:wrap">
          <div class="campo" style="flex:1; min-width:180px">
            <label>Buscar usuário</label>
            <input id="fULogin" placeholder="${isAdminViewer ? 'Filtrar por login, nome de guerra ou completo…' : 'Filtrar por nome…'}">
          </div>
          <div class="campo" style="min-width:160px">
            <label>Grupo</label>
            <select id="fUGrupo">
              <option value="">todos os grupos</option>
              ${grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo" style="min-width:140px">
            <label>Papel</label>
            <select id="fUPapel">
              <option value="">todos os papéis</option>
              <option value="admin">Administrador</option>
              <option value="gerente">Gerente</option>
              <option value="operador">Operador</option>
              <option value="chefe_setor">Chefe de Setor</option>
            </select>
          </div>
          <div class="campo" style="min-width:120px">
            <label>Status</label>
            <select id="fUAtivo">
              <option value="">todas</option>
              <option value="1">ativa</option>
              <option value="0">desativada</option>
            </select>
          </div>
        </div>

        <div class="rolagem">
          <table id="tabU">
            <thead>
              <tr>
                <th style="width:38px; text-align:center">Foto</th>
                <th>ID</th>
                <th>${isAdminViewer ? 'Login' : 'Guerra'}</th>
                <th>Identificação Militar</th>
                <th>Funções / Cadeiras Atribuídas</th>
                <th>Status</th>
                <th>Ações</th>
              </tr>
            </thead>
            <tbody>
              ${lista.map(u => {
                const papeis = u.papeis || [];
                const chips = papeis.map(p => {
                  const rot = rotuloPapel(p.papel);
                  const grp = p.grupo_nome ? p.grupo_nome : (p.papel === 'admin' ? 'Global' : 'Sem grupo');
                  const func = p.funcao_nome ? ` · ${p.funcao_nome}` : '';
                  const ehUnico = p.papel === 'gerente' ? ' (Titular Único)' : '';
                  return `
                    <span class="papel-chip ${p.papel}">
                      <b>${esc(rot)}</b>: ${esc(grp)}${esc(func)}${ehUnico}
                      ${papeis.length > 1 ? `<button type="button" class="btn-del-papel" data-delpapel="${p.id}" data-uid="${u.id}" title="Remover este papel do usuário">✕</button>` : ''}
                    </span>
                  `;
                }).join('') || `<span class="papel-chip ${u.papel}"><b>${rotuloPapel(u.papel)}</b>: ${esc(nomeGrupo(u.grupo_id))}</span>`;

                const papeisStr = papeis.map(p => p.papel).join(' ') + ' ' + u.papel;

                return `
                  <tr data-login="${esc(u.login)}" data-nomes="${esc((u.nome_guerra || '') + ' ' + (u.nome_completo || ''))}" data-gid="${u.grupo_id || 0}" data-papeis="${esc(papeisStr)}" data-ativo="${u.ativo ? 1 : 0}">
                    <td style="text-align:center; vertical-align:middle">
                      <div style="width:32px; height:32px; border-radius:50%; overflow:hidden; background:var(--painel3); border:1px solid var(--borda); display:inline-flex; align-items:center; justify-content:center">
                        ${u.foto_base64 ? `<img src="${u.foto_base64}" style="width:100%; height:100%; object-fit:cover">` : `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`}
                      </div>
                    </td>
                    <td class="num">#${u.id}</td>
                    <td><b>${rotuloConta(u)}</b></td>
                    <td>
                      <div>${formatarNomeUsuario(u.nome_completo, u.nome_guerra)}</div>
                    </td>
                    <td>
                      <div style="display:flex; flex-wrap:wrap; align-items:center; gap:4px">
                        ${chips}
                        <button type="button" class="acao-linha" data-addpapel="${u.id}" data-login="${esc(u.login)}" style="font-size:11px; padding:2px 8px; font-weight:700; color:var(--verde-claro)">+ Função</button>
                      </div>
                    </td>
                    <td><span style="color:${u.ativo ? 'var(--verde-claro)' : 'var(--tx3)'}; font-weight:700">${u.ativo ? 'Ativa' : 'Desativada'}</span></td>
                    <td>
                      <button class="acao-linha" data-edperfil="${u.id}" style="font-weight:600">👤 Perfil</button>
                      <button class="acao-linha" data-ednomes="${u.id}" data-login="${esc(u.login)}" data-guerra="${esc(u.nome_guerra || '')}" data-completo="${esc(u.nome_completo || '')}">✏️ Nomes</button>
                      <button class="acao-linha" data-id="${u.id}" data-login="${esc(u.login)}">🔑 Senha</button>
                      ${u.papel !== 'admin' ? `
                        <button class="acao-linha" data-mv="${u.id}" data-login="${esc(u.login)}" data-papel="${u.papel}">Mover</button>
                        <button class="acao-linha perigo" data-exc="${u.id}" data-login="${esc(u.login)}">Excluir</button>
                      ` : ''}
                    </td>
                  </tr>
                `;
              }).join('') || '<tr><td colspan="6"><span class="vazio">Nenhuma conta encontrada.</span></td></tr>'}
            </tbody>
          </table>
        </div>
        <p style="color:var(--tx2);font-size:12.5px;margin-top:10px">
          💡 <b>Doutrina de Multi-Funções:</b> Usuários podem acumular múltiplas funções em diferentes grupos. Cada grupo possui apenas <b>1 Gerente titular</b> (função única), enquanto pode possuir <b>múltiplos Operadores</b> (função múltipla com caixa de email compartilhada).
        </p>
      </div>
    `;

    const aplicarFiltro = () => {
      const q = $('#fULogin').value.trim().toLowerCase();
      const gid = $('#fUGrupo').value;
      const papel = $('#fUPapel').value;
      const at = $('#fUAtivo').value;
      document.querySelectorAll('#tabU tbody tr[data-login]').forEach(tr => {
        // ITEM 2: filtro por NOME para todos; por login só quando o viewer é admin
        // (data-login permanece como atributo interno — nada é exibido na tela).
        const nomeMatches = tr.dataset.nomes.toLowerCase().includes(q);
        const loginMatches = isAdminViewer && tr.dataset.login.toLowerCase().includes(q);
        const grupoMatches = !gid || tr.dataset.gid === gid;
        const papelMatches = !papel || tr.dataset.papeis.includes(papel);
        const ativoMatches = at === '' || tr.dataset.ativo === at;
        tr.style.display = (nomeMatches || loginMatches) && grupoMatches && papelMatches && ativoMatches ? '' : 'none';
      });
    };

    $('#fULogin').oninput = aplicarFiltro;
    $('#fUGrupo').onchange = aplicarFiltro;
    $('#fUPapel').onchange = aplicarFiltro;
    $('#fUAtivo').onchange = aplicarFiltro;

    // Ação: Novo Usuário
    $('#btNovoUsuario').onclick = () => modalNovoUsuario();

    // Ação: Atribuir Papel
    document.querySelectorAll('#tabU button[data-addpapel]').forEach(b => {
      b.onclick = () => modalAtribuirPapel(b.dataset.addpapel, b.dataset.login);
    });

    // Ação: Remover Papel
    document.querySelectorAll('#tabU button[data-delpapel]').forEach(b => {
      b.onclick = async (ev) => {
        ev.stopPropagation();
        const pId = b.dataset.delpapel;
        const uId = b.dataset.uid;
        if (!(await confirmar('Deseja remover esta função/cadeira atribuída a este usuário?'))) return;
        try {
          await api(`/api/usuarios/${uId}/papeis/${pId}`, { method: 'DELETE' });
          toast('Função removida com sucesso!');
          admUsuarios();
        } catch (e) {}
      };
    });

    // Ação: Editar Perfil Completo
    document.querySelectorAll('#tabU button[data-edperfil]').forEach(b => {
      b.onclick = () => {
        const uid = +b.dataset.edperfil;
        const uAlvo = lista.find(x => x.id === uid);
        if (uAlvo) modalEditarPerfilUsuario(uAlvo);
      };
    });

    // Ação: Editar Nomes
    document.querySelectorAll('#tabU button[data-ednomes]').forEach(b => {
      b.onclick = () => modalEditarNomes(b.dataset.ednomes, b.dataset.login, b.dataset.guerra, b.dataset.completo);
    });

    // Ação: Senha
    document.querySelectorAll('#tabU button[data-id]').forEach(b => {
      b.onclick = () => modalSenha(b.dataset.id, b.dataset.login, true, () => admUsuarios());
    });

    // Ação: Mover
    document.querySelectorAll('#tabU button[data-mv]').forEach(b => {
      b.onclick = () => modalMover(b.dataset.mv, b.dataset.login, optsGrupos,
        'Se for o único gerente do grupo de origem, virará operador no destino e o operador mais antigo assumirá o grupo.',
        () => admUsuarios());
    });

    // Ação: Excluir
    document.querySelectorAll('#tabU button[data-exc]').forEach(b => {
      b.onclick = async () => {
        if (!(await confirmar(`Excluir a conta "${b.dataset.login}"?`))) return;
        try {
          const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
          toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
          admUsuarios();
        } catch (e) {}
      };
    });

    function modalEditarPerfilUsuario(u) {
      let fotoAtual = u.foto_base64 || '';
      const sangueOpts = ["", "A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"];
      const html = `
        <div class="modal" style="max-width:620px; max-height:90vh; overflow-y:auto">
          <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:14px">
            <div>
              <h3 style="margin:0 0 2px">👤 Perfil do Usuário — ${esc(u.nome_guerra || u.nome_completo || (isAdminViewer ? u.login : '—'))}</h3>
              <p style="color:var(--tx2);font-size:12px;margin:0">ID #${u.id} · Papel: <b>${rotuloPapel(u.papel)}</b></p>
            </div>
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">✕</button>
          </div>

          <!-- Foto 1x1 e Ações de Imagem -->
          <div style="display:flex; align-items:center; gap:16px; margin-bottom:16px; padding-bottom:14px; border-bottom:1px solid var(--borda)">
            <div id="mPfFotoBox" style="width:76px; height:76px; border-radius:12px; border:2px solid var(--borda2); overflow:hidden; display:flex; align-items:center; justify-content:center; background:var(--painel2); flex-shrink:0">
              ${fotoAtual ? `<img src="${fotoAtual}" style="width:100%; height:100%; object-fit:cover">` : `<svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`}
            </div>
            <div style="display:flex; flex-direction:column; gap:6px">
              <input type="file" id="mPfInpFile" accept="image/*" style="display:none">
              <button type="button" class="acao-linha" id="mPfBtTrocarFoto" style="font-size:12px; padding:4px 10px">📷 Alterar Foto 1x1</button>
              ${fotoAtual ? `<button type="button" class="acao-linha perigo" id="mPfBtRemoverFoto" style="font-size:11.5px; padding:3px 8px">Remover Foto</button>` : ''}
              <small style="color:var(--tx3); font-size:11px">Recorte 1x1 automático via navegador.</small>
            </div>
          </div>

          <!-- Campos Cadastrais -->
          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Nome de Guerra (NOME) *</label>
              <input id="mPfNg" value="${esc(u.nome_guerra || '')}">
            </div>
            <div class="campo" style="flex:1">
              <label>Nome Completo *</label>
              <input id="mPfNc" value="${esc(u.nome_completo || '')}">
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Data de Nascimento</label>
              <input id="mPfDataNasc" type="date" value="${esc(u.data_nascimento || '')}">
            </div>
            <div class="campo" style="flex:1">
              <label>Tipo Sanguíneo</label>
              <select id="mPfTipoSang">
                <option value="">Não informado</option>
                ${sangueOpts.filter(Boolean).map(s => `<option value="${s}" ${u.tipo_sanguineo === s ? 'selected' : ''}>${s}</option>`).join('')}
              </select>
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Telefone / WhatsApp</label>
              <input id="mPfTel" type="tel" value="${esc(u.telefone || '')}" placeholder="(XX) XXXXX-XXXX">
            </div>
            <div class="campo" style="flex:1">
              <label>E-mail</label>
              <input id="mPfEmail" type="email" value="${esc(u.email || '')}" placeholder="militar@eb.mil.br">
            </div>
          </div>

          <div class="campo" style="margin-bottom:8px">
            <label>Endereço Completo</label>
            <input id="mPfEndereco" value="${esc(u.endereco || '')}" placeholder="Rua, número, bairro, cidade - UF">
          </div>

          <div class="campo" style="margin-bottom:16px">
            <label>Unidade / Grupo de Alocação</label>
            <select id="mPfGrupo">
              <option value="">— Sem grupo (Global) —</option>
              ${grupos.map(g => `<option value="${g.id}" ${u.grupo_id === g.id ? 'selected' : ''}>${esc(g.nome)}</option>`).join('')}
            </select>
          </div>

          <div style="display:flex; justify-content:flex-end; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button class="primario" id="mPfSalvar">Salvar Dados</button>
          </div>
        </div>
      `;
      const m = modal(html);
      const fotoBox = m.querySelector('#mPfFotoBox');
      const inpFile = m.querySelector('#mPfInpFile');

      m.querySelector('#mPfBtTrocarFoto').onclick = () => inpFile.click();
      inpFile.onchange = (e) => {
        const file = e.target.files && e.target.files[0];
        if (file) {
          processarFoto1x1(file, (dataUrl) => {
            fotoAtual = dataUrl;
            fotoBox.innerHTML = `<img src="${fotoAtual}" style="width:100%; height:100%; object-fit:cover">`;
          });
        }
      };

      const btRem = m.querySelector('#mPfBtRemoverFoto');
      if (btRem) {
        btRem.onclick = () => {
          fotoAtual = '';
          fotoBox.innerHTML = `<svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`;
          btRem.remove();
        };
      }

      m.querySelector('#mPfSalvar').onclick = async () => {
        const ng = m.querySelector('#mPfNg').value.trim();
        const nc = m.querySelector('#mPfNc').value.trim();
        const dataNasc = m.querySelector('#mPfDataNasc').value;
        const tipoSang = m.querySelector('#mPfTipoSang').value;
        const tel = m.querySelector('#mPfTel').value.trim();
        const email = m.querySelector('#mPfEmail').value.trim();
        const endr = m.querySelector('#mPfEndereco').value.trim();
        const gidVal = m.querySelector('#mPfGrupo').value;
        const gid = gidVal ? +gidVal : 0;

        if (!ng || !nc) {
          toast('Nome de guerra e nome completo são obrigatórios', 'erro');
          return;
        }

        try {
          await api(`/api/usuarios/${u.id}`, {
            method: 'PATCH',
            body: JSON.stringify({
              nome_guerra: ng,
              nome_completo: nc,
              data_nascimento: dataNasc,
              tipo_sanguineo: tipoSang,
              telefone: tel,
              email: email,
              endereco: endr,
              foto_base64: fotoAtual,
              grupo_id: gid
            })
          });
          toast('Perfil do usuário atualizado com sucesso!');
          m.remove();
          admUsuarios();
        } catch (e) {}
      };
    }

    function modalNovoUsuario() {
      const html = `
        <div class="modal" style="max-width:500px">
          <h3 style="margin-top:0">Criar Novo Usuário</h3>
          <p style="color:var(--tx2);font-size:13px;margin-bottom:14px">Cadastre a conta de acesso e atribua o papel inicial.</p>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Nº Identificação (CPF/ID) *</label>
              <input id="nuLogin" placeholder="${isAdminViewer ? 'ex.: 000.000.000-00' : 'gerado pelo sistema a partir do nome'}">
            </div>
            <div class="campo" style="flex:1">
              <label>Senha Inicial (opcional)</label>
              <input type="password" id="nuSenha" placeholder="Padrão: sci">
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Nome Completo *</label>
              <input id="nuCompleto" placeholder="ex.: Hermes da Silva">
            </div>
            <div class="campo" style="flex:1">
              <label>Nome de Guerra *</label>
              <input id="nuGuerra" placeholder="ex.: Silva">
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Papel Inicial *</label>
              <select id="nuPapel">
                <option value="operador">Operador (Função Múltipla)</option>
                <option value="chefe_setor">Chefe de Setor (Conferência do Setor)</option>
                <option value="gerente">Gerente (Função Única por Grupo)</option>
                <option value="admin">Administrador (Global)</option>
              </select>
            </div>
            <div class="campo" style="flex:1">
              <label>Função / Encarregado (opcional)</label>
              <select id="nuFuncao">
                <option value="">— Nenhuma —</option>
                ${funcoesLista.map(f => `<option value="${f.id}">${esc(f.nome)}</option>`).join('')}
              </select>
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" id="nuCampoGrupo" style="flex:1">
              <label>Grupo / Unidade (padrão: sem grupo)</label>
              <select id="nuGrupo"><option value="">— Sem grupo (padrão) —</option>${optsGrupos}</select>
            </div>
          </div>

          <div class="form-linha" id="nuLinhaSetor" style="margin-bottom:8px; display:none">
            <div class="campo" style="flex:1">
              <label>Setor / Seção (escopo do chefe de setor)</label>
              <select id="nuSetor"><option value="">— Nenhum —</option>${setoresLista.map(s => `<option value="${s.id}">${esc(s.nome)}${s.sigla ? ' (' + esc(s.sigla) + ')' : ''}</option>`).join('')}</select>
            </div>
          </div>

          <div id="nuAjudaPapel" style="font-size:12px; color:var(--tx2); background:var(--painel2); border:1px solid var(--borda); border-radius:6px; padding:10px; margin-bottom:14px">
            ℹ️ <b>Hierarquia (ordem 04/10):</b> contas criadas pelo admin nascem SEM grupo. O gerente designa os chefes de setor; cada chefe seleciona os operadores do seu setor.
          </div>

          <div class="modal-acoes">
            <button class="fantasma" id="nuX">Cancelar</button>
            <button class="primario" id="nuGo">Criar Conta</button>
          </div>
        </div>
      `;
      const div = modal(html);
      if (!div) return;

      const selP = div.querySelector('#nuPapel');
      const grpField = div.querySelector('#nuCampoGrupo');
      const linhaSetor = div.querySelector('#nuLinhaSetor');
      const ajuda = div.querySelector('#nuAjudaPapel');

      selP.onchange = () => {
        const p = selP.value;
        if (linhaSetor) linhaSetor.style.display = (p === 'chefe_setor') ? 'flex' : 'none';
        if (p === 'admin') {
          grpField.style.display = 'none';
          ajuda.innerHTML = 'ℹ️ <b>Administrador:</b> Acesso irrestrito a configurações globais, backup e governança da estrutura.';
        } else if (p === 'gerente') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🔒 <b>Gerente (Função Única):</b> Comandante da unidade. Cada grupo só pode ter 1 gerente titular ativo.';
        } else if (p === 'chefe_setor') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🏢 <b>Chefe de Setor:</b> Responsável pela conferência de faltas e presenças exclusivamente do seu próprio setor/efetivo.';
        } else {
          grpField.style.display = 'block';
          ajuda.innerHTML = 'ℹ️ <b>Operador (Função Múltipla):</b> Acesso operacional diário às conferências e caixa de email compartilhada do grupo.';
        }
      };

      div.querySelector('#nuX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#nuGo').onclick = async () => {
        const completo = div.querySelector('#nuCompleto').value.trim();
        const guerra = div.querySelector('#nuGuerra').value.trim();
        const papel = selP.value;
        // ITEM 2 (ordem Diretor 06/10): o campo de identificação não é digitado
        // nem exibido para não-admin — o sistema deriva o login do nome de guerra
        // (acentos/espaços normalizados). O admin continua digitando o seu.
        let login;
        if (isAdminViewer) {
          login = div.querySelector('#nuLogin').value.trim();
        } else {
          login = guerra.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase()
            .replace(/[^a-z0-9]+/g, '.').replace(/^\.+|\.+$/g, '') || 'conta' + Date.now();
        }
        const senha = div.querySelector('#nuSenha').value;
        // ordem 04/10: padrão é SEM grupo (admin vincula depois se quiser);
        // gerente continua exigindo unidade.
        const grupoId = +div.querySelector('#nuGrupo').value || null;
        const funcaoId = +div.querySelector('#nuFuncao').value || null;
        // onda itens79: setor vai no create quando papel = chefe_setor (hUsuariosAdd
        // JÁ grava usuarios.setor_id — server.go:4604). chefe sem setor pode escolher
        // depois; se selecionado, manda o id do catálogo.
        const setorSel = div.querySelector('#nuSetor');
        const setorId = (papel === 'chefe_setor' && setorSel && setorSel.value) ? (+setorSel.value || null) : null;

        if (!login || !completo || !guerra) {
          toast('Preencha os campos obrigatórios (*)', 'erro');
          return;
        }
        if (senha && senha.length > 0 && senha.length < 8) {
          toast('Se preencher a senha, use no mínimo 8 caracteres. (Padrão: sci)', 'erro');
          return;
        }
        if (papel === 'gerente' && !grupoId) {
          toast('Gerente deve obrigatoriamente estar vinculado a uma unidade', 'erro');
          return;
        }

        try {
          const res = await api('/api/usuarios', {
            method: 'POST',
            body: JSON.stringify({
              login,
              senha,
              papel,
              grupo_id: grupoId,
              funcao_id: funcaoId,
              setor_id: setorId
            })
          });
          if (res && res.id) {
            await api(`/api/usuarios/${res.id}`, {
              method: 'PATCH',
              body: JSON.stringify({
                nome_completo: completo,
                nome_guerra: guerra
              })
            });
            toast('Usuário criado com sucesso!');
            div.fechar && div.fechar();
            admUsuarios();
          }
        } catch (e) {}
      };
    }

    function modalAtribuirPapel(uid, login) {
      const tituloConta = isAdminViewer ? esc(login) : esc((lista.find(x => x.id === +uid) || {}).nome_guerra || 'usuário');
      const html = `
        <div class="modal" style="max-width:480px">
          <h3 style="margin-top:0">Atribuir Função / Cadeira — ${tituloConta}</h3>
          <p style="color:var(--tx2);font-size:13px;margin-bottom:14px">Permita que este usuário acumule funções adicionais em grupos específicos.</p>

          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="flex:1">
              <label>Papel / Responsabilidade *</label>
              <select id="apPapel">
                <option value="operador">Operador (Função Múltipla)</option>
                <option value="chefe_setor">Chefe de Setor (Conferência do Setor)</option>
                <option value="gerente">Gerente (Função Única por Grupo)</option>
                <option value="admin">Administrador (Global)</option>
              </select>
            </div>
            <div class="campo" id="apCampoGrupo" style="flex:1">
              <label>Grupo / Pelotão *</label>
              <select id="apGrupo">${optsGrupos}</select>
            </div>
          </div>

          <div class="form-linha" id="apLinhaSetor" style="margin-bottom:10px; display:none">
            <div class="campo" style="flex:1">
              <label>Setor / Seção (obrigatório p/ chefe de setor)</label>
              <select id="apSetor"><option value="">— Nenhum —</option>${setoresLista.map(s => `<option value="${s.id}" data-grupo="${s.grupo_id == null ? '' : s.grupo_id}">${esc(s.nome)}${s.sigla ? ' (' + esc(s.sigla) + ')' : ''}</option>`).join('')}</select>
            </div>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label>Função (Opcional)</label>
            <select id="apFuncao">
              <option value="">— Nenhuma / Padrão —</option>
              ${funcoesLista.map(f => `<option value="${f.id}">${esc(f.nome)}</option>`).join('')}
            </select>
          </div>

          <div id="apAjuda" style="font-size:12px; color:var(--tx2); background:var(--painel2); border:1px solid var(--borda); border-radius:6px; padding:10px; margin-bottom:14px">
            ℹ️ <b>Operador:</b> O grupo pode comportar múltiplos operadores com acesso operacional e email compartilhado.
          </div>

          <div class="modal-acoes">
            <button class="fantasma" id="apX">Cancelar</button>
            <button class="primario" id="apGo">Atribuir Função</button>
          </div>
        </div>
      `;
      const div = modal(html);
      if (!div) return;

      const selP = div.querySelector('#apPapel');
      const grpField = div.querySelector('#apCampoGrupo');
      const linhaSetor = div.querySelector('#apLinhaSetor');
      const selSetor = div.querySelector('#apSetor');
      const selGrupo = div.querySelector('#apGrupo');
      const ajuda = div.querySelector('#apAjuda');

      // onda itens79: a linha de setor aparece só p/ chefe_setor e as opções
      // filtram pelo grupo escolhido (ou globais, grupo NULL). O backend
      // (hUsuarioPapelAdd) valida de novo — 400 se o setor não é do grupo.
      const filtrarSetores = () => {
        if (!selSetor) return;
        const gid = selGrupo.value;
        selSetor.querySelectorAll('option[data-grupo]').forEach(op => {
          const gOp = op.dataset.grupo;
          op.style.display = (!gid || gOp === '' || gOp === gid) ? '' : 'none';
        });
        if (selSetor.selectedOptions[0] && selSetor.selectedOptions[0].style.display === 'none') {
          selSetor.value = '';
        }
      };
      if (selGrupo) selGrupo.onchange = filtrarSetores;

      selP.onchange = () => {
        const p = selP.value;
        if (linhaSetor) {
          const mostrar = (p === 'chefe_setor');
          linhaSetor.style.display = mostrar ? 'flex' : 'none';
          if (mostrar) filtrarSetores();
        }
        if (p === 'admin') {
          grpField.style.display = 'none';
          ajuda.innerHTML = '🌐 <b>Administrador:</b> Acesso global ao sistema.';
        } else if (p === 'gerente') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🔒 <b>Gerente (Função Única):</b> Cada grupo só pode ter 1 gerente titular ativo. Se o grupo já tiver gerente, a operação será recusada.';
        } else if (p === 'chefe_setor') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🏢 <b>Chefe de Setor:</b> Permite ao usuário conduzir a conferência e marcar faltas/presenças de pessoas do seu setor.';
        } else {
          grpField.style.display = 'block';
          ajuda.innerHTML = '👥 <b>Operador (Função Múltipla):</b> O grupo pode comportar múltiplos operadores com acesso operacional compartilhado.';
        }
      };

      div.querySelector('#apX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#apGo').onclick = async () => {
        const papel = selP.value;
        const grupoId = +div.querySelector('#apGrupo').value || null;
        const funcaoId = +div.querySelector('#apFuncao').value || null;
        // onda itens79: chefe_setor pode nascer já com o setor (conveniência do
        // formulário; nomear_chefe continua sendo a via canônica).
        const setorId = (papel === 'chefe_setor' && selSetor && selSetor.value) ? (+selSetor.value || null) : null;

        if (papel !== 'admin' && !grupoId) {
          toast('Selecione o grupo para esta função', 'erro');
          return;
        }

        try {
          await api(`/api/usuarios/${uid}/papeis`, {
            method: 'POST',
            body: JSON.stringify({
              papel,
              grupo_id: grupoId,
              funcao_id: funcaoId,
              setor_id: setorId
            })
          });
          toast('Nova função atribuída com sucesso!');
          div.fechar && div.fechar();
          admUsuarios();
        } catch (e) {}
      };
    }

    function modalEditarNomes(uid, login, nomeGuerraAtual, nomeCompletoAtual) {
      const html = `
        <div class="modal" style="max-width:440px">
          <h3 style="margin-top:0">Editar Identificação — ${isAdminViewer ? esc(login) : esc(nomeGuerraAtual || nomeCompletoAtual || 'usuário')}</h3>
          <p style="color:var(--tx2);font-size:13px;margin-bottom:14px">O nome curto/de guerra aparecerá destacado nas assinaturas e caixas postais.</p>

          <div class="campo" style="margin-bottom:10px">
            <label>Nome Completo *</label>
            <input id="edCompleto" value="${esc(nomeCompletoAtual)}">
          </div>
          <div class="campo" style="margin-bottom:14px">
            <label>Nome de Guerra / Nome Curto *</label>
            <input id="edGuerra" value="${esc(nomeGuerraAtual)}">
          </div>

          <div class="modal-acoes">
            <button class="fantasma" id="edX">Cancelar</button>
            <button class="primario" id="edGo">Salvar Nomes</button>
          </div>
        </div>
      `;
      const div = modal(html);
      if (!div) return;

      div.querySelector('#edX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#edGo').onclick = async () => {
        const comp = div.querySelector('#edCompleto').value.trim();
        const guer = div.querySelector('#edGuerra').value.trim();
        if (!comp || !guer) {
          toast('Nomes obrigatórios', 'erro');
          return;
        }
        try {
          await api(`/api/usuarios/${uid}`, {
            method: 'PATCH',
            body: JSON.stringify({
              nome_completo: comp,
              nome_guerra: guer
            })
          });
          toast('Nomes atualizados com sucesso!');
          div.fechar && div.fechar();
          admUsuarios();
        } catch (e) {}
      };
    }
  }

  /* --- GESTÃO DE GRUPOS & UNIDADES (Modais Compartilhados Admin & Gerente) --- */
  async function modalGerenciarSetoresGrupo(grupoId, grupoNome) {
    const html = `
      <div class="modal" style="max-width:560px;width:95%">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h3 style="margin:0">🏢 Setores da Unidade — ${esc(grupoNome)}</h3>
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">✕</button>
        </div>
        <p style="color:var(--tx2);font-size:12.5px;margin-bottom:14px">Cadastre e gerencie seções ou pelotões pertencentes exclusivamente a este grupo.</p>
        
        <div style="background:var(--painel2);border:1px solid var(--borda);border-radius:var(--raio);padding:12px;margin-bottom:14px">
          <h4 style="margin:0 0 8px;font-size:13px">Novo Setor</h4>
          <div class="form-linha" style="margin:0;gap:8px">
            <div class="campo" style="flex:2;margin:0">
              <label>Nome do Setor *</label>
              <input id="nSetNome" placeholder="ex.: 1º Pelotão, Almoxarifado...">
            </div>
            <div class="campo" style="flex:1;margin:0">
              <label>Sigla</label>
              <input id="nSetSigla" placeholder="ex.: 1º PEL">
            </div>
            <div class="campo" style="align-self:flex-end;margin:0">
              <button class="primario" id="btAddSetor" style="min-height:38px">Adicionar</button>
            </div>
          </div>
        </div>

        <div id="listaSetoresGrupo" style="max-height:280px;overflow-y:auto;border:1px solid var(--borda);border-radius:6px;padding:8px">
          <div class="carregando">Carregando setores...</div>
        </div>
        <div class="modal-acoes" style="margin-top:14px">
          <button class="fantasma" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;
    const m = modal(html);
    const listaEl = m.querySelector('#listaSetoresGrupo');

    async function carregarLista() {
      try {
        listaEl.innerHTML = '<div class="carregando">Atualizando...</div>';
        const todosSetores = await api('/api/catalogo/setores');
        const setoresGrupo = (todosSetores || []).filter(s => s.grupo_id === grupoId);
        if (setoresGrupo.length === 0) {
          listaEl.innerHTML = '<div class="vazio" style="padding:16px;text-align:center">Nenhum setor cadastrado neste grupo.</div>';
          return;
        }
        listaEl.innerHTML = `
          <table style="width:100%;font-size:12.5px">
            <thead><tr><th>Nome</th><th>Sigla</th><th style="text-align:right">Ações</th></tr></thead>
            <tbody>
              ${setoresGrupo.map(s => `
                <tr>
                  <td><b>${esc(s.nome)}</b></td>
                  <td>${esc(s.sigla || '—')}</td>
                  <td style="text-align:right">
                    <button class="acao-linha" data-editset="${s.id}" data-nome="${esc(s.nome)}" data-sigla="${esc(s.sigla || '')}">Editar</button>
                    <button class="acao-linha perigo" data-excset="${s.id}">Excluir</button>
                  </td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        `;

        listaEl.querySelectorAll('[data-editset]').forEach(bt => {
          bt.onclick = async () => {
            const sid = bt.dataset.editset;
            const novoNome = prompt('Novo nome do setor:', bt.dataset.nome);
            if (!novoNome || !novoNome.trim()) return;
            const novaSigla = prompt('Sigla (opcional):', bt.dataset.sigla);
            try {
              await api(`/api/catalogo/setores/${sid}`, {
                method: 'PATCH',
                body: JSON.stringify({ nome: novoNome.trim(), sigla: (novaSigla || '').trim() })
              });
              toast('Setor atualizado!');
              carregarLista();
            } catch (e) {
              toast(e.message || 'Falha ao atualizar', 'erro');
            }
          };
        });

        listaEl.querySelectorAll('[data-excset]').forEach(bt => {
          bt.onclick = async () => {
            const sid = bt.dataset.excset;
            if (!confirm('Deseja excluir este setor?')) return;
            try {
              await api(`/api/catalogo/setores/${sid}`, { method: 'DELETE' });
              toast('Setor excluído!');
              carregarLista();
            } catch (e) {
              toast(e.message || 'Falha ao excluir setor', 'erro');
            }
          };
        });
      } catch (e) {
        listaEl.innerHTML = '<div class="vazio">Erro ao carregar setores.</div>';
      }
    }

    carregarLista();

    m.querySelector('#btAddSetor').onclick = async () => {
      const nome = m.querySelector('#nSetNome').value.trim();
      const sigla = m.querySelector('#nSetSigla').value.trim();
      if (!nome) { toast('Nome do setor obrigatório', 'erro'); return; }
      try {
        await api('/api/catalogo/setores', {
          method: 'POST',
          body: JSON.stringify({ nome, sigla, grupo_id: grupoId })
        });
        toast('Setor adicionado ao grupo!');
        m.querySelector('#nSetNome').value = '';
        m.querySelector('#nSetSigla').value = '';
        carregarLista();
      } catch (e) {
        toast(e.message || 'Falha ao cadastrar setor', 'erro');
      }
    };
  }

  function modalCriarGrupo(superiorId, superiorNome, aoFim) {
    const html = `
      <div class="modal" style="max-width:480px">
        <h3 style="margin-top:0">${superiorId ? `Criar Subgrupo subordinado a "${esc(superiorNome)}"` : 'Criar Nova Unidade / Grupo'}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">A criação de unidade requer apenas o nome. Em seguida, o administrador pode alocar usuários e definir o gerente da unidade.</p>
        
        <div class="campo" style="margin-bottom:16px">
          <label>Nome da Unidade / Subgrupo *</label>
          <input id="gNome" placeholder="ex.: 1ª Companhia de Fuzileiros" autofocus>
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
      if (!nome) {
        toast('Informe o nome da unidade', 'erro');
        return;
      }
      try {
        const r = await api('/api/grupos', {
          method: 'POST',
          body: JSON.stringify({ nome })
        });
        const novoId = r.id;
        if (superiorId && novoId) {
          await api('/api/admin/grupos/vinculo', {
            method: 'POST',
            body: JSON.stringify({ superior_id: superiorId, subordinado_id: novoId })
          });
        }
        toast(`Unidade "${nome}" criada com sucesso! (Código ${r.codigo})`);
        m.remove();
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };
  }

  async function modalAlocarUsuariosGrupo(gid, gnome, contas, aoFim) {
    let cList = contas;
    if (!cList || !cList.length) {
      try { cList = await api('/api/usuarios'); } catch (e) { cList = []; }
    }
    const outrasContas = (cList || []).filter(c => c.papel !== 'admin' && c.ativo && c.grupo_id !== gid);
    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0">👥 Alocar Usuário — ${esc(gnome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">Mover um usuário existente para integrar este grupo.</p>
        
        <div class="campo" style="margin-bottom:16px">
          <label>Selecione o usuário</label>
          <select id="mSelAlocarConta">
            <option value="">— Selecione um usuário cadastrado —</option>
            ${outrasContas.map(c => `<option value="${c.id}">${esc(c.login)} (${rotuloPapel(c.papel)}) - ${esc(c.nome_guerra || c.login)} [${c.grupo_id ? 'Grupo #' + c.grupo_id : 'Sem grupo'}]</option>`).join('')}
          </select>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mBtnAlocarGo">Alocar no Grupo</button>
        </div>
      </div>
    `;
    const m = modal(html);
    m.querySelector('#mBtnAlocarGo').onclick = async () => {
      const uid = m.querySelector('#mSelAlocarConta').value;
      if (!uid) { toast('Selecione um usuário', 'erro'); return; }
      try {
        await api(`/api/usuarios/${uid}/mover`, {
          method: 'PATCH',
          body: JSON.stringify({ grupo_id: gid })
        });
        toast('Usuário alocado na unidade com sucesso!');
        m.remove();
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };
  }

  async function modalTrocarGerente(gid, gnome, contas, aoFim) {
    let cList = contas;
    if (!cList || !cList.length) {
      try { cList = await api('/api/usuarios'); } catch (e) { cList = []; }
    }
    // Ordem Diretor 04/10: gerente pode ser QUALQUER conta indicada pelo admin —
    // não apenas quem já está alocado ao grupo (o backend vincula na troca).
    const contasGrupo = (cList || []).filter(c => c.papel !== 'admin' && c.ativo);
    const temGerente = contasGrupo.some(c => c.papel === 'gerente');
    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0">👤 ${temGerente ? 'Trocar Gerente' : 'Definir Gerente'} — ${esc(gnome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">
          ${temGerente
            ? 'Selecione qualquer conta ativa para assumir a titularidade — de dentro ou de fora do grupo (ordem do Diretor). O gerente atual passará a operador.'
            : 'Selecione qualquer conta ativa para ser o Gerente titular — o vínculo com o grupo é feito automaticamente.'}
        </p>

        ${contasGrupo.length === 0 ? `
          <div class="vazio" style="padding:20px; text-align:center; margin-bottom:14px">
            Nenhuma conta ativa disponível.
          </div>
        ` : `
          <div class="campo" style="margin-bottom:14px">
            <label>Conta a assumir a gerência</label>
            <select id="mSelNovaConta">
              <option value="">— Selecione uma conta —</option>
              ${contasGrupo.map(c => { const atual = c.papel === 'gerente' && c.grupo_id === gid; return `<option value="${esc(c.login)}" ${atual ? 'disabled' : ''}>${esc(c.login)} (${rotuloPapel(c.papel)}) - ${esc(c.nome_guerra || c.login)} ${atual ? '★ Atual Gerente' : ''}</option>`; }).join('')}
            </select>
          </div>
        `}

        <div style="display:flex; justify-content:space-between; align-items:center">
          ${temGerente ? `<button type="button" class="perigo acao-linha" id="mBtnRemoverGer" style="padding:4px 8px">Remover Gerente</button>` : `<div></div>`}
          <div style="display:flex; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            ${contasGrupo.length > 0 ? `<button class="primario" id="mBtnTrocarGer">Promover a Gerente</button>` : ''}
          </div>
        </div>
      </div>
    `;
    const m = modal(html);
    const btnRem = m.querySelector('#mBtnRemoverGer');
    if (btnRem) {
      btnRem.onclick = async () => {
        try {
          await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login: "__REMOVE__" }) });
          toast('Gerência vaga com sucesso.');
          m.remove();
          if (aoFim) aoFim();
          else if (window.ViewAdmin) window.ViewAdmin();
        } catch (e) {}
      };
    }
    const btn = m.querySelector('#mBtnTrocarGer');
    if (btn) {
      btn.onclick = async () => {
        const login = m.querySelector('#mSelNovaConta').value;
        if (!login) { toast('Selecione a conta para promover', 'erro'); return; }
        try {
          await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login }) });
          toast('Gerente atribuído com sucesso! Função herdada.');
          m.remove();
          if (aoFim) aoFim();
          else if (window.ViewAdmin) window.ViewAdmin();
        } catch (e) {}
      };
    }
  }

  async function modalGerenciarSubordinacao(gid, gnome, grupos, aoFim) {
    let gList = grupos;
    if (!gList || !gList.length) {
      try { gList = await api('/api/grupos'); } catch (e) { gList = []; }
    }
    const gAtual = (gList || []).find(x => x.id === gid);
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
            ${(gList || []).filter(x => x.id !== gid).map(x => `<option value="${x.id}" ${x.id === supAtualId ? 'selected' : ''}>${esc(x.nome)} (#${x.codigo})</option>`).join('')}
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
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };

    if (m.querySelector('#mBtnRemoverSub')) {
      m.querySelector('#mBtnRemoverSub').onclick = async () => {
        try {
          await api(`/api/admin/grupos/vinculo?superior_id=${supAtualId}&subordinado_id=${gid}`, { method: 'DELETE', body: '{}' });
          toast('Subordinação removida');
          m.remove();
          if (aoFim) aoFim();
          else if (window.ViewAdmin) window.ViewAdmin();
        } catch (e) {}
      };
    }
  }

  async function modalAuditarContas(gid, nome, contas) {
    let cList = contas;
    if (!cList || !cList.length) {
      try { cList = await api('/api/usuarios'); } catch (e) { cList = []; }
    }
    const cGrupo = (cList || []).filter(c => c.grupo_id === gid);
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

  function modalNukeFinal(gid, gnome, divAnterior, aoFim) {
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
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };
    const inp = div.querySelector('#fSenhaNuke');
    if (inp) inp.focus();
  }

  function modalExcluirGrupo(gid, gnome, aoFim) {
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
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };
    div.querySelector('#mForce').onclick = async () => {
      const senha = div.querySelector('#fSenha').value;
      if (!senha) { toast('Digite a senha de admin', 'erro'); return; }
      try {
        const r = await api(`/api/grupos/${gid}?forcar=1`, { method: 'DELETE', body: JSON.stringify({ senha }) });
        toast(`Grupo excluído forçadamente — ${r.contas_removidas} conta(s), ${r.pessoas_removidas} pessoa(s) removidas`);
        div.fechar && div.fechar();
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };
    div.querySelector('#mNuke').onclick = async () => {
      if (!(await confirmar(`☢️ Quer fazer isso mesmo? Isto APAGA "${gnome}" — contas, pessoal, catálogos e TODO o histórico de conferências. Não tem volta.`))) return;
      modalNukeFinal(gid, gnome, div, aoFim);
    };
  }

  function modalEditarGrupo(n, aoFim, ctx) {
    const eu = quem() || {};
    const isAdmin = eu.papel === 'admin';
    const temGer = !!n.gerente;
    const html = `
      <div class="modal" style="max-width:520px; width:95%">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:12px">
          <h3 style="margin:0; font-size:16px">⚙️ Gerenciar Unidade</h3>
          <span>${codigoChip(n.codigo)}</span>
        </div>
        
        <!-- Resumo da Unidade -->
        <div style="background:var(--painel3); padding:10px 14px; border-radius:8px; border:1px solid var(--borda); margin-bottom:14px; font-size:12.5px; line-height:1.6">
          <div style="font-size:14px; font-weight:700; color:var(--tx); margin-bottom:4px">${esc(n.nome)} <small style="color:var(--tx3); font-family:monospace">#${n.id}</small></div>
          <div style="color:var(--tx2)">
            👤 Gerente: <b>${esc(n.gerente || 'Sem gerente definido')}</b><br>
            👥 Efetivo: <b>${n.efetivo_total || n.efetivo || 0}</b> · 🔑 Contas: <b>${n.contas_total || n.contas || 0}</b> · 🌲 Subgrupos: <b>${n.subgrupos_total !== undefined ? n.subgrupos_total : (n.filhos ? n.filhos.length : 0)}</b>
          </div>
        </div>

        <!-- Formulário de Edição Direta (Nome e Sigla/Código) -->
        <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:12px; margin-bottom:14px">
          <h4 style="margin:0 0 10px; font-size:13px">✏️ Editar Dados da Unidade</h4>
          <div class="form-linha" style="margin:0; gap:8px">
            <div class="campo" style="flex:2; margin:0">
              <label>Nome da Unidade *</label>
              <input id="mEdNome" value="${esc(n.nome)}">
            </div>
            <div class="campo" style="flex:1; margin:0">
              <label>Sigla / Código</label>
              <input id="mEdCodigo" value="${esc(n.codigo || '')}" placeholder="ex.: 1CIA" style="text-transform:uppercase">
            </div>
            <div class="campo" style="align-self:flex-end; margin:0">
              <button class="primario" id="mEdSalvarDados" style="min-height:38px">Salvar</button>
            </div>
          </div>
        </div>

        <!-- Botões de Ações Adicionais -->
        <div style="display:flex; flex-direction:column; gap:8px">
          <button type="button" class="acao-linha" id="mEdSetores" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
            🏢 <b style="margin-left:6px">Editar Setores da Unidade</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Setores/Seções</span>
          </button>
          <button type="button" class="acao-linha" id="mEdAuditar" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
            📊 <b style="margin-left:6px">Auditar Contas & Efetivo</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Auditoria</span>
          </button>
          ${isAdmin ? `
            <button type="button" class="acao-linha" id="mEdSubgrupo" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
              ➕ <b style="margin-left:6px">Criar Subgrupo</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Novo subordinado</span>
            </button>
            <button type="button" class="acao-linha" id="mEdAlocar" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
              👥 <b style="margin-left:6px">Alocar Usuário</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Associar operador</span>
            </button>
            <button type="button" class="acao-linha" id="mEdGerente" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
              👤 <b style="margin-left:6px">${temGer ? 'Trocar Gerente' : 'Definir Gerente'}</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Liderança</span>
            </button>
            <button type="button" class="acao-linha" id="mEdSubordinar" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
              ⛓️ <b style="margin-left:6px">Subordinação Hierárquica</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Vínculo superior</span>
            </button>
            <button type="button" class="acao-linha perigo" id="mEdExcluir" style="justify-content:flex-start; padding:10px 14px; font-size:13px">
              🗑️ <b style="margin-left:6px">Excluir Unidade</b> <span style="margin-left:auto; font-size:11px; color:var(--tx3)">Remoção segura</span>
            </button>
          ` : ''}
        </div>

        <div class="modal-acoes" style="margin-top:16px">
          <button type="button" class="fantasma" id="mEdFechar">Fechar</button>
        </div>
      </div>
    `;
    const div = modal(html);
    if (!div) return;
    div.querySelector('#mEdFechar').onclick = () => div.fechar && div.fechar();

    div.querySelector('#mEdSalvarDados').onclick = async () => {
      const novoNome = div.querySelector('#mEdNome').value.trim();
      const novoCod = div.querySelector('#mEdCodigo').value.trim().toUpperCase();
      if (!novoNome) {
        toast('Nome da unidade é obrigatório', 'erro');
        return;
      }
      try {
        const r = await api(`/api/grupos/${n.id}`, {
          method: 'PATCH',
          body: JSON.stringify({ nome: novoNome, codigo: novoCod })
        });
        n.nome = r.nome || novoNome;
        n.codigo = r.codigo || novoCod;
        toast(`Unidade "${novoNome}" atualizada com sucesso!`);
        div.fechar && div.fechar();
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {
        toast(e.message || 'Falha ao atualizar unidade', 'erro');
      }
    };

    div.querySelector('#mEdSetores').onclick = () => { div.fechar && div.fechar(); modalGerenciarSetoresGrupo(n.id, n.nome); };
    div.querySelector('#mEdAuditar').onclick = () => { div.fechar && div.fechar(); modalAuditarContas(n.id, n.nome, ctx && ctx.contas); };

    if (isAdmin) {
      const bSub = div.querySelector('#mEdSubgrupo');
      if (bSub) bSub.onclick = () => { div.fechar && div.fechar(); modalCriarGrupo(n.id, n.nome, aoFim); };
      const bAlo = div.querySelector('#mEdAlocar');
      if (bAlo) bAlo.onclick = () => { div.fechar && div.fechar(); modalAlocarUsuariosGrupo(n.id, n.nome, ctx && ctx.contas, aoFim); };
      const bGer = div.querySelector('#mEdGerente');
      if (bGer) bGer.onclick = () => { div.fechar && div.fechar(); modalTrocarGerente(n.id, n.nome, ctx && ctx.contas, aoFim); };
      const bSubord = div.querySelector('#mEdSubordinar');
      if (bSubord) bSubord.onclick = () => { div.fechar && div.fechar(); modalGerenciarSubordinacao(n.id, n.nome, ctx && ctx.grupos, aoFim); };
      const bExc = div.querySelector('#mEdExcluir');
      if (bExc) bExc.onclick = () => { div.fechar && div.fechar(); modalExcluirGrupo(n.id, n.nome, aoFim); };
    }
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
              <button class="acao-linha" id="btRetornarTopo" title="Retornar à visão do topo/raiz">⬆️ Retornar ao Topo</button>
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

      // Retornar ao Topo (Admin vai para a raiz geral; Gerente/Usuário com grupo vai para o seu grupo raiz)
      if ($('#btRetornarTopo')) {
        $('#btRetornarTopo').onclick = () => {
          const eu = quem();
          if (eu && eu.papel !== 'admin' && eu.grupo_id) {
            FOCO_GRUPO_ID = eu.grupo_id;
          } else {
            FOCO_GRUPO_ID = null; // Admin ou sem grupo retorna à raiz
          }
          renderConteudo();
        };
      }

      // Botão de Novo Grupo Raiz
      $('#btNovoGrupoRaiz').onclick = () => modalCriarGrupo(null, null, () => window.ViewAdmin());

      // Botão Consolidado "Editar" (abre modal de gestão da unidade)
      $('#arvoreAdm').querySelectorAll('[data-editargrupo]').forEach(b => {
        b.onclick = (e) => {
          e.stopPropagation();
          const gid = +b.dataset.editargrupo;
          const no = buscarNoPorId(arvore, gid);
          if (no) modalEditarGrupo(no, () => window.ViewAdmin(), { grupos, contas });
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

    renderConteudo();
  }

  /* --- ADMIN › backup: telemetria em tempo real + gerar download .db + IMPORTAR upload com confirmação --- */
  function admBackup() {
    const anoAtual = new Date().getFullYear();
    $('#adm').innerHTML = `
      <!-- Telemetria e Monitoramento de Recursos em Tempo Real -->
      <div class="cartao" style="margin-bottom:14px" id="cardTelemetria">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
          <div>
            <h3 style="margin:0; display:flex; align-items:center; gap:8px">
              <span>⚡</span> <span>Telemetria do Servidor em Tempo Real</span>
            </h3>
            <p style="color:var(--tx2); font-size:12px; margin:2px 0 0">
              Consumo dinâmico de CPU, memória RAM e alocação de armazenamento no host.
            </p>
          </div>
          <span id="badgeLiveStatus" style="font-size:11px; font-weight:700; color:var(--verde-claro); display:inline-flex; align-items:center; gap:6px; background:rgba(16,185,129,0.12); border:1px solid rgba(16,185,129,0.3); padding:4px 12px; border-radius:12px">
            <span style="width:7px; height:7px; background:var(--verde-claro); border-radius:50%; display:inline-block"></span> AO VIVO
          </span>
        </div>

        <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(220px, 1fr)); gap:12px; margin-bottom:4px">
          <!-- CPU -->
          <div style="background:var(--painel3); border:1px solid var(--borda); border-radius:8px; padding:12px 14px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
              <span style="font-size:12px; color:var(--tx2); font-weight:600">🖥️ Uso de CPU</span>
              <span id="txtCPUVal" style="font-size:15px; font-weight:700; color:var(--verde-claro)">-- %</span>
            </div>
            <div style="height:6px; background:rgba(255,255,255,0.08); border-radius:3px; overflow:hidden">
              <div id="barCPU" style="height:100%; width:0%; background:var(--verde-claro); transition:width 0.4s ease"></div>
            </div>
            <div id="txtCPUSub" style="font-size:11px; color:var(--tx3); margin-top:6px">-- núcleos ativos</div>
          </div>

          <!-- RAM -->
          <div style="background:var(--painel3); border:1px solid var(--borda); border-radius:8px; padding:12px 14px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
              <span style="font-size:12px; color:var(--tx2); font-weight:600">🧠 Memória RAM</span>
              <span id="txtRAMVal" style="font-size:15px; font-weight:700; color:#60a5fa">-- MB</span>
            </div>
            <div style="height:6px; background:rgba(255,255,255,0.08); border-radius:3px; overflow:hidden">
              <div id="barRAM" style="height:100%; width:0%; background:#60a5fa; transition:width 0.4s ease"></div>
            </div>
            <div id="txtRAMSub" style="font-size:11px; color:var(--tx3); margin-top:6px">Sys: -- MB · Heap: -- MB</div>
          </div>

          <!-- Armazenamento DATA e Disco -->
          <div style="background:var(--painel3); border:1px solid var(--borda); border-radius:8px; padding:12px 14px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
              <span style="font-size:12px; color:var(--tx2); font-weight:600">💾 Armazenamento / Disco</span>
              <span id="txtDataVal" style="font-size:15px; font-weight:700; color:#fbbf24">-- %</span>
            </div>
            <div style="height:6px; background:rgba(255,255,255,0.08); border-radius:3px; overflow:hidden">
              <div id="barDisco" style="height:100%; width:0%; background:#fbbf24; transition:width 0.4s ease"></div>
            </div>
            <div id="txtDataSub" style="font-size:11px; color:var(--tx3); margin-top:6px">Disco livre: -- GB de -- GB</div>
          </div>

          <!-- Tamanho Banco SQLite -->
          <div style="background:var(--painel3); border:1px solid var(--borda); border-radius:8px; padding:12px 14px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
              <span style="font-size:12px; color:var(--tx2); font-weight:600">🗄️ Banco SQLite (sci.db)</span>
              <span id="txtBancoVal" style="font-size:15px; font-weight:700; color:#c084fc">-- MB</span>
            </div>
            <div style="height:6px; background:rgba(255,255,255,0.08); border-radius:3px; overflow:hidden">
              <div id="barBanco" style="height:100%; width:0%; background:#c084fc; transition:width 0.4s ease"></div>
            </div>
            <div id="txtBancoSub" style="font-size:11px; color:var(--tx3); margin-top:6px">Modo WAL ativo · Integridade OK</div>
          </div>
        </div>
      </div>

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

    // Atualização contínua de telemetria em tempo real
    let telemetriaTimer = null;
    const atualizarTelemetria = async () => {
      try {
        const m = await api('/api/admin/sistema/metricas');
        if (!m || !$('#cardTelemetria')) return;

        // CPU
        const cpu = (m.cpu_percent || 0).toFixed(1);
        const cpuValEl = $('#txtCPUVal');
        const barCPUEl = $('#barCPU');
        const cpuSubEl = $('#txtCPUSub');
        if (cpuValEl) cpuValEl.textContent = cpu + '%';
        if (barCPUEl) {
          barCPUEl.style.width = Math.min(100, Math.max(3, m.cpu_percent)) + '%';
          barCPUEl.style.background = m.cpu_percent > 80 ? 'var(--verm)' : m.cpu_percent > 50 ? 'var(--ambar)' : 'var(--verde-claro)';
        }
        if (cpuSubEl) cpuSubEl.textContent = `${m.num_cpu} núcleos · ${m.goroutines} rotinas Go`;

        // RAM
        const ramMB = (m.ram_processo_mb || 0).toFixed(1);
        const ramSys = (m.ram_sistema_mb || 0).toFixed(1);
        const ramValEl = $('#txtRAMVal');
        const barRAMEl = $('#barRAM');
        const ramSubEl = $('#txtRAMSub');
        const sysTotal = Math.max(m.ram_sistema_mb || 0, m.ram_processo_mb || 0, 1);
        const ramPct = Math.min(100, Math.max(1, ((m.ram_processo_mb || 0) / sysTotal) * 100));
        if (ramValEl) ramValEl.textContent = `${ramMB} MB (${ramPct.toFixed(0)}%)`;
        if (barRAMEl) barRAMEl.style.width = ramPct.toFixed(1) + '%';
        if (ramSubEl) ramSubEl.textContent = `Sys: ${ramSys} MB · Heap: ${(m.ram_heap_mb || 0).toFixed(1)} MB`;

        // Armazenamento do Host / Disco e Pasta DATA
        let dataTxt = (m.dados_mb || 0).toFixed(2) + ' MB';
        if (m.dados_mb > 1024) dataTxt = (m.dados_mb / 1024).toFixed(2) + ' GB';
        const dataValEl = $('#txtDataVal');
        const barDiscoEl = $('#barDisco');
        const dataSubEl = $('#txtDataSub');
        const discoPct = Math.min(100, Math.max(0, m.disco_usado_pct || 0));
        if (dataValEl) dataValEl.textContent = discoPct.toFixed(1) + '%';
        if (barDiscoEl) barDiscoEl.style.width = Math.max(1, discoPct).toFixed(1) + '%';
        if (dataSubEl) dataSubEl.textContent = `Livre: ${(m.disco_livre_gb || 0).toFixed(1)} GB de ${(m.disco_total_gb || 0).toFixed(1)} GB · DATA: ${dataTxt}`;

        // Database SQLite sci.db
        let bancoTxt = (m.banco_mb || 0).toFixed(2) + ' MB';
        if (m.banco_mb > 1024) bancoTxt = (m.banco_mb / 1024).toFixed(2) + ' GB';
        const bancoValEl = $('#txtBancoVal');
        const barBancoEl = $('#barBanco');
        const pctBanco = (m.dados_mb && m.dados_mb > 0) ? Math.min(100, ((m.banco_mb || 0) / m.dados_mb) * 100) : 100;
        if (bancoValEl) bancoValEl.textContent = `${bancoTxt} (${pctBanco.toFixed(0)}%)`;
        if (barBancoEl) barBancoEl.style.width = Math.min(100, Math.max(1, pctBanco)).toFixed(1) + '%';
        const bancoSubEl = $('#txtBancoSub');
        if (bancoSubEl) bancoSubEl.textContent = `sci.db: ${bancoTxt} de ${dataTxt} (DATA)`;
      } catch (e) {}
    };

    atualizarTelemetria();
    telemetriaTimer = setInterval(atualizarTelemetria, 2500);

    const observer = new MutationObserver(() => {
      if (!$('#cardTelemetria')) {
        clearInterval(telemetriaTimer);
        observer.disconnect();
      }
    });
    const appEl = $('#app');
    if (appEl) observer.observe(appEl, { childList: true, subtree: true });
  }

  /* =====================================================================
     #/grupos — GERENCIAR (gerente): Pessoal · Tags · Grupos · Operadores
     ===================================================================== */
  let setoresCat = null, funcoesCat = null; // cache compartilhado (v9.16.9)
  window.ViewGrupos = async function () {
    const eu = quem();
    // ordem 06/10 (P4): encarregado/auxiliar de pessoal também gerenciam —
    // mesmas abas; ações restritivas dentro delas são filtradas por gestorPessoal
    // (senha de conta segue gerente/admin no servidor).
    if (!eu || (eu.papel !== 'gerente' && !(window.gestorPessoal && window.gestorPessoal()))) { location.hash = '#/hoje'; return; }
    // ordem 06/10: quem chega aqui sem papel do sistema é encarregado/auxiliar —
    // o servidor nega exclusão de catálogo/pessoa, senha e mover; o front esconde.
    const souFuncaoPessoal = eu.papel !== 'gerente';
    navAtiva('#/grupos');
    $('#app').innerHTML = '<div class="carregando">…</div>';
    const [grupos, arvore, pessoas, setores, funcoes, contas, apresentacaoDados] = await Promise.all([
      api('/api/grupos'), api('/api/grupos/arvore'), api('/api/pessoas'),
      api('/api/catalogo/setores'), api('/api/catalogo/funcoes'), api('/api/usuarios'),
      api('/api/pessoas/apresentacao').catch(() => ({ apresentacao: [] }))]);
    const mapaApresentacao = {};
    ((apresentacaoDados && apresentacaoDados.apresentacao) || []).forEach(a => {
      if (a && a.pessoa_id != null) mapaApresentacao[a.pessoa_id] = a;
    });
    const podeApresentacao = typeof window.gestorPessoal === 'function' ? (window.gestorPessoal() || (typeof window.ehEncarregado === 'function' && window.ehEncarregado())) : false;
    let optSetores = ativosDe(setores), optFuncoes = ativosDe(funcoes);
    setoresCat = setores; funcoesCat = funcoes; // cache p/ carregarCats (v9.16.9)
    const operadores = contas.filter(c => c.grupo_id === eu.grupo_id && c.papel === 'operador');
    const meus = grupos.filter(g => g.id === eu.grupo_id);
    const gerenteDe = {};
    contas.filter(c => c.papel === 'gerente' && c.ativo && c.grupo_id).forEach(c => { gerenteDe[c.grupo_id] = c.login; });
    marcarGerente(arvore, gerenteDe);

    async function atualizarSelectsCatalogos() {
      try {
        const [setoresNovos, funcoesNovas] = await Promise.all([
          api('/api/catalogo/setores'), api('/api/catalogo/funcoes')
        ]);
        optSetores = ativosDe(setoresNovos);
        optFuncoes = ativosDe(funcoesNovas);
        const selS = $('#pSetor'), selF = $('#pFuncao');
        if (selS) {
          const val = selS.value;
          selS.innerHTML = '<option value="">—</option>' + optSetores.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('');
          selS.value = val;
        }
        if (selF) {
          const val = selF.value;
          selF.innerHTML = '<option value="">—</option>' + optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('');
          selF.value = val;
        }
      } catch (e) {}
    }

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
        <p style="color:var(--tx2);font-size:12px;margin:4px 0">Formato (1 por linha, separado por ponto-e-vírgula): <code>nome de guerra ; nome completo ; setor ; posto/graduação</code> — setor e posto/graduação são opcionais e devem já existir no catálogo.</p>
        <textarea id="csv" rows="5" placeholder="SILVA;José da Silva;Comando;Motorista&#10;SOUSA;Maria de Sousa;Serviços&#10;PERES;Bruno Peres"></textarea>
        <button class="acao-linha" id="csvGo" style="margin-top:8px">Importar linhas</button></div>`;

    /* --- mapa local de rótulos e classes de apresentação --- */
    const GPX_ROTULO_APRESENTACAO = {
      'presente': { rotulo: 'Presente', classe: 'pill-presente' },
      'dispensado': { rotulo: 'Dispensado', classe: 'gpx-pill-dispensado' },
      'descompensado': { rotulo: 'Descompensado', classe: 'gpx-pill-descompensado' },
      'a serviço externo': { rotulo: 'A serviço externo', classe: 'gpx-pill-servico-ext' },
      'atrasado': { rotulo: 'Atrasado', classe: 'pill-atraso' },
      'falta': { rotulo: 'Falta', classe: 'pill-falta' }
    };
    function gpxRenderPillApresentacao(estado) {
      if (!estado) return '';
      const norm = String(estado).trim().toLowerCase();
      const info = GPX_ROTULO_APRESENTACAO[norm];
      if (info) return `<span class="pill ${info.classe}">${esc(info.rotulo)}</span>`;
      return `<span class="pill">${esc(estado)}</span>`;
    }

    /* --- banco de pessoal (checkbox por linha p/ operações em lote) --- */
    const linhasP = (pessoas.pessoas || []).map(p => {
      const ap = mapaApresentacao[p.id];
      const pillAp = ap && ap.estado ? ` ${gpxRenderPillApresentacao(ap.estado)}` : '';
      const celMod = p.ultima_mod_em
        ? `<div class="gpx-mod-cel"><div>${fmtData(p.ultima_mod_em)} ${fmtHora(p.ultima_mod_em)}</div>${p.ultima_mod_por ? `<small class="gpx-mod-por">por ${esc(p.ultima_mod_por)}</small>` : ''}</div>`
        : '—';
      return `<tr data-p='${esc(JSON.stringify(p))}'><td><input type="checkbox" class="chkP" data-id="${p.id}"></td>
       <td class="num">#${p.id}</td><td><b>${esc(p.nome_guerra)}</b>${pillAp}</td><td>${esc(p.nome_completo)}</td>
       <td>${esc(p.setor || 'SEM SETOR')}</td>
       <td>${esc(p.funcao || 'INDEFINIDO')}</td>
       <td>${p.status === 'ativo' ? '<span class="alerta-ok">● ATIVO</span>' : '<span style="color:var(--tx3)">● INATIVO</span>'}</td>
       <td>${celMod}</td>
       <td><div class="gpx-acoes">
       ${podeApresentacao ? `<button class="acao-linha" data-apresentacao="${p.id}" data-nome="${esc(p.nome_guerra)}">APRESENTAÇÃO</button>` : ''}
       ${podeApresentacao ? `<button class="acao-linha" data-historico="${p.id}" data-nome="${esc(p.nome_guerra)}">HISTÓRICO</button>` : ''}
       <button class="acao-linha" data-edit="${p.id}">editar</button>
       <button class="acao-linha" data-fichap="${p.id}" title="Imprimir Dossiê / Ficha Cadastral">📄 ficha</button>
       ${souFuncaoPessoal ? '' : `<button class="acao-linha" data-excP="${p.id}" data-nome="${esc(p.nome_guerra)}">excluir</button>`}
       </div></td></tr>`;
    }).join('');

    const optsMoverGer = `<option value="">— destino (dentro da sua hierarquia) —</option>` +
      grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');

    // ordem 04/10: abas do módulo → DROPDOWN estilizado
    $('#app').innerHTML = `<h2>Gerenciar</h2>
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:6px">
        <span style="font-size:12px;color:var(--tx2)">Seção:</span>
        <div id="abasGerDD" style="min-width:200px"></div></div>
      <div id="gerPessoal" class="${abaGer === 'pessoal' ? '' : 'oculto'}">
        ${formPessoa}
        <div class="cartao"><h3 style="margin-top:0">BANCO DE PESSOAL (${(pessoas.pessoas || []).length})</h3>
          <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:8px">
            <label style="font-size:13px"><input type="checkbox" id="chkTodosP"> todos</label>
            <button class="primario" id="btEditLote" disabled>Editar selecionados (<span id="nSel">0</span>)</button>
            ${souFuncaoPessoal ? '' : '<button class="perigo" id="btExcLote" disabled>Excluir selecionados (<span id="nSel2">0</span>)</button>'}
            <span style="color:var(--tx2);font-size:12px">com histórico de conferência: exclusão vira inativo (histórico preservado)</span></div>
          <div class="rolagem"><table><thead><tr><th></th><th>ID</th><th>Guerra</th><th>Completo</th><th>Setor</th><th>Função</th><th>Ativo</th><th>ÚLTIMA MODIFICAÇÃO</th><th></th></tr></thead>
          <tbody id="tabP">${linhasP || '<tr><td colspan="9"><span class="vazio">nenhum militar cadastrado</span></td></tr>'}</tbody></table></div></div>
        <div class="cartao gpx-rel-card">
          <h3 style="margin-top:0">RELATÓRIO DE FALTAS E ATRASOS</h3>
          <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Selecione uma conferência fechada para emitir o relatório analítico filtrado por situação.</p>
          <div class="gpx-rel-controles">
            <div id="gpxRelConfDD" style="min-width:280px"></div>
            <button type="button" class="primario" id="gpxBtFaltas">SÓ FALTAS</button>
            <button type="button" id="gpxBtAtrasos">SÓ ATRASOS</button>
          </div>
        </div>
      </div>
      <div id="gerTags" class="${abaGer === 'tags' ? '' : 'oculto'}">
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; margin-bottom:12px">
            <div>
              <h3 style="margin:0 0 4px">Catálogos & TAGS Organizacionais</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:0">Gerencie Tags, Setores, Postos/Graduações e Destinos. Superiores podem editar itens próprios e de subordinados; itens de superiores são somente leitura (🔒).</p>
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
                  <optgroup label="Tags de Pessoal">
                    <option value="destinos">📍 Destinos (Faltas Justificadas)</option>
                  </optgroup>
                  <optgroup label="Tags de Material">
                    <option value="tags">🏷️ Situação do Material (Disponível, etc.)</option>
                    <option value="material_tipos">📦 Tipos de Material</option>
                    <option value="material_classes">🎖️ Classes de Material</option>
                  </optgroup>
                  <optgroup label="Estrutura Organizacional">
                    <option value="setores">🏢 Setores / Seções</option>
                  </optgroup>
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
        (meus.map(g => `
          <div style="margin-bottom:8px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px">
            <div>• <b>${esc(g.nome)}</b> ${codigoChip(g.codigo)} — ${g.efetivo} no efetivo · ${g.contas} conta(s)</div>
            <button class="primario" style="font-size:12px; padding:4px 12px" data-editargrupo="${g.id}">⚙️ Editar</button>
          </div>
        `).join('')
          || '<span class="vazio">nenhum grupo</span>') + `</div>
        <div class="cartao"><h3 style="margin-top:0">Subordinação — Estrutura e Hierarquia da Unidade</h3>
          <div id="arvore2">${arvoreHTML(arvore, true)}</div></div>
      </div>
      <div id="gerFuncoes" class="${abaGer === 'funcoes' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">FUNÇÕES DO GRUPO — Titulares e Auxiliares</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Funções <b>administrativas do grupo</b> (Encarregado de Pessoal e afins — sem postos/graduações): <b>1 titular</b> por função (garantido pelo sistema) e quantos auxiliares forem necessários. Somente contas do SEU grupo.</p>
        <div class="rolagem"><table><thead><tr><th>Função</th><th>Designados</th><th>Designar</th></tr></thead>
        <tbody id="tabFun"><tr><td colspan="3"><span class="carregando">…</span></td></tr></tbody></table></div></div>
      </div>
      <div id="gerChefes" class="${abaGer === 'chefes' ? '' : 'oculto'}">
        <!-- ordem Diretor 07/10: aba SETORES absorve Setores (Gestão) e a Visão
             Agregada — três blocos: GESTÃO (novo/editar/excluir), CHEFIAS
             (nomear/destituir) e PANORAMA (leitura, próprio grupo + subordinados). -->
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; margin-bottom:10px">
            <div>
              <h3 style="margin:0 0 4px">🏢 Setores — Gestão</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:0">EDITAR abre as ações do setor: ALTERAR NOME, NOMEAR CHEFE (candidatos = pessoas do setor) e EXCLUIR (remaneja o pessoal para SEM SETOR; setor com histórico de conferências não é apagado — desative).</p>
            </div>
            <button class="primario" id="btNovoSetorChefes" style="min-height:36px">+ Novo setor</button>
          </div>
          <div class="rolagem"><table><thead><tr><th>Setor</th><th>Sigla</th><th>Unidade</th><th>Chefe</th><th>Ações</th></tr></thead>
          <tbody id="tabSetores"><tr><td colspan="5"><span class="carregando">…</span></td></tr></tbody></table></div>
        </div>
        <div class="cartao"><h3 style="margin-top:0">CHEFIAS — Nomeação e Destituição rápida</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Cada setor com seu chefe atual. Nomear atribui o papel <b>chefe_setor</b> e vincula o setor à conta; destituir remove o papel (setor fica livre). Um usuário pode chefiar VÁRIOS setores.</p>
        <div class="rolagem"><table><thead><tr><th>Setor</th><th>Chefe atual</th><th>Nomear</th></tr></thead>
        <tbody id="tabChefes"><tr><td colspan="3"><span class="carregando">…</span></td></tr></tbody></table></div></div>
        <div class="cartao"><h3 style="margin-top:0">PANORAMA — Visão Agregada do escopo</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Somente leitura: pessoal ativo no banco e contas ativas por setor (próprio grupo e subordinados). Nomear/destituir use os blocos acima.</p>
        <div class="rolagem"><table><thead><tr><th>Setor</th><th>Unidade</th><th class="num">Pessoal ativo</th><th class="num">Contas ativas</th><th>Chefe</th></tr></thead>
        <tbody id="tabAgreg"><tr><td colspan="5"><span class="carregando">…</span></td></tr></tbody></table></div></div>
      </div>
      <div id="gerSetores" class="${abaGer === 'setores' ? '' : 'oculto'}">
        <!-- ordem 06/10 (item 8): modo SETORES do Gerenciar — EDITAR por setor
             (nome / chefe / excluir com confirmação dupla) + NOVO SETOR. -->
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; margin-bottom:10px">
            <div>
              <h3 style="margin:0 0 4px">🏢 Setores — Gestão</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:0">EDITAR abre as ações do setor: ALTERAR NOME, NOMEAR CHEFE (candidatos = pessoas do setor) e EXCLUIR (remaneja o pessoal para SEM SETOR; setor com histórico de conferências não é apagado — desative).</p>
            </div>
            <button class="primario" id="btNovoSetor" style="min-height:38px">➕ NOVO SETOR</button>
          </div>
          <div class="rolagem"><table><thead><tr><th>Setor</th><th>Sigla</th><th>Unidade</th><th>Chefe atual</th><th></th></tr></thead>
          <tbody id="tabSetores"><tr><td colspan="5"><span class="carregando">…</span></td></tr></tbody></table></div>
        </div>
      </div>
      <div id="gerOperadores" class="${abaGer === 'operadores' ? '' : 'oculto'}">
        <!-- ordem 04/10: gerente NÃO cria operador — seleciona CHEFES DE SETOR
             (aba Pessoal não cobre: isso fica no modal de usuário do admin) e são
             os chefes que designam operadores dentre as contas do seu setor. -->
        <div class="cartao"><h3 style="margin-top:0">OPERADORES do meu grupo (${operadores.length})</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">O gerente não cria operadores. Fluxo da hierarquia: <b>gerente</b> designa os <b>chefes de setor</b> → cada <b>chefe</b> seleciona os operadores dentre as contas do SEU setor (aba de gestão do chefe).</p>
        ${souFuncaoPessoal ? `
        <!-- ordem 06/10 (item 3): encarregado/auxiliar CRIAM contas de operador/chefe
             de setor do próprio grupo (POST /api/usuarios); gerente segue sem criar
             operador (fluxo da hierarquia acima). -->
        <div class="cartao" style="margin-bottom:12px;background:var(--painel2)">
          <h4 style="margin:0 0 8px;font-size:13px">Designar conta do grupo</h4>
          <p style="color:var(--tx2);font-size:12px;margin:0 0 8px">Cria a conta com login derivado do nome de guerra (senha padrão <code>sci</code> — troca obrigatória no 1º acesso). <b>Só operador ou chefe de setor do próprio grupo.</b></p>
          <div class="form-linha" style="align-items:flex-end">
            <div class="campo" style="flex:1;min-width:140px"><label>Nome de guerra *</label><input id="encNg" placeholder="ex.: SILVA"></div>
            <div class="campo" style="flex:2;min-width:180px"><label>Nome completo *</label><input id="encNc" placeholder="ex.: José da Silva"></div>
            <div class="campo" style="width:150px"><label>Função na conta</label>
              <select id="encPapel"><option value="operador">Operador</option><option value="chefe_setor">Chefe de setor</option></select>
            </div>
            <button class="primario" id="encCriar" style="min-height:40px">Criar conta</button>
          </div>
        </div>` : ''}
        <div class="campo" style="margin-bottom:8px"><label>Filtrar operadores</label><input id="fOp" placeholder="buscar login…"></div>
        <div class="rolagem" style="margin-top:10px"><table><thead><tr><th>ID</th><th>Login</th><th>Status</th><th>Criada</th><th>Ações</th></tr></thead>
        <tbody>${operadores.map(o => `
          <tr data-login="${esc(o.login)}"><td class="num">#${o.id}</td><td><b>${esc(o.login)}</b></td><td>${o.ativo ? 'ativa' : 'desativada'}</td><td>${fmtData(o.criado_em)}</td>
          <td>${souFuncaoPessoal
            ? `<button class="acao-linha" data-editu="${o.id}" data-login="${esc(o.login)}" data-ng="${esc(o.nome_guerra || '')}" data-nc="${esc(o.nome_completo || '')}">editar</button>`
            : `<button class="acao-linha" data-senha="${o.id}" data-login="${esc(o.login)}">senha</button>
          <button class="acao-linha" data-mv="${o.id}" data-login="${esc(o.login)}">mover</button>
          <button class="acao-linha" data-exc="${o.id}" data-login="${esc(o.login)}">excluir</button>`}</td></tr>`).join('')
          || '<tr><td colspan="5"><span class="vazio">nenhum operador</span></td></tr>'}</tbody></table></div>
      </div>`;

    /* --- alternância de sub-abas (ordem 04/10: dropdown estilizado) --- */
    if (typeof criarDropdown === 'function') {
      criarDropdown($('#abasGerDD'), [
        { valor: 'pessoal', rotulo: 'Pessoal' },
        { valor: 'tags', rotulo: 'Tags' },
        { valor: 'grupos', rotulo: 'Grupos' },
        { valor: 'operadores', rotulo: 'Operadores' },
        { valor: 'funcoes', rotulo: 'Funções' },
        { valor: 'chefes', rotulo: 'Setores' }
      ], {
        valorPadrao: abaGer,
        onChange: (k) => {
          abaGer = k;
          ['pessoal', 'tags', 'grupos', 'operadores', 'funcoes', 'chefes'].forEach(kk => {
            const el = $('#ger' + kk[0].toUpperCase() + kk.slice(1));
            if (el) el.classList.toggle('oculto', kk !== abaGer);
          });
          if (abaGer === 'tags') carregarCats();
          if (abaGer === 'pessoal') atualizarSelectsCatalogos();
          if (abaGer === 'funcoes') carregarFuncoesMembros();
          if (abaGer === 'chefes') { carregarModoSetores(); carregarChefes(); carregarAgregadoSetores(); }
        }
      });
    }

    /* --- ordem 06/10 (item 3): encarregado/auxiliar criam contas do grupo --- */
    const btEncCriar = $('#encCriar');
    if (btEncCriar) {
      btEncCriar.onclick = async () => {
        const ng = ($('#encNg') && $('#encNg').value.trim()) || '';
        const nc = ($('#encNc') && $('#encNc').value.trim()) || '';
        const papelConta = ($('#encPapel') && $('#encPapel').value) || 'operador';
        if (!ng || !nc) { toast('Nome de guerra e nome completo são obrigatórios', 'erro'); return; }
        // mesmo derivador do item 2: login vem do nome de guerra (não-admin)
        const login = ng.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase()
          .replace(/[^a-z0-9]+/g, '.').replace(/^\.+|\.+$/g, '') || 'conta' + Date.now();
        const r = await processar(() => api('/api/usuarios', { method: 'POST', body: JSON.stringify({ login, papel: papelConta }) }), 'Criando conta…');
        if (r.ok) {
          try {
            const rid = r.resultado && r.resultado.id;
            if (rid) {
              await api(`/api/usuarios/${rid}`, { method: 'PATCH', body: JSON.stringify({ nome_guerra: ng, nome_completo: nc }) });
            }
          } catch (e) {}
          toast('Conta criada — senha padrão sci (troca no 1º acesso)');
          window.ViewGrupos();
        }
      };
    }
    /* --- ordem 06/10: edição de nomes da conta pela função de pessoal --- */
    document.querySelectorAll('[data-editu]').forEach(b => {
      b.onclick = async () => {
        const novoNg = prompt('Nome de guerra:', b.dataset.ng || '');
        if (novoNg === null) return;
        const novoNc = prompt('Nome completo:', b.dataset.nc || '');
        if (novoNc === null) return;
        const r = await processar(() => api(`/api/usuarios/${b.dataset.editu}`, { method: 'PATCH', body: JSON.stringify({ nome_guerra: novoNg.trim(), nome_completo: novoNc.trim() }) }), 'Salvando conta…');
        if (r.ok) window.ViewGrupos();
      };
    });

    ligarToggles($('#app'));

    /* --- ordem 06/10 (item 10): controles de tabela nas listas do GERENCIA ---
       Banco de pessoal: ordenar em todas as colunas + paginação 20/página.
       Operadores: idem (trivial — mesma chamada). Demais abas plugam o helper
       nos próprios loaders (funcoes/chefes/agregado/setores). Relatórios NÃO. */
    const tabPTbl = document.querySelector('#gerPessoal table');
    if (tabPTbl) {
      window.tabelaControles('ger-pessoal', tabPTbl, $('#tabP'), [
        null, { tipo: 'num' }, { tipo: 'txt' }, { tipo: 'txt' },
        { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, null
      ], 20);
    }
    const tabOpTbl = document.querySelector('#gerOperadores table');
    if (tabOpTbl) {
      window.tabelaControles('ger-operadores', tabOpTbl, tabOpTbl.querySelector('tbody'), [
        { tipo: 'num' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, null
      ], 20);
    }

    /* --- onda C2 (05/10): aba FUNÇÕES — designação de membros por função ---
       GET /api/grupo/funcoes/membros (escopo do grupo da sessão) + POST/DELETE.
       1 titular por função é garantia do backend (índice parcial único, 409). */
    async function carregarFuncoesMembros() {
      const tb = $('#tabFun');
      if (!tb) return;
      tb.innerHTML = '<tr><td colspan="3"><span class="carregando">…</span></td></tr>';
      try {
        const linhas = await api('/api/grupo/funcoes/membros');
        const porFuncao = {};
        (linhas || []).forEach(l => {
          (porFuncao[l.funcao_id] = porFuncao[l.funcao_id] || { funcao_id: l.funcao_id, funcao_nome: l.funcao_nome, membros: [] }).membros.push(l);
        });
        const ids = Object.keys(porFuncao).sort((a, b) => String(porFuncao[a].funcao_nome).localeCompare(String(porFuncao[b].funcao_nome)));
        if (!ids.length) { tb.innerHTML = '<tr><td colspan="3"><span class="vazio">nenhuma função no catálogo</span></td></tr>'; return; }
        const opContas = contas.filter(c => c.grupo_id === eu.grupo_id && c.ativo)
          .map(c => `<option value="${c.id}">${esc(c.nome_guerra || c.login)} (${esc(c.login)})</option>`).join('');
        tb.innerHTML = ids.map(fid => {
          const f = porFuncao[fid];
          const membros = f.membros.filter(m => m.membro_id > 0);
          const titular = membros.find(m => m.titularidade === 'titular');
          const auxiliares = membros.filter(m => m.titularidade === 'auxiliar');
          const linhaTit = titular
            ? `<b>👑 ${esc(titular.nome_guerra || titular.login)}</b> <button class="acao-linha" data-remfun="${titular.membro_id}">remover</button>`
            : '<span style="color:var(--tx3)">sem titular</span>';
          const linhasAux = auxiliares.map(m => `<div style="margin-top:4px">${esc(m.nome_guerra || m.login)} <button class="acao-linha" data-remfun="${m.membro_id}">remover</button></div>`).join('');
          return `<tr>
            <td><b>${esc(f.funcao_nome)}</b></td>
            <td>${linhaTit}${auxiliares.length ? '<div style="margin-top:6px;border-top:1px dashed var(--borda);padding-top:4px">' + linhasAux + '</div>' : ''}</td>
            <td>
              <div class="form-linha" style="gap:6px;align-items:center;flex-wrap:wrap">
                <select data-seluser="${f.funcao_id}" style="min-width:170px"><option value="">— conta —</option>${opContas}</select>
                <select data-seltit="${f.funcao_id}"><option value="titular">titular</option><option value="auxiliar">auxiliar</option></select>
                <button class="primario" data-addfun="${f.funcao_id}" style="font-size:12px;padding:4px 12px">Designar</button>
              </div>
            </td></tr>`;
        }).join('');
        tb.querySelectorAll('[data-addfun]').forEach(bt => {
          bt.onclick = async () => {
            const fID = +bt.dataset.addfun;
            const selU = tb.querySelector(`[data-seluser="${fID}"]`);
            const selT = tb.querySelector(`[data-seltit="${fID}"]`);
            if (!selU.value) { toast('Escolha a conta', 'erro'); return; }
            const r = await processar(() => api('/api/grupo/funcoes/membros', { method: 'POST', body: JSON.stringify({ funcao_id: fID, usuario_id: +selU.value, titularidade: selT.value }) }), 'Designando…');
            if (r.ok) carregarFuncoesMembros();
          };
        });
        tb.querySelectorAll('[data-remfun]').forEach(bt => {
          bt.onclick = async () => {
            const r = await processar(() => api('/api/grupo/funcoes/membros/' + bt.dataset.remfun, { method: 'DELETE' }), 'Removendo designação…');
            if (r.ok) carregarFuncoesMembros();
          };
        });
        // ordem 06/10 (item 10): ordenar POR nos cabeçalhos (Função / Designados / Designar)
        const tabFunTbl = document.querySelector('#gerFuncoes table');
        if (tabFunTbl) window.tblOrdenar('ger-funcoes', tabFunTbl, tb, [{ tipo: 'txt' }, { tipo: 'txt' }]);
      } catch (e) {
        tb.innerHTML = '<tr><td colspan="3"><span class="vazio">Falha ao carregar funções.</span></td></tr>';
      }
    }
    if (abaGer === 'funcoes') carregarFuncoesMembros();
    if (abaGer === 'chefes') { carregarModoSetores(); carregarChefes(); carregarAgregadoSetores(); }

    /* --- ordem 06/10 (item 8): modo SETORES do Gerenciar ---
       Listagem com botão EDITAR por setor (modal com 3 ações: ALTERAR NOME via
       PATCH /api/catalogo/setores/{id}; NOMEAR CHEFE via POST /api/grupos/{id}/
       nomear_chefe {usuario_id, setor_id} — candidatos = pessoas do setor;
       EXCLUIR via DELETE /api/setores/{id} com confirmação dupla) e NOVO SETOR
       no topo (POST /api/catalogo/setores). */
    async function carregarModoSetores() {
      const tb = $('#tabSetores');
      if (!tb) return;
      tb.innerHTML = '<tr><td colspan="5"><span class="carregando">…</span></td></tr>';
      try {
        const [setoresG, pessoalR, contasR] = await Promise.all([
          api('/api/catalogo/setores'), api('/api/pessoas'), api('/api/usuarios')]);
        const listaS = (setoresG || []).filter(s => s.ativo !== false);
        const gid = eu.grupo_id;
        const doGrupo = (contasR || []).filter(c => c.grupo_id === gid && c.ativo);
        const chefeDeSetor = {};
        doGrupo.forEach(c => {
          (c.papeis || []).forEach(p => {
            if (p.papel === 'chefe_setor' && c.setor_id) chefeDeSetor[c.setor_id] = c;
          });
        });
        if (!listaS.length) {
          tb.innerHTML = '<tr><td colspan="5"><span class="vazio">nenhum setor no catálogo — use NOVO SETOR</span></td></tr>';
        } else {
          tb.innerHTML = listaS.map(s => {
            const ch = chefeDeSetor[s.id];
            return `<tr>
            <td><b>${esc(s.nome)}</b></td>
            <td>${s.sigla ? '<code style="font-size:11px">' + esc(s.sigla) + '</code>' : '<span style="color:var(--tx3)">—</span>'}</td>
            <td>${esc((grupos.find(g => g.id === s.grupo_id) || {}).nome || '—')}</td>
            <td>${ch
              ? `<span class="alerta-ok">👑 ${esc(ch.nome_guerra || ch.login)}</span>`
              : '<span style="color:var(--tx3)">sem chefe</span>'}</td>
            <td><button class="acao-linha" data-edsetor="${s.id}" data-nome="${esc(s.nome)}" data-sigla="${esc(s.sigla || '')}">EDITAR</button></td>
          </tr>`;
          }).join('');
          // ordem 06/10 (item 10): ordenar POR nos cabeçalhos (Setor/Sigla/Unidade/Chefe)
          const tabStTbl = document.querySelector('#gerSetores table');
          if (tabStTbl) window.tblOrdenar('ger-setores', tabStTbl, tb,
            [{ tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }]);
        }
        // ligar EDITAR em cada setor (mesmo com lista vazia não há botões — ok)
        tb.querySelectorAll('[data-edsetor]').forEach(bt => {
          bt.onclick = () => modalEditarSetor(+bt.dataset.edsetor, bt.dataset.nome, bt.dataset.sigla || '');
        });
      } catch (e) {
        tb.innerHTML = '<tr><td colspan="5"><span class="vazio">Falha ao carregar setores.</span></td></tr>';
      }
    }

    // modal EDITAR do setor: 3 ações (nome / chefe / excluir com dupla confirmação)
    function modalEditarSetor(sID, sNome, sSigla) {
      const div = modal(`<div class="modal-inner">
        <h3>🏢 Editar setor — ${esc(sNome)}</h3>
        <div class="form-linha">
          <div class="campo"><label>ALTERAR NOME</label><input id="esNome" value="${esc(sNome)}"></div>
          <div class="campo" style="width:130px"><label>Sigla</label><input id="esSigla" value="${esc(sSigla)}"></div>
        </div>
        <div class="campo"><label>NOMEAR CHEFE — candidatos: pessoal do setor</label>
          <select id="esChefe"><option value="">— selecionar —</option></select>
          <small id="esChefeInfo" style="color:var(--tx2);font-size:11.5px">carregando candidatos…</small></div>
        <div class="modal-acoes" style="justify-content:space-between; flex-wrap:wrap; gap:8px">
          <button class="perigo" id="esExc">🗑 EXCLUIR SETOR</button>
          <div style="display:flex; gap:8px">
            <button class="fantasma" id="esX">Fechar</button>
            <button class="primario" id="esGo">Salvar alterações</button>
          </div>
        </div></div>`);
      if (!div) return;
      // candidatos a chefe: pessoas do setor com conta ativa no grupo
      (async () => {
        const sel = div.querySelector('#esChefe');
        try {
          const pessoal = await api('/api/pessoas');
          const doSetor = (pessoal.pessoas || []).filter(p => p.setor_id === sID && p.status === 'ativo');
          const contasL = await api('/api/usuarios');
          const contaPorPessoa = {};
          (contasL || []).forEach(c => { if (c.pessoa_id) contaPorPessoa[c.pessoa_id] = c; });
          const cand = doSetor.filter(p => {
            const c = contaPorPessoa[p.id];
            return c && c.grupo_id === eu.grupo_id && c.ativo;
          });
          sel.innerHTML = '<option value="">— selecionar —</option>' + cand.map(p => {
            const c = contaPorPessoa[p.id];
            return `<option value="${c.id}">${esc(p.nome_guerra || p.nome_completo)} (${esc(c.login)})</option>`;
          }).join('');
          div.querySelector('#esChefeInfo').textContent = cand.length
            ? cand.length + ' candidato(s) com conta ativa no grupo'
            : 'nenhuma pessoa do setor tem conta ativa no grupo — cadastre em Pessoal ou pelo admin';
        } catch (e) {
          div.querySelector('#esChefeInfo').textContent = 'falha ao carregar candidatos';
        }
      })();
      div.querySelector('#esX').onclick = () => div.fechar && div.fechar();
      // ALTERAR NOME (PATCH catálogo existente)
      div.querySelector('#esGo').onclick = async () => {
        const nome = div.querySelector('#esNome').value.trim();
        const sigla = div.querySelector('#esSigla').value.trim();
        if (!nome) { toast('Informe o nome', 'erro'); return; }
        const r = await processar(() => api('/api/catalogo/setores/' + sID, { method: 'PATCH', body: JSON.stringify({ nome, sigla }) }), 'Salvando setor…');
        if (r.ok) {
          toast('Setor atualizado');
          div.fechar && div.fechar();
          setoresCat = funcoesCat = null;
          window.ViewGrupos();
        }
      };
      // NOMEAR CHEFE (endpoint existente da onda escalas)
      div.querySelector('#esChefe').addEventListener('change', async () => {
        const uid = +div.querySelector('#esChefe').value;
        if (!uid) return;
        const r = await processar(() => api('/api/grupos/' + eu.grupo_id + '/nomear_chefe', { method: 'POST', body: JSON.stringify({ usuario_id: uid, setor_id: sID }) }), 'Nomeando chefe…');
        if (r.ok) {
          toast('Chefe nomeado');
          div.fechar && div.fechar();
          window.ViewGrupos();
        }
      });
      // EXCLUIR SETOR — confirmação dupla padrão do sistema
      div.querySelector('#esExc').onclick = async () => {
        if (!(await confirmar(`Excluir o setor "${sNome}"? O pessoal dele passará para SEM SETOR.`))) return;
        if (!(await confirmar(`TEM CERTEZA? Excluir "${sNome}" NÃO tem volta (setor com histórico de conferências não pode ser excluído — desative).`))) return;
        div.fechar && div.fechar();
        const r = await processar(() => api('/api/setores/' + sID, { method: 'DELETE' }), 'Excluindo setor…');
        if (r.ok) {
          toast(`Setor excluído — ${(r.resultado && r.resultado.pessoas_remanejadas) || 0} pessoa(s) e ${(r.resultado && r.resultado.contas_remanejadas) || 0} conta(s) foram para SEM SETOR`);
          setoresCat = funcoesCat = null;
          window.ViewGrupos();
        }
      };
    }

    /* --- onda itens79 (item 9): aba SETORES — visão agregada do escopo ---
       GET /api/setores/agregado (admin vê tudo; gerente vê próprio grupo +
       subordinados). Somente leitura: nomear/destituir segue na aba Chefes
       (via canônica POST /api/grupos/{id}/nomear_chefe). */
    async function carregarAgregadoSetores() {
      const tb = $('#tabAgreg');
      if (!tb) return;
      tb.innerHTML = '<tr><td colspan="5"><span class="carregando">…</span></td></tr>';
      try {
        const r = await api('/api/setores/agregado');
        const lista = r.setores || [];
        if (!lista.length) {
          tb.innerHTML = '<tr><td colspan="5"><span class="vazio">nenhum setor no escopo — crie em Tags › Estrutura Organizacional</span></td></tr>';
          return;
        }
        tb.innerHTML = lista.map(s => `<tr>
          <td><b>${esc(s.nome)}</b>${s.sigla ? ' <code style="font-size:11px">' + esc(s.sigla) + '</code>' : ''}</td>
          <td>${esc(s.grupo_nome || '—')}</td>
          <td class="num">${s.pessoas || 0}</td>
          <td class="num">${s.contas || 0}</td>
          <td>${s.tem_chefe
            ? `<span class="alerta-ok">👑 ${esc(s.chefe_nome)}</span>`
            : '<span style="color:var(--tx3)">sem chefe</span>'}</td>
        </tr>`).join('');
        // ordem 06/10 (item 10): ordenar POR nos cabeçalhos (Setor/Unidade/Pessoal/Contas/Chefe)
        const tabAgrTbl = document.querySelector('#gerAgregado table');
        if (tabAgrTbl) window.tblOrdenar('ger-agregado', tabAgrTbl, tb,
          [{ tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'num' }, { tipo: 'num' }, { tipo: 'txt' }]);
      } catch (e) {
        tb.innerHTML = '<tr><td colspan="5"><span class="vazio">Falha ao carregar a visão agregada de setores.</span></td></tr>';
      }
    }

    /* --- onda Escalas (05/10): aba CHEFES — nomear/destituir chefe de setor --- */
    async function carregarChefes() {
      const tb = $('#tabChefes');
      if (!tb) return;
      tb.innerHTML = '<tr><td colspan="3"><span class="carregando">…</span></td></tr>';
      try {
        const [setoresG, contasR] = await Promise.all([api('/api/catalogo/setores'), api('/api/usuarios')]);
        const doGrupo = (contasR || []).filter(c => c.grupo_id === eu.grupo_id && c.ativo);
        // ordem 06/10 (item 14): o COMANDO por setor vem do backend —
        // chefe_usuario_id/chefe_nome em /api/catalogo/setores (chefe_setores).
        // Um usuário pode chefiar VÁRIOS setores (mapa setor → chefe).
        const chefeDeSetor = {};
        (setoresG || []).forEach(s => {
          if (s.chefe_usuario_id) chefeDeSetor[s.id] = { id: s.chefe_usuario_id, nome_guerra: s.chefe_nome, login: s.chefe_nome };
        });
        const listaS = (setoresG || []).filter(s => s.ativo !== false);
        if (!listaS.length) { tb.innerHTML = '<tr><td colspan="3"><span class="vazio">nenhum setor no catálogo</span></td></tr>'; return; }
        const opContas = doGrupo.map(c => `<option value="${c.id}">${esc(c.nome_guerra || c.login)} (${esc(c.login)})</option>`).join('');
        tb.innerHTML = listaS.map(s => {
          const ch = chefeDeSetor[s.id];
          const atual = ch
            ? `<b>👑 ${esc(ch.nome_guerra || ch.login)}</b> <small style="color:var(--tx2)">(${esc(ch.login)})</small>
               <button class="acao-linha" data-destituir="${ch.id}" data-setorid="${s.id}" data-nome="${esc(ch.nome_guerra || ch.login)}" data-setor="${esc(s.nome)}">destituir</button>`
            : '<span style="color:var(--tx3)">sem chefe</span>';
          return `<tr>
            <td><b>${esc(s.nome)}</b>${s.sigla ? ' <code style="font-size:11px">' + esc(s.sigla) + '</code>' : ''}</td>
            <td>${atual}</td>
            <td>
              <div class="form-linha" style="gap:6px;align-items:center;flex-wrap:wrap">
                <select data-selchefe="${s.id}" style="min-width:170px"><option value="">— conta —</option>${opContas}</select>
                <button class="primario" data-nomear="${s.id}" data-setor="${esc(s.nome)}" style="font-size:12px;padding:4px 12px">Nomear</button>
              </div>
            </td></tr>`;
        }).join('');
        tb.querySelectorAll('[data-nomear]').forEach(bt => {
          bt.onclick = async () => {
            const sID = +bt.dataset.nomear;
            const sel = tb.querySelector(`[data-selchefe="${sID}"]`);
            if (!sel || !sel.value) { toast('Escolha a conta', 'erro'); return; }
            const r = await processar(() => api('/api/grupos/' + eu.grupo_id + '/nomear_chefe', { method: 'POST', body: JSON.stringify({ usuario_id: +sel.value, setor_id: sID }) }), 'Nomeando chefe…');
            if (r.ok) carregarChefes();
          };
        });
        tb.querySelectorAll('[data-destituir]').forEach(bt => {
          bt.onclick = async () => {
            // ordem 06/10 (item 14): destitui SÓ o comando daquele setor — o chefe
            // que comanda outros setores continua chefe (papel permanece).
            if (!(await confirmar(`Destituir ${bt.dataset.nome} como chefe de ${bt.dataset.setor}?`))) return;
            const r = await processar(() => api('/api/grupos/' + eu.grupo_id + '/destituir_chefe', { method: 'POST', body: JSON.stringify({ usuario_id: +bt.dataset.destituir, setor_id: +bt.dataset.setorid }) }), 'Destituindo chefe…');
            if (r.ok) carregarChefes();
          };
        });
        // ordem 06/10 (item 10): ordenar POR nos cabeçalhos (Setor / Chefe atual / Nomear)
        const tabChTbl = document.querySelector('#gerChefes table');
        if (tabChTbl) window.tblOrdenar('ger-chefes', tabChTbl, tb, [{ tipo: 'txt' }, { tipo: 'txt' }]);
      } catch (e) {
        tb.innerHTML = '<tr><td colspan="3"><span class="vazio">Falha ao carregar chefes.</span></td></tr>';
      }
    }

    // Eventos da Árvore e Cards de Grupos para Gerente (#gerGrupos)
    const gerGrp = $('#gerGrupos');
    if (gerGrp) {
      ligarToggles(gerGrp);
      gerGrp.querySelectorAll('[data-focargrupo]').forEach(b => {
        b.onclick = (e) => {
          e.stopPropagation();
          const gid = +b.dataset.focargrupo;
          FOCO_GRUPO_ID = gid > 0 ? gid : null;
          window.ViewGrupos();
        };
      });
      gerGrp.querySelectorAll('[data-editargrupo]').forEach(b => {
        b.onclick = (e) => {
          e.stopPropagation();
          const gid = +b.dataset.editargrupo;
          let no = buscarNoPorId(arvore, gid);
          if (!no) {
            const gObj = grupos.find(x => x.id === gid);
            if (gObj) {
              no = {
                id: gObj.id,
                nome: gObj.nome,
                codigo: gObj.codigo,
                gerente: gerenteDe[gObj.id] || '',
                efetivo: gObj.efetivo || 0,
                contas: gObj.contas || 0,
                filhos: []
              };
            }
          }
          if (no) modalEditarGrupo(no, () => window.ViewGrupos(), { grupos, contas });
        };
      });
    }

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
          let sid = (optSetores.find(s => s.nome.toLowerCase() === (st || '').toLowerCase()) || {}).id || null;
          if (st && !sid) {
            try {
              const resSt = await api('/api/catalogo/setores', { method: 'POST', body: JSON.stringify({ nome: st }) });
              if (resSt && resSt.id) {
                sid = resSt.id;
                optSetores.push({ id: sid, nome: st, ativo: 1 });
              }
            } catch (e) {}
          }
          let fid = (optFuncoes.find(s => s.nome.toLowerCase() === (fn || '').toLowerCase()) || {}).id || null;
          if (fn && !fid) {
            try {
              const resFn = await api('/api/catalogo/funcoes', { method: 'POST', body: JSON.stringify({ nome: fn }) });
              if (resFn && resFn.id) {
                fid = resFn.id;
                optFuncoes.push({ id: fid, nome: fn, ativo: 1 });
              }
            } catch (e) {}
          }
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
      const nSel2 = $('#nSel2'); // ausente p/ função de pessoal (sem excluir em lote)
      if (nSel2) nSel2.textContent = n;
      const btExc = $('#btExcLote');
      if (btExc) btExc.disabled = n === 0;
      $('#btEditLote').disabled = n === 0;
    };
    document.querySelectorAll('.chkP').forEach(ch => ch.onchange = refreshSel);
    $('#chkTodosP').onchange = () => {
      document.querySelectorAll('.chkP').forEach(ch => { ch.checked = $('#chkTodosP').checked; });
      refreshSel();
    };
    const btExcLoteEl = $('#btExcLote');
    if (btExcLoteEl) btExcLoteEl.onclick = async () => {
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

    /* --- imprimir ficha de pessoal --- */
    document.querySelectorAll('#tabP [data-fichap]').forEach(bt => {
      bt.onclick = ev => {
        ev.stopPropagation();
        const id = bt.dataset.fichap;
        window.open('/api/pessoas/' + id + '/pdf', '_blank');
      };
    });

    /* --- onda GPX (item 2): edição de apresentação (presente → dispensado etc.) --- */
    document.querySelectorAll('#tabP [data-apresentacao]').forEach(bt => {
      bt.onclick = ev => {
        ev.stopPropagation();
        const pid = bt.dataset.apresentacao;
        const nomeGuerra = bt.dataset.nome || '';
        const atual = mapaApresentacao[pid] || {};
        const estadoAtual = atual.estado || 'presente';

        const div = modal(`<div class="modal-inner" style="max-width:440px">
          <h3>Apresentação — ${esc(nomeGuerra)}</h3>
          <div class="campo" style="margin-bottom:12px">
            <label>Estado de Apresentação</label>
            <div id="gpxAprEstadoDD"></div>
          </div>
          <div class="campo" style="margin-bottom:16px">
            <label>Motivo (opcional)</label>
            <textarea id="gpxAprMotivo" rows="3" placeholder="Justificativa ou observação…">${esc(atual.motivo || '')}</textarea>
          </div>
          <div class="modal-acoes" style="display:flex;justify-content:flex-end;gap:8px">
            <button class="fantasma" id="gpxAprCanc">Cancelar</button>
            <button class="primario" id="gpxAprReg">Registrar</button>
          </div>
        </div>`);
        if (!div) return;

        let estadoEscolhido = estadoAtual;
        const ddContainer = div.querySelector('#gpxAprEstadoDD');
        if (ddContainer && typeof criarDropdown === 'function') {
          criarDropdown(ddContainer, [
            { valor: 'presente', rotulo: 'Presente' },
            { valor: 'dispensado', rotulo: 'Dispensado' },
            { valor: 'descompensado', rotulo: 'Descompensado' },
            { valor: 'a serviço externo', rotulo: 'A serviço externo' },
            { valor: 'atrasado', rotulo: 'Atrasado' },
            { valor: 'falta', rotulo: 'Falta' }
          ], {
            valorPadrao: estadoAtual,
            onChange: v => { estadoEscolhido = v; }
          });
        }

        const fecharModal = () => { div.remove(); };
        const btCanc = div.querySelector('#gpxAprCanc');
        if (btCanc) btCanc.onclick = fecharModal;

        const btReg = div.querySelector('#gpxAprReg');
        if (btReg) {
          btReg.onclick = async () => {
            const motivoTxt = (div.querySelector('#gpxAprMotivo')?.value || '').trim();
            btReg.disabled = true;
            try {
              const resp = await api(`/api/pessoas/${pid}/apresentacao`, {
                method: 'POST',
                body: JSON.stringify({ estado: estadoEscolhido, motivo: motivoTxt })
              });
              if (resp && resp.ok) {
                toast('Apresentação registrada');
                fecharModal();
                window.ViewGrupos();
              } else {
                toast((resp && resp.erro) || 'Falha ao registrar apresentação', 'erro');
                btReg.disabled = false;
              }
            } catch (err) {
              toast((err && err.message) || 'Erro ao comunicar com o servidor', 'erro');
              btReg.disabled = false;
            }
          };
        }
      };
    });

    /* --- onda GPX (item 3): trilha de modificações por pessoa --- */
    document.querySelectorAll('#tabP [data-historico]').forEach(bt => {
      bt.onclick = async ev => {
        ev.stopPropagation();
        const pid = bt.dataset.historico;
        const nomeGuerra = bt.dataset.nome || '';

        const div = modal(`<div class="modal-inner" style="max-width:540px">
          <h3>Histórico de Modificações — ${esc(nomeGuerra)}</h3>
          <div id="gpxHistCorpo"><div class="carregando">…</div></div>
          <div class="modal-acoes" style="display:flex;justify-content:flex-end;margin-top:14px">
            <button class="fantasma" id="gpxHistFechar">Fechar</button>
          </div>
        </div>`);
        if (!div) return;

        const btFechar = div.querySelector('#gpxHistFechar');
        if (btFechar) btFechar.onclick = () => { div.remove(); };

        try {
          const r = await api(`/api/pessoas/${pid}/modificacoes`);
          const lista = (r && r.modificacoes) || [];
          const cont = div.querySelector('#gpxHistCorpo');
          if (!cont) return;
          if (!lista.length) {
            cont.innerHTML = '<div class="gpx-hist-vazio">Nenhuma modificação registrada até o momento.</div>';
          } else {
            cont.innerHTML = `<div class="gpx-hist">${lista.map(m => `
              <div class="gpx-hist-item">
                <div class="gpx-hist-quando">${fmtData(m.quando)} ${fmtHora(m.quando)}${m.fonte ? ` <span style="opacity:0.7">(${esc(m.fonte)})</span>` : ''}</div>
                <div class="gpx-hist-quem">${esc(m.quem || 'Sistema')}</div>
                <div class="gpx-hist-acao">${esc(m.acao || '—')}</div>
              </div>`).join('')}</div>`;
          }
        } catch (e) {
          const cont = div.querySelector('#gpxHistCorpo');
          if (cont) cont.innerHTML = '<div class="gpx-hist-vazio">Falha ao carregar modificações.</div>';
        }
      };
    });

    /* --- onda GPX (item 4): relatório de faltas e atrasos de conferência fechada --- */
    (async () => {
      const relDDCont = $('#gpxRelConfDD');
      const btFaltas = $('#gpxBtFaltas');
      const btAtrasos = $('#gpxBtAtrasos');
      if (!relDDCont || !btFaltas || !btAtrasos) return;

      let confsFechadas = [];
      try {
        const respConfs = await api('/api/conferencia/lista');
        confsFechadas = (respConfs || []).filter(c => c && c.status === 'fechada');
      } catch (err) {
        confsFechadas = [];
      }

      let confIdSelecionada = confsFechadas.length ? String(confsFechadas[0].id) : null;

      if (!confsFechadas.length) {
        relDDCont.innerHTML = '<span style="color:var(--tx3);font-size:12.5px">Nenhuma conferência fechada encontrada</span>';
        btFaltas.disabled = true;
        btAtrasos.disabled = true;
        return;
      }

      const opcoesDD = confsFechadas.map(c => ({
        valor: String(c.id),
        rotulo: `#${c.id} — ${fmtData(c.data)} ${c.nome ? '· ' + c.nome : ''} (${c.lancamentos || 0} lançamentos)`
      }));

      if (typeof criarDropdown === 'function') {
        criarDropdown(relDDCont, opcoesDD, {
          valorPadrao: confIdSelecionada,
          onChange: val => { confIdSelecionada = val; }
        });
      }

      btFaltas.onclick = () => {
        if (!confIdSelecionada) { toast('Selecione uma conferência fechada', 'erro'); return; }
        window.open('/api/conferencia/' + encodeURIComponent(confIdSelecionada) + '/relatorio.pdf?filtro=faltas', '_blank');
      };

      btAtrasos.onclick = () => {
        if (!confIdSelecionada) { toast('Selecione uma conferência fechada', 'erro'); return; }
        window.open('/api/conferencia/' + encodeURIComponent(confIdSelecionada) + '/relatorio.pdf?filtro=atrasos', '_blank');
      };
    })();

    /* --- aba TAGS: Catálogos divididos em Pessoal e Material (v1.5) --- */
    const rotCat = {
      destinos: '📍 Destinos (Pessoal)',
      funcoes: '🎖️ Postos / Graduações (Pessoal)',
      tags: '🏷️ Situação do Material (Material)',
      material_tipos: '📦 Tipos de Material (Material)',
      material_classes: '🎖️ Classes de Material (Material)',
      setores: '🏢 Setores / Seções (Estrutura)'
    };
    
    // Toggle de campos no form do topo
    const cgTSel = $('#cgT');
    if (cgTSel) {
      cgTSel.onchange = () => {
        const val = cgTSel.value;
        const corWrap = $('#cgCorWrap');
        const siglaWrap = $('#cgSiglaWrap');
        if (corWrap) corWrap.style.display = (val === 'tags' || val === 'material_tipos' || val === 'material_classes') ? 'block' : 'none';
        if (siglaWrap) siglaWrap.style.display = val === 'setores' ? 'block' : 'none';
      };
    }

    const carregarCats = async () => {
      const cont = $('#catGer');
      if (!cont) return;
      const tipos = ['destinos', 'tags', 'material_tipos', 'material_classes', 'setores'];
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
      
      let html = `
        <div style="background:rgba(59,130,246,0.08);border:1px solid rgba(59,130,246,0.25);border-radius:8px;padding:12px;margin-bottom:18px;font-size:12.5px;color:var(--tx2)">
          💡 <b>Doutrina de Organização v1.5:</b> As tags estão divididas em <b>Pessoal</b> (Situação/Destino) e <b>Material</b> (Situação/Tipo/Classe). A tag de setor do militar é preenchida automaticamente pelo Setor ao qual ele está alocado. As <b>Funções administrativas</b> (Encarregado de Pessoal e auxiliares) são geridas na aba <b>Funções</b>.
        </div>`;

      for (const [t, lista] of resultados) {
        const minhas = lista.filter(x => x.grupo_id === meuGid);
        const subordinadas = lista.filter(x => x.grupo_id && subsIds.has(x.grupo_id));
        const herdadas = lista.filter(x => x.grupo_id !== meuGid && (!x.grupo_id || !subsIds.has(x.grupo_id)));

        const nivelTag = x => {
          let n = 0, pai = x.pai_id;
          while (pai != null) { const p = lista.find(y => y.id === pai); if (!p) break; n++; pai = p.pai_id; }
          return n;
        };

        const chipCorTag = x => (t === 'tags' || t === 'material_tipos' || t === 'material_classes') && x.cor
          ? `<span style="display:inline-block; width:12px; height:12px; border-radius:50%; background:${esc(x.cor)}; margin-right:6px; vertical-align:middle; border:1px solid rgba(255,255,255,0.2)"></span>`
          : '';

        const linhaLista = (x, tipoPermissao, nomeOrigem) => {
          const podeGerenciar = tipoPermissao === 'meu' || tipoPermissao === 'subordinado';
          const recuo = ((t === 'tags' || t === 'material_tipos') && tipoPermissao === 'meu') ? `margin-left:${nivelTag(x) * 18}px` : '';

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
                  ${souFuncaoPessoal ? '' : `<button class="acao-linha perigo" style="font-size:11.5px; padding:2px 8px" data-delcat="${t}" data-cid="${x.id}">excluir</button>`}
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

        const btAnt = (t === 'tags' || t === 'setores' || t === 'funcoes' || t === 'material_tipos') && minhas.length >= 2
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
        atualizarSelectsCatalogos();
      }
    };
    if (abaGer === 'tags') carregarCats(); else $('#gerTags').addEventListener('renderTags', carregarCats, { once: true });

    /* --- operadores: senha/mover/excluir + filtro (ordem 04/10: gerente NÃO
       cria operador — o form de criação foi removido; chefes designam) --- */
    $('#fOp').oninput = () => {
      const q = $('#fOp').value.trim().toLowerCase();
      document.querySelectorAll('#gerOperadores tbody tr[data-login]').forEach(tr => {
        tr.style.display = !q || tr.dataset.login.toLowerCase().includes(q) ? '' : 'none';
      });
    };
    document.querySelectorAll('#gerOperadores [data-mv]').forEach(b => b.onclick = () =>
      modalMover(b.dataset.mv, b.dataset.login, optsMoverGer,
        'Permitido apenas entre o seu grupo e seus subordinados.', () => window.ViewGrupos()));
    // ordem 06/10 (item 8b): NOVO SETOR no topo do modo Setores (POST catálogo existente)
    // + NOVO SETOR: mesmo handler para o botão do bloco antigo (Tags, se existir)
    // e o do bloco GESTÃO da aba Setores (ordem Diretor 07/10).
    const abrirModalNovoSetor = () => {
      const div = modal(`<div class="modal-inner" style="max-width:420px">
        <h3>🏢 NOVO SETOR</h3>
        <div class="campo"><label>Nome do setor *</label><input id="nsNome" placeholder="ex.: Seção de Comunicação Social"></div>
        <div class="campo"><label>Sigla (opcional)</label><input id="nsSigla" placeholder="ex.: SCCOM"></div>
        <div class="modal-acoes">
          <button class="fantasma" id="nsX">Cancelar</button>
          <button class="primario" id="nsGo">Criar setor</button></div></div>`);
      if (!div) return;
      div.querySelector('#nsNome').focus();
      div.querySelector('#nsX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#nsGo').onclick = async () => {
        const nome = div.querySelector('#nsNome').value.trim();
        const sigla = div.querySelector('#nsSigla').value.trim();
        if (!nome) { toast('Informe o nome do setor', 'erro'); return; }
        const r = await processar(() => api('/api/catalogo/setores', { method: 'POST', body: JSON.stringify({ nome, sigla }) }), 'Criando setor…');
        if (r.ok) {
          toast('Setor criado');
          div.fechar && div.fechar();
          setoresCat = funcoesCat = null;
          window.ViewGrupos();
        }
      };
    };
    const btNovoSetor = $('#btNovoSetor');
    if (btNovoSetor) btNovoSetor.onclick = abrirModalNovoSetor;
    const btNovoSetorCh = $('#btNovoSetorChefes');
    if (btNovoSetorCh) btNovoSetorCh.onclick = abrirModalNovoSetor;
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
     #/perfil — PERFIL DO USUÁRIO & FOTO 1X1 (disponível para todos os perfis)
     ===================================================================== */
  window.ViewPerfil = async function () {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    navAtiva('#/perfil');
    $('#app').innerHTML = '<div class="carregando">Carregando perfil…</div>';
    
    let d;
    try {
      d = await api('/api/perfil');
    } catch (e) {
      $('#app').innerHTML = '<div class="vazio">Falha ao carregar perfil.</div>';
      return;
    }

    const u = d.usuario;
    let fotoAtual = u.foto_base64 || '';
    const sangueOpts = ["", "A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"];

    $('#app').innerHTML = `
      <div style="margin-bottom:18px">
        <h2 style="margin:0 0 4px">Meu Perfil de Usuário</h2>
        <p style="color:var(--tx2);font-size:13px;margin:0">Informações cadastrais, identificação institucional e foto de perfil 1x1.</p>
      </div>

      <div class="perfil-card">
        <!-- Topo: Foto 1x1 e Resumo de Identificação -->
        <div class="perfil-topo">
          <div class="perfil-foto-wrapper">
            <div class="perfil-foto-preview" id="pfFotoBox">
              ${fotoAtual ? `<img src="${fotoAtual}" alt="Foto 1x1">` : `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`}
            </div>
          </div>
          <div class="perfil-foto-acoes">
            <div style="font-size:17px; font-weight:800; color:var(--tx)">
              ${esc(u.nome_guerra || u.login)}
              ${u.tipo_sanguineo ? `<span style="font-size:11.5px; margin-left:6px; background:rgba(239,68,68,0.18); color:var(--verm-txt); border:1px solid rgba(239,68,68,0.35); padding:2px 7px; border-radius:10px; font-weight:700">🩸 ${esc(u.tipo_sanguineo)}</span>` : ''}
            </div>
            <div style="font-size:12.5px; color:var(--tx3)">
              ${esc(u.nome_completo || 'Nome completo não informado')}
            </div>
            <div style="display:flex; gap:8px; align-items:center; margin-top:4px">
              <input type="file" id="pfInpFile" accept="image/*" style="display:none">
              <button type="button" class="primario" id="pfBtAlterarFoto" style="font-size:12px; padding:6px 14px">📷 Alterar Foto 1x1</button>
              ${fotoAtual ? `<button type="button" class="acao-linha perigo" id="pfBtRemoverFoto" style="font-size:12px; padding:6px 12px">Remover Foto</button>` : ''}
            </div>
            <small style="color:var(--tx3); font-size:11px">Foto 1x1: o sistema recorta e compacta centralizadamente no seu navegador.</small>
          </div>
        </div>

        <!-- Seção 1: Identificação no Sistema (Leitura) -->
        <div class="perfil-secao-tit">1. Identificação no Sistema</div>
        <div class="perfil-grid-campos">
          <div class="campo">
            <label>ID do Usuário</label>
            <input value="#${u.id}" disabled style="background:var(--painel3); font-weight:700">
          </div>
          <div class="campo">
            <label>Login de Acesso</label>
            <input value="${esc(u.login)}" disabled style="background:var(--painel3); font-weight:700">
          </div>
          <div class="campo">
            <label>Função / Papel na Conta</label>
            <input value="${esc(rotuloPapel(u.papel))}" disabled style="background:var(--painel3)">
          </div>
          <div class="campo">
            <label>Unidade / Grupo</label>
            <input value="${esc(d.grupo || 'Global / Sem grupo')}" disabled style="background:var(--painel3)">
          </div>
        </div>

        <!-- Seção 2: Dados Pessoais & Militares -->
        <div class="perfil-secao-tit">2. Dados Pessoais & Militares</div>
        <div class="perfil-grid-campos">
          <div class="campo">
            <label>NOME (Nome de Guerra) *</label>
            <input id="pfNg" value="${esc(u.nome_guerra || '')}" placeholder="ex.: SILVA">
          </div>
          <div class="campo">
            <label>NOME COMPLETO *</label>
            <input id="pfNc" value="${esc(u.nome_completo || '')}" placeholder="Nome civil completo">
          </div>
          <div class="campo">
            <label>DATA NASC (Data de Nascimento)</label>
            <input id="pfDataNasc" type="date" value="${esc(u.data_nascimento || '')}">
          </div>
          <div class="campo">
            <label>TIPO SANGUÍNEO</label>
            <select id="pfTipoSang">
              <option value="">Não informado</option>
              ${sangueOpts.filter(Boolean).map(s => `<option value="${s}" ${u.tipo_sanguineo === s ? 'selected' : ''}>${s}</option>`).join('')}
            </select>
          </div>
        </div>

        <!-- Seção 3: Comunicação & Endereço -->
        <div class="perfil-secao-tit">3. Comunicação & Endereço</div>
        <div class="perfil-grid-campos">
          <div class="campo">
            <label>TELEFONE (Celular / WhatsApp)</label>
            <input id="pfTel" type="tel" value="${esc(u.telefone || '')}" placeholder="(XX) XXXXX-XXXX">
          </div>
          <div class="campo">
            <label>EMAIL (Institucional ou Pessoal)</label>
            <input id="pfEmail" type="email" value="${esc(u.email || '')}" placeholder="usuario@dominio.eb.mil.br">
          </div>
        </div>
        <div class="campo" style="margin-top:10px">
          <label>ENDEREÇO (Residencial / Contato)</label>
          <input id="pfEndereco" value="${esc(u.endereco || '')}" placeholder="Logradouro, número, complemento, bairro, cidade - UF">
        </div>

        <!-- Botões de Ação -->
        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:24px; padding-top:16px; border-top:1px solid var(--borda); flex-wrap:wrap; gap:10px">
          <button type="button" class="acao-linha" id="pfBtMudarSenha">🔑 Alterar Minha Senha</button>
          <div style="display:flex; gap:10px">
            <button type="button" class="primario" id="pfGo" style="padding:10px 22px; font-weight:700">💾 Salvar Perfil</button>
          </div>
        </div>
      </div>
    `;

    // Eventos de Foto 1x1
    const fotoBox = $('#pfFotoBox');
    const inpFile = $('#pfInpFile');
    $('#pfBtAlterarFoto').onclick = () => inpFile.click();

    inpFile.onchange = (e) => {
      const file = e.target.files && e.target.files[0];
      if (file) {
        processarFoto1x1(file, (dataUrl) => {
          fotoAtual = dataUrl;
          fotoBox.innerHTML = `<img src="${fotoAtual}" alt="Foto 1x1">`;
          toast('Foto 1x1 processada. Clique em "Salvar Perfil" para confirmar.', 'ok');
        });
      }
    };

    const btRemFoto = $('#pfBtRemoverFoto');
    if (btRemFoto) {
      btRemFoto.onclick = () => {
        fotoAtual = '';
        fotoBox.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`;
        btRemFoto.remove();
        toast('Foto removida. Clique em "Salvar Perfil" para confirmar.');
      };
    }

    // Modal de Senha (ordem Diretor 04/10/26: atual + nova + confirmação antes do save).
    // window.modalSenha = versão do core.js (preflight em /api/senha); o modalSenha local
    // daqui é o de REDEFINIÇÃO por admin/gerente (POST /api/usuarios/{id}/senha) — chamar
    // o local sem id era o bug do "ID não reconhecido" no Meu Perfil.
    $('#pfBtMudarSenha').onclick = () => window.modalSenha();

    // Salvar Perfil
    $('#pfGo').onclick = async () => {
      const ng = $('#pfNg').value.trim();
      const nc = $('#pfNc').value.trim();
      const dataNasc = $('#pfDataNasc').value;
      const tipoSang = $('#pfTipoSang').value;
      const tel = $('#pfTel').value.trim();
      const email = $('#pfEmail').value.trim();
      const endr = $('#pfEndereco').value.trim();

      if (!ng || !nc) {
        toast('Nome de guerra e nome completo são obrigatórios', 'erro');
        return;
      }

      $('#pfGo').disabled = true;
      try {
        await api('/api/perfil', {
          method: 'PATCH',
          body: JSON.stringify({
            nome_guerra: ng,
            nome_completo: nc,
            data_nascimento: dataNasc,
            tipo_sanguineo: tipoSang,
            telefone: tel,
            email: email,
            endereco: endr,
            foto_base64: fotoAtual
          })
        });

        // Atualiza o estado em memória local ME
        const atual = quem();
        if (atual) {
          atual.nome_guerra = ng;
          atual.nome_completo = nc;
          atual.data_nascimento = dataNasc;
          atual.tipo_sanguineo = tipoSang;
          atual.telefone = tel;
          atual.email = email;
          atual.endereco = endr;
          atual.foto_base64 = fotoAtual;
        }

        // Atualiza avatar e nome na Sidebar em tempo real
        const sbAvatar = document.getElementById('sbAvatarWrapper');
        if (sbAvatar) {
          if (fotoAtual) {
            sbAvatar.innerHTML = `<img src="${fotoAtual}" class="sidebar-avatar-img" alt="Foto">`;
          } else {
            sbAvatar.innerHTML = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle cx="12" cy="8" r="4" stroke="currentColor" stroke-width="2"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`;
          }
        }
        const sbNome = document.querySelector('.sidebar-usuario-nome');
        if (sbNome) sbNome.textContent = ng || atual.login;

        toast('Perfil salvo com sucesso!');
        $('#pfGo').disabled = false;
        window.ViewPerfil();
      } catch (e) {
        $('#pfGo').disabled = false;
      }
    };
  };
})();
