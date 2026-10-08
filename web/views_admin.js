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
  let abaAdmin = 'dashboard'; // sub-aba corrente do painel admin (persiste na sessão)
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
    if (abaAdmin === 'dashboard') await window.admDashboard();
    else if (abaAdmin === 'usuarios') await admUsuarios();
    else if (abaAdmin === 'grupos') await admGrupos();
    else admBackup();
  };
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
  let setoresCat = null, funcoesCat = null; // cache compartilhado (v9.16.9)

  // exports (referenciados por outros módulos / router)
  window.FOCO_GRUPO_ID = FOCO_GRUPO_ID;
  window.buscarNoPorId = buscarNoPorId;
  window.ligarToggles = ligarToggles;
  window.marcarGerente = marcarGerente;
  window.modal = modal;
  window.modalCriarGrupo = modalCriarGrupo;
  window.modalEditarGrupo = modalEditarGrupo;
  window.modalMover = modalMover;
  window.modalSenha = modalSenha;
  window.processarFoto1x1 = processarFoto1x1;
  window.setoresCat = setoresCat;
  window.tblPaginarPara = tblPaginarPara;
})();
