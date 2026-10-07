/* SCI — views de GESTÃO (redesign): #/admin · #/grupos (Gerenciar) · #/perfil
   window.ViewAdmin / window.ViewGrupos / window.ViewPerfil (async).
   Consome os helpers globais do core: api, esc, toast, fmtData, fmtHora, pill,
   abrirModal (insere .modal-mask no body e devolve o elemento), confirmar(msg) →
   Promise<boolean>, navAtiva(hash). Vanilla, sem build, sem CDN.
   Comportamento copiado do protótipo web/app.js (v9.11.2) — nada pode sumir. */
/* [FATIADO da views_gestao.js — onda de modularização; recorte puro] */
'use strict';
  const $ = (s) => document.querySelector(s);
(function () {
'use strict';
  let abaGer = 'pessoal';    // sub-aba corrente do Gerenciar
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
    const [grupos, arvore, pessoas, setores, funcoes, contas] = await Promise.all([
      api('/api/grupos'), api('/api/grupos/arvore'), api('/api/pessoas'),
      api('/api/catalogo/setores'), api('/api/catalogo/funcoes'), api('/api/usuarios')]);
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
        <div class="campo"><label>Posto / Graduação</label><select id="pFuncao"><option value="">—</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
        <div class="campo"><label>Status</label><select id="pStatus"><option value="ativo">ativo</option><option value="inativo">inativo</option></select></div></div>
        <button class="primario" id="pSalvar">Salvar</button>
        <h3 style="margin-top:16px">Adição em lote — cole as linhas e importe</h3>
        <p style="color:var(--tx2);font-size:12px;margin:4px 0">Formato (1 por linha, separado por ponto-e-vírgula): <code>nome de guerra ; nome completo ; setor ; posto/graduação</code> — setor e posto/graduação são opcionais e devem já existir no catálogo.</p>
        <textarea id="csv" rows="5" placeholder="SILVA;José da Silva;Comando;Motorista&#10;SOUSA;Maria de Sousa;Serviços&#10;PERES;Bruno Peres"></textarea>
        <button class="acao-linha" id="csvGo" style="margin-top:8px">Importar linhas</button></div>`;

    /* --- banco de pessoal (checkbox por linha p/ operações em lote) --- */
    const linhasP = (pessoas.pessoas || []).map(p =>
      `<tr data-p='${esc(JSON.stringify(p))}'><td><input type="checkbox" class="chkP" data-id="${p.id}"></td>
       <td class="num">#${p.id}</td><td><b>${esc(p.nome_guerra)}</b></td><td>${esc(p.nome_completo)}</td>
       <td>${esc(p.setor || 'SEM SETOR')}</td>
       <td>${esc(p.funcao || 'INDEFINIDO')}</td>
       <td>${p.status === 'ativo' ? '<span class="alerta-ok">● ATIVO</span>' : '<span style="color:var(--tx3)">● INATIVO</span>'}</td>
       <td><button class="acao-linha" data-edit="${p.id}">editar</button>
       <button class="acao-linha" data-fichap="${p.id}" title="Imprimir Dossiê / Ficha Cadastral">📄 ficha</button>
       ${souFuncaoPessoal ? '' : `<button class="acao-linha" data-excP="${p.id}" data-nome="${esc(p.nome_guerra)}">excluir</button>`}</td></tr>`).join('');

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
          <div class="rolagem"><table><thead><tr><th></th><th>ID</th><th>Guerra</th><th>Completo</th><th>Setor</th><th>Posto / Graduação</th><th>Ativo</th><th></th></tr></thead>
          <tbody id="tabP">${linhasP || '<tr><td colspan="8"><span class="vazio">nenhum militar cadastrado</span></td></tr>'}</tbody></table></div></div>
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
                    <option value="funcoes">🎖️ Postos / Graduações</option>
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
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Designação de membros por função do catálogo: <b>1 titular</b> por função (garantido pelo sistema) e quantos auxiliares forem necessários. Somente contas do SEU grupo.</p>
        <div class="rolagem"><table><thead><tr><th>Função</th><th>Designados</th><th>Designar</th></tr></thead>
        <tbody id="tabFun"><tr><td colspan="3"><span class="carregando">…</span></td></tr></tbody></table></div></div>
      </div>
      <div id="gerChefes" class="${abaGer === 'chefes' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">CHEFES DE SETOR — Nomeação e Destituição</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Cada setor com seu chefe atual. Nomear atribui o papel <b>chefe_setor</b> e vincula o setor à conta; destituir remove o papel (setor fica livre).</p>
        <div class="rolagem"><table><thead><tr><th>Setor</th><th>Chefe atual</th><th>Nomear</th></tr></thead>
        <tbody id="tabChefes"><tr><td colspan="3"><span class="carregando">…</span></td></tr></tbody></table></div></div>
      </div>
      <div id="gerAgregado" class="${abaGer === 'agregado' ? '' : 'oculto'}">
        <div class="cartao"><h3 style="margin-top:0">SETORES — Visão Agregada</h3>
        <p style="color:var(--tx2);font-size:12.5px;margin:0 0 10px">Panorama por setor/seção do escopo (próprio grupo e subordinados): pessoal ativo no banco, contas ativas e chefe atual. Use a aba <b>Chefes</b> para nomear/destituir.</p>
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
        { valor: 'chefes', rotulo: 'Chefes' },
        { valor: 'setores', rotulo: 'Setores (Gestão)' },
        { valor: 'agregado', rotulo: 'Setores (Visão Agregada)' }
      ], {
        valorPadrao: abaGer,
        onChange: (k) => {
          abaGer = k;
          ['pessoal', 'tags', 'grupos', 'operadores', 'funcoes', 'chefes', 'agregado', 'setores'].forEach(kk => {
            const el = $('#ger' + kk[0].toUpperCase() + kk.slice(1));
            if (el) el.classList.toggle('oculto', kk !== abaGer);
          });
          if (abaGer === 'tags') carregarCats();
          if (abaGer === 'pessoal') atualizarSelectsCatalogos();
          if (abaGer === 'funcoes') carregarFuncoesMembros();
          if (abaGer === 'chefes') carregarChefes();
          if (abaGer === 'agregado') carregarAgregadoSetores();
          if (abaGer === 'setores') carregarModoSetores();
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
        { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, null
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
    if (abaGer === 'chefes') carregarChefes();
    if (abaGer === 'agregado') carregarAgregadoSetores();
    if (abaGer === 'setores') carregarModoSetores();

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
          <div class="campo"><label>Posto / Graduação</label><select id="lFuncao"><option value="">(manter)</option>${optFuncoes.map(x => `<option value="${x.id}">${esc(x.nome)}</option>`).join('')}</select></div>
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
      const tipos = ['destinos', 'funcoes', 'tags', 'material_tipos', 'material_classes', 'setores'];
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
          💡 <b>Doutrina de Organização v1.5:</b> As tags estão divididas em <b>Pessoal</b> (Situação/Destino/Postos e Graduações) e <b>Material</b> (Situação/Tipo/Classe). A tag de setor do militar é preenchida automaticamente pelo Setor ao qual ele está alocado.
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
    const btNovoSetor = $('#btNovoSetor');
    if (btNovoSetor) btNovoSetor.onclick = () => {
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

  // exports (referenciados por outros módulos / router)
  window.ViewGrupos = ViewGrupos;
})();
