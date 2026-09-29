/* SCI — Tela de CONFERÊNCIA (#/hoje) e RELATÓRIOS (#/relatorios).
   Consome os helpers globais do core (api, esc, toast, fmtData, fmtHora,
   pill, abrirModal, confirmar, navAtiva) e as classes do protótipo. v200 */
'use strict';
(() => {
  const $ = s => document.querySelector(s);
  const SITUACOES = ['presente', 'atraso', 'falta', 'justificada'];
  const ROTULO = { presente: 'Presente', atraso: 'Atraso', falta: 'Falta', justificada: 'Justificada' };

  /* estado da conferência em curso (RAM, como no protótipo — a fonte de verdade
     é o servidor: estados só vão para o banco no FECHAR) */
  let C = null;

  /* abrirModal pode ou não devolver a raiz; garantir um nó consultável */
  const modalRaiz = html => {
    const m = abrirModal(html);
    if (m && m.querySelector) return m;
    const ms = document.querySelectorAll('.modal-mask');
    return ms.length ? ms[ms.length - 1] : document;
  };
  const fecharModal = raiz => { if (raiz && raiz.remove) raiz.remove(); };

  /* ================================================================
     #/hoje — CONFERÊNCIA DE PESSOAL (gerente/operador)
     ================================================================ */
  /* --- salvamento parcial (v9.13): grava o estado de 1 militar na conferência aberta --- */
  let CONF_ID = null; // conferência aberta sendo editada (várias simultâneas v9.14.2)
  let marcaTimer = {}, marcaPend = {};
  const marcarParcial = (pid, situacao, destinoId, observacao, verificado) => {
    if (!C.c) return;
    marcaPend[pid] = { situacao: situacao || null, destino_id: destinoId ?? null, observacao: observacao ?? null, verificado: !!verificado };
    clearTimeout(marcaTimer[pid]);
    marcaTimer[pid] = setTimeout(async () => {
      const corpo = marcaPend[pid];
      delete marcaPend[pid];
      try { await api('/api/conferencia/marcar' + (CONF_ID ? '?id=' + CONF_ID : ''), { method: 'POST', body: JSON.stringify({ pessoa_id: pid, ...corpo }) }); }
      catch (e) { toast('Falha ao salvar estado parcial', 'erro'); }
    }, 350);
  };
  /* --- LISTAS (v9.14): #/hoje mostra SÓ as listas de conferências; a conferência
     em si fica em #/conferencia (botão Abrir). Abertas editáveis; fechadas = PDF. --- */
  window.ViewHoje = async function () {
    navAtiva('#/hoje');
    $('#app').innerHTML = '<div class="carregando">Carregando conferências…</div>';
    let lista = [];
    try { lista = await api('/api/conferencia/lista'); } catch (e) { lista = []; }
    let haAberta = null;
    try { const d = await api('/api/conferencia/hoje'); haAberta = d.conferencia || null; } catch (e) {}
    const linha = c => `
      <tr data-cid="${c.id}"><td class="num"><b>#${c.id}</b></td>
      <td>${c.status === 'aberta' ? 'Aberta' : 'Fechada'}</td>
      <td>${c.status === 'aberta' ? fmtHora(c.criada_em) : fmtHora(c.fechada_em)}</td>
      <td>${fmtData(c.data)}</td>
      <td>${esc(c.grupo || '—')}</td>
      <td>${esc(c.criado_por || '—')}</td>
      <td class="num">${c.lancamentos}</td>
      <td>${c.status === 'fechada'
        ? `<a href="/api/conferencia/${c.id}/relatorio.pdf?t=${Date.now()}" target="_blank"><button class="primario" style="min-height:36px;padding:8px 12px">Relatório PDF</button></a>`
        : `<button class="primario" data-abrir="${c.id}" style="min-height:36px;padding:8px 12px">Abrir</button>`}</td></tr>`;
    const porData = (a, b) => String(b.data || '').localeCompare(String(a.data || '')) || b.id - a.id;
    const abertas = lista.filter(c => c.status === 'aberta').sort(porData);
    const fechadas = lista.filter(c => c.status === 'fechada').sort(porData);
    const tabela = (titulo, itens) => `
      <h3 style="margin:14px 0 8px">${titulo} (${itens.length})</h3>
      <div class="cartao"><div class="rolagem"><table>
      <thead><tr><th class="num">ID</th><th>Status</th><th>Horário</th><th>Data</th><th>Grupo</th><th>Operador</th><th class="num">Lanç.</th><th>Ações</th></tr></thead>
      <tbody>${itens.map(linha).join('') || '<tr><td colspan="8"><span class="vazio">nenhuma</span></td></tr>'}</tbody></table></div></div>`;
    $('#app').innerHTML = `<h2>Conferências</h2>
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:6px">
        <button class="primario" id="btNovaConf" style="min-height:44px">▶ Nova conferência</button>
        <span style="color:var(--tx2);font-size:12px">abertas podem ser editadas · várias simultâneas · fechadas viram relatório (PDF)</span></div>
      <div class="cartao" style="margin-bottom:10px"><div class="campo" style="margin:0"><label>Pesquisar por ID da conferência</label><input id="fConfID" placeholder="ex.: 3"></div></div>` +
      tabela('Abertas', abertas) + tabela('Fechadas', fechadas) +
      `<p style="color:var(--tx2);font-size:12px">O relatório PDF só é gerado para conferências fechadas.</p>`;
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
    $('#fConfID').oninput = () => {
      const q = $('#fConfID').value.trim().replace('#', '');
      document.querySelectorAll('#app tr[data-cid]').forEach(tr => {
        tr.style.display = !q || tr.dataset.cid === q ? '' : 'none';
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
    C = { c: d.conferencia, pessoas: d.pessoas || [], destinos, est, dest, obs, verif, temComentario };
    confRender();
    $('#btVoltar').onclick = () => { location.hash = '#/hoje'; };
    $('#btDescartar').onclick = async () => {
      if (!(await confirmar(`DESCARTAR a conferência #${C.c.id}? O estado parcial gravado será apagado. Esta ação não pode ser desfeita.`))) return;
      try {
        await api('/api/conferencia/' + C.c.id, { method: 'DELETE' });
        toast('Conferência descartada');
        location.hash = '#/hoje';
        location.reload();
      } catch (e) {}
    };
  };

  function confRender(filtro = '') {
    const semC = !C.c;
    const f = (filtro || '').trim().toLowerCase();
    const porSetor = {};
    C.pessoas
      .filter(p => !f || (p.nome_guerra || '').toLowerCase().includes(f) || (p.nome_completo || '').toLowerCase().includes(f))
      .forEach(p => { (porSetor[p.setor || 'Sem setor'] = porSetor[p.setor || 'Sem setor'] || []).push(p); });
    // ordem dentro do setor (v9.14.1): sem check primeiro, depois alfabética
    const ordemCheck = (a, b) => (C.verif.has(a.id) - C.verif.has(b.id))
      || (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt', { sensitivity: 'base' });
    for (const s of Object.keys(porSetor)) porSetor[s].sort(ordemCheck);
    let listas = '';
    for (const setor of Object.keys(porSetor).sort()) {
      listas += `<div class="grupo-setor"><h4>${esc(setor)} · ${porSetor[setor].length}</h4><div class="lista-pessoa">` +
        porSetor[setor].map(p => {
          const sit = C.est[p.id] || 'presente';
          const selDest = sit === 'justificada'
            ? `<select class="sel-destino" data-id="${p.id}"><option value="">destino…</option>` +
              C.destinos.map(dx => `<option value="${dx.id}" ${C.dest[p.id] == dx.id ? 'selected' : ''}>${esc(dx.nome)}</option>`).join('') + '</select>'
            : '';
          const optSit = s => `<option value="${s}" ${sit === s ? 'selected' : ''}>${ROTULO[s]}</option>`;
          return `<div class="pessoa ${C.verif.has(p.id) ? 'verificado' : ''}" data-id="${p.id}">
            <input type="checkbox" class="chk" data-id="${p.id}" ${C.verif.has(p.id) ? 'checked' : ''} title="verifiquei esta pessoa">
            <span class="nome"><b>${esc(p.nome_guerra)}</b><small>${esc(p.nome_completo)}${p.funcao ? ' · ' + esc(p.funcao) : ''}${C.obs[p.id] ? ' · 📝' : ''}${C.temComentario[p.id] ? ' · 💬' : ''}</small></span>
            <select class="sel-situacao" data-id="${p.id}" title="situação">${SITUACOES.map(optSit).join('')}</select>
            ${selDest}<button type="button" class="fantasma bt-coment" data-id="${p.id}" title="comentários" style="min-height:36px;padding:4px 8px">💬</button></div>`;
        }).join('') + '</div></div>';
    }
    const banner = semC
      ? `<div class="cartao"><p style="color:var(--tx2)">Nenhuma conferência aberta. Ao iniciar, a data e o horário de Brasília são registrados automaticamente.</p>
         <div style="display:flex;gap:8px;align-items:end;margin-top:10px">
           <button class="primario" id="btIniciar" style="min-height:44px">▶ Iniciar conferência</button></div></div>`
      : `<div class="cartao">
         <span>${pill('aberta')} <b>Conferência #${C.c.id}</b> · aberta em ${fmtData(C.c.data)} às ${fmtHora(C.c.criada_em)}${C.c.local ? ' · ' + esc(C.c.local) : ''}</span></div>`;
    $('#app').innerHTML = `<div style="margin-bottom:10px"><button class="fantasma" id="btVoltar" style="min-height:38px">← Retornar</button></div>
      <h2 style="margin-top:0">Conferência de pessoal</h2>${banner}
      <div class="barra-fixa">
        <input id="busca" placeholder="buscar nome…">
        <button class="primario" id="btFecharBarra">✕ FECHAR CONFERÊNCIA</button>
      </div>
      <div id="lista">${listas}</div>
      <div style="display:flex;justify-content:flex-end;margin-top:28px;padding-top:14px;border-top:1px solid var(--borda)">
        <button class="perigo" id="btDescartar" style="min-height:40px">🗑 Descartar conferência</button>
      </div>`;
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
      $('#btFecharBarra').onclick = confFechar;
    };
    atualizar();
    /* situação por DROP-DOWN (não cíclico) — falta/justificada abre modal */
    document.querySelectorAll('.sel-situacao').forEach(s => s.onchange = () => {
      const id = +s.dataset.id;
      const novo = s.value;
      const atual = C.est[id] || 'presente';
      if (novo === atual) return;
      C.est[id] = novo;
      C.verif.add(id);
      if (novo === 'falta' || novo === 'justificada') confModalLancamento(id, novo, () => confRender($('#busca') ? $('#busca').value : ''));
      else { marcarParcial(id, novo, C.dest[id] ?? null, C.obs[id] ?? null, true); confRender($('#busca').value); }
    });
    document.querySelectorAll('.pessoa .chk').forEach(ch => ch.onchange = () => {
      const id = +ch.dataset.id;
      if (ch.checked) C.verif.add(id); else C.verif.delete(id);
      ch.closest('.pessoa').classList.toggle('verificado', ch.checked);
      atualizar();
      // salvamento parcial: check grava o estado atual (ou presente) daquele nome + verificado
      if (ch.checked) marcarParcial(id, C.est[id] || 'presente', C.dest[id] ?? null, C.obs[id] ?? null, true);
    });
    document.querySelectorAll('.sel-destino').forEach(s => s.onchange = () => { C.dest[+s.dataset.id] = +s.value || null; });
    document.querySelectorAll('.bt-coment').forEach(b => b.onclick = ev => { ev.stopPropagation(); confModalComentarios(+b.dataset.id); });
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
        raiz.querySelector('#cmLista').innerHTML = meus.length ? meus.map(c =>
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
      C.verif.add(id);
      marcarParcial(id, sit, destino, obs, true); // salvamento parcial imediato (já verificado)
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
    if (naoVerif.length) msg = `ATENÇÃO: ${naoVerif.length} sem verificação (${naoVerif.join(', ')}).\n\n` + msg;
    if (!(await confirmar(msg))) return;
    const lanc = C.pessoas.map(p => ({
      pessoa_id: p.id,
      situacao: C.est[p.id] || 'presente',
      destino_id: C.dest[p.id] || null,
      observacao: C.obs[p.id] || null
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
      <div class="caixa"><b>${totalFaltas}</b><span>faltas tot. (J+NJ)</span></div>
      <div class="caixa" style="border-color:var(--verde)"><b>${b.pct_pronto}%</b><span>ef. pronto</span></div></div>`;
    /* ordem do Tenente: efetivo em ORDEM ALFABÉTICA (backend pode vir agrupado
       por função — o前端 garante a ordem e a antiguidade é o posto alfabético) */
    const pessoas = (b.pessoas || []).slice()
      .sort((a, x) => String(a.nome_guerra || '').localeCompare(String(x.nome_guerra || ''), 'pt', { sensitivity: 'base' }));
    const linhas = pessoas.map(p =>
      `<tr><td class="num">${p.antiguidade ?? ''}</td><td>${esc(p.funcao || '—')}</td><td><b>${esc(p.nome_guerra)}</b></td>
       <td>${esc(p.setor)}</td><td>${esc(p.grupo || '—')}</td><td class="num">${p.presencas}</td>
       <td class="num">${p.atrasos}</td><td class="num">${p.faltas}</td><td class="num">${p.justificadas}</td></tr>`).join('');
    const forms = (b.formaturas || []).map(f =>
      `<tr><td>${fmtData(f.data)}</td><td>${esc(f.tipo)}</td><td>${esc(f.hora || '—')}</td>
       <td>${f.status === 'fechada' ? 'Fechada' : 'Aberta'}</td>
       <td class="num">${f.presentes}</td><td class="num">${f.faltas}</td></tr>`).join('');
    return `${res}<div class="cartao"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
      <h3 style="margin:0">${titulo} — ${b.convocacoes} conferências no período</h3>
      <a href="/api/relatorio.pdf?de=${encodeURIComponent(b.De)}&ate=${encodeURIComponent(b.Ate)}${grupoQ}&t=${Date.now()}" target="_blank"><button class="primario">ABRIR PDF</button></a></div>
      ${forms ? `<div class="rolagem" style="margin-bottom:12px"><table><thead><tr><th>Data</th><th>Tipo</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead><tbody>${forms}</tbody></table></div>` : ''}
      <div class="rolagem"><table><thead><tr><th class="num">ORD</th><th>Função</th><th>Nome</th><th>Setor</th><th>Grupo</th><th class="num">Pres.</th><th class="num">Atraso</th>
      <th class="num">Falta</th><th class="num">Just.</th></tr></thead><tbody>${linhas || '<tr><td colspan="9"><span class="vazio">sem efetivo ativo no escopo</span></td></tr>'}</tbody></table></div></div>`;
  }

  window.ViewRelatorios = async function () {
    navAtiva('#/relatorios');
    $('#app').innerHTML = '<div class="carregando">…</div>';
    let me = window.ME;
    if (!me || !me.papel) {
      try { me = (await api('/api/me')).usuario; } catch (e) { me = {}; }
    }
    const hoje = new Date();
    let gSel = '';
    try {
      const gs = await api('/api/grupos');
      if (me.papel === 'admin') {
        gSel = `<div class="campo"><label>Grupo</label><select id="fGrupo"><option value="">Todos</option>` +
          gs.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('') + `</select></div>`;
      } else {
        /* escopo: Meu grupo + subordinados / Meu grupo (somente) / cada subordinado */
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
    } catch (e) {}
    $('#app').innerHTML = `<h2>Relatórios</h2>
      <div class="cartao">
        ${gSel ? `<div class="form-linha" style="margin-bottom:8px">${gSel}</div>` : ''}
        <div class="abas" id="modos">
          <button data-m="dia" class="ativo">Dia</button>
          <button data-m="semana">Semana</button>
          <button data-m="ano">Ano</button>
          <button data-m="livre">Período livre</button></div>
        <div class="form-linha" style="grid-template-columns:1fr auto;align-items:end">
          <div id="entrada"></div>
          <button class="primario" id="btGerar" style="min-height:44px">Gerar</button></div></div>
      <div id="estadoAtual"><div class="carregando">Carregando estado atual do efetivo…</div></div>
      <div id="saida"><div class="carregando">Gerando dia atual…</div></div>`;
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
    /* v9.15: estado ATUAL do efetivo no topo do dashboard de relatórios */
    (async () => {
      try {
        const html = await window.EfetivoAtualHTML();
        const alvo = document.querySelector('#estadoAtual');
        if (alvo) alvo.innerHTML = html;
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
})();
