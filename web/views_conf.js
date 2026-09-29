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
  window.ViewHoje = async function () {
    navAtiva('#/hoje');
    $('#app').innerHTML = '<div class="carregando">Carregando efetivo…</div>';
    const d = await api('/api/conferencia/hoje');
    let destinos = [];
    try { destinos = await api('/api/catalogo/destinos'); } catch (e) { destinos = []; }
    destinos = destinos.filter(x => x.ativo === 1 || x.ativo === true);
    const est = {}, dest = {}, obs = {}, verif = new Set(), temComentario = {};
    if (d.conferencia) {
      for (const [pid, v] of Object.entries(d.conferencia.estados || {})) {
        est[pid] = v.situacao;
        dest[pid] = v.destino_id;
        if (v.observacao) obs[pid] = v.observacao;
        verif.add(+pid);
      }
    }
    C = { c: d.conferencia, pessoas: d.pessoas || [], destinos, est, dest, obs, verif, temComentario };
    confRender();
  };

  function confRender(filtro = '') {
    const semC = !C.c;
    const f = (filtro || '').trim().toLowerCase();
    const porSetor = {};
    C.pessoas
      .filter(p => !f || (p.nome_guerra || '').toLowerCase().includes(f) || (p.nome_completo || '').toLowerCase().includes(f))
      .forEach(p => { (porSetor[p.setor || 'Sem setor'] = porSetor[p.setor || 'Sem setor'] || []).push(p); });
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
      : `<div class="cartao" style="display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap">
         <span>${pill('aberta')} <b>Conferência #${C.c.id}</b> · aberta em ${fmtData(C.c.data)} às ${fmtHora(C.c.criada_em)}${C.c.local ? ' · ' + esc(C.c.local) : ''} · situação pelo menu de cada militar, ✅ para verificar</span>
         <button class="perigo" id="btFechar">✕ Fechar conferência</button></div>`;
    $('#app').innerHTML = `<h2>Conferência de pessoal</h2>${banner}
      ${semC ? '' : `<div class="barra-fixa">
        <input id="busca" placeholder="buscar nome…">
        <button class="primario" id="btFecharBarra">✕ FECHAR CONFERÊNCIA</button>
      </div>`}
      <div id="lista">${semC ? '' : listas}</div>
      <div id="listaConf"><div class="carregando">…</div></div>`;
    if (semC) {
      $('#btIniciar').onclick = async () => {
        try {
          await api('/api/conferencia/iniciar', { method: 'POST', body: JSON.stringify({}) });
          toast('Conferência iniciada — data e hora de Brasília registradas');
          window.ViewHoje();
        } catch (err) {}
      };
      confHistorico();
      return;
    }
    const contSpan = () => `<b>${C.verif.size}/${C.pessoas.length}</b> verificados`;
    const atualizar = () => {
      const barra = document.querySelector('.barra-fixa');
      if (!barra) return;
      barra.innerHTML = `<input id="busca" placeholder="buscar nome…" value="${esc(f)}">` +
        `<span class="cont">${contSpan()}</span>` +
        `<button class="perigo" id="btFecharBarra">✕ FECHAR CONFERÊNCIA</button>`;
      ligarBarra();
    };
    const ligarBarra = () => {
      $('#busca').oninput = ev => confRender(ev.target.value);
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
      C.verif.delete(id);
      if (novo === 'falta' || novo === 'justificada') confModalLancamento(id, novo, () => confRender($('#busca') ? $('#busca').value : ''));
      else confRender($('#busca').value);
    });
    document.querySelectorAll('.pessoa .chk').forEach(ch => ch.onchange = () => {
      const id = +ch.dataset.id;
      if (ch.checked) C.verif.add(id); else C.verif.delete(id);
      ch.closest('.pessoa').classList.toggle('verificado', ch.checked);
      atualizar();
    });
    document.querySelectorAll('.sel-destino').forEach(s => s.onchange = () => { C.dest[+s.dataset.id] = +s.value || null; });
    document.querySelectorAll('.bt-coment').forEach(b => b.onclick = ev => { ev.stopPropagation(); confModalComentarios(+b.dataset.id); });
    $('#btFechar').onclick = confFechar;
    confHistorico();
  }

  /* --- histórico embutido: filtro por ID + tabelas Abertas/Fechadas --- */
  async function confHistorico() {
    const alvo = $('#listaConf');
    if (!alvo) return;
    let lista = [];
    try { lista = await api('/api/conferencia/lista'); }
    catch (e) { alvo.innerHTML = ''; return; }
    const linha = c => `
      <tr data-cid="${c.id}"><td class="num"><b>#${c.id}</b></td>
      <td>${c.status === 'aberta' ? 'Aberta' : 'Fechada'}</td>
      <td>${c.status === 'aberta' ? fmtHora(c.criada_em) : fmtHora(c.fechada_em)}</td>
      <td>${fmtData(c.data)}</td>
      <td>${esc(c.grupo || '—')}</td>
      <td>${esc(c.criado_por || '—')}</td>
      <td class="num">${c.lancamentos}</td>
      <td>${c.status === 'fechada'
        ? `<a href="/api/conferencia/${c.id}/relatorio.pdf" target="_blank"><button class="primario" style="min-height:36px;padding:8px 12px">Relatório PDF</button></a>`
        : `<span style="color:var(--ambar);font-size:13px">em andamento</span>`}</td></tr>`;
    const porData = (a, b) => String(b.data || '').localeCompare(String(a.data || '')) || b.id - a.id;
    const abertas = lista.filter(c => c.status === 'aberta').sort(porData);
    const fechadas = lista.filter(c => c.status === 'fechada').sort(porData);
    const tabela = (titulo, itens) => `
      <h3 style="margin:14px 0 8px">${titulo} (${itens.length})</h3>
      <div class="cartao"><div class="rolagem"><table>
      <thead><tr><th class="num">ID</th><th>Status</th><th>Horário</th><th>Data</th><th>Grupo</th><th>Operador</th><th class="num">Lanç.</th><th>Relatório</th></tr></thead>
      <tbody>${itens.map(linha).join('') || '<tr><td colspan="8"><span class="vazio">nenhuma</span></td></tr>'}</tbody></table></div></div>`;
    alvo.innerHTML = `<h2 style="margin-top:22px">Histórico de conferências</h2>
      <div class="cartao" style="margin-bottom:10px"><div class="campo" style="margin:0"><label>Pesquisar por ID da conferência</label><input id="fConfID" placeholder="ex.: 3"></div></div>` +
      tabela('Abertas', abertas) + tabela('Fechadas', fechadas) +
      `<p style="color:var(--tx2);font-size:12px">O relatório PDF só é gerado para conferências fechadas.</p>`;
    $('#fConfID').oninput = () => {
      const q = $('#fConfID').value.trim().replace('#', '');
      document.querySelectorAll('#listaConf tr[data-cid]').forEach(tr => {
        tr.style.display = !q || tr.dataset.cid === q ? '' : 'none';
      });
    };
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
      location.hash = '#/hoje';
      location.reload(); // ordem do Tenente: fechar = recarga completa da página
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
      <div class="caixa"><b>${b.convocacoes}</b><span>conferências</span></div>
      <div class="caixa"><b>${b.efetivo_ativo}</b><span>efetivo</span></div>
      <div class="caixa"><b>${b.presentes}</b><span>presentes</span></div>
      <div class="caixa"><b>${b.atrasos}</b><span>atrasos</span></div>
      <div class="caixa"><b>${b.falta}</b><span>faltas</span></div>
      <div class="caixa"><b>${b.justificadas}</b><span>justificadas</span></div>
      <div class="caixa"><b>${totalFaltas}</b><span>faltas tot. (J+NJ)</span></div>
      <div class="caixa"><b>${b.pct_geral}%</b><span>% válidas (P+A)</span></div>
      <div class="caixa" style="border-color:var(--verde)"><b>${b.pct_pronto}%</b><span>ef. pronto</span></div></div>`;
    /* ordem do Tenente: efetivo em ORDEM ALFABÉTICA (backend pode vir agrupado
       por função — o前端 garante a ordem e a antiguidade é o posto alfabético) */
    const pessoas = (b.pessoas || []).slice()
      .sort((a, x) => String(a.nome_guerra || '').localeCompare(String(x.nome_guerra || ''), 'pt', { sensitivity: 'base' }));
    const linhas = pessoas.map(p =>
      `<tr><td class="num">#${p.antiguidade ?? ''}</td><td><b>${esc(p.nome_guerra)}</b></td>
       <td>${esc(p.funcao || '—')}${p.funcao_id ? ` <small style="color:var(--tx2)">#${p.funcao_id}</small>` : ''}</td>
       <td>${esc(p.setor)}</td><td>${esc(p.grupo || '—')}</td><td class="num">${p.presencas}</td>
       <td class="num">${p.atrasos}</td><td class="num">${p.faltas}</td><td class="num">${p.justificadas}</td></tr>`).join('');
    const forms = (b.formaturas || []).map(f =>
      `<tr><td>${fmtData(f.data)}</td><td>${esc(f.tipo)}</td><td>${esc(f.hora || '—')}</td>
       <td>${f.status === 'fechada' ? 'Fechada' : 'Aberta'}</td>
       <td class="num">${f.presentes}</td><td class="num">${f.faltas}</td></tr>`).join('');
    return `${res}<div class="cartao"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
      <h3 style="margin:0">${titulo} — ${b.convocacoes} conferências no período</h3>
      <a href="/api/relatorio.pdf?de=${encodeURIComponent(b.De)}&ate=${encodeURIComponent(b.Ate)}${grupoQ}" target="_blank"><button class="primario">ABRIR PDF</button></a></div>
      ${forms ? `<div class="rolagem" style="margin-bottom:12px"><table><thead><tr><th>Data</th><th>Tipo</th><th>Hora</th><th>Status</th><th class="num">Presentes</th><th class="num">Faltas</th></tr></thead><tbody>${forms}</tbody></table></div>` : ''}
      <div class="rolagem"><table><thead><tr><th class="num">Antig.</th><th>Nome</th><th>Função</th><th>Setor</th><th>Grupo</th><th class="num">Pres.</th><th class="num">Atraso</th>
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
    gerar();
    document.querySelectorAll('#modos button').forEach(b => b.onclick = () => {
      modo = b.dataset.m;
      document.querySelectorAll('#modos button').forEach(x => x.classList.toggle('ativo', x === b));
      entrada();
    });
    $('#btGerar').onclick = gerar;
  };
})();
