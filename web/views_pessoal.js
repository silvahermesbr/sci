/* SCI — MÓDULO PESSOAL (f2/modulo-pessoal): window.ViewPessoal (async).
   Extraído de views_gestao.js (recorte do bloco Gerenciar) e reorganizado:
   EFETIVO (ex-aba Pessoal) · FUNÇÕES · SETORES (Gestão + Chefias + Panorama).
   Acessível ao gerente e ao encarregado/auxiliar de pessoal (gestorPessoal());
   Gerenciar (#/grupos) fica com a estrutura do grupo (Tags · Grupos · Operadores).
   Consome APENAS helpers globais do core.js (eager): api, esc, toast, fmtData,
   confirmar, modal, processar, navAtiva, criarDropdown. Ordenação/paginação de
   tabela são LOCAIS (pesOrdenar/pesTabelaControles) — autocontido: o lazy pode
   carregá-lo sem views_gestao.js (deep-link F5). Vanilla, sem build, sem CDN. */
'use strict';
(function () {
  const $ = s => document.querySelector(s);
  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const ativosDe = l => (l || []).filter(x => x.ativo === 1 || x.ativo === true);

  /* ---------- ordenação + paginação LOCAIS (autocontidas) ---------- */
  const TBL_PES = {}; // chave → { col, asc, tipo, pag, tamPag }
  function pesHeaders(table) { return [...table.querySelectorAll('thead th')]; }
  function pesLinhas(tbody) { return [...tbody.querySelectorAll('tr')]; }
  const pesLinhaVazio = tr => tr.querySelector('.vazio, .carregando') !== null;
  function pesPaginarRender(chave, table, tbody) {
    const st = TBL_PES[chave];
    if (!st || !st.tamPag) return;
    const linhas = pesLinhas(tbody).filter(tr => !pesLinhaVazio(tr) && !tr.classList.contains('conf-toggle-detalhe'));
    const pags = Math.max(1, Math.ceil(linhas.length / st.tamPag));
    st.pag = Math.min(st.pag || 1, pags);
    linhas.forEach((tr, i) => { tr.style.display = (i >= (st.pag - 1) * st.tamPag && i < st.pag * st.tamPag) ? '' : 'none'; });
    let pagDiv = table.parentElement.parentElement.querySelector('.tbl-pag');
    if (pags <= 1) { if (pagDiv) pagDiv.remove(); return; }
    if (!pagDiv) {
      pagDiv = document.createElement('div');
      pagDiv.className = 'tbl-pag';
      table.parentElement.parentElement.insertAdjacentElement('afterend', pagDiv);
    }
    const info = {
      pagina: st.pag,
      totalPaginas: pags,
      inicio: linhas.length === 0 ? 0 : (st.pag - 1) * st.tamPag + 1,
      fim: Math.min(st.pag * st.tamPag, linhas.length),
      totalItens: linhas.length
    };
    if (typeof renderPaginadorHTML === 'function') {
      pagDiv.innerHTML = renderPaginadorHTML(info, chave);
    } else {
      pagDiv.innerHTML = `<button type="button" class="paginacao-btn" data-pg="${st.pag - 1}" ${st.pag <= 1 ? 'disabled' : ''}>‹</button>
        <span class="paginacao-info" style="font-size:12px;color:var(--tx2);margin:0 8px">Página ${st.pag} de ${pags}</span>
        <button type="button" class="paginacao-btn" data-pg="${st.pag + 1}" ${st.pag >= pags ? 'disabled' : ''}>›</button>`;
    }
    pagDiv.querySelectorAll('.paginacao-btn[data-pg]').forEach(b => {
      b.onclick = () => { st.pag = +b.dataset.pg; pesPaginarRender(chave, table, tbody); };
    });
  }
  function pesOrdenarPor(chave, table, tbody, col, tipo, asc) {
    const linhas = pesLinhas(tbody).filter(tr => !pesLinhaVazio(tr) && !tr.classList.contains('conf-toggle-detalhe'));
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
    pesHeaders(table).forEach((th, i) => {
      th.classList.remove('sorted-asc', 'sorted-desc');
      if (i === col) th.classList.add(asc ? 'sorted-asc' : 'sorted-desc');
    });
    (TBL_PES[chave] = TBL_PES[chave] || {}).col = col;
    TBL_PES[chave].asc = asc;
    TBL_PES[chave].tipo = tipo;
    TBL_PES[chave].pag = 1;
    pesPaginarRender(chave, table, tbody);
  }
  function pesOrdenar(chave, table, tbody, cols) {
    if (!table || !tbody) return;
    (TBL_PES[chave] = TBL_PES[chave] || {});
    pesHeaders(table).forEach((th, i) => {
      if (!cols[i]) return;
      th.style.cursor = 'pointer';
      th.title = 'Ordenar';
      th.onclick = () => {
        const st = TBL_PES[chave];
        const asc = !(st.col === i && st.asc);
        pesOrdenarPor(chave, table, tbody, i, cols[i].tipo, asc);
      };
    });
  }
  function pesTabelaControles(chave, table, tbody, cols, tamPag) {
    if (!table || !tbody) return;
    (TBL_PES[chave] = TBL_PES[chave] || {}).tamPag = tamPag;
    pesHeaders(table).forEach((th, i) => {
      if (!cols[i]) return;
      th.style.cursor = 'pointer';
      th.title = 'Ordenar';
      th.onclick = () => {
        const st = TBL_PES[chave];
        const asc = !(st.col === i && st.asc);
        pesOrdenarPor(chave, table, tbody, i, cols[i].tipo, asc);
      };
    });
    pesOrdenarPor(chave, table, tbody, 0, 'num', true); // estado inicial: ordena e pagina
  }

  let abaPes = 'efetivo';    // sub-aba corrente do módulo Pessoal
  let setoresCat = null, funcoesCat = null; // cache (v9.16.9)
  const expandedChefiasSetores = new Set(); // setores com toggle expandido na aba Chefias

  window.ViewPessoal = async function () {
    const eu = quem();
    // ordem 06/10 (P4): encarregado/auxiliar de pessoal também gerenciam —
    // mesmas abas; ações restritivas dentro delas são filtradas por gestorPessoal
    // (senha de conta segue gerente/admin no servidor).
    if (!eu || (eu.papel !== 'gerente' && !(window.gestorPessoal && window.gestorPessoal()))) { location.hash = '#/hoje'; return; }
    // ordem 06/10: quem chega aqui sem papel do sistema é encarregado/auxiliar —
    // fix 09/10: podeDesignar = poder REAL de designar (gerente/admin no
    // servidor); gestorPessoal() de designado não injeta botões que o
    // servidor nega (403), nem esconde do gerente.
    // o servidor nega exclusão de catálogo/pessoa, senha e mover; o front esconde.
    const souFuncaoPessoal = eu.papel !== 'gerente';
    const ehGerente = !!(window.ME && window.ME.papel === 'gerente');
    const podeDesignar = (window.ME && window.ME.papel === 'gerente') || !!(window.gestorPessoal && window.gestorPessoal());
    navAtiva('#/pessoal');
    $('#app').innerHTML = '<div class="carregando">…</div>';
    const [grupos, pessoas, setores, funcoes, contas, apresentacaoDados] = await Promise.all([
      api('/api/grupos'), api('/api/pessoas'),
      api('/api/catalogo/setores'), api('/api/catalogo/funcoes'), api('/api/usuarios'),
      api('/api/pessoas/apresentacao').catch(() => ({ apresentacao: [] }))]);
    const mapaApresentacao = {};
    ((apresentacaoDados && apresentacaoDados.apresentacao) || []).forEach(a => {
      if (a && a.pessoa_id != null) mapaApresentacao[a.pessoa_id] = a;
    });
    const podeApresentacao = typeof window.gestorPessoal === 'function' ? (window.gestorPessoal() || (typeof window.ehEncarregado === 'function' && window.ehEncarregado())) : false;
    let optSetores = ativosDe(setores), optFuncoes = ativosDe((funcoes && funcoes.funcoes) || funcoes);
    setoresCat = setores; funcoesCat = funcoes; // cache

    async function atualizarSelectsCatalogos() {
      try {
        const [setoresNovos, funcoesNovas] = await Promise.all([
          api('/api/catalogo/setores'), api('/api/catalogo/funcoes')
        ]);
        optSetores = ativosDe(setoresNovos);
        optFuncoes = ativosDe((funcoesNovas && funcoesNovas.funcoes) || funcoesNovas);
        const selS = $('#fPesSetor'), selF = $('#fPesAntiguidade');
        if (selS) {
          const val = selS.value;
          selS.innerHTML = '<option value="">Todos os setores</option><option value="__sem_setor__">Sem setor</option>' + optSetores.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('');
          selS.value = val;
        }
        if (selF) {
          const val = selF.value;
          selF.innerHTML = '<option value="">Todas as antiguidades</option><option value="__sem_antiguidade__">Sem antiguidade</option>' + optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('');
          selF.value = val;
        }
      } catch (e) {}
    }

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

    // ordem 04/10: abas do módulo → DROPDOWN estilizado
    $('#app').innerHTML = `<h2>Pessoal</h2>
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:6px">
        <span style="font-size:12px;color:var(--tx2)">Seção:</span>
        <div id="abasPesDD" style="min-width:200px"></div></div>
      <div id="pesEfetivo" class="${abaPes === 'efetivo' ? '' : 'oculto'}">
        <div class="cartao" style="margin-bottom:12px">
          <!-- Topo do Banco de Pessoal: título + botões cadastrar / importar lote -->
          <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:10px;margin-bottom:12px">
            <h3 style="margin:0">BANCO DE PESSOAL (<span id="contagemPes">${(pessoas.pessoas || []).length}</span>)</h3>
            <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
              <button type="button" class="primario" id="btAbrirCadastrar" style="min-height:36px;display:flex;align-items:center;gap:6px">
                <span style="font-size:16px;line-height:1">+</span> Cadastrar militar
              </button>
              <button type="button" class="acao-linha" id="btAbrirLoteCsv" style="min-height:36px;display:flex;align-items:center;gap:6px" title="Importar múltiplos militares">
                📥 Importar em lote
              </button>
            </div>
          </div>

          <!-- Tabela de Filtros -->
          <div class="filtros-pessoal-grid" style="display:grid;grid-template-columns:repeat(auto-fit, minmax(180px, 1fr));gap:10px;align-items:flex-end;margin-bottom:14px;background:var(--painel2);padding:12px;border-radius:var(--raio-p);border:1px solid var(--borda)">
            <div class="campo" style="margin:0">
              <label style="font-size:12px;font-weight:600;color:var(--tx2)">Pesquisar militar</label>
              <input type="text" id="fPesBusca" list="listaPesMilitares" placeholder="Digite o nome..." autocomplete="off">
              <datalist id="listaPesMilitares"></datalist>
            </div>
            <div class="campo" style="margin:0">
              <label style="font-size:12px;font-weight:600;color:var(--tx2)">Setor</label>
              <select id="fPesSetor">
                <option value="">Todos os setores</option>
                <option value="__sem_setor__">Sem setor</option>
                ${optSetores.map(s => `<option value="${s.id}">${esc(s.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="margin:0">
              <label style="font-size:12px;font-weight:600;color:var(--tx2)">Antiguidade</label>
              <select id="fPesAntiguidade">
                <option value="">Todas as antiguidades</option>
                <option value="__sem_antiguidade__">Sem antiguidade</option>
                ${optFuncoes.map(f => `<option value="${f.id}">${esc(f.nome)}</option>`).join('')}
              </select>
            </div>
            <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;padding-bottom:4px">
              <label style="display:inline-flex;align-items:center;gap:6px;cursor:pointer;font-size:13px;user-select:none;margin:0">
                <input type="checkbox" id="fPesInativos"> Mostrar inativos
              </label>
              <button type="button" class="fantasma" id="fPesLimpar" style="font-size:12px;padding:4px 8px">Limpar</button>
            </div>
          </div>

          <!-- Operações em Lote -->
          <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:8px">
            <label style="font-size:13px;cursor:pointer"><input type="checkbox" id="chkTodosP"> todos da página</label>
            <button type="button" class="primario" id="btEditLote" disabled>Editar selecionados (<span id="nSel">0</span>)</button>
            ${souFuncaoPessoal ? '' : '<button type="button" class="perigo" id="btExcLote" disabled>Excluir selecionados (<span id="nSel2">0</span>)</button>'}
            <span style="color:var(--tx2);font-size:12px">com histórico de conferência: exclusão vira inativo (histórico preservado)</span>
          </div>

          <!-- Tabela com Ordenação -->
          <div class="rolagem">
            <table>
              <thead>
                <tr>
                  <th style="width:36px"></th>
                  <th class="num" data-col="id" style="cursor:pointer;user-select:none" title="Ordenar por ID">ID</th>
                  <th data-col="guerra" style="cursor:pointer;user-select:none" title="Ordenar por Guerra">Guerra</th>
                  <th data-col="completo" style="cursor:pointer;user-select:none" title="Ordenar por Completo">Completo</th>
                  <th data-col="setor" style="cursor:pointer;user-select:none" title="Ordenar por Setor">Setor</th>
                  <th data-col="antiguidade" style="cursor:pointer;user-select:none" title="Ordenar por Antiguidade">Antiguidade</th>
                  <th data-col="ativo" style="cursor:pointer;user-select:none" title="Ordenar por Status">Ativo</th>
                  <th data-col="mod" style="cursor:pointer;user-select:none" title="Ordenar por Modificação">ÚLTIMA MODIFICAÇÃO</th>
                  <th style="text-align:right">Ações</th>
                </tr>
              </thead>
              <tbody id="tabP"></tbody>
            </table>
          </div>

          <!-- Container de Paginação no modelo de Relatórios -->
          <div id="pagPessoasCont"></div>
        </div>
      </div>
      <div id="pesFuncoes" class="${abaPes === 'funcoes' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">FUNÇÕES DO GRUPO — Titulares e Auxiliares</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Funções <b>administrativas do grupo</b> (Encarregado de Pessoal e afins — sem postos/graduações): <b>1 titular</b> por função (garantido pelo sistema) e quantos auxiliares forem necessários. Somente contas do SEU grupo.</p>
        ${!podeDesignar ? '<div style="background:rgba(59,130,246,0.08);border:1px solid rgba(59,130,246,0.25);border-radius:6px;padding:8px 12px;margin-bottom:12px;font-size:12.5px;color:var(--tx2)">ℹ️ A designação de funções é realizada pelo gerente do grupo.</div>' : ''}
        <div style="background:rgba(59,130,246,0.08);border:1px solid rgba(59,130,246,0.25);border-radius:6px;padding:8px 12px;margin-bottom:12px;font-size:12.5px;color:var(--tx2)">🔒 Funções de grupo são <b>fixas do sistema</b>: Gerente, Encarregado de Pessoal e Encarregado de Material. Aqui se designa quem ocupa cada cadeira (1 titular + auxiliares).</div>
        <div class="rolagem"><table><thead><tr><th>Função</th><th>Designados</th>${podeDesignar ? '<th>Designar</th>' : ''}</tr></thead>
        <tbody id="tabFun"><tr><td colspan="${podeDesignar ? 3 : 2}"><span class="carregando">…</span></td></tr></tbody></table></div></div>
      </div>
      <div id="pesSetores" class="${abaPes === 'setores' ? '' : 'oculto'}">
        <!-- aba SETORES do módulo Pessoal — CHEFIAS (árvore Reddit: Setor › Titular Chefe › Operadores) -->
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; margin-bottom:12px">
            <div>
              <h3 style="margin:0 0 4px">CHEFIAS — Nomeação e Destituição rápida</h3>
              <p style="color:var(--tx2);font-size:12.5px;margin:0">Cada setor com seu titular chefe e operadores. Clique no setor para expandir a lista hierárquica. Nomear atribui o papel <b>chefe_setor</b>; destituir remove o comando.</p>
            </div>
            <div style="display:flex; gap:8px; align-items:center; flex-wrap:wrap">
              <button type="button" class="fantasma" id="btToggleTodosChefes" style="min-height:36px;font-size:12.5px">Expandir todos</button>
              <button type="button" class="primario" id="btNovoSetorChefes" style="min-height:36px">+ Novo setor</button>
            </div>
          </div>
          <div class="campo" style="margin-bottom:12px">
            <input id="fFiltroChefes" placeholder="Buscar setor, chefe ou operador…" style="width:100%;max-width:380px">
          </div>
          <div id="tabChefes">
            <div class="carregando" style="padding:16px 0">Carregando chefias e setores…</div>
          </div>
        </div>
      </div>`;

    /* --- alternância de sub-abas (ordem 04/10: dropdown estilizado) --- */
    if (typeof criarDropdown === 'function') {
      criarDropdown($('#abasPesDD'), [
        { valor: 'efetivo', rotulo: 'Efetivo' },
        { valor: 'funcoes', rotulo: 'Funções' },
        { valor: 'setores', rotulo: 'Setores' }
      ], {
        valorPadrao: abaPes,
        onChange: (k) => {
          abaPes = k;
          ['efetivo', 'funcoes', 'setores'].forEach(kk => {
            const el = $('#pes' + kk[0].toUpperCase() + kk.slice(1));
            if (el) el.classList.toggle('oculto', kk !== abaPes);
          });
          if (abaPes === 'funcoes') carregarFuncoesMembros();
          if (abaPes === 'setores') { carregarChefes(); }
        }
      });
    }

    /* --- f3: criação de nova função de grupo pelo gerente/admin --- */
    // v39 (ordem Diretor): funções de grupo são HARDCODED — sem criação pela UI
    // (o form "Nova função de grupo" foi removido; as 3 cadeiras vêm da migração).

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
          window.ViewPessoal();
        }
      };
    }
    /* --- f2: edição de nomes da conta (data-editu) fica no Gerenciar (Operadores)
       — o módulo Pessoal usa os handlers data-apresentacao/data-historico. --- */

    // f2: toggles de árvore são da view Grupos (Gerenciar) — aqui não há árvore.

    /* --- controles das tabelas de Funções / Chefes / Setores plugam pesTabelaControles nos loaders --- */

    /* --- onda C2 (05/10): aba FUNÇÕES — designação de membros por função ---
       GET /api/grupo/funcoes/membros (escopo do grupo da sessão) + POST/DELETE.
       1 titular por função é garantia do backend (índice parcial único, 409). */
    async function carregarFuncoesMembros() {
      const tb = $('#tabFun');
      if (!tb) return;
      tb.innerHTML = `<tr><td colspan="${podeDesignar ? 3 : 2}"><span class="carregando">…</span></td></tr>`;
      try {
        const linhas = await api('/api/grupo/funcoes/membros');
        const porFuncao = {};
        (linhas || []).forEach(l => {
          (porFuncao[l.funcao_id] = porFuncao[l.funcao_id] || { funcao_id: l.funcao_id, funcao_nome: l.funcao_nome, chave: l.chave, membros: [] }).membros.push(l);
        });
        const ids = Object.keys(porFuncao).sort((a, b) => String(porFuncao[a].funcao_nome).localeCompare(String(porFuncao[b].funcao_nome)));
        if (!ids.length) { tb.innerHTML = `<tr><td colspan="${podeDesignar ? 3 : 2}"><span class="vazio">nenhuma função no catálogo</span></td></tr>`; return; }
        const opContas = contas.filter(c => c.grupo_id === eu.grupo_id && c.ativo)
          .map(c => `<option value="${c.id}">${esc(c.nome_guerra || c.login)} (${esc(c.login)})</option>`).join('');
        tb.innerHTML = ids.map(fid => {
          const f = porFuncao[fid];
          // Onda 10/10: espelha a trava do servidor (hFuncaoMembrosSet/Del) —
          // quem não é gerente só mexe na cadeira enc_material; o servidor
          // devolve `chave` (fix: sem ele a trava visual caía no nome, frágil).
          const desab = !ehGerente && f.chave !== 'enc_material';
          const disAttr = desab ? ' disabled title="Esta cadeira só é alterada pelo gerente"' : '';
          const membros = f.membros.filter(m => m.membro_id > 0);
          const titular = membros.find(m => m.titularidade === 'titular');
          const auxiliares = membros.filter(m => m.titularidade === 'auxiliar');
          const remBtTit = podeDesignar && titular ? ` <button class="acao-linha" data-remfun="${titular.membro_id}"${disAttr}>remover</button>` : '';
          const linhaTit = titular
            ? `<b>👑 ${esc(titular.nome_guerra || titular.login)}</b>${remBtTit}`
            : '<span style="color:var(--tx3)">sem titular</span>';
          const linhasAux = auxiliares.map(m => {
            const remBtAux = podeDesignar ? ` <button class="acao-linha" data-remfun="${m.membro_id}"${disAttr}>remover</button>` : '';
            return `<div style="margin-top:4px">${esc(m.nome_guerra || m.login)}${remBtAux}</div>`;
          }).join('');
          const colDesignar = podeDesignar ? `
            <td>
              <div class="form-linha" style="gap:6px;align-items:center;flex-wrap:wrap">
                <select data-seluser="${f.funcao_id}" style="min-width:170px"${disAttr}><option value="">— conta —</option>${opContas}</select>
                <select data-seltit="${f.funcao_id}"${disAttr}><option value="titular">titular</option><option value="auxiliar">auxiliar</option></select>
                <button class="primario" data-addfun="${f.funcao_id}" style="font-size:12px;padding:4px 12px"${disAttr}>Designar</button>
              </div>
            </td>` : '';
          return `<tr>
            <td><b>${esc(f.funcao_nome)}</b></td>
            <td>${linhaTit}${auxiliares.length ? '<div style="margin-top:6px;border-top:1px dashed var(--borda);padding-top:4px">' + linhasAux + '</div>' : ''}</td>
            ${colDesignar}</tr>`;
        }).join('');
        if (podeDesignar) {
          tb.querySelectorAll('[data-addfun]').forEach(bt => {
            bt.onclick = async () => {
              if (bt.disabled) return;
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
              if (bt.disabled) return;
              const r = await processar(() => api('/api/grupo/funcoes/membros/' + bt.dataset.remfun, { method: 'DELETE' }), 'Removendo designação…');
              if (r.ok) carregarFuncoesMembros();
            };
          });
        }
        // ordem 06/10 (item 10): ordenar POR nos cabeçalhos (Função / Designados / Designar)
        const tabFunTbl = document.querySelector('#pesFuncoes table');
        if (tabFunTbl) pesOrdenar('pes-funcoes', tabFunTbl, tb, [{ tipo: 'txt' }, { tipo: 'txt' }]);
      } catch (e) {
        tb.innerHTML = `<tr><td colspan="${podeDesignar ? 3 : 2}"><span class="vazio">Falha ao carregar funções.</span></td></tr>`;
      }
    }
    if (abaPes === 'funcoes') carregarFuncoesMembros();
    if (abaPes === 'efetivo') atualizarSelectsCatalogos();
    if (abaPes === 'setores') { carregarChefes(); }

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
          window.ViewPessoal();
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
          window.ViewPessoal();
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
          window.ViewPessoal();
        }
      };
    }

    /* --- onda Escalas: aba CHEFIAS — lista de setores com toggle hierárquico estilo Reddit --- */
    async function carregarChefes() {
      const tb = $('#tabChefes');
      if (!tb) return;
      tb.innerHTML = '<div class="carregando" style="padding:16px 0">Carregando chefias e setores…</div>';
      try {
        const [setoresG, contasR, gruposR] = await Promise.all([
          api('/api/catalogo/setores'),
          api('/api/usuarios'),
          api('/api/grupos').catch(() => [])
        ]);
        const listaS = (setoresG || []).filter(s => s.ativo !== false);
        if (!listaS.length) {
          tb.innerHTML = '<div class="vazio" style="padding:24px 0">Nenhum setor cadastrado no catálogo. Use o botão "+ Novo setor" acima.</div>';
          return;
        }

        const doGrupo = (contasR || []).filter(c => c.grupo_id === eu.grupo_id && c.ativo);
        const opContas = doGrupo.map(c => `<option value="${c.id}">${esc(c.nome_guerra || c.login)} (${esc(c.login)})</option>`).join('');

        // Mapeamento de chefes por setor
        const chefeDeSetor = {};
        (setoresG || []).forEach(s => {
          if (s.chefe_usuario_id) chefeDeSetor[s.id] = { id: s.chefe_usuario_id, nome_guerra: s.chefe_nome, login: s.chefe_nome };
        });

        tb.className = 'chefias-reddit-container';
        tb.innerHTML = listaS.map(s => {
          const ch = chefeDeSetor[s.id];
          const chConta = ch ? (contasR || []).find(c => c.id === ch.id) : null;
          const chNome = (chConta && (chConta.nome_guerra || chConta.login)) || (ch && ch.nome_guerra) || s.chefe_nome || '';
          const chLogin = (chConta && chConta.login) || (ch && ch.login) || s.chefe_nome || '';
          const chCompleto = (chConta && chConta.nome_completo) || '';
          const temChefe = !!(ch || s.chefe_usuario_id || chConta);
          const opIdChefe = chConta ? chConta.id : (ch ? ch.id : s.chefe_usuario_id);

          // Operadores vinculados a este setor (exclui o titular chefe)
          const opsDoSetor = (contasR || []).filter(c => {
            if (c.ativo === false) return false;
            if (opIdChefe && c.id === opIdChefe) return false;
            return c.setor_id === s.id;
          });

          const gNome = ((gruposR || grupos || []).find(g => g.id === s.grupo_id) || {}).nome || '';
          const isExp = expandedChefiasSetores.has(s.id);

          return `
          <div class="chefia-setor-card ${isExp ? 'expandido' : ''}" data-setor-id="${s.id}">
            <div class="chefia-setor-header" tabindex="0" role="button" aria-expanded="${isExp ? 'true' : 'false'}" aria-label="Setor ${esc(s.nome)}">
              <div class="chefia-setor-info-left">
                <span class="chefia-setor-toggle-btn" title="Alternar expansão">
                  <span class="setor-toggle-arrow">▶</span>
                </span>
                <span class="chefia-setor-nome">${esc(s.nome)}</span>
                ${s.sigla ? `<code class="chefia-setor-sigla">${esc(s.sigla)}</code>` : ''}
                ${gNome ? `<span class="chefia-setor-unidade">(${esc(gNome)})</span>` : ''}
              </div>
              <div class="chefia-setor-badges">
                ${temChefe
                  ? `<span class="alerta-ok" style="font-size:11.5px">👑 ${esc(chNome)}</span>`
                  : `<span style="color:var(--tx3);font-size:11.5px">sem chefe</span>`}
                <span class="pill" style="background:rgba(59,130,246,0.12);color:#93c5fd;border-color:rgba(59,130,246,0.3);font-size:11px">
                  ${opsDoSetor.length} op${opsDoSetor.length === 1 ? '' : 's'}
                </span>
                <button type="button" class="acao-linha" data-edsetor="${s.id}" data-nome="${esc(s.nome)}" data-sigla="${esc(s.sigla || '')}" style="font-size:11px;padding:2px 8px" title="Editar informações do setor">editar</button>
              </div>
            </div>

            <div class="chefia-reddit-thread ${isExp ? '' : 'oculto'}">
              <div class="reddit-threadline" title="Clique para recolher"></div>
              <div class="reddit-thread-conteudo">

                <!-- 1º item da lista: Titular Chefe -->
                <div class="reddit-comment-node chefe-node">
                  <div class="reddit-node-header">
                    <div class="reddit-node-meta">
                      <span class="papel-chip chefe_setor">👑 TITULAR CHEFE</span>
                      ${temChefe ? `
                        <span class="reddit-node-nome"><b>${esc(chNome)}</b></span>
                        <span class="reddit-node-login">(@${esc(chLogin)})</span>
                        ${chCompleto ? `<span style="color:var(--tx2);font-size:12px">· ${esc(chCompleto)}</span>` : ''}
                      ` : `
                        <span style="color:var(--tx3);font-size:12.5px;font-style:italic">Sem titular chefe definido</span>
                      `}
                    </div>
                    ${temChefe ? `
                      <button type="button" class="acao-linha perigo" data-destituir="${opIdChefe}" data-setorid="${s.id}" data-nome="${esc(chNome)}" data-setor="${esc(s.nome)}" style="font-size:11.5px">
                        destituir
                      </button>
                    ` : ''}
                  </div>
                  <div class="reddit-nomear-box">
                    <span style="font-size:12px;color:var(--tx2)">${temChefe ? 'Substituir chefe:' : 'Nomear chefe:'}</span>
                    <select data-selchefe="${s.id}" style="min-width:170px;font-size:12px"><option value="">— escolher conta do grupo —</option>${opContas}</select>
                    <button type="button" class="primario" data-nomear="${s.id}" data-setor="${esc(s.nome)}" style="font-size:12px;padding:4px 12px">Nomear</button>
                  </div>
                </div>

                <!-- Abaixo: Operadores -->
                <div class="reddit-operadores-secao">
                  <div class="reddit-operadores-titulo">Operadores (${opsDoSetor.length})</div>
                  ${opsDoSetor.length > 0 ? opsDoSetor.map(op => `
                    <div class="reddit-comment-node operador-node">
                      <div class="reddit-node-header">
                        <div class="reddit-node-meta">
                          <span class="papel-chip operador">OPERADOR</span>
                          <span class="reddit-node-nome"><b>${esc(op.nome_guerra || op.login)}</b></span>
                          <span class="reddit-node-login">(@${esc(op.login)})</span>
                          ${op.nome_completo ? `<span style="color:var(--tx2);font-size:12px">· ${esc(op.nome_completo)}</span>` : ''}
                        </div>
                        ${op.ativo
                          ? '<span class="pill pill-presente" style="font-size:10px;padding:1px 6px">Ativo</span>'
                          : '<span class="pill pill-falta" style="font-size:10px;padding:1px 6px">Inativo</span>'}
                      </div>
                    </div>
                  `).join('') : '<div class="reddit-comment-node vazio">Nenhum operador vinculado a este setor.</div>'}
                </div>

              </div>
            </div>
          </div>`;
        }).join('');

        // Ligar toggles expansíveis ao clicar no cabeçalho ou na threadline
        tb.querySelectorAll('.chefia-setor-card').forEach(card => {
          const sID = +card.dataset.setorId;
          const toggleHandler = (e) => {
            if (e.target.closest('button, select, input, a, option')) return;
            const exp = card.classList.toggle('expandido');
            const hdr = card.querySelector('.chefia-setor-header');
            if (hdr) hdr.setAttribute('aria-expanded', exp ? 'true' : 'false');
            const thread = card.querySelector('.chefia-reddit-thread');
            if (thread) thread.classList.toggle('oculto', !exp);
            if (sID) {
              if (exp) expandedChefiasSetores.add(sID);
              else expandedChefiasSetores.delete(sID);
            }
          };

          const hdr = card.querySelector('.chefia-setor-header');
          if (hdr) {
            hdr.onclick = toggleHandler;
            hdr.onkeydown = (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                if (e.target.closest('button, select, input, a, option')) return;
                e.preventDefault();
                toggleHandler(e);
              }
            };
          }

          const tline = card.querySelector('.reddit-threadline');
          if (tline) tline.onclick = toggleHandler;
        });

        // Ligar botões Nomear
        tb.querySelectorAll('[data-nomear]').forEach(bt => {
          bt.onclick = async (e) => {
            e.stopPropagation();
            const sID = +bt.dataset.nomear;
            const sel = tb.querySelector(`[data-selchefe="${sID}"]`);
            if (!sel || !sel.value) { toast('Escolha a conta', 'erro'); return; }
            const r = await processar(() => api('/api/grupos/' + eu.grupo_id + '/nomear_chefe', {
              method: 'POST',
              body: JSON.stringify({ usuario_id: +sel.value, setor_id: sID })
            }), 'Nomeando chefe…');
            if (r.ok) {
              expandedChefiasSetores.add(sID);
              carregarChefes();
            }
          };
        });

        // Ligar botões Destituir
        tb.querySelectorAll('[data-destituir]').forEach(bt => {
          bt.onclick = async (e) => {
            e.stopPropagation();
            if (!(await confirmar(`Destituir ${bt.dataset.nome} como chefe de ${bt.dataset.setor}?`))) return;
            const sID = +bt.dataset.setorid;
            const r = await processar(() => api('/api/grupos/' + eu.grupo_id + '/destituir_chefe', {
              method: 'POST',
              body: JSON.stringify({ usuario_id: +bt.dataset.destituir, setor_id: sID })
            }), 'Destituindo chefe…');
            if (r.ok) {
              expandedChefiasSetores.add(sID);
              carregarChefes();
            }
          };
        });

        // Ligar botões Editar setor
        tb.querySelectorAll('[data-edsetor]').forEach(bt => {
          bt.onclick = (e) => {
            e.stopPropagation();
            modalEditarSetor(+bt.dataset.edsetor, bt.dataset.nome, bt.dataset.sigla || '');
          };
        });

        // Ligar filtro de busca
        const fFiltro = $('#fFiltroChefes');
        if (fFiltro) {
          fFiltro.oninput = () => {
            const q = fFiltro.value.trim().toLowerCase();
            tb.querySelectorAll('.chefia-setor-card').forEach(card => {
              const txt = card.textContent.toLowerCase();
              card.style.display = (!q || txt.includes(q)) ? '' : 'none';
            });
          };
        }

        // Ligar Expandir todos / Recolher todos
        const btToggleAll = $('#btToggleTodosChefes');
        if (btToggleAll) {
          let todosExp = false;
          btToggleAll.onclick = () => {
            todosExp = !todosExp;
            btToggleAll.textContent = todosExp ? 'Recolher todos' : 'Expandir todos';
            tb.querySelectorAll('.chefia-setor-card').forEach(card => {
              const sID = +card.dataset.setorId;
              const thread = card.querySelector('.chefia-reddit-thread');
              const hdr = card.querySelector('.chefia-setor-header');
              if (todosExp) {
                card.classList.add('expandido');
                if (hdr) hdr.setAttribute('aria-expanded', 'true');
                if (thread) thread.classList.remove('oculto');
                if (sID) expandedChefiasSetores.add(sID);
              } else {
                card.classList.remove('expandido');
                if (hdr) hdr.setAttribute('aria-expanded', 'false');
                if (thread) thread.classList.add('oculto');
                if (sID) expandedChefiasSetores.delete(sID);
              }
            });
          };
        }
      } catch (e) {
        tb.innerHTML = '<div class="vazio" style="padding:24px 0">Falha ao carregar chefias e setores.</div>';
      }
    }

    /* --- Banco de Pessoal: estado de filtros, ordenação e paginação --- */
    const listaPessoasTotal = (pessoas && pessoas.pessoas) || [];
    let filtroSetor = '';
    let filtroAntiguidade = '';
    let filtroMostrarInativos = false;
    let filtroBusca = '';
    let pgAtual = 1;
    const tamPag = 10;
    let colOrdem = 'id';
    let ordemAsc = true;
    const idsSelecionados = new Set();

    function normalizarTexto(txt) {
      return String(txt || '')
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '')
        .toLowerCase()
        .trim();
    }

    function atualizarDatalistMilitares() {
      const dl = $('#listaPesMilitares');
      if (!dl) return;
      const jaAdicionados = new Set();
      const opcoes = [];
      listaPessoasTotal.forEach(p => {
        if (!filtroMostrarInativos && p.status !== 'ativo') return;
        const nc = (p.nome_completo || '').trim();
        const ng = (p.nome_guerra || '').trim();
        if (!nc || jaAdicionados.has(nc)) return;
        jaAdicionados.add(nc);
        opcoes.push(`<option value="${esc(nc)}">${esc(ng ? ng + ' — ' : '')}${esc(nc)}</option>`);
      });
      dl.innerHTML = opcoes.join('');
    }

    function filtrarEOrdenarPessoas() {
      const buscaNorm = normalizarTexto(filtroBusca);
      const filtradas = listaPessoasTotal.filter(p => {
        if (!filtroMostrarInativos && p.status !== 'ativo') return false;
        if (filtroSetor !== '') {
          if (filtroSetor === '__sem_setor__') {
            if (p.setor_id != null && p.setor_id !== 0) return false;
          } else {
            if (String(p.setor_id) !== String(filtroSetor)) return false;
          }
        }
        if (filtroAntiguidade !== '') {
          if (filtroAntiguidade === '__sem_antiguidade__') {
            if (p.funcao_id != null && p.funcao_id !== 0) return false;
          } else {
            if (String(p.funcao_id) !== String(filtroAntiguidade)) return false;
          }
        }
        if (buscaNorm) {
          const gNorm = normalizarTexto(p.nome_guerra);
          const cNorm = normalizarTexto(p.nome_completo);
          const idStr = String(p.id);
          if (!gNorm.includes(buscaNorm) && !cNorm.includes(buscaNorm) && !idStr.includes(buscaNorm)) {
            return false;
          }
        }
        return true;
      });

      filtradas.sort((a, b) => {
        let va, vb;
        if (colOrdem === 'id') {
          va = Number(a.id) || 0;
          vb = Number(b.id) || 0;
          return ordemAsc ? (va - vb) : (vb - va);
        }
        if (colOrdem === 'guerra') {
          va = a.nome_guerra || ''; vb = b.nome_guerra || '';
        } else if (colOrdem === 'completo') {
          va = a.nome_completo || ''; vb = b.nome_completo || '';
        } else if (colOrdem === 'setor') {
          va = a.setor || ''; vb = b.setor || '';
        } else if (colOrdem === 'antiguidade') {
          va = a.funcao || ''; vb = b.funcao || '';
        } else if (colOrdem === 'ativo') {
          va = a.status || ''; vb = b.status || '';
        } else if (colOrdem === 'mod') {
          va = a.ultima_mod_em || ''; vb = b.ultima_mod_em || '';
        } else {
          va = a[colOrdem] || ''; vb = b[colOrdem] || '';
        }
        const cmp = String(va).localeCompare(String(vb), 'pt', { sensitivity: 'base' }) || String(va).localeCompare(String(vb));
        return ordemAsc ? cmp : -cmp;
      });

      return filtradas;
    }

    function renderizarTabelaPessoas() {
      const filtradas = filtrarEOrdenarPessoas();
      const fnPag = typeof paginarArray === 'function' ? paginarArray : window.paginarArray;
      const fnRenderPag = typeof renderPaginadorHTML === 'function' ? renderPaginadorHTML : window.renderPaginadorHTML;

      const pag = fnPag ? fnPag(filtradas, pgAtual, tamPag) : {
        pagina: 1, totalPaginas: 1, totalItens: filtradas.length,
        inicio: 1, fim: filtradas.length, dados: filtradas
      };
      pgAtual = pag.pagina;

      const contEl = $('#contagemPes');
      if (contEl) {
        contEl.textContent = (filtradas.length !== listaPessoasTotal.length)
          ? `${filtradas.length} de ${listaPessoasTotal.length}`
          : `${listaPessoasTotal.length}`;
      }

      const tbody = $('#tabP');
      if (!tbody) return;

      if (pag.totalItens === 0) {
        tbody.innerHTML = '<tr><td colspan="9"><span class="vazio">nenhum militar encontrado</span></td></tr>';
      } else {
        tbody.innerHTML = pag.dados.map(p => {
          const ap = mapaApresentacao[p.id];
          const pillAp = ap && ap.estado ? ` ${gpxRenderPillApresentacao(ap.estado)}` : '';
          const celMod = p.ultima_mod_em
            ? `<div class="gpx-mod-cel"><div>${fmtData(p.ultima_mod_em)} ${fmtHora(p.ultima_mod_em)}</div>${p.ultima_mod_por ? `<small class="gpx-mod-por">por ${esc(p.ultima_mod_por)}</small>` : ''}</div>`
            : '—';
          const isSel = idsSelecionados.has(p.id);
          return `<tr data-p='${esc(JSON.stringify(p))}'>
            <td><input type="checkbox" class="chkP" data-id="${p.id}" ${isSel ? 'checked' : ''}></td>
            <td class="num">#${p.id}</td>
            <td><b>${esc(p.nome_guerra)}</b>${pillAp}</td>
            <td>${esc(p.nome_completo)}</td>
            <td>${esc(p.setor || 'SEM SETOR')}</td>
            <td>${esc(p.funcao || 'INDEFINIDO')}</td>
            <td>${p.status === 'ativo' ? '<span class="alerta-ok">● ATIVO</span>' : '<span style="color:var(--tx3)">● INATIVO</span>'}</td>
            <td>${celMod}</td>
            <td><div class="gpx-acoes">
              ${podeApresentacao ? `<button type="button" class="acao-linha" data-apresentacao="${p.id}" data-nome="${esc(p.nome_guerra)}">APRESENTAÇÃO</button>` : ''}
              ${podeApresentacao ? `<button type="button" class="acao-linha" data-historico="${p.id}" data-nome="${esc(p.nome_guerra)}">HISTÓRICO</button>` : ''}
              <button type="button" class="acao-linha" data-edit="${p.id}">editar</button>
              <button type="button" class="acao-linha" data-fichap="${p.id}" title="Imprimir Dossiê / Ficha Cadastral">📄 ficha</button>
              ${souFuncaoPessoal ? '' : `<button type="button" class="acao-linha" data-excP="${p.id}" data-nome="${esc(p.nome_guerra)}">excluir</button>`}
            </div></td>
          </tr>`;
        }).join('');
      }

      const pagContainer = $('#pagPessoasCont');
      if (pagContainer) {
        pagContainer.innerHTML = fnRenderPag ? fnRenderPag(pag, 'pagBancoPessoal') : '';
        pagContainer.querySelectorAll('.paginacao-btn[data-pg]').forEach(b => {
          b.onclick = () => {
            pgAtual = +b.dataset.pg;
            renderizarTabelaPessoas();
          };
        });
      }

      vincularEventosLinhasPessoas();
      atualizarSelecaoLoteUI();
    }

    function vincularEventosLinhasPessoas() {
      const tbody = $('#tabP');
      if (!tbody) return;

      tbody.querySelectorAll('.chkP').forEach(ch => {
        ch.onchange = () => {
          const id = +ch.dataset.id;
          if (ch.checked) idsSelecionados.add(id);
          else idsSelecionados.delete(id);
          atualizarSelecaoLoteUI();
        };
      });

      tbody.querySelectorAll('[data-edit]').forEach(bt => {
        bt.onclick = ev => {
          ev.stopPropagation();
          const tr = bt.closest('tr');
          if (!tr || !tr.dataset.p) return;
          const p = JSON.parse(tr.dataset.p);
          abrirModalMilitar(p);
        };
      });

      tbody.querySelectorAll('[data-fichap]').forEach(bt => {
        bt.onclick = ev => {
          ev.stopPropagation();
          window.open('/api/pessoas/' + bt.dataset.fichap + '/pdf', '_blank');
        };
      });

      tbody.querySelectorAll('[data-excP]').forEach(bt => {
        bt.onclick = () => excPessoa(bt.dataset.excP, bt.dataset.nome);
      });

      tbody.querySelectorAll('[data-apresentacao]').forEach(bt => {
        bt.onclick = ev => abrirModalApresentacao(ev, bt);
      });

      tbody.querySelectorAll('[data-historico]').forEach(bt => {
        bt.onclick = ev => abrirModalHistorico(ev, bt);
      });
    }

    function atualizarSelecaoLoteUI() {
      const totalSel = idsSelecionados.size;
      const nSel = $('#nSel');
      if (nSel) nSel.textContent = totalSel;
      const nSel2 = $('#nSel2');
      if (nSel2) nSel2.textContent = totalSel;
      const btEdit = $('#btEditLote');
      if (btEdit) btEdit.disabled = totalSel === 0;
      const btExc = $('#btExcLote');
      if (btExc) btExc.disabled = totalSel === 0;

      const chkTodos = $('#chkTodosP');
      if (chkTodos) {
        const chksPag = [...document.querySelectorAll('#tabP .chkP')];
        if (chksPag.length > 0 && chksPag.every(c => c.checked)) {
          chkTodos.checked = true;
          chkTodos.indeterminate = false;
        } else if (chksPag.some(c => c.checked)) {
          chkTodos.checked = false;
          chkTodos.indeterminate = true;
        } else {
          chkTodos.checked = false;
          chkTodos.indeterminate = false;
        }
      }
    }

    function configurarCabecalhosOrdenacao() {
      const thead = document.querySelector('#pesEfetivo table thead');
      if (!thead) return;
      thead.querySelectorAll('th[data-col]').forEach(th => {
        th.onclick = () => {
          const col = th.dataset.col;
          if (colOrdem === col) {
            ordemAsc = !ordemAsc;
          } else {
            colOrdem = col;
            ordemAsc = true;
          }
          atualizarClassesCabecalhos(thead);
          renderizarTabelaPessoas();
        };
      });
      atualizarClassesCabecalhos(thead);
    }

    function atualizarClassesCabecalhos(thead) {
      thead.querySelectorAll('th[data-col]').forEach(th => {
        th.classList.remove('sorted-asc', 'sorted-desc');
        if (th.dataset.col === colOrdem) {
          th.classList.add(ordemAsc ? 'sorted-asc' : 'sorted-desc');
        }
      });
    }

    /* --- modal Cadastrar / Editar Militar --- */
    function abrirModalMilitar(p = null) {
      const editando = !!(p && p.id);
      const titulo = editando ? `Editar militar — ${esc(p.nome_guerra || '')}` : 'Cadastrar militar';
      const valNg = editando ? (p.nome_guerra || '') : '';
      const valNc = editando ? (p.nome_completo || '') : '';
      const valSetor = editando ? (p.setor_id || '') : '';
      const valFuncao = editando ? (p.funcao_id || '') : '';
      const valStatus = editando ? (p.status || 'ativo') : 'ativo';

      const div = modal(`
        <div class="modal-inner" style="max-width:520px">
          <h3 style="margin-top:0">${titulo}</h3>
          <div class="form-linha" style="margin-bottom:12px">
            <div class="campo" style="flex:1">
              <label>Nome de guerra *</label>
              <input id="mNg" value="${esc(valNg)}" placeholder="Ex.: SILVA">
            </div>
            <div class="campo" style="flex:1">
              <label>Nome completo *</label>
              <input id="mNc" value="${esc(valNc)}" placeholder="Ex.: José da Silva">
            </div>
          </div>
          <div class="form-linha" style="margin-bottom:12px">
            <div class="campo" style="flex:1">
              <label>Setor</label>
              <select id="mSetor">
                <option value="">— Selecionar setor —</option>
                ${optSetores.map(x => `<option value="${x.id}" ${String(x.id) === String(valSetor) ? 'selected' : ''}>${esc(x.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="flex:1">
              <label>Antiguidade</label>
              <select id="mFuncao">
                <option value="">— Selecionar antiguidade —</option>
                ${optFuncoes.map(x => `<option value="${x.id}" ${String(x.id) === String(valFuncao) ? 'selected' : ''}>${esc(x.nome)}</option>`).join('')}
              </select>
            </div>
          </div>
          <div class="campo" style="margin-bottom:18px">
            <label>Status</label>
            <select id="mStatus">
              <option value="ativo" ${valStatus === 'ativo' ? 'selected' : ''}>Ativo</option>
              <option value="inativo" ${valStatus === 'inativo' ? 'selected' : ''}>Inativo</option>
            </select>
          </div>
          <div class="modal-acoes" style="display:flex;justify-content:flex-end;gap:8px">
            <button type="button" class="fantasma" id="mX">Cancelar</button>
            <button type="button" class="primario" id="mSalvar">${editando ? 'Salvar alterações' : 'Cadastrar militar'}</button>
          </div>
        </div>
      `);
      if (!div) return;
      const fechar = () => div.fechar ? div.fechar() : div.remove();
      div.querySelector('#mX').onclick = fechar;
      const fNg = div.querySelector('#mNg');
      if (fNg) fNg.focus();

      div.querySelector('#mSalvar').onclick = async () => {
        const ng = div.querySelector('#mNg').value.trim();
        const nc = div.querySelector('#mNc').value.trim();
        const sid = +div.querySelector('#mSetor').value || null;
        const fid = +div.querySelector('#mFuncao').value || null;
        const st = div.querySelector('#mStatus').value || 'ativo';

        if (!ng || !nc) {
          toast('Nome de guerra e nome completo são obrigatórios', 'erro');
          return;
        }

        const corpo = {
          nome_guerra: ng,
          nome_completo: nc,
          setor_id: sid,
          funcao_id: fid,
          status: st
        };

        const r = await processar(
          () => editando
            ? api('/api/pessoas/' + p.id, { method: 'PATCH', body: JSON.stringify(corpo) })
            : api('/api/pessoas', { method: 'POST', body: JSON.stringify(corpo) }),
          editando ? 'Salvando alterações…' : 'Cadastrando militar…'
        );

        if (r && r.ok) {
          toast(editando ? 'Militar atualizado com sucesso' : 'Militar cadastrado com sucesso');
          fechar();
          window.ViewPessoal();
        }
      };
    }

    /* --- modal Adição em lote (CSV) --- */
    function abrirModalLoteCsv() {
      const div = modal(`
        <div class="modal-inner" style="max-width:540px">
          <h3 style="margin-top:0">Adição em lote — Importar militares</h3>
          <p style="color:var(--tx2);font-size:12px;margin:4px 0 10px">
            Formato (1 por linha, separado por ponto-e-vírgula):<br>
            <code>nome de guerra ; nome completo ; setor ; posto/graduação</code><br>
            Setor e posto/graduação são opcionais e devem já existir no catálogo.
          </p>
          <div class="campo" style="margin-bottom:14px">
            <textarea id="csv" rows="6" placeholder="SILVA;José da Silva;Comando;Motorista&#10;SOUSA;Maria de Sousa;Serviços&#10;PERES;Bruno Peres"></textarea>
          </div>
          <div class="modal-acoes" style="display:flex;justify-content:flex-end;gap:8px">
            <button type="button" class="fantasma" id="csvX">Cancelar</button>
            <button type="button" class="primario" id="csvGo">Importar linhas</button>
          </div>
        </div>
      `);
      if (!div) return;
      const fechar = () => div.fechar ? div.fechar() : div.remove();
      div.querySelector('#csvX').onclick = fechar;
      div.querySelector('#csvGo').onclick = async () => {
        const linhas = (div.querySelector('#csv')?.value || '').split('\n').map(l => l.trim()).filter(Boolean);
        if (!linhas.length) { toast('Cole ao menos uma linha', 'erro'); return; }
        let ok = 0, falha = 0;
        await processar(async () => {
          for (const l of linhas) {
            const [ng, nc, st, fn] = l.split(';').map(x => (x || '').trim());
            let sid = (optSetores.find(s => s.nome.toLowerCase() === (st || '').toLowerCase()) || {}).id || null;
            if (st && !sid) {
              try {
                const resSt = await api('/api/catalogo/setores', { method: 'POST', body: JSON.stringify({ nome: st }) });
                if (resSt && resSt.id) { sid = resSt.id; optSetores.push({ id: sid, nome: st, ativo: 1 }); }
              } catch (e) {}
            }
            let fid = (optFuncoes.find(s => s.nome.toLowerCase() === (fn || '').toLowerCase()) || {}).id || null;
            if (fn && !fid) {
              try {
                const resFn = await api('/api/catalogo/funcoes', { method: 'POST', body: JSON.stringify({ nome: fn }) });
                if (resFn && resFn.id) { fid = resFn.id; optFuncoes.push({ id: fid, nome: fn, ativo: 1 }); }
              } catch (e) {}
            }
            try {
              await api('/api/pessoas', { method: 'POST', body: JSON.stringify({ nome_guerra: ng, nome_completo: nc, setor_id: sid, funcao_id: fid, status: 'ativo' }) });
              ok++;
            } catch (e) { falha++; }
          }
        }, `Importando ${linhas.length} linha(s)...`);
        toast(`${ok} importado(s)${falha ? ' · ' + falha + ' linha(s) com falha' : ''}`, falha && !ok ? 'erro' : 'ok');
        if (ok) {
          fechar();
          window.ViewPessoal();
        }
      };
    }

    /* --- exclusão de militar (individual; com histórico → desativa) --- */
    const excPessoa = async (id, nome) => {
      if (!(await confirmar(`Excluir "${nome}" do banco de pessoal? (com histórico de conferência virará inativo)`))) return;
      const r = await processar(() => api(`/api/pessoas/${id}`, { method: 'DELETE' }), `Excluindo ${nome}…`);
      if (r.ok) {
        toast(r.resultado.desativado ? 'Desativado (histórico preservado)' : 'Excluído');
        window.ViewPessoal();
      }
    };

    /* --- exclusão em lote --- */
    const btExcLoteEl = $('#btExcLote');
    if (btExcLoteEl) {
      btExcLoteEl.onclick = async () => {
        const ids = [...idsSelecionados];
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
        if (ok || des) window.ViewPessoal();
      };
    }

    /* --- edição em lote: modal único aplica setor/função/status ('(manter)' = não altera) --- */
    const btEditLoteEl = $('#btEditLote');
    if (btEditLoteEl) {
      btEditLoteEl.onclick = () => {
        const sel = [...idsSelecionados];
        if (!sel.length) { toast('Selecione ao menos um militar', 'erro'); return; }
        const div = modal(`<div class="modal-inner"><h3>Editar em lote — ${sel.length} militar(es)</h3>
          <p style="color:var(--tx2);font-size:12px;margin:4px 0">Campos em <b>(manter)</b> não são alterados. Aplica a todos os selecionados.</p>
          <div class="form-linha">
            <div class="campo"><label>Setor</label><select id="lSetor"><option value="">(manter)</option>${optSetores.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
            <div class="campo"><label>Antiguidade</label><select id="lFuncao"><option value="">(manter)</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
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
            window.ViewPessoal();
          }
        };
      };
    }

    /* --- eventos dos filtros do Banco de Pessoal --- */
    const fBusca = $('#fPesBusca');
    if (fBusca) {
      fBusca.addEventListener('input', () => {
        filtroBusca = fBusca.value;
        pgAtual = 1;
        renderizarTabelaPessoas();
      });
    }

    let ddSetor = null;
    const fSetor = $('#fPesSetor');
    if (fSetor) {
      fSetor.addEventListener('change', () => {
        filtroSetor = fSetor.value;
        pgAtual = 1;
        renderizarTabelaPessoas();
      });
      if (typeof converterSelectEmDropdown === 'function') {
        ddSetor = converterSelectEmDropdown(fSetor);
      }
    }

    let ddAntiguidade = null;
    const fAntiguidade = $('#fPesAntiguidade');
    if (fAntiguidade) {
      fAntiguidade.addEventListener('change', () => {
        filtroAntiguidade = fAntiguidade.value;
        pgAtual = 1;
        renderizarTabelaPessoas();
      });
      if (typeof converterSelectEmDropdown === 'function') {
        ddAntiguidade = converterSelectEmDropdown(fAntiguidade);
      }
    }

    const fInativos = $('#fPesInativos');
    if (fInativos) {
      fInativos.addEventListener('change', () => {
        filtroMostrarInativos = fInativos.checked;
        pgAtual = 1;
        atualizarDatalistMilitares();
        renderizarTabelaPessoas();
      });
    }

    const fLimpar = $('#fPesLimpar');
    if (fLimpar) {
      fLimpar.addEventListener('click', () => {
        filtroBusca = '';
        filtroSetor = '';
        filtroAntiguidade = '';
        filtroMostrarInativos = false;
        if (fBusca) fBusca.value = '';
        if (fSetor) fSetor.value = '';
        if (fAntiguidade) fAntiguidade.value = '';
        if (ddSetor) ddSetor.setValor('');
        if (ddAntiguidade) ddAntiguidade.setValor('');
        if (fInativos) fInativos.checked = false;
        pgAtual = 1;
        atualizarDatalistMilitares();
        renderizarTabelaPessoas();
      });
    }

    const btAbrirCad = $('#btAbrirCadastrar');
    if (btAbrirCad) btAbrirCad.onclick = () => abrirModalMilitar(null);

    const btAbrirCsv = $('#btAbrirLoteCsv');
    if (btAbrirCsv) btAbrirCsv.onclick = () => abrirModalLoteCsv();

    const chkTodosP = $('#chkTodosP');
    if (chkTodosP) {
      chkTodosP.onchange = () => {
        const marcar = chkTodosP.checked;
        document.querySelectorAll('#tabP .chkP').forEach(ch => {
          ch.checked = marcar;
          const id = +ch.dataset.id;
          if (marcar) idsSelecionados.add(id);
          else idsSelecionados.delete(id);
        });
        atualizarSelecaoLoteUI();
      };
    }

    /* --- onda GPX (item 2): edição de apresentação --- */
    function abrirModalApresentacao(ev, bt) {
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
              window.ViewPessoal();
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
    }

    /* --- onda GPX (item 3): trilha de modificações por pessoa --- */
    function abrirModalHistorico(ev, bt) {
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

      (async () => {
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
      })();
    }

    // Inicialização da tabela do Banco de Pessoal
    atualizarDatalistMilitares();
    configurarCabecalhosOrdenacao();
    renderizarTabelaPessoas();

    // ordem 06/10 (item 8b): NOVO SETOR no topo do modo Setores (POST catálogo existente)
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
          window.ViewPessoal();
        }
      };
    };
    const btNovoSetor = $('#btNovoSetor');
    if (btNovoSetor) btNovoSetor.onclick = abrirModalNovoSetor;
    const btNovoSetorCh = $('#btNovoSetorChefes');
    if (btNovoSetorCh) btNovoSetorCh.onclick = abrirModalNovoSetor;
  };


  // exports
})();
