(function () {
  'use strict';

  /* =====================================================================
     #/escalas — MÓDULO DE ESCALAS 2.0 (v1.5)
     Modelos de Escala, Postos, Aptos, Fases, Aplicação/Limpeza, Delegação,
     Relatório Diário em PDF e Minhas Escalas.
     ===================================================================== */

  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const modal = (...args) => window.modal(...args);
  const alerta = (...args) => window.alerta(...args);

  let abaEscalas = 'grade'; // 'grade' | 'modelos' | 'minhas'
  let modoEscala = 'tabela'; // 'tabela' | 'gantt'
  let diaSelecionado = new Date().toISOString().slice(0, 10);
  let diaGantt = new Date().toISOString().slice(0, 10);
  let TURNOS_CACHE = [];
  let MODELOS_CACHE = [];

  window.ViewEscalas = async function (subAba) {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    if (eu.papel === 'admin') { location.hash = '#/admin'; return; }
    navAtiva('#/escalas');

    if (subAba) abaEscalas = subAba;

    const ehChefeOuOp = eu.papel === 'gerente' || eu.papel === 'operador' || eu.papel === 'chefe_setor';
    // Usuários sem função gerencial veem por padrão "Minhas Escalas"
    if (!ehChefeOuOp && abaEscalas !== 'minhas') {
      abaEscalas = 'minhas';
    }

    const app = document.getElementById('app');
    app.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:12px; margin-bottom:14px">
        <div>
          <h2 style="margin:0 0 4px">Escalas de Serviço & Plantões</h2>
          <p style="color:var(--tx2); font-size:13px; margin:0">Planejamento, modelos repetitivos, delegação hierárquica e controle de efetivo de serviço.</p>
        </div>
        ${ehChefeOuOp ? `
        <div style="display:flex; gap:8px; flex-wrap:wrap">
          <button class="acao-linha" id="btTiposEscala">⚙️ Tipos de Posto</button>
          <button class="primario" id="btNovoTurnoManual">+ Posto Manual</button>
        </div>` : ''}
      </div>

      <!-- Abas de Navegação -->
      <div class="abas" style="margin-bottom:14px">
        ${ehChefeOuOp ? `
          <button data-aba="grade" class="${abaEscalas === 'grade' ? 'ativo' : ''}">📅 Grade de Serviço</button>
          <button data-aba="modelos" class="${abaEscalas === 'modelos' ? 'ativo' : ''}">📐 Modelos de Escala (Templates)</button>
        ` : ''}
        <button data-aba="minhas" class="${abaEscalas === 'minhas' ? 'ativo' : ''}">👤 Minhas Escalas</button>
      </div>

      <div id="escalaConteudo"><div class="carregando">Carregando módulo de escalas…</div></div>
    `;

    document.querySelectorAll('.abas button[data-aba]').forEach(b => {
      b.onclick = () => {
        abaEscalas = b.dataset.aba;
        window.ViewEscalas();
      };
    });

    if (ehChefeOuOp) {
      const btNovoMan = $('#btNovoTurnoManual');
      if (btNovoMan) btNovoMan.onclick = () => modalNovoTurno(null, () => carregarGrade());
      const btTipos = $('#btTiposEscala');
      if (btTipos) btTipos.onclick = () => modalGerenciarTipos(() => carregarGrade());
    }

    if (abaEscalas === 'grade') await renderAbaGrade();
    else if (abaEscalas === 'modelos') await renderAbaModelos();
    else await renderAbaMinhas();
  };

  /* =====================================================================
     ABA 1: GRADE DE SERVIÇO & CALENDÁRIO DIÁRIO
     ===================================================================== */
  async function renderAbaGrade() {
    const cont = $('#escalaConteudo');
    const mesAtual = diaSelecionado.slice(0, 7);

    cont.innerHTML = `
      <!-- Barra de Controle do Dia e Ações em Lote -->
      <div class="cartao" style="margin-bottom:14px; padding:14px">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:12px">
          <div style="display:flex; align-items:center; gap:10px; flex-wrap:wrap">
            <div class="campo" style="margin:0; min-width:160px">
              <label style="font-weight:700">Data de Serviço</label>
              <input type="date" id="fDiaEscala" value="${diaSelecionado}" style="font-weight:700">
            </div>
            ${eu.papel === 'gerente' ? `
            <div class="campo" style="margin:0; min-width:140px">
              <label>Fase da Escala</label>
              <div id="badgeFaseWrap" style="margin-top:4px">
                <span class="vazio">…</span>
              </div>
            </div>
            ` : ''}
          </div>
          <div style="display:flex; gap:8px; flex-wrap:wrap">
            <button class="primario" id="btNovoPostoAvulso" style="background:#10b981; border-color:#10b981">
              ➕ Posto Avulso (Missão)
            </button>
            <button class="primario" id="btAplicarModeloDia" style="background:#0284c7; border-color:#0284c7">
              ⚡ Aplicar Modelo no Dia
            </button>
            <button class="perigo" id="btLimparEscalaDia" style="background:rgba(239,68,68,0.15); color:var(--verm); border:1px solid rgba(239,68,68,0.3)">
              🗑️ Limpar Dia
            </button>
            <button class="acao-linha" id="btRelatorioDiaPDF" style="color:var(--verde-claro); font-weight:700">
              📄 Relatório Diário (PDF)
            </button>
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

      <!-- Grade de Turnos -->
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:12px">
          <div style="display:flex; align-items:center; gap:10px">
            <h3 style="margin:0">📅 Grade de Postos do Dia (<span id="txtDiaTitulo">${fmtData(diaSelecionado)}</span>)</h3>
            <span id="txtQtdPostos" style="font-size:12px; color:var(--tx2)"></span>
          </div>
          <div style="display:flex; gap:6px">
            <button class="acao-linha" id="btModoTabela" style="font-size:12px; font-weight:700">📋 Tabela</button>
            <button class="acao-linha" id="btModoGantt" style="font-size:12px; font-weight:700">📊 Cronograma (Gantt)</button>
            <button class="acao-linha" id="btImprimirEscalaMes" style="font-size:12px">📄 Escala Mensal</button>
          </div>
        </div>
        <div id="listaTurnos"><div class="carregando">Carregando turnos do dia…</div></div>
      </div>
    `;

    $('#fDiaEscala').onchange = (ev) => {
      diaSelecionado = ev.target.value;
      diaGantt = diaSelecionado;
      $('#txtDiaTitulo').innerText = fmtData(diaSelecionado);
      carregarGrade();
    };

    $('#btNovoPostoAvulso').onclick = () => modalCriarPostoAvulso(diaSelecionado, () => carregarGrade());
    $('#btAplicarModeloDia').onclick = () => modalAplicarModeloDia(diaSelecionado, () => carregarGrade());
    $('#btLimparEscalaDia').onclick = () => acaoLimparEscalaDia(diaSelecionado, () => carregarGrade());
    $('#btRelatorioDiaPDF').onclick = () => window.open('/api/escalas/relatorio-dia.pdf?data=' + diaSelecionado + '&t=' + Date.now(), '_blank');
    $('#btImprimirEscalaMes').onclick = () => window.open('/api/escalas/pdf?mes=' + encodeURIComponent(diaSelecionado.slice(0, 7)), '_blank');

    $('#btModoTabela').onclick = () => { modoEscala = 'tabela'; alternarModo(); renderTabelaGrade(); };
    $('#btModoGantt').onclick = () => { modoEscala = 'gantt'; alternarModo(); renderGanttTurnos(TURNOS_CACHE); };

    function alternarModo() {
      const bT = $('#btModoTabela'), bG = $('#btModoGantt');
      if (bT) bT.style.background = modoEscala === 'tabela' ? 'var(--verde)' : 'transparent';
      if (bG) bG.style.background = modoEscala === 'gantt' ? 'var(--verde)' : 'transparent';
    }
    alternarModo();

    await carregarGrade();
  }

  async function carregarGrade() {
    try {
      const [resHoje, resTurnos] = await Promise.all([
        api('/api/escalas/hoje'),
        api('/api/escalas/turnos?data=' + diaSelecionado)
      ]);

      renderServicosHoje(resHoje.escalados || []);
      TURNOS_CACHE = resTurnos.turnos || [];

      // Atualizar badge de fase
      atualizarBadgeFase(diaSelecionado, TURNOS_CACHE);

      const qtdSpan = $('#txtQtdPostos');
      if (qtdSpan) qtdSpan.innerText = `${TURNOS_CACHE.length} posto(s) cadastrado(s)`;

      if (modoEscala === 'gantt') renderGanttTurnos(TURNOS_CACHE);
      else renderTabelaGrade();
    } catch (e) {
      $('#listaTurnos').innerHTML = '<span class="vazio">Falha ao carregar grade de postos.</span>';
    }
  }

  function atualizarBadgeFase(dataStr, turnos) {
    const eu = quem();
    if (!eu || eu.papel !== 'gerente') return; // controle de fase e exclusivo do gerente
    const wrap = $('#badgeFaseWrap');
    if (!wrap) return;
    const primeiraFase = (turnos[0] && turnos[0].fase) || 'aberto';
    const fases = ['aberto', 'preenchido', 'aprovado', 'publicado'];
    const rotulos = {
      aberto: '🔓 Aberto',
      preenchido: '📝 Preenchido',
      aprovado: '✅ Aprovado',
      publicado: '📢 Publicado'
    };
    const cores = {
      aberto: 'background:rgba(100,116,139,0.2); color:var(--tx2)',
      preenchido: 'background:rgba(59,130,246,0.2); color:#60a5fa',
      aprovado: 'background:rgba(245,158,11,0.2); color:var(--ambar-txt)',
      publicado: 'background:rgba(16,185,129,0.2); color:var(--verde-claro)'
    };

    wrap.innerHTML = `
      <select id="selFaseEscala" style="font-size:12px; font-weight:700; padding:4px 8px; border-radius:6px; ${cores[primeiraFase]}">
        ${fases.map(f => `<option value="${f}" ${f === primeiraFase ? 'selected' : ''}>${rotulos[f]}</option>`).join('')}
      </select>
    `;

    wrap.querySelector('#selFaseEscala').onchange = async (ev) => {
      const novaFase = ev.target.value;
      try {
        await api('/api/escalas/fase', {
          method: 'PATCH',
          body: JSON.stringify({ data: dataStr, fase: novaFase })
        });
        toast('Fase da escala alterada para: ' + novaFase.toUpperCase());
        carregarGrade();
      } catch (err) {
        alerta('Erro ao alterar fase: ' + (err.message || err));
      }
    };
  }

  function renderServicosHoje(escalados) {
    const grid = $('#gridHoje');
    if (!grid) return;
    if (!escalados || !escalados.length) {
      grid.innerHTML = '<div style="color:var(--tx3); font-size:13px; grid-column:1/-1; padding:8px 0">Nenhum militar registrado em escala de serviço na data de hoje.</div>';
      return;
    }
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

  function renderTabelaGrade() {
    const cont = $('#listaTurnos');
    if (!cont) return;
    if (!TURNOS_CACHE || !TURNOS_CACHE.length) {
      cont.innerHTML = `
        <div style="text-align:center; padding:32px 14px">
          <div style="font-size:24px; margin-bottom:6px">📋</div>
          <div style="font-weight:700; margin-bottom:4px">Nenhum posto escalado para ${fmtData(diaSelecionado)}</div>
          <p style="color:var(--tx2); font-size:13px; margin:0 0 14px">Utilize "⚡ Aplicar Modelo no Dia" para criar os postos automaticamente ou cadastre um posto manual.</p>
        </div>
      `;
      return;
    }

    const linhas = TURNOS_CACHE.map(t => {
      const p = (t.pessoas && t.pessoas[0]) || null;
      let badgeDelegado = '';
      if (t.status_delegacao === 'delegado') {
        badgeDelegado = `<span style="font-size:11px; padding:2px 6px; border-radius:4px; background:rgba(59,130,246,0.15); color:#60a5fa; border:1px solid rgba(59,130,246,0.3)">⚡ Delegado: ${esc(t.grupo_delegado_nome || 'Subordinado')}</span>`;
      }

      let militarHTML = '';
      if (p) {
        militarHTML = `
          <div>
            <b>${p.funcao ? esc(p.funcao) + ' ' : ''}${esc(p.nome_guerra)}</b> <small style="color:var(--tx2)">(${esc(p.nome_completo)})</small>
            ${p.setor ? `<br><small style="color:var(--tx3)">Setor: ${esc(p.setor)}</small>` : ''}
          </div>
        `;
      } else {
        militarHTML = `<span style="color:var(--verm); font-style:italic">[VAGO / NÃO PREENCHIDO]</span>`;
      }

      const hIni = (t.data_inicio || '').slice(11, 16) || '07:00';
      const hFim = (t.data_fim || '').slice(11, 16) || '07:00';

      let faixaTxt = '';
      if (t.posto_grad_min_nome && t.posto_grad_max_nome) {
        faixaTxt = `<br><span style="color:var(--tx3); font-size:11px">🎖️ ${esc(t.posto_grad_min_nome)} a ${esc(t.posto_grad_max_nome)}</span>`;
      } else if (t.posto_grad_min_nome) {
        faixaTxt = `<br><span style="color:var(--tx3); font-size:11px">🎖️ Mín: ${esc(t.posto_grad_min_nome)}</span>`;
      } else if (t.posto_grad_max_nome) {
        faixaTxt = `<br><span style="color:var(--tx3); font-size:11px">🎖️ Máx: ${esc(t.posto_grad_max_nome)}</span>`;
      }
      let obsTxt = t.observacao ? `<br><small style="color:var(--tx2); font-size:11px">📝 ${esc(t.observacao)}</small>` : '';

      return `
        <tr data-id="${t.id}">
          <td><b>${esc(t.tipo_nome)}</b>${faixaTxt}${obsTxt}</td>
          <td><code>${hIni} às ${hFim}</code></td>
          <td>${militarHTML}</td>
          <td>${badgeDelegado || '<span style="color:var(--tx3); font-size:12px">Próprio Grupo</span>'}</td>
          <td style="text-align:right; white-space:nowrap">
            <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-editposto="${t.id}" title="Editar Posto, Horários e Posto/Graduação">⚙️ Posto</button>
            <button class="primario" style="font-size:12px; padding:4px 10px; margin-right:4px" data-preencher="${t.id}">
              ${p ? '✏️ Alterar Militar' : '👤 Escalar / Delegar'}
            </button>
            <button class="acao-linha perigo" style="font-size:12px; padding:4px 8px" data-del="${t.id}">🗑️</button>
          </td>
        </tr>
      `;
    }).join('');

    cont.innerHTML = `
      <div class="rolagem">
        <table id="tabGradeTurnos">
          <thead>
            <tr>
              <th>Posto / Serviço</th>
              <th>Horário</th>
              <th>Militar Escalado</th>
              <th>Origem / Delegação</th>
              <th style="text-align:right">Ações</th>
            </tr>
          </thead>
          <tbody>${linhas}</tbody>
        </table>
      </div>
    `;

    cont.querySelectorAll('button[data-editposto]').forEach(b => {
      b.onclick = () => {
        const tid = +b.dataset.editposto;
        const turno = TURNOS_CACHE.find(x => x.id === tid);
        if (turno) modalEditarPostoTurno(turno, () => carregarGrade());
      };
    });

    cont.querySelectorAll('button[data-preencher]').forEach(b => {
      b.onclick = () => {
        const tid = +b.dataset.preencher;
        const turno = TURNOS_CACHE.find(x => x.id === tid);
        if (turno) modalAlocarOuDelegarPosto(turno, () => carregarGrade());
      };
    });

    cont.querySelectorAll('button[data-del]').forEach(b => {
      b.onclick = async () => {
        const tid = +b.dataset.del;
        if (!(await confirmar('Excluir este posto desta data?'))) return;
        try {
          await api('/api/escalas/turnos/' + tid, { method: 'DELETE' });
          toast('Posto excluído');
          carregarGrade();
        } catch (e) {}
      };
    });
  }

  /* Modal de Alocação de Militar vs Delegação para Grupo Subordinado (Checklist Style) */
  async function modalAlocarOuDelegarPosto(turno, aoConcluir) {
    const eu = quem();
    const [candRes, grpRes] = await Promise.all([
      api('/api/escalas/turnos/' + turno.id + '/candidatos'),
      api('/api/grupos/arvore')
    ]);

    const todosCandidatos = (candRes && candRes.candidatos) || [];
    // Requisito 2: Ao selecionar ESCALAR MILITAR DO GRUPO, deve ser utilizado apenas os aptos!
    const candidatosAptos = todosCandidatos.filter(c => c.apto);

    const arvore = grpRes.arvore || [];
    const coletarSubordinados = (no, acc = []) => {
      if (!no) return acc;
      if (Array.isArray(no)) { no.forEach(n => coletarSubordinados(n, acc)); return acc; }
      if (no.id && no.id !== eu.grupo_id) acc.push(no);
      if (no.filhos) no.filhos.forEach(f => coletarSubordinados(f, acc));
      return acc;
    };
    const gruposSubordinados = coletarSubordinados(arvore);

    const pessoaAtualID = (turno.pessoas && turno.pessoas[0] && turno.pessoas[0].pessoa_id) || null;
    let pessoaSelecionadaID = pessoaAtualID;
    let modoAtual = (turno.status_delegacao === 'delegado') ? 'delegar' : 'proprio';

    let faixaInfo = '';
    if (turno.posto_grad_min_nome && turno.posto_grad_max_nome) {
      faixaInfo = `🎖️ Requisito: ${esc(turno.posto_grad_min_nome)} a ${esc(turno.posto_grad_max_nome)}`;
    } else if (turno.posto_grad_min_nome) {
      faixaInfo = `🎖️ Requisito mínimo: ${esc(turno.posto_grad_min_nome)}`;
    } else if (turno.posto_grad_max_nome) {
      faixaInfo = `🎖️ Requisito máximo: ${esc(turno.posto_grad_max_nome)}`;
    }

    const html = `
      <div class="modal" style="max-width:580px; max-height:92vh; display:flex; flex-direction:column">
        <div style="margin-bottom:12px">
          <h3 style="margin:0 0 4px">Preenchimento de Posto: ${esc(turno.tipo_nome)}</h3>
          <div style="font-size:12px; color:var(--tx2); display:flex; gap:12px; flex-wrap:wrap">
            <span>⏰ <b>${(turno.data_inicio||'').slice(11,16)} às ${(turno.data_fim||'').slice(11,16)}</b></span>
            ${faixaInfo ? `<span>${faixaInfo}</span>` : ''}
          </div>
        </div>

        <!-- Seletor de Modo (Estilo Cards Obsidian Glass) -->
        <div style="display:flex; gap:10px; margin-bottom:14px">
          <div id="btModoProprio" class="card-modo-alocar" style="flex:1; cursor:pointer; padding:10px 12px; border-radius:8px; border:2px solid ${modoAtual === 'proprio' ? 'var(--verde)' : 'var(--borda)'}; background:${modoAtual === 'proprio' ? 'rgba(16,185,129,0.12)' : 'var(--painel2)'}; display:flex; align-items:center; gap:10px; transition:all .2s ease">
            <span style="font-size:22px">👤</span>
            <div>
              <div style="font-weight:700; font-size:13px; color:${modoAtual === 'proprio' ? 'var(--verde-claro)' : 'var(--tx1)'}">Membro do Grupo</div>
              <small style="color:var(--tx3); font-size:11px">Efetivo apto do grupo</small>
            </div>
          </div>
          <div id="btModoDelegar" class="card-modo-alocar" style="flex:1; cursor:pointer; padding:10px 12px; border-radius:8px; border:2px solid ${modoAtual === 'delegar' ? '#60a5fa' : 'var(--borda)'}; background:${modoAtual === 'delegar' ? 'rgba(59,130,246,0.12)' : 'var(--painel2)'}; display:flex; align-items:center; gap:10px; transition:all .2s ease">
            <span style="font-size:22px">⚡</span>
            <div>
              <div style="font-weight:700; font-size:13px; color:${modoAtual === 'delegar' ? '#60a5fa' : 'var(--tx1)'}">Delegar para Subgrupo</div>
              <small style="color:var(--tx3); font-size:11px">Fração subordinada preenche</small>
            </div>
          </div>
        </div>

        <!-- Conteúdo 1: Lista de Militares Aptos (Estilo Checklist de Conferências) -->
        <div id="wrapMembroProprio" style="flex:1; overflow-y:auto; padding-right:4px; ${modoAtual !== 'proprio' ? 'display:none' : ''}">
          <div style="margin-bottom:8px">
            <input id="fFiltroCand" placeholder="🔍 Filtrar militares aptos por nome, posto ou setor…" style="width:100%; font-size:12.5px; padding:7px 10px">
          </div>

          <div id="listaCandCards" style="max-height:280px; overflow-y:auto; display:flex; flex-direction:column; gap:6px; padding-right:4px">
            ${candidatosAptos.length === 0 ? `
              <div style="text-align:center; padding:24px 10px; color:var(--tx3); font-size:13px">
                Nenhum militar registrado como <b>apto</b> para concorrer nesta escala.
                <br><small style="color:var(--tx2)">Acesse a edição do modelo para habilitar militares aptos.</small>
              </div>
            ` : candidatosAptos.map(c => {
              const sel = c.id === pessoaSelecionadaID;
              const conflito = c.descanso && c.descanso.conflito;
              const nivel = c.descanso ? c.descanso.nivel : 'ok';
              const folgaH = c.descanso ? c.descanso.horas_folga : 999;

              let badgeDesc = '';
              if (conflito) {
                badgeDesc = `<span style="font-size:10.5px; font-weight:700; color:var(--verm); background:rgba(239,68,68,0.2); padding:2px 6px; border-radius:4px" title="${esc(c.descanso.mensagem)}">🔴 Conflito</span>`;
              } else if (nivel === 'critico') {
                badgeDesc = `<span style="font-size:10.5px; font-weight:700; color:var(--verm); background:rgba(239,68,68,0.2); padding:2px 6px; border-radius:4px" title="${esc(c.descanso.mensagem)}">🔴 Folga < 24h (${folgaH}h)</span>`;
              } else if (nivel === 'alerta') {
                badgeDesc = `<span style="font-size:10.5px; font-weight:700; color:#fb923c; background:rgba(249,115,22,0.2); padding:2px 6px; border-radius:4px" title="${esc(c.descanso.mensagem)}">🟠 Folga < 48h (${folgaH}h)</span>`;
              } else if (nivel === 'atencao') {
                badgeDesc = `<span style="font-size:10.5px; font-weight:700; color:var(--ambar-txt); background:rgba(245,158,11,0.2); padding:2px 6px; border-radius:4px" title="${esc(c.descanso.mensagem)}">🟡 Folga < 72h (${folgaH}h)</span>`;
              } else {
                badgeDesc = `<span style="font-size:10.5px; font-weight:700; color:var(--verde-claro); background:rgba(16,185,129,0.15); padding:2px 6px; border-radius:4px">🟢 Descanso OK</span>`;
              }

              let compatBadge = '';
              if (!c.compativel_posto_grad) {
                compatBadge = `<span style="font-size:10px; color:var(--ambar-txt); background:rgba(245,158,11,0.15); padding:2px 6px; border-radius:4px; margin-left:4px">⚠️ Fora da Faixa</span>`;
              }

              const cardBorder = sel ? 'var(--verde)' : (conflito ? 'rgba(239,68,68,0.4)' : 'var(--borda)');
              const cardBg = sel ? 'rgba(16,185,129,0.14)' : (conflito ? 'rgba(239,68,68,0.05)' : 'var(--painel2)');
              const opac = conflito ? 'opacity:0.7;' : '';

              return `
                <div class="card-cand-row ${sel ? 'selecionado' : ''} ${conflito ? 'com-conflito' : ''}"
                     data-id="${c.id}"
                     data-texto="${esc((c.nome_guerra + ' ' + c.nome_completo + ' ' + (c.funcao||'') + ' ' + (c.setor||'')).toLowerCase())}"
                     data-conflito="${conflito ? '1' : '0'}"
                     style="display:flex; justify-content:space-between; align-items:center; padding:8px 12px; border-radius:8px; border:1px solid ${cardBorder}; background:${cardBg}; cursor:${conflito ? 'not-allowed' : 'pointer'}; ${opac} transition:all .15s ease">
                  <div style="flex:1; min-width:0; padding-right:8px">
                    <div style="display:flex; align-items:center; gap:6px; flex-wrap:wrap">
                      <b style="font-size:13px; color:${sel ? 'var(--verde-claro)' : 'var(--tx1)'}">${esc(c.nome_guerra)}</b>
                      <small style="color:var(--tx2); font-size:11.5px">(${esc(c.nome_completo)})</small>
                      ${compatBadge}
                    </div>
                    <div style="font-size:11px; color:var(--tx3); margin-top:2px">
                      🎖️ <b>${esc(c.funcao || 'Sem Posto/Grad.')}</b> · 🏛️ ${esc(c.setor || 'Sem Setor')}
                    </div>
                  </div>
                  <div style="display:flex; align-items:center; gap:8px; flex-shrink:0">
                    ${badgeDesc}
                    <span class="chk-indicador" style="font-size:16px">${sel ? '✅' : '⚪'}</span>
                  </div>
                </div>
              `;
            }).join('')}
          </div>
        </div>

        <!-- Conteúdo 2: Delegação para Subgrupo -->
        <div id="wrapDelegar" style="flex:1; ${modoAtual !== 'delegar' ? 'display:none' : ''}">
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:8px; padding:14px">
            <div class="campo" style="margin-bottom:8px">
              <label style="font-weight:700">Selecione o Grupo Subordinado Destino</label>
              <select id="selGrupoDelegado">
                <option value="">— Selecionar Grupo Subordinado —</option>
                ${gruposSubordinados.map(g => `
                  <option value="${g.id}" ${g.id === turno.grupo_delegado_id ? 'selected' : ''}>
                    ${esc(g.nome)} ${g.codigo ? '(' + esc(g.codigo) + ')' : ''}
                  </option>
                `).join('')}
              </select>
            </div>
            <p style="font-size:12px; color:var(--tx3); margin:8px 0 0; line-height:1.4">
              Ao delegar o posto, a responsabilidade do preenchimento será transferida para o chefe de pessoal da fração subordinada. O nome alocado atualizará automaticamente nesta visão.
            </p>
          </div>
        </div>

        <!-- Ações do Modal -->
        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:14px; border-top:1px solid var(--borda); padding-top:12px">
          <button class="acao-linha perigo" id="btDesocuparPosto">Desocupar Posto</button>
          <div style="display:flex; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button class="primario" id="btSalvarAlocacao">Salvar</button>
          </div>
        </div>
      </div>
    `;

    const m = modal(html);

    const btModoP = m.querySelector('#btModoProprio');
    const btModoD = m.querySelector('#btModoDelegar');
    const wrapP = m.querySelector('#wrapMembroProprio');
    const wrapD = m.querySelector('#wrapDelegar');

    const atualizarModoVisual = () => {
      if (modoAtual === 'proprio') {
        btModoP.style.borderColor = 'var(--verde)';
        btModoP.style.background = 'rgba(16,185,129,0.12)';
        btModoP.querySelector('div div').style.color = 'var(--verde-claro)';
        btModoD.style.borderColor = 'var(--borda)';
        btModoD.style.background = 'var(--painel2)';
        btModoD.querySelector('div div').style.color = 'var(--tx1)';
        wrapP.style.display = 'block';
        wrapD.style.display = 'none';
      } else {
        btModoD.style.borderColor = '#60a5fa';
        btModoD.style.background = 'rgba(59,130,246,0.12)';
        btModoD.querySelector('div div').style.color = '#60a5fa';
        btModoP.style.borderColor = 'var(--borda)';
        btModoP.style.background = 'var(--painel2)';
        btModoP.querySelector('div div').style.color = 'var(--tx1)';
        wrapP.style.display = 'none';
        wrapD.style.display = 'block';
      }
    };

    btModoP.onclick = () => { modoAtual = 'proprio'; atualizarModoVisual(); };
    btModoD.onclick = () => { modoAtual = 'delegar'; atualizarModoVisual(); };

    // Filtro instantâneo de candidatos
    const fBusca = m.querySelector('#fFiltroCand');
    if (fBusca) {
      fBusca.oninput = ev => {
        const q = ev.target.value.trim().toLowerCase();
        m.querySelectorAll('.card-cand-row').forEach(row => {
          const txt = row.dataset.texto || '';
          row.style.display = (!q || txt.includes(q)) ? 'flex' : 'none';
        });
      };
    }

    // Clique nos cards de candidatos (estilo checklist)
    m.querySelectorAll('.card-cand-row').forEach(row => {
      row.onclick = () => {
        if (row.dataset.conflito === '1') {
          toast('Este militar possui conflito de sobreposição de horário!', 'erro');
          return;
        }
        const cid = +row.dataset.id;
        if (pessoaSelecionadaID === cid) {
          pessoaSelecionadaID = null; // desmarcar
        } else {
          pessoaSelecionadaID = cid;
        }

        m.querySelectorAll('.card-cand-row').forEach(r => {
          const isThis = +r.dataset.id === pessoaSelecionadaID;
          r.style.borderColor = isThis ? 'var(--verde)' : (r.dataset.conflito === '1' ? 'rgba(239,68,68,0.4)' : 'var(--borda)');
          r.style.background = isThis ? 'rgba(16,185,129,0.14)' : (r.dataset.conflito === '1' ? 'rgba(239,68,68,0.05)' : 'var(--painel2)');
          const chk = r.querySelector('.chk-indicador');
          if (chk) chk.innerText = isThis ? '✅' : '⚪';
          const titulo = r.querySelector('b');
          if (titulo) titulo.style.color = isThis ? 'var(--verde-claro)' : 'var(--tx1)';
        });
      };
    });

    m.querySelector('#btDesocuparPosto').onclick = async () => {
      try {
        await api('/api/escalas/turnos/' + turno.id + '/alocar', {
          method: 'POST',
          body: JSON.stringify({ pessoa_id: null, funcao_escala: '' })
        });
        await api('/api/escalas/turnos/' + turno.id + '/delegar', {
          method: 'POST',
          body: JSON.stringify({ grupo_delegado_id: null })
        });
        toast('Posto desocupado');
        m.closest('.modal-mask').remove();
        if (aoConcluir) aoConcluir();
      } catch (err) {
        alerta('Erro ao desocupar posto: ' + (err.message || err));
      }
    };

    m.querySelector('#btSalvarAlocacao').onclick = async () => {
      try {
        if (modoAtual === 'proprio') {
          if (!pessoaSelecionadaID) {
            alerta('Por favor, selecione um militar na lista para assumir o posto.');
            return;
          }
          // Revoga eventual delegação anterior
          await api('/api/escalas/turnos/' + turno.id + '/delegar', {
            method: 'POST', body: JSON.stringify({ grupo_delegado_id: null })
          });
          const resAloc = await api('/api/escalas/turnos/' + turno.id + '/alocar', {
            method: 'POST', body: JSON.stringify({ pessoa_id: pessoaSelecionadaID, funcao_escala: turno.tipo_nome })
          });

          if (resAloc && resAloc.alerta_descanso && resAloc.alerta_descanso.nivel === 'critico') {
            toast('Militar alocado! Atenção: ' + resAloc.alerta_descanso.mensagem, 'aviso');
          } else {
            toast('Militar alocado com sucesso!');
          }
        } else {
          const selG = +m.querySelector('#selGrupoDelegado').value || null;
          if (!selG) { alerta('Por favor, selecione um grupo subordinado para delegar o posto.'); return; }
          // Remove militar do grupo superior
          await api('/api/escalas/turnos/' + turno.id + '/alocar', {
            method: 'POST', body: JSON.stringify({ pessoa_id: null, funcao_escala: '' })
          });
          await api('/api/escalas/turnos/' + turno.id + '/delegar', {
            method: 'POST', body: JSON.stringify({ grupo_delegado_id: selG })
          });
          toast('Posto delegado com sucesso!');
        }
        m.closest('.modal-mask').remove();
        if (aoConcluir) aoConcluir();
      } catch (err) {
        alerta('Erro ao salvar escala: ' + (err.message || err));
      }
    };
  }

  /* Modal de Posto Avulso / Edição de Posto */
  async function modalEditarPostoTurno(alvo, aoConcluir) {
    const ehEdicao = alvo && typeof alvo === 'object';
    const turno = ehEdicao ? alvo : null;
    const diaStr = ehEdicao ? ((turno.data_inicio || '').slice(0, 10) || diaSelecionado) : alvo;

    const [tiposRes, funcoesRes] = await Promise.all([
      api('/api/escalas/tipos'),
      api('/api/catalogo/funcoes')
    ]);
    const tipos = tiposRes.tipos || [];
    const funcoes = (funcoesRes || []).filter(f => f.ativo);

    const valTipo = ehEdicao ? turno.tipo_id : (tipos[0] ? tipos[0].id : 0);
    const valHi = ehEdicao ? ((turno.data_inicio || '').slice(11, 16) || '07:00') : '07:00';
    const valHf = ehEdicao ? ((turno.data_fim || '').slice(11, 16) || '07:00') : '07:00';
    const valObs = ehEdicao ? (turno.observacao || '') : '';
    const valPgMin = ehEdicao ? (turno.posto_grad_min_id || '') : '';
    const valPgMax = ehEdicao ? (turno.posto_grad_max_id || '') : '';

    const html = `
      <div class="modal" style="max-width:520px">
        <h3 style="margin-top:0">${ehEdicao ? '⚙️ Editar Posto de Serviço' : '➕ Adicionar Posto Avulso / Missão'}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">
          ${ehEdicao ? `Edite os horários, tipo e restrições de Posto/Graduação para este posto no dia <b>${fmtData(diaStr)}</b>.` : `Cadastre um posto individual para o dia <b>${fmtData(diaStr)}</b> (ex.: missões especiais, representações ou reforço de guarda).`}
        </p>

        <div class="form-linha" style="margin-bottom:10px">
          <div class="campo" style="flex:2">
            <label>Tipo de Posto / Serviço *</label>
            <select id="avTipoPosto">
              ${tipos.map(t => `<option value="${t.id}" ${t.id === valTipo ? 'selected' : ''}>${esc(t.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo" style="flex:1">
            <label>Início</label>
            <input type="time" id="avHoraIni" value="${esc(valHi)}">
          </div>
          <div class="campo" style="flex:1">
            <label>Término</label>
            <input type="time" id="avHoraFim" value="${esc(valHf)}">
          </div>
        </div>

        <div class="campo" style="margin-bottom:10px">
          <label>Observação / Descrição da Missão</label>
          <input id="avObs" value="${esc(valObs)}" placeholder="ex.: Operação Especial / Escolta / Representação de Cerimonial">
        </div>

        <div class="form-linha" style="margin-bottom:14px">
          <div class="campo">
            <label>Posto / Graduação Mínimo</label>
            <select id="avPgMin">
              <option value="">— Qualquer Posto/Graduação —</option>
              ${funcoes.map(f => `<option value="${f.id}" ${f.id === valPgMin ? 'selected' : ''}>${esc(f.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo">
            <label>Posto / Graduação Máximo</label>
            <select id="avPgMax">
              <option value="">— Qualquer Posto/Graduação —</option>
              ${funcoes.map(f => `<option value="${f.id}" ${f.id === valPgMax ? 'selected' : ''}>${esc(f.nome)}</option>`).join('')}
            </select>
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px; border-top:1px solid var(--borda); padding-top:12px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="btSalvarPostoAvulso">${ehEdicao ? 'Salvar Alterações' : 'Criar Posto'}</button>
        </div>
      </div>
    `;

    const m = modal(html);
    m.querySelector('#btSalvarPostoAvulso').onclick = async () => {
      const tipoID = +m.querySelector('#avTipoPosto').value;
      const hi = m.querySelector('#avHoraIni').value || '07:00';
      const hf = m.querySelector('#avHoraFim').value || '07:00';
      const obs = m.querySelector('#avObs').value.trim();
      const pgMin = +m.querySelector('#avPgMin').value || null;
      const pgMax = +m.querySelector('#avPgMax').value || null;

      if (!tipoID) { alerta('Selecione o tipo de posto.'); return; }

      let dataFimDia = diaStr;
      if (hf <= hi) {
        const dFim = new Date(diaStr + 'T12:00:00');
        dFim.setDate(dFim.getDate() + 1);
        dataFimDia = dFim.toISOString().slice(0, 10);
      }

      const payload = {
        tipo_id: tipoID,
        data_inicio: diaStr + 'T' + hi + ':00',
        data_fim: dataFimDia + 'T' + hf + ':00',
        observacao: obs,
        posto_grad_min_id: pgMin,
        posto_grad_max_id: pgMax
      };
      if (ehEdicao) payload.id = turno.id;

      try {
        await api('/api/escalas/turnos', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        toast(ehEdicao ? 'Posto atualizado com sucesso!' : 'Posto avulso criado com sucesso!');
        m.closest('.modal-mask').remove();
        if (aoConcluir) aoConcluir();
      } catch (err) {
        alerta((ehEdicao ? 'Erro ao atualizar posto: ' : 'Erro ao criar posto avulso: ') + (err.message || err));
      }
    };
  }
  const modalCriarPostoAvulso = (dia, cb) => modalEditarPostoTurno(dia, cb);

  /* Modal de Aplicação de Modelo */
  async function modalAplicarModeloDia(dataStr, aoConcluir) {
    let modelos = [];
    try {
      const r = await api('/api/escalas/modelos');
      modelos = (r.modelos || []).filter(m => m.ativo);
    } catch (e) { modelos = []; }

    if (!modelos.length) {
      alerta('Nenhum Modelo de Escala cadastrado. Acesse a aba "Modelos de Escala (Templates)" para cadastrar o primeiro modelo.');
      return;
    }

    const html = `
      <div class="modal" style="max-width:440px">
        <h3 style="margin-top:0">⚡ Aplicar Modelo de Escala</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">
          Selecione o modelo para replicar todos os postos definidos para a data <b>${fmtData(dataStr)}</b>. A escala nascerá na fase <b>ABERTO</b>.
        </p>
        <div class="campo" style="margin-bottom:16px">
          <label style="font-weight:700">Modelo de Escala</label>
          <select id="selModAplicar">
            ${modelos.map(m => `<option value="${m.id}">${esc(m.nome)} (${m.total_postos} postos)</option>`).join('')}
          </select>
        </div>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="btConfirmaAplicar">Aplicar Escala</button>
        </div>
      </div>
    `;

    const m = modal(html);
    m.querySelector('#btConfirmaAplicar').onclick = async () => {
      const modID = +m.querySelector('#selModAplicar').value;
      try {
        const r = await api('/api/escalas/aplicar-modelo', {
          method: 'POST',
          body: JSON.stringify({ modelo_id: modID, data: dataStr })
        });
        toast(`Modelo aplicado com sucesso! ${r.turnos_criados} posto(s) criados.`);
        m.closest('.modal-mask').remove();
        if (aoConcluir) aoConcluir();
      } catch (err) {
        alerta('Erro ao aplicar modelo: ' + (err.message || err));
      }
    };
  }

  /* Ação de Limpar Escala do Dia */
  async function acaoLimparEscalaDia(dataStr, aoConcluir) {
    const msg = `Tem certeza que deseja LIMPAR todos os postos de escala do dia ${fmtData(dataStr)}?\n\nEsta ação removerá todos os postos escalados desta data para que você possa recomeçar do zero.`;
    if (!(await confirmar(msg))) return;

    try {
      const r = await api('/api/escalas/limpar-dia', {
        method: 'POST',
        body: JSON.stringify({ data: dataStr })
      });
      toast(`Escala do dia limpa! ${r.removidos} postos removidos.`);
      if (aoConcluir) aoConcluir();
    } catch (err) {
      alerta('Erro ao limpar escala: ' + (err.message || err));
    }
  }

  /* Visualização Gantt */
  function renderGanttTurnos(turnos) {
    const cont = $('#listaTurnos');
    if (!cont) return;
    const horasMarcas = [0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24];
    const porPosto = {};
    (turnos || []).forEach(t => {
      const posto = t.tipo_nome || 'Geral';
      (porPosto[posto] = porPosto[posto] || []).push(t);
    });
    const postos = Object.keys(porPosto);

    let linhasHtml = '';
    if (!postos.length) {
      linhasHtml = '<div class="vazio" style="padding:30px 0; text-align:center">Nenhum serviço escalado para a data selecionada (' + fmtData(diaSelecionado) + ').</div>';
    } else {
      linhasHtml = postos.map(posto => {
        const turnosPosto = porPosto[posto];
        const blocos = turnosPosto.map(t => {
          let hIni = 7, hFim = 19;
          if (t.data_inicio && t.data_inicio.includes('T')) {
            const [hh, mm] = t.data_inicio.split('T')[1].slice(0, 5).split(':').map(Number);
            if (!isNaN(hh)) hIni = hh + (mm || 0) / 60;
          }
          if (t.data_fim && t.data_fim.includes('T')) {
            const [hh, mm] = t.data_fim.split('T')[1].slice(0, 5).split(':').map(Number);
            if (!isNaN(hh)) hFim = hh + (mm || 0) / 60;
          }
          if (hFim <= hIni) hFim = 24;
          const leftPct = (hIni / 24) * 100;
          const widthPct = Math.max(5, ((hFim - hIni) / 24) * 100);
          const nomes = (t.pessoas || []).map(p => esc(p.nome_guerra || p.nome_completo)).join(', ') || 'Vago';

          return `
            <div class="gantt-bloco" data-preencher="${t.id}" title="${esc(posto)}: ${esc(nomes)}"
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
      <div style="overflow-x:auto; border:1px solid var(--borda); border-radius:var(--raio)">
        <div style="display:flex; border-bottom:2px solid var(--borda); background:var(--painel2); font-size:11px; font-weight:700; color:var(--tx2)">
          <div style="width:200px; padding:8px 12px; border-right:1px solid var(--borda)">Posto / Serviço</div>
          <div style="flex:1; position:relative; min-width:600px; height:28px">
            ${horasMarcas.map(h => `<span style="position:absolute; left:${(h/24)*100}%; transform:translateX(-50%); top:6px">${String(h).padStart(2,'0')}h</span>`).join('')}
          </div>
        </div>
        ${linhasHtml}
      </div>
    `;

    cont.querySelectorAll('.gantt-bloco[data-preencher]').forEach(b => {
      b.onclick = () => {
        const tid = +b.dataset.preencher;
        const turno = TURNOS_CACHE.find(x => x.id === tid);
        if (turno) modalAlocarOuDelegarPosto(turno, () => carregarGrade());
      };
    });
  }

  /* =====================================================================
     ABA 2: MODELOS DE ESCALA (TEMPLATES)
     ===================================================================== */
  async function renderAbaModelos() {
    const cont = $('#escalaConteudo');
    cont.innerHTML = '<div class="carregando">Carregando modelos de escala…</div>';

    try {
      const res = await api('/api/escalas/modelos');
      MODELOS_CACHE = res.modelos || [];
    } catch (e) { MODELOS_CACHE = []; }

    const linhas = MODELOS_CACHE.map(m => `
      <tr>
        <td><b>${esc(m.nome)}</b></td>
        <td style="color:var(--tx2); font-size:12.5px">${esc(m.descricao || '—')}</td>
        <td style="text-align:center"><span style="font-weight:700; color:var(--verde-claro)">${m.total_postos} postos</span></td>
        <td style="text-align:center"><span style="font-weight:700; color:#60a5fa">${m.total_aptos} militares</span></td>
        <td style="text-align:right; white-space:nowrap">
          <button class="acao-linha" data-editmodelo="${m.id}">✏️ Editar Modelo</button>
          <button class="acao-linha perigo" data-delmodelo="${m.id}">🗑️ Excluir</button>
        </td>
      </tr>
    `).join('');

    cont.innerHTML = `
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
          <div>
            <h3 style="margin:0 0 4px">Modelos de Escala Repetitivos</h3>
            <p style="color:var(--tx2); font-size:12.5px; margin:0">Cadastre a estrutura fixa de postos e selecione todo o efetivo habilitado a concorrer.</p>
          </div>
          <button class="primario" id="btNovoModelo">+ Criar Novo Modelo de Escala</button>
        </div>

        <div class="rolagem">
          <table>
            <thead>
              <tr>
                <th>Nome do Modelo</th>
                <th>Descrição / Regras</th>
                <th style="text-align:center">Postos Definidos</th>
                <th style="text-align:center">Militares Aptos</th>
                <th style="text-align:right">Ações</th>
              </tr>
            </thead>
            <tbody>${linhas || '<tr><td colspan="5"><span class="vazio">Nenhum modelo cadastrado.</span></td></tr>'}</tbody>
          </table>
        </div>
      </div>
    `;

    $('#btNovoModelo').onclick = () => modalEditarModelo(0, () => renderAbaModelos());
    cont.querySelectorAll('button[data-editmodelo]').forEach(b => {
      b.onclick = () => modalEditarModelo(+b.dataset.editmodelo, () => renderAbaModelos());
    });
    cont.querySelectorAll('button[data-delmodelo]').forEach(b => {
      b.onclick = async () => {
        const id = +b.dataset.delmodelo;
        if (!(await confirmar('Excluir este modelo de escala?'))) return;
        try {
          await api('/api/escalas/modelos/' + id, { method: 'DELETE' });
          toast('Modelo excluído');
          renderAbaModelos();
        } catch (e) {}
      };
    });
  }

  async function modalEditarModelo(modeloID, aoConcluir) {
    const [tiposRes, pesRes, funcoesRes] = await Promise.all([
      api('/api/escalas/tipos'),
      api('/api/pessoas'),
      api('/api/catalogo/funcoes')
    ]);

    const tipos = tiposRes.tipos || [];
    const pessoas = pesRes.pessoas || [];
    const funcoes = (funcoesRes || []).filter(f => f.ativo);

    let dadosMod = { id: 0, nome: '', descricao: '', postos: [], aptos: [] };
    if (modeloID > 0) {
      try {
        const res = await api('/api/escalas/modelos/' + modeloID);
        dadosMod = {
          id: res.modelo.id,
          nome: res.modelo.nome,
          descricao: res.modelo.descricao,
          postos: res.postos || [],
          aptos: res.aptos || []
        };
      } catch (e) {}
    }

    const aptosSet = new Set(dadosMod.aptos.map(a => a.pessoa_id));

    const html = `
      <div class="modal" style="max-width:840px; max-height:92vh; display:flex; flex-direction:column">
        <h3 style="margin-top:0">${modeloID > 0 ? 'Editar Modelo de Escala' : 'Novo Modelo de Escala'}</h3>
        
        <div style="overflow-y:auto; flex:1; padding-right:4px">
          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="flex:2">
              <label>Nome do Modelo *</label>
              <input id="modNome" value="${esc(dadosMod.nome)}" placeholder="ex.: Guarda ao Quartel (Dias Úteis)">
            </div>
            <div class="campo" style="flex:3">
              <label>Descrição / Finalidade</label>
              <input id="modDesc" value="${esc(dadosMod.descricao)}" placeholder="ex.: Serviço de 24h, rendição às 07:00">
            </div>
          </div>

          <!-- Seção 1: Postos do Modelo -->
          <div style="border-top:1px solid var(--borda); padding-top:12px; margin-top:12px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
              <div>
                <label style="font-weight:700; margin:0">1. Postos & Horários do Modelo</label>
                <div style="font-size:11px; color:var(--tx3)">Defina os postos, horários e faixas mín./máx. de Posto/Graduação permitidas.</div>
              </div>
              <button class="acao-linha" id="btAddPostoLinha" style="font-size:12px">+ Adicionar Posto</button>
            </div>
            <div id="listaPostosMod" style="display:flex; flex-direction:column; gap:6px; margin-bottom:12px"></div>
          </div>

          <!-- Seção 2: Efetivo Apto para Concorrer -->
          <div style="border-top:1px solid var(--borda); padding-top:12px; margin-top:12px">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
              <label style="font-weight:700; margin:0">2. Militares Aptos a Concorrer nesta Escala</label>
              <input id="fBuscaAptos" placeholder="Filtrar por nome, setor ou posto…" style="max-width:240px; font-size:12px; padding:4px 8px">
            </div>

            <!-- Botões de Seleção Rápida por Posto/Graduação -->
            <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:6px; padding:8px 10px; margin-bottom:8px">
              <div style="font-size:11px; font-weight:700; color:var(--tx2); margin-bottom:6px">
                🎖️ Pré-seleção Rápida por Posto / Graduação:
              </div>
              <div style="display:flex; flex-wrap:wrap; gap:5px; align-items:center">
                ${funcoes.map(f => `
                  <button type="button" class="acao-linha btn-pg-toggle" data-fid="${f.id}" data-fnome="${esc(f.nome)}" style="font-size:11px; padding:2px 8px; border-radius:12px">
                    + ${esc(f.nome)}
                  </button>
                `).join('')}
                <span style="border-left:1px solid var(--borda); height:16px; margin:0 4px"></span>
                <button type="button" class="acao-linha" id="btSelTodosAptos" style="font-size:11px; padding:2px 8px; border-radius:12px">Todos</button>
                <button type="button" class="acao-linha" id="btDeselTodosAptos" style="font-size:11px; padding:2px 8px; border-radius:12px">Nenhum</button>
              </div>
            </div>

            <div id="listaAptosMod" style="max-height:230px; overflow-y:auto; border:1px solid var(--borda); border-radius:8px; padding:6px; display:flex; flex-direction:column; gap:4px">
              ${pessoas.map(p => {
                const checked = aptosSet.has(p.id);
                return `
                  <label class="item-apto-pes" 
                         data-id="${p.id}"
                         data-fid="${p.funcao_id || ''}"
                         data-fnome="${esc(p.funcao || '')}"
                         data-texto="${esc((p.nome_guerra + ' ' + p.nome_completo + ' ' + (p.funcao || '') + ' ' + (p.setor || '')).toLowerCase())}"
                         style="display:flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px; cursor:pointer; background:${checked ? 'rgba(59,130,246,.12)' : 'transparent'}; border:1px solid ${checked ? 'rgba(59,130,246,.3)' : 'transparent'}; transition:all .15s ease">
                    <input type="checkbox" class="chk-apto" value="${p.id}" ${checked ? 'checked' : ''} style="width:16px; height:16px">
                    <div style="flex:1; min-width:0; display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap">
                      <div>
                        <b>${esc(p.nome_guerra)}</b> <small style="color:var(--tx2); font-size:11.5px">(${esc(p.nome_completo)})</small>
                      </div>
                      <div style="display:flex; align-items:center; gap:6px; font-size:11px; flex-shrink:0">
                        <span style="background:var(--painel2); border:1px solid var(--borda); padding:1px 6px; border-radius:4px; color:var(--tx1)">🎖️ <b>${esc(p.funcao || 'Sem Posto/Grad.')}</b></span>
                        ${p.setor ? `<span style="background:var(--painel2); border:1px solid var(--borda); padding:1px 6px; border-radius:4px; color:var(--tx2)">🏛️ ${esc(p.setor)}</span>` : ''}
                      </div>
                    </div>
                  </label>
                `;
              }).join('')}
            </div>
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:14px; border-top:1px solid var(--borda); padding-top:12px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="btSalvarModeloDef">Salvar Modelo</button>
        </div>
      </div>
    `;

    const m = modal(html);

    // Gerenciador de linhas de postos
    const containerPostos = m.querySelector('#listaPostosMod');
    const adicionarPostoLinha = (p = null) => {
      const tipoID = (p && p.tipo_id) || (tipos[0] && tipos[0].id) || 1;
      const hi = (p && p.hora_inicio) || '07:00';
      const hf = (p && p.hora_fim) || '07:00';
      const qtd = (p && p.quantidade) || 1;
      const pgMin = (p && p.posto_grad_min_id) || '';
      const pgMax = (p && p.posto_grad_max_id) || '';

      const div = document.createElement('div');
      div.className = 'linha-posto-mod';
      div.style.cssText = 'display:flex; flex-wrap:wrap; gap:8px; align-items:center; background:var(--painel2); padding:8px 10px; border-radius:6px; border:1px solid var(--borda)';
      div.innerHTML = `
        <select class="sel-tipo-posto" style="flex:2; min-width:140px">
          ${tipos.map(t => `<option value="${t.id}" ${t.id === tipoID ? 'selected' : ''}>${esc(t.nome)}</option>`).join('')}
        </select>
        <div style="display:flex; align-items:center; gap:4px">
          <label style="font-size:11px; color:var(--tx3)">De:</label>
          <input type="time" class="inp-hi" value="${hi}" style="width:85px">
        </div>
        <div style="display:flex; align-items:center; gap:4px">
          <label style="font-size:11px; color:var(--tx3)">Até:</label>
          <input type="time" class="inp-hf" value="${hf}" style="width:85px">
        </div>
        <div style="display:flex; align-items:center; gap:4px">
          <label style="font-size:11px; color:var(--tx3)">Qtd:</label>
          <input type="number" class="inp-qtd" min="1" max="50" value="${qtd}" style="width:55px">
        </div>
        <div style="display:flex; align-items:center; gap:4px; flex:1; min-width:130px">
          <label style="font-size:11px; color:var(--tx3)">Mín:</label>
          <select class="sel-pg-min" style="width:100%; font-size:11px">
            <option value="">— Mínimo —</option>
            ${funcoes.map(f => `<option value="${f.id}" ${f.id === pgMin ? 'selected' : ''}>${esc(f.nome)}</option>`).join('')}
          </select>
        </div>
        <div style="display:flex; align-items:center; gap:4px; flex:1; min-width:130px">
          <label style="font-size:11px; color:var(--tx3)">Máx:</label>
          <select class="sel-pg-max" style="width:100%; font-size:11px">
            <option value="">— Máximo —</option>
            ${funcoes.map(f => `<option value="${f.id}" ${f.id === pgMax ? 'selected' : ''}>${esc(f.nome)}</option>`).join('')}
          </select>
        </div>
        <button type="button" class="acao-linha perigo bt-del-posto" style="padding:4px 8px" title="Remover Posto">✕</button>
      `;
      div.querySelector('.bt-del-posto').onclick = () => div.remove();
      containerPostos.appendChild(div);
    };

    if (dadosMod.postos && dadosMod.postos.length) {
      dadosMod.postos.forEach(p => adicionarPostoLinha(p));
    } else {
      adicionarPostoLinha();
    }

    m.querySelector('#btAddPostoLinha').onclick = () => adicionarPostoLinha();

    // Filtro de aptos
    m.querySelector('#fBuscaAptos').oninput = ev => {
      const q = ev.target.value.trim().toLowerCase();
      m.querySelectorAll('.item-apto-pes').forEach(el => {
        const txt = el.dataset.texto || '';
        el.style.display = (!q || txt.includes(q)) ? '' : 'none';
      });
    };

    // Botões de Seleção Rápida por Posto/Graduação
    m.querySelectorAll('.btn-pg-toggle').forEach(btn => {
      btn.onclick = () => {
        const fid = btn.dataset.fid;
        const fnome = btn.dataset.fnome;
        const matchingLabels = Array.from(m.querySelectorAll('.item-apto-pes')).filter(el => {
          return (fid && el.dataset.fid === fid) || (fnome && el.dataset.fnome === fnome);
        });
        if (!matchingLabels.length) {
          toast(`Nenhum militar com posto/graduação "${fnome}" encontrado.`, 'aviso');
          return;
        }
        const allChecked = matchingLabels.every(l => l.querySelector('.chk-apto').checked);
        const novoEstado = !allChecked;
        matchingLabels.forEach(l => {
          const chk = l.querySelector('.chk-apto');
          chk.checked = novoEstado;
          l.style.background = novoEstado ? 'rgba(59,130,246,.12)' : 'transparent';
          l.style.borderColor = novoEstado ? 'rgba(59,130,246,.3)' : 'transparent';
        });
        toast(`${novoEstado ? 'Selecionados' : 'Desmarcados'} ${matchingLabels.length} militares (${fnome}).`);
      };
    });

    const btTodos = m.querySelector('#btSelTodosAptos');
    if (btTodos) {
      btTodos.onclick = () => {
        m.querySelectorAll('.item-apto-pes').forEach(l => {
          const chk = l.querySelector('.chk-apto');
          chk.checked = true;
          l.style.background = 'rgba(59,130,246,.12)';
          l.style.borderColor = 'rgba(59,130,246,.3)';
        });
      };
    }
    const btNenhum = m.querySelector('#btDeselTodosAptos');
    if (btNenhum) {
      btNenhum.onclick = () => {
        m.querySelectorAll('.item-apto-pes').forEach(l => {
          const chk = l.querySelector('.chk-apto');
          chk.checked = false;
          l.style.background = 'transparent';
          l.style.borderColor = 'transparent';
        });
      };
    }

    m.querySelectorAll('.chk-apto').forEach(chk => {
      chk.onchange = () => {
        const pLabel = chk.closest('.item-apto-pes');
        pLabel.style.background = chk.checked ? 'rgba(59,130,246,.12)' : 'transparent';
        pLabel.style.borderColor = chk.checked ? 'rgba(59,130,246,.3)' : 'transparent';
      };
    });

    m.querySelector('#btSalvarModeloDef').onclick = async () => {
      const nome = m.querySelector('#modNome').value.trim();
      const desc = m.querySelector('#modDesc').value.trim();
      if (!nome) { alerta('Por favor, informe o nome do modelo.'); return; }

      const postos = [];
      m.querySelectorAll('.linha-posto-mod').forEach((row, idx) => {
        postos.push({
          tipo_id: +row.querySelector('.sel-tipo-posto').value,
          hora_inicio: row.querySelector('.inp-hi').value || '07:00',
          hora_fim: row.querySelector('.inp-hf').value || '07:00',
          quantidade: +row.querySelector('.inp-qtd').value || 1,
          posto_grad_min_id: +row.querySelector('.sel-pg-min').value || null,
          posto_grad_max_id: +row.querySelector('.sel-pg-max').value || null,
          ordem: idx
        });
      });

      const aptosIDs = Array.from(m.querySelectorAll('.chk-apto:checked')).map(c => +c.value);

      try {
        await api('/api/escalas/modelos', {
          method: 'POST',
          body: JSON.stringify({
            id: modeloID,
            nome,
            descricao: desc,
            postos,
            aptos_ids: aptosIDs
          })
        });
        toast('Modelo de escala salvo com sucesso!');
        m.closest('.modal-mask').remove();
        if (aoConcluir) aoConcluir();
      } catch (err) {
        alerta('Erro ao salvar modelo: ' + (err.message || err));
      }
    };
  }

  /* =====================================================================
     ABA 3: MINHAS ESCALAS (VISÃO DO MILITAR)
     ===================================================================== */
  async function renderAbaMinhas() {
    const cont = $('#escalaConteudo');
    cont.innerHTML = '<div class="carregando">Carregando seus registros de escala…</div>';

    try {
      const d = await api('/api/escalas/minhas');
      const aptas = d.escalas_aptas || [];
      const prox = d.proximos_turnos || [];
      const hist = d.historico || [];

      cont.innerHTML = `
        <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(320px, 1fr)); gap:16px">
          <!-- 1. Próximos Serviços Escalados -->
          <div class="cartao">
            <h3 style="margin-top:0; color:var(--verde-claro); display:flex; align-items:center; gap:8px">
              <span>📅</span> <span>Próximos Serviços Escalados (${prox.length})</span>
            </h3>
            ${prox.length ? `
              <div style="display:flex; flex-direction:column; gap:8px">
                ${prox.map(t => `
                  <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:8px; padding:10px 12px">
                    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:4px">
                      <b style="font-size:14px">${esc(t.posto_nome)}</b>
                      <span style="font-size:11px; padding:2px 6px; border-radius:4px; background:rgba(16,185,129,0.15); color:var(--verde-claro); font-weight:700">
                        ${esc(t.fase ? t.fase.toUpperCase() : 'ESCALADO')}
                      </span>
                    </div>
                    <div style="font-size:13px; color:var(--tx2)">
                      Início: <b>${fmtData(t.data_inicio)} às ${(t.data_inicio||'').slice(11,16)}</b><br>
                      Término: <b>${fmtData(t.data_fim)} às ${(t.data_fim||'').slice(11,16)}</b>
                    </div>
                    ${t.funcao_escala ? `<div style="font-size:12px; color:var(--tx3); margin-top:4px">Função: ${esc(t.funcao_escala)}</div>` : ''}
                  </div>
                `).join('')}
              </div>
            ` : '<div class="vazio" style="padding:20px 0">Você não possui escalas ou serviços agendados para os próximos dias.</div>'}
          </div>

          <!-- 2. Escalas em que Concorre como Apto -->
          <div class="cartao">
            <h3 style="margin-top:0; color:#60a5fa; display:flex; align-items:center; gap:8px">
              <span>🎖️</span> <span>Escalas em que Estou Habilitado (${aptas.length})</span>
            </h3>
            ${aptas.length ? `
              <div style="display:flex; flex-direction:column; gap:8px">
                ${aptas.map(a => `
                  <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:8px; padding:10px 12px">
                    <b style="font-size:13.5px">${esc(a.nome)}</b>
                    <p style="font-size:12px; color:var(--tx2); margin:4px 0 0">${esc(a.descricao || 'Habilitado a concorrer nesta escala.')}</p>
                  </div>
                `).join('')}
              </div>
            ` : '<div class="vazio" style="padding:20px 0">Você ainda não foi incluído como apto em nenhuma escala.</div>'}
          </div>
        </div>

        <!-- 3. Histórico Recente de Serviços -->
        <div class="cartao" style="margin-top:16px">
          <h3 style="margin-top:0; color:var(--tx2); display:flex; align-items:center; gap:8px">
            <span>📜</span> <span>Histórico Recente de Serviços Realizados (${hist.length})</span>
          </h3>
          ${hist.length ? `
            <div class="rolagem">
              <table>
                <thead>
                  <tr>
                    <th>Data</th>
                    <th>Posto / Serviço</th>
                    <th>Função Cumprida</th>
                    <th>Unidade</th>
                  </tr>
                </thead>
                <tbody>
                  ${hist.map(h => `
                    <tr>
                      <td><b>${fmtData(h.data_inicio)}</b></td>
                      <td>${esc(h.posto_nome)}</td>
                      <td>${esc(h.funcao_escala || 'Plantão')}</td>
                      <td><code style="font-size:11px">${esc(h.grupo_nome || '—')}</code></td>
                    </tr>
                  `).join('')}
                </tbody>
              </table>
            </div>
          ` : '<div class="vazio" style="padding:20px 0">Nenhum serviço anterior registrado recentemente.</div>'}
        </div>
      `;
    } catch (e) {
      cont.innerHTML = '<span class="vazio">Falha ao carregar seus dados de escala.</span>';
    }
  }

  /* Modais auxiliares legados (Tipo de Posto e Manual) */
  async function modalGerenciarTipos(aoConcluir) {
    const res = await api('/api/escalas/tipos');
    const tipos = res.tipos || [];
    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0">Postos / Tipos de Serviço</h3>
        <div style="max-height:260px; overflow-y:auto; margin-bottom:12px">
          ${tipos.map(t => `
            <div style="display:flex; justify-content:space-between; align-items:center; padding:6px 8px; border-bottom:1px solid var(--borda)">
              <div><b>${esc(t.nome)}</b><br><small style="color:var(--tx3)">${esc(t.descricao || '—')}</small></div>
              <button class="acao-linha perigo" style="padding:2px 6px; font-size:11px" data-deltipo="${t.id}">excluir</button>
            </div>
          `).join('')}
        </div>
        <div class="form-linha">
          <div class="campo" style="flex:1"><label>Novo Posto</label><input id="novoTipoNome" placeholder="ex: Cabo da Guarda"></div>
          <button class="primario" id="btAddTipo" style="align-self:flex-end">Adicionar</button>
        </div>
        <div style="display:flex; justify-content:flex-end; margin-top:14px"><button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button></div>
      </div>
    `;
    const m = modal(html);
    m.querySelector('#btAddTipo').onclick = async () => {
      const nome = m.querySelector('#novoTipoNome').value.trim();
      if (!nome) return;
      await api('/api/escalas/tipos', { method: 'POST', body: JSON.stringify({ nome }) });
      toast('Tipo adicionado');
      m.closest('.modal-mask').remove();
      modalGerenciarTipos(aoConcluir);
      if (aoConcluir) aoConcluir();
    };
    m.querySelectorAll('button[data-deltipo]').forEach(b => {
      b.onclick = async () => {
        if (!(await confirmar('Excluir este tipo de posto?'))) return;
        await api('/api/escalas/tipos/' + b.dataset.deltipo, { method: 'DELETE' });
        m.closest('.modal-mask').remove();
        modalGerenciarTipos(aoConcluir);
        if (aoConcluir) aoConcluir();
      };
    });
  }

  async function modalNovoTurno(turno, aoConcluir) {
    const [tiposRes, pesRes] = await Promise.all([api('/api/escalas/tipos'), api('/api/pessoas')]);
    const tipos = tiposRes.tipos || [];
    const pessoas = pesRes.pessoas || [];
    const html = `
      <div class="modal" style="max-width:440px">
        <h3 style="margin-top:0">${turno ? 'Editar Posto Manual' : 'Novo Posto Manual'}</h3>
        <div class="campo"><label>Tipo de Serviço *</label>
          <select id="mManTipo">${tipos.map(t => `<option value="${t.id}">${esc(t.nome)}</option>`).join('')}</select>
        </div>
        <div class="campo"><label>Data Início *</label><input type="datetime-local" id="mManIni" value="${diaSelecionado}T07:00"></div>
        <div class="campo"><label>Data Término *</label><input type="datetime-local" id="mManFim" value="${diaSelecionado}T19:00"></div>
        <div class="campo"><label>Militar Escalado</label>
          <select id="mManPes"><option value="">— Sem militar (Vago) —</option>${pessoas.map(p => `<option value="${p.id}">${esc(p.nome_guerra)} (${esc(p.nome_completo)})</option>`).join('')}</select>
        </div>
        <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:14px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="btSalvarMan">Salvar</button>
        </div>
      </div>
    `;
    const m = modal(html);
    m.querySelector('#btSalvarMan').onclick = async () => {
      const tipoID = +m.querySelector('#mManTipo').value;
      const di = m.querySelector('#mManIni').value;
      const df = m.querySelector('#mManFim').value;
      const pid = +m.querySelector('#mManPes').value || null;
      const pesList = pid ? [{ pessoa_id: pid, funcao_escala: '' }] : [];
      try {
        await api('/api/escalas/turnos', {
          method: 'POST',
          body: JSON.stringify({ tipo_id: tipoID, data_inicio: di, data_fim: df, pessoas: pesList })
        });
        toast('Posto criado com sucesso!');
        m.closest('.modal-mask').remove();
        if (aoConcluir) aoConcluir();
      } catch (e) { alerta('Erro ao salvar posto: ' + (e.message || e)); }
    };
  }
})();
