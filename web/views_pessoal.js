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
    pagDiv.innerHTML = `<button type="button" class="paginacao-btn" data-pg="${st.pag - 1}" ${st.pag <= 1 ? 'disabled' : ''}>‹</button>
      <span class="paginacao-info" style="font-size:12px;color:var(--tx2);margin:0 8px">Página ${st.pag} de ${pags}</span>
      <button type="button" class="paginacao-btn" data-pg="${st.pag + 1}" ${st.pag >= pags ? 'disabled' : ''}>›</button>`;
    pagDiv.querySelectorAll('.paginacao-btn').forEach(b => {
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

  window.ViewPessoal = async function () {
    const eu = quem();
    // ordem 06/10 (P4): encarregado/auxiliar de pessoal também gerenciam —
    // mesmas abas; ações restritivas dentro delas são filtradas por gestorPessoal
    // (senha de conta segue gerente/admin no servidor).
    if (!eu || (eu.papel !== 'gerente' && !(window.gestorPessoal && window.gestorPessoal()))) { location.hash = '#/hoje'; return; }
    // ordem 06/10: quem chega aqui sem papel do sistema é encarregado/auxiliar —
    // o servidor nega exclusão de catálogo/pessoa, senha e mover; o front esconde.
    const souFuncaoPessoal = eu.papel !== 'gerente';
    const podeDesignar = eu && (eu.papel === 'gerente' || eu.papel === 'admin');
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
    let optSetores = ativosDe(setores), optFuncoes = ativosDe(funcoes);
    setoresCat = setores; funcoesCat = funcoes; // cache

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
        <div class="campo"><label>Antiguidade</label><select id="pFuncao"><option value="">—</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
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

    // ordem 04/10: abas do módulo → DROPDOWN estilizado
    $('#app').innerHTML = `<h2>Pessoal</h2>
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:6px">
        <span style="font-size:12px;color:var(--tx2)">Seção:</span>
        <div id="abasPesDD" style="min-width:200px"></div></div>
      <div id="pesEfetivo" class="${abaPes === 'efetivo' ? '' : 'oculto'}">
        ${formPessoa}
        <div class="cartao"><h3 style="margin-top:0">BANCO DE PESSOAL (${(pessoas.pessoas || []).length})</h3>
          <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:8px">
            <label style="font-size:13px"><input type="checkbox" id="chkTodosP"> todos</label>
            <button class="primario" id="btEditLote" disabled>Editar selecionados (<span id="nSel">0</span>)</button>
            ${souFuncaoPessoal ? '' : '<button class="perigo" id="btExcLote" disabled>Excluir selecionados (<span id="nSel2">0</span>)</button>'}
            <span style="color:var(--tx2);font-size:12px">com histórico de conferência: exclusão vira inativo (histórico preservado)</span></div>
          <div class="rolagem"><table><thead><tr><th></th><th>ID</th><th>Guerra</th><th>Completo</th><th>Setor</th><th>Antiguidade</th><th>Ativo</th><th>ÚLTIMA MODIFICAÇÃO</th><th></th></tr></thead>
          <tbody id="tabP">${linhasP || '<tr><td colspan="9"><span class="vazio">nenhum militar cadastrado</span></td></tr>'}</tbody></table></div></div>
        <div class="cartao gpx-rel-card">
          <h3 style="margin-top:16px">RELATÓRIO DE FALTAS E ATRASOS</h3>
          <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Selecione uma conferência fechada para emitir o relatório analítico filtrado por situação.</p>
          <div class="gpx-rel-controles">
            <div id="gpxRelConfDD" style="min-width:280px"></div>
            <button type="button" class="primario" id="gpxBtFaltas">SÓ FALTAS</button>
            <button type="button" id="gpxBtAtrasos">SÓ ATRASOS</button>
          </div>
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
        <!-- f2: aba SETORES do módulo Pessoal — GESTÃO (novo/editar/excluir),
             CHEFIAS (nomear/destituir) e PANORAMA (leitura, próprio + subordinados).
             excluir setor: enc/aux SÓ no próprio grupo (servidor valida). -->
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; margin-bottom:10px">
            <div>
              <h3 style="margin:0 0 4px">🏢 Setores — Gestão</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:0">EDITAR abre as ações do setor: ALTERAR NOME, NOMEAR CHEFE (candidatos = pessoas do setor) e EXCLUIR (remaneja o pessoal para SEM SETOR; setor com histórico de conferências não é apagado — desative).</p>
            </div>
            <button class="primario" id="btNovoSetor" style="min-height:36px">+ Novo setor</button>
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
          if (abaPes === 'setores') { carregarModoSetores(); carregarChefes(); carregarAgregadoSetores(); }
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

    /* --- ordem 06/10 (item 10): controles de tabela nas listas do GERENCIA ---
       Banco de pessoal: ordenar em todas as colunas + paginação 20/página.
       Operadores: idem (trivial — mesma chamada). Demais abas plugam o helper
       nos próprios loaders (funcoes/chefes/agregado/setores). Relatórios NÃO. */
    const tabPTbl = document.querySelector('#pesEfetivo table');
    if (tabPTbl) {
      pesTabelaControles('pes-efetivo', tabPTbl, $('#tabP'), [
        null, { tipo: 'num' }, { tipo: 'txt' }, { tipo: 'txt' },
        { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, null
      ], 20);
    }

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
          (porFuncao[l.funcao_id] = porFuncao[l.funcao_id] || { funcao_id: l.funcao_id, funcao_nome: l.funcao_nome, membros: [] }).membros.push(l);
        });
        const ids = Object.keys(porFuncao).sort((a, b) => String(porFuncao[a].funcao_nome).localeCompare(String(porFuncao[b].funcao_nome)));
        if (!ids.length) { tb.innerHTML = `<tr><td colspan="${podeDesignar ? 3 : 2}"><span class="vazio">nenhuma função no catálogo</span></td></tr>`; return; }
        const opContas = contas.filter(c => c.grupo_id === eu.grupo_id && c.ativo)
          .map(c => `<option value="${c.id}">${esc(c.nome_guerra || c.login)} (${esc(c.login)})</option>`).join('');
        tb.innerHTML = ids.map(fid => {
          const f = porFuncao[fid];
          const membros = f.membros.filter(m => m.membro_id > 0);
          const titular = membros.find(m => m.titularidade === 'titular');
          const auxiliares = membros.filter(m => m.titularidade === 'auxiliar');
          const remBtTit = podeDesignar && titular ? ` <button class="acao-linha" data-remfun="${titular.membro_id}">remover</button>` : '';
          const linhaTit = titular
            ? `<b>👑 ${esc(titular.nome_guerra || titular.login)}</b>${remBtTit}`
            : '<span style="color:var(--tx3)">sem titular</span>';
          const linhasAux = auxiliares.map(m => {
            const remBtAux = podeDesignar ? ` <button class="acao-linha" data-remfun="${m.membro_id}">remover</button>` : '';
            return `<div style="margin-top:4px">${esc(m.nome_guerra || m.login)}${remBtAux}</div>`;
          }).join('');
          const colDesignar = podeDesignar ? `
            <td>
              <div class="form-linha" style="gap:6px;align-items:center;flex-wrap:wrap">
                <select data-seluser="${f.funcao_id}" style="min-width:170px"><option value="">— conta —</option>${opContas}</select>
                <select data-seltit="${f.funcao_id}"><option value="titular">titular</option><option value="auxiliar">auxiliar</option></select>
                <button class="primario" data-addfun="${f.funcao_id}" style="font-size:12px;padding:4px 12px">Designar</button>
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
    if (abaPes === 'setores') { carregarModoSetores(); carregarChefes(); carregarAgregadoSetores(); }

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
          const tabStTbl = document.querySelector('#pesSetores table');
          if (tabStTbl) pesOrdenar('pes-setores', tabStTbl, tb,
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
        const tabAgrTbl = document.querySelector('#pesAgregado table');
        if (tabAgrTbl) pesOrdenar('pes-agregado', tabAgrTbl, tb,
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
        const tabChTbl = document.querySelector('#pesChefes table');
        if (tabChTbl) pesOrdenar('pes-chefes', tabChTbl, tb, [{ tipo: 'txt' }, { tipo: 'txt' }]);
      } catch (e) {
        tb.innerHTML = '<tr><td colspan="3"><span class="vazio">Falha ao carregar chefes.</span></td></tr>';
      }
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
      if (r.ok) window.ViewPessoal();
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
      if (ok) window.ViewPessoal();
    };

    /* --- exclusão de militar (individual + lote; com histórico → desativa) --- */
    const excPessoa = async (id, nome) => {
      if (!(await confirmar(`Excluir "${nome}" do banco de pessoal? (com histórico de conferência virará inativo)`))) return;
      const r = await processar(() => api(`/api/pessoas/${id}`, { method: 'DELETE' }), `Excluindo ${nome}…`);
      if (r.ok) {
        toast(r.resultado.desativado ? 'Desativado (histórico preservado)' : 'Excluído');
        window.ViewPessoal();
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
      if (ok || des) window.ViewPessoal();
    };

    /* --- edição em lote: modal único aplica setor/função/status ('(manter)' = não altera) --- */
    $('#btEditLote').onclick = () => {
      const sel = [...document.querySelectorAll('.chkP:checked')].map(c => +c.dataset.id);
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
