/* SCI — Tela de CONFERÊNCIA (#/hoje) e RELATÓRIOS (#/relatorios).
   Consome os helpers globais do core (api, esc, toast, fmtData, fmtHora,
   pill, abrirModal, confirmar, navAtiva) e as classes do protótipo. v200 */
'use strict';
(() => {
  const $ = s => document.querySelector(s);
  const SITUACOES = ['presente', 'atraso', 'falta', 'justificada'];
  const ROTULO = { presente: 'Presente', atraso: 'Atraso', falta: 'Falta', justificada: 'Justificada', nao_verificado: 'NÃO VERIFICADO' };

  /* estado da conferência em curso (RAM, como no protótipo — a fonte de verdade
     é o servidor: estados só vão para o banco no FECHAR) */
  let C = null;
  // ordem Tenente 30/09: militar nunca nasce mais como "presente" implícito —
  // quem ainda não foi verificado exibe NÃO VERIFICADO até o ✅ (ou escolha) do operador
  const sitDe = p => C.est[p.id] || 'nao_verificado';

  /* abrirModal pode ou não devolver a raiz; garantir um nó consultável */
  const modalRaiz = html => {
    const m = abrirModal(html);
    if (m && m.querySelector) return m;
    const ms = document.querySelectorAll('.modal-mask');
    return ms.length ? ms[ms.length - 1] : document;
  };
  const fecharModal = raiz => { if (raiz && raiz.remove) raiz.remove(); };

  // Modal de filtros do Relatório PDF da conferência (ordem Tenente 02/10)
  window.abrirModalPDFConferencia = function (confID) {
    const presets = [
      { k: 'todos', label: 'Selecionar todos', desc: 'conferência completa' },
      { k: 'faltas', label: 'Somente os com falta', desc: 'apenas lançamentos de falta' },
      { k: 'justificados', label: 'Somente os justificados', desc: 'apenas lançamentos justificados (com destino)' },
    ];
    const html = `
      <h3 style="margin:0 0 4px">Relatório PDF da Conferência</h3>
      <div style="font-size:12.5px;color:var(--tx2);margin-bottom:14px">Escolha o recorte antes de gerar:</div>
      <div style="display:flex;flex-direction:column;gap:8px;margin-bottom:16px">
        ${presets.map((p, i) => `
          <label style="display:flex;gap:10px;align-items:flex-start;padding:10px;border:1px solid var(--borda);border-radius:8px;cursor:pointer">
            <input type="radio" name="pdfFiltro" value="${p.k}" ${i === 0 ? 'checked' : ''} style="margin-top:3px">
            <span><b>${p.label}</b><br><span style="font-size:11.5px;color:var(--tx2)">${p.desc}</span></span>
          </label>`).join('')}
      </div>
      <div style="display:flex;gap:8px;justify-content:flex-end">
        <button type="button" class="secundario" id="pdfCancelar">Cancelar</button>
        <button type="button" class="primario" id="pdfGerar">Gerar PDF</button>
      </div>`;
    const m = window.abrirModal(html);
    m.modal.querySelector('#pdfCancelar').onclick = () => m.fechar();
    m.modal.querySelector('#pdfGerar').onclick = () => {
      const f = m.modal.querySelector('input[name="pdfFiltro"]:checked').value;
      m.fechar();
      window.open(`/api/conferencia/${confID}/relatorio.pdf?filtro=${f}&t=${Date.now()}`, '_blank');
    };
  };

  // Modal para abrir e inspecionar detalhes de qualquer conferência (fechada ou arquivada)
  window.abrirModalDetalhesConferencia = async function (confID) {
    try {
      const d = await api('/api/conferencia/' + confID);
      const res = d.resumo || {};
      const pres = d.lancamentos || [];
      const linhaLanc = l => `
        <tr data-texto="${esc((l.nome_guerra + ' ' + (l.setor || '') + ' ' + (l.funcao || '') + ' ' + (l.observacao || '') + ' ' + (l.destino || '')).toLowerCase())}">
          <td><b>${esc(l.nome_guerra)}</b><br><small style="color:var(--tx2)">${esc(l.funcao || '—')}</small></td>
          <td><code style="font-size:11px">${esc(l.setor || '—')}</code></td>
          <td>${pill(l.situacao)}</td>
          <td>${esc(l.destino || '—')}</td>
          <td style="font-size:12px;color:var(--tx2)">${esc(l.observacao || '—')}</td>
        </tr>`;

      const html = `
        <div style="max-width:850px;width:95vw">
          <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px;margin-bottom:12px">
            <div>
              <h3 style="margin:0">Conferência #${d.id} · ${fmtData(d.data)} (${d.status ? d.status.toUpperCase() : 'CONFERÊNCIA'})</h3>
              <div style="font-size:12px;color:var(--tx2)">Criada em ${fmtHora(d.criada_em)}${d.fechada_em ? ' · Fechada em ' + fmtHora(d.fechada_em) : ''} · Responsável: ${esc(d.criado_por || '—')}</div>
            </div>
            <div style="display:flex;gap:6px">
              <button type="button" class="primario" id="modalConfPDF">📄 Relatório PDF</button>
              <button type="button" class="fantasma" id="modalConfFechar">✕ Fechar</button>
            </div>
          </div>
          <div class="resumo" style="margin-bottom:12px">
            <div class="caixa"><b style="color:var(--verde-claro)">${res.presentes || 0}</b><span>Presentes</span></div>
            <div class="caixa"><b style="color:var(--ambar-txt)">${res.atrasos || 0}</b><span>Atrasos</span></div>
            <div class="caixa"><b style="color:var(--verm)">${res.faltas || 0}</b><span>Faltas</span></div>
            <div class="caixa"><b style="color:#60a5fa">${res.justificadas || 0}</b><span>Justificadas</span></div>
          </div>
          <div class="campo" style="margin-bottom:10px">
            <input id="fModalLanc" placeholder="Filtrar por nome de guerra, setor, posto/graduação ou observação…">
          </div>
          <div class="rolagem" style="max-height:55vh">
            <table id="tabModalLanc">
              <thead><tr><th>Militar</th><th>Setor</th><th>Situação</th><th>Destino</th><th>Observação</th></tr></thead>
              <tbody>${pres.map(linhaLanc).join('') || '<tr><td colspan="5"><span class="vazio">Nenhum militar registrado.</span></td></tr>'}</tbody>
            </table>
          </div>
        </div>`;
      const m = window.abrirModal(html);
      m.modal.querySelector('#modalConfFechar').onclick = () => m.fechar();
      m.modal.querySelector('#modalConfPDF').onclick = () => {
        m.fechar();
        window.abrirModalPDFConferencia(confID);
      };
      const fInp = m.modal.querySelector('#fModalLanc');
      if (fInp) {
        fInp.oninput = () => {
          const q = fInp.value.trim().toLowerCase();
          m.modal.querySelectorAll('#tabModalLanc tbody tr').forEach(tr => {
            const txt = tr.dataset.texto || '';
            tr.style.display = !q || txt.includes(q) ? '' : 'none';
          });
        };
      }
    } catch (e) {
      toast('Falha ao abrir conferência: ' + (e.message || e), 'erro');
    }
  };

  /* ================================================================
     #/hoje — CONFERÊNCIA DE PESSOAL (gerente/operador)
     ================================================================ */
  /* --- salvamento parcial (v9.13): grava o estado de 1 militar na conferência aberta --- */
  let CONF_ID = null; // conferência aberta sendo editada (várias simultâneas v9.14.2)
  let marcaTimer = {}, marcaPend = {};
  const marcarParcial = (pid, situacao, destinoId, observacao, verificado) => {
    if (!C.c) return;
    const currentConfId = CONF_ID;
    marcaPend[pid] = { situacao: situacao || null, destino_id: destinoId ?? null, observacao: observacao ?? null, verificado: !!verificado };
    clearTimeout(marcaTimer[pid]);
    marcaTimer[pid] = setTimeout(async () => {
      const corpo = marcaPend[pid];
      delete marcaPend[pid];
      try { await api('/api/conferencia/marcar' + (currentConfId ? '?id=' + currentConfId : ''), { method: 'POST', body: JSON.stringify({ pessoa_id: pid, ...corpo }) }); }
      catch (e) { toast('Falha ao salvar estado parcial', 'erro'); }
    }, 350);
  };
  /* ordem Tenente 30/09: RASCUNHO PERMANENTE — abre na terça à noite, fecha a página,
     quarta continua de onde parou. Se a página fecha com gravação pendente no debounce,
     descarrega NA HORA (fetch keepalive sobrevive ao unload; X-SCI segue exigido). */
  const descarregarPendentes = () => {
    for (const [pid, corpo] of Object.entries(marcaPend)) {
      clearTimeout(marcaTimer[pid]);
      delete marcaTimer[pid];
      try {
        fetch('/api/conferencia/marcar' + (CONF_ID ? '?id=' + CONF_ID : ''), {
          method: 'POST', keepalive: true,
          headers: { 'Content-Type': 'application/json', 'X-SCI': '1' },
          body: JSON.stringify({ pessoa_id: +pid, ...corpo })
        }).catch(() => {});
      } catch (e) {}
    }
    marcaPend = {};
  };
  window.addEventListener('beforeunload', descarregarPendentes);
  window.addEventListener('pagehide', descarregarPendentes);
  /* --- LISTAS (v9.14): #/hoje mostra SÓ as listas de conferências; a conferência
     em si fica em #/conferencia (botão Abrir). Abertas editáveis; fechadas = PDF. --- */
  window.ViewHoje = async function (modoTela) {
    navAtiva('#/hoje');
    $('#app').innerHTML = '<div class="carregando">Carregando conferências…</div>';
    modoTela = modoTela || 'conferencias';
    // ordem Tenente 30/09: aba ARQUIVO (arquivadas, sem limite de período) × CONFERÊNCIAS
    // (abertas + fechadas com filtro Dia/Semana/Mês/Ano/Livre — mesmo seletor do relatório)
    const hojeD = new Date();
    let qs = '';
    if (modoTela === 'arquivo') {
      qs = '?arq=1';
    } else {
      const p = window.__perConfSel || { m: 'semana', dia: dataLocal(hojeD) };
      window.__perConfSel = p; // garante inicialização imediata
      const sem = d => { const dd = new Date(d + 'T12:00:00'); const dow = (dd.getDay() + 6) % 7; const i = new Date(dd.getTime() - dow * 864e5); return [dataLocal(i), dataLocal(new Date(i.getTime() + 6 * 864e5))]; };
      if (p.m === 'dia') qs = '?de=' + p.dia + '&ate=' + p.dia;
      else if (p.m === 'semana') { const [a, b] = sem(p.dia); qs = '?de=' + a + '&ate=' + b; }
      else if (p.m === 'mes') { const d = new Date(p.dia + 'T12:00:00'); const i = new Date(d.getFullYear(), d.getMonth(), 1); const f = new Date(d.getFullYear(), d.getMonth() + 1, 0); qs = '?de=' + dataLocal(i) + '&ate=' + dataLocal(f); }
      else if (p.m === 'ano') qs = '?de=' + p.dia.slice(0, 4) + '-01-01&ate=' + p.dia.slice(0, 4) + '-12-31';
      else if (p.m === 'livre') qs = '?de=' + (p.de || '') + '&ate=' + (p.ate || '');
    }
    let lista = [];
    try { lista = await api('/api/conferencia/lista' + qs); } catch (e) { lista = []; }
    let haAberta = null;
    try { const d = await api('/api/conferencia/hoje'); haAberta = d.conferencia || null; } catch (e) {}
    const souAdmin = (window.ME && window.ME.papel) === 'admin';
    const linha = c => `
      <tr data-cid="${c.id}"><td class="num"><b>#${c.id}</b></td>
      <td>${c.status === 'aberta' ? 'Aberta' : 'Fechada'}</td>
      <td>${c.status === 'aberta' ? fmtHora(c.criada_em) : fmtHora(c.fechada_em)}</td>
      <td>${fmtData(c.data)}</td>
      <td>${esc(c.grupo || '—')}</td>
      <td>${esc(c.criado_por || '—')}</td>
      <td class="num">${c.lancamentos}</td>
      <td style="white-space:nowrap">${modoTela === 'arquivo'
        ? `<button class="primario" data-abrir-detalhes="${c.id}" style="min-height:36px;padding:8px 12px">Visualizar</button>
           <button style="min-height:36px;padding:8px 12px" data-pdfconf="${c.id}">Relatório PDF</button>
           ${souAdmin ? `<button class="perigo" data-excluir-arq="${c.id}" style="min-height:36px;padding:8px 12px">Excluir</button>` : ''}`
        : (c.status === 'fechada'
          ? `<button class="primario" data-abrir-detalhes="${c.id}" style="min-height:36px;padding:8px 12px">Visualizar</button>
             <button style="min-height:36px;padding:8px 12px" data-pdfconf="${c.id}">Relatório PDF</button>
             <button data-arquivar="${c.id}" style="min-height:36px;padding:8px 12px">Arquivar</button>`
          : `<button class="primario" data-abrir="${c.id}" style="min-height:36px;padding:8px 12px">Abrir</button>`)}</td></tr>`;
    const porData = (a, b) => String(b.data || '').localeCompare(String(a.data || '')) || b.id - a.id;
    const abertas = lista.filter(c => c.status === 'aberta').sort(porData);
    const fechadas = lista.filter(c => c.status === 'fechada').sort(porData);
    const tabela = (titulo, itens, cols) => `
      <h3 style="margin:14px 0 8px">${titulo} (${itens.length})</h3>
      <div class="cartao"><div class="rolagem"><table>
      <thead><tr><th class="num">ID</th><th>Status</th><th>Horário</th><th>Data</th><th>Grupo</th><th>Operador</th><th class="num">Lanç.</th><th>Ações</th></tr></thead>
      <tbody>${itens.map(linha).join('') || `<tr><td colspan="8"><span class="vazio">${cols || 'nenhuma'}</span></td></tr>`}</tbody></table></div></div>`;
    const abasTela = `
      <div class="abas" style="margin:10px 0">
        <button data-t="conferencias" class="${modoTela !== 'arquivo' ? 'ativo' : ''}">CONFERÊNCIAS</button>
        <button data-t="arquivo" class="${modoTela === 'arquivo' ? 'ativo' : ''}">ARQUIVO</button></div>`;
    let seletor = '';
    if (modoTela !== 'arquivo') {
      const p = window.__perConfSel || { m: 'semana', dia: dataLocal(hojeD) };
      if (!p.dia) p.dia = dataLocal(hojeD);
      seletor = `<div class="cartao" style="margin-bottom:10px">
        <div class="abas" id="perConf">
          <button data-p="dia" class="${p.m === 'dia' ? 'ativo' : ''}">Dia</button>
          <button data-p="semana" class="${p.m === 'semana' ? 'ativo' : ''}">Semana</button>
          <button data-p="mes" class="${p.m === 'mes' ? 'ativo' : ''}">Mês</button>
          <button data-p="ano" class="${p.m === 'ano' ? 'ativo' : ''}">Ano</button>
          <button data-p="livre" class="${p.m === 'livre' ? 'ativo' : ''}">Período livre</button></div>
        <div class="form-linha" style="margin-top:8px"><div id="perConfEntrada"></div>
        <button class="primario" id="perConfIr" style="min-height:40px">Aplicar</button></div></div>`;
    }
    const ehChefeSetor = (window.ME && window.ME.papel) === 'chefe_setor';
    $('#app').innerHTML = `<h2>Conferências</h2>${abasTela}${seletor}
      <div class="cartao" style="margin-bottom:10px">
        <div class="campo" style="margin:0">
          <label>Pesquisar conferências por ID, Data ou Operador</label>
          <input id="fConfID" placeholder="Digite para filtrar instantaneamente…">
        </div>
      </div>
      ${modoTela !== 'arquivo' ? `<div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:6px">
        ${!ehChefeSetor ? `<button class="primario" id="btNovaConf" style="min-height:44px">▶ Nova conferência</button>` : ''}
        <span style="color:var(--tx2);font-size:12px">${ehChefeSetor ? 'Como Chefe de Setor, selecione uma conferência aberta para lançar presença do seu efetivo.' : 'abertas podem ser editadas · várias simultâneas · fechadas viram relatório (PDF)'}</span></div>` +
      tabela('Abertas', abertas) + tabela('Fechadas', fechadas)
      : tabela('Arquivadas', lista, 'nenhuma conferência arquivada')}
      <p style="color:var(--tx2);font-size:12px">${modoTela === 'arquivo'
        ? 'No arquivo, você pode pesquisar, abrir e conferir registros antigos ou gerar relatórios PDF a qualquer momento.'
        : 'O relatório PDF só é gerado para conferências fechadas. Arquivar tira a conferência desta listagem (vai para o ARQUIVO).'}`;
    document.querySelectorAll('.abas button[data-t]').forEach(b => b.onclick = () => window.ViewHoje(b.dataset.t));
    const pc = $('#perConfEntrada');
    if (pc) {
      const p = window.__perConfSel || (window.__perConfSel = { m: 'semana', dia: dataLocal(hojeD) });
      const inp = () => {
        if (p.m === 'livre') pc.innerHTML = `<div class="campo"><label>De — até</label><div style="display:flex;gap:6px"><input type="date" id="pcDe" value="${p.de || ''}"><input type="date" id="pcAte" value="${p.ate || ''}"></div></div>`;
        else pc.innerHTML = `<div class="campo"><label>${p.m === 'ano' ? 'Ano (qualquer dia do ano)' : p.m === 'mes' ? 'Mês (qualquer dia do mês)' : p.m === 'dia' ? 'Dia' : 'Semana (qualquer dia dela)'}</label><input type="date" id="pcDia" value="${p.dia || dataLocal(hojeD)}"></div>`;
      };
      inp();
      document.querySelectorAll('#perConf button').forEach(b => b.onclick = () => {
        p.m = b.dataset.p; window.__perConfSel = p;
        document.querySelectorAll('#perConf button').forEach(x => x.classList.toggle('ativo', x === b));
        inp();
      });
      $('#perConfIr').onclick = () => {
        const d = $('#pcDia'); const de = $('#pcDe'); const ate = $('#pcAte');
        if (d) p.dia = d.value;
        if (de) p.de = de.value;
        if (ate) p.ate = ate.value;
        window.ViewHoje('conferencias');
      };
    }
    const btNova = $('#btNovaConf');
    if (btNova) btNova.onclick = async () => {
      try {
        const r = await api('/api/conferencia/iniciar', { method: 'POST', body: '{}' });
        location.hash = '#/conferencia?id=' + (r.conferencia ? r.conferencia.id : r.id);
      } catch (e) {}
    };
    document.querySelectorAll('[data-abrir]').forEach(b => b.onclick = () => {
      location.hash = '#/conferencia?id=' + b.dataset.abrir;
    });
    document.querySelectorAll('[data-abrir-detalhes]').forEach(b => b.onclick = () => {
      window.abrirModalDetalhesConferencia(b.dataset.abrirDetalhes);
    });
    document.querySelectorAll('[data-pdfconf]').forEach(b => {
      b.onclick = () => window.abrirModalPDFConferencia(b.dataset.pdfconf);
    });
    document.querySelectorAll('[data-arquivar]').forEach(b => b.onclick = async () => {
      if (!(await confirmar(`Arquivar a conferência #${b.dataset.arquivar}? Ela sai desta listagem e vai para o ARQUIVO.`))) return;
      try { await api(`/api/conferencia/${b.dataset.arquivar}/arquivar`, { method: 'POST', body: '{}' }); toast('Conferência arquivada'); window.ViewHoje('conferencias'); } catch (e) {}
    });
    document.querySelectorAll('[data-excluir-arq]').forEach(b => {
      if (!souAdmin) { b.style.display = 'none'; return; } // exclusão é exclusiva do admin
      b.onclick = async () => {
        if (!(await confirmar(`☢️ Excluir do ARQUIVO a conferência #${b.dataset.excluirArq}? Apaga conferência, lançamentos e comentários — não tem volta.`))) return;
        try { await api(`/api/conferencia/arquivada/${b.dataset.excluirArq}`, { method: 'DELETE', body: '{}' }); toast('Conferência excluída do arquivo'); window.ViewHoje('arquivo'); } catch (e) {}
      };
    });
    const fID = $('#fConfID');
    if (fID) fID.oninput = () => {
      const q = fID.value.trim().toLowerCase().replace('#', '');
      document.querySelectorAll('#app tr[data-cid]').forEach(tr => {
        const text = tr.innerText.toLowerCase();
        tr.style.display = !q || tr.dataset.cid === q || text.includes(q) ? '' : 'none';
      });
    };
  };

  /* --- CONFERÊNCIA (edição): #/conferencia — aberta em andamento --- */
  window.ViewConferencia = async function () {
    navAtiva('#/hoje');
    $('#app').innerHTML = '<div class="carregando">Carregando efetivo…</div>';
    CONF_ID = new URLSearchParams(location.hash.split('?')[1] || '').get('id') || null;
    const qs = CONF_ID ? '?id=' + CONF_ID : '';
    const d = await api('/api/conferencia/hoje' + qs);
    if (!d.conferencia) { location.hash = '#/hoje'; return; }
    let destinos = [];
    try { destinos = await api('/api/catalogo/destinos'); } catch (e) { destinos = []; }
    destinos = destinos.filter(x => x.ativo === 1 || x.ativo === true);
    const est = {}, dest = {}, obs = {}, verif = new Set(), temComentario = {};
    if (d.conferencia) {
      for (const [pid, v] of Object.entries(d.conferencia.estados || {})) {
        est[pid] = v.situacao;
        dest[pid] = v.destino_id;
        if (v.observacao) obs[pid] = v.observacao;
        // v9.16.4: checkbox só vem marcado se o operador JÁ verificou NESTA conferência
        // (carry over grava verificado=0 → checkbox começa zerado)
        if (v.verificado) verif.add(+pid);
      }
    }
    const escalados = d.escalados || [];
    const escaladosOntem = d.escalados_ontem || [];
    let pessoasLista = d.pessoas || [];
    const ehChefe = window.ME && window.ME.papel === 'chefe_setor';
    if (ehChefe) {
      const meuSetorId = window.ME.setor_id;
      if (meuSetorId) {
        pessoasLista = pessoasLista.filter(p => p.setor_id === meuSetorId);
      }
    }
    C = { c: d.conferencia, pessoas: pessoasLista, destinos, est, dest, obs, verif, temComentario, escalados, escaladosOntem, setoresStatus: d.setores_status || [] };
    confRender();
    $('#btVoltar').onclick = () => { location.hash = '#/hoje'; };
    const btDesc = $('#btDescartar');
    if (btDesc) {
      btDesc.onclick = async () => {
        if (!(await confirmar(`DESCARTAR a conferência #${C.c.id}? O estado parcial gravado será apagado. Esta ação não pode ser desfeita.`))) return;
        try {
          await api('/api/conferencia/' + C.c.id, { method: 'DELETE' });
          toast('Conferência descartada');
          location.hash = '#/hoje';
          location.reload();
        } catch (e) {}
      };
    }
  };

  function confRender(filtro = '') {
    const semC = !C.c;
    const ehChefe = window.ME && window.ME.papel === 'chefe_setor';
    const f = (filtro || '').trim().toLowerCase();
    const porSetor = {};
    C.pessoas
      .filter(p => !f || (p.nome_guerra || '').toLowerCase().includes(f) || (p.nome_completo || '').toLowerCase().includes(f))
      .forEach(p => { (porSetor[p.setor || 'INDEFINIDO'] = porSetor[p.setor || 'INDEFINIDO'] || []).push(p); });
    // ordem dentro do setor (v9.14.1): sem check primeiro, depois alfabética
    const ordemCheck = (a, b) => (C.verif.has(a.id) - C.verif.has(b.id))
      || (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt', { sensitivity: 'base' });
    for (const s of Object.keys(porSetor)) porSetor[s].sort(ordemCheck);
    let listas = '';
    for (const setor of Object.keys(porSetor).sort()) {
      const pessoasSetor = porSetor[setor];
      const totalVerif = pessoasSetor.filter(x => C.verif.has(x.id)).length;
      let jaInseriuDivisor = false;

      const itensHTML = pessoasSetor.map((p, idx) => {
        const sit = sitDe(p);
        const ehVerif = C.verif.has(p.id);
        const escHoje = (C.escalados || []).find(x => x.pessoa_id === p.id);
        const escOntem = (C.escaladosOntem || []).find(x => x.pessoa_id === p.id);
        let badgeEscala = '';
        if (escHoje) {
          badgeEscala = `<span style="display:inline-flex;align-items:center;margin-left:4px;cursor:help;font-size:13px" title="Escalado HOJE em: ${esc(escHoje.tipo_nome || 'Escala')}">📅🔴</span>`;
        } else if (escOntem) {
          badgeEscala = `<span style="display:inline-flex;align-items:center;margin-left:4px;cursor:help;font-size:13px" title="Escalado ONTEM (dia pós-escala) em: ${esc(escOntem.tipo_nome || 'Escala')}">📅🟡</span>`;
        }
        const selDest = sit === 'justificada'
          ? `<select class="sel-destino" data-id="${p.id}"><option value="">destino…</option>` +
            C.destinos.map(dx => `<option value="${dx.id}" ${C.dest[p.id] == dx.id ? 'selected' : ''}>${esc(dx.nome)}</option>`).join('') + '</select>'
          : '';
        const optSit = s => `<option value="${s}" ${sit === s ? 'selected' : ''}>${ROTULO[s]}</option>`;

        let divisorHTML = '';
        if (ehVerif && !jaInseriuDivisor) {
          jaInseriuDivisor = true;
          divisorHTML = `
            <div class="divisor-verificados">
              <span class="divisor-linha"></span>
              <span class="divisor-rotulo">✓ Verificados (${totalVerif} de ${pessoasSetor.length})</span>
              <span class="divisor-linha"></span>
            </div>
          `;
        }

        return divisorHTML + `<div class="pessoa ${ehVerif ? 'verificado' : ''}" data-id="${p.id}">
          <input type="checkbox" class="chk" data-id="${p.id}" ${ehVerif ? 'checked' : ''} title="verifiquei esta pessoa">
          <span class="nome"><b>${esc(p.nome_guerra)}</b> ${badgeEscala}<small>${esc(p.nome_completo)}${p.funcao ? ' · ' + esc(p.funcao) : ''}${C.obs[p.id] ? ' · 📝' : ''}${C.temComentario[p.id] ? ' · 💬' : ''}</small></span>
          <select class="sel-situacao" data-id="${p.id}" title="situação">${sit === 'nao_verificado' ? '<option value="nao_verificado" disabled selected>NÃO VERIFICADO</option>' : ''}${SITUACOES.map(optSit).join('')}</select>
          ${selDest}<button type="button" class="fantasma bt-coment" data-id="${p.id}" title="comentários" style="min-height:36px;padding:4px 8px">💬</button></div>`;
      }).join('');

    let dashboardSetoresHTML = '';
    if (!semC && C.setoresStatus && C.setoresStatus.length > 0) {
      const concCount = C.setoresStatus.filter(s => s.status === 'concluida').length;
      const andamCount = C.setoresStatus.filter(s => s.status === 'em_andamento').length;
      const naoIniCount = C.setoresStatus.filter(s => s.status === 'nao_iniciada').length;
      
      const cards = C.setoresStatus.map(s => {
        const isMeuSetor = ehChefe && window.ME.setor_id === s.setor_id;
        const podeGerenciar = !ehChefe || isMeuSetor;
        
        let statusBadge = '';
        let bordaCor = 'var(--borda)';
        let bgCor = 'rgba(255,255,255,0.02)';
        
        if (s.status === 'concluida') {
          bordaCor = 'rgba(16, 185, 129, 0.4)';
          bgCor = 'rgba(16, 185, 129, 0.06)';
          statusBadge = `<span style="display:inline-flex;align-items:center;gap:4px;color:#10b981;font-weight:600;font-size:11.5px">
            <span style="font-size:14px">🟢</span> Concluída
          </span>`;
        } else if (s.status === 'em_andamento') {
          bordaCor = 'rgba(245, 158, 11, 0.4)';
          bgCor = 'rgba(245, 158, 11, 0.06)';
          statusBadge = `<span style="display:inline-flex;align-items:center;gap:4px;color:#f59e0b;font-weight:600;font-size:11.5px">
            <span style="font-size:14px">⏳</span> Em andamento
          </span>`;
        } else {
          bordaCor = 'rgba(239, 68, 68, 0.4)';
          bgCor = 'rgba(239, 68, 68, 0.06)';
          statusBadge = `<span style="display:inline-flex;align-items:center;gap:4px;color:#ef4444;font-weight:600;font-size:11.5px">
            <span style="font-size:14px">🔴</span> Não iniciada
          </span>`;
        }

        let acaoBtn = '';
        if (podeGerenciar) {
          if (s.status === 'concluida') {
            acaoBtn = `<button type="button" class="fantasma bt-setor-acao" data-acao="reabrir" data-sid="${s.setor_id}" style="min-height:28px;padding:2px 8px;font-size:11px">↺ Reabrir</button>`;
          } else {
            acaoBtn = `<button type="button" class="primario bt-setor-acao" data-acao="concluir" data-sid="${s.setor_id}" style="min-height:28px;padding:2px 8px;font-size:11px;background:#10b981;border-color:#10b981">✓ Concluir</button>`;
          }
        }

        return `
          <div style="border:1px solid ${bordaCor};background:${bgCor};border-radius:8px;padding:10px 12px;display:flex;flex-direction:column;justify-content:space-between;gap:8px">
            <div>
              <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:6px">
                <span style="font-weight:700;font-size:13.5px">${esc(s.setor_sigla || s.setor_nome)}</span>
                ${statusBadge}
              </div>
              <div style="font-size:11.5px;color:var(--tx2);margin-top:2px">${esc(s.setor_nome)}</div>
            </div>
            <div style="display:flex;justify-content:space-between;align-items:center;padding-top:6px;border-top:1px solid rgba(255,255,255,0.06)">
              <span style="font-size:11.5px;color:var(--tx2)"><b>${s.verificados}</b> de ${s.total_pessoas} verif.</span>
              ${acaoBtn}
            </div>
          </div>
        `;
      }).join('');

      dashboardSetoresHTML = `
        <div class="cartao" style="margin-bottom:14px;padding:12px 14px">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px;flex-wrap:wrap;gap:8px">
            <div style="display:flex;align-items:center;gap:8px">
              <span style="font-size:16px">📋</span>
              <b style="font-size:13px;letter-spacing:0.5px;text-transform:uppercase">Conferência por Setores</b>
            </div>
            <div style="display:flex;gap:12px;font-size:12px">
              <span style="color:#10b981">🟢 ${concCount} concluídos</span>
              <span style="color:#f59e0b">⏳ ${andamCount} em andamento</span>
              <span style="color:#ef4444">🔴 ${naoIniCount} não iniciados</span>
            </div>
          </div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(210px,1fr));gap:10px">
            ${cards}
          </div>
        </div>
      `;
    }

    for (const setor of Object.keys(porSetor).sort()) {
      const pessoasSetor = porSetor[setor];
      const sObj = (C.setoresStatus || []).find(x => (x.setor_nome || '').toLowerCase() === setor.toLowerCase() || (x.setor_sigla || '').toLowerCase() === setor.toLowerCase() || (pessoasSetor[0] && x.setor_id === pessoasSetor[0].setor_id));
      let setorBadgeHeader = '';
      if (sObj) {
        if (sObj.status === 'concluida') {
          setorBadgeHeader = `<span style="font-size:11.5px;color:#10b981;font-weight:600;padding:2px 8px;border-radius:12px;background:rgba(16,185,129,0.1);border:1px solid rgba(16,185,129,0.3)">🟢 Concluída</span>`;
        } else if (sObj.status === 'em_andamento') {
          setorBadgeHeader = `<span style="font-size:11.5px;color:#f59e0b;font-weight:600;padding:2px 8px;border-radius:12px;background:rgba(245,158,11,0.1);border:1px solid rgba(245,158,11,0.3)">⏳ Em andamento</span>`;
        } else {
          setorBadgeHeader = `<span style="font-size:11.5px;color:#ef4444;font-weight:600;padding:2px 8px;border-radius:12px;background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3)">🔴 Não iniciada</span>`;
        }
      }

      listas += `<div class="grupo-setor">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
          <h4 style="margin:0">${esc(setor)} · ${pessoasSetor.length}</h4>
          ${setorBadgeHeader}
        </div>
        <div class="lista-pessoa">${itensHTML}</div>
      </div>`;
    }
    const banner = semC
      ? `<div class="cartao"><p style="color:var(--tx2)">Nenhuma conferência aberta. Ao iniciar, a data e o horário de Brasília são registrados automaticamente.</p>
         ${!ehChefe ? `<div style="display:flex;gap:8px;align-items:end;margin-top:10px">
           <button class="primario" id="btIniciar" style="min-height:44px">▶ Iniciar conferência</button></div>` : '<p style="color:var(--tx3);font-size:12px;margin-top:8px">Aguarde o Gerente ou Operador iniciar a conferência do grupo.</p>'}</div>`
      : `<div class="cartao">
         <span>${pill('aberta')} <b>Conferência #${C.c.id}</b> · aberta em ${fmtData(C.c.data)} às ${fmtHora(C.c.criada_em)}${C.c.local ? ' · ' + esc(C.c.local) : ''}</span></div>`;
    $('#app').innerHTML = `<div style="margin-bottom:10px"><button class="fantasma" id="btVoltar" style="min-height:38px">← Retornar</button></div>
      <h2 style="margin-top:0">Conferência de pessoal</h2>${banner}
      ${dashboardSetoresHTML}
      <div class="barra-fixa">
        <input id="busca" placeholder="buscar nome…">
        ${!ehChefe ? `<button class="primario" id="btFecharBarra">✕ FECHAR CONFERÊNCIA</button>` : `<span style="font-size:12px;color:var(--tx2);font-weight:600">Área do Chefe de Setor</span>`}
      </div>
      <div id="lista">${listas}</div>
      ${!ehChefe ? `
      <div style="display:flex;justify-content:flex-end;margin-top:28px;padding-top:14px;border-top:1px solid var(--borda)">
        <button class="perigo" id="btDescartar" style="min-height:40px">🗑 Descartar conferência</button>
      </div>` : ''}`;
    const contSpan = () => `<b>${C.verif.size}/${C.pessoas.length}</b> verificados`;
    const atualizar = () => {
      // v9.15.1: NÃO recriar a barra (perdia foco a cada dígito) — só o contador muda
      const cont = document.querySelector('.barra-fixa .cont');
      if (cont) cont.innerHTML = contSpan();
    };
    let buscaTimer = null;
    const ligarBarra = () => {
      const inp = $('#busca');
      if (inp) inp.oninput = ev => {
        const v = ev.target.value;
        clearTimeout(buscaTimer);
        buscaTimer = setTimeout(() => {
          const pos = inp.selectionStart; // preserva cursor
          confRender(v);
          const inp2 = $('#busca');
          if (inp2) { inp2.focus(); inp2.setSelectionRange(pos, pos); }
        }, 250);
      };
      const btF = $('#btFecharBarra');
      if (btF) btF.onclick = confFechar;
    };
    atualizar();
    /* situação por DROP-DOWN — mudança de situação NÃO dá check automático (check é manual) */
    document.querySelectorAll('.sel-situacao').forEach(s => s.onchange = () => {
      const id = +s.dataset.id;
      const novo = s.value;
      const atual = C.est[id] || 'nao_verificado';
      if (novo === atual) return;
      C.est[id] = novo;
      // v1.5: destinos só existem em justificada (presente/falta/atraso destino zerado)
      if (novo !== 'justificada') {
        C.dest[id] = null;
      }
      // v1.5: caso ocorra mudança de estado, a observação é resetada
      C.obs[id] = '';
      // Regra operacional: o check é estritamente manual, preserva o estado atual de verificação
      const jaVerif = C.verif.has(id);
      if (novo === 'falta' || novo === 'justificada') {
        confModalLancamento(id, novo, () => confRender($('#busca') ? $('#busca').value : ''));
      } else {
        marcarParcial(id, novo, null, '', jaVerif);
        confRender($('#busca') ? $('#busca').value : '');
      }
    });
    document.querySelectorAll('.pessoa .chk').forEach(ch => ch.onchange = () => {
      const id = +ch.dataset.id;
      if (ch.checked) C.verif.add(id); else C.verif.delete(id);
      ch.closest('.pessoa').classList.toggle('verificado', ch.checked);
      atualizar();
      // salvamento parcial: check grava o estado atual + verificado; UNCHECK grava
      // verificado=0 (situacao preservada ou nula)
      if (ch.checked) marcarParcial(id, C.est[id] || 'presente', C.dest[id] ?? null, C.obs[id] ?? null, true);
      else marcarParcial(id, C.est[id] || null, C.dest[id] ?? null, C.obs[id] ?? null, false);
      confRender($('#busca') ? $('#busca').value : '');
    });
    document.querySelectorAll('.sel-destino').forEach(s => s.onchange = () => {
      const id = +s.dataset.id;
      const novo = +s.value || null;
      if ((C.est[id] || '') === 'justificada' && !novo) {
        toast('Justificada exige destino', 'erro'); // volta ao anterior; não grava estado inválido
        s.value = C.dest[id] || '';
        return;
      }
      C.dest[id] = novo;
      // ordem Tenente 30/09: destino TAMBÉM grava na hora (antes ficava só na RAM
      // e a mudança se perdia num reload acidental)
      marcarParcial(id, C.est[id] || 'presente', novo, C.obs[id] ?? null, C.verif.has(id));
    });
    document.querySelectorAll('.bt-coment').forEach(b => b.onclick = ev => { ev.stopPropagation(); confModalComentarios(+b.dataset.id); });
    document.querySelectorAll('.bt-setor-acao').forEach(btn => {
      btn.onclick = async ev => {
        ev.stopPropagation();
        const sid = +btn.dataset.sid;
        const acao = btn.dataset.acao;
        try {
          if (acao === 'concluir') {
            await api(`/api/conferencia/${C.c.id}/setor/${sid}/concluir`, { method: 'POST' });
            toast('Conferência do setor concluída com sucesso!');
          } else {
            await api(`/api/conferencia/${C.c.id}/setor/${sid}/reabrir`, { method: 'POST' });
            toast('Conferência do setor reaberta!');
          }
          await window.ViewConferencia();
        } catch (e) {
          toast('Erro: ' + (e.message || e), 'erro');
        }
      };
    });
    atualizar(); // contador de verificados acompanha o re-render (v9.15.2)
    ligarBarra(); // v9.16.5b: barra é recriada no innerHTML — religar FECHAR e busca
  }


  /* --- comentários: append-only, por conferência --- */
  async function confModalComentarios(pessoaId) {
    if (!C.c) return;
    const p = C.pessoas.find(x => x.id === pessoaId);
    const cid = C.c.id;
    const raiz = modalRaiz(`<div class="modal" style="max-width:460px">
      <h3>💬 Comentários — ${esc(p ? p.nome_guerra : '')}</h3>
      <div id="cmLista" style="max-height:220px;overflow:auto;margin-bottom:10px">
        <span class="vazio">carregando…</span></div>
      <div class="campo"><label>Novo comentário</label>
        <textarea id="cmNovo" rows="2" placeholder="registre aqui…"></textarea></div>
      <div class="modal-acoes">
        <button class="fantasma" id="cmX">Fechar</button>
        <button class="primario" id="cmGo">Adicionar</button></div></div>`);
    const fechar = () => fecharModal(raiz);
    raiz.onclick = ev => { if (ev.target === raiz) fechar(); };
    raiz.querySelector('#cmX').onclick = fechar;
    raiz.addEventListener('keydown', ev => { if (ev.key === 'Escape') fechar(); });
    const carregar = async () => {
      try {
        const lista = await api(`/api/comentarios/${cid}`);
        const meus = lista.filter(c => p && c.pessoa === p.nome_guerra);
        C.temComentario[pessoaId] = meus.length > 0;
        
        const mostrar = meus.slice(-3);
        const omitidos = meus.length - mostrar.length;
        const msgOmitidos = omitidos > 0 ? `<div style="text-align:center;font-size:12px;color:var(--tx3);margin-bottom:8px">▲ ${omitidos} comentários anteriores ocultos. Acesse a Busca Individual para ver todos.</div>` : '';
        
        raiz.querySelector('#cmLista').innerHTML = meus.length ? msgOmitidos + mostrar.map(c =>
          `<div class="cartao" style="padding:8px;margin-bottom:6px">
           <small style="color:var(--tx2)">#${c.ordem} · ${esc(c.operador)} · ${(c.datahora || '').slice(0, 16).replace('T', ' ')}</small>
           <div>${esc(c.comentario)}</div></div>`).join('') : '<span class="vazio">sem comentários</span>';
        confRender($('#busca') ? $('#busca').value : '');
      } catch (e) { raiz.querySelector('#cmLista').innerHTML = '<span class="vazio">falha ao carregar</span>'; }
    };
    await carregar();
    raiz.querySelector('#cmGo').onclick = async () => {
      const txt = raiz.querySelector('#cmNovo').value.trim();
      if (!txt) { toast('Escreva o comentário', 'erro'); return; }
      try {
        await api('/api/comentarios', { method: 'POST', body: JSON.stringify({ conferencia_id: cid, pessoa_id: pessoaId, comentario: txt }) });
        raiz.querySelector('#cmNovo').value = '';
        toast('Comentário adicionado');
        await carregar();
      } catch (e) {}
    };
  }

  /* --- modal de lançamento: falta/justificada; justificada EXIGE destino --- */
  function confModalLancamento(id, sit, aoSalvar) {
    const p = C.pessoas.find(x => x.id === id);
    const raiz = modalRaiz(`<div class="modal">
      <h3>${sit === 'falta' ? 'Falta' : 'Justificada'} — ${esc(p ? p.nome_guerra : '')}</h3>
      ${sit === 'justificada' ? `<div class="campo"><label>Destino (obrigatório)</label>
        <select id="mDest"><option value="">— escolher —</option>${C.destinos.map(dx =>
          `<option value="${dx.id}" ${C.dest[id] == dx.id ? 'selected' : ''}>${esc(dx.nome)}</option>`).join('')}</select></div>` : ''}
      <div class="campo"><label>Motivo / observação</label><textarea id="mObs" rows="3" placeholder="ex.: não compareceu, sem contato…">${esc(C.obs[id] || '')}</textarea></div>
      <div class="modal-acoes">
        <button class="fantasma" id="mCancel">Cancelar</button>
        <button class="primario" id="mOk">Gravar</button>
      </div></div>`);
    const fechar = () => fecharModal(raiz);
    raiz.onclick = ev => { if (ev.target === raiz) fechar(); };
    raiz.querySelector('#mCancel').onclick = fechar;
    raiz.querySelector('#mOk').onclick = () => {
      const destino = raiz.querySelector('#mDest') ? (+raiz.querySelector('#mDest').value || null) : null;
      const obs = raiz.querySelector('#mObs').value.trim();
      if (sit === 'justificada' && !destino) { toast('Justificada exige destino', 'erro'); return; }
      C.dest[id] = destino; C.obs[id] = obs;
      const jaVerif = C.verif.has(id);
      marcarParcial(id, sit, destino, obs, jaVerif);
      fechar();
      if (aoSalvar) aoSalvar();
    };
    raiz.addEventListener('keydown', ev => { if (ev.key === 'Escape') fechar(); });
    const ta = raiz.querySelector('#mObs');
    if (ta) ta.focus();
  }

  /* --- fechar: valida, confirma, grava N lançamentos e RECARREGA a página --- */
  async function confFechar() {
    if (!C.c) return;
    const semDest = C.pessoas.filter(p => (C.est[p.id] || 'presente') === 'justificada' && !C.dest[p.id]);
    if (semDest.length) { toast('Justificada exige destino: ' + semDest.map(p => p.nome_guerra).join(', '), 'erro'); return; }
    const naoVerif = C.pessoas.filter(p => !C.verif.has(p.id)).map(p => p.nome_guerra);
    let msg = `Fecha a conferência e grava ${C.pessoas.length} lançamentos?`;
    const setoresPendentes = (C.setoresStatus || []).filter(s => s.status !== 'concluida');
    if (setoresPendentes.length > 0) {
      const nomes = setoresPendentes.map(s => `${s.setor_sigla || s.setor_nome} (${s.status === 'nao_iniciada' ? 'Não iniciada' : 'Em andamento'})`).join(', ');
      msg = `ATENÇÃO: Os setores [${nomes}] ainda não concluíram a conferência setorial.\n\n` + msg;
    }
    if (naoVerif.length) msg = `ATENÇÃO: ${naoVerif.length} sem verificação (${naoVerif.join(', ')}).\n\n` + msg;
    if (!(await confirmar(msg))) return;
    const lanc = C.pessoas.map(p => ({
      pessoa_id: p.id,
      situacao: C.est[p.id] || 'presente',
      destino_id: C.dest[p.id] || null,
      observacao: C.obs[p.id] || null,
      verificado: !!C.verif.has(p.id) // ordem Tenente 30/09: sem ✅ o SERVIDOR grava NÃO VERIFICADO
    }));
    try {
      const r = await api('/api/conferencia/fechar', { method: 'POST', body: JSON.stringify({ id: C.c.id, lancamentos: lanc }) });
      toast(`Conferência fechada — ${r.gravados} lançamentos gravados`);
      location.hash = '#/hoje'; // volta para as listas
      location.reload(); // recarga completa da página
    } catch (err) {}
  }

  /* ================================================================
     #/relatorios — RELATÓRIOS (todos os papéis)
     ================================================================ */
  const dataLocal = d => d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
  const semanaDe = isoDia => {
    const d = new Date(isoDia + 'T12:00:00');
    const dow = (d.getDay() + 6) % 7; // segunda = 0
    const ini = new Date(d.getTime() - dow * 864e5);
    const fim = new Date(ini.getTime() + 6 * 864e5);
    return [dataLocal(ini), dataLocal(fim)];
  };

  function relRender(b, titulo, grupoQ) {
    const totalFaltas = b.total_faltas ?? ((b.falta || 0) + (b.justificadas || 0));
    const res = `<div class="resumo">
      <div class="caixa"><b>${b.efetivo_ativo}</b><span>efetivo</span></div>
      <div class="caixa"><b>${b.presentes}</b><span>presentes</span></div>
      <div class="caixa"><b>${b.atrasos}</b><span>atrasos</span></div>
      <div class="caixa"><b>${b.falta}</b><span>faltas</span></div>
      <div class="caixa"><b>${b.justificadas}</b><span>justificadas</span></div>
      <div class="caixa"><b>${b.nao_verificados || 0}</b><span>não verif.</span></div>
      <div class="caixa"><b>${totalFaltas}</b><span>faltas tot. (J+NJ)</span></div>
      <div class="caixa" style="border-color:var(--verde)"><b>${b.pct_pronto}%</b><span>ef. pronto</span></div></div>`;

    const todasPessoas = (b.pessoas || []).slice()
      .sort((a, x) => String(a.nome_guerra || '').localeCompare(String(x.nome_guerra || ''), 'pt', { sensitivity: 'base' }));

    const forms = (b.formaturas || []).map(f =>
      `<tr><td>${fmtData(f.data)}</td><td>${esc(f.tipo)}</td><td>${esc(f.hora || '—')}</td>
       <td>${f.status === 'fechada' ? 'Fechada' : 'Aberta'}</td>
       <td class="num">${esc(f.presentes)}</td><td class="num">${esc(f.faltas)}</td></tr>`).join('');

    let pgAtual = 1;
    let filtroTexto = '';

    const renderTabelaPessoas = (container) => {
      const filtradas = todasPessoas.filter(p => {
        if (!filtroTexto) return true;
        const q = filtroTexto.toLowerCase();
        return (p.nome_guerra || '').toLowerCase().includes(q) ||
               (p.setor || '').toLowerCase().includes(q) ||
               (p.funcao || '').toLowerCase().includes(q);
      });

      const pag = paginarArray(filtradas, pgAtual, 10);
      const linhas = pag.dados.map(p =>
        `<tr><td class="num">${esc(p.antiguidade ?? '')}</td><td>${esc(p.funcao || '—')}</td><td><b>${esc(p.nome_guerra)}</b></td>
         <td>${esc(p.setor)}</td><td>${esc(p.grupo || '—')}</td><td class="num">${esc(p.presencas)}</td>
         <td class="num">${esc(p.atrasos)}</td><td class="num">${esc(p.faltas)}</td><td class="num">${esc(p.justificadas)}</td><td class="num">${esc(p.nao_verificados || 0)}</td></tr>`).join('');

      container.innerHTML = `
        <div class="rolagem"><table><thead><tr><th class="num">ORD</th><th>Posto / Graduação</th><th>Nome</th><th>Setor</th><th>Grupo</th><th class="num">Pres.</th><th class="num">Atraso</th>
        <th class="num">Falta</th><th class="num">Just.</th><th class="num">N.V.</th></tr></thead>
        <tbody>${linhas || '<tr><td colspan="10"><span class="vazio">nenhum militar encontrado</span></td></tr>'}</tbody></table></div>
        ${renderPaginadorHTML(pag, 'pagConsolidado')}
      `;

      container.querySelectorAll('.paginacao-btn[data-pg]').forEach(btn => {
        btn.onclick = () => {
          pgAtual = +btn.dataset.pg;
          renderTabelaPessoas(container);
        };
      });
    };

    const containerId = 'contEf_' + Math.random().toString(36).slice(2, 7);

    setTimeout(() => {
      const c = document.getElementById(containerId);
      const inpBusca = document.getElementById('buscaEf_' + containerId);
      if (c) {
        renderTabelaPessoas(c);
        if (inpBusca) {
          inpBusca.oninput = () => {
            filtroTexto = inpBusca.value.trim();
            pgAtual = 1;
            renderTabelaPessoas(c);
          };
        }
      }
    }, 10);

    return `
      ${res}
      <div class="cartao">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;flex-wrap:wrap;gap:8px">
          <h3 style="margin:0">${esc(titulo)} — ${esc(b.convocacoes)} conferências</h3>
          <a href="/api/relatorio.pdf?de=${encodeURIComponent(b.De)}&ate=${encodeURIComponent(b.Ate)}${grupoQ}&t=${Date.now()}" target="_blank">
            <button type="button" class="primario" style="display:inline-flex;align-items:center;gap:6px">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
              <span>Gerar Relatório Consolidado (PDF)</span>
            </button>
          </a>
        </div>
        ${forms ? `<div class="rolagem" style="margin-bottom:14px"><table><thead><tr><th>Data</th><th>Tipo / Turno</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead><tbody>${forms}</tbody></table></div>` : ''}
        
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
          <h4 style="margin:0; font-size:13px; color:var(--tx2)">Detalhamento do Efetivo no Período (Máx. 10 por página)</h4>
          <input id="buscaEf_${containerId}" placeholder="Filtrar militar/setor…" style="max-width:200px; height:32px; font-size:12px">
        </div>
        <div id="${containerId}"></div>
      </div>
    `;
  }

  window.ViewRelatorios = async function () {
    navAtiva('#/relatorios');
    $('#app').innerHTML = '<div class="carregando">Carregando módulo de relatórios…</div>';
    let me = window.ME;
    if (!me || !me.papel) {
      try { me = (await api('/api/me')).usuario; } catch (e) { me = {}; }
    }
    const hoje = new Date();
    let gs = [];
    try { gs = await api('/api/grupos'); } catch (e) {}

    let gSel = '';
    if (me.papel === 'admin') {
      gSel = `<div class="campo"><label>Grupo / Unidade</label><select id="fGrupo"><option value="">Todos os Grupos</option>` +
        gs.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('') + `</select></div>`;
    } else {
      const meuG = gs.find(g => g.id === me.grupo_id);
      const subs = (meuG && meuG.subordinados_ids) || [];
      const nomes = (meuG && meuG.subordinados) || [];
      if (subs.length) {
        gSel = `<div class="campo"><label>Escopo</label><select id="fGrupo">
          <option value="">Meu grupo + subordinados</option>
          <option value="${me.grupo_id}">Meu grupo (somente)</option>` +
          subs.map((id, i) => `<option value="${id}">Somente: ${esc(nomes[i] || ('#' + id))}</option>`).join('') +
          `</select></div>`;
      }
    }

    let abaPrincipal = 'consolidado';

    const renderTela = () => {
      $('#app').innerHTML = `
        <h2>Relatórios & Auditoria</h2>
        <div class="abas" id="abasRelatorios" style="margin-bottom:16px">
          <button data-aba="consolidado" class="${abaPrincipal === 'consolidado' ? 'ativo' : ''}">📊 Resumo Consolidado & PDF</button>
          <button data-aba="conferencias" class="${abaPrincipal === 'conferencias' ? 'ativo' : ''}">📋 Conferências Individuais</button>
          <button data-aba="individual" class="${abaPrincipal === 'individual' ? 'ativo' : ''}">🔍 Busca Individual por Militar</button>
        </div>
        <div id="corpoRelatorios"></div>
      `;

      document.querySelectorAll('#abasRelatorios button').forEach(btn => {
        btn.onclick = () => {
          abaPrincipal = btn.dataset.aba;
          renderTela();
        };
      });

      if (abaPrincipal === 'consolidado') viewRelConsolidado();
      else if (abaPrincipal === 'conferencias') viewRelConferenciasIndividuais();
      else viewRelBuscaIndividual();
    };

    /* =========================================================================
       1. ABA: RESUMO CONSOLIDADO & PDF DO PERÍODO
       ========================================================================= */
    const viewRelConsolidado = () => {
      const alvo = $('#corpoRelatorios');
      alvo.innerHTML = `
        <div class="cartao">
          ${gSel ? `<div class="form-linha" style="margin-bottom:8px">${gSel}</div>` : ''}
          <div class="abas" id="modos" style="margin-bottom:12px">
            <button data-m="dia" class="ativo">Dia</button>
            <button data-m="semana">Semana</button>
            <button data-m="ano">Ano</button>
            <button data-m="livre">Período livre</button>
          </div>
          <div class="form-linha" style="grid-template-columns:1fr auto;align-items:end">
            <div id="entrada"></div>
            <button class="primario" id="btGerar" style="min-height:44px; padding:0 20px">Gerar Relatório</button>
          </div>
        </div>
        <div id="estadoAtual"><div class="carregando">Carregando estado atual do efetivo…</div></div>
        <div id="saida"><div class="carregando">Gerando consolidado…</div></div>
      `;

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
          const gq = $('#fGrupo') && $('#fGrupo').value ? '&grupo=' + encodeURIComponent($('#fGrupo').value) : '';
          const b = await api(`/api/relatorio?de=${encodeURIComponent(de)}&ate=${encodeURIComponent(ate)}${gq}`);
          b.De = de; b.Ate = ate;
          $('#saida').innerHTML = relRender(b, `${rotulos[modo]} ${fmtData(de)} a ${fmtData(ate)}`, gq);
        } catch (e) { $('#saida').innerHTML = ''; }
      };

      entrada();
      (async () => {
        try {
          const html = await window.EfetivoAtualHTML();
          const alvoEf = document.querySelector('#estadoAtual');
          if (alvoEf) alvoEf.innerHTML = html;
        } catch (e) {}
      })();
      gerar();

      document.querySelectorAll('#modos button').forEach(b => b.onclick = () => {
        modo = b.dataset.m;
        document.querySelectorAll('#modos button').forEach(x => x.classList.toggle('ativo', x === b));
        entrada();
      });
      $('#btGerar').onclick = gerar;
    };

    /* =========================================================================
       2. ABA: CONFERÊNCIAS INDIVIDUAIS & RELATÓRIOS INDIVIDUAIS EM PDF
       ========================================================================= */
    const viewRelConferenciasIndividuais = async () => {
      const alvo = $('#corpoRelatorios');
      alvo.innerHTML = `
        <div class="cartao">
          <h3 style="margin-top:0">📋 Conferências Registradas & Relatórios Individuais</h3>
          <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">
            Consulte todas as conferências individualmente com filtros finos. Você pode baixar o relatório PDF específico de cada chamada ou visualizar a ficha de presença completa.
          </p>

          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="max-width:180px">
              <label>De (Data Inicial)</label>
              <input type="date" id="cfDe" value="${dataLocal(new Date(Date.now() - 30 * 864e5))}">
            </div>
            <div class="campo" style="max-width:180px">
              <label>Até (Data Final)</label>
              <input type="date" id="cfAte" value="${dataLocal(hoje)}">
            </div>
            ${gSel ? gSel : ''}
            <div class="campo" style="max-width:170px">
              <label>Situação</label>
              <select id="cfStatus">
                <option value="">Todas</option>
                <option value="fechada">Fechadas</option>
                <option value="aberta">Abertas</option>
                <option value="arquivada">Arquivadas</option>
              </select>
            </div>
          </div>

          <div class="form-linha" style="align-items:flex-end">
            <div class="campo" style="flex:1">
              <label>Buscar por Local / Responsável</label>
              <input id="cfBuscaTxt" placeholder="Digite para filtrar instantaneamente…">
            </div>
            <button type="button" class="primario" id="btFiltrarConfs" style="min-height:42px; padding:0 20px">
              Filtrar Conferências
            </button>
          </div>
        </div>

        <div id="resultadoConfs"><div class="carregando">Buscando conferências…</div></div>
      `;

      let listaConfs = [];
      let pgConfs = 1;

      const renderizarTabelaConfs = () => {
        const txt = ($('#cfBuscaTxt').value || '').trim().toLowerCase();
        const filtradas = listaConfs.filter(c => {
          if (!txt) return true;
          return (c.local || '').toLowerCase().includes(txt) ||
                 (c.criado_por || '').toLowerCase().includes(txt) ||
                 (c.grupo_nome || '').toLowerCase().includes(txt) ||
                 String(c.id).includes(txt);
        });

        const pag = paginarArray(filtradas, pgConfs, 10);
        const resEl = $('#resultadoConfs');

        if (pag.totalItens === 0) {
          resEl.innerHTML = `<div class="cartao"><span class="vazio">Nenhuma conferência encontrada para o período/filtro selecionado.</span></div>`;
          return;
        }

        const linhas = pag.dados.map(c => {
          const isFechada = c.status === 'fechada';
          const isArq = !!c.arquivada_em;
          const statusBadge = isArq
            ? '<span class="pill" style="background:#475569; color:#cbd5e1">Arquivada</span>'
            : (isFechada
                ? '<span class="pill pill-presente">Fechada</span>'
                : '<span class="pill pill-atraso">Aberta</span>');

          return `
            <tr>
              <td class="num"><b>#${c.id}</b></td>
              <td><b>${fmtData(c.data)}</b></td>
              <td>${esc(c.hora || '—')}</td>
              <td>${esc(c.local || 'Geral')}</td>
              <td>${esc(c.grupo_nome || '—')}</td>
              <td>${statusBadge}</td>
              <td class="num"><b>${c.lancados || 0}</b></td>
              <td>${esc(c.criado_por || '—')}</td>
              <td style="white-space:nowrap">
                <div style="display:flex; gap:6px">
                  <button type="button" class="acao-linha" data-pdfconf="${c.id}" style="font-size:12px; padding:4px 8px" title="Gerar e Baixar PDF">
                    📄 PDF
                  </button>
                  <button type="button" class="acao-linha bt-detalhe-conf" data-cid="${c.id}" style="font-size:12px; padding:4px 8px" title="Ver lista de lançamentos">
                    🔍 Detalhes
                  </button>
                </div>
              </td>
            </tr>
          `;
        }).join('');

        resEl.innerHTML = `
          <div class="cartao">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
              <h4 style="margin:0; font-size:13px; color:var(--tx2)">Lista de Conferências (Paginada de 10 em 10)</h4>
            </div>
            <div class="rolagem">
              <table>
                <thead>
                  <tr>
                    <th class="num">ID</th>
                    <th>Data</th>
                    <th>Hora/Turno</th>
                    <th>Local</th>
                    <th>Grupo</th>
                    <th>Status</th>
                    <th class="num">Lançados</th>
                    <th>Criado Por</th>
                    <th>Ações</th>
                  </tr>
                </thead>
                <tbody>${linhas}</tbody>
              </table>
            </div>
            ${renderPaginadorHTML(pag, 'pagConfs')}
          </div>
        `;

        resEl.querySelectorAll('.paginacao-btn[data-pg]').forEach(b => {
          b.onclick = () => {
            pgConfs = +b.dataset.pg;
            renderizarTabelaConfs();
          };
        });

        resEl.querySelectorAll('.bt-detalhe-conf').forEach(b => {
          b.onclick = () => abrirModalDetalhesConferencia(+b.dataset.cid);
        });

        resEl.querySelectorAll('[data-pdfconf]').forEach(b => {
          b.onclick = () => window.abrirModalPDFConferencia(b.dataset.pdfconf);
        });
      };

      const carregarConfs = async () => {
        const de = $('#cfDe').value;
        const ate = $('#cfAte').value;
        const status = $('#cfStatus').value;
        const grupo = $('#fGrupo') ? $('#fGrupo').value : '';

        $('#resultadoConfs').innerHTML = '<div class="carregando">Carregando conferências…</div>';
        try {
          let url = `/api/conferencias?de=${encodeURIComponent(de)}&ate=${encodeURIComponent(ate)}`;
          if (status === 'arquivada') url += '&arq=1';
          if (grupo) url += '&grupo=' + encodeURIComponent(grupo);

          const r = await api(url);
          listaConfs = Array.isArray(r) ? r : ((r && r.conferencias) || []);
          if (status === 'fechada') listaConfs = listaConfs.filter(x => x.status === 'fechada');
          else if (status === 'aberta') listaConfs = listaConfs.filter(x => x.status === 'aberta');
          
          pgConfs = 1;
          renderizarTabelaConfs();
        } catch (e) {
          $('#resultadoConfs').innerHTML = '<div class="cartao"><span class="vazio">Falha ao carregar conferências.</span></div>';
        }
      };

      $('#btFiltrarConfs').onclick = carregarConfs;
      $('#cfBuscaTxt').oninput = () => {
        pgConfs = 1;
        renderizarTabelaConfs();
      };

      carregarConfs();
    };

    /* Modal de Detalhes da Conferência Individual */
    const abrirModalDetalhesConferencia = async (confId) => {
      const m = modal(`
        <div class="modal-inner" style="max-width:720px">
          <div class="carregando">Carregando detalhes da conferência #${confId}…</div>
        </div>
      `);

      try {
        const r = await api('/api/conferencia/' + confId);
        const c = r.conferencia || r || {};
        const pres = r.presencas || r.lancamentos || [];

        let pgPres = 1;
        const inner = m.querySelector('.modal-inner');

        const renderModalPres = () => {
          const pag = paginarArray(pres, pgPres, 10);
          const linhas = pag.dados.map(p => `
            <tr>
              <td><b>${esc(p.nome_guerra)}</b></td>
              <td>${esc(p.funcao || '—')}</td>
              <td>${esc(p.setor || '—')}</td>
              <td>${pill(p.situacao)}</td>
              <td>${esc(p.destino || '—')}</td>
              <td>${esc(p.observacao || '—')}</td>
              <td class="num">${p.verificado ? '✅' : '❌'}</td>
            </tr>
          `).join('');

          inner.innerHTML = `
            <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:12px; border-bottom:1px solid var(--borda); padding-bottom:10px">
              <div>
                <h3 style="margin:0">Conferência #${c.id} — ${fmtData(c.data)}</h3>
                <p style="margin:4px 0 0; font-size:12.5px; color:var(--tx2)">
                  Local: <b>${esc(c.local || 'Geral')}</b> · Grupo: <b>${esc(c.grupo_nome || '—')}</b> · Status: <b>${esc(c.status)}</b>
                </p>
              </div>
              <button type="button" class="primario" data-pdfconf="${c.id}" style="font-size:12.5px; padding:6px 14px">📄 Baixar PDF</button>
            </div>

            <div class="rolagem" style="max-height:300px">
              <table>
                <thead>
                  <tr>
                    <th>Militar</th>
                    <th>Posto / Graduação</th>
                    <th>Setor</th>
                    <th>Situação</th>
                    <th>Destino</th>
                    <th>Observação</th>
                    <th class="num">Verif.</th>
                  </tr>
                </thead>
                <tbody>${linhas || '<tr><td colspan="7"><span class="vazio">sem lançamentos</span></td></tr>'}</tbody>
              </table>
            </div>

            ${renderPaginadorHTML(pag, 'pagModalPres')}

            <div class="modal-acoes" style="margin-top:14px">
              <button class="fantasma" id="btFecharDetConf">Fechar</button>
            </div>
          `;

          inner.querySelector('#btFecharDetConf').onclick = () => m.remove();
          inner.querySelectorAll('.paginacao-btn[data-pg]').forEach(b => {
            b.onclick = () => {
              pgPres = +b.dataset.pg;
              renderModalPres();
            };
          });
          inner.querySelectorAll('[data-pdfconf]').forEach(b => {
            b.onclick = () => window.abrirModalPDFConferencia(b.dataset.pdfconf);
          });
        };

        renderModalPres();
      } catch (e) {
        m.querySelector('.modal-inner').innerHTML = `
          <span class="vazio">Falha ao obter detalhes da conferência.</span>
          <div class="modal-acoes"><button class="fantasma" onclick="this.closest('.modal-mask').remove()">Fechar</button></div>
        `;
      }
    };

    /* =========================================================================
       3. ABA: BUSCA INDIVIDUAL POR MILITAR & TAGS
       ========================================================================= */
    const viewRelBuscaIndividual = async () => {
      let [tags, setores, funcoes] = await Promise.all([
        api('/api/relatorio/tags').catch(() => []),
        api('/api/catalogo/setores').catch(() => []),
        api('/api/catalogo/funcoes').catch(() => [])
      ]);
      const regras = [];

      const optSetores = (setores || []).filter(s => s.ativo !== 0);
      const optFuncoes = (funcoes || []).filter(f => f.ativo !== 0);

      const alvo = $('#corpoRelatorios');
      alvo.innerHTML = `
        <div class="cartao">
          <h3 style="margin-top:0">🔍 Busca Individual por Militar</h3>
          <div class="form-linha" style="align-items:end; margin-bottom:8px">
            <div class="campo" style="margin:0; flex:1">
              <label>Militar (Nome de Guerra ou Completo)</label>
              <input id="biNome" placeholder="Digite o nome de guerra…">
            </div>
            <div class="campo" style="margin:0; width:180px">
              <label>Seção / Setor</label>
              <select id="biSetor">
                <option value="">— Todas as Seções —</option>
                ${optSetores.map(s => `<option value="${s.id}">${esc(s.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="margin:0; width:180px">
              <label>Posto / Graduação</label>
              <select id="biFuncao">
                <option value="">— Todos os Postos/Grad. —</option>
                ${optFuncoes.map(f => `<option value="${f.id}">${esc(f.nome)}</option>`).join('')}
              </select>
            </div>
          </div>
          <div class="form-linha" style="align-items:end">
            <div class="campo" style="margin:0; flex:1">
              <label>De</label>
              <input type="date" id="biDe" value="${dataLocal(new Date(Date.now() - 29 * 864e5))}">
            </div>
            <div class="campo" style="margin:0; flex:1">
              <label>Até</label>
              <input type="date" id="biAte" value="${dataLocal(hoje)}">
            </div>
            <button class="primario" id="biIr" style="min-height:44px; padding:0 24px">Buscar</button>
          </div>
          <div id="biRes" style="margin-top:12px"></div>

          <details style="margin-top:14px">
            <summary style="cursor:pointer;color:var(--tx2);font-size:13px; font-weight:600">
              🏷️ Filtros Cruzados por TAG (Multi-seleção em OU)
            </summary>
            <div id="biFiltros" style="margin-top:8px"></div>
            <div style="display:flex;gap:8px;margin-top:8px">
              <button type="button" id="biAddF" style="min-height:36px">+ Adicionar Filtro</button>
              <button type="button" class="primario" id="biAplicarF" style="min-height:36px">Aplicar Filtros</button>
            </div>
            <div id="biFRes" style="margin-top:10px"></div>
          </details>
        </div>
      `;

      const rotuloSit = s => ({ presente: 'Presente', atraso: 'Atraso', falta: 'Falta', justificada: 'Justificada', nao_verificado: 'NÃO VERIFICADO' })[s] || s;

      const abrirFicha = async (pessoaId, listaRegs) => {
        let ficha = {};
        try { ficha = await api('/api/pessoas/' + pessoaId + '/ficha'); } catch (e) {}
        const div = modal(`
          <div class="modal-inner" style="max-width:640px">
            <h3>${esc(ficha.nome_guerra || '')} <small style="color:var(--tx2);font-weight:400">${esc(ficha.nome_completo || '')}</small></h3>
            <p style="color:var(--tx2);font-size:13px;margin:4px 0">Setor: <b>${esc(ficha.setor || 'INDEFINIDO')}</b> · Posto/Grad.: <b>${esc(ficha.funcao || 'INDEFINIDO')}</b> · Grupo: <b>${esc(ficha.grupo || '—')}</b> · Status: <b>${esc(ficha.status || '')}</b></p>
            <div class="rolagem" style="max-height:220px;margin:8px 0">
              <table>
                <thead><tr><th>Data</th><th>Conf.</th><th>Situação</th><th>Destino</th><th>Tag</th></tr></thead>
                <tbody>${(listaRegs || []).map(r => `<tr><td>${fmtData(r.data)}</td><td>#${r.conferencia_id}</td><td><b>${esc(r.situacao_rotulo || r.situacao || '')}</b></td><td>${esc(r.destino || '')}</td><td>${esc(r.tags || '')}</td></tr>`).join('') || '<tr><td colspan="5"><span class="vazio">sem registros</span></td></tr>'}</tbody>
              </table>
            </div>
            <div class="cartao" style="margin:0">
              <h4 style="margin:0 0 6px">Adicionar Comentário com TAG</h4>
              <div class="campo"><label>Tag</label><select id="mcTag"><option value="">— sem tag —</option>${tags.map(t => `<option value="${t.id}">${esc(t.nome)}</option>`).join('')}</select></div>
              <div class="campo"><label>Comentário</label><textarea id="mcTxt" rows="2"></textarea></div>
              <button class="primario" id="mcGo" style="min-height:38px">Gravar Comentário</button>
            </div>
            <div id="historicoComentariosBox" style="margin-top:14px; display:none">
              <h4 style="margin:0 0 6px">Histórico de Comentários</h4>
              <div id="hcLista" class="rolagem" style="max-height:200px"></div>
            </div>
            <div class="modal-acoes" style="justify-content:space-between">
              <button class="acao-linha" id="btnVerComentarios">Ver Histórico de Comentários</button>
              <button class="fantasma" id="mcX">Fechar</button>
            </div>
          </div>`);
        if (!div) return;
        div.querySelector('#mcX').onclick = () => div.remove();
        div.querySelector('#btnVerComentarios').onclick = async () => {
          const btn = div.querySelector('#btnVerComentarios');
          const box = div.querySelector('#historicoComentariosBox');
          const listaDiv = div.querySelector('#hcLista');
          if (box.style.display === 'block') {
            box.style.display = 'none';
            btn.textContent = 'Ver Histórico de Comentários';
            return;
          }
          btn.textContent = 'Ocultar Comentários';
          box.style.display = 'block';
          listaDiv.innerHTML = '<span class="vazio">carregando...</span>';
          try {
            const coms = await api('/api/pessoas/' + pessoaId + '/comentarios');
            listaDiv.innerHTML = coms.length ? coms.map(c => 
              `<div class="cartao" style="padding:8px;margin-bottom:6px">
                <small style="color:var(--tx2)">Data: ${(c.datahora || '').slice(0, 16).replace('T', ' ')} · Conf #${c.conferencia_id} · Op: ${esc(c.operador)} ${c.tag ? `· Tag: <b>${esc(c.tag)}</b>` : ''}</small>
                <div style="margin-top:4px">${esc(c.comentario)}</div>
              </div>`
            ).join('') : '<span class="vazio">Nenhum comentário registrado.</span>';
          } catch (e) {
            listaDiv.innerHTML = '<span class="vazio">Erro ao carregar comentários.</span>';
          }
        };
        div.querySelector('#mcGo').onclick = async () => {
          const txt = div.querySelector('#mcTxt').value.trim();
          if (!txt) { toast('Escreva o comentário', 'erro'); return; }
          const tagVal = div.querySelector('#mcTag').value;
          try {
            await api('/api/comentarios', { method: 'POST', body: JSON.stringify({
              conferencia_id: parseInt((listaRegs && listaRegs[0] && listaRegs[0].conferencia_id) || 0),
              pessoa_id: pessoaId, comentario: txt,
              tag_id: tagVal ? parseInt(tagVal) : null }) });
            toast('Comentário gravado');
            div.remove();
          } catch (e) {}
        };
      };

      const carregarPessoaRegs = async (pid) => {
        const de = $('#biDe').value, ate = $('#biAte').value;
        try {
          const r = await api(`/api/relatorio/registros?pessoa=${pid}&de=${encodeURIComponent(de)}&ate=${encodeURIComponent(ate)}`);
          const regs = (r.registros || []).map(x => ({ ...x, situacao_rotulo: rotuloSit(x.situacao) }));
          
          let pgBi = 1;
          const renderBiRegs = () => {
            const pag = paginarArray(regs, pgBi, 10);
            const linhas = pag.dados.map((rg, i) => `
              <tr data-bi="${(pgBi-1)*10 + i}" style="cursor:pointer">
                <td>${fmtData(rg.data)}</td>
                <td>#${rg.conferencia_id}</td>
                <td><b>${esc(rg.situacao_rotulo)}</b></td>
                <td>${esc(rg.destino || '—')}</td>
                <td>${esc(rg.tags || '—')}</td>
              </tr>
            `).join('');

            $('#biRes').innerHTML = `
              <div class="cartao" style="margin:0">
                <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
                  <h4 style="margin:0">${esc(r.pessoa && r.pessoa.nome_guerra || '')} — ${regs.length} registro(s)</h4>
                  <button type="button" class="primario" id="btAbrirFichaBi" style="font-size:12px; padding:4px 10px">📝 Abrir Ficha / Comentar</button>
                </div>
                <div class="rolagem">
                  <table>
                    <thead><tr><th>Data</th><th>Conf.</th><th>Situação</th><th>Destino</th><th>Tag</th></tr></thead>
                    <tbody>${linhas || '<tr><td colspan="5"><span class="vazio">sem registros no período</span></td></tr>'}</tbody>
                  </table>
                </div>
                ${renderPaginadorHTML(pag, 'pagBiRegs')}
              </div>
            `;

            $('#btAbrirFichaBi').onclick = () => abrirFicha(pid, regs);
            document.querySelectorAll('#biRes tr[data-bi]').forEach(tr => {
              tr.onclick = () => abrirFicha(pid, regs);
            });
            document.querySelectorAll('#biRes .paginacao-btn[data-pg]').forEach(b => {
              b.onclick = () => {
                pgBi = +b.dataset.pg;
                renderBiRegs();
              };
            });
          };

          renderBiRegs();
        } catch (e) {
          $('#biRes').innerHTML = '<span class="vazio">falha na busca</span>';
        }
      };

      const buscarPessoa = async () => {
        const q = ($('#biNome').value || '').trim().toLowerCase();
        const sid = $('#biSetor') ? $('#biSetor').value : '';
        const fid = $('#biFuncao') ? $('#biFuncao').value : '';

        if (!q && !sid && !fid) { toast('Informe o nome, setor ou posto/graduação para buscar', 'erro'); return; }
        $('#biRes').innerHTML = '<div class="carregando">Buscando…</div>';
        try {
          const ps = await api('/api/pessoas');
          const todas = (ps.pessoas || []);
          const cand = todas.filter(x => {
            const matchNome = !q || (x.nome_guerra || '').toLowerCase().includes(q) || (x.nome_completo || '').toLowerCase().includes(q);
            const matchSetor = !sid || String(x.setor_id) === String(sid);
            const matchFuncao = !fid || String(x.funcao_id) === String(fid);
            return matchNome && matchSetor && matchFuncao && (x.status || 'ativo') === 'ativo';
          });
          if (!cand.length) {
            $('#biRes').innerHTML = '<span class="vazio">Nenhum militar encontrado com os filtros informados.</span>';
            return;
          }
          if (cand.length > 1) {
            $('#biRes').innerHTML = `
              <div class="cartao" style="margin:0">
                <h4 style="margin:0 0 6px">${cand.length} militar(es) encontrado(s) — clique para ver o histórico:</h4>
                <div style="display:flex; gap:6px; flex-wrap:wrap">
                  ${cand.map((x, i) => `<button type="button" class="acao-linha" data-pcand="${i}"><b>${esc(x.nome_guerra)}</b> <small style="color:var(--tx3)">(${esc(x.setor || '—')} · ${esc(x.funcao || '—')})</small></button>`).join('')}
                </div>
              </div>
            `;
            document.querySelectorAll('#biRes [data-pcand]').forEach(b => b.onclick = () => carregarPessoaRegs(cand[+b.dataset.pcand].id));
            return;
          }
          carregarPessoaRegs(cand[0].id);
        } catch (e) {
          $('#biRes').innerHTML = '<span class="vazio">Falha na busca.</span>';
        }
      };

      $('#biIr').onclick = buscarPessoa;
      $('#biNome').addEventListener('keydown', ev => { if (ev.key === 'Enter') buscarPessoa(); });

      const pintaFiltros = () => {
        $('#biFiltros').innerHTML = regras.map((rg, i) => `
          <div class="form-linha" data-fid="${i}" style="align-items:end;margin-bottom:4px">
            <div class="campo"><label>Tag</label><select data-c="tag">${tags.map(t => `<option value="${t.id}" ${rg.tag == t.id ? 'selected' : ''}>${esc(t.nome)}</option>`).join('')}</select></div>
            <div class="campo"><label>De</label><input type="date" data-c="de" value="${rg.de || ''}"></div>
            <div class="campo"><label>Até</label><input type="date" data-c="ate" value="${rg.ate || ''}"></div>
            <button type="button" data-rm="${i}" style="min-height:36px">✕</button></div>`).join('') ||
          '<span class="vazio">nenhum filtro — adicione regras TAG × período (o resultado é a UNIÃO de todas)</span>';
        document.querySelectorAll('#biFiltros [data-rm]').forEach(b => b.onclick = () => { regras.splice(+b.dataset.rm, 1); pintaFiltros(); });
        document.querySelectorAll('#biFiltros .form-linha').forEach(linha => {
          const i = +linha.dataset.fid;
          linha.querySelector('[data-c=tag]').onchange = e => regras[i].tag = +e.target.value;
          linha.querySelector('[data-c=de]').onchange = e => regras[i].de = e.target.value;
          linha.querySelector('[data-c=ate]').onchange = e => regras[i].ate = e.target.value;
        });
      };
      $('#biAddF').onclick = () => {
        regras.push({ tag: tags[0] ? tags[0].id : 0, de: dataLocal(new Date(Date.now() - 6 * 864e5)), ate: dataLocal(hoje) });
        pintaFiltros();
      };
      $('#biAplicarF').onclick = async () => {
        if (!regras.length) { toast('Adicione pelo menos um filtro', 'erro'); return; }
        $('#biFRes').innerHTML = '<div class="carregando">Aplicando filtros…</div>';
        try {
          const r = await api('/api/relatorio/registros?filtros=' + encodeURIComponent(JSON.stringify(regras)));
          const regs = r.registros || [];
          
          let pgFiltros = 1;
          const renderFiltrosRegs = () => {
            const pag = paginarArray(regs, pgFiltros, 10);
            const linhas = pag.dados.map((rg, i) => `
              <tr data-bf="${(pgFiltros-1)*10 + i}" style="cursor:pointer">
                <td>${fmtData(rg.data)}</td>
                <td>#${rg.conferencia_id}</td>
                <td><b>${esc(rg.tag)}</b></td>
                <td>${esc(rg.pessoa)}</td>
                <td>${esc(rg.comentario)}</td>
              </tr>
            `).join('');

            $('#biFRes').innerHTML = `
              <div class="cartao" style="margin:0">
                <h4 style="margin:0 0 6px">Resultado dos filtros — ${regs.length} comentário(s) taggeado(s)</h4>
                <div class="rolagem"><table><thead><tr><th>Data</th><th>Conf.</th><th>Tag</th><th>Militar</th><th>Comentário</th></tr></thead>
                <tbody>${linhas || '<tr><td colspan="5"><span class="vazio">nada encontrado</span></td></tr>'}</tbody></table></div>
                ${renderPaginadorHTML(pag, 'pagFiltrosRegs')}
              </div>
            `;

            document.querySelectorAll('#biFRes .paginacao-btn[data-pg]').forEach(b => {
              b.onclick = () => {
                pgFiltros = +b.dataset.pg;
                renderFiltrosRegs();
              };
            });
          };

          renderFiltrosRegs();
        } catch (e) {
          $('#biFRes').innerHTML = '<span class="vazio">falha nos filtros</span>';
        }
      };
    };

    renderTela();
  };
})();
