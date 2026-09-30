(function () {
  'use strict';

  /* =====================================================================
     #/escalas — MÓDULO DE ESCALAS E SERVIÇOS INTEGRADOS (v1.0)
     ===================================================================== */

  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const modal = (...args) => window.modal(...args);
  const alerta = (...args) => window.alerta(...args);

  window.ViewEscalas = async function () {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    if (eu.papel === 'admin') { location.hash = '#/admin'; return; }
    navAtiva('#/escalas');

    const app = document.getElementById('app');
    app.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:12px; margin-bottom:16px">
        <div>
          <h2 style="margin:0 0 4px">Escalas & Turnos de Serviço</h2>
          <p style="color:var(--tx2); font-size:13px; margin:0">Planejamento e controle de plantões, guardas e serviços diários.</p>
        </div>
        <div style="display:flex; gap:8px; flex-wrap:wrap">
          <button class="acao-linha" id="btTiposEscala">⚙️ Tipos de Serviço</button>
          <button class="primario" id="btNovoTurno">+ Novo Turno de Serviço</button>
        </div>
      </div>

      <!-- Filtros de Período -->
      <div class="cartao" style="margin-bottom:14px; padding:12px 16px">
        <div class="form-linha" style="align-items:flex-end">
          <div class="campo" style="max-width:200px">
            <label>Mês de Referência</label>
            <input type="month" id="fMesEscala" value="${new Date().toISOString().slice(0, 7)}">
          </div>
          <div class="campo" style="flex:1">
            <label>Buscar por nome ou posto</label>
            <input id="fBuscaEscala" placeholder="Filtrar por nome de guerra ou posto…">
          </div>
        </div>
      </div>

      <!-- Serviços Ativos Hoje -->
      <div class="cartao" style="margin-bottom:14px">
        <h3 style="margin:0 0 10px; display:flex; align-items:center; gap:8px">
          <span>🛡️</span> <span>Efetivo de Serviço Hoje (${fmtData(new Date().toISOString().slice(0, 10))})</span>
        </h3>
        <div id="gridHoje" style="display:grid; grid-template-columns:repeat(auto-fill, minmax(260px, 1fr)); gap:10px">
          <div class="carregando">Carregando serviços de hoje…</div>
        </div>
      </div>

      <!-- Lista de Turnos do Mês -->
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:12px">
          <h3 style="margin:0">📅 Grade de Turnos Programados</h3>
          <div style="display:flex; gap:6px">
            <button class="acao-linha" id="btModoTabela" style="font-size:12px; font-weight:700">📋 Tabela</button>
            <button class="acao-linha" id="btModoGantt" style="font-size:12px; font-weight:700">📊 Cronograma (Gantt)</button>
            <button class="acao-linha" id="btImprimirEscala" style="font-size:12px; font-weight:700; color:var(--verde-claro)">📄 Imprimir Escala</button>
          </div>
        </div>
        <div id="listaTurnos"><div class="carregando">Carregando grade de escalas…</div></div>
      </div>
    `;

    $('#btModoTabela').onclick = () => { modoEscala = 'tabela'; alternarBotoesModo(); renderGradeTurnos(TURNOS_CACHE); };
    $('#btModoGantt').onclick = () => { modoEscala = 'gantt'; alternarBotoesModo(); renderGanttTurnos(TURNOS_CACHE); };
    $('#btImprimirEscala').onclick = () => {
      const mes = $('#fMesEscala') ? $('#fMesEscala').value : '';
      window.open('/api/escalas/pdf?mes=' + encodeURIComponent(mes), '_blank');
    };

    function alternarBotoesModo() {
      const bT = $('#btModoTabela'), bG = $('#btModoGantt');
      if (bT) bT.style.background = modoEscala === 'tabela' ? 'var(--verde)' : 'transparent';
      if (bG) bG.style.background = modoEscala === 'gantt' ? 'var(--verde)' : 'transparent';
    }
    alternarBotoesModo();

    // Carregar dados
    await carregarEscalas();

    // Eventos
    $('#fMesEscala').onchange = () => carregarEscalas();
    $('#fBuscaEscala').oninput = () => filtrarGrade();
    $('#btNovoTurno').onclick = () => modalNovoTurno(null, () => carregarEscalas());
    $('#btTiposEscala').onclick = () => modalGerenciarTipos(() => carregarEscalas());
  };

  let modoEscala = 'tabela';
  let diaGantt = new Date().toISOString().slice(0, 10);
  let TURNOS_CACHE = [];

  async function carregarEscalas() {
    const mes = $('#fMesEscala').value;
    try {
      const [resHoje, resTurnos] = await Promise.all([
        api('/api/escalas/hoje'),
        api('/api/escalas/turnos?mes=' + mes)
      ]);

      renderServicosHoje(resHoje.escalados || []);
      TURNOS_CACHE = resTurnos.turnos || [];
      if (modoEscala === 'gantt') renderGanttTurnos(TURNOS_CACHE);
      else renderGradeTurnos(TURNOS_CACHE);
    } catch (e) {
      $('#gridHoje').innerHTML = '<span class="vazio">Sem dados de escala para hoje.</span>';
      $('#listaTurnos').innerHTML = '<span class="vazio">Falha ao carregar turnos do período.</span>';
    }
  }

  function renderServicosHoje(escalados) {
    const grid = $('#gridHoje');
    if (!grid) return;
    if (!escalados || !escalados.length) {
      grid.innerHTML = '<div style="color:var(--tx3); font-size:13px; grid-column:1/-1; padding:8px 0">Nenhum militar registrado em escala de serviço na data de hoje.</div>';
      return;
    }

    // Agrupar por tipo de escala
    const porTipo = {};
    escalados.forEach(e => {
      const t = e.tipo_nome || 'Serviço Geral';
      (porTipo[t] = porTipo[t] || []).push(e);
    });

    let html = '';
    for (const [tipo, pessoas] of Object.entries(porTipo)) {
      html += `
        <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:12px">
          <div style="font-weight:700; color:var(--verde-claro); font-size:13.5px; margin-bottom:8px; border-bottom:1px solid var(--borda); padding-bottom:4px">
            ${esc(tipo)} <span style="font-size:11px; color:var(--tx3); font-weight:normal">(${pessoas.length})</span>
          </div>
          <div style="display:flex; flex-direction:column; gap:6px">
            ${pessoas.map(p => `
              <div style="display:flex; justify-content:space-between; align-items:center; font-size:13px">
                <b>${esc(p.nome_guerra || p.nome_completo)}</b>
                <span style="color:var(--tx2); font-size:11.5px; background:var(--painel3); padding:2px 6px; border-radius:4px">
                  ${esc(p.funcao_escala || p.funcao || 'Plantão')}
                </span>
              </div>
            `).join('')}
          </div>
        </div>
      `;
    }
    grid.innerHTML = html;
  }

  function renderGradeTurnos(turnos) {
    const cont = $('#listaTurnos');
    if (!cont) return;
    if (!turnos || !turnos.length) {
      cont.innerHTML = '<span class="vazio">Nenhum turno ou serviço cadastrado neste mês.</span>';
      return;
    }

    const linhas = turnos.map(t => {
      const qtdPessoas = (t.pessoas && t.pessoas.length) || 0;
      const nomes = (t.pessoas || []).map(p => esc(p.nome_guerra || p.nome_completo) + (p.funcao_escala ? ' (' + esc(p.funcao_escala) + ')' : '')).join(', ');
      return `
        <tr data-id="${t.id}" data-texto="${esc((t.tipo_nome + ' ' + nomes + ' ' + t.observacao).toLowerCase())}">
          <td><b>${fmtData(t.data_inicio)}</b></td>
          <td>${fmtData(t.data_fim)}</td>
          <td><span style="color:var(--verde-claro); font-weight:600">${esc(t.tipo_nome)}</span></td>
          <td>
            <div style="font-size:13px; line-height:1.4">
              <b>${qtdPessoas} militar(es):</b> ${nomes || '<span style="color:var(--tx3)">sem efetivo</span>'}
            </div>
            ${t.observacao ? `<div style="font-size:11.5px; color:var(--tx3); margin-top:2px">Obs: ${esc(t.observacao)}</div>` : ''}
          </td>
          <td style="white-space:nowrap; text-align:right">
            <button class="acao-linha" data-editar="${t.id}">Editar</button>
            <button class="acao-linha perigo" data-del="${t.id}">Excluir</button>
          </td>
        </tr>
      `;
    }).join('');

    cont.innerHTML = `
      <div class="rolagem">
        <table id="tabGradeTurnos">
          <thead>
            <tr>
              <th>Início</th>
              <th>Término</th>
              <th>Posto / Escala</th>
              <th>Militares Escalados</th>
              <th style="text-align:right">Ações</th>
            </tr>
          </thead>
          <tbody>${linhas}</tbody>
        </table>
      </div>
    `;

    cont.querySelectorAll('button[data-editar]').forEach(b => {
      b.onclick = () => {
        const tid = +b.dataset.editar;
        const turno = TURNOS_CACHE.find(x => x.id === tid);
        if (turno) modalNovoTurno(turno, () => carregarEscalas());
      };
    });

    cont.querySelectorAll('button[data-del]').forEach(b => {
      b.onclick = async () => {
        const tid = +b.dataset.del;
        if (!(await confirmar('Excluir este turno de serviço da escala?'))) return;
        try {
          await api('/api/escalas/turnos/' + tid, { method: 'DELETE' });
          toast('Turno removido');
          carregarEscalas();
        } catch (e) {}
      };
    });
  }

  function filtrarGrade() {
    const q = ($('#fBuscaEscala').value || '').trim().toLowerCase();
    document.querySelectorAll('#tabGradeTurnos tbody tr').forEach(tr => {
      const txt = tr.dataset.texto || '';
      tr.style.display = (!q || txt.includes(q)) ? '' : 'none';
    });
  }

  /* ---------- Visualização Gantt (Cronograma 24 Horas) ---------- */
  function renderGanttTurnos(turnos) {
    const cont = $('#listaTurnos');
    if (!cont) return;

    const horasMarcas = [0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24];

    const turnosDoDia = (turnos || []).filter(t => {
      const dIni = (t.data_inicio || '').slice(0, 10);
      const dFim = (t.data_fim || '').slice(0, 10);
      return diaGantt >= dIni && diaGantt <= dFim;
    });

    const porPosto = {};
    turnosDoDia.forEach(t => {
      const posto = t.tipo_nome || 'Geral';
      if (!porPosto[posto]) porPosto[posto] = [];
      porPosto[posto].push(t);
    });
    const postos = Object.keys(porPosto);

    let linhasHtml = '';
    if (!postos.length) {
      linhasHtml = '<div class="vazio" style="padding:30px 0; text-align:center">Nenhum serviço escalado para a data selecionada (' + fmtData(diaGantt) + ').</div>';
    } else {
      linhasHtml = postos.map(posto => {
        const turnosPosto = porPosto[posto];
        const blocos = turnosPosto.map(t => {
          let hIni = 0, hFim = 24;
          if (t.data_inicio && t.data_inicio.includes('T')) {
            const horaPart = t.data_inicio.split('T')[1] || '';
            const [hh, mm] = horaPart.split(':').map(Number);
            if (!isNaN(hh)) hIni = hh + (mm || 0) / 60;
          }
          if (t.data_fim && t.data_fim.includes('T')) {
            const horaPart = t.data_fim.split('T')[1] || '';
            const [hh, mm] = horaPart.split(':').map(Number);
            if (!isNaN(hh)) hFim = hh + (mm || 0) / 60;
          }
          if (hFim <= hIni) hFim = 24;
          const leftPct = (hIni / 24) * 100;
          const widthPct = Math.max(5, ((hFim - hIni) / 24) * 100);

          const nomes = (t.pessoas || []).map(p => esc(p.nome_guerra || p.nome_completo)).join(', ') || 'Sem efetivo';

          return `
            <div class="gantt-bloco" data-turno="${t.id}" title="${esc(posto)}: ${nomes} (${(t.data_inicio||'').slice(11,16) || '00:00'} às ${(t.data_fim||'').slice(11,16) || '24:00'})"
                 style="position:absolute; left:${leftPct}%; width:${widthPct}%; top:6px; bottom:6px; background:linear-gradient(135deg, var(--verde), #065f46); color:#fff; border-radius:6px; padding:4px 8px; font-size:11.5px; font-weight:700; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; cursor:pointer; box-shadow:0 2px 6px rgba(0,0,0,0.25); border:1px solid rgba(255,255,255,0.2); display:flex; align-items:center">
              <span>👤 ${nomes}</span>
            </div>
          `;
        }).join('');

        return `
          <div style="display:flex; border-bottom:1px solid var(--borda); min-height:48px; align-items:stretch">
            <div style="width:200px; padding:10px 12px; font-weight:700; font-size:13px; background:var(--painel2); border-right:1px solid var(--borda); display:flex; align-items:center; flex-shrink:0">
              🛡️ ${esc(posto)}
            </div>
            <div style="flex:1; position:relative; min-width:600px; background:var(--bg)">
              ${horasMarcas.map(h => `<div style="position:absolute; left:${(h/24)*100}%; top:0; bottom:0; width:1px; background:rgba(255,255,255,0.04)"></div>`).join('')}
              ${blocos}
            </div>
          </div>
        `;
      }).join('');
    }

    cont.innerHTML = `
      <div style="margin-bottom:12px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px">
        <div style="display:flex; align-items:center; gap:8px">
          <label style="font-size:13px; font-weight:700">Data de Observação:</label>
          <input type="date" id="fDataGantt" value="${diaGantt}" style="max-width:160px; font-size:13px; padding:4px 8px">
        </div>
        <div style="font-size:12px; color:var(--tx3)">
          Clique em qualquer bloco horizontal para editar o turno.
        </div>
      </div>

      <div style="border:1px solid var(--borda); border-radius:var(--raio); overflow-x:auto; background:var(--painel)">
        <!-- Régua de Horas -->
        <div style="display:flex; border-bottom:2px solid var(--borda); background:var(--painel2); font-size:11px; font-weight:700; color:var(--tx2)">
          <div style="width:200px; padding:8px 12px; border-right:1px solid var(--borda); flex-shrink:0">Posto / Serviço</div>
          <div style="flex:1; position:relative; min-width:600px; height:28px">
            ${horasMarcas.map(h => `
              <div style="position:absolute; left:${(h/24)*100}%; top:6px; transform:translateX(-50%); font-size:10.5px">
                ${String(h).padStart(2, '0')}:00
              </div>
            `).join('')}
          </div>
        </div>

        <!-- Linhas de Postos -->
        <div>${linhasHtml}</div>
      </div>
    `;

    $('#fDataGantt').onchange = (ev) => {
      diaGantt = ev.target.value;
      renderGanttTurnos(turnos);
    };

    cont.querySelectorAll('.gantt-bloco').forEach(el => {
      el.onclick = () => {
        const tid = +el.dataset.turno;
        const t = (turnos || []).find(x => x.id === tid);
        if (t) modalNovoTurno(t, () => carregarEscalas());
      };
    });
  }

  /* ---------- Modal: Novo / Editar Turno ---------- */
  async function modalNovoTurno(turnoEdicao, onSalvo) {
    const [tiposRes, pessoasRes] = await Promise.all([
      api('/api/escalas/tipos'),
      api('/api/pessoas')
    ]);

    const tipos = (tiposRes.tipos || []).filter(t => t.ativo);
    const pessoas = (pessoasRes.pessoas || []).filter(p => p.status === 'ativo')
      .sort((a, b) => (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt-BR'));

    const hojeStr = new Date().toISOString().slice(0, 10);
    const iniVal = (turnoEdicao && turnoEdicao.data_inicio) || hojeStr;
    const fimVal = (turnoEdicao && turnoEdicao.data_fim) || hojeStr;
    const selTipo = (turnoEdicao && turnoEdicao.tipo_id) || (tipos[0] && tipos[0].id) || 0;
    const obsVal = (turnoEdicao && turnoEdicao.observacao) || '';

    // Mapa de pessoas já selecionadas
    const alocadosMap = {};
    if (turnoEdicao && turnoEdicao.pessoas) {
      turnoEdicao.pessoas.forEach(p => {
        alocadosMap[p.pessoa_id] = p.funcao_escala || '';
      });
    }

    const html = `
      <div class="modal" style="max-width:680px; max-height:88vh; display:flex; flex-direction:column">
        <h3 style="margin-top:0">${turnoEdicao ? 'Editar Turno de Serviço' : 'Novo Turno de Serviço'}</h3>
        
        <div style="overflow-y:auto; flex:1; padding-right:4px">
          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="flex:1">
              <label>Posto / Tipo de Serviço *</label>
              <select id="mTipoEscala">
                ${tipos.map(t => `<option value="${t.id}" ${t.id === selTipo ? 'selected' : ''}>${esc(t.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo">
              <label>Data Início *</label>
              <input type="date" id="mDataIni" value="${iniVal}">
            </div>
            <div class="campo">
              <label>Data Término *</label>
              <input type="date" id="mDataFim" value="${fimVal}">
            </div>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label>Observação do Turno</label>
            <input id="mObsTurno" value="${esc(obsVal)}" placeholder="ex.: Viatura de Ronda, Guarda do Portão das Armas, etc.">
          </div>

          <div style="border-top:1px solid var(--borda); padding-top:10px; margin-top:10px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
              <label style="font-weight:700; margin:0">Selecione o Efetivo Escalado</label>
              <input id="mBuscaPessoa" placeholder="buscar no efetivo…" style="max-width:220px; font-size:12.5px; padding:4px 8px">
            </div>

            <div id="mListaPessoas" style="max-height:260px; overflow-y:auto; border:1px solid var(--borda); border-radius:var(--raio); padding:6px; display:flex; flex-direction:column; gap:4px">
              ${pessoas.map(p => {
                const checked = alocadosMap.hasOwnProperty(p.id);
                const funcEscala = alocadosMap[p.id] || '';
                return `
                  <div class="item-escala-pes" data-pid="${p.id}" data-texto="${esc((p.nome_guerra + ' ' + p.nome_completo + ' ' + (p.setor || '')).toLowerCase())}"
                       style="display:flex; align-items:center; gap:8px; padding:6px 8px; border-radius:6px; background:${checked ? 'rgba(87,161,115,.12)' : 'transparent'}">
                    <input type="checkbox" class="chk-pes" value="${p.id}" ${checked ? 'checked' : ''} style="width:18px; height:18px">
                    <div style="flex:1; min-width:0; font-size:13px">
                      <b>${esc(p.nome_guerra)}</b> <span style="color:var(--tx3); font-size:11.5px">(${esc(p.setor || 'Geral')})</span>
                    </div>
                    <input class="inp-funcao" value="${esc(funcEscala)}" placeholder="Função (ex: Sentinela)"
                           style="max-width:160px; font-size:12px; padding:3px 6px; ${checked ? '' : 'opacity:.4'}" ${checked ? '' : 'disabled'}>
                  </div>
                `;
              }).join('')}
            </div>
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:16px; border-top:1px solid var(--borda); padding-top:12px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mSalvarTurno">Salvar Turno</button>
        </div>
      </div>
    `;

    const m = modal(html);

    // Ligar busca de pessoas no modal
    m.querySelector('#mBuscaPessoa').oninput = ev => {
      const q = ev.target.value.trim().toLowerCase();
      m.querySelectorAll('.item-escala-pes').forEach(el => {
        const txt = el.dataset.texto || '';
        el.style.display = (!q || txt.includes(q)) ? '' : 'none';
      });
    };

    // Ligar checkboxes
    m.querySelectorAll('.chk-pes').forEach(chk => {
      chk.onchange = () => {
        const row = chk.closest('.item-escala-pes');
        const inp = row.querySelector('.inp-funcao');
        row.style.background = chk.checked ? 'rgba(87,161,115,.12)' : 'transparent';
        inp.disabled = !chk.checked;
        inp.style.opacity = chk.checked ? '1' : '.4';
        if (chk.checked && !inp.value) inp.focus();
      };
    });

    // Salvar
    m.querySelector('#mSalvarTurno').onclick = async () => {
      const tipoID = +m.querySelector('#mTipoEscala').value;
      const dtIni = m.querySelector('#mDataIni').value;
      const dtFim = m.querySelector('#mDataFim').value;
      const obs = m.querySelector('#mObsTurno').value.trim();

      if (!tipoID || !dtIni || !dtFim) {
        toast('Tipo de serviço e datas são obrigatórios', 'erro');
        return;
      }
      if (dtFim < dtIni) {
        toast('Data de término não pode ser anterior à data de início', 'erro');
        return;
      }

      const selecionados = [];
      m.querySelectorAll('.item-escala-pes').forEach(row => {
        const chk = row.querySelector('.chk-pes');
        if (chk && chk.checked) {
          const pid = +chk.value;
          const func = row.querySelector('.inp-funcao').value.trim();
          selecionados.push({ pessoa_id: pid, funcao_escala: func });
        }
      });

      if (!selecionados.length) {
        toast('Selecione ao menos um militar para o turno', 'erro');
        return;
      }

      try {
        await api('/api/escalas/turnos', {
          method: 'POST',
          body: JSON.stringify({
            id: turnoEdicao ? turnoEdicao.id : 0,
            tipo_id: tipoID,
            data_inicio: dtIni,
            data_fim: dtFim,
            observacao: obs,
            pessoas: selecionados
          })
        });
        toast(turnoEdicao ? 'Turno atualizado com sucesso' : 'Turno de escala criado!');
        m.remove();
        if (onSalvo) onSalvo();
      } catch (e) {}
    };
  }

  /* ---------- Modal: Gerenciar Tipos de Serviço ---------- */
  async function modalGerenciarTipos(onAlterado) {
    const res = await api('/api/escalas/tipos');
    const tipos = res.tipos || [];

    const html = `
      <div class="modal" style="max-width:540px">
        <h3 style="margin-top:0">Tipos de Serviço & Postos</h3>
        <p style="color:var(--tx2); font-size:12.5px; margin-bottom:12px">Cadastre os postos de serviço ativos da unidade.</p>

        <div style="display:flex; gap:6px; margin-bottom:14px">
          <input id="mNovoTipoNome" placeholder="Novo posto (ex.: Guarda das Garagens)" style="flex:1">
          <button class="primario" id="mBtnAddTipo">Adicionar</button>
        </div>

        <div id="mListaTipos" style="max-height:280px; overflow-y:auto; display:flex; flex-direction:column; gap:6px">
          ${tipos.map(t => `
            <div style="display:flex; justify-content:space-between; align-items:center; padding:8px 10px; background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio)">
              <div>
                <b>${esc(t.nome)}</b>
                ${t.ativo ? '' : '<span style="color:var(--verm-txt); font-size:11px; margin-left:6px">(inativo)</span>'}
              </div>
              <button class="acao-linha perigo" data-deltipo="${t.id}">Remover</button>
            </div>
          `).join('')}
        </div>

        <div style="text-align:right; margin-top:16px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;

    const m = modal(html);

    m.querySelector('#mBtnAddTipo').onclick = async () => {
      const nome = m.querySelector('#mNovoTipoNome').value.trim();
      if (!nome) return;
      try {
        await api('/api/escalas/tipos', { method: 'POST', body: JSON.stringify({ nome, ativo: true }) });
        toast('Tipo de serviço adicionado');
        m.remove();
        modalGerenciarTipos(onAlterado);
        if (onAlterado) onAlterado();
      } catch (e) {}
    };

    m.querySelectorAll('button[data-deltipo]').forEach(b => {
      b.onclick = async () => {
        const id = +b.dataset.deltipo;
        try {
          await api('/api/escalas/tipos/' + id, { method: 'DELETE' });
          toast('Tipo removido ou desativado');
          m.remove();
          modalGerenciarTipos(onAlterado);
          if (onAlterado) onAlterado();
        } catch (e) {}
      };
    });
  }

})();
