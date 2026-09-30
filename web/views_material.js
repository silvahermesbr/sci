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
      ['balcao', '⚡ Balcão de Cautela Express'],
      ['inventario', '📦 Inventário & Carga'],
      ['historico', '📜 Histórico de Cautelas & Anexos']
    ];

    app.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:12px; margin-bottom:16px">
        <div>
          <h2 style="margin:0 0 4px">Reserva de Material, Armaria & Cautelas</h2>
          <p style="color:var(--tx2); font-size:13px; margin:0">Controle de carga, armamento, viaturas, chaves e cautelas com escaneamento de fichas.</p>
        </div>
        <div style="display:flex; gap:8px; flex-wrap:wrap">
          <button class="acao-linha" id="btImprimirInventario" style="color:var(--verde-claro)">📄 Imprimir Inventário</button>
          <button class="acao-linha" id="btAjudaScanner" title="Guia de uso de leitores de código e câmera">❓ Ajuda</button>
          <button class="acao-linha" id="btScannerMaterial">📷 Escanear QR / Código</button>
          <button class="acao-linha" id="btNovoItemMaterial">+ Novo Item / Bem</button>
          <button class="primario" id="btIniciarCautelaTopo" style="box-shadow: 0 4px 14px rgba(16,185,129,0.35)">
            ⚡ Iniciar Nova Cautela
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

    $('#btImprimirInventario').onclick = () => window.open('/api/material/inventario/pdf', '_blank');
    $('#btAjudaScanner').onclick = () => modalAjudaScanner();
    $('#btScannerMaterial').onclick = () => modalScannerMaterial();
    $('#btNovoItemMaterial').onclick = () => modalNovoItem(null, () => window.ViewMaterial());
    $('#btIniciarCautelaTopo').onclick = () => modalIniciarCautelaGeral(() => window.ViewMaterial());

    await carregarMetricas();

    if (abaMat === 'balcao') await renderBalcao();
    else if (abaMat === 'inventario') await renderInventario();
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
              const sens = it.nivel_sensibilidade || 'padrao';
              return `
                <div style="background:var(--painel2); border:1px solid ${atrasado ? 'var(--verm)' : 'var(--borda)'}; border-radius:var(--raio); padding:12px; ${atrasado ? 'box-shadow: 0 0 10px rgba(239,68,68,0.15)' : ''}">
                  <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:6px">
                    <div>
                      <b style="font-size:14px">${esc(it.nome)}</b>
                      <span style="font-size:11.5px; color:var(--tx3); margin-left:6px">#${esc(it.codigo_patrimonio)}</span>
                      ${sens !== 'padrao' ? `<span style="font-size:10px; margin-left:6px; font-weight:700; text-transform:uppercase; padding:1px 5px; border-radius:3px; ${sens === 'restrito' ? 'background:rgba(239,68,68,0.2); color:var(--verm)' : 'background:rgba(245,158,11,0.2); color:var(--ambar-txt)'}">${esc(sens)}</span>` : ''}
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
                    <button class="primario" style="flex:1; font-size:13px; padding:6px 0; min-width:80px" data-devolver="${c.id || it.id}" data-itemnome="${esc(it.nome)}">
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
        modalDevolverItem(cid, nome, () => window.ViewMaterial());
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

  /* ---------- Aba 2: Inventário Completo ---------- */
  async function renderInventario() {
    const cont = $('#matConteudo');
    const [catsRes] = await Promise.all([api('/api/material/categorias')]);
    const cats = catsRes.categorias || [];

    const linhas = ITENS_CACHE.map(it => {
      const stColor = it.status === 'disponivel' ? 'var(--verde-claro)' : it.status === 'acautelado' ? 'var(--ambar-txt)' : it.status === 'manutencao' ? '#60a5fa' : 'var(--tx3)';
      const stNome = it.status === 'disponivel' ? 'Disponível' : it.status === 'acautelado' ? 'Acautelado' : it.status === 'manutencao' ? 'Manutenção' : 'Baixado';
      
      let acoesHtml = '';
      if (it.status === 'baixado') {
        acoesHtml = `
          <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-reativar="${it.id}">♻️ Reativar</button>
          <button class="acao-linha perigo" style="font-size:12px; padding:4px 8px" data-delitem="${it.id}">🗑️ Excluir Definitivo</button>
        `;
      } else {
        const btAcaoCautela = it.status === 'disponivel'
          ? `<button class="primario" style="font-size:12px; padding:4px 8px; margin-right:4px" data-cautelar="${it.id}">⚡ Cautelar</button>`
          : (it.status === 'acautelado' && it.cautela_ativa
              ? `<button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-devolver="${it.cautela_ativa.id}" data-itemnome="${esc(it.nome)}">📥 Devolver</button>`
              : '');
        
        acoesHtml = `
          ${btAcaoCautela}
          <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-qritem="${it.id}" title="Gerar e imprimir etiqueta com QR Code">🖨️ QR</button>
          <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-edititem="${it.id}">✏️ Editar</button>
          <button class="acao-linha" style="font-size:12px; padding:4px 8px; margin-right:4px" data-baixaritem="${it.id}" title="Dar baixa no patrimônio (mantém histórico)">📦 Baixar</button>
          <button class="acao-linha perigo" style="font-size:12px; padding:4px 8px" data-delitem="${it.id}" title="Excluir item definitivamente">🗑️ Excluir</button>
        `;
      }

      return `
        <tr data-status="${esc(it.status)}" data-texto="${esc((it.nome + ' ' + it.codigo_patrimonio + ' ' + (it.categoria_nome || '') + ' ' + (it.numero_serie || '') + ' ' + (it.nivel_sensibilidade || '')).toLowerCase())}">
          <td><b>#${esc(it.codigo_patrimonio)}</b></td>
          <td><b>${esc(it.nome)}</b></td>
          <td>${esc(it.categoria_nome || '—')}</td>
          <td>
            <span style="font-size:10.5px; font-weight:700; text-transform:uppercase; padding:2px 6px; border-radius:3px; ${it.nivel_sensibilidade === 'restrito' ? 'background:rgba(239,68,68,0.2); color:var(--verm)' : it.nivel_sensibilidade === 'sensivel' ? 'background:rgba(245,158,11,0.2); color:var(--ambar-txt)' : 'background:rgba(100,116,139,0.2); color:var(--tx2)'}">
              ${esc(it.nivel_sensibilidade || 'padrao')}
            </span>
          </td>
          <td>${esc(it.numero_serie || '—')}</td>
          <td><span style="color:${stColor}; font-weight:700">${stNome}</span></td>
          <td style="font-size:12px; color:var(--tx2)">${esc(it.observacao || '—')}</td>
          <td style="text-align:right; white-space:nowrap">
            ${acoesHtml}
          </td>
        </tr>
      `;
    }).join('');

    cont.innerHTML = `
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:12px">
          <div class="form-linha" style="flex:1; margin:0; flex-wrap:wrap; gap:8px">
            <div class="campo" style="flex:2; min-width:200px; margin:0">
              <input id="fBuscaInv" placeholder="Buscar por nome, patrimônio, série, sensibilidade…">
            </div>
            <div class="campo" style="flex:1; min-width:160px; margin:0">
              <select id="fCatInv">
                <option value="">Todas as categorias</option>
                ${cats.map(c => `<option value="${c.nome}">${esc(c.nome)}</option>`).join('')}
              </select>
            </div>
            <div class="campo" style="flex:1; min-width:160px; margin:0">
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
          <button class="primario" id="btIniciarCautelaInv">⚡ Iniciar Cautela de Item</button>
        </div>

        <div class="rolagem">
          <table id="tabInventario">
            <thead>
              <tr>
                <th>Patrimônio</th>
                <th>Descrição do Item</th>
                <th>Categoria</th>
                <th>Sensibilidade</th>
                <th>Nº Série</th>
                <th>Status</th>
                <th>Observações</th>
                <th style="text-align:right">Ações</th>
              </tr>
            </thead>
            <tbody>${linhas || '<tr><td colspan="8"><span class="vazio">Nenhum item cadastrado.</span></td></tr>'}</tbody>
          </table>
        </div>
      </div>
    `;

    const filtrar = () => {
      const q = ($('#fBuscaInv').value || '').trim().toLowerCase();
      const cat = ($('#fCatInv').value || '').trim().toLowerCase();
      const stFiltro = ($('#fStatusInv').value || '').trim();

      document.querySelectorAll('#tabInventario tbody tr').forEach(tr => {
        const txt = tr.dataset.texto || '';
        const st = tr.dataset.status || '';

        let okStatus = true;
        if (stFiltro === 'ativos') {
          okStatus = (st !== 'baixado');
        } else if (stFiltro) {
          okStatus = (st === stFiltro);
        }

        const ok = okStatus && (!q || txt.includes(q)) && (!cat || txt.includes(cat));
        tr.style.display = ok ? '' : 'none';
      });
    };

    $('#fBuscaInv').oninput = filtrar;
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
        modalDevolverItem(cid, nome, () => window.ViewMaterial());
      };
    });

    cont.querySelectorAll('button[data-edititem]').forEach(b => {
      b.onclick = () => {
        const id = +b.dataset.edititem;
        const item = ITENS_CACHE.find(x => x.id === id);
        if (item) modalNovoItem(item, () => window.ViewMaterial());
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

  function modalDevolverItem(cautelaId, itemNome, onConcluido) {
    const html = `
      <div class="modal" style="max-width:480px">
        <h3 style="margin-top:0">📥 Receber Material: ${esc(itemNome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Confirme o retorno do item para a reserva de material.</p>

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
      try {
        await api('/api/material/devolver', {
          method: 'POST',
          body: JSON.stringify({ cautela_id: cautelaId, obs_devolucao: obs })
        });
        toast('Material devolvido à reserva com sucesso!');
        m.remove();
        if (onConcluido) onConcluido();
      } catch (e) {}
    };
  }

  async function modalNovoItem(itemEdicao, onConcluido) {
    const catsRes = await api('/api/material/categorias');
    const cats = (catsRes.categorias || []).filter(c => c.ativo);

    const html = `
      <div class="modal" style="max-width:540px">
        <h3 style="margin-top:0">${itemEdicao ? 'Editar Item do Inventário' : 'Cadastrar Novo Item'}</h3>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label>Nome / Descrição do Item *</label>
            <input id="mItemNome" value="${esc((itemEdicao && itemEdicao.nome) || '')}" placeholder="ex.: Fuzil 7,62mm FAL">
          </div>
          <div class="campo" style="max-width:160px">
            <label>Cód. Patrimônio *</label>
            <input id="mItemCod" value="${esc((itemEdicao && itemEdicao.codigo_patrimonio) || '')}" placeholder="ex.: ARM-042">
          </div>
        </div>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label>Categoria *</label>
            <select id="mItemCat">
              <option value="">— Selecione —</option>
              ${cats.map(c => `<option value="${c.id}" ${itemEdicao && itemEdicao.categoria_id === c.id ? 'selected' : ''}>${esc(c.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo" style="max-width:180px">
            <label>Nº de Série</label>
            <input id="mItemSerie" value="${esc((itemEdicao && itemEdicao.numero_serie) || '')}" placeholder="ex.: 481920">
          </div>
        </div>

        <div class="form-linha" style="margin-bottom:8px">
          <div class="campo" style="flex:1">
            <label>Status</label>
            <select id="mItemStatus">
              <option value="disponivel" ${itemEdicao && itemEdicao.status === 'disponivel' ? 'selected' : ''}>Disponível na Reserva</option>
              <option value="manutencao" ${itemEdicao && itemEdicao.status === 'manutencao' ? 'selected' : ''}>Em Manutenção</option>
              <option value="acautelado" ${itemEdicao && itemEdicao.status === 'acautelado' ? 'selected' : ''}>Acautelado</option>
              <option value="baixado" ${itemEdicao && itemEdicao.status === 'baixado' ? 'selected' : ''}>Baixado / Inativo</option>
            </select>
          </div>
          <div class="campo" style="max-width:200px">
            <label>Sensibilidade Logística</label>
            <select id="mItemSensibilidade">
              <option value="padrao" ${itemEdicao && itemEdicao.nivel_sensibilidade === 'padrao' ? 'selected' : ''}>Padrão (Uso Geral)</option>
              <option value="sensivel" ${itemEdicao && itemEdicao.nivel_sensibilidade === 'sensivel' ? 'selected' : ''}>Sensível (TI / Rádio)</option>
              <option value="restrito" ${itemEdicao && itemEdicao.nivel_sensibilidade === 'restrito' ? 'selected' : ''}>Restrito (Armamento)</option>
            </select>
          </div>
        </div>

        <div class="campo" style="margin-bottom:14px">
          <label>Observações Adicionais</label>
          <input id="mItemObs" value="${esc((itemEdicao && itemEdicao.observacao) || '')}" placeholder="ex.: Carregador extra incluído, sem coronha dobrável.">
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mBtnSalvarItem">Salvar Item</button>
        </div>
      </div>
    `;

    const m = modal(html);

    m.querySelector('#mBtnSalvarItem').onclick = async () => {
      const nome = m.querySelector('#mItemNome').value.trim();
      const cod = m.querySelector('#mItemCod').value.trim();
      const catId = +m.querySelector('#mItemCat').value || null;
      const serie = m.querySelector('#mItemSerie').value.trim();
      const status = m.querySelector('#mItemStatus').value;
      const sens = m.querySelector('#mItemSensibilidade').value;
      const obs = m.querySelector('#mItemObs').value.trim();

      if (!nome || !cod) {
        toast('Nome e Código de Patrimônio são obrigatórios', 'erro');
        return;
      }

      try {
        await api('/api/material/itens', {
          method: 'POST',
          body: JSON.stringify({
            id: itemEdicao ? itemEdicao.id : 0,
            grupo_id: itemEdicao ? itemEdicao.grupo_id : null,
            categoria_id: catId,
            nome: nome,
            codigo_patrimonio: cod,
            numero_serie: serie,
            status: status,
            nivel_sensibilidade: sens,
            observacao: obs
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
            <span style="font-weight:700; text-transform:uppercase">Sens: ${esc(item.nivel_sensibilidade || 'padrao')}</span>
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

})();
