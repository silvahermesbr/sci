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
          <label class="pdf-opcao ${i === 0 ? 'selecionada' : ''}">
            <input type="radio" name="pdfFiltro" value="${p.k}" ${i === 0 ? 'checked' : ''}>
            <span><b>${p.label}</b><small>${p.desc}</small></span>
          </label>`).join('')}
      </div>
      <div style="display:flex;gap:8px;justify-content:flex-end">
        <button type="button" class="secundario" id="pdfCancelar">Cancelar</button>
        <button type="button" class="primario" id="pdfGerar">Gerar PDF</button>
      </div>`;
    const m = window.abrirModal(html, null, { largura: '520px' });
    // Onda UX 0510 (item 2): cartões clicáveis no padrão dos checkboxes da
    // conferência — clique em qualquer parte marca; seleção destacada; ÚNICA.
    m.modal.querySelectorAll('.pdf-opcao').forEach(cartao => {
      const radio = cartao.querySelector('input[name="pdfFiltro"]');
      const destacar = () => {
        m.modal.querySelectorAll('.pdf-opcao').forEach(c => c.classList.toggle('selecionada', c.querySelector('input').checked));
      };
      cartao.addEventListener('click', ev => {
        if (ev.target !== radio) radio.checked = true;
        destacar();
      });
      radio.addEventListener('change', destacar);
    });
    m.modal.querySelector('#pdfCancelar').onclick = () => m.fechar();
    m.modal.querySelector('#pdfGerar').onclick = () => {
      const f = m.modal.querySelector('input[name="pdfFiltro"]:checked').value;
      m.fechar();
      window.open(`/api/conferencia/${confID}/relatorio.pdf?filtro=${f}&t=${Date.now()}`, '_blank');
    };
  };

  /* --- onda Escalas (05/10): painel ESCALA dentro do modal de detalhes ---
     d.escala (GET /api/conferencia/{id}) já traz {usuario_id, papel_na_escala,
     login, nome_guerra}. Quem designa (mesma regra do guardaEscala no backend):
     gerente → chefe+operador do próprio grupo; chefe_setor → SÓ operador do SEU
     setor. Admin/operador/encarregado veem a escala só-leitura. */
  const escalaLinhasHTML = (escala, podeDesignar) => {
    const souChefe = window.ME && window.ME.papel === 'chefe_setor';
    if (!escala.length) return '<tr><td colspan="3"><span class="vazio">Ninguém designado na escala.</span></td></tr>';
    return escala.map(e => `
        <tr>
          <td><b>${esc(e.nome_guerra || e.login)}</b> <small style="color:var(--tx2)">(${esc(e.login)})</small></td>
          <td>${e.papel_na_escala === 'chefe' ? '👑 chefe' : 'operador'}</td>
          <td style="text-align:right">${podeDesignar && (!souChefe || e.papel_na_escala === 'operador')
            ? `<button type="button" class="acao-linha" data-remesc="${e.usuario_id}" data-nome="${esc(e.nome_guerra || e.login)}">remover</button>` : ''}</td>
        </tr>`).join('');
  };

  const blocoEscalaHTML = (d) => {
    const escala = d.escala || [];
    const pode = !!(window.ME && (window.ME.papel === 'gerente' || window.ME.papel === 'chefe_setor'));
    return `
      <div style="margin-top:14px;border-top:1px solid var(--borda);padding-top:10px">
        <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:6px;margin-bottom:8px">
          <h4 style="margin:0">📅 ESCALA DA CONFERÊNCIA</h4>
          ${pode ? '<button type="button" class="primario" id="btAddEscala" style="font-size:12px;padding:4px 12px">+ Designar</button>' : ''}
        </div>
        <table id="tabEscalaModal">
          <thead><tr><th>Conta</th><th>Papel na escala</th><th></th></tr></thead>
          <tbody id="tbEscalaModal">${escalaLinhasHTML(escala, pode)}</tbody>
        </table>
      </div>`;
  };

  const ligarBlocoEscala = (m, d) => {
    const bt = m.modal.querySelector('#btAddEscala');
    if (bt) bt.onclick = () => abrirModalDesignarEscala(d, m);
    m.modal.querySelectorAll('[data-remesc]').forEach(b => {
      b.onclick = async () => {
        if (!(await confirmar(`Remover ${b.dataset.nome} da escala desta conferência?`))) return;
        const r = await processar(() => api(`/api/conferencia/${d.id}/escala/${b.dataset.remesc}`, { method: 'DELETE' }), 'Removendo da escala…');
        if (r.ok) recarregarModalEscala(m, d);
      };
    });
  };

  // Re-render SÓ do bloco da escala (não reabre o modal — preserva o filtro
  // digitado e o scroll dos lançamentos).
  const recarregarModalEscala = async (m, d) => {
    try {
      const novo = await api('/api/conferencia/' + d.id);
      d.escala = novo.escala || [];
      const tb = m.modal.querySelector('#tbEscalaModal');
      if (!tb) return;
      const pode = !!(window.ME && (window.ME.papel === 'gerente' || window.ME.papel === 'chefe_setor'));
      tb.innerHTML = escalaLinhasHTML(d.escala, pode);
      ligarBlocoEscala(m, d);
    } catch (e) {
      toast('Falha ao recarregar a escala: ' + (e.message || e), 'erro');
    }
  };

  // Modal secundário de designação: candidatos de GET /api/usuarios —
  // gerente: grupo_id === ME.grupo_id && ativo; chefe: setor_id === ME.setor_id
  // && ativo (e papel fixo 'operador', igual ao guardaEscala do backend).
  const abrirModalDesignarEscala = async (d, m) => {
    try {
      const me = window.ME || {};
      const souChefe = me.papel === 'chefe_setor';
      const contas = (await api('/api/usuarios')) || [];
      let cand;
      if (souChefe) cand = contas.filter(c => c.ativo && me.setor_id && c.setor_id === me.setor_id);
      else cand = contas.filter(c => c.ativo && c.grupo_id === me.grupo_id);
      const jaNa = new Set((d.escala || []).map(e => e.usuario_id));
      const livres = cand.filter(c => !jaNa.has(c.id));
      if (!livres.length) {
        toast(souChefe && !me.setor_id ? 'Sua conta não tem setor vinculado — solicite ao gerente' : 'Sem candidatos disponíveis', 'erro');
        return;
      }
      const opts = livres.map(c => `<option value="${c.id}">${esc(c.nome_guerra || c.login)} (${esc(c.login)})</option>`).join('');
      const html = `
        <div>
          <h3 style="margin:0 0 10px">Designar na escala — conferência #${d.id}</h3>
          <div class="campo" style="margin-bottom:10px">
            <select id="selEscConta"><option value="">— conta —</option>${opts}</select>
          </div>
          ${souChefe ? '' : `
          <div class="campo" style="margin-bottom:10px">
            <select id="selEscPapel"><option value="operador">Operador</option><option value="chefe">👑 Chefe</option></select>
          </div>`}
          <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:12px">
            <button type="button" class="fantasma" id="escCancelar">Cancelar</button>
            <button type="button" class="primario" id="escAplicar">Designar</button>
          </div>
        </div>`;
      const mm = window.abrirModal(html, null, { largura: '460px' });
      mm.modal.querySelector('#escCancelar').onclick = () => mm.fechar();
      mm.modal.querySelector('#escAplicar').onclick = async () => {
        const sel = mm.modal.querySelector('#selEscConta');
        if (!sel.value) { toast('Escolha a conta', 'erro'); return; }
        const selP = mm.modal.querySelector('#selEscPapel');
        const papel = souChefe ? 'operador' : (selP ? selP.value : 'operador');
        const r = await processar(() => api(`/api/conferencia/${d.id}/escala`, {
          method: 'POST', body: JSON.stringify({ usuario_id: +sel.value, papel_na_escala: papel })
        }), 'Designando na escala…');
        if (r.ok) { mm.fechar(); recarregarModalEscala(m, d); }
      };
    } catch (e) {
      toast('Falha ao carregar candidatos: ' + (e.message || e), 'erro');
    }
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
        <div>
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
          ${blocoEscalaHTML(d, pres.length)}
        </div>`;
      // Onda UX 0510: largura via opção do abrirModal — sem div interna duplicando
      // max-width (transbordava do .modal de 440px fixo).
      const m = window.abrirModal(html, null, { largura: '850px' });
      m.modal.querySelector('#modalConfFechar').onclick = () => m.fechar();
      m.modal.querySelector('#modalConfPDF').onclick = () => {
        m.fechar();
        window.abrirModalPDFConferencia(confID);
      };
      ligarBlocoEscala(m, d);
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

  /* === Ciclo 2 — DASHBOARD DE CONTAGENS =====================================
     confContagens: conta situações sobre C.pessoas × C.est (sem_linha = militar
     sem lançamento na conferência; nao_verificado = linha explícita do servidor).
     confDashRender: patch cirúrgico — só o <b> de cada chip muda, NUNCA
     innerHTML do painel inteiro. Faixa usa .resumo/.caixa da casa (sem CSS novo).
     ========================================================================== */
  function confContagens() {
    const n = { presente: 0, atraso: 0, falta: 0, justificada: 0, nao_verificado: 0, sem_linha: 0 };
    for (const p of (C ? C.pessoas : [])) {
      const s = C.est[p.id];
      if (s === undefined || s === null || n[s] === undefined) n.sem_linha++;
      else n[s]++;
    }
    return n;
  }
  const confChipHTML = (k, n, label, cor) =>
    `<div class="caixa" id="confDash-${k}" style="padding:8px 6px"><b style="color:${cor};font-size:20px">${n}</b><span style="font-size:11px">${label}</span></div>`;

  function confDashHTML() {
    if (!C || !C.c) return '';
    const n = confContagens();
    return `<div class="resumo" id="confDash" style="margin-bottom:14px;grid-template-columns:repeat(auto-fit,minmax(105px,1fr))">
      ${confChipHTML('presente', n.presente, 'Presentes', 'var(--verde-claro)')}
      ${confChipHTML('atraso', n.atraso, 'Atrasos', 'var(--ambar-txt)')}
      ${confChipHTML('falta', n.falta, 'Faltas', 'var(--verm)')}
      ${confChipHTML('justificada', n.justificada, 'Justificadas', '#60a5fa')}
      ${confChipHTML('nao_verificado', n.nao_verificado, 'N.V.', 'var(--tx2)')}
      ${confChipHTML('sem_linha', n.sem_linha, 'Não conferidas', 'var(--tx3)')}
    </div>`;
  }

  function confDashRender() {
    const faixa = document.getElementById('confDash');
    if (!faixa) return; // fora da view de conferência: não faz nada
    const n = confContagens();
    for (const [k, v] of Object.entries(n)) {
      const b = faixa.querySelector('#confDash-' + k + ' b');
      if (b && b.textContent !== String(v)) b.textContent = String(v);
    }
    // P3 (ordem 06/10): contador VERIFICADO/TOTAL do TOPO acompanha o pooling
    // mesmo sem re-render da lista (o do pooling só atualiza por confRender).
    const topo = document.getElementById('confContTopo');
    if (topo) topo.innerHTML = contSpan();

    confPainelGerenteRender();
  }

  /* === Painel do Gerente (Ranking e Acompanhamento) ========================= */
  function confRankingCardsHTML(setoresList) {
    if (!setoresList || !setoresList.length) {
      return '<span class="vazio" style="font-size:12px;padding:6px 0">Nenhum setor despachado nesta conferência.</span>';
    }
    const lista = (setoresList || []).filter(s => s.status !== 'sem_setor' && s.setor_id > 0).map(s => {
      const vTot = s.total_verificados ?? s.verificados ?? 0;
      const eTot = s.total_efetivo ?? s.total_pessoas ?? 0;
      const pct = s.pct_conferido !== undefined ? Math.round(s.pct_conferido) : (eTot > 0 ? Math.round((vTot / eTot) * 100) : 0);
      return {
        ...s,
        pct,
        vTot,
        eTot,
        nome: s.setor_sigla || s.setor_nome || ('Setor #' + s.setor_id)
      };
    });
    // Ordenado do PIOR para o MELHOR (menor % primeiro)
    lista.sort((a, b) => a.pct - b.pct || (a.nome || '').localeCompare(b.nome || '', 'pt', { sensitivity: 'base' }));

    return lista.map(s => {
      let corPct = 'var(--verm)';
      if (s.pct >= 100 || s.status === 'concluida') corPct = 'var(--verde-claro)';
      else if (s.pct >= 50) corPct = 'var(--ambar-txt)';

      return `
        <div class="cfd-card-ranking">
          <div style="display:flex;justify-content:space-between;align-items:baseline;gap:6px">
            <span class="cfd-card-pct" style="color:${corPct}">${s.pct}%</span>
            <span style="font-size:11px;color:var(--tx3)">${s.status === 'concluida' ? '✅' : '⏳'}</span>
          </div>
          <div class="cfd-card-nome" title="${esc(s.setor_nome || s.nome)}">${esc(s.nome)}</div>
          <div class="cfd-card-detalhe">${s.vTot} de ${s.eTot} verif.</div>
        </div>`;
    }).join('');
  }

  function confPainelGerenteHTML() {
    const ehChefeOuOper = window.ME && (window.ME.papel === 'chefe_setor' || window.ME.papel === 'operador');
    if (ehChefeOuOper || !C || !C.c) return '';

    const totBanco = C.totalBanco ?? (C.pessoas ? C.pessoas.length : 0);
    const totVerif = C.totalVerificado ?? (C.verif ? C.verif.size : 0);
    const setFech = C.setoresFechados ?? (C.setoresStatus || []).filter(s => s.status === 'concluida').length;
    const totSet = C.totalSetores ?? (C.setoresStatus || []).filter(s => s.setor_id > 0).length;

    return `
      <div class="cartao cfd-painel-gerente" style="margin-bottom:14px;padding:12px 14px">
        <div class="cfd-barra-gerente">
          <span style="font-size:16px">📊</span>
          <span id="cfdBarraGerenteTexto">Total verificado/total do banco: <b>${totVerif}/${totBanco}</b> · Setores fechados: <b>${setFech}/${totSet}</b></span>
        </div>
        <div class="cfd-ranking-titulo">
          Ranking de % conferido por setor (menor % primeiro):
        </div>
        <div id="cfdRankingContainer" class="cfd-ranking-container">
          ${confRankingCardsHTML(C.setoresStatus)}
        </div>
      </div>`;
  }

  function confPainelGerenteRender() {
    const ehChefeOuOper = window.ME && (window.ME.papel === 'chefe_setor' || window.ME.papel === 'operador');
    if (ehChefeOuOper || !C || !C.c) return;

    const textoEl = document.getElementById('cfdBarraGerenteTexto');
    if (textoEl) {
      const totBanco = C.totalBanco ?? (C.pessoas ? C.pessoas.length : 0);
      const totVerif = C.totalVerificado ?? (C.verif ? C.verif.size : 0);
      const setFech = C.setoresFechados ?? (C.setoresStatus || []).filter(s => s.status === 'concluida').length;
      const totSet = C.totalSetores ?? (C.setoresStatus || []).filter(s => s.setor_id > 0).length;
      textoEl.innerHTML = `Total verificado/total do banco: <b>${totVerif}/${totBanco}</b> · Setores fechados: <b>${setFech}/${totSet}</b>`;
    }

    const rCont = document.getElementById('cfdRankingContainer');
    if (rCont) {
      rCont.innerHTML = confRankingCardsHTML(C.setoresStatus);
    }
  }

  /* === Modal de pré-fechamento (conferência rápida) ========================= */
  async function abrirModalPreFechamentoSetor(cid, sid, sNome) {
    if (!C || !C.c) return;
    let dados;
    try {
      dados = await api(`/api/conferencia/${cid}/setor/${sid}/pre_fechamento`);
    } catch (e) {
      toast('Erro ao carregar pré-fechamento: ' + ((e && e.erro) || (e && e.message) || e), 'erro');
      return;
    }
    await window.ViewConferencia();
    const itens = dados.itens || [];
    const renderLista = () => {
      const verifCount = itens.filter(i => i.verificado).length;
      return `
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
          <span id="cfaContagem" style="font-size:12px;color:var(--tx2)">Verificados: <b>${verifCount}/${itens.length}</b></span>
          <a href="#" id="cfaMarcarTodos" style="font-size:11px;color:var(--tx2)">Marcar todos</a>
        </div>
        <div class="cfa-lista">
          ${itens.map(item => `
            <label class="cfa-item" style="display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid rgba(255,255,255,0.05)">
              <input type="checkbox" class="cfa-chk" data-pid="${item.pessoa_id}" ${item.verificado ? 'checked' : ''}>
              <span style="flex:1">
                <b>${esc(item.nome_guerra)}</b>
                <small style="color:var(--tx2)">${esc(item.funcao || '')}</small>
                ${pill(item.situacao)}
                ${item.observacao ? `<small style="color:var(--tx3)"> · ${esc(item.observacao)}</small>` : ''}
              </span>
            </label>
          `).join('')}
        </div>`;
    };
    const html = `
      <div class="modal" style="max-width:520px;width:92%">
        <h3 style="margin-top:0">Conferência rápida — ${esc(sNome)}</h3>
        <div id="cfaCorpo">${renderLista()}</div>
        <div class="modal-acoes" style="display:flex;justify-content:flex-end;gap:10px">
          <button type="button" class="secundario" id="cfaVoltar">Voltar</button>
          <button type="button" class="primario" style="background:#10b981;border-color:#10b981" id="cfaDespachar">Despachar</button>
        </div>
      </div>`;
    const m = modal(html);
    const atualizarContagem = () => {
      const chks = Array.from(m.querySelectorAll('.cfa-chk'));
      const marcados = chks.filter(c => c.checked).length;
      const total = chks.length;
      const span = m.querySelector('#cfaContagem');
      if (span) span.innerHTML = `Verificados: <b>${marcados}/${total}</b>`;
    };
    m.querySelectorAll('.cfa-chk').forEach(ch => {
      ch.onchange = async () => {
        const pid = +ch.dataset.pid;
        const item = itens.find(i => i.pessoa_id === pid);
        if (!item) return;
        const novoValor = ch.checked;
        const situacao = item.situacao !== 'nao_verificado' ? item.situacao : 'presente';
        try {
          await api('/api/conferencia/marcar?id=' + cid, {
            method: 'POST',
            body: JSON.stringify({ pessoa_id: pid, verificado: novoValor, situacao })
          });
        } catch (e) {
          toast('Erro ao marcar', 'erro');
          ch.checked = !novoValor;
          return;
        }
        item.verificado = novoValor;
        if (novoValor && item.situacao === 'nao_verificado') item.situacao = 'presente';
        atualizarContagem();
      };
    });
    const btMarcarTodos = m.querySelector('#cfaMarcarTodos');
    if (btMarcarTodos) {
      btMarcarTodos.onclick = async (ev) => {
        ev.preventDefault();
        const pendentes = Array.from(m.querySelectorAll('.cfa-chk:not(:checked)'));
        if (!pendentes.length) { toast('Todos já estão marcados'); return; }
        await Promise.all(pendentes.map(async ch => {
          const pid = +ch.dataset.pid;
          const item = itens.find(i => i.pessoa_id === pid);
          if (!item) return;
          const situacao = item.situacao !== 'nao_verificado' ? item.situacao : 'presente';
          try {
            await api('/api/conferencia/marcar?id=' + cid, {
              method: 'POST',
              body: JSON.stringify({ pessoa_id: pid, verificado: true, situacao })
            });
            ch.checked = true;
            item.verificado = true;
            if (item.situacao === 'nao_verificado') item.situacao = 'presente';
          } catch (e) {}
        }));
        atualizarContagem();
      };
    }
    m.querySelector('#cfaVoltar').onclick = () => m.remove();
    m.querySelector('#cfaDespachar').onclick = async () => {
      try {
        await api(`/api/conferencia/${cid}/setor/${sid}/concluir`, { method: 'POST' });
        toast('Conferência do setor concluída');
        m.remove();
        await window.ViewConferencia();
      } catch (e) {
        toast('Erro: ' + (e.message || e), 'erro');
      }
    };
  }

  /* === Modal Iniciar / Despachar Conferência ================================ */
  async function abrirModalIniciarOuDespacharConf() {
    let setores = [];
    try { setores = await api('/api/catalogo/setores'); } catch (e) { setores = []; }
    setores = (setores || []).filter(s => s.ativo === 1 || s.ativo === true);
    if (window.ME && window.ME.grupo_id && setores.some(s => s.grupo_id != null)) {
      setores = setores.filter(s => !s.grupo_id || s.grupo_id === window.ME.grupo_id);
    }
    setores.sort((a, b) => (a.sigla || a.nome || '').localeCompare(b.sigla || b.nome || '', 'pt', { sensitivity: 'base' }));
    // correção 09/10 (ordem do dono): a conferência por antiguidade usa a
    // escada DO GRUPO (tags que o gerente mantém no catálogo "Antiguidade
    // (Pessoal)"), não a lista global — endpoint escopado no grupo da conferência.
    let antig = { funcoes: [], tem_tags: true, aviso: '' };
    try { antig = await api('/api/conferencia/funcoes-antiguidade'); } catch (e) { antig.funcoes = []; }
    const antigHTML = (antig.funcoes || []).map(f => `
      <label style="font-size:11px;display:flex;align-items:center;gap:4px"><input type="checkbox" class="cfa-chk-funcao" value="${f.id}"><span>${esc(f.nome)}</span></label>
    `).join('');
    const avisoSemTags = antig.tem_tags ? '' :
      `<div id="cfaAvisoSemTags" style="font-size:12px;color:var(--verm-txt,#b00020);border:1px solid currentColor;border-radius:6px;padding:8px 10px;margin-bottom:8px">${esc(antig.aviso || 'Grupo sem tags de antiguidade — cadastre no catálogo do grupo (módulo Pessoal)')}</div>`;

    const setoresHTML = setores.map(s => `
      <label class="cfd-setor-item-modal">
        <input type="checkbox" class="cfd-chk-setor" value="${s.id}" checked>
        <span><b>${esc(s.sigla || s.nome)}</b> <small style="color:var(--tx2)">(${esc(s.nome)})</small></span>
      </label>
    `).join('') || '<span class="vazio">Nenhum setor disponível</span>';

    const html = `
      <div class="modal" style="max-width:480px;width:92%">
        <h3 style="margin-top:0">Iniciar Conferência</h3>
        <div class="campo" style="margin-bottom:10px">
          <label style="font-weight:700">Nome da conferência</label>
          <input type="text" id="ncNome" placeholder="ex.: Conferência de pessoal — 2ª semana" style="width:100%">
        </div>
        <div class="campo" style="margin-bottom:12px">
          <label style="font-weight:700">Prazo final (pronto da conferência)</label>
          <input type="time" id="ncPrazo" style="width:100%">
          <small style="color:var(--tx2);font-size:11.5px">Operadores têm até este horário para finalizar a conferência do pessoal do seu setor.</small>
        </div>
        <div class="campo" style="margin-bottom:14px">
          <label style="font-weight:700;margin-bottom:6px;display:block">Modalidade da conferência</label>
          <label style="font-size:12px;display:inline-flex;align-items:center;gap:4px;margin-right:14px"><input type="radio" name="cfaModalidade" value="setores" checked> Por setor</label>
          <label style="font-size:12px;display:inline-flex;align-items:center;gap:4px"><input type="radio" name="cfaModalidade" value="antiguidade"> Por antiguidade</label>
        </div>
        <div class="campo" id="cfdBlocoSetores" style="margin-bottom:14px">
          <label style="font-weight:700;margin-bottom:6px;display:block">Setores convocados</label>
          <label class="cfd-chk-todos">
            <input type="checkbox" id="ncTodosSetores" ${setores.length ? 'checked' : ''}>
            <span>Selecionar todos</span>
          </label>
          <div class="cfd-lista-setores-modal">
            ${setoresHTML}
          </div>
        </div>
        <div class="campo" id="cfaBlocoAntiguidade" style="margin-bottom:14px;display:none">
          <label style="font-weight:700;margin-bottom:6px;display:block">Postos e graduações convocados (antiguidade do grupo)</label>
          ${avisoSemTags}
          <div style="display:flex;gap:6px;margin-bottom:8px">
            <button type="button" class="acao-linha" id="cfaTodasFuncoes" style="font-size:11px;padding:2px 8px">Todas</button>
            <button type="button" class="acao-linha" id="cfaLimparFuncoes" style="font-size:11px;padding:2px 8px">Limpar</button>
          </div>
          <div class="cfd-lista-setores-modal">
            ${antigHTML || '<span class="vazio">Nenhuma tag de antiguidade cadastrada</span>'}
          </div>
        </div>
        <div class="modal-acoes" style="justify-content:flex-end;gap:8px">
          <button type="button" class="acao-linha" id="ncCancelar">Cancelar</button>
          <button type="button" class="primario" id="ncDespachar">Despachar</button>
        </div>
      </div>`;
    const m = modal(html);
    // Modalidade: por setor × por antiguidade (onda 09/10)
    const blocoSetores = m.querySelector('#cfdBlocoSetores');
    const blocoAntig = m.querySelector('#cfaBlocoAntiguidade');
    m.querySelectorAll('input[name="cfaModalidade"]').forEach(r => {
      r.onchange = () => {
        const modo = m.querySelector('input[name="cfaModalidade"]:checked').value;
        blocoSetores.style.display = modo === 'setores' ? '' : 'none';
        blocoAntig.style.display = modo === 'antiguidade' ? '' : 'none';
      };
    });
    const ligaGrupo = (btId, seletor) => {
      const bt = m.querySelector(btId);
      if (bt) bt.onclick = (ev) => {
        ev.preventDefault();
        const alvos = Array.from(m.querySelectorAll(seletor));
        const todos = alvos.every(c => c.checked);
        alvos.forEach(c => { c.checked = !todos; });
      };
    };
    ligaGrupo('#cfaTodasFuncoes', '#cfaBlocoAntiguidade .cfa-chk-funcao');
    const btLimpar = m.querySelector('#cfaLimparFuncoes');
    if (btLimpar) btLimpar.onclick = (ev) => {
      ev.preventDefault();
      m.querySelectorAll('.cfa-chk-funcao').forEach(c => { c.checked = false; });
    };
    const chkTodos = m.querySelector('#ncTodosSetores');
    const chks = m.querySelectorAll('.cfd-chk-setor');
    if (chkTodos) {
      chkTodos.onchange = () => {
        chks.forEach(c => { c.checked = chkTodos.checked; });
      };
    }
    chks.forEach(c => {
      c.onchange = () => {
        if (chkTodos) {
          chkTodos.checked = Array.from(chks).every(x => x.checked);
        }
      };
    });
    m.querySelector('#ncCancelar').onclick = () => m.remove();
    m.querySelector('#ncDespachar').onclick = async () => {
      const nome = m.querySelector('#ncNome').value.trim();
      const prazo = m.querySelector('#ncPrazo').value;
      if (!nome) { toast('Informe o nome da conferência', 'erro'); return; }
      const marcados = Array.from(m.querySelectorAll('.cfd-chk-setor:checked')).map(c => +c.value);
      const modo = (m.querySelector('input[name="cfaModalidade"]:checked') || {}).value || 'setores';
      const funcoesMarcadas = Array.from(m.querySelectorAll('.cfa-chk-funcao:checked')).map(c => +c.value);
      if (modo === 'antiguidade') {
        // R3: grupo sem tags de antiguidade → modalidade BLOQUEADA com aviso
        // claro; nunca cai silenciosamente na lista global.
        if (!antig.tem_tags) {
          toast(antig.aviso || 'Grupo sem tags de antiguidade — cadastre no catálogo do grupo (módulo Pessoal)', 'erro');
          return;
        }
        if (funcoesMarcadas.length === 0) {
          toast('Selecione ao menos um posto/graduação', 'erro'); return;
        }
      }
      try {
        let r;
        if (modo === 'antiguidade') {
          // onda 09/10 — conferência POR ANTIGUIDADE: funcao_ids define o filtro.
          // Sem setores marcados → conferência do grupo inteiro (iniciar); com
          // setores → despachar os setores já afunilados pelo filtro.
          const corpo = { nome, prazo_final: prazo, funcao_ids: funcoesMarcadas };
          if (marcados.length > 0) corpo.setores = marcados;
          r = await api(marcados.length > 0 ? '/api/conferencia/despachar' : '/api/conferencia/iniciar', {
            method: 'POST',
            body: JSON.stringify(corpo)
          });
        } else if (marcados.length > 0) {
          r = await api('/api/conferencia/despachar', {
            method: 'POST',
            body: JSON.stringify({ nome, prazo_final: prazo, setores: marcados })
          });
        } else {
          r = await api('/api/conferencia/iniciar', {
            method: 'POST',
            body: JSON.stringify({ nome, prazo_final: prazo })
          });
        }
        m.remove();
        toast('Conferência despachada');
        const cid = r.conferencia_id || (r.conferencia ? r.conferencia.id : r.id);
        location.hash = '#/conferencia?id=' + cid;
      } catch (e) {
        toast((e && e.erro) || (e && e.message) || 'Falha ao despachar', 'erro');
      }
    };
  }

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
      confDashRender(); // ciclo 2: dashboard acompanha qualquer toque local
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

  /* === Ciclo 2 — POOLING 2s =================================================
     Fetch PRÓPRIO (nunca api(): sem overlay/toast/redirect; falha = console.debug).
     Regras: foco dentro do painel da conferência ou modal aberto → NÃO re-renderiza
     a lista (só dashboard); militar com marcaPend/marcaTimer ativo NÃO é sobreposto;
     aplica só DIFERENÇAS entre payload e C.est/C.verif (patch por militar); C.c/
     C.est/C.verif atualizados com o payload fresco. Timer é limpo ao sair da view
     (confPoolingStop) — sem timer órfão. visibilitychange dá tick imediato ao voltar.
     ========================================================================== */
  let poolTimer = null;
  let poolInFlight = false;
  let poolObserver = null;

  const confPoolingFetch = async () => {
    const qs = CONF_ID ? '?id=' + CONF_ID : '';
    const r = await fetch('/api/conferencia/hoje' + qs, { headers: { 'Accept': 'application/json' } });
    if (!r.ok) throw new Error('pool ' + r.status);
    return r.json();
  };

  // P4 (ordem 06/10, item 4): HASH DE ESTADO por setor — tick de 2s consulta
  // /api/conferencia/estado (SHA-1 por setor no servidor). Hash igual = ZERO
  // mutação de DOM (nem fetch pesado de hoje); mudou = busca o payload e aplica
  // SÓ o setor alterado (animação ~300ms). Fetch PRÓPRIO silencioso (nunca
  // api(): sem overlay/toast/redirect; falha = console.debug).
  const confEstadoFetch = async () => {
    const qs = CONF_ID ? '?id=' + CONF_ID : '';
    const r = await fetch('/api/conferencia/estado' + qs, { headers: { 'Accept': 'application/json' } });
    if (!r.ok) throw new Error('estado ' + r.status);
    return r.json();
  };
  const confHashesIguais = (a, b) =>
    !!a && !!b && a.hash_geral === b.hash_geral &&
    (a.setores || []).length === (b.setores || []).length &&
    (a.setores || []).every((s, i) => s.setor_id === b.setores[i].setor_id && s.hash === b.setores[i].hash);
  let confUltimoEstado = null; // último hash conhecido (null = primeiro tick)

  // aplica SÓ o setor alterado: patch cirúrgico nos .pessoa do grupo + badge do
  // cabeçalho + refresh dos contadores; animação de atualização ~300ms.
  const confAnimarSetor = setorNome => {
    const grps = document.querySelectorAll('#lista .grupo-setor');
    for (const g of grps) {
      const h4 = g.querySelector('h4');
      if (h4 && (h4.textContent || '').trim().startsWith(setorNome)) {
        g.style.transition = 'opacity 0.3s ease';
        g.style.opacity = '0.35';
        setTimeout(() => { g.style.opacity = '1'; }, 300);
        return true;
      }
    }
    return false;
  };

  function confPoolingStop() {
    if (poolTimer) { clearInterval(poolTimer); poolTimer = null; }
    if (poolObserver) { poolObserver.disconnect(); poolObserver = null; }
    document.removeEventListener('visibilitychange', confPoolingVis);
    confUltimoEstado = null; // P4: próximo confPoolingStart re-aprende o hash (teardown idempotente)
  }
  function confPoolingVis() {
    if (document.visibilityState === 'visible') confPoolingTick(); // voltou pra aba: tick imediato
  }

  async function confPoolingTick() {
    if (poolInFlight || !C || !C.c) return;
    poolInFlight = true;
    let est = null;
    try { est = await confEstadoFetch(); }
    catch (e) { console.debug('[pool-conf] falha de rede (silencioso):', e && e.message); }
    finally { poolInFlight = false; }
    if (!est || !C || !C.c) return; // falhou / saiu da view durante o fetch
    // P4: hash IGUAL ao último conhecido → ZERO mutação de DOM (o diff pesado
    // só existe quando o servidor realmente mudou).
    if (confUltimoEstado && confHashesIguais(confUltimoEstado, est)) return;
    const estadoAnterior = confUltimoEstado;
    confUltimoEstado = est;
    // hash_geral null ou undefined = conferência sumiu/inexistente → sai da view.
    // hash_geral "" (vazio) é estado válido: sinaliza 'sem setores ativos', NÃO
    // reinicia a view (evita loop infinito de "Carregando efetivo…").
    if (est.hash_geral === null || est.hash_geral === undefined) { confPoolingStop(); location.hash = '#/hoje'; return; }
    // hash mudou: busca o payload completo (fonte de verdade) e aplica
    let d = null;
    poolInFlight = true;
    try { d = await confPoolingFetch(); }
    catch (e) {
      poolInFlight = false;
      console.debug('[pool-conf] falha de rede (silencioso):', e && e.message);
      return;
    }
    poolInFlight = false;
    if (!d || !d.conferencia) return;
    if (!C || !C.c) return; // saiu da view durante o fetch
    // conferência mudou (fechada por outro operador / troca de ID): recarga completa
    if (d.conferencia.id !== C.c.id || d.conferencia.status !== 'aberta') { confPoolingStop(); window.ViewConferencia(); return; }
    // foco dentro do painel ou modal aberto: lista intocada (só dashboard)
    const focoNoPainel = document.activeElement && document.activeElement.closest && !!document.activeElement.closest('#lista');
    const modalAberto = !!document.querySelector('.modal-mask');
    const estServ = d.conferencia.estados || {};
    const verifServ = new Set(Object.entries(estServ).filter(([, v]) => v.verificado).map(([pid]) => +pid));

    // pendência local: militar sendo digitado/gravado agora não é sobreposto pelo servidor
    const pendente = pid => marcaPend[pid] || marcaTimer[pid];

    // P4: setores cujo hash mudou (só eles ganham patch + animação)
    const hashesAntes = {};
    for (const s of (estadoAnterior ? estadoAnterior.setores : [])) hashesAntes[s.setor_id] = s.hash;
    const setoresMudaram = new Set();
    for (const s of (est.setores || [])) {
      if (hashesAntes[s.setor_id] !== undefined && hashesAntes[s.setor_id] !== s.hash) setoresMudaram.add(+s.setor_id);
    }

    // aplica DIFERENÇAS por militar (patch cirúrgico, sem innerHTML total)
    let mudouLista = false;
    const pessoasDoSetor = {};
    for (const p of C.pessoas) {
      if (p.setor_id != null) (pessoasDoSetor[p.setor_id] = pessoasDoSetor[p.setor_id] || []).push(p);
      if (pendente(p.id)) continue;
      const k = String(p.id);
      const sv = estServ[k];
      const sitNova = sv ? sv.situacao : undefined;
      const verifNova = verifServ.has(p.id);
      const sitVelha = C.est[p.id];
      const verifVelha = C.verif.has(p.id);
      const destVelho = C.dest[p.id];
      const destNovo = sv ? (sv.destino_id ?? null) : null;
      if (sitNova === sitVelha && verifNova === verifVelha && destNovo === destVelho) continue;
      mudouLista = true;
      if (sitNova === undefined) delete C.est[p.id]; else C.est[p.id] = sitNova;
      C.dest[p.id] = destNovo;
      if (verifNova) C.verif.add(p.id); else C.verif.delete(p.id);
      const el = document.querySelector('#lista .pessoa[data-id="' + p.id + '"]');
      if (el) {
        const sel = el.querySelector('.sel-situacao');
        if (sel && document.activeElement !== sel) {
          const tem = sel.querySelector('option[value="' + sitNova + '"]');
          if (tem) sel.value = sitNova;
          else if (sitNova === undefined && !sel.disabled) sel.selectedIndex = 0; // NÃO VERIFICADO
        }
        const chk = el.querySelector('.chk');
        if (chk && chk.checked !== verifNova) chk.checked = verifNova;
        el.classList.toggle('verificado', verifNova);
      }
    }
    // C.c e setores_status com o payload fresco (fonte de verdade = servidor)
    C.c.estados = estServ;
    if (d.setores_status) C.setoresStatus = d.setores_status;
    if (d.total_banco !== undefined) C.totalBanco = d.total_banco;
    if (d.total_verificado !== undefined) C.totalVerificado = d.total_verificado;
    if (d.setores_fechados !== undefined) C.setoresFechados = d.setores_fechados;
    if (d.total_setores !== undefined) C.totalSetores = d.total_setores;
    if (mudouLista && !focoNoPainel && !modalAberto) {
      // P4: mudança localizada num setor conhecido → re-render dirigido + flash
      // apenas no grupo alterado (re-render único preserva invariante: sem
      // atualização parcial de DOM que desalinhe handlers); mudança espalhada →
      // re-render integral como antes.
      const setorAlvo = setoresMudaram.size === 1 ? setoresMudaram.values().next().value : null;
      const nomeAlvo = setorAlvo != null && pessoasDoSetor[setorAlvo] && pessoasDoSetor[setorAlvo][0]
        ? (pessoasDoSetor[setorAlvo][0].setor || '') : null;
      confRender($('#busca') ? $('#busca').value : '');
      if (nomeAlvo) confAnimarSetor(nomeAlvo);
    } else {
      confDashRender(); // contagens acompanham mesmo com foco/modal aberto
    }
  }

  function confPoolingStart() {
    confPoolingStop(); // idempotente: nunca dois timers
    poolTimer = setInterval(confPoolingTick, 2000);
    document.addEventListener('visibilitychange', confPoolingVis);
    // teardown garantido: se o DOM da conferência sair do #app por QUALQUER via
    // (top-nav, hashchange de fora, recarga da view — não só btVoltar/descartar/
    // fechar), o timer morre. Mesmo padrão do observer de telemetria (views_gestao).
    try {
      const appEl = document.getElementById('app');
      if (appEl) {
        poolObserver = new MutationObserver(() => {
          if (!document.getElementById('confDash')) confPoolingStop();
        });
        poolObserver.observe(appEl, { childList: true, subtree: true });
      }
    } catch (e) {}
  }

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
      const p = window.__perConfSel || { m: 'dia', dia: dataLocal(hojeD) };
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
    let dadosHoje = null;
    try { dadosHoje = await api('/api/conferencia/hoje'); haAberta = dadosHoje.conferencia || null; } catch (e) {}
    const souAdmin = (window.ME && window.ME.papel) === 'admin';
    const linha = c => {
      const emAberto = c.status === 'aberta' && modoTela !== 'arquivo';
      const detalheToggle = emAberto ? `
        <tr class="conf-toggle-detalhe" data-toggle-de="${c.id}" style="display:none">
          <td colspan="10" style="background:var(--painel2)">
            <div style="display:flex;gap:18px;flex-wrap:wrap;padding:8px 4px;font-size:12.5px">
              <span>Nome: <b>${esc(c.nome || 'Conferência #' + c.id)}</b></span>
              <span>Prazo p/ pronto: <b>${esc(c.prazo_final || '—')}</b></span>
              <span>Encarregado de pessoal: <b>${esc(c.encarregado_nome || 'a designar')}</b></span>
              <span>Lançamentos: <b>${c.lancamentos}</b></span>
            </div>
          </td>
        </tr>` : '';
      const celulas = `
      <td>${c.status === 'aberta' ? 'Aberta' : 'Fechada'}</td>
      <td>${c.status === 'aberta' ? fmtHora(c.criada_em) : fmtHora(c.fechada_em)}</td>
      <td>${fmtData(c.data)}</td>
      <td>${esc(c.grupo || '—')}</td>
      <td>${esc(c.criado_por || '—')}</td>
      <td>${emAberto ? `${esc(c.nome || '—')} · prazo ${esc(c.prazo_final || '—')} · enc.: ${esc(c.encarregado_nome || '—')}` : esc(c.nome || '—')}</td>
      <td class="num">${c.lancamentos}</td>`;
      const acoes = `
      <td style="white-space:nowrap">${modoTela === 'arquivo'
        ? `<button class="primario" data-abrir-detalhes="${c.id}" style="min-height:36px;padding:8px 12px">Visualizar</button>
           <button style="min-height:36px;padding:8px 12px" data-pdfconf="${c.id}">Relatório PDF</button>
           ${souAdmin ? `<button class="perigo" data-excluir-arq="${c.id}" style="min-height:36px;padding:8px 12px">Excluir</button>` : ''}`
        : (c.status === 'fechada'
          ? `<button class="primario" data-abrir-detalhes="${c.id}" style="min-height:36px;padding:8px 12px">Visualizar</button>
             <button style="min-height:36px;padding:8px 12px" data-pdfconf="${c.id}">Relatório PDF</button>
             <button data-arquivar="${c.id}" style="min-height:36px;padding:8px 12px">Arquivar</button>`
          : `<button class="primario" data-abrir="${c.id}" style="min-height:36px;padding:8px 12px">Abrir</button>`)}</td></tr>${detalheToggle}`;
      return `<tr data-cid="${c.id}" ${emAberto ? `data-toggle-btn="${c.id}" style="cursor:pointer"` : ''}><td class="num"><b>#${c.id}</b></td>${celulas}${acoes}`;
    };
    const porData = (a, b) => String(b.data || '').localeCompare(String(a.data || '')) || b.id - a.id;
    const abertas = lista.filter(c => c.status === 'aberta').sort(porData);
    const fechadas = lista.filter(c => c.status === 'fechada').sort(porData);
    const tabela = (titulo, itens, cols) => `
      <h3 style="margin:14px 0 8px">${titulo} (${itens.length})</h3>
      <div class="cartao"><div class="rolagem"><table>
      <thead><tr><th class="num">ID</th><th>Status</th><th>Horário</th><th>Data</th><th>Grupo</th><th>Operador</th><th>Nome / Prazo / Encarregado</th><th class="num">Lanç.</th><th>Ações</th></tr></thead>
      <tbody>${itens.map(linha).join('') || `<tr><td colspan="10"><span class="vazio">${cols || 'nenhuma'}</span></td></tr>`}</tbody></table></div></div>`;
    // ordem 04/10: abas do módulo → DROPDOWN estilizado (Conferências × Arquivo)
    const abasTela = `<div style="margin:10px 0;display:flex;align-items:center;gap:10px">
      <span style="font-size:12px;color:var(--tx2)">Lista:</span>
      <div id="abasHojeDD" style="min-width:190px"></div></div>`;
    let seletor = '';
    if (modoTela !== 'arquivo') {
      const p = window.__perConfSel || { m: 'dia', dia: dataLocal(hojeD) };
      if (!p.dia) p.dia = dataLocal(hojeD);
      seletor = `<div class="cartao" style="margin-bottom:10px">
        <div class="form-linha" style="align-items:center;gap:10px">
          <div id="perConf" style="min-width:170px"></div>
          <div id="perConfEntrada" style="flex:1"></div>
          <button class="primario" id="perConfIr" style="min-height:40px">Aplicar</button>
        </div></div>`;
    }
    const ehChefeSetor = (window.ME && window.ME.papel) === 'chefe_setor';
    const ehOperador = (window.ME && window.ME.papel) === 'operador';
    const ehGerente = (window.ME && window.ME.papel) === 'gerente';
    const ehEnc = !!(window.ehEncarregado && window.ehEncarregado());
    const ehGestor = !!(window.gestorPessoal && window.gestorPessoal());
    const podeIniciar = ehGerente || ehEnc || ehGestor;
    $('#app').innerHTML = `<h2>Conferências</h2>${abasTela}${seletor}
      <div class="cartao" style="margin-bottom:10px">
        <div class="campo" style="margin:0">
          <label>Pesquisar conferências por ID, Data ou Operador</label>
          <input id="fConfID" placeholder="Digite para filtrar instantaneamente…">
        </div>
      </div>
      ${modoTela !== 'arquivo' ? `
      ${(ehChefeSetor || ehOperador) && !haAberta && (!dadosHoje || !dadosHoje.setores_status || dadosHoje.setores_status.length === 0) ? `
      <div class="cartao cfd-vazio" style="margin-bottom:12px;padding:20px 16px">
        <h4 style="margin:0 0 4px">Nenhuma conferência despachada para o seu setor.</h4>
        <p style="color:var(--tx2);font-size:12.5px;margin:0">Aguarde o Gerente ou Encarregado iniciar a conferência.</p>
      </div>` : ''}
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:6px">
        ${podeIniciar ? `<button class="primario" id="btIniciarConf" style="min-height:44px">▶ Iniciar conferência</button>
        <button class="fantasma" id="btNovaConf" style="min-height:44px">+ Nova conferência</button>` : ''}
        <span style="color:var(--tx2);font-size:12px">${ehChefeSetor ? 'Como Chefe de Setor, selecione uma conferência aberta para lançar presença do seu efetivo.' : 'abertas podem ser editadas · várias simultâneas · fechadas viram relatório (PDF)'}</span></div>` +
      tabela('Abertas', abertas) + tabela('Fechadas', fechadas)
      : tabela('Arquivadas', lista, 'nenhuma conferência arquivada')}
      <p style="color:var(--tx2);font-size:12px">${modoTela === 'arquivo'
        ? 'No arquivo, você pode pesquisar, abrir e conferir registros antigos ou gerar relatórios PDF a qualquer momento.'
        : 'O relatório PDF só é gerado para conferências fechadas. Arquivar tira a conferência desta listagem (vai para o ARQUIVO).'}`;
    // ordem 04/10: abas → dropdown
    if (typeof criarDropdown === 'function') {
      criarDropdown($('#abasHojeDD'), [
        { valor: 'conferencias', rotulo: 'Conferências' },
        { valor: 'arquivo', rotulo: 'Arquivo' }
      ], { valorPadrao: modoTela, onChange: (t) => window.ViewHoje(t) });
    }
    // ordem 04/10 (Fase G6): CHEFE DE SETOR seleciona os OPERADORES do seu setor
    if (ehChefeSetor) renderPainelOperadoresChefe();
    const pc = $('#perConfEntrada');
    if (pc) {
      const p = window.__perConfSel || (window.__perConfSel = { m: 'dia', dia: dataLocal(hojeD) });
      const inp = () => {
        if (p.m === 'livre') pc.innerHTML = `<div class="campo"><label>De — até</label><div style="display:flex;gap:6px"><input type="date" id="pcDe" value="${p.de || ''}"><input type="date" id="pcAte" value="${p.ate || ''}"></div></div>`;
        else pc.innerHTML = `<div class="campo"><label>${p.m === 'ano' ? 'Ano (qualquer dia do ano)' : p.m === 'mes' ? 'Mês (qualquer dia do mês)' : p.m === 'dia' ? 'Dia' : 'Semana (qualquer dia dela)'}</label><input type="date" id="pcDia" value="${p.dia || dataLocal(hojeD)}"></div>`;
      };
      inp();
      // ordem 04/10: abas do módulo → DROPDOWN estilizado
      if (typeof criarDropdown === 'function') {
        criarDropdown($('#perConf'), [
          { valor: 'dia', rotulo: 'Dia' },
          { valor: 'semana', rotulo: 'Semana' },
          { valor: 'mes', rotulo: 'Mês' },
          { valor: 'ano', rotulo: 'Ano' },
          { valor: 'livre', rotulo: 'Período livre' }
        ], {
          valorPadrao: p.m,
          onChange: (m) => { p.m = m; window.__perConfSel = p; inp(); }
        });
      }
      $('#perConfIr').onclick = () => {
        const d = $('#pcDia'); const de = $('#pcDe'); const ate = $('#pcAte');
        if (d) p.dia = d.value;
        if (de) p.de = de.value;
        if (ate) p.ate = ate.value;
        window.ViewHoje('conferencias');
      };
    }
    // INICIAR / NOVA CONFERÊNCIA: modal com nome, prazo e seleção de setores
    const btIniciar = $('#btIniciarConf');
    if (btIniciar) btIniciar.onclick = abrirModalIniciarOuDespacharConf;
    const btNova = $('#btNovaConf');
    if (btNova) btNova.onclick = abrirModalIniciarOuDespacharConf;
    // TOGGLE da conferência em aberto (ordem 04/10): clicar na linha expande
    // os detalhes (nome, prazo, encarregado).
    document.querySelectorAll('[data-toggle-btn]').forEach(tr => {
      tr.onclick = (ev) => {
        if (ev.target.closest('button')) return; // botões da linha mantêm o comportamento próprio
        const det = document.querySelector(`[data-toggle-de="${tr.dataset.toggleBtn}"]`);
        if (det) det.style.display = det.style.display === 'none' ? '' : 'none';
      };
    });
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
    // ordem 06/10 (item 10c): ORDENAR POR nas listas de conferências (#/hoje) via
    // helper compartilhado (window.* definido em views_gestao.js) — APENAS o
    // plug nas tabelas; sem paginação aqui (listas de período costumam ser curtas).
    const cfgConf = [{ tipo: 'num' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'txt' }, { tipo: 'num' }, null];
    if (typeof window.tblOrdenar === 'function') {
      const tabsConf = document.querySelectorAll('#app .cartao .rolagem table');
      if (modoTela === 'arquivo') {
        if (tabsConf[0]) window.tblOrdenar('conf-arquivo', tabsConf[0], tabsConf[0].querySelector('tbody'), cfgConf);
      } else {
        // tabelas na ordem: Abertas (0) e Fechadas (1); a 1ª .cartao é o seletor de
        // período (sem .rolagem), a 2ª é a pesquisa — por isso o seletor pega só as
        // .cartao que contêm tabela.
        let idx = 0;
        tabsConf.forEach(t => {
          if (idx <= 1) {
            window.tblOrdenar(idx === 0 ? 'conf-abertas' : 'conf-fechadas', t, t.querySelector('tbody'), cfgConf);
            idx++;
          }
        });
      }
    }
  };

  /* --- CONFERÊNCIA (edição): #/conferencia — aberta em andamento --- */
  window.ViewConferencia = async function () {
    navAtiva('#/hoje');
    $('#app').innerHTML = '<div class="carregando">Carregando efetivo…</div>';
    CONF_ID = new URLSearchParams(location.hash.split('?')[1] || '').get('id') || null;
    const qs = CONF_ID ? '?id=' + CONF_ID : '';
    let d;
    try {
      d = await api('/api/conferencia/hoje' + qs);
    } catch (e) {
      d = { conferencia: null, setores_status: [] };
    }
    const ehChefe = window.ME && window.ME.papel === 'chefe_setor';
    const ehOper = window.ME && window.ME.papel === 'operador';
    const ehChefeOuOper = ehChefe || ehOper;
    const setoresStatus = d.setores_status || [];

    if (ehChefeOuOper && (!d.conferencia || d.conferencia === null) && setoresStatus.length === 0) {
      $('#app').innerHTML = `
        <div style="margin-bottom:10px"><button class="fantasma" id="btVoltar" style="min-height:38px">← Retornar</button></div>
        <div class="cartao cfd-vazio">
          <div style="font-size:36px;margin-bottom:10px">📭</div>
          <h3 style="margin:0 0 6px">Nenhuma conferência despachada para o seu setor.</h3>
          <p style="color:var(--tx2);font-size:13px;margin:0">Aguarde a liberação da conferência pelo Gerente ou Encarregado.</p>
        </div>`;
      $('#btVoltar').onclick = () => { confPoolingStop(); location.hash = '#/hoje'; };
      return;
    }

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
    if (ehChefe || ehOper) {
      // Confia no filtro do servidor: se vier setores (1+), renderize SOMENTE eles
      const setoresPermitidos = new Set(setoresStatus.map(s => s.setor_id));
      // ordem 08/10 — dupla defesa: o cliente também corta pelo setor ATIVO do
      // contexto (ME.setor_id → fallback pessoa vinculada). O servidor já filtra
      // "pessoas", mas nenhum efetivo alheio pode chegar à tela.
      const meuSetorCtx = (window.ME && (window.ME.setor_id || window.ME.pessoa_setor_id)) || null;
      if (meuSetorCtx) {
        pessoasLista = pessoasLista.filter(p => p.setor_id === meuSetorCtx);
      }
      if (setoresPermitidos.size > 0) {
        pessoasLista = pessoasLista.filter(p => setoresPermitidos.has(p.setor_id));
      } else {
        const escopoSetor = meuSetorCtx;
        if (escopoSetor) pessoasLista = pessoasLista.filter(p => p.setor_id === escopoSetor);
      }
    }
    C = {
      c: d.conferencia,
      modo: d.modo || 'setores',
      funcoesFiltro: d.funcoes_filtro || [],
      pessoas: pessoasLista,
      destinos, est, dest, obs, verif, temComentario,
      escalados, escaladosOntem,
      setoresStatus: d.setores_status || [],
      totalBanco: d.total_banco,
      totalVerificado: d.total_verificado,
      setoresFechados: d.setores_fechados,
      totalSetores: d.total_setores
    };
    confRender();
    confPoolingStart(); // ciclo 2: pooling 2s enquanto a conferência estiver na tela
    $('#btVoltar').onclick = () => { confPoolingStop(); location.hash = '#/hoje'; };
    const btDesc = $('#btDescartar');
    if (btDesc) {
      btDesc.onclick = async () => {
        if (!(await confirmar(`DESCARTAR a conferência #${C.c.id}? O estado parcial gravado será apagado. Esta ação não pode ser desfeita.`))) return;
        try {
          await api('/api/conferencia/' + C.c.id, { method: 'DELETE' });
          confPoolingStop(); // ciclo 2: sem timer órfão ao descartar
          toast('Conferência descartada');
          location.hash = '#/hoje';
          location.reload();
        } catch (e) {}
      };
    }
  };

  function confRender(filtro = '') {
    const semC = !C.c;
    const ehChefe = window.ME && (window.ME.papel === 'chefe_setor' || window.ME.papel === 'operador');
    const f = (filtro || '').trim().toLowerCase();
    // ordem 06/10 (item 9): militares SEM setor agrupados sob 'SEM SETOR',
    // posicionado PRIMEIRO (acima de todos os setores, que seguem alfabéticos).
    const SEM_SETOR = 'SEM SETOR';
    const porSetor = {};
    C.pessoas
      .filter(p => !f || (p.nome_guerra || '').toLowerCase().includes(f) || (p.nome_completo || '').toLowerCase().includes(f))
      .forEach(p => { (porSetor[p.setor || SEM_SETOR] = porSetor[p.setor || SEM_SETOR] || []).push(p); });
    // ordem dentro do setor (v9.14.1): sem check primeiro, depois alfabética
    const ordemCheck = (a, b) => (C.verif.has(a.id) - C.verif.has(b.id))
      || (a.nome_guerra || '').localeCompare(b.nome_guerra || '', 'pt', { sensitivity: 'base' });
    for (const s of Object.keys(porSetor)) porSetor[s].sort(ordemCheck);
    let listas = '';
    let dashboardSetoresHTML = '';
    // ordem 06/10 (item 9): card "SEM SETOR" sintetizado no dashboard, em PRIMEIRO
    if (!semC && ((C.setoresStatus && C.setoresStatus.length > 0) || C.pessoas.some(p => !p.setor))) {
      const semSetorPessoas = C.pessoas.filter(p => !p.setor);
      const listaSetoresDash = (C.setoresStatus || []).slice();
      if (semSetorPessoas.length > 0) {
        const vSem = semSetorPessoas.filter(x => C.verif.has(x.id)).length;
        listaSetoresDash.unshift({ setor_id: 0, setor_nome: 'Militares sem setor', setor_sigla: 'SEM SETOR', status: 'sem_setor', total_efetivo: semSetorPessoas.length, total_pessoas: semSetorPessoas.length, total_verificados: vSem, verificados: vSem });
      }
      const concCount = C.setoresStatus.filter(s => s.status === 'concluida').length;
      const andamCount = C.setoresStatus.filter(s => s.status === 'em_andamento').length;
      const naoIniCount = C.setoresStatus.filter(s => s.status === 'nao_iniciada').length;

      const cards = listaSetoresDash.map(s => {
        const isMeuSetor = ehChefe && window.ME.setor_id === s.setor_id;
        const podeGerenciar = !ehChefe || isMeuSetor;

        let statusBadge = '';
        let bordaCor = 'var(--borda)';
        let bgCor = 'rgba(255,255,255,0.02)';

        if (s.status === 'sem_setor') {
          // ordem 06/10 (item 9): card SEM SETOR — sem botão, sem status setorial
          bordaCor = 'rgba(148, 163, 184, 0.4)';
          bgCor = 'rgba(148, 163, 184, 0.06)';
          statusBadge = `<span style="display:inline-flex;align-items:center;gap:4px;color:#94a3b8;font-weight:600;font-size:11.5px">
            <span style="font-size:14px">➖</span> Sem setor
          </span>`;
        } else if (s.status === 'concluida') {
          bordaCor = 'rgba(16, 185, 129, 0.4)';
          bgCor = 'rgba(16, 185, 129, 0.06)';
          statusBadge = `<span style="display:inline-flex;align-items:center;gap:4px;color:#10b981;font-weight:600;font-size:11.5px">
            <span style="font-size:14px">✅</span> Concluída
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
            <span style="font-size:14px">❌</span> Não iniciada
          </span>`;
        }

        let acaoBtn = '';
        if (podeGerenciar && s.status !== 'sem_setor') {
          if (s.status === 'concluida') {
            acaoBtn = `<button type="button" class="fantasma bt-setor-acao" data-acao="reabrir" data-sid="${s.setor_id}" style="min-height:28px;padding:2px 8px;font-size:11px">↺ Reabrir</button>`;
          } else {
            acaoBtn = `<button type="button" class="primario bt-setor-acao" data-acao="concluir" data-sid="${s.setor_id}" style="min-height:28px;padding:2px 8px;font-size:11px;background:#10b981;border-color:#10b981">✓ Concluir</button>`;
          }
        }

        const vTot = s.total_verificados ?? s.verificados ?? 0;
        const eTot = s.total_efetivo ?? s.total_pessoas ?? 0;

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
              <span style="font-size:11.5px;color:var(--tx2)"><b>${vTot}</b> de ${eTot} verif.</span>
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
              <span style="color:#10b981">✅ ${concCount} concluídos</span>
              <span style="color:#f59e0b">⏳ ${andamCount} em andamento</span>
              <span style="color:#ef4444">❌ ${naoIniCount} não iniciados</span>
            </div>
          </div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(210px,1fr));gap:10px">
            ${cards}
          </div>
        </div>
      `;
    }

    // ordem 06/10 (item 9): SEM SETOR primeiro; demais setores em ordem alfabética
    const ordemSetores = Object.keys(porSetor).sort((a, b) =>
      (a === SEM_SETOR ? -1 : b === SEM_SETOR ? 1 : a.localeCompare(b, 'pt', { sensitivity: 'base' })));
    for (const setor of ordemSetores) {
      const pessoasSetor = porSetor[setor];
      const sObj = (C.setoresStatus || []).find(x => (x.setor_nome || '').toLowerCase() === setor.toLowerCase() || (x.setor_sigla || '').toLowerCase() === setor.toLowerCase() || (pessoasSetor[0] && x.setor_id === pessoasSetor[0].setor_id));
      let setorBadgeHeader = '';
      if (sObj) {
        if (sObj.status === 'concluida') {
          setorBadgeHeader = `<span style="font-size:11.5px;color:#10b981;font-weight:600;padding:2px 8px;border-radius:12px;background:rgba(16,185,129,0.1);border:1px solid rgba(16,185,129,0.3)">✅ Concluída</span>`;
        } else if (sObj.status === 'em_andamento') {
          setorBadgeHeader = `<span style="font-size:11.5px;color:#f59e0b;font-weight:600;padding:2px 8px;border-radius:12px;background:rgba(245,158,11,0.1);border:1px solid rgba(245,158,11,0.3)">⏳ Em andamento</span>`;
        } else {
          setorBadgeHeader = `<span style="font-size:11.5px;color:#ef4444;font-weight:600;padding:2px 8px;border-radius:12px;background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3)">❌ Não iniciada</span>`;
        }
      }

      const totalVerif = pessoasSetor.filter(x => C.verif.has(x.id)).length;
      let jaInseriuDivisor = false;

      const itensHTML = pessoasSetor.map((p, idx) => {
        const sit = sitDe(p);
        const ehVerif = C.verif.has(p.id);
        const escInfo = (C.escalados || []).find(x => x.pessoa_id === p.id);
        const badgeEscala = escInfo
          ? `<span style="background:rgba(87,161,115,.2); color:var(--verde-claro); font-size:11px; padding:1px 6px; border-radius:4px; font-weight:700" title="Escalado em ${esc(escInfo.tipo_nome)}">🛡️ ${esc(escInfo.tipo_nome)}</span>`
          : '';
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

      listas += `<div class="grupo-setor">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
          <h4 style="margin:0">${esc(setor)} · ${pessoasSetor.length}</h4>
          ${setorBadgeHeader}
        </div>
        <div class="lista-pessoa">${itensHTML}</div>
      </div>`;
    }
    // ordem 06/10 (P3 — itens 11+12): contador VERIFICADO/TOTAL + ação NO TOPO,
    // colados ao alerta ABERTA. Gerente/encarregado fecham a CONFERÊNCIA;
    // operador/chefe_setor fecham o PRÓPRIO SETOR (endpoint de concluir setor;
    // sem setor → botão desabilitado com tooltip). Doutrina "fechar só na barra
    // fixa inferior" REVOGADA pelo Diretor.
    const contSpan = () => `<b>${C.verif.size}/${C.pessoas.length}</b> verificados`;
    const acoesTopo = () => {
      // P2 (ordem 06/10): fechar CONFERÊNCIA = gerente + encarregado de pessoal
      // (papel-conf derivado no core.js). Admin segue PROIBIDO (403 no backend) —
      // não ganha botão, só aviso. Papel explícito, NÃO !ehChefe (incluía admin).
      const papelT = (window.ME && window.ME.papel) || '';
      const ehGerEnc = papelT === 'gerente' || papelT === 'encarregado';
      // P3: operador/chefe fecha o PRÓPRIO setor — mesma expressão de escopoSetor
      // em ViewConferencia (usuarios.setor_id → fallback pessoa vinculada).
      const meuSetor = window.ME ? (window.ME.setor_id || window.ME.pessoa_setor_id) : null;
      if (ehGerEnc) {
        return `<button class="perigo" id="btFecharTopo" style="min-height:40px">✕ FECHAR CONFERÊNCIA</button>`;
      }
      if (papelT === 'admin') {
        return `<span style="font-size:11.5px;color:var(--tx3)">fechamento: gerente/encarregado</span>`;
      }
      return `<button class="primario" id="btFecharSetorTopo" style="min-height:40px" ${meuSetor ? '' : 'disabled title="sua conta não tem setor atribuído — solicite ao gerente"'}>✓ FECHAR MEU SETOR</button>`;
    };
    const banner = semC
      ? `<div class="cartao"><p style="color:var(--tx2)">Nenhuma conferência aberta. Ao iniciar, a data e o horário de Brasília são registrados automaticamente.</p>
         ${!ehChefe ? `<div style="display:flex;gap:8px;align-items:end;margin-top:10px">
           <button class="primario" id="btIniciar" style="min-height:44px">▶ Iniciar conferência</button></div>` : '<p style="color:var(--tx3);font-size:12px;margin-top:8px">Aguarde o Gerente ou Operador iniciar a conferência do grupo.</p>'}</div>`
      : `<div class="cartao">
         <span>${pill('aberta')} <b>Conferência #${C.c.id}</b> · aberta em ${fmtData(C.c.data)} às ${fmtHora(C.c.criada_em)}${C.c.local ? ' · ' + esc(C.c.local) : ''}</span>
         ${C.modo === 'antiguidade' ? `<div class="cfa-badge" title="Conferência limitada às funções selecionadas">FILTRO POR ANTIGUIDADE: ${esc((C.funcoesFiltro || []).join(', '))}</div>` : ''}
         <div id="confTopoBarra" style="display:flex;justify-content:flex-end;align-items:center;gap:12px;margin-top:10px;padding-top:10px;border-top:1px solid var(--borda);flex-wrap:wrap">
           <span id="confContTopo" style="font-size:12.5px;color:var(--tx2)">${contSpan()}</span>
           ${acoesTopo()}
         </div></div>`;
    $('#app').innerHTML = `<div style="margin-bottom:10px"><button class="fantasma" id="btVoltar" style="min-height:38px">← Retornar</button></div>
      <h2 style="margin-top:0">Conferência de pessoal</h2>${banner}
      ${confDashHTML()}
      ${confPainelGerenteHTML()}
      ${dashboardSetoresHTML}
      <div class="barra-fixa">
        <input id="busca" placeholder="buscar nome…">
        ${!ehChefe ? `<button class="primario" id="btFecharBarra">✕ FECHAR CONFERÊNCIA</button>` : ((C.setoresStatus && C.setoresStatus.length > 0) ? '' : `<span style="font-size:12px;color:var(--tx2);font-weight:600">Área do Chefe de Setor</span>`)}
      </div>
      <div id="lista">${listas}</div>
      ${!ehChefe ? `
      <div style="display:flex;justify-content:flex-end;margin-top:28px;padding-top:14px;border-top:1px solid var(--borda)">
        <button class="perigo" id="btDescartar" style="min-height:40px">🗑 Descartar conferência</button>
      </div>` : ''}`;
    const atualizar = () => {
      // v9.15.1: NÃO recriar a barra (perdia foco a cada dígito) — só o contador muda
      const cont = document.querySelector('.barra-fixa .cont');
      if (cont) cont.innerHTML = contSpan();
      // P3: contador do topo acompanha (mesmo conteúdo)
      const topo = document.getElementById('confContTopo');
      if (topo) topo.innerHTML = contSpan();
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
      // P3: FECHAR CONFERÊNCIA no topo (gerente/encarregado) — mesmo confFechar
      const btFT = document.getElementById('btFecharTopo');
      if (btFT) btFT.onclick = confFechar;
      // P3 (ordem 06/10): FECHAR MEU SETOR no topo — operador/chefe concluir
      // o PRÓPRIO setor (endpoint existente) e recarregar a view. Setor já
      // concluído → botão vira estado (sem POST redundante).
      const btFS = document.getElementById('btFecharSetorTopo');
      if (btFS) btFS.onclick = () => {
        const meuSetor = window.ME ? (window.ME.setor_id || window.ME.pessoa_setor_id) : null;
        if (!meuSetor || !C.c) return;
        const sObj = (C.setoresStatus || []).find(x => x.setor_id === meuSetor);
        if (sObj && sObj.status === 'concluida') { toast('Seu setor já está concluído nesta conferência', 'erro'); return; }
        const sNome = sObj ? (sObj.setor_sigla || sObj.setor_nome) : 'Meu Setor';
        // onda 09/10: recarrega a view (estado do servidor) e abre modal de
        // conferência rápida com lista do setor; Despachar confirma a conclusão.
        abrirModalPreFechamentoSetor(C.c.id, meuSetor, sNome);
      };
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
        if (acao === 'concluir') {
          const sObj = (C.setoresStatus || []).find(x => x.setor_id === sid);
          const sNome = sObj ? (sObj.setor_sigla || sObj.setor_nome) : 'Setor';
          // onda 09/10: mesmo fluxo do topo — estado do servidor + conferência rápida.
          abrirModalPreFechamentoSetor(C.c.id, sid, sNome);
          return;
        }
        try {
          await api(`/api/conferencia/${C.c.id}/setor/${sid}/reabrir`, { method: 'POST' });
          toast('Conferência do setor reaberta!');
          await window.ViewConferencia();
        } catch (e) {
          toast('Erro: ' + (e.message || e), 'erro');
        }
      };
    });
    const btIniBanner = $('#btIniciar');
    if (btIniBanner) btIniBanner.onclick = abrirModalIniciarOuDespacharConf;
    atualizar(); // contador de verificados acompanha o re-render (v9.15.2)
    ligarBarra(); // v9.16.5b: barra é recriada no innerHTML — religar FECHAR e busca
    confPainelGerenteRender();
    // P3 (ordem 06/10): os handlers do TOPO (btFecharTopo/btFecharSetorTopo) são
    // ligados DENTRO de ligarBarra — re-criados a cada re-render (innerHTML total).
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
        <div id="cmEditor" style="margin-bottom:8px"></div></div>
      <div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:8px">
        <button type="button" class="acao-linha" id="cmAnxDrive" style="cursor:pointer;font-size:11.5px;padding:2px 8px">🗂️ Anexar: Do Drive</button>
        <label class="acao-linha" style="cursor:pointer;font-size:11.5px;padding:2px 8px">
          📎 Do computador
          <input type="file" id="cmAnxInput" multiple style="display:none">
        </label>
        <div id="cmAnxLista" style="display:flex;gap:6px;flex-wrap:wrap"></div>
      </div>
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
           <div>${c.comentario_rico ? c.comentario_rico : esc(c.comentario)}</div>
           ${(c.anexos && c.anexos.length) ? `<div style="display:flex;gap:6px;flex-wrap:wrap;margin-top:4px">
             ${c.anexos.map(anx => anx.drive_arquivo_id
               ? `<a href="/api/drive/download/${anx.drive_arquivo_id}" class="msg-anexo-item" style="font-size:11px" target="_blank">🗂️ ${esc(anx.nome || 'arquivo')}</a>`
               : (anx.id ? `<a href="/api/anexos/comentario/${anx.id}" class="msg-anexo-item" style="font-size:11px" target="_blank">📄 ${esc(anx.nome || 'anexo')}</a>` : ''))}</div>` : ''}</div>`).join('') : '<span class="vazio">sem comentários</span>';
        confRender($('#busca') ? $('#busca').value : '');
      } catch (e) { raiz.querySelector('#cmLista').innerHTML = '<span class="vazio">falha ao carregar</span>'; }
    };
    // onda 05/10: editor rico (delegação EditorRico) + anexos (mesmo padrão do cautelar/avisos)
    const editorComm = window.EditorRico ? window.EditorRico.init(raiz.querySelector('#cmEditor'), { placeholder: 'registre aqui…' }) : null;
    let anexosComm = [];
    const renderAnexosComm = () => {
      const contA = raiz.querySelector('#cmAnxLista');
      if (!contA) return;
      contA.innerHTML = anexosComm.map((a, i) => `
        <span class="msg-anexo-item" style="font-size:11px">
          🗂️ ${esc(a.nome)} <small>(${window.formatarTamanhoBytes ? formatarTamanhoBytes(a.tamanho) : a.tamanho})</small>
          <span data-rm="${i}" style="cursor:pointer;font-weight:bold;margin-left:4px;color:var(--verm-txt)">&times;</span>
        </span>`).join('');
      contA.querySelectorAll('[data-rm]').forEach(b => {
        b.onclick = () => { anexosComm.splice(+b.dataset.rm, 1); renderAnexosComm(); };
      });
    };
    raiz.querySelector('#cmAnxDrive').onclick = () => {
      if (!window.abrirSeletorDrive) { toast('Seletor do drive indisponível', 'erro'); return; }
      window.abrirSeletorDrive({ jaSelecionados: anexosComm, onConfirma: (escolhidos) => { anexosComm = escolhidos; renderAnexosComm(); } });
    };
    raiz.querySelector('#cmAnxInput').onchange = async (e) => {
      const files = Array.from(e.target.files || []);
      for (const file of files) {
        if (file.size > 25 * 1024 * 1024) { toast(`Arquivo ${file.name} excede 25MB`, 'erro'); continue; }
        try {
          const ref = await window.enviarArquivoParaDrive(file);
          anexosComm.push(ref);
        } catch (err) {
          toast(`Falha ao enviar ${file.name} ao drive`, 'erro');
        }
      }
      e.target.value = '';
      renderAnexosComm();
    };

    await carregar();
    raiz.querySelector('#cmGo').onclick = async () => {
      const txt = editorComm ? editorComm.getHTML() : '';
      if (!txt || txt === '<p><br></p>') { toast('Escreva o comentário', 'erro'); return; }
      try {
        await api('/api/comentarios', { method: 'POST', body: JSON.stringify({ conferencia_id: cid, pessoa_id: pessoaId, comentario: txt, anexos: anexosComm }) });
        if (editorComm) editorComm.setHTML('');
        anexosComm = [];
        renderAnexosComm();
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
      confPoolingStop(); // ciclo 2: sem timer órfão ao fechar
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
        <div class="rolagem"><table><thead><tr><th class="num">ORD</th><th>Antiguidade</th><th>Nome</th><th>Setor</th><th>Grupo</th><th class="num">Pres.</th><th class="num">Atraso</th>
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
          <div style="display:inline-flex;gap:8px;align-items:center;flex-wrap:wrap">
            <a href="/api/relatorio.pdf?de=${encodeURIComponent(b.De)}&ate=${encodeURIComponent(b.Ate)}${grupoQ}&t=${Date.now()}" target="_blank">
              <button type="button" style="display:inline-flex;align-items:center;gap:6px">
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
                <span>Relatório Simples (PDF)</span>
              </button>
            </a>
            <a href="/api/relatorio/detalhado.pdf?de=${encodeURIComponent(b.De)}&ate=${encodeURIComponent(b.Ate)}${grupoQ}&modo=detalhado&t=${Date.now()}" target="_blank">
              <button type="button" class="primario" style="display:inline-flex;align-items:center;gap:6px">
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
                <span>Relatório Detalhado (PDF)</span>
              </button>
            </a>
          </div>
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
        <div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">
          <span style="font-size:12px;color:var(--tx2)">Seção:</span>
          <div id="abasRelDD" style="min-width:250px"></div>
        </div>
        <div id="corpoRelatorios"></div>
      `;

      // ordem 04/10: abas do módulo → DROPDOWN estilizado
      if (typeof criarDropdown === 'function') {
        criarDropdown($('#abasRelDD'), [
          { valor: 'consolidado', rotulo: '📊 Resumo Consolidado & PDF' },
          { valor: 'conferencias', rotulo: '📋 Conferências Individuais' },
          { valor: 'individual', rotulo: '🔍 Busca Individual por Militar' }
        ], {
          valorPadrao: abaPrincipal,
          onChange: (k) => { abaPrincipal = k; renderTela(); }
        });
      }

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
          <div class="form-linha" style="align-items:center;gap:10px;margin-bottom:12px">
            <span style="font-size:12px;color:var(--tx2)">Período:</span>
            <div id="modosDD" style="min-width:160px"></div>
            <div id="entrada" style="flex:1"></div>
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

      // ordem 04/10: modos de período → dropdown estilizado
      if (typeof criarDropdown === 'function') {
        criarDropdown($('#modosDD'), [
          { valor: 'dia', rotulo: 'Dia' },
          { valor: 'semana', rotulo: 'Semana' },
          { valor: 'ano', rotulo: 'Ano' },
          { valor: 'livre', rotulo: 'Período livre' }
        ], { valorPadrao: modo, onChange: (m) => { modo = m; entrada(); } });
      }
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
                    <th>Antiguidade</th>
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
      const optFuncoes = ((funcoes && funcoes.funcoes) || funcoes || []).filter(f => f.ativo !== 0);

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
              <label>Antiguidade</label>
              <select id="biFuncao">
                <option value="">— Todas as Antiguidades —</option>
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

  /* =====================================================================
     ordem 04/10 (Fase G6): CHEFE DE SETOR — seleciona OPERADORES dentre as
     contas do SEU setor (endpoint /api/operadores-do-setor). O operador não é
     criado: é designado. Painel injetado no fim da página de Conferências.
     ===================================================================== */
  window.__renderPainelOperadoresChefe = async function () {
    let cont = document.getElementById('painelOpSetor');
    if (!cont) {
      cont = document.createElement('div');
      cont.id = 'painelOpSetor';
      cont.style.margin = '10px 0';
      const app = document.getElementById('app');
      if (!app) return;
      app.appendChild(cont);
    }
    cont.innerHTML = '<div class="carregando">Carregando efetivo do setor…</div>';
    let ops = [];
    try { ops = await api('/api/operadores-do-setor'); } catch (e) {}
    const linha = o => `
      <tr data-oplogin="${esc(o.login)}">
        <td class="num">#${o.id}</td>
        <td><b>${esc(o.nome_guerra || o.login)}</b></td>
        <td>${esc(o.nome_completo || '—')}</td>
        <td>${esc(o.funcao_nome || '—')}</td>
        <td>${o.eh_operador ? '<span class="alerta-ok">● OPERADOR</span>' : '<span style="color:var(--tx3)">● conta</span>'}</td>
        <td>${o.eh_operador
          ? `<span style="color:var(--tx3);font-size:12px">designado</span>`
          : `<button class="primario" data-designar="${esc(o.login)}" style="min-height:34px;padding:6px 12px">Designar operador</button>`}</td>
      </tr>`;
    cont.innerHTML = `
      <div class="cartao">
        <h3 style="margin-top:0">OPERADORES DO MEU SETOR (${(ops || []).length})</h3>
        <p style="color:var(--tx2);font-size:12px;margin:0 0 8px">O operador não é criado: você SELECIONA dentre as contas do seu setor. Conta nova no setor? Procure o encarregado de pessoal.</p>
        <div class="campo" style="margin-bottom:8px"><label>Filtrar</label><input id="fOpSetor" placeholder="buscar login/nome…"></div>
        <div class="rolagem"><table><thead><tr><th class="num">ID</th><th>Nome de Guerra</th><th>Nome Completo</th><th>Função</th><th>Situação</th><th>Ação</th></tr></thead>
        <tbody>${(ops || []).map(linha).join('') || '<tr><td colspan="6"><span class="vazio">nenhuma conta no seu setor</span></td></tr>'}</tbody></table></div>
      </div>`;
    cont.querySelector('#fOpSetor').oninput = () => {
      const q = cont.querySelector('#fOpSetor').value.trim().toLowerCase();
      cont.querySelectorAll('tr[data-oplogin]').forEach(tr => {
        tr.style.display = !q || tr.dataset.oplogin.toLowerCase().includes(q) ? '' : 'none';
      });
    };
    cont.querySelectorAll('[data-designar]').forEach(b => {
      b.onclick = async () => {
        try {
          await api('/api/operadores-do-setor', { method: 'POST', body: JSON.stringify({ login: b.dataset.designar }) });
          toast('Operador designado');
          window.__renderPainelOperadoresChefe();
        } catch (e) {}
      };
    });
  };
  function renderPainelOperadoresChefe() { window.__renderPainelOperadoresChefe(); }
})();
