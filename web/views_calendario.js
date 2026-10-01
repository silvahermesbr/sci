/* SCI — Módulo de Calendário Operacional & Mesh Unificado (v1.2 Fase 3)
   Visão em Grid Mensal consolidando Eventos da Unidade, Escalas de Serviço ativas
   e Prazos de Despachos/Fórum com Compartilhamento Granular estilo Google Calendar. */
(function () {
  'use strict';

  function quem() {
    return (typeof ME !== 'undefined' && ME) || window.ME || null;
  }

  const NOMES_MESES = [
    'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho',
    'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro'
  ];

  const DIAS_SEMANA = ['Dom', 'Seg', 'Ter', 'Qua', 'Qui', 'Sex', 'Sáb'];

  window.ViewCalendario = async function (mesParam) {
    const u = quem();
    if (!u) { location.hash = '#/login'; return; }
    if (window.navAtiva) navAtiva('#/calendario');

    const app = document.getElementById('app');
    if (!app) return;

    const hoje = new Date();
    let anoAtual = hoje.getFullYear();
    let mesAtual = hoje.getMonth() + 1; // 1 a 12

    if (mesParam && mesParam.includes('-')) {
      const pts = mesParam.split('-');
      if (pts.length === 2) {
        anoAtual = parseInt(pts[0], 10) || anoAtual;
        mesAtual = parseInt(pts[1], 10) || mesAtual;
      }
    }

    let exibirEventos = true;
    let exibirEscalas = true;
    let exibirDespachos = true;

    app.innerHTML = `
      <div class="cal-header-topo">
        <div class="cal-titulo-bloco">
          <h2 style="margin:0 0 4px">Calendário Operacional</h2>
          <p style="color:var(--tx2);font-size:13px;margin:0">Visão unificada de eventos institucionais, escalas de serviço e prazos de despachos.</p>
        </div>
        <div class="cal-acoes-topo">
          <div class="cal-nav-mes">
            <button type="button" class="btn-cal-nav" id="btMesAnt" title="Mês Anterior">‹</button>
            <button type="button" class="btn-cal-nav" id="btMesHoje">Hoje</button>
            <button type="button" class="btn-cal-nav" id="btMesProx" title="Próximo Mês">›</button>
            <span class="cal-mes-rotulo" id="lblMesAno">${NOMES_MESES[mesAtual - 1]} ${anoAtual}</span>
          </div>
          <button type="button" class="primario" id="btNovoEvento" style="display:flex;align-items:center;gap:6px">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            Novo Evento
          </button>
        </div>
      </div>

      <!-- Filtros Rápidos de Camadas (Estilo Google Calendar) -->
      <div class="cal-filtros-bar">
        <span style="font-size:12px;font-weight:600;color:var(--tx3);margin-right:8px">CAMADAS:</span>
        <label class="cal-filtro-item">
          <input type="checkbox" id="chkFiltrarEventos" checked>
          <span class="cal-cor-dot" style="background:#2563eb"></span> Eventos da Unidade
        </label>
        <label class="cal-filtro-item">
          <input type="checkbox" id="chkFiltrarEscalas" checked>
          <span class="cal-cor-dot" style="background:#059669"></span> Escalas de Serviço
        </label>
        <label class="cal-filtro-item">
          <input type="checkbox" id="chkFiltrarDespachos" checked>
          <span class="cal-cor-dot" style="background:#d97706"></span> Prazos & Despachos
        </label>
      </div>

      <!-- Grade do Calendário -->
      <div class="cal-container">
        <div class="cal-semana-dias">
          ${DIAS_SEMANA.map(d => `<div class="cal-dia-col-header">${d}</div>`).join('')}
        </div>
        <div class="cal-grade-corpo" id="calGradeCorpo">
          <div class="carregando" style="padding:40px">Carregando calendário…</div>
        </div>
      </div>
    `;

    const carregarMes = async () => {
      const grade = document.getElementById('calGradeCorpo');
      if (!grade) return;
      grade.innerHTML = '<div class="carregando" style="padding:40px">Atualizando eventos…</div>';

      const strMes = `${anoAtual}-${String(mesAtual).padStart(2, '0')}`;
      document.getElementById('lblMesAno').textContent = `${NOMES_MESES[mesAtual - 1]} ${anoAtual}`;

      try {
        const dados = await api(`/api/calendario/visao?mes=${strMes}`);
        renderizarGrade(dados);
      } catch (err) {
        console.error('[CAL]', err);
        grade.innerHTML = '<div class="carregando">Erro ao carregar o calendário operacional.</div>';
      }
    };

    const renderizarGrade = (dados) => {
      const grade = document.getElementById('calGradeCorpo');
      if (!grade) return;

      const eventos = dados.eventos || [];
      const escalas = dados.escalas || [];
      const despachos = dados.despachos || [];

      // Primeiro dia do mês e total de dias
      const primeiroDiaSemana = new Date(anoAtual, mesAtual - 1, 1).getDay(); // 0 a 6
      const totalDiasMes = new Date(anoAtual, mesAtual, 0).getDate();
      const totalDiasMesAnt = new Date(anoAtual, mesAtual - 1, 0).getDate();

      const hojeDataStr = `${hoje.getFullYear()}-${String(hoje.getMonth() + 1).padStart(2, '0')}-${String(hoje.getDate()).padStart(2, '0')}`;

      let celulasHtml = '';

      // Dias do mês anterior para completar a primeira semana
      for (let i = primeiroDiaSemana - 1; i >= 0; i--) {
        const diaNum = totalDiasMesAnt - i;
        celulasHtml += `<div class="cal-celula cal-celula-outro-mes"><span class="cal-dia-num">${diaNum}</span></div>`;
      }

      // Dias do mês corrente
      for (let dia = 1; dia <= totalDiasMes; dia++) {
        const dataIso = `${anoAtual}-${String(mesAtual).padStart(2, '0')}-${String(dia).padStart(2, '0')}`;
        const ehHoje = (dataIso === hojeDataStr);

        // Filtrar itens do dia
        const evsDia = exibirEventos ? eventos.filter(e => {
          const dtIni = (e.data_inicio || '').substring(0, 10);
          const dtFim = e.data_fim ? e.data_fim.substring(0, 10) : dtIni;
          return dataIso >= dtIni && dataIso <= dtFim;
        }) : [];

        const escDia = exibirEscalas ? escalas.filter(es => {
          const dtIni = (es.data_inicio || '').substring(0, 10);
          const dtFim = (es.data_fim || '').substring(0, 10);
          return dataIso >= dtIni && dataIso <= dtFim;
        }) : [];

        const despDia = exibirDespachos ? despachos.filter(d => (d.data || '').substring(0, 10) === dataIso) : [];

        celulasHtml += `
          <div class="cal-celula ${ehHoje ? 'cal-celula-hoje' : ''}" data-data="${dataIso}">
            <div class="cal-celula-topo">
              <span class="cal-dia-num ${ehHoje ? 'hoje-badge' : ''}">${dia}</span>
              <button type="button" class="btn-cal-add-dia" data-data="${dataIso}" title="Adicionar evento neste dia">+</button>
            </div>
            <div class="cal-itens-container">
              ${evsDia.map(ev => `
                <div class="cal-chip cal-chip-evento" style="border-left-color:${ev.cor || '#2563eb'}" data-evid="${ev.ID || ev.id}" title="${esc(ev.titulo)}">
                  <span class="chip-txt">${esc(ev.titulo)}</span>
                </div>
              `).join('')}

              ${escDia.map(es => `
                <div class="cal-chip cal-chip-escala" data-turnoid="${es.turno_id}" title="Escala: ${esc(es.tipo_nome)} (${es.total_efetivo} militares)">
                  <span class="chip-txt">🛡️ ${esc(es.tipo_nome)} (${es.total_efetivo})</span>
                </div>
              `).join('')}

              ${despDia.map(d => `
                <div class="cal-chip cal-chip-despacho" data-despid="${d.id}" title="Despacho: ${esc(d.assunto)}">
                  <span class="chip-txt">⏳ ${esc(d.assunto)}</span>
                </div>
              `).join('')}
            </div>
          </div>
        `;
      }

      // Preencher dias do mês seguinte até fechar as semanas (múltiplo de 7)
      const totalCelulas = primeiroDiaSemana + totalDiasMes;
      const celulasFaltantes = (7 - (totalCelulas % 7)) % 7;
      for (let j = 1; j <= celulasFaltantes; j++) {
        celulasHtml += `<div class="cal-celula cal-celula-outro-mes"><span class="cal-dia-num">${j}</span></div>`;
      }

      grade.innerHTML = celulasHtml;

      // Eventos de clique nas células (Ver Agenda do Dia)
      grade.querySelectorAll('.cal-celula:not(.cal-celula-outro-mes)').forEach(cel => {
        cel.onclick = (e) => {
          if (e.target.closest('.btn-cal-add-dia') || e.target.closest('.cal-chip-evento')) return;
          const dt = cel.dataset.data;
          abrirModalAgendaDia(dt, dados, () => carregarMes());
        };
      });

      // Clique em botão '+' rápido
      grade.querySelectorAll('.btn-cal-add-dia').forEach(bt => {
        bt.onclick = (e) => {
          e.stopPropagation();
          const dt = bt.dataset.data;
          abrirModalFormEvento(null, dt, () => carregarMes());
        };
      });

      // Clique no chip de evento
      grade.querySelectorAll('.cal-chip-evento').forEach(ch => {
        ch.onclick = (e) => {
          e.stopPropagation();
          const evId = +(ch.dataset.evid);
          const ev = eventos.find(item => (item.id || item.ID) === evId);
          if (ev) abrirModalDetalhesEvento(ev, () => carregarMes());
        };
      });
    };

    // Navegação de Meses
    document.getElementById('btMesAnt').onclick = () => {
      mesAtual--;
      if (mesAtual < 1) { mesAtual = 12; anoAtual--; }
      carregarMes();
    };

    document.getElementById('btMesProx').onclick = () => {
      mesAtual++;
      if (mesAtual > 12) { mesAtual = 1; anoAtual++; }
      carregarMes();
    };

    document.getElementById('btMesHoje').onclick = () => {
      anoAtual = hoje.getFullYear();
      mesAtual = hoje.getMonth() + 1;
      carregarMes();
    };

    // Filtros de Camadas
    document.getElementById('chkFiltrarEventos').onchange = (e) => {
      exibirEventos = e.target.checked;
      carregarMes();
    };
    document.getElementById('chkFiltrarEscalas').onchange = (e) => {
      exibirEscalas = e.target.checked;
      carregarMes();
    };
    document.getElementById('chkFiltrarDespachos').onchange = (e) => {
      exibirDespachos = e.target.checked;
      carregarMes();
    };

    // Botão Novo Evento
    document.getElementById('btNovoEvento').onclick = () => {
      const dtHoje = `${anoAtual}-${String(mesAtual).padStart(2, '0')}-01`;
      abrirModalFormEvento(null, dtHoje, () => carregarMes());
    };

    carregarMes();
  };

  // Modal com Agenda Completa do Dia
  function abrirModalAgendaDia(dataIso, dados, aoAtualizar) {
    const eventos = (dados.eventos || []).filter(e => {
      const dtIni = (e.data_inicio || '').substring(0, 10);
      const dtFim = e.data_fim ? e.data_fim.substring(0, 10) : dtIni;
      return dataIso >= dtIni && dataIso <= dtFim;
    });

    const escalas = (dados.escalas || []).filter(es => {
      const dtIni = (es.data_inicio || '').substring(0, 10);
      const dtFim = (es.data_fim || '').substring(0, 10);
      return dataIso >= dtIni && dataIso <= dtFim;
    });

    const despachos = (dados.despachos || []).filter(d => (d.data || '').substring(0, 10) === dataIso);

    const partes = dataIso.split('-');
    const dataFmt = `${partes[2]}/${partes[1]}/${partes[0]}`;

    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:580px;margin:auto">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px">
          <div>
            <h3 style="margin:0 0 4px">Agenda do Dia: ${esc(dataFmt)}</h3>
            <span style="font-size:12px;color:var(--tx3)">Visão consolidada de todas as operações e serviços</span>
          </div>
          <button type="button" class="btn-fechar-modal" id="btXAgenda">✕</button>
        </div>

        <div class="agenda-dia-conteudo" style="max-height:400px;overflow-y:auto;padding-right:6px">
          <!-- Seção de Eventos -->
          <div style="margin-bottom:16px">
            <h4 style="margin:0 0 8px;font-size:13px;color:var(--tx2);display:flex;justify-content:space-between">
              <span>Eventos da Unidade (${eventos.length})</span>
              <button type="button" class="btn-mini-link" id="btAddEvDia">+ Adicionar</button>
            </h4>
            ${eventos.length === 0 ? '<div style="font-size:12px;color:var(--tx3);padding:6px 0">Nenhum evento agendado.</div>' : `
              <div style="display:flex;flex-direction:column;gap:6px">
                ${eventos.map(ev => `
                  <div class="item-agenda-card" style="border-left:3px solid ${ev.cor || '#2563eb'};padding:8px 12px;background:var(--bg2);border-radius:4px">
                    <div style="display:flex;justify-content:space-between;align-items:center">
                      <strong style="font-size:13px">${esc(ev.titulo)}</strong>
                      <span class="badge-mini" style="font-size:10px">${esc(ev.tipo)}</span>
                    </div>
                    ${ev.descricao ? `<div style="font-size:12px;color:var(--tx2);margin-top:4px">${esc(ev.descricao)}</div>` : ''}
                    <div style="font-size:11px;color:var(--tx3);margin-top:4px">Responsável: ${esc(ev.autor_nome)} · Unidade: ${esc(ev.grupo_nome)}</div>
                  </div>
                `).join('')}
              </div>
            `}
          </div>

          <!-- Seção de Escalas de Serviço -->
          <div style="margin-bottom:16px">
            <h4 style="margin:0 0 8px;font-size:13px;color:var(--tx2)">Escalas de Serviço no Dia (${escalas.length})</h4>
            ${escalas.length === 0 ? '<div style="font-size:12px;color:var(--tx3);padding:6px 0">Nenhuma escala ativa cadastrada para este dia.</div>' : `
              <div style="display:flex;flex-direction:column;gap:8px">
                ${escalas.map(es => `
                  <div class="item-agenda-card" style="border-left:3px solid var(--verde);padding:8px 12px;background:var(--bg2);border-radius:4px">
                    <div style="font-weight:700;font-size:13px;color:var(--verde-claro)">🛡️ ${esc(es.tipo_nome)} — ${esc(es.grupo_nome)}</div>
                    ${es.observacao ? `<div style="font-size:12px;color:var(--tx2);margin:4px 0">${esc(es.observacao)}</div>` : ''}
                    <div style="margin-top:6px;font-size:12px">
                      <strong>Militares Escalados:</strong>
                      <div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px">
                        ${(es.militares || []).map(m => `
                          <span class="badge-mini" style="background:var(--bg3);padding:2px 8px">${esc(m.nome_guerra)} (${esc(m.funcao)})</span>
                        `).join('')}
                      </div>
                    </div>
                  </div>
                `).join('')}
              </div>
            `}
          </div>

          <!-- Seção de Despachos e Prazos -->
          <div>
            <h4 style="margin:0 0 8px;font-size:13px;color:var(--tx2)">Prazos de Despacho (${despachos.length})</h4>
            ${despachos.length === 0 ? '<div style="font-size:12px;color:var(--tx3);padding:6px 0">Nenhum despacho expirando nesta data.</div>' : `
              <div style="display:flex;flex-direction:column;gap:6px">
                ${despachos.map(d => `
                  <div class="item-agenda-card" style="border-left:3px solid var(--ambar);padding:8px 12px;background:var(--bg2);border-radius:4px">
                    <strong style="font-size:13px">⏳ ${esc(d.assunto)}</strong>
                    <div style="font-size:11px;color:var(--tx3);margin-top:4px">Remetente: ${esc(d.remetente_nome)} · Status: ${d.pendente ? 'Pendente' : 'Atendido'}</div>
                  </div>
                `).join('')}
              </div>
            `}
          </div>
        </div>

        <div style="text-align:right;margin-top:16px">
          <button type="button" class="primario" id="btFecharAgenda">Fechar</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btXAgenda').onclick = m.fechar;
    m.modal.querySelector('#btFecharAgenda').onclick = m.fechar;

    m.modal.querySelector('#btAddEvDia').onclick = () => {
      m.fechar();
      abrirModalFormEvento(null, dataIso, aoAtualizar);
    };
  }

  // Modal para Criar ou Editar Evento
  function abrirModalFormEvento(eventoExistente, dataPadrao, aoSalvar) {
    const ehEdicao = !!eventoExistente;
    const ev = eventoExistente || {
      titulo: '',
      descricao: '',
      tipo: 'evento',
      cor: '#2563eb',
      data_inicio: dataPadrao || '',
      data_fim: dataPadrao || '',
      dia_inteiro: true
    };

    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:480px;margin:auto">
        <h3 style="margin:0 0 12px">${ehEdicao ? 'Editar Evento' : 'Novo Evento no Calendário'}</h3>
        <div class="campo">
          <label>Título do Evento</label>
          <input type="text" id="evTitulo" value="${esc(ev.titulo)}" placeholder="ex: Instrução de Tiro, Reunião de Oficiais..." autofocus required>
        </div>
        <div class="campo">
          <label>Tipo de Evento</label>
          <select id="evTipo">
            <option value="evento" ${ev.tipo === 'evento' ? 'selected' : ''}>Geral / Evento</option>
            <option value="instrucao" ${ev.tipo === 'instrucao' ? 'selected' : ''}>Instrução / Treinamento</option>
            <option value="missao" ${ev.tipo === 'missao' ? 'selected' : ''}>Missão / Operação</option>
            <option value="reuniao" ${ev.tipo === 'reuniao' ? 'selected' : ''}>Reunião</option>
            <option value="formatura" ${ev.tipo === 'formatura' ? 'selected' : ''}>Formatura</option>
          </select>
        </div>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
          <div class="campo">
            <label>Data Início</label>
            <input type="date" id="evDataInicio" value="${esc((ev.data_inicio || '').substring(0, 10))}" required>
          </div>
          <div class="campo">
            <label>Data Fim (Opcional)</label>
            <input type="date" id="evDataFim" value="${esc((ev.data_fim || '').substring(0, 10))}">
          </div>
        </div>
        <div class="campo">
          <label>Cor do Marcador</label>
          <div style="display:flex;gap:8px;align-items:center">
            <input type="color" id="evCor" value="${esc(ev.cor || '#2563eb')}" style="height:38px;padding:2px;cursor:pointer">
            <span style="font-size:12px;color:var(--tx3)">Escolha a cor de destaque no calendário</span>
          </div>
        </div>
        <div class="campo">
          <label>Descrição / Observações</label>
          <textarea id="evDescricao" rows="3" placeholder="Detalhes, local, uniforme exigido...">${esc(ev.descricao || '')}</textarea>
        </div>
        <div style="display:flex;justify-content:flex-end;gap:8px;margin-top:16px">
          <button type="button" class="btn-cancelar" id="btCancEv">Cancelar</button>
          <button type="button" class="primario" id="btSalvarEv">${ehEdicao ? 'Salvar Alterações' : 'Criar Evento'}</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btCancEv').onclick = m.fechar;

    m.modal.querySelector('#btSalvarEv').onclick = async () => {
      const titulo = m.modal.querySelector('#evTitulo').value.trim();
      const dtIni = m.modal.querySelector('#evDataInicio').value;
      if (!titulo || !dtIni) { toast('Título e Data de Início são obrigatórios', 'erro'); return; }

      const dtFim = m.modal.querySelector('#evDataFim').value;
      const payload = {
        id: ehEdicao ? (ev.id || ev.ID) : 0,
        titulo: titulo,
        tipo: m.modal.querySelector('#evTipo').value,
        data_inicio: dtIni,
        data_fim: dtFim || null,
        cor: m.modal.querySelector('#evCor').value,
        descricao: m.modal.querySelector('#evDescricao').value.trim(),
        dia_inteiro: true
      };

      try {
        await api('/api/calendario/eventos', { method: 'POST', body: JSON.stringify(payload) });
        toast(ehEdicao ? 'Evento atualizado!' : 'Evento criado com sucesso!');
        m.fechar();
        if (aoSalvar) aoSalvar();
      } catch (err) {
        toast(err.message || 'Falha ao salvar evento', 'erro');
      }
    };
  }

  // Modal de Detalhes e Ações do Evento
  function abrirModalDetalhesEvento(ev, aoAtualizar) {
    const id = ev.id || ev.ID;
    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:440px;margin:auto">
        <div style="border-left:4px solid ${ev.cor || '#2563eb'};padding-left:10px;margin-bottom:12px">
          <h3 style="margin:0 0 4px">${esc(ev.titulo)}</h3>
          <span class="badge-mini">${esc(ev.tipo)}</span>
        </div>
        <div style="font-size:13px;color:var(--tx2);margin-bottom:8px">
          <strong>Período:</strong> ${(ev.data_inicio || '').substring(0, 10)} ${ev.data_fim ? 'até ' + ev.data_fim.substring(0, 10) : ''}
        </div>
        ${ev.descricao ? `<div style="font-size:13px;background:var(--bg2);padding:10px;border-radius:6px;margin-bottom:12px">${esc(ev.descricao)}</div>` : ''}
        <div style="font-size:11px;color:var(--tx3);margin-bottom:16px">
          Criado por: ${esc(ev.autor_nome)} · Unidade: ${esc(ev.grupo_nome)}
        </div>

        <div style="display:flex;gap:8px;flex-wrap:wrap;justify-content:flex-end">
          <button type="button" class="btn-acao-mini" id="btCompEvento">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><line x1="20" y1="8" x2="20" y2="14"/><line x1="23" y1="11" x2="17" y2="11"/></svg>
            Compartilhar
          </button>
          ${ev.pode_editar ? `
            <button type="button" class="btn-acao-mini" id="btEditarEv">Editar</button>
            <button type="button" class="btn-acao-mini perigo" id="btExcluirEv">Excluir</button>
          ` : ''}
          <button type="button" class="btn-cancelar" id="btFecharDetEv">Fechar</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btFecharDetEv').onclick = m.fechar;

    m.modal.querySelector('#btCompEvento').onclick = () => {
      m.fechar();
      abrirModalCompartilharEvento(id, ev.titulo);
    };

    if (ev.pode_editar) {
      m.modal.querySelector('#btEditarEv').onclick = () => {
        m.fechar();
        abrirModalFormEvento(ev, null, aoAtualizar);
      };

      m.modal.querySelector('#btExcluirEv').onclick = async () => {
        m.fechar();
        if (confirm(`Excluir o evento "${ev.titulo}"?`)) {
          try {
            await api(`/api/calendario/eventos/${id}`, { method: 'DELETE' });
            toast('Evento excluído!');
            if (aoAtualizar) aoAtualizar();
          } catch (e) {
            toast('Falha ao excluir', 'erro');
          }
        }
      };
    }
  }

  // Modal de Compartilhamento Granular estilo Google Calendar
  async function abrirModalCompartilharEvento(eventoID, titulo) {
    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:520px;margin:auto">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
          <h3 style="margin:0">Compartilhar Evento</h3>
          <button type="button" class="btn-fechar-modal" id="btXCompCal">✕</button>
        </div>
        <p style="color:var(--tx3);font-size:12px;margin:0 0 16px">Convide outras Unidades ou Militares para acompanhar "${esc(titulo)}".</p>

        <div class="cal-form-compartilhar">
          <div class="campo" style="margin-bottom:8px">
            <label>Convidar / Compartilhar com</label>
            <select id="selCalTipo">
              <option value="grupo">Grupo / Unidade</option>
              <option value="usuario">Militar / Usuário</option>
            </select>
          </div>
          <div class="campo" style="margin-bottom:12px">
            <label id="lblCalAlvo">Selecione</label>
            <select id="selCalAlvo"><option value="">Carregando…</option></select>
          </div>
          <button type="button" class="primario" id="btCalAddPermissao" style="width:100%">Compartilhar Evento</button>
        </div>

        <div style="margin-top:20px">
          <h4 style="margin:0 0 8px;font-size:13px;color:var(--tx2)">Compartilhamentos Ativos</h4>
          <div id="listaCompCalAtivos" style="max-height:160px;overflow-y:auto;border:1px solid var(--borda);border-radius:6px;padding:8px">
            <div class="carregando" style="padding:10px">Carregando…</div>
          </div>
        </div>

        <div style="text-align:right;margin-top:16px">
          <button type="button" class="primario" id="btConcluirCompCal">Concluído</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btXCompCal').onclick = m.fechar;
    m.modal.querySelector('#btConcluirCompCal').onclick = m.fechar;

    const selTipo = m.modal.querySelector('#selCalTipo');
    const selAlvo = m.modal.querySelector('#selCalAlvo');
    const lblAlvo = m.modal.querySelector('#lblCalAlvo');

    let gruposCache = [], usuariosCache = [];
    try {
      const gRes = await api('/api/grupos').catch(() => []);
      gruposCache = Array.isArray(gRes) ? gRes : (gRes.grupos || []);
    } catch (e) {}
    try {
      const uRes = await api('/api/usuarios').catch(() => []);
      usuariosCache = Array.isArray(uRes) ? uRes : (uRes.usuarios || []);
    } catch (e) {}

    const atualizarOpcoes = () => {
      const t = selTipo.value;
      if (t === 'grupo') {
        lblAlvo.textContent = 'Selecione a Unidade';
        selAlvo.innerHTML = gruposCache.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');
      } else {
        lblAlvo.textContent = 'Selecione o Usuário';
        selAlvo.innerHTML = usuariosCache.map(u => `<option value="${u.id}">${esc(u.nome_guerra || u.login)}</option>`).join('');
      }
    };
    selTipo.onchange = atualizarOpcoes;
    atualizarOpcoes();

    const carregarComps = async () => {
      const cBox = m.modal.querySelector('#listaCompCalAtivos');
      try {
        const res = await api(`/api/calendario/compartilhamentos?evento_id=${eventoID}`);
        const comps = res.compartilhamentos || [];
        if (comps.length === 0) {
          cBox.innerHTML = '<div style="font-size:12px;color:var(--tx3);text-align:center;padding:10px">Nenhum compartilhamento externo ativo.</div>';
          return;
        }
        cBox.innerHTML = comps.map(c => `
          <div style="display:flex;align-items:center;justify-content:space-between;padding:6px 4px;border-bottom:1px solid var(--borda)">
            <div>
              <div style="font-weight:600;font-size:13px">${esc(c.alvo_nome)} <span class="badge-mini" style="font-size:10px">${esc(c.alvo_tipo)}</span></div>
            </div>
            <button type="button" class="btn-icone-mini btnRevogarCompCal" data-cid="${c.id}" title="Remover" style="color:var(--verm)">✕</button>
          </div>
        `).join('');

        cBox.querySelectorAll('.btnRevogarCompCal').forEach(b => {
          b.onclick = async () => {
            const cid = +b.dataset.cid;
            try {
              await api(`/api/calendario/compartilhamentos/${cid}`, { method: 'DELETE' });
              toast('Compartilhamento removido!');
              carregarComps();
            } catch (err) {
              toast('Falha ao remover', 'erro');
            }
          };
        });
      } catch (e) {
        cBox.innerHTML = '<div style="color:var(--verm);font-size:12px">Erro ao carregar permissões.</div>';
      }
    };

    carregarComps();

    m.modal.querySelector('#btCalAddPermissao').onclick = async () => {
      const alvoId = +selAlvo.value;
      if (!alvoId) { toast('Selecione um alvo válido', 'erro'); return; }

      const payload = {
        evento_id: eventoID,
        alvo_tipo: selTipo.value,
        alvo_id: alvoId,
        pode_editar: false
      };

      try {
        await api('/api/calendario/compartilhar', { method: 'POST', body: JSON.stringify(payload) });
        toast('Evento compartilhado!');
        carregarComps();
      } catch (err) {
        toast(err.message || 'Falha ao compartilhar', 'erro');
      }
    };
  }

})();
