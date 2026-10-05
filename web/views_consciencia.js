(function () {
  'use strict';

  /* =====================================================================
     #/consciencia — MÓDULO DE CONSCIÊNCIA SITUACIONAL (v1.5)
     Comando e Visão Geral de Unidades Subordinadas, Relatórios,
     Efetivo Pronto, Conferências do Dia, Materiais Críticos e Busca Rápida.
     ===================================================================== */

  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const esc = s => window.esc(s);
  const fmtData = s => window.fmtData(s);
  const toast = (m, t) => window.toast(m, t);

  window.ViewConsciencia = async function () {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    window.navAtiva('#/consciencia');

    const app = document.getElementById('app');
    app.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:12px; margin-bottom:16px">
        <div>
          <div style="display:flex; align-items:center; gap:8px">
            <span style="font-size:22px">🌐</span>
            <h2 style="margin:0">Consciência Situacional Operacional</h2>
          </div>
          <p style="color:var(--tx2); font-size:13px; margin:4px 0 0">Visão integrada e em tempo real do grupo e todas as subunidades subordinadas.</p>
        </div>
        <div style="display:flex; gap:8px; align-items:center">
          <button class="secundario" id="btRecarregarConsciencia" style="min-height:38px">🔄 Atualizar Painel</button>
        </div>
      </div>

      <!-- Barra de Busca Rápida de Perfil / Militar -->
      <div class="cartao" style="margin-bottom:16px; background:var(--painel-escuro); border-color:rgba(16,185,129,0.3)">
        <div style="display:flex; gap:10px; align-items:center; flex-wrap:wrap">
          <div style="flex:1; min-width:260px; position:relative">
            <label style="font-size:11px; text-transform:uppercase; letter-spacing:0.5px; color:var(--tx2); font-weight:700; display:block; margin-bottom:4px">🔍 Consulta Rápida de Militar no Banco de Pessoal</label>
            <input type="text" id="inpBuscaRapidaMilitar" placeholder="Digite nome de guerra ou nome completo para buscar perfil…" style="width:100%; border-radius:8px">
          </div>
          <button class="primario" id="btBuscarRapidoMilitar" style="align-self:flex-end; min-height:40px">Consultar Perfil</button>
        </div>
        <div id="resultadoBuscaMilitar" style="margin-top:10px"></div>
      </div>

      <!-- Container do Dashboard Situacional -->
      <div id="dashboardConsciencia"><div class="carregando">Carregando indicadores consolidados…</div></div>
    `;

    // Eventos PRIMEIRO (fix onda 0510: se a carga falha, os handlers morriam
    // antes de serem ligados — botão 'Atualizar Painel' ficava morto)
    document.getElementById('btRecarregarConsciencia').onclick = () => carregarDashboard();

    // Carregar dados da API (erro fica no container, não derruba a view)
    try { await carregarDashboard(); } catch (e) {
      const cont = document.getElementById('dashboardConsciencia');
      if (cont) cont.innerHTML = '<div class="vazio">Falha ao carregar indicadores: ' + esc(String(e && e.message || e)) + '</div>';
    }

    const inpBusca = document.getElementById('inpBuscaRapidaMilitar');
    const btBusca = document.getElementById('btBuscarRapidoMilitar');

    const executarBusca = async () => {
      const q = inpBusca.value.trim().toLowerCase();
      const cont = document.getElementById('resultadoBuscaMilitar');
      if (!q || q.length < 2) {
        cont.innerHTML = '<span style="font-size:12px; color:var(--tx3)">Digite ao menos 2 letras para pesquisar.</span>';
        return;
      }
      cont.innerHTML = '<div class="carregando" style="font-size:12px">Buscando militares…</div>';
      try {
        const resp = await window.api('/api/pessoas');
        const pessoas = resp.pessoas || resp || [];
        const filtradas = pessoas.filter(p =>
          (p.nome_guerra && p.nome_guerra.toLowerCase().includes(q)) ||
          (p.nome_completo && p.nome_completo.toLowerCase().includes(q))
        ).slice(0, 8);

        if (filtradas.length === 0) {
          cont.innerHTML = '<span style="font-size:12px; color:var(--verm-txt)">Nenhum militar localizado com esse termo.</span>';
          return;
        }

        cont.innerHTML = `
          <div style="display:flex; flex-direction:column; gap:6px">
            ${filtradas.map(p => `
              <div style="display:flex; justify-content:space-between; align-items:center; background:var(--painel2); border:1px solid var(--borda); padding:8px 12px; border-radius:8px">
                <div>
                  <b>${esc(p.nome_guerra || p.nome_completo)}</b>
                  <span style="font-size:12px; color:var(--tx2); margin-left:8px">${esc(p.nome_completo || '')} · Setor: ${esc(p.setor || 'Geral')} · Status: <span class="pill pill-${(p.status || 'ativo').toLowerCase()}">${esc(p.status || 'Ativo')}</span></span>
                </div>
                <div style="display:flex; gap:6px">
                  <button class="acao-linha" onclick="window.abrirModalPerfilMilitar(${p.id})">👤 Ver Ficha</button>
                  <button class="secundario" onclick="window.open('/api/pessoas/${p.id}/pdf', '_blank')">📄 Ficha PDF</button>
                </div>
              </div>
            `).join('')}
          </div>
        `;
      } catch (err) {
        cont.innerHTML = '<span style="font-size:12px; color:var(--verm-txt)">Erro ao buscar militar: ' + esc(err.message) + '</span>';
      }
    };

    btBusca.onclick = executarBusca;
    inpBusca.onkeydown = (e) => { if (e.key === 'Enter') executarBusca(); };
  };

  async function carregarDashboard() {
    const cont = document.getElementById('dashboardConsciencia');
    if (!cont) return;
    try {
      const data = await window.api('/api/consciencia/resumo');
      renderDashboard(data);
    } catch (err) {
      cont.innerHTML = `<div class="cartao" style="border-color:var(--verm)"><p style="color:var(--verm-txt); margin:0">Erro ao carregar dados da Consciência Situacional: ${esc(err.message)}</p></div>`;
    }
  }

  function renderDashboard(d) {
    const cont = document.getElementById('dashboardConsciencia');
    if (!cont) return;

    const sub = d.subordinados || [];
    const efTotal = d.total_efetivo || 0;
    const efPresente = d.total_presentes_hoje || 0;
    const pctPronto = efTotal > 0 ? Math.round((efPresente / efTotal) * 100) : 100;

    cont.innerHTML = `
      <!-- Cards de Métricas Principais -->
      <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(210px, 1fr)); gap:12px; margin-bottom:16px">
        <div class="cartao" style="margin:0; border-left:4px solid var(--verde)">
          <div style="font-size:11px; text-transform:uppercase; color:var(--tx2); font-weight:700">Efetivo Operacional Total</div>
          <div style="font-size:26px; font-weight:800; margin:4px 0">${efTotal}</div>
          <div style="font-size:12px; color:var(--verde-claro)">● ${efPresente} presentes hoje (${pctPronto}%)</div>
        </div>
        <div class="cartao" style="margin:0; border-left:4px solid #3b82f6">
          <div style="font-size:11px; text-transform:uppercase; color:var(--tx2); font-weight:700">Escalados de Serviço Hoje</div>
          <div style="font-size:26px; font-weight:800; margin:4px 0">${d.total_escalados_hoje || 0}</div>
          <div style="font-size:12px; color:var(--tx2)">📅 Postos e plantões ativos</div>
        </div>
        <div class="cartao" style="margin:0; border-left:4px solid #f59e0b">
          <div style="font-size:11px; text-transform:uppercase; color:var(--tx2); font-weight:700">Materiais & Cautelas Ativas</div>
          <div style="font-size:26px; font-weight:800; margin:4px 0">${d.total_cautelas_ativas || 0}</div>
          <div style="font-size:12px; color:var(--tx2)">De ${d.total_materiais || 0} itens patrimoniados</div>
        </div>
        <div class="cartao" style="margin:0; border-left:4px solid #8b5cf6">
          <div style="font-size:11px; text-transform:uppercase; color:var(--tx2); font-weight:700">Conferências Realizadas Hoje</div>
          <div style="font-size:26px; font-weight:800; margin:4px 0">${d.conferencias_fechadas || 0} / ${sub.length}</div>
          <div style="font-size:12px; color:var(--tx2)">Subunidades monitoradas</div>
        </div>
      </div>

      <!-- Workflow de Sugestões Pendentes (se houver) -->
      ${(d.sugestoes_pendentes > 0) ? `
      <div class="cartao" style="background:rgba(245,158,11,0.08); border-color:rgba(245,158,11,0.4); margin-bottom:16px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px">
        <div>
          <b style="color:var(--ambar-txt)">⚠️ Fila Setorial de Sugestões:</b>
          <span style="font-size:13px; color:var(--tx2); margin-left:6px">Existem <b>${d.sugestoes_pendentes}</b> sugestões de auxiliares aguardando aprovação do Chefe de Setor.</span>
        </div>
        <button class="acao-linha" onclick="window.abrirModalSugestoesSetor()">📋 Abrir Painel de Sugestões</button>
      </div>` : ''}

      <!-- Tabela de Subunidades e Grupos Subordinados -->
      <div class="cartao" style="margin-bottom:16px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:12px; flex-wrap:wrap; gap:8px">
          <h3 style="margin:0">Panorama das Subunidades & Grupos Subordinados</h3>
          <span style="font-size:12px; color:var(--tx2)">Data de referência: ${fmtData(d.data_hoje)}</span>
        </div>

        <div class="rolagem">
          <table>
            <thead>
              <tr>
                <th>Subunidade / Grupo</th>
                <th class="num">Efetivo Ativo</th>
                <th class="num">Presentes Hoje</th>
                <th class="num">Faltas / Justif.</th>
                <th>Status Conferência Hoje</th>
                <th class="num">Cautelas Ativas</th>
                <th style="text-align:right">Ações Rápidas</th>
              </tr>
            </thead>
            <tbody>
              ${sub.map(s => {
                let badgeConf = '<span class="pill" style="background:rgba(156,163,175,0.2); color:#9ca3af">Pendente</span>';
                if (s.status_conferencia === 'fechada') {
                  badgeConf = '<span class="pill pill-presente">✅ Fechada</span>';
                } else if (s.status_conferencia === 'aberta') {
                  badgeConf = '<span class="pill" style="background:rgba(59,130,246,0.2); color:#93c5fd">⏳ Em Andamento</span>';
                }
                return `
                  <tr>
                    <td><b>${esc(s.nome)}</b></td>
                    <td class="num">${s.efetivo}</td>
                    <td class="num" style="color:var(--verde-claro)"><b>${s.presentes_hoje}</b></td>
                    <td class="num" style="color:${s.faltas_hoje > 0 ? 'var(--verm-txt)' : 'var(--tx3)'}">${s.faltas_hoje}</td>
                    <td>${badgeConf}</td>
                    <td class="num">${s.materiais_acautelados}</td>
                    <td style="text-align:right; white-space:nowrap">
                      ${s.conferencia_id ? `
                        <button class="acao-linha" style="padding:4px 8px; font-size:11.5px" onclick="window.abrirModalDetalhesConferencia(${s.conferencia_id})">🔍 Conferência</button>
                        <button class="secundario" style="padding:4px 8px; font-size:11.5px" onclick="window.abrirModalPDFConferencia(${s.conferencia_id})">📄 PDF</button>
                      ` : '<span style="font-size:11px; color:var(--tx3)">Sem conferência</span>'}
                    </td>
                  </tr>
                `;
              }).join('') || '<tr><td colspan="7" class="vazio">Nenhum grupo subordinado registrado</td></tr>'}
            </tbody>
          </table>
        </div>
      </div>
    `;
  }

  // Modal para detalhar ficha militar instantaneamente
  window.abrirModalPerfilMilitar = async function (pessoaID) {
    try {
      const p = await window.api(`/api/pessoas/${pessoaID}/ficha`);
      const html = `
        <h3 style="margin:0 0 10px">Ficha Individual do Militar</h3>
        <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:8px; padding:12px; margin-bottom:14px">
          <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
            <span style="font-size:16px; font-weight:800; color:var(--verde-claro)">${esc(p.nome_guerra)}</span>
            <span class="pill pill-${(p.status || 'ativo').toLowerCase()}">${esc(p.status || 'Ativo')}</span>
          </div>
          <div style="font-size:13px; color:var(--tx); margin-bottom:4px"><b>Nome Completo:</b> ${esc(p.nome_completo || '—')}</div>
          <div style="font-size:13px; color:var(--tx2); margin-bottom:4px"><b>Setor:</b> ${esc(p.setor || '—')} · <b>Posto/Grad.:</b> ${esc(p.funcao || '—')}</div>
          <div style="font-size:13px; color:var(--tx2)"><b>Grupo:</b> ${esc(p.grupo || '—')}</div>
        </div>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="secundario" onclick="window.open('/api/pessoas/${pessoaID}/pdf', '_blank')">🖨️ Imprimir PDF</button>
          <button class="primario" id="btFecharFichaModal">Fechar</button>
        </div>
      `;
      const h = window.abrirModal(html);
      h.modal.querySelector('#btFecharFichaModal').onclick = () => h.fechar();
    } catch (e) {
      window.toast('Falha ao abrir ficha: ' + e.message, 'erro');
    }
  };

  // Modal de Sugestões Setoriais (Workflow Auxiliar -> Chefe de Setor)
  let hSugestoes = null; // handle do modal de sugestoes (fechamento limpo)
  window.abrirModalSugestoesSetor = async function () {
    try {
      const resp = await window.api('/api/setores/sugestoes?status=pendente');
      const sugestoes = resp.sugestoes || [];

      const html = `
        <h3 style="margin:0 0 4px">📋 Workflow de Sugestões Setoriais</h3>
        <p style="font-size:12.5px; color:var(--tx2); margin:0 0 14px">Sugestões de auxiliares que requerem homologação e assinatura oficial do Chefe de Setor.</p>
        
        <div style="max-height:400px; overflow-y:auto; display:flex; flex-direction:column; gap:10px; margin-bottom:14px">
          ${sugestoes.map(s => `
            <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:8px; padding:12px">
              <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
                <span style="font-weight:700; color:var(--tx)">Setor ${esc(s.setor_tipo.toUpperCase())} · Ação: ${esc(s.tipo_acao)}</span>
                <span style="font-size:11px; color:var(--tx3)">${fmtData(s.criado_em)}</span>
              </div>
              <div style="font-size:12.5px; color:var(--tx2); margin-bottom:8px">
                Autor: <b>${esc(s.autor_nome)}</b> · Grupo: <b>${esc(s.grupo_nome)}</b>
              </div>
              <pre style="background:var(--painel-escuro); padding:6px; border-radius:6px; font-size:11px; color:var(--tx2); margin:0 0 10px; overflow-x:auto">${esc(JSON.stringify(s.dados, null, 2))}</pre>
              <div style="display:flex; justify-content:flex-end; gap:8px">
                <button class="perigo" style="padding:4px 10px; font-size:12px" onclick="window.avaliarSugestao(${s.id}, 'rejeitar')">Rejeitar</button>
                <button class="primario" style="padding:4px 10px; font-size:12px" onclick="window.avaliarSugestao(${s.id}, 'aprovar')">✅ Aprovar (Chancela do Chefe)</button>
              </div>
            </div>
          `).join('') || '<div class="vazio">Nenhuma sugestão pendente no momento.</div>'}
        </div>

        <div style="display:flex; justify-content:flex-end">
          <button class="secundario" id="btFecharSugestoesModal">Fechar</button>
        </div>
      `;
      hSugestoes = window.abrirModal(html);
      hSugestoes.modal.querySelector('#btFecharSugestoesModal').onclick = () => hSugestoes.fechar();
    } catch (e) {
      window.toast('Falha ao abrir sugestões: ' + e.message, 'erro');
    }
  };

  window.avaliarSugestao = async function (id, acao) {
    const just = prompt(`Informe a justificativa/despacho para ${acao}:`, acao === 'aprovar' ? 'Homologado pelo Chefe de Setor' : 'Rejeitado');
    if (just === null) return;
    try {
      const r = await window.api(`/api/setores/sugestoes/${id}/avaliar`, {
        method: 'POST',
        body: JSON.stringify({ acao: acao, justificativa: just })
      });
      window.toast(r.mensagem || 'Sugestão avaliada!');
      if (hSugestoes) hSugestoes.fechar();
      window.abrirModalSugestoesSetor();
      if (typeof window.ViewConsciencia === 'function') window.ViewConsciencia();
    } catch (e) {
      window.toast(e.message, 'erro');
    }
  };

})();
