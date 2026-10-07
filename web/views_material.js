(function () {
  'use strict';

  /* =====================================================================
     #/material — MÓDULO DE MATERIAL, RESERVA & CAUTELAS COM ANEXOS (v1.0)
     ===================================================================== */

  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const modal = (...args) => window.modal(...args);
  const alerta = (...args) => window.alerta(...args);
  let abaMat = 'balcao'; // balcao | inventario | historico

  window.ViewMaterial = async function () {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    if (eu.papel === 'admin') { location.hash = '#/admin'; return; }
    navAtiva('#/material');

    const app = document.getElementById('app');
    const abas = [
      ['balcao', '⚡ Balcão Express'],
      ['inventario', '📦 Inventário & Carga'],
      ['garagem', '🚘 Garagem & Frota'],
      ['conferencia', '🎯 Check Diário (Pronto)'],
      ['historico', '📜 Histórico de Cautelas']
    ];

    app.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:12px; margin-bottom:16px">
        <div>
          <h2 style="margin:0 0 4px">Reserva de Material, Armaria & Cautelas</h2>
          <p style="color:var(--tx2); font-size:13px; margin:0">Controle de carga por setor, viaturas, chaves, check diário e relatórios de pronto.</p>
        </div>
        <div style="display:flex; gap:8px; flex-wrap:wrap">
          <button class="acao-linha" id="btResponsaveisMaterial" title="Definir Encarregado e Auxiliar de Material por Setor">👤 Encarregados</button>
          <button class="acao-linha" id="btImprimirInventario" style="color:var(--verde-claro)">📄 Imprimir Inventário</button>
          <button class="acao-linha" id="btAjudaScanner" title="Guia de uso de leitores de código e câmera">❓ Ajuda</button>
          <button class="acao-linha" id="btScannerMaterial">📷 Escanear QR / Código</button>
          <button class="acao-linha" id="btNovoItemMaterial">+ Novo Item / Viatura</button>
          <button class="primario" id="btIniciarCautelaTopo" style="box-shadow: 0 4px 14px rgba(16,185,129,0.35)">
            ⚡ Iniciar Cautela
          </button>
        </div>
      </div>

      <!-- Resumo / Métricas Rápidas -->
      <div id="matResumo" class="resumo" style="margin-bottom:14px">
        <div class="carregando">Carregando métricas…</div>
      </div>

      <!-- Abas -->
      <div class="abas" style="margin-bottom:14px">
        ${abas.map(([k, t]) => `<button data-a="${k}" class="${abaMat === k ? 'ativo' : ''}">${t}</button>`).join('')}
      </div>

      <div id="matConteudo"><div class="carregando">Carregando…</div></div>
    `;

    document.querySelectorAll('.abas button').forEach(b => {
      b.onclick = () => {
        abaMat = b.dataset.a;
        window.ViewMaterial();
      };
    });

    $('#btResponsaveisMaterial').onclick = () => modalGerenciarResponsaveis();
    $('#btImprimirInventario').onclick = () => window.open('/api/material/inventario/pdf', '_blank');
    $('#btAjudaScanner').onclick = () => modalAjudaScanner();
    $('#btScannerMaterial').onclick = () => modalScannerMaterial();
    $('#btNovoItemMaterial').onclick = () => modalNovoItem(null, () => window.ViewMaterial());
    $('#btIniciarCautelaTopo').onclick = () => modalIniciarCautelaGeral(() => window.ViewMaterial());

    await carregarMetricas();

    if (abaMat === 'balcao') await renderBalcao();
    else if (abaMat === 'inventario') await renderInventario();
    else if (abaMat === 'garagem') await renderGaragem();
    else if (abaMat === 'conferencia') await renderConferenciasMaterial();
    else await renderHistorico();
  };

  let ITENS_CACHE = [];

  async function carregarMetricas() {
    try {
      const res = await api('/api/material/itens');
      ITENS_CACHE = res.itens || [];
      const total = ITENS_CACHE.length;
      const disp = ITENS_CACHE.filter(x => x.status === 'disponivel').length;
      const acaut = ITENS_CACHE.filter(x => x.status === 'acautelado').length;
      const manut = ITENS_CACHE.filter(x => x.status === 'manutencao' || x.status === 'baixado').length;

      $('#matResumo').innerHTML = `
        <div class="caixa"><b>${total}</b><span>Total no Inventário</span></div>
        <div class="caixa"><b style="color:var(--verde-claro)">${disp}</b><span>Disponíveis na Reserva</span></div>
        <div class="caixa"><b style="color:var(--ambar-txt)">${acaut}</b><span>Acautelados (Em Uso)</span></div>
        <div class="caixa"><b style="color:var(--tx3)">${manut}</b><span>Manutenção / Baixados</span></div>
      `;
    } catch (e) {
      $('#matResumo').innerHTML = '<span class="vazio">Falha ao carregar indicadores de material.</span>';
    }
  }

  /* ---------- Aba 1: Balcão de Cautela Express ---------- */
  async function renderBalcao() {
    const cont = $('#matConteudo');
    const acautelados = ITENS_CACHE.filter(x => x.status === 'acautelado');
    const disponiveis = ITENS_CACHE.filter(x => x.status === 'disponivel');

    cont.innerHTML = `
      <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(360px, 1fr)); gap:16px">
        
        <!-- Coluna 1: Acautelados no Momento -->
        <div class="cartao" style="border-top:3px solid var(--ambar)">
          <h3 style="margin-top:0; color:var(--ambar-txt); display:flex; align-items:center; gap:8px">
            <span>⚠️</span> <span>Itens Acautelados no Momento (${acautelados.length})</span>
          </h3>
          <p style="color:var(--tx2); font-size:12.5px; margin-bottom:12px">Bens em uso. Clique para devolver ou gerenciar documentos anexos.</p>
          
          <div style="display:flex; flex-direction:column; gap:8px; max-height:480px; overflow-y:auto">
            ${acautelados.length ? acautelados.map(it => {
              const c = it.cautela_ativa || {};
              const horasFora = c.data_saida ? Math.round((Date.now() - new Date(c.data_saida).getTime()) / 3600000) : 0;
              const atrasado = horasFora > 24;
              const sens = it.sensibilidade || (it.nivel_sensibilidade === 'restrito' || it.nivel_sensibilidade === 'sensivel' ? 'controlado' : 'convencional');
              return `
                <div style="background:var(--painel2); border:1px solid ${atrasado ? 'var(--verm)' : 'var(--borda)'}; border-radius:var(--raio); padding:12px; ${atrasado ? 'box-shadow: 0 0 10px rgba(239,68,68,0.15)' : ''}">
                  <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:6px">
                    <div>
                      <b style="font-size:14px">${esc(it.nome)}</b>
                      <span style="font-size:11.5px; color:var(--tx3); margin-left:6px">#${esc(it.codigo_patrimonio)}</span>
                      ${sens === 'controlado' ? `<span style="font-size:10px; margin-left:6px; font-weight:700; text-transform:uppercase; padding:1px 5px; border-radius:3px; background:rgba(239,68,68,0.2); color:var(--verm)">Controlado</span>` : ''}
                    </div>
                    <div style="display:flex; gap:4px">
                      ${atrasado ? `<span style="font-size:10px; background:#ef4444; color:#fff; padding:2px 6px; border-radius:4px; font-weight:700">⏰ ATRASADO (+${horasFora}h)</span>` : ''}
                      <span style="font-size:11px; background:rgba(245,158,11,.15); color:var(--ambar-txt); padding:2px 6px; border-radius:4px; font-weight:700">EM USO</span>
                    </div>
                  </div>
                  <div style="font-size:13px; color:var(--tx2); line-height:1.4; margin-bottom:8px">
                    Retirado por: <b style="color:var(--tx)">${esc(c.pessoa_nome_guerra || c.pessoa_nome_completo || 'Militar')}</b><br>
                    <small style="color:var(--tx3)">Saída: ${(c.data_saida || '').slice(0,16).replace('T',' ')} · Resp: ${esc(c.responsavel_entrega || '—')}</small>
                    ${c.obs_saida ? `<br><small style="color:var(--tx3)">Obs: ${esc(c.obs_saida)}</small>` : ''}
                  </div>
                  <div style="display:flex; gap:6px; flex-wrap:wrap">
                    <button class="acao-linha" style="font-size:12px; padding:4px 8px" data-anexos="${c.id}" data-itemnome="${esc(it.nome)}">
                      📎 Fichas / Anexos
                    </button>
                    <button class="acao-linha" style="font-size:12px; padding:4px 8px" data-recibo="${c.id}" title="Imprimir Recibo / Ticket formal de cautela">
                      📄 Recibo
                    </button>
                    <button class="primario" style="flex:1; font-size:13px; padding:6px 0; min-width:80px" data-devolver="${c.id || it.id}" data-itemnome="${esc(it.nome)}" data-qtd="${c.quantidade || it.quantidade || 1}" data-sens="${sens}">
                      📥 Devolver
                    </button>
                  </div>
                </div>
              `;
            }).join('') : '<div class="vazio" style="padding:20px 0">Nenhum item acautelado no momento. Reserva completa!</div>'}
          </div>
        </div>

        <!-- Coluna 2: Prontos para Cautela -->
        <div class="cartao" style="border-top:3px solid var(--verde)">
          <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
            <h3 style="margin:0; color:var(--verde-claro); display:flex; align-items:center; gap:8px">
              <span>✅</span> <span>Itens Disponíveis na Reserva (${disponiveis.length})</span>
            </h3>
            <button class="primario" id="btIniciarBalcao" style="font-size:12px; padding:5px 10px">+ Cautelar</button>
          </div>
          <div class="campo" style="margin-bottom:10px">
            <input id="fBuscaDisp" placeholder="Buscar material disponível por nome ou patrimônio…">
          </div>
          <div id="listaDisp" style="display:flex; flex-direction:column; gap:6px; max-height:440px; overflow-y:auto">
            ${disponiveis.length ? disponiveis.map(it => `
              <div class="item-disp" data-texto="${esc((it.nome + ' ' + it.codigo_patrimonio + ' ' + it.categoria_nome).toLowerCase())}"
                   style="display:flex; justify-content:space-between; align-items:center; padding:10px 12px; background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio)">
                <div>
                  <b style="font-size:13.5px">${esc(it.nome)}</b>
                  <span style="font-size:11.5px; color:var(--tx3); margin-left:6px">#${esc(it.codigo_patrimonio)}</span>
                  <div style="font-size:11.5px; color:var(--tx2)">${esc(it.categoria_nome || 'Geral')} ${it.numero_serie ? '· Série: ' + esc(it.numero_serie) : ''}</div>
                </div>
                <button class="primario" style="font-size:12px; padding:5px 12px" data-cautelar="${it.id}" data-itemnome="${esc(it.nome)}">
                  ⚡ Iniciar Cautela
                </button>
              </div>
            `).join('') : '<div class="vazio">Nenhum item disponível na reserva.</div>'}
          </div>
        </div>
      </div>
    `;

    // Filtro de disponíveis
    $('#fBuscaDisp').oninput = ev => {
      const q = ev.target.value.trim().toLowerCase();
      document.querySelectorAll('#listaDisp .item-disp').forEach(el => {
        const txt = el.dataset.texto || '';
        el.style.display = (!q || txt.includes(q)) ? '' : 'none';
      });
    };

    $('#btIniciarBalcao').onclick = () => modalIniciarCautelaGeral(() => window.ViewMaterial());

    // Botões de Cautelar específicos
    cont.querySelectorAll('button[data-cautelar]').forEach(b => {
      b.onclick = () => {
        const id = +b.dataset.cautelar;
        modalCautelarItem(id, () => window.ViewMaterial());
      };
    });

    // Botões de Devolver
    cont.querySelectorAll('button[data-devolver]').forEach(b => {
      b.onclick = () => {
        const cid = +b.dataset.devolver;
        const nome = b.dataset.itemnome;
        const qtd = +b.dataset.qtd || 1;
        const sens = b.dataset.sens || 'convencional';
        modalDevolverItem(cid, nome, () => window.ViewMaterial(), qtd, sens);
      };
    });

    // Botões de Anexos
    cont.querySelectorAll('button[data-anexos]').forEach(b => {
      b.onclick = () => {
        const cid = +b.dataset.anexos;
        const nome = b.dataset.itemnome;
        modalGerenciarAnexos(cid, nome);
      };
    });

    // Botões de Recibo
    cont.querySelectorAll('button[data-recibo]').forEach(b => {
      b.onclick = () => {
        const cid = +b.dataset.recibo;
        window.open('/api/material/cautelas/' + cid + '/recibo.pdf', '_blank');
      };
    });
  }

  const EXPANDED_CONTROLADOS = window.__EXPANDED_CONTROLADOS || (window.__EXPANDED_CONTROLADOS = new Set());

  /* ---------- Aba 2: Inventário Completo ---------- */
  async function renderInventario() {
    const cont = $('#matConteudo');
    const [catsRes, setoresRes] = await Promise.all([
      api('/api/material/categorias'),
      api('/api/catalogo/setores')
    ]);
    const cats = catsRes.categorias || [];
    const setores = (setoresRes.itens || []).filter(s => s.ativo);

    // Separar itens controlados (agrupados por modelo/nome) e convencionais
    const ctrlGrupos = {};
    const convencionais = [];

    ITENS_CACHE.forEach(it => {
      const ehCtrl = it.sensibilidade === 'controlado' || it.nivel_sensibilidade === 'restrito' || it.nivel_sensibilidade === 'sensivel';
      if (ehCtrl) {
        const gk = (it.nome || '').trim().toLowerCase() + '::' + (it.categoria_id || 0);
        if (!ctrlGrupos[gk]) {
          ctrlGrupos[gk] = {
            key: gk,
            nome: it.nome,
            categoria_nome: it.categoria_nome,
            categoria_id: it.categoria_id,
            itens: []
          };
        }
        ctrlGrupos[gk].itens.push(it);
      } else {
        convencionais.push(it);
      }
    });

    const getAcoesHtml = (it, isChild = false) => {
      if (it.status === 'baixado') {
        return `
          <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-reativar="${it.id}">♻️ Reativar</button>
          <button class="acao-linha perigo" style="font-size:12px; padding:4px 8px" data-delitem="${it.id}">🗑️ Excluir Definitivo</button>
        `;
      }
      const btAcaoCautela = (it.status === 'disponivel' || (it.quantidade_disponivel !== undefined && it.quantidade_disponivel > 0))
        ? `<button class="primario" style="font-size:12px; padding:4px 8px; margin-right:4px" data-cautelar="${it.id}">⚡ Cautelar</button>`
        : (it.status === 'acautelado' && it.cautela_ativa
            ? `<button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-devolver="${it.cautela_ativa.id}" data-itemnome="${esc(it.nome)}" data-qtd="${it.cautela_ativa.quantidade || it.quantidade || 1}" data-sens="${it.sensibilidade || 'convencional'}">📥 Devolver</button>`
            : '');
      
      return `
        ${btAcaoCautela}
        <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-duplicar="${it.id}" title="Duplicar este item">📋 Duplicar</button>
        <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-qritem="${it.id}" title="Gerar e imprimir etiqueta com QR Code">🖨️ QR</button>
        <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-edititem="${it.id}">✏️ Editar</button>
        <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-baixaritem="${it.id}" title="Dar baixa no patrimônio">📦 Baixar</button>
        <button class="acao-linha perigo" style="font-size:12px; padding:4px 8px" data-delitem="${it.id}" title="Excluir item definitivamente">🗑️ Excluir</button>
      `;
    };

    let linhas = '';

    // 1. Grupos Controlados (Sanfona / Accordion)
    Object.values(ctrlGrupos).forEach(g => {
      const isExp = EXPANDED_CONTROLADOS.has(g.key);
      const dispCount = g.itens.filter(x => x.status === 'disponivel').length;
      const acautCount = g.itens.filter(x => x.status === 'acautelado').length;
      const manutCount = g.itens.filter(x => x.status === 'manutencao' || x.status === 'baixado').length;
      const primeiroId = g.itens[0]?.id;

      const textoPesquisaGrupo = (g.nome + ' ' + (g.categoria_nome || '') + ' controlado ' + g.itens.map(x => x.codigo_patrimonio + ' ' + (x.numero_serie || '')).join(' ')).toLowerCase();

      // Linha Pai (Accordion Header)
      linhas += `
        <tr class="tr-grupo-ctrl" data-gkey="${esc(g.key)}" data-status="${dispCount > 0 ? 'disponivel' : 'acautelado'}" data-texto="${esc(textoPesquisaGrupo)}" style="background:rgba(239,68,68,0.04); border-top:1px solid rgba(239,68,68,0.2)">
          <td style="width:36px; text-align:center">
            <input type="checkbox" class="chk-inv-grupo" data-gkey="${esc(g.key)}">
          </td>
          <td>
            <button type="button" class="fantasma bt-sanfona" data-gkey="${esc(g.key)}" style="min-height:26px; padding:2px 8px; font-weight:700; font-size:13px; color:var(--tx)">
              ${isExp ? '▼' : '▶'} <span style="font-size:11.5px; color:var(--tx2)">(${g.itens.length} un.)</span>
            </button>
          </td>
          <td>
            <b>${esc(g.nome)}</b>
            <div style="font-size:11px; color:var(--tx2)">Item Controlado (Gestão Individual Sanfonada)</div>
          </td>
          <td>${esc(g.categoria_nome || '—')}</td>
          <td>
            <span style="font-size:10.5px; font-weight:700; text-transform:uppercase; padding:2px 6px; border-radius:3px; background:rgba(239,68,68,0.2); color:var(--verm)">
              Controlado
            </span>
          </td>
          <td><b>${g.itens.length} un.</b></td>
          <td>
            <span style="color:var(--verde-claro); font-weight:700">${dispCount} disp.</span> /
            <span style="color:var(--ambar-txt); font-weight:700">${acautCount} acaut.</span>
          </td>
          <td style="font-size:12px; color:var(--tx2)">Clique no toggle ▶ para expandir os ${g.itens.length} itens individuais</td>
          <td style="text-align:right; white-space:nowrap">
            <button class="acao-linha" style="font-size:12px; padding:4px 8px" data-duplicar="${primeiroId}" title="Cadastrar nova unidade deste mesmo item controlado">
              📋 Duplicar (+1)
            </button>
          </td>
        </tr>
      `;

      // Linhas Filhas (Itens Individuais)
      g.itens.forEach(it => {
        const stColor = it.status === 'disponivel' ? 'var(--verde-claro)' : it.status === 'acautelado' ? 'var(--ambar-txt)' : it.status === 'manutencao' ? '#60a5fa' : 'var(--tx3)';
        const stNome = it.status === 'disponivel' ? 'Disponível' : it.status === 'acautelado' ? 'Acautelado' : it.status === 'manutencao' ? 'Manutenção' : 'Baixado';
        const displayEstilo = isExp ? '' : 'display:none;';

        linhas += `
          <tr class="tr-filho-ctrl" data-gkey="${esc(g.key)}" data-id="${it.id}" data-status="${esc(it.status)}" data-texto="${esc((it.nome + ' ' + it.codigo_patrimonio + ' ' + (it.categoria_nome || '') + ' ' + (it.numero_serie || '') + ' controlado').toLowerCase())}" style="${displayEstilo} background:rgba(255,255,255,0.015); border-left:3px solid var(--verm)">
            <td style="width:36px; text-align:center">
              <input type="checkbox" class="chk-inv-item" data-id="${it.id}" data-gkey="${esc(g.key)}">
            </td>
            <td>
              <span style="color:var(--tx3); margin-right:4px">↳</span>
              <b>#${esc(it.codigo_patrimonio)}</b>
            </td>
            <td>
              <span style="color:var(--tx2); padding-left:8px">${esc(it.nome)}</span>
            </td>
            <td>${esc(it.categoria_nome || '—')}</td>
            <td><span style="font-size:10px; color:var(--verm)">Controlado</span></td>
            <td>Série: <b>${esc(it.numero_serie || 's/n')}</b></td>
            <td><span style="color:${stColor}; font-weight:700">${stNome}</span></td>
            <td style="font-size:12px; color:var(--tx2)">${esc(it.observacao || '—')}</td>
            <td style="text-align:right; white-space:nowrap">
              ${getAcoesHtml(it, true)}
            </td>
          </tr>
        `;
      });
    });

    // 2. Itens Convencionais
    convencionais.forEach(it => {
      const stColor = it.status === 'disponivel' ? 'var(--verde-claro)' : it.status === 'acautelado' ? 'var(--ambar-txt)' : it.status === 'manutencao' ? '#60a5fa' : 'var(--tx3)';
      const stNome = it.status === 'disponivel' ? 'Disponível' : it.status === 'acautelado' ? 'Acautelado' : it.status === 'manutencao' ? 'Manutenção' : 'Baixado';
      const dispQtd = it.quantidade_disponivel !== undefined ? it.quantidade_disponivel : it.quantidade;
      const acautQtd = it.quantidade_acautelada !== undefined ? it.quantidade_acautelada : 0;

      linhas += `
        <tr data-id="${it.id}" data-status="${esc(it.status)}" data-texto="${esc((it.nome + ' ' + it.codigo_patrimonio + ' ' + (it.categoria_nome || '') + ' ' + (it.numero_serie || '') + ' convencional').toLowerCase())}">
          <td style="width:36px; text-align:center"><input type="checkbox" class="chk-inv-item" data-id="${it.id}"></td>
          <td><b>#${esc(it.codigo_patrimonio)}</b></td>
          <td><b>${esc(it.nome)}</b></td>
          <td>${esc(it.categoria_nome || '—')}</td>
          <td>
            <span style="font-size:10.5px; font-weight:700; text-transform:uppercase; padding:2px 6px; border-radius:3px; background:rgba(100,116,139,0.2); color:var(--tx2)">
              Convencional
            </span>
          </td>
          <td>
            <b>${dispQtd} / ${it.quantidade || 1} un.</b>
            ${acautQtd > 0 ? `<div style="font-size:11px; color:var(--ambar-txt)">(${acautQtd} acauteladas)</div>` : ''}
          </td>
          <td><span style="color:${stColor}; font-weight:700">${stNome}</span></td>
          <td style="font-size:12px; color:var(--tx2)">${esc(it.observacao || '—')}</td>
          <td style="text-align:right; white-space:nowrap">
            ${getAcoesHtml(it, false)}
          </td>
        </tr>
      `;
    });

    cont.innerHTML = `
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:12px">
          <div class="form-linha" style="flex:1; margin:0; flex-wrap:wrap; gap:8px">
            <div class="campo" style="flex:2; min-width:180px; margin:0">
              <input id="fBuscaInv" placeholder="Buscar por nome, patrimônio, série, setor…">
            </div>
            <div class="campo" style="flex:1; min-width:150px; margin:0">
              <select id="fSetorInv">
                <option value="">Todos os Setores (Carga Global)</option>
                <option value="0">Carga Geral da Unidade</option>
                ${setores.map(s => `<option value="${s.id}">${esc(s.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="flex:1; min-width:150px; margin:0">
              <select id="fCatInv">
                <option value="">Todas as categorias</option>
                ${cats.map(c => `<option value="${c.nome}">${esc(c.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="flex:1; min-width:150px; margin:0">
              <select id="fStatusInv">
                <option value="ativos">Itens Ativos (Ocultar Baixados)</option>
                <option value="">Todos os Itens (inclusive baixados)</option>
                <option value="disponivel">Apenas Disponíveis</option>
                <option value="acautelado">Apenas Acautelados</option>
                <option value="manutencao">Apenas Manutenção</option>
                <option value="baixado">Apenas Baixados</option>
              </select>
            </div>
          </div>
          <div style="display:flex; gap:8px; flex-wrap:wrap">
            <button class="primario" id="btImprimirLoteEtiquetas" disabled style="background:#0284c7; border-color:#0284c7" title="Imprimir 10 etiquetas por folha A4 com QR Code dos itens marcados">
              🖨️ Etiquetas em Lote (<span id="countSelEtiquetas">0</span>)
            </button>
            <button class="primario" id="btIniciarCautelaInv">⚡ Iniciar Cautela de Item</button>
          </div>
        </div>

        <div class="rolagem">
          <table id="tabInventario">
            <thead>
              <tr>
                <th style="width:36px; text-align:center"><input type="checkbox" id="chkTodosInv" title="Selecionar todos"></th>
                <th>Patrimônio</th>
                <th>Descrição do Item</th>
                <th>Categoria</th>
                <th>Setor (Carga)</th>
                <th>Sensibilidade</th>
                <th>Qtd / Série</th>
                <th>Status</th>
                <th>Observações</th>
                <th style="text-align:right">Ações</th>
              </tr>
            </thead>
            <tbody>${linhas || '<tr><td colspan="9"><span class="vazio">Nenhum item cadastrado.</span></td></tr>'}</tbody>
          </table>
        </div>
      </div>
    `;

    const atualizarBotoesLote = () => {
      const marcados = cont.querySelectorAll('.chk-inv-item:checked');
      const total = marcados.length;
      const countEl = $('#countSelEtiquetas');
      const btLote = $('#btImprimirLoteEtiquetas');
      if (countEl) countEl.innerText = total;
      if (btLote) btLote.disabled = total === 0;
    };

    const chkMaster = $('#chkTodosInv');
    if (chkMaster) {
      chkMaster.onchange = () => {
        const estado = chkMaster.checked;
        cont.querySelectorAll('#tabInventario tbody tr').forEach(tr => {
          if (tr.style.display !== 'none') {
            const chk = tr.querySelector('.chk-inv-item');
            if (chk) chk.checked = estado;
          }
        });
        atualizarBotoesLote();
      };
    }

    cont.querySelectorAll('.chk-inv-item').forEach(c => {
      c.onchange = atualizarBotoesLote;
    });

    const btLoteEtq = $('#btImprimirLoteEtiquetas');
    if (btLoteEtq) {
      btLoteEtq.onclick = () => {
        const ids = Array.from(cont.querySelectorAll('.chk-inv-item:checked')).map(c => c.dataset.id);
        if (ids.length === 0) return;
        window.open('/api/material/etiquetas-lote.pdf?ids=' + ids.join(',') + '&t=' + Date.now(), '_blank');
      };
    }

    const filtrar = () => {
      const q = ($('#fBuscaInv').value || '').trim().toLowerCase();
      const cat = ($('#fCatInv').value || '').trim().toLowerCase();
      const stFiltro = ($('#fStatusInv').value || '').trim();
      const setorFiltro = ($('#fSetorInv') ? $('#fSetorInv').value : '').trim();

      document.querySelectorAll('#tabInventario tbody tr').forEach(tr => {
        const txt = tr.dataset.texto || '';
        const st = tr.dataset.status || '';
        const sId = tr.dataset.setor || '';

        let okStatus = true;
        if (stFiltro === 'ativos') {
          okStatus = (st !== 'baixado');
        } else if (stFiltro) {
          okStatus = (st === stFiltro);
        }

        let okSetor = true;
        if (setorFiltro !== '') {
          okSetor = (sId === setorFiltro);
        }

        const ok = okStatus && okSetor && (!q || txt.includes(q)) && (!cat || txt.includes(cat));
        tr.style.display = ok ? '' : 'none';
      });
      atualizarBotoesLote();
    };

    $('#fBuscaInv').oninput = filtrar;
    if ($('#fSetorInv')) $('#fSetorInv').onchange = filtrar;
    $('#fCatInv').onchange = filtrar;
    $('#fStatusInv').onchange = filtrar;
    filtrar(); // Aplica filtro inicial (ocultando baixados por padrão)

    $('#btIniciarCautelaInv').onclick = () => modalIniciarCautelaGeral(() => window.ViewMaterial());

    cont.querySelectorAll('button[data-cautelar]').forEach(b => {
      b.onclick = () => {
        const id = +b.dataset.cautelar;
        modalCautelarItem(id, () => window.ViewMaterial());
      };
    });

    cont.querySelectorAll('button[data-devolver]').forEach(b => {
      b.onclick = () => {
        const cid = +b.dataset.devolver;
        const nome = b.dataset.itemnome;
        const qtd = +b.dataset.qtd || 1;
        const sens = b.dataset.sens || 'convencional';
        modalDevolverItem(cid, nome, () => window.ViewMaterial(), qtd, sens);
      };
    });

    cont.querySelectorAll('button[data-edititem]').forEach(b => {
      b.onclick = () => {
        const id = +b.dataset.edititem;
        const item = ITENS_CACHE.find(x => x.id === id);
        if (item) modalNovoItem(item, () => window.ViewMaterial());
      };
    });

    cont.querySelectorAll('button[data-duplicar]').forEach(b => {
      b.onclick = () => {
        const id = +b.dataset.duplicar;
        const item = ITENS_CACHE.find(x => x.id === id);
        if (item) modalNovoItem(null, () => window.ViewMaterial(), item);
      };
    });

    cont.querySelectorAll('.bt-sanfona').forEach(b => {
      b.onclick = () => {
        const gk = b.dataset.gkey;
        if (EXPANDED_CONTROLADOS.has(gk)) {
          EXPANDED_CONTROLADOS.delete(gk);
        } else {
          EXPANDED_CONTROLADOS.add(gk);
        }
        const isExp = EXPANDED_CONTROLADOS.has(gk);
        const subSpans = b.querySelectorAll('span');
        const contTxt = subSpans.length ? subSpans[0].outerHTML : '';
        b.innerHTML = `${isExp ? '▼' : '▶'} ${contTxt}`;
        cont.querySelectorAll(`.tr-filho-ctrl[data-gkey="${gk}"]`).forEach(tr => {
          tr.style.display = isExp ? '' : 'none';
        });
      };
    });

    cont.querySelectorAll('.chk-inv-grupo').forEach(chk => {
      chk.onchange = () => {
        const gk = chk.dataset.gkey;
        const est = chk.checked;
        cont.querySelectorAll(`.chk-inv-item[data-gkey="${gk}"]`).forEach(c => {
          c.checked = est;
        });
        atualizarBotoesLote();
      };
    });

    cont.querySelectorAll('button[data-qritem]').forEach(b => {
      b.onclick = () => {
        const id = +b.dataset.qritem;
        const item = ITENS_CACHE.find(x => x.id === id);
        if (item) modalVisualizarQREtiqueta(item);
      };
    });

    cont.querySelectorAll('button[data-delitem]').forEach(b => {
      b.onclick = async () => {
        const id = +b.dataset.delitem;
        const it = ITENS_CACHE.find(x => x.id === id);
        if (!it) return;

        if (it.status === 'acautelado') {
          alerta('O item "' + it.nome + '" está acautelado no momento. Realize a devolução antes de excluí-lo.');
          return;
        }

        const msg = `Tem certeza que deseja EXCLUIR DEFINITIVAMENTE o item "${it.nome}" (#${it.codigo_patrimonio})?\n\nEsta ação removerá o cadastro e histórico permanentemente.`;
        if (!(await confirmar(msg))) return;

        try {
          await api('/api/material/itens/' + id, { method: 'DELETE' });
          toast('Item "' + it.nome + '" excluído definitivamente.');
          await window.ViewMaterial();
        } catch (err) {
          alerta('Erro ao excluir item: ' + (err.message || err));
        }
      };
    });

    cont.querySelectorAll('button[data-baixaritem]').forEach(b => {
      b.onclick = async () => {
        const id = +b.dataset.baixaritem;
        const it = ITENS_CACHE.find(x => x.id === id);
        if (!it) return;

        if (it.status === 'acautelado') {
          alerta('O item "' + it.nome + '" está acautelado no momento. Realize a devolução antes de dar baixa.');
          return;
        }

        const msg = `Deseja dar BAIXA no patrimônio do item "${it.nome}" (#${it.codigo_patrimonio})?\n\nO item sairá da reserva ativa, mas o histórico de cautelas anteriores permanecerá arquivado.`;
        if (!(await confirmar(msg))) return;

        try {
          await api('/api/material/itens/' + id + '?modo=baixar', { method: 'DELETE' });
          toast('Item baixado do patrimônio.');
          await window.ViewMaterial();
        } catch (err) {
          alerta('Erro ao dar baixa no item: ' + (err.message || err));
        }
      };
    });

    cont.querySelectorAll('button[data-reativar]').forEach(b => {
      b.onclick = async () => {
        const id = +b.dataset.reativar;
        const it = ITENS_CACHE.find(x => x.id === id);
        if (!it) return;

        if (!(await confirmar(`Reativar o item "${it.nome}" (#${it.codigo_patrimonio}) como disponível na reserva?`))) return;

        try {
          await api('/api/material/itens', {
            method: 'POST',
            body: JSON.stringify({
              id: it.id,
              grupo_id: it.grupo_id,
              categoria_id: it.categoria_id,
              nome: it.nome,
              codigo_patrimonio: it.codigo_patrimonio,
              numero_serie: it.numero_serie,
              status: 'disponivel',
              observacao: it.observacao
            })
          });
          toast('Item reativado na reserva com sucesso!');
          await window.ViewMaterial();
        } catch (err) {
          alerta('Erro ao reativar item: ' + (err.message || err));
        }
      };
    });
  }

  /* ---------- Aba 3: Histórico de Cautelas & Anexos ---------- */
  async function renderHistorico() {
    const cont = $('#matConteudo');
    try {
      const res = await api('/api/material/cautelas');
      const cautelas = res.cautelas || [];

      const linhas = cautelas.map(c => {
        const stColor = c.status === 'ativa' ? 'var(--ambar-txt)' : 'var(--verde-claro)';
        const stTxt = c.status === 'ativa' ? 'EM USO' : 'DEVOLVIDA';
        return `
          <tr>
            <td>#${c.id}</td>
            <td><b>${esc(c.item_nome)}</b> <small style="color:var(--tx3)">(${esc(c.codigo_patrimonio)})</small></td>
            <td><b>${esc(c.pessoa_nome_guerra || c.pessoa_nome_completo)}</b></td>
            <td><small>${(c.data_saida || '').slice(0, 16).replace('T', ' ')}</small> <br><small style="color:var(--tx3)">Por: ${esc(c.responsavel_entrega || '—')}</small></td>
            <td><small>${c.data_devolucao ? (c.data_devolucao.slice(0, 16).replace('T', ' ') + '<br><span style="color:var(--tx3)">Rec: ' + esc(c.responsavel_recebimento || '—') + '</span>') : '—'}</small></td>
            <td><span style="color:${stColor}; font-weight:700; font-size:12px">${stTxt}</span></td>
            <td>
              <button class="acao-linha" style="font-size:12px; padding:3px 8px" data-anexoshist="${c.id}" data-itemnome="${esc(c.item_nome)}">
                📎 Documentos / Fichas
              </button>
              <button class="acao-linha" style="font-size:12px; padding:3px 8px; margin-left:4px" data-recibohist="${c.id}" title="Imprimir Recibo / Ticket formal de cautela">
                📄 Recibo
              </button>
            </td>
          </tr>
        `;
      }).join('');

      cont.innerHTML = `
        <div class="cartao">
          <h3 style="margin-top:0">Auditoria de Cautelas & Fichas Escaneadas</h3>
          <div class="rolagem">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Item / Bem</th>
                  <th>Retirado Por</th>
                  <th>Data de Saída</th>
                  <th>Devolução</th>
                  <th>Situação</th>
                  <th>Ações & Documentos</th>
                </tr>
              </thead>
              <tbody>${linhas || '<tr><td colspan="7"><span class="vazio">Nenhum registro de cautela encontrado.</span></td></tr>'}</tbody>
            </table>
          </div>
        </div>
      `;

      cont.querySelectorAll('button[data-anexoshist]').forEach(b => {
        b.onclick = () => {
          const cid = +b.dataset.anexoshist;
          const nome = b.dataset.itemnome;
          modalGerenciarAnexos(cid, nome);
        };
      });

      cont.querySelectorAll('button[data-recibohist]').forEach(b => {
        b.onclick = () => {
          const cid = +b.dataset.recibohist;
          window.open('/api/material/cautelas/' + cid + '/recibo.pdf', '_blank');
        };
      });
    } catch (e) {
      cont.innerHTML = '<span class="vazio">Falha ao carregar histórico.</span>';
    }
  }

  /* ---------- Modal: Iniciar Nova Cautela (Com Seleção de Item + Anexos) ---------- */
  async function modalIniciarCautelaGeral(onConcluido) {
    modalCautelarItem(null, onConcluido);
  }

  async function modalCautelarItem(preSelectItemId, onConcluido) {
    const [pessoasRes, itensRes] = await Promise.all([
      api('/api/pessoas'),
      api('/api/material/itens?status=disponivel')
    ]);

    const pessoas = (pessoasRes.pessoas || []).filter(p => p.status === 'ativo')
      .sort((a, b) => (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt-BR'));

    const itensDisponiveis = itensRes.itens || [];

    const html = `
      <div class="modal" style="max-width:600px; max-height:90vh; display:flex; flex-direction:column">
        <h3 style="margin-top:0">⚡ Iniciar Cautela de Material / Carga</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Preencha os dados e anexe documentos escaneados ou fotos da cautela assinada.</p>

        <div style="overflow-y:auto; flex:1; padding-right:4px">
          <div class="campo" style="margin-bottom:10px">
            <label>Item / Bem a ser Acautelado *</label>
            <select id="mItemCautelaSelect">
              <option value="">— Selecione o item disponível —</option>
              ${itensDisponiveis.map(it => `
                <option value="${it.id}" ${preSelectItemId === it.id ? 'selected' : ''}>
                  #${esc(it.codigo_patrimonio)} - ${esc(it.nome)} (${esc(it.categoria_nome || 'Geral')})
                </option>
              `).join('')}
            </select>
          </div>

          <div class="campo" id="mCampoQtdCautela" style="display:none; margin-bottom:10px">
            <label>Quantidade a Acautelar *</label>
            <input type="number" id="mInputQtdCautela" min="1" value="1" style="max-width:140px">
            <div id="mDicaQtdCautela" style="font-size:11.5px; color:var(--tx2); margin-top:2px"></div>
          </div>

          <div class="campo" style="margin-bottom:10px">
            <label>Militar Responsável pela Retirada *</label>
            <select id="mPesCautela">
              <option value="">— Selecione o militar —</option>
              ${pessoas.map(p => `<option value="${p.id}">${esc(p.nome_guerra)} (${esc(p.nome_completo)}) - ${esc(p.setor || 'Geral')}</option>`).join('')}
            </select>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label>Finalidade / Missão / Observação</label>
            <input id="mObsCautela" placeholder="ex.: Para serviço de guarda das 08h, sem avarias.">
          </div>

          <!-- Seção de Anexos / Escaneamento -->
          <div style="border-top:1px solid var(--borda); padding-top:10px; margin-top:10px">
            <label style="font-weight:700; display:block; margin-bottom:6px">📎 Anexar Documentos Escaneados / Fotos / PDFs</label>
            <p style="color:var(--tx3); font-size:12px; margin-bottom:8px">Você pode selecionar quantos arquivos desejar (ficha impressa e assinada, termo de responsabilidade, etc).</p>
            
            <input type="file" id="mInputArquivos" multiple accept="image/*,application/pdf" style="margin-bottom:8px">
            
            <div id="mListaArquivosPre" style="display:flex; flex-direction:column; gap:4px; max-height:140px; overflow-y:auto"></div>
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:16px; border-top:1px solid var(--borda); padding-top:12px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mBtnConfirmarCautela">Autorizar e Salvar Cautela</button>
        </div>
      </div>
    `;

    const m = modal(html);
    const anexosBuffer = [];

    const selItem = m.querySelector('#mItemCautelaSelect');
    const campoQtd = m.querySelector('#mCampoQtdCautela');
    const inputQtd = m.querySelector('#mInputQtdCautela');
    const dicaQtd = m.querySelector('#mDicaQtdCautela');

    const atualizarVisibilidadeQtd = () => {
      const itId = +selItem.value;
      const it = itensDisponiveis.find(x => x.id === itId);
      if (it && (it.sensibilidade === 'convencional' || (it.quantidade && it.quantidade > 1))) {
        const disp = it.quantidade_disponivel !== undefined ? it.quantidade_disponivel : it.quantidade;
        campoQtd.style.display = 'block';
        inputQtd.max = disp;
        inputQtd.value = 1;
        dicaQtd.innerText = `Disponível na reserva: ${disp} un.`;
      } else {
        campoQtd.style.display = 'none';
        inputQtd.value = 1;
      }
    };

    selItem.onchange = atualizarVisibilidadeQtd;
    atualizarVisibilidadeQtd();

    const fileInput = m.querySelector('#mInputArquivos');
    const previewList = m.querySelector('#mListaArquivosPre');

    fileInput.onchange = () => {
      const files = Array.from(fileInput.files);
      files.forEach(f => {
        const reader = new FileReader();
        reader.onload = () => {
          anexosBuffer.push({
            nome_arquivo: f.name,
            tipo_mime: f.type,
            tamanho: f.size,
            dados_base64: reader.result
          });
          renderArquivosPreview();
        };
        reader.readAsDataURL(f);
      });
      fileInput.value = '';
    };

    function renderArquivosPreview() {
      if (!anexosBuffer.length) {
        previewList.innerHTML = '';
        return;
      }
      previewList.innerHTML = anexosBuffer.map((a, idx) => `
        <div style="display:flex; justify-content:space-between; align-items:center; background:var(--painel2); border:1px solid var(--borda); border-radius:6px; padding:4px 8px; font-size:12.5px">
          <span>📄 <b>${esc(a.nome_arquivo)}</b> <small style="color:var(--tx3)">(${(a.tamanho/1024).toFixed(1)} KB)</small></span>
          <button type="button" class="acao-linha perigo" style="min-height:24px; padding:0 6px; font-size:11px" data-remanexo="${idx}">✕</button>
        </div>
      `).join('');

      previewList.querySelectorAll('button[data-remanexo]').forEach(b => {
        b.onclick = () => {
          const idx = +b.dataset.remanexo;
          anexosBuffer.splice(idx, 1);
          renderArquivosPreview();
        };
      });
    }

    m.querySelector('#mBtnConfirmarCautela').onclick = async () => {
      const itemId = +m.querySelector('#mItemCautelaSelect').value;
      const pid = +m.querySelector('#mPesCautela').value;
      const obs = m.querySelector('#mObsCautela').value.trim();
      const it = itensDisponiveis.find(x => x.id === itemId);
      let qtd = 1;
      if (campoQtd.style.display !== 'none') {
        qtd = parseInt(inputQtd.value, 10) || 1;
        const dispMax = it && (it.quantidade_disponivel !== undefined ? it.quantidade_disponivel : it.quantidade);
        if (dispMax && qtd > dispMax) {
          toast(`Quantidade solicitada (${qtd}) excede o disponível na reserva (${dispMax})`, 'erro');
          return;
        }
      }

      if (!itemId) {
        toast('Selecione o item a ser acautelado', 'erro');
        return;
      }
      if (!pid) {
        toast('Selecione o militar recebedor', 'erro');
        return;
      }

      try {
        await api('/api/material/cautelar', {
          method: 'POST',
          body: JSON.stringify({
            item_id: itemId,
            pessoa_id: pid,
            quantidade: qtd,
            obs_saida: obs,
            anexos: anexosBuffer
          })
        });
        toast('Material acautelado com sucesso!');
        m.remove();
        if (onConcluido) onConcluido();
      } catch (e) {}
    };
  }

  /* ---------- Modal: Gerenciar / Visualizar / Adicionar Anexos de Cautela ---------- */
  async function modalGerenciarAnexos(cautelaId, itemNome) {
    const res = await api(`/api/material/cautelas/${cautelaId}/anexos`);
    const anexos = res.anexos || [];

    const html = `
      <div class="modal" style="max-width:560px; max-height:86vh; display:flex; flex-direction:column">
        <h3 style="margin-top:0">📎 Documentos & Fichas Escaneadas (Cautela #${cautelaId})</h3>
        <p style="color:var(--tx2); font-size:12.5px; margin-bottom:10px">Item: <b>${esc(itemNome)}</b></p>

        <!-- Anexar Novo Arquivo -->
        <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px; margin-bottom:12px">
          <label style="font-weight:700; font-size:12px; display:block; margin-bottom:4px">+ Escanear / Anexar Mais Documentos</label>
          <input type="file" id="mAddArquivoInput" accept="image/*,application/pdf" style="font-size:12.5px">
        </div>

        <div style="overflow-y:auto; flex:1">
          <div style="display:flex; flex-direction:column; gap:6px" id="mListaDocs">
            ${anexos.length ? anexos.map(a => `
              <div style="display:flex; justify-content:space-between; align-items:center; background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:8px 12px">
                <div>
                  <div style="font-weight:700; font-size:13px">📄 ${esc(a.nome_arquivo)}</div>
                  <small style="color:var(--tx3)">${(a.tamanho/1024).toFixed(1)} KB · Anexado em ${(a.criado_em || '').slice(0,16).replace('T',' ')}</small>
                </div>
                <div style="display:flex; gap:6px">
                  <a href="/api/material/anexos/${a.id}" target="_blank" class="primario" style="font-size:12px; padding:4px 10px; display:inline-flex; align-items:center">
                    Visualizar / Baixar
                  </a>
                  <button class="acao-linha perigo" style="font-size:12px; padding:4px 8px" data-delanexo="${a.id}">
                    ✕
                  </button>
                </div>
              </div>
            `).join('') : '<div class="vazio" style="padding:16px">Nenhum documento ou ficha escaneada anexada a esta cautela.</div>'}
          </div>
        </div>

        <div style="text-align:right; margin-top:16px; border-top:1px solid var(--borda); padding-top:10px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;

    const m = modal(html);

    const inputNovo = m.querySelector('#mAddArquivoInput');
    inputNovo.onchange = () => {
      const f = inputNovo.files[0];
      if (!f) return;
      const reader = new FileReader();
      reader.onload = async () => {
        try {
          await api(`/api/material/cautelas/${cautelaId}/anexos`, {
            method: 'POST',
            body: JSON.stringify({
              nome_arquivo: f.name,
              tipo_mime: f.type,
              tamanho: f.size,
              dados_base64: reader.result
            })
          });
          toast('Documento anexado com sucesso!');
          m.remove();
          modalGerenciarAnexos(cautelaId, itemNome);
        } catch (e) {}
      };
      reader.readAsDataURL(f);
    };

    m.querySelectorAll('button[data-delanexo]').forEach(b => {
      b.onclick = async () => {
        const anexoId = +b.dataset.delanexo;
        if (!(await confirmar('Excluir este anexo?'))) return;
        try {
          await api(`/api/material/anexos/${anexoId}`, { method: 'DELETE' });
          toast('Anexo removido');
          m.remove();
          modalGerenciarAnexos(cautelaId, itemNome);
        } catch (e) {}
      };
    });
  }

  function modalDevolverItem(cautelaId, itemNome, onConcluido, cautelaQtd = 1, sensibilidade = 'convencional') {
    const ehConvencionalComQtd = sensibilidade === 'convencional' && cautelaQtd > 1;
    const html = `
      <div class="modal" style="max-width:480px">
        <h3 style="margin-top:0">📥 Receber Material: ${esc(itemNome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Confirme o retorno do item para a reserva de material.</p>

        ${ehConvencionalComQtd ? `
          <div class="campo" style="margin-bottom:12px">
            <label>Quantidade a Devolver (Saldo nesta Cautela: <b>${cautelaQtd}</b>)</label>
            <input type="number" id="mQtdDevolucao" min="1" max="${cautelaQtd}" value="${cautelaQtd}">
            <small style="color:var(--tx3); font-size:11px">Você pode devolver parcialmente (ex.: 2 de 5) ou integralmente.</small>
          </div>
        ` : ''}

        <div class="campo" style="margin-bottom:14px">
          <label>Condições de Devolução / Observação</label>
          <input id="mObsDevolucao" placeholder="ex.: Devolvido em perfeito estado e limpo.">
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mBtnConfirmarDevolucao">Confirmar Devolução</button>
        </div>
      </div>
    `;

    const m = modal(html);

    m.querySelector('#mBtnConfirmarDevolucao').onclick = async () => {
      const obs = m.querySelector('#mObsDevolucao').value.trim();
      const campoQtd = m.querySelector('#mQtdDevolucao');
      const qtd = campoQtd ? (+campoQtd.value || cautelaQtd) : cautelaQtd;
      if (ehConvencionalComQtd && (qtd <= 0 || qtd > cautelaQtd)) {
        alerta(`Quantidade inválida. Deve ser entre 1 e ${cautelaQtd}.`);
        return;
      }
      try {
        await api('/api/material/devolver', {
          method: 'POST',
          body: JSON.stringify({ cautela_id: cautelaId, obs_devolucao: obs, quantidade: qtd })
        });
        toast(qtd < cautelaQtd ? `Devolução parcial (${qtd} itens) registrada com sucesso!` : 'Material devolvido à reserva com sucesso!');
        m.closest('.modal-mask').remove();
        if (onConcluido) onConcluido();
      } catch (e) {
        alerta('Erro ao devolver: ' + (e.message || e));
      }
    };
  }

  async function modalNovoItem(itemEdicao, onConcluido, itemDuplicar = null) {
    const [catsRes, setoresRes, pessoasRes] = await Promise.all([
      api('/api/material/categorias'),
      api('/api/catalogo/setores'),
      api('/api/pessoas')
    ]);
    const cats = (catsRes.categorias || []).filter(c => c.ativo);
    const setores = (setoresRes.itens || []).filter(s => s.ativo);
    const pessoas = (pessoasRes.pessoas || []).filter(p => p.status === 'ativo')
      .sort((a, b) => (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt-BR'));

    const base = itemEdicao || itemDuplicar || {};
    const viatBase = base.viatura || {};
    const ehDuplicacao = !!itemDuplicar;
    const sensPadrao = base.sensibilidade || (base.nivel_sensibilidade === 'restrito' || base.nivel_sensibilidade === 'sensivel' ? 'controlado' : 'convencional');
    const qtdPadrao = sensPadrao === 'controlado' ? 1 : (base.quantidade || 1);

    const tituloModal = itemEdicao ? 'Editar Item / Viatura' : (ehDuplicacao ? 'Duplicar Item (Novo Registro)' : 'Cadastrar Novo Item / Viatura');

    const html = `
      <div class="modal" style="max-width:580px; max-height:88vh; overflow-y:auto">
        <h3 style="margin-top:0">${tituloModal}</h3>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label>Nome / Descrição do Item ou Viatura *</label>
            <input id="mItemNome" value="${esc(base.nome || '')}" placeholder="ex.: Viatura Marruá 3/4 Ton ou FAL 7,62mm">
          </div>
          <div class="campo" style="max-width:180px">
            <label>Sensibilidade *</label>
            <select id="mItemSensibilidade">
              <option value="convencional" ${sensPadrao === 'convencional' ? 'selected' : ''}>Convencional</option>
              <option value="controlado" ${sensPadrao === 'controlado' ? 'selected' : ''}>Controlado</option>
            </select>
          </div>
        </div>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label>Categoria *</label>
            <select id="mItemCat">
              <option value="">— Selecione —</option>
              ${cats.map(c => `<option value="${c.id}" ${base.categoria_id === c.id ? 'selected' : ''}>${esc(c.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo" style="flex:1">
            <label>Setor Responsável (Carga)</label>
            <select id="mItemSetor">
              <option value="">Carga Geral da Unidade</option>
              ${setores.map(s => `<option value="${s.id}" ${base.setor_id === s.id ? 'selected' : ''}>${esc(s.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo" id="cQtd" style="max-width:100px; display:${sensPadrao === 'convencional' ? 'block' : 'none'}">
            <label>Quantidade</label>
            <input id="mItemQtd" type="number" min="1" value="${qtdPadrao}">
          </div>
        </div>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label id="lblPatrimonio">${sensPadrao === 'controlado' ? 'Cód. Patrimônio *' : 'Cód. Patrimônio (opcional)'}</label>
            <input id="mItemCod" value="${esc(ehDuplicacao ? '' : (base.codigo_patrimonio || ''))}" placeholder="${sensPadrao === 'controlado' ? 'ex.: ARM-042' : 'ex.: MAT-LOTE (vazio = auto)'}">
          </div>
          <div class="campo" style="flex:1">
            <label id="lblSerie">${sensPadrao === 'controlado' ? 'Nº de Série' : 'Nº de Série (opcional)'}</label>
            <input id="mItemSerie" value="${esc(ehDuplicacao ? '' : (base.numero_serie || ''))}" placeholder="ex.: 481920">
          </div>
        </div>

        <!-- Seção Específica para Viaturas / Garagem -->
        <div id="blocoViatura" style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px; margin-bottom:10px">
          <b style="font-size:12.5px; color:var(--azul-claro)">🚘 Dados de Frota / Garagem (Opcional)</b>
          <div class="form-linha" style="margin:6px 0">
            <div class="campo" style="flex:1">
              <label>Placa da Viatura</label>
              <input id="mItemPlaca" value="${esc(viatBase.placa || '')}" placeholder="ex.: EB-1234">
            </div>
            <div class="campo" style="flex:1">
              <label>Renavam / Registro</label>
              <input id="mItemRenavam" value="${esc(viatBase.renavam || '')}" placeholder="ex.: 00987654321">
            </div>
          </div>
          <div class="form-linha" style="margin:6px 0">
            <div class="campo" style="flex:1">
              <label>Padrinho Titular (Membro do Pessoal)</label>
              <select id="mItemPadTit">
                <option value="">— Nenhum padrinho —</option>
                ${pessoas.map(p => `<option value="${p.id}" ${viatBase.padrinho_titular_id === p.id ? 'selected' : ''}>${esc(p.nome_guerra)} (${esc(p.nome_completo)})</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="flex:1">
              <label>Padrinho Substituto</label>
              <select id="mItemPadSub">
                <option value="">— Nenhum padrinho —</option>
                ${pessoas.map(p => `<option value="${p.id}" ${viatBase.padrinho_substituto_id === p.id ? 'selected' : ''}>${esc(p.nome_guerra)} (${esc(p.nome_completo)})</option>`).join('')}
              </select>
            </div>
          </div>
          <div class="form-linha" style="margin:6px 0">
            <div class="campo" style="flex:1">
              <label>Hodômetro Atual (km)</label>
              <input type="number" id="mItemHodometro" value="${viatBase.hodometro_atual || 0}">
            </div>
            <div class="campo" style="flex:1">
              <label>Nível de Combustível</label>
              <select id="mItemCombustivel">
                <option value="cheio" ${viatBase.combustivel_atual === 'cheio' ? 'selected' : ''}>Tanque Cheio</option>
                <option value="3/4" ${viatBase.combustivel_atual === '3/4' ? 'selected' : ''}>3/4 do Tanque</option>
                <option value="1/2" ${viatBase.combustivel_atual === '1/2' ? 'selected' : ''}>1/2 Tanque</option>
                <option value="1/4" ${viatBase.combustivel_atual === '1/4' ? 'selected' : ''}>1/4 (Reserva)</option>
              </select>
            </div>
          </div>
        </div>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label>Status</label>
            <select id="mItemStatus">
              <option value="disponivel" ${base.status === 'disponivel' || !base.status ? 'selected' : ''}>Disponível na Reserva / Garagem</option>
              <option value="manutencao" ${base.status === 'manutencao' ? 'selected' : ''}>Em Manutenção / Oficina</option>
              <option value="acautelado" ${base.status === 'acautelado' ? 'selected' : ''}>Acautelado / Em Missão</option>
              <option value="baixado" ${base.status === 'baixado' ? 'selected' : ''}>Baixado / Inativo</option>
            </select>
          </div>
        </div>

        <div class="campo" style="margin-bottom:14px">
          <label>Observações Adicionais</label>
          <input id="mItemObs" value="${esc(base.observacao || '')}" placeholder="ex.: Acessórios, ferramentas, histórico, etc.">
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mBtnSalvarItem">${itemEdicao ? 'Salvar Alterações' : 'Cadastrar Item'}</button>
        </div>
      </div>
    `;

    const m = modal(html);
    const selSens = m.querySelector('#mItemSensibilidade');
    const cQtd = m.querySelector('#cQtd');
    const lblPatrimonio = m.querySelector('#lblPatrimonio');
    const lblSerie = m.querySelector('#lblSerie');
    const inpCod = m.querySelector('#mItemCod');

    selSens.onchange = () => {
      const v = selSens.value;
      if (v === 'controlado') {
        cQtd.style.display = 'none';
        lblPatrimonio.innerText = 'Cód. Patrimônio *';
        lblSerie.innerText = 'Nº de Série';
        inpCod.placeholder = 'ex.: ARM-042';
      } else {
        cQtd.style.display = 'block';
        lblPatrimonio.innerText = 'Cód. Patrimônio (opcional)';
        lblSerie.innerText = 'Nº de Série (opcional)';
        inpCod.placeholder = 'ex.: MAT-LOTE (vazio = auto)';
      }
    };

    m.querySelector('#mBtnSalvarItem').onclick = async () => {
      const nome = m.querySelector('#mItemNome').value.trim();
      const cod = m.querySelector('#mItemCod').value.trim();
      const catId = +m.querySelector('#mItemCat').value || null;
      const setorId = +m.querySelector('#mItemSetor').value || null;
      const serie = m.querySelector('#mItemSerie').value.trim();
      const status = m.querySelector('#mItemStatus').value;
      const sens = selSens.value;
      const qtd = sens === 'controlado' ? 1 : Math.max(1, parseInt(m.querySelector('#mItemQtd').value, 10) || 1);
      const obs = m.querySelector('#mItemObs').value.trim();

      const placa = m.querySelector('#mItemPlaca').value.trim();
      const renavam = m.querySelector('#mItemRenavam').value.trim();
      const padTit = +m.querySelector('#mItemPadTit').value || null;
      const padSub = +m.querySelector('#mItemPadSub').value || null;
      const hodometro = parseInt(m.querySelector('#mItemHodometro').value, 10) || 0;
      const combustivel = m.querySelector('#mItemCombustivel').value;

      if (!nome) {
        toast('Nome do item é obrigatório', 'erro');
        return;
      }
      if (sens === 'controlado' && !cod) {
        toast('Código de Patrimônio é obrigatório para material controlado', 'erro');
        return;
      }

      try {
        await api('/api/material/itens', {
          method: 'POST',
          body: JSON.stringify({
            id: itemEdicao ? itemEdicao.id : 0,
            grupo_id: itemEdicao ? itemEdicao.grupo_id : null,
            setor_id: setorId,
            categoria_id: catId,
            nome: nome,
            codigo_patrimonio: cod,
            numero_serie: serie,
            status: status,
            sensibilidade: sens,
            quantidade: qtd,
            observacao: obs,
            placa: placa,
            renavam: renavam,
            padrinho_titular_id: padTit,
            padrinho_substituto_id: padSub,
            hodometro_atual: hodometro,
            combustivel_atual: combustivel
          })
        });
        toast(itemEdicao ? 'Item atualizado com sucesso!' : 'Item cadastrado com sucesso!');
        m.remove();
        if (onConcluido) onConcluido();
      } catch (err) {}
    };
  }

  /* ---------- Visualizar e Imprimir Etiqueta com QR Code ---------- */
  function modalVisualizarQREtiqueta(item) {
    if (!item) return;
    const html = `
      <div class="modal" style="max-width:440px; text-align:center">
        <h3 style="margin-top:0">🏷️ Etiqueta de Patrimônio</h3>
        <div id="etiquetaCard" style="background:#fff; color:#0f172a; padding:18px; border-radius:8px; border:2px solid #0f172a; margin:14px 0; text-align:center; box-shadow:0 4px 12px rgba(0,0,0,0.1)">
          <div style="font-size:11px; font-weight:800; text-transform:uppercase; letter-spacing:1px; margin-bottom:4px; color:#475569">
            SCI · CONTROLE PATRIMONIAL
          </div>
          <div style="font-size:16px; font-weight:900; margin-bottom:2px">${esc(item.nome)}</div>
          <div style="font-size:13px; font-weight:700; color:#1e293b; margin-bottom:10px">PATRIMÔNIO: #${esc(item.codigo_patrimonio)}</div>
          
          <div style="display:flex; justify-content:center; margin:10px 0">
            <img src="/api/material/itens/${item.id}/qr" alt="QR Code" style="width:160px; height:160px; border:1px solid #cbd5e1; border-radius:4px" />
          </div>

          <div style="font-size:11px; color:#64748b; margin-top:6px; display:flex; justify-content:space-between">
            <span>Cat: ${esc(item.categoria_nome || 'Geral')}</span>
            <span style="font-weight:700; text-transform:uppercase">Sens: ${esc(item.sensibilidade || (item.nivel_sensibilidade === 'restrito' || item.nivel_sensibilidade === 'sensivel' ? 'controlado' : 'convencional'))}</span>
          </div>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button>
          <button class="primario" id="btImprimirEtiqueta">🖨️ Imprimir Etiqueta</button>
        </div>
      </div>
    `;
    const m = modal(html);
    m.querySelector('#btImprimirEtiqueta').onclick = () => {
      const card = m.querySelector('#etiquetaCard');
      const w = window.open('', '_blank');
      w.document.write(`
        <html><head><title>Imprimir Etiqueta - ${item.codigo_patrimonio}</title>
        <style>
          @page { size: auto; margin: 10mm; }
          body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display:flex; justify-content:center; align-items:center; min-height:90vh; }
        </style>
        </head><body>${card.outerHTML}
        <script>window.onload=()=>{ window.print(); window.close(); }<\/script>
        </body></html>
      `);
      w.document.close();
    };
  }

  /* ---------- Scanner Híbrido de Material & Pessoal ---------- */
  function modalScannerMaterial() {
    const html = `
      <div class="modal" style="max-width:520px">
        <h3 style="margin-top:0; display:flex; align-items:center; gap:8px">
          <span>📷</span> <span>Ponto Expresso — Leitor de Código & QR</span>
        </h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">
          Aponte um leitor óptico USB/Bluetooth ou digite o patrimônio e tecle Enter. Você também pode capturar foto da etiqueta com a câmera.
        </p>

        <div class="campo" style="margin-bottom:14px">
          <label style="font-weight:700">Código de Barras / QR Code / Patrimônio *</label>
          <div style="display:flex; gap:8px">
            <input id="fScannerInput" placeholder="Ex: ARM-042 ou sci://m:10:ARM-042" autofocus style="font-size:15px; font-weight:700">
            <button class="primario" id="btProcessarScan" style="padding:0 16px">OK</button>
          </div>
        </div>

        <div style="background:var(--painel2); border:1px dashed var(--borda); border-radius:var(--raio); padding:16px; text-align:center; margin-bottom:14px">
          <button class="acao-linha" id="btAbrirCamera" style="margin-bottom:8px">📸 Capturar Imagem da Câmera</button>
          <input type="file" id="fCameraInput" accept="image/*" capture="environment" style="display:none">
          <div id="statusCamera" style="font-size:12px; color:var(--tx3)">Pronto para leitura.</div>
        </div>

        <div style="display:flex; justify-content:space-between; align-items:center">
          <button class="acao-linha" id="btAjudaScanModal" style="font-size:12px">❓ Como usar o leitor</button>
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;

    const m = modal(html);
    const inp = m.querySelector('#fScannerInput');
    inp.focus();

    const processarCodigo = (codCru) => {
      const cod = (codCru || '').trim();
      if (!cod) return;

      // Se for formato sci://m:ID:PATRIMONIO ou sci://p:ID:NOME
      let itemId = null, pat = cod;
      if (cod.startsWith('sci://m:') || cod.startsWith('sci://item:')) {
        const partes = cod.replace(/^sci:\/\/(m|item):/, '').split(':');
        itemId = +partes[0];
        if (partes[1]) pat = partes[1];
      }

      // Procurar item no cache do inventário
      let item = null;
      if (itemId) {
        item = ITENS_CACHE.find(x => x.id === itemId);
      }
      if (!item) {
        const busca = pat.toLowerCase();
        item = ITENS_CACHE.find(x => (x.codigo_patrimonio || '').toLowerCase() === busca || (x.nome || '').toLowerCase().includes(busca));
      }

      if (!item) {
        toast(`Item com código "${cod}" não encontrado no inventário`, 'erro');
        inp.select();
        return;
      }

      toast(`Item localizado: ${item.nome} (#${item.codigo_patrimonio})`);
      m.remove();

      if (item.status === 'disponivel') {
        modalCautelarItem(item.id, () => window.ViewMaterial());
      } else if (item.status === 'acautelado' && item.cautela_ativa) {
        modalDevolverItem(item.cautela_ativa.id, item.nome, () => window.ViewMaterial());
      } else {
        toast(`O item ${item.nome} está com status "${item.status}".`, 'erro');
      }
    };

    m.querySelector('#btProcessarScan').onclick = () => processarCodigo(inp.value);
    inp.onkeydown = (ev) => {
      if (ev.key === 'Enter') {
        ev.preventDefault();
        processarCodigo(inp.value);
      }
    };

    // Botão câmera / BarcodeDetector
    const btCam = m.querySelector('#btAbrirCamera');
    const fCam = m.querySelector('#fCameraInput');
    const stCam = m.querySelector('#statusCamera');

    btCam.onclick = () => fCam.click();

    fCam.onchange = async () => {
      const file = fCam.files && fCam.files[0];
      if (!file) return;
      stCam.textContent = 'Processando imagem da etiqueta…';

      if ('BarcodeDetector' in window) {
        try {
          const detector = new window.BarcodeDetector({ formats: ['qr_code', 'code_128', 'code_39', 'ean_13'] });
          const imgBitmap = await createImageBitmap(file);
          const barcodes = await detector.detect(imgBitmap);
          if (barcodes.length > 0) {
            stCam.textContent = `Código detectado: ${barcodes[0].rawValue}`;
            processarCodigo(barcodes[0].rawValue);
            return;
          }
        } catch (e) {}
      }

      stCam.textContent = 'Leitura concluída. Se o código não for lido automaticamente, digite o patrimônio acima.';
      inp.focus();
    };

    m.querySelector('#btAjudaScanModal').onclick = () => {
      m.remove();
      modalAjudaScanner();
    };
  }

  /* ---------- Modal de Ajuda do Scanner ---------- */
  function modalAjudaScanner() {
    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0; display:flex; align-items:center; gap:8px">
          <span>❓</span> <span>Instruções do Ponto Expresso de Cautela</span>
        </h3>
        <div style="font-size:13px; color:var(--tx2); line-height:1.5; margin-bottom:16px">
          <p>O leitor express suporta 3 modalidades de alta velocidade totalmente offline:</p>
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px 12px; margin-bottom:8px">
            <b style="color:var(--verde-claro)">1. Leitor Óptico USB / Bluetooth (Recomendado)</b>
            <p style="margin:4px 0 0; font-size:12px; color:var(--tx3)">Conecte a pistola leitora de código de barras no computador. Ao abrir o scanner, basta mirar no código impresso na etiqueta do material — ele preencherá e executará a ação instantaneamente.</p>
          </div>
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px 12px; margin-bottom:8px">
            <b style="color:var(--ambar-txt)">2. Câmera de Celular / Tablet</b>
            <p style="margin:4px 0 0; font-size:12px; color:var(--tx3)">Clique em "Capturar Imagem da Câmera" para fotografar o QR Code colado no material ou na credencial do militar.</p>
          </div>
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px 12px">
            <b style="color:var(--tx)">3. Digitação Direta do Patrimônio</b>
            <p style="margin:4px 0 0; font-size:12px; color:var(--tx3)">Caso a etiqueta esteja danificada, digite o número ou código de patrimônio (ex.: ARM-001) e pressione a tecla Enter.</p>
          </div>
        </div>
        <div style="display:flex; justify-content:flex-end">
          <button class="primario" onclick="this.closest('.modal-mask').remove()">Entendido</button>
        </div>
      </div>
    `;
    modal(html);
  }

  /* =====================================================================
     ABA 3: GARAGEM & FROTA (FICHAS DE VIATURAS, PADRINHOS E DOSSIÊS) (v1.5)
     ===================================================================== */
  async function renderGaragem() {
    const cont = $('#matConteudo');
    cont.innerHTML = '<div class="carregando">Carregando frota de viaturas…</div>';

    try {
      const res = await api('/api/material/itens?garagem=1');
      const viaturas = res.itens || [];

      cont.innerHTML = `
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
            <div>
              <h3 style="margin:0">🚘 Garagem de Viaturas & Controle de Frota</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:4px 0 0">Gestão de viaturas operacionais e administrativas, padrinhos e histórico de manutenção.</p>
            </div>
            <div style="display:flex; gap:8px">
              <button class="primario" id="btNovaViatura">+ Cadastrar Viatura</button>
            </div>
          </div>

          <div class="rolagem">
            <table>
              <thead>
                <tr>
                  <th>Viatura / Modelo</th>
                  <th>Placa / Chassi</th>
                  <th>Setor Responsável</th>
                  <th>Padrinhos (Titular / Substituto)</th>
                  <th>Hodômetro / Combustível</th>
                  <th>Situação</th>
                  <th style="text-align:right">Dossiê & Ações</th>
                </tr>
              </thead>
              <tbody>
                ${viaturas.length ? viaturas.map(v => {
                  const viat = v.viatura || {};
                  const st = v.status || 'disponivel';
                  const stBadge = st === 'disponivel'
                    ? '<span style="color:var(--verde-claro); font-weight:700">PRONTA (GARAGEM)</span>'
                    : (st === 'acautelado'
                        ? '<span style="color:var(--ambar-txt); font-weight:700">EM MISSÃO / FORA</span>'
                        : '<span style="color:var(--tx3); font-weight:700">OFICINA / MANUT</span>');

                  const padrinhosTxt = viat.padrinho_titular_nome
                    ? `<b>${esc(viat.padrinho_titular_nome)}</b> (Titular)<br><small style="color:var(--tx3)">Subst: ${esc(viat.padrinho_substituto_nome || '—')}</small>`
                    : '<span style="color:var(--tx3); font-style:italic">Sem padrinho designado</span>';

                  return `
                    <tr>
                      <td>
                        <b>${esc(v.nome)}</b><br>
                        <small style="color:var(--tx3)">PAT: #${esc(v.codigo_patrimonio)}</small>
                      </td>
                      <td>
                        <b style="color:var(--azul-claro)">${esc(viat.placa || 'Sem Placa')}</b><br>
                        <small style="color:var(--tx3)">${esc(viat.renavam || '—')}</small>
                      </td>
                      <td>${esc(v.setor_nome || 'Carga Geral')}</td>
                      <td>${padrinhosTxt}</td>
                      <td>
                        ${viat.hodometro_atual ? viat.hodometro_atual + ' km' : '—'}<br>
                        <small style="color:var(--tx3)">Tanque: ${esc(viat.combustivel_atual || 'cheio')}</small>
                      </td>
                      <td>${stBadge}</td>
                      <td style="text-align:right; white-space:nowrap">
                        <button class="primario" style="font-size:12px; padding:4px 10px; margin-right:4px" data-fichaviat="${v.id}">
                          📂 Ficha & Dossiê
                        </button>
                        <button class="acao-linha" style="font-size:12px; padding:4px 8px" data-edititem="${v.id}">
                          ✏️ Editar
                        </button>
                      </td>
                    </tr>
                  `;
                }).join('') : '<tr><td colspan="7"><span class="vazio">Nenhuma viatura cadastrada na garagem.</span></td></tr>'}
              </tbody>
            </table>
          </div>
        </div>
      `;

      $('#btNovaViatura').onclick = () => modalNovoItem(null, () => renderGaragem());

      cont.querySelectorAll('button[data-fichaviat]').forEach(b => {
        b.onclick = () => {
          const id = +b.dataset.fichaviat;
          modalFichaViatura(id);
        };
      });

      cont.querySelectorAll('button[data-edititem]').forEach(b => {
        b.onclick = () => {
          const id = +b.dataset.edititem;
          const it = viaturas.find(x => x.id === id);
          if (it) modalNovoItem(it, () => renderGaragem());
        };
      });

    } catch (e) {
      cont.innerHTML = '<span class="vazio">Falha ao carregar garagem de viaturas.</span>';
    }
  }

  /* ---------- Modal: Ficha da Viatura (Dossiê, Anexos, Comentários) ---------- */
  async function modalFichaViatura(itemId) {
    const [itensRes, anexosRes, comRes] = await Promise.all([
      api('/api/material/itens'),
      api(`/api/material/itens/${itemId}/anexos`),
      api(`/api/material/itens/${itemId}/comentarios`)
    ]);

    const item = (itensRes.itens || []).find(x => x.id === itemId);
    if (!item) {
      alerta('Viatura não localizada.');
      return;
    }

    const viat = item.viatura || {};
    const anexos = anexosRes.anexos || [];
    const comentarios = comRes.comentarios || [];

    const html = `
      <div class="modal" style="max-width:680px; max-height:88vh; display:flex; flex-direction:column">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:12px">
          <div>
            <h3 style="margin:0">🚘 Dossiê da Viatura: ${esc(item.nome)}</h3>
            <p style="color:var(--tx2); font-size:12.5px; margin:2px 0 0">
              Placa: <b style="color:var(--azul-claro)">${esc(viat.placa || '—')}</b> · 
              Patrimônio: #${esc(item.codigo_patrimonio)} · 
              Setor: <b>${esc(item.setor_nome || 'Carga Geral')}</b>
            </p>
          </div>
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">✕</button>
        </div>

        <!-- Cartões de Padrinhos e Status -->
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px; margin-bottom:14px">
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px">
            <span style="font-size:11px; color:var(--tx3); text-transform:uppercase; font-weight:700">Padrinho Titular</span>
            <div style="font-size:14px; font-weight:700; margin-top:2px">
              ${esc(viat.padrinho_titular_nome || 'Nenhum definido')}
            </div>
            <span style="font-size:11px; color:var(--tx3); text-transform:uppercase; font-weight:700; margin-top:6px; display:inline-block">Padrinho Substituto</span>
            <div style="font-size:13px; font-weight:600">
              ${esc(viat.padrinho_substituto_nome || 'Nenhum definido')}
            </div>
          </div>

          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px">
            <span style="font-size:11px; color:var(--tx3); text-transform:uppercase; font-weight:700">Hodômetro & Combustível</span>
            <div style="font-size:14px; font-weight:700; margin-top:2px">
              ${viat.hodometro_atual ? viat.hodometro_atual + ' km' : 'Não lançado'}
            </div>
            <div style="font-size:12px; color:var(--tx2); margin-top:4px">
              Nível do Tanque: <b>${esc(viat.combustivel_atual || 'cheio')}</b>
            </div>
          </div>
        </div>

        <div style="overflow-y:auto; flex:1; display:flex; flex-direction:column; gap:16px">
          
          <!-- Seção de Arquivos e Textos Anexos -->
          <div>
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
              <b style="font-size:13.5px">📄 Arquivos & Documentos do Veículo (${anexos.length})</b>
              <label class="acao-linha" style="font-size:12px; padding:3px 8px; cursor:pointer">
                + Anexar Documento
                <input type="file" id="inpAnexoViatura" style="display:none">
              </label>
            </div>
            <div style="display:flex; flex-direction:column; gap:6px">
              ${anexos.length ? anexos.map(a => `
                <div style="display:flex; justify-content:space-between; align-items:center; background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:6px 10px">
                  <div>
                    <b style="font-size:12.5px">${esc(a.nome_arquivo)}</b>
                    <small style="color:var(--tx3); margin-left:6px">${(a.tamanho/1024).toFixed(1)} KB · ${(a.criado_em || '').slice(0,16).replace('T',' ')}</small>
                  </div>
                  <div style="display:flex; gap:4px">
                    <a href="/api/material/item-anexos/${a.id}" target="_blank" class="primario" style="font-size:11px; padding:2px 8px">Baixar</a>
                    <button class="acao-linha perigo" style="font-size:11px; padding:2px 6px" data-delanexoviat="${a.id}">✕</button>
                  </div>
                </div>
              `).join('') : '<div class="vazio" style="padding:10px">Nenhum documento ou CRLV anexado.</div>'}
            </div>
          </div>

          <!-- Seção de Comentários / Manutenções -->
          <div>
            <b style="font-size:13.5px">📝 Diário de Bordo & Comentários Técnicos</b>
            <div style="display:flex; gap:6px; margin:8px 0">
              <input id="inpComentarioViatura" placeholder="Anotar alteração, revisão, troca de óleo ou observação técnica…" style="flex:1">
              <button class="primario" id="btAddComentarioViatura" style="font-size:12px">Adicionar</button>
            </div>
            <div style="display:flex; flex-direction:column; gap:6px; max-height:220px; overflow-y:auto">
              ${comentarios.length ? comentarios.map(c => `
                <div style="background:var(--painel2); border-left:3px solid var(--azul-claro); border-radius:4px; padding:8px 10px; font-size:12.5px">
                  <div style="display:flex; justify-content:space-between; color:var(--tx3); font-size:11px; margin-bottom:2px">
                    <b>@${esc(c.operador)}</b>
                    <span>${(c.criado_em || '').slice(0,16).replace('T',' ')}</span>
                  </div>
                  <div style="color:var(--tx)">${esc(c.texto)}</div>
                </div>
              `).join('') : '<div class="vazio" style="padding:10px">Nenhum comentário registrado no histórico.</div>'}
            </div>
          </div>

        </div>

        <div style="text-align:right; margin-top:14px; border-top:1px solid var(--borda); padding-top:10px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar Dossiê</button>
        </div>
      </div>
    `;

    const m = modal(html);

    // Upload de anexo
    const inpFile = m.querySelector('#inpAnexoViatura');
    inpFile.onchange = () => {
      const f = inpFile.files[0];
      if (!f) return;
      const reader = new FileReader();
      reader.onload = async () => {
        try {
          await api(`/api/material/itens/${itemId}/anexos`, {
            method: 'POST',
            body: JSON.stringify({
              nome_arquivo: f.name,
              tipo_mime: f.type,
              tamanho: f.size,
              dados_base64: reader.result
            })
          });
          toast('Documento anexado com sucesso!');
          m.remove();
          modalFichaViatura(itemId);
        } catch (e) {}
      };
      reader.readAsDataURL(f);
    };

    // Exclusão de anexo
    m.querySelectorAll('button[data-delanexoviat]').forEach(b => {
      b.onclick = async () => {
        const anexoId = +b.dataset.delanexoviat;
        if (!(await confirmar('Excluir este anexo?'))) return;
        try {
          await api(`/api/material/item-anexos/${anexoId}`, { method: 'DELETE' });
          toast('Anexo removido');
          m.remove();
          modalFichaViatura(itemId);
        } catch (e) {}
      };
    });

    // Inserção de comentário
    const inpTxt = m.querySelector('#inpComentarioViatura');
    m.querySelector('#btAddComentarioViatura').onclick = async () => {
      const t = inpTxt.value.trim();
      if (!t) return;
      try {
        await api(`/api/material/itens/${itemId}/comentarios`, {
          method: 'POST',
          body: JSON.stringify({ texto: t })
        });
        toast('Comentário registrado');
        m.remove();
        modalFichaViatura(itemId);
      } catch (e) {}
    };
  }

  /* =====================================================================
     ABA 4: CHECK DIÁRIO DE MATERIAL (CONFERÊNCIA & PRONTO DE MATERIAL) (v1.5)
     ===================================================================== */
  async function renderConferenciasMaterial() {
    const cont = $('#matConteudo');
    cont.innerHTML = '<div class="carregando">Carregando conferências de material…</div>';

    try {
      const [confRes, setoresRes] = await Promise.all([
        api('/api/material/conferencias'),
        api('/api/catalogo/setores')
      ]);

      const lista = confRes.conferencias || [];
      const setores = (setoresRes.itens || []).filter(s => s.ativo);

      cont.innerHTML = `
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
            <div>
              <h3 style="margin:0">🎯 Check Diário de Material & Prontos da Reserva</h3>
              <p style="color:var(--tx2); font-size:12.5px; margin:4px 0 0">
                Rotina diária de verificação física do acervo por setor, bipagem e homologação de Prontos oficiais.
              </p>
            </div>
            <div style="display:flex; gap:8px">
              <button class="primario" id="btIniciarCheckDiario" style="box-shadow: 0 4px 14px rgba(16,185,129,0.35)">
                + Iniciar Novo Check Diário
              </button>
            </div>
          </div>

          <div class="rolagem">
            <table>
              <thead>
                <tr>
                  <th>Data</th>
                  <th>Setor Auditado</th>
                  <th>Responsáveis (Encarregado / Auxiliar)</th>
                  <th>Progresso / Itens</th>
                  <th>Status</th>
                  <th style="text-align:right">Ações & Relatório</th>
                </tr>
              </thead>
              <tbody>
                ${lista.length ? lista.map(c => {
                  const fechada = c.status === 'fechada';
                  const stBadge = fechada
                    ? '<span style="color:var(--verde-claro); font-weight:700">CONCLUÍDO (PRONTO)</span>'
                    : '<span style="color:var(--ambar-txt); font-weight:700">EM ANDAMENTO</span>';

                  const respTxt = c.encarregado_nome_guerra
                    ? `<b>${esc(c.encarregado_nome_guerra)}</b><br><small style="color:var(--tx3)">Aux: ${esc(c.auxiliar_nome_guerra || '—')}</small>`
                    : '<small style="color:var(--tx3)">Sem designação formal</small>';

                  return `
                    <tr>
                      <td>
                        <b>${c.data}</b><br>
                        <small style="color:var(--tx3)">Aberta por: ${esc(c.aberta_por)}</small>
                      </td>
                      <td><b>${esc(c.setor_nome || 'Carga Geral')}</b></td>
                      <td>${respTxt}</td>
                      <td>
                        <b>${c.itens_presentes + c.itens_acautelados} / ${c.total_itens}</b> verificados<br>
                        <small style="color:var(--tx3)">(${c.itens_presentes} presentes, ${c.itens_acautelados} cautelados)</small>
                      </td>
                      <td>${stBadge}</td>
                      <td style="text-align:right; white-space:nowrap">
                        ${!fechada ? `
                          <button class="primario" style="font-size:12px; padding:4px 10px; margin-right:4px" data-executarcheck="${c.id}">
                            📷 Bipar / Executar
                          </button>
                        ` : ''}
                        <button class="acao-linha" style="font-size:12px; padding:4px 10px; color:var(--verde-claro)" data-verprontopdf="${c.id}">
                          📄 Relatório de Pronto
                        </button>
                      </td>
                    </tr>
                  `;
                }).join('') : '<tr><td colspan="6"><span class="vazio">Nenhum check diário realizado até o momento.</span></td></tr>'}
              </tbody>
            </table>
          </div>
        </div>
      `;

      $('#btIniciarCheckDiario').onclick = () => modalIniciarCheckDiario(setores);

      cont.querySelectorAll('button[data-executarcheck]').forEach(b => {
        b.onclick = () => {
          const id = +b.dataset.executarcheck;
          modalExecutarCheckDiario(id);
        };
      });

      cont.querySelectorAll('button[data-verprontopdf]').forEach(b => {
        b.onclick = () => {
          const id = +b.dataset.verprontopdf;
          window.open(`/api/material/conferencias/${id}/pronto.pdf`, '_blank');
        };
      });

    } catch (e) {
      cont.innerHTML = '<span class="vazio">Falha ao carregar conferências de material.</span>';
    }
  }

  /* ---------- Modal: Iniciar Sessão de Check Diário ---------- */
  function modalIniciarCheckDiario(setores) {
    const hoje = new Date().toISOString().slice(0, 10);
    const html = `
      <div class="modal" style="max-width:440px">
        <h3 style="margin-top:0">🎯 Abrir Sessão de Check Diário</h3>
        <p style="color:var(--tx2); font-size:13px">Selecione o setor da reserva para iniciar a contagem física e bipagem de carga.</p>

        <div class="campo" style="margin-bottom:12px">
          <label>Data de Referência *</label>
          <input type="date" id="mCheckData" value="${hoje}">
        </div>

        <div class="campo" style="margin-bottom:16px">
          <label>Setor / Carga *</label>
          <select id="mCheckSetor">
            <option value="">Carga Geral da Unidade</option>
            ${setores.map(s => `<option value="${s.id}">${esc(s.nome)}</option>`).join('')}
          </select>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="btConfirmarAbrirCheck">Iniciar Conferência</button>
        </div>
      </div>
    `;

    const m = modal(html);
    m.querySelector('#btConfirmarAbrirCheck').onclick = async () => {
      const data = m.querySelector('#mCheckData').value;
      const setorVal = m.querySelector('#mCheckSetor').value;
      const setorId = setorVal ? +setorVal : null;

      try {
        const res = await api('/api/material/conferencias/iniciar', {
          method: 'POST',
          body: JSON.stringify({ data: data, setor_id: setorId })
        });
        toast('Sessão de Check Diário iniciada!');
        m.remove();
        modalExecutarCheckDiario(res.id);
      } catch (e) {}
    };
  }

  /* ---------- Modal: Executar / Bipar Check Diário ---------- */
  async function modalExecutarCheckDiario(confId) {
    const res = await api(`/api/material/conferencias/${confId}`);
    const conf = res;
    const itens = conf.itens || [];

    const html = `
      <div class="modal" style="max-width:820px; max-height:90vh; display:flex; flex-direction:column">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:10px">
          <div>
            <h3 style="margin:0">🎯 Execução do Check Diário: ${esc(conf.setor_nome || 'Carga Geral')}</h3>
            <p style="color:var(--tx2); font-size:12.5px; margin:2px 0 0">
              Data: <b>${conf.data}</b> · 
              Encarregado: <b>${esc(conf.encarregado_nome_guerra || '—')}</b> · 
              Status: <span style="font-weight:700; text-transform:uppercase">${conf.status}</span>
            </p>
          </div>
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove(); renderConferenciasMaterial();">✕</button>
        </div>

        <!-- Área de Entrada Rápida de Bipagem -->
        <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:10px; margin-bottom:12px">
          <div style="display:flex; gap:8px">
            <input id="inpBipCheck" placeholder="Mire o leitor ou digite o Cód. de Patrimônio (ex.: ARM-001)…" style="flex:1; font-size:14px; font-weight:700">
            <button class="primario" id="btBiparCheck">Bipar Presente</button>
          </div>
          <div id="msgBipFeedback" style="font-size:12px; margin-top:4px; color:var(--tx3)">Aguardando leitura de patrimônio…</div>
        </div>

        <!-- Tabela de Itens da Conferência -->
        <div class="rolagem" style="flex:1; overflow-y:auto">
          <table>
            <thead>
              <tr>
                <th>Patrimônio</th>
                <th>Item / Descrição</th>
                <th>Sensibilidade</th>
                <th>Qtd Esperada</th>
                <th>Situação Atual</th>
                <th style="text-align:right">Ações Manuais</th>
              </tr>
            </thead>
            <tbody id="tbItensCheck">
              ${itens.map(it => {
                let badge = '<span style="color:var(--tx3); font-weight:700">NÃO CONFERIDO</span>';
                if (it.status === 'presente') badge = '<span style="color:var(--verde-claro); font-weight:700">PRONTO (PRESENTE)</span>';
                else if (it.status === 'acautelado') badge = `<span style="color:var(--ambar-txt); font-weight:700">EM CAUTELA (${esc(it.cautela_responsavel || 'Militar')})</span>`;
                else if (it.status === 'manutencao') badge = '<span style="color:var(--tx3); font-weight:700">MANUTENÇÃO</span>';

                return `
                  <tr data-itemid="${it.item_id}" data-cod="${esc(it.codigo_patrimonio)}">
                    <td><b>#${esc(it.codigo_patrimonio)}</b></td>
                    <td><b>${esc(it.nome)}</b></td>
                    <td><span style="font-size:11px; text-transform:uppercase">${esc(it.sensibilidade || 'convencional')}</span></td>
                    <td>${it.quantidade_esperada}</td>
                    <td>${badge}</td>
                    <td style="text-align:right; white-space:nowrap">
                      <button class="primario" style="font-size:11px; padding:2px 6px" data-marcarstatus="presente">Presente</button>
                      <button class="acao-linha" style="font-size:11px; padding:2px 6px" data-marcarstatus="ausente">Falta</button>
                    </td>
                  </tr>
                `;
              }).join('')}
            </tbody>
          </table>
        </div>

        <!-- Rodapé e Fechamento -->
        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:12px; border-top:1px solid var(--borda); padding-top:10px">
          <div>
            <button class="acao-linha" onclick="window.open('/api/material/conferencias/${confId}/pronto.pdf', '_blank')">
              📄 Visualizar Pronto (Rascunho)
            </button>
          </div>
          <div style="display:flex; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove(); renderConferenciasMaterial();">Salvar e Sair</button>
            <button class="primario" id="btConcluirFecharCheck" style="background:var(--verde); border-color:var(--verde)">
              ✅ Homologar & Fechar Check Diário
            </button>
          </div>
        </div>
      </div>
    `;

    const m = modal(html);
    const inp = m.querySelector('#inpBipCheck');
    const msg = m.querySelector('#msgBipFeedback');

    const registrarLeitura = async (codigo, status = 'presente') => {
      try {
        const bipRes = await api(`/api/material/conferencias/${confId}/bipar`, {
          method: 'POST',
          body: JSON.stringify({ codigo_patrimonio: codigo, status: status })
        });
        msg.innerHTML = `<span style="color:var(--verde-claro)">✓ ${esc(bipRes.nome)} (#${esc(bipRes.codigo_patrimonio)}) marcado como PRESENTE!</span>`;
        inp.value = '';
        inp.focus();
        // Recarregar modal
        m.remove();
        modalExecutarCheckDiario(confId);
      } catch (err) {
        msg.innerHTML = `<span style="color:var(--verm)">✕ Código '${esc(codigo)}' não localizado na carga deste setor.</span>`;
      }
    };

    m.querySelector('#btBiparCheck').onclick = () => {
      if (inp.value.trim()) registrarLeitura(inp.value.trim());
    };

    inp.onkeydown = (e) => {
      if (e.key === 'Enter' && inp.value.trim()) {
        e.preventDefault();
        registrarLeitura(inp.value.trim());
      }
    };

    m.querySelectorAll('button[data-marcarstatus]').forEach(b => {
      b.onclick = () => {
        const tr = b.closest('tr');
        const cod = tr.dataset.cod;
        const st = b.dataset.marcarstatus;
        registrarLeitura(cod, st);
      };
    });

    m.querySelector('#btConcluirFecharCheck').onclick = async () => {
      if (!(await confirmar('Deseja realmente homologar e fechar a conferência diária? Após o fechamento, o Pronto oficial será emitido de forma imutável.'))) return;
      try {
        await api(`/api/material/conferencias/${confId}/fechar`, { method: 'POST' });
        toast('Conferência diária concluída e homologada com sucesso!');
        m.remove();
        renderConferenciasMaterial();
        window.open(`/api/material/conferencias/${confId}/pronto.pdf`, '_blank');
      } catch (e) {}
    };

    setTimeout(() => inp.focus(), 150);
  }

  /* =====================================================================
     MODAL: GERENCIAR ENCARREGADOS E AUXILIARES DE MATERIAL (v1.5)
     ===================================================================== */
  async function modalGerenciarResponsaveis() {
    const [respRes, setoresRes, pessoasRes] = await Promise.all([
      api('/api/material/responsaveis'),
      api('/api/catalogo/setores'),
      api('/api/pessoas')
    ]);

    const responsaveis = respRes.responsaveis || [];
    const setores = (setoresRes.itens || []).filter(s => s.ativo);
    const pessoas = (pessoasRes.pessoas || []).filter(p => p.status === 'ativo')
      .sort((a, b) => (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt-BR'));

    const html = `
      <div class="modal" style="max-width:640px">
        <h3 style="margin-top:0">👤 Designação de Encarregados de Material</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">
          Defina os militares responsáveis pela carga de material do Grupo e de cada Setor (Encarregado e Auxiliar que assinam os Prontos).
        </p>

        <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:12px; margin-bottom:14px">
          <div class="campo" style="margin-bottom:8px">
            <label>Setor de Atribuição *</label>
            <select id="mRespSetor">
              <option value="">Carga Geral da Unidade</option>
              ${setores.map(s => `<option value="${s.id}">${esc(s.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="flex:1">
              <label>Encarregado de Material (Titular)</label>
              <select id="mRespEnc">
                <option value="">— Nenhum —</option>
                ${pessoas.map(p => `<option value="${p.id}">${esc(p.nome_guerra)} (${esc(p.nome_completo)})</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="flex:1">
              <label>Auxiliar do Encarregado</label>
              <select id="mRespAux">
                <option value="">— Nenhum —</option>
                ${pessoas.map(p => `<option value="${p.id}">${esc(p.nome_guerra)} (${esc(p.nome_completo)})</option>`).join('')}
              </select>
            </div>
          </div>
          <div style="text-align:right">
            <button class="primario" id="btSalvarResponsavel">+ Salvar Designação</button>
          </div>
        </div>

        <div class="rolagem">
          <table>
            <thead>
              <tr>
                <th>Setor / Carga</th>
                <th>Encarregado</th>
                <th>Auxiliar</th>
              </tr>
            </thead>
            <tbody>
              ${responsaveis.length ? responsaveis.map(r => `
                <tr>
                  <td><b>${esc(r.setor_nome)}</b></td>
                  <td><b>${esc(r.encarregado_nome_guerra || '—')}</b></td>
                  <td>${esc(r.auxiliar_nome_guerra || '—')}</td>
                </tr>
              `).join('') : '<tr><td colspan="3"><span class="vazio">Nenhum responsável registrado.</span></td></tr>'}
            </tbody>
          </table>
        </div>

        <div style="text-align:right; margin-top:14px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;

    const m = modal(html);
    m.querySelector('#btSalvarResponsavel').onclick = async () => {
      const sVal = m.querySelector('#mRespSetor').value;
      const sid = sVal ? +sVal : null;
      const eVal = m.querySelector('#mRespEnc').value;
      const eid = eVal ? +eVal : null;
      const aVal = m.querySelector('#mRespAux').value;
      const aid = aVal ? +aVal : null;

      try {
        await api('/api/material/responsaveis', {
          method: 'POST',
          body: JSON.stringify({
            setor_id: sid,
            encarregado_id: eid,
            auxiliar_encarregado_id: aid
          })
        });
        toast('Encarregados salvos com sucesso!');
        m.remove();
        modalGerenciarResponsaveis();
      } catch (e) {}
    };
  }

})();
