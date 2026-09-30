/* SCI — Módulo de Mensageria Interna por Função (Caixa de Correio Institucional)
   Mensagens atreladas aos papéis/cargos e não ao indivíduo.
   Histórico institucional permanece intacto após transição de ocupante. */
(function () {
  'use strict';

  function quem() {
    return (typeof ME !== 'undefined' && ME) || window.ME || null;
  }

  /* Formata o nome completo destacando o nome curto/guerra em negrito */
  function formatarNomeRemetente(nomeCompleto, nomeGuerra) {
    const nc = (nomeCompleto || '').trim();
    const ng = (nomeGuerra || '').trim();
    if (!nc && !ng) return 'Anônimo';
    if (!ng) return esc(nc);
    if (!nc) return `<strong>${esc(ng)}</strong>`;

    const idx = nc.toLowerCase().indexOf(ng.toLowerCase());
    if (idx !== -1) {
      const antes = nc.substring(0, idx);
      const exato = nc.substring(idx, idx + ng.length);
      const depois = nc.substring(idx + ng.length);
      return `${esc(antes)}<strong>${esc(exato)}</strong>${esc(depois)}`;
    }
    return `${esc(nc)} (<strong>${esc(ng)}</strong>)`;
  }

  window.ViewMensagens = async function () {
    const u = quem();
    if (!u) { location.hash = '#/login'; return; }
    if (window.navAtiva) navAtiva('#/mensagens');

    const app = document.getElementById('app');
    if (!app) return;

    app.innerHTML = `
      <div class="msg-header-topo">
        <div>
          <h2 style="margin:0 0 4px">Mensageria & Correio Institucional</h2>
          <p style="color:var(--tx2);font-size:13px;margin:0">Comunicação formal entre funções e escalões. O histórico pertence à função.</p>
        </div>
        <div style="display:flex;gap:8px">
          <button type="button" class="primario" id="btNovaMsg" style="display:flex;align-items:center;gap:6px;padding:8px 16px">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            Nova Mensagem
          </button>
        </div>
      </div>

      <div class="msg-abas-container">
        <button type="button" class="msg-aba-btn ativo" id="abaInbox">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
          Caixa de Entrada <span id="badgeInboxAba" class="badge-mini oculto">0</span>
        </button>
        <button type="button" class="msg-aba-btn" id="abaEnviadas">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
          Mensagens Enviadas
        </button>
      </div>

      <div id="msgConteudo">
        <div class="carregando">Carregando mensagens…</div>
      </div>
    `;

    let abaAtual = 'inbox';
    const cont = document.getElementById('msgConteudo');
    const bInbox = document.getElementById('abaInbox');
    const bEnv = document.getElementById('abaEnviadas');

    bInbox.onclick = () => {
      if (abaAtual === 'inbox') return;
      abaAtual = 'inbox';
      bInbox.classList.add('ativo');
      bEnv.classList.remove('ativo');
      carregarInbox();
    };

    bEnv.onclick = () => {
      if (abaAtual === 'enviadas') return;
      abaAtual = 'enviadas';
      bEnv.classList.add('ativo');
      bInbox.classList.remove('ativo');
      carregarEnviadas();
    };

    document.getElementById('btNovaMsg').onclick = () => abrirModalCompor();

    async function carregarInbox() {
      cont.innerHTML = '<div class="carregando">Carregando caixa de entrada…</div>';
      try {
        const msgs = await api('/api/mensagens/inbox');
        renderListaInbox(msgs);
        atualizarBadges();
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar mensagens.</div>';
      }
    }

    async function carregarEnviadas() {
      cont.innerHTML = '<div class="carregando">Carregando mensagens enviadas…</div>';
      try {
        const msgs = await api('/api/mensagens/enviadas');
        renderListaEnviadas(msgs);
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar mensagens.</div>';
      }
    }

    function renderListaInbox(msgs) {
      if (!msgs || msgs.length === 0) {
        cont.innerHTML = `
          <div class="msg-vazio-card">
            <svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
            <p>Sua caixa de entrada para esta função está vazia.</p>
          </div>
        `;
        return;
      }

      const html = msgs.map(m => {
        const lida = !!m.lida_em;
        const rem = m.remetente || {};
        const nomeRem = formatarNomeRemetente(rem.nome_completo, rem.nome_guerra);
        const funcaoRem = rem.funcao_nome || 'Função Organizacional';
        const papelRem = (rem.papel || '').toUpperCase() + (rem.grupo_nome ? ' — ' + rem.grupo_nome : ' — Global');

        return `
          <div class="msg-card ${lida ? 'lida' : 'nao-lida'}" data-msgid="${m.id}">
            <div class="msg-card-status">
              <span class="msg-dot ${lida ? 'lida' : ''}"></span>
            </div>
            <div class="msg-card-corpo-prev">
              <div class="msg-card-linha1">
                <span class="msg-remetente-tit">${nomeRem} · <small class="msg-funcao-tag">${esc(funcaoRem)}</small></span>
                <span class="msg-data-txt">${fmtData(m.criada_em)} ${fmtHora(m.criada_em)}</span>
              </div>
              <div class="msg-card-linha2">
                <span class="msg-papel-sub">${esc(papelRem)}</span>
              </div>
              <div class="msg-card-assunto">${esc(m.assunto)}</div>
              <div class="msg-card-resumo">${esc(m.corpo.substring(0, 120))}${m.corpo.length > 120 ? '…' : ''}</div>
            </div>
          </div>
        `;
      }).join('');

      cont.innerHTML = `<div class="msg-lista-wrapper">${html}</div>`;

      cont.querySelectorAll('.msg-card').forEach(card => {
        card.onclick = () => {
          const id = +card.dataset.msgid;
          const msgObj = msgs.find(x => x.id === id);
          if (msgObj) abrirModalVisualizar(msgObj, 'inbox');
        };
      });
    }

    function renderListaEnviadas(msgs) {
      if (!msgs || msgs.length === 0) {
        cont.innerHTML = `
          <div class="msg-vazio-card">
            <svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
            <p>Nenhuma mensagem enviada por esta função ainda.</p>
          </div>
        `;
        return;
      }

      const html = msgs.map(m => {
        const dests = m.destinatarios || [];
        const destTxt = dests.map(d => `${(d.papel || '').toUpperCase()}${d.grupo_nome ? ' (' + d.grupo_nome + ')' : ''}`).join(', ') || 'Sem destinatários';

        return `
          <div class="msg-card lida" data-msgid="${m.id}">
            <div class="msg-card-status">
              <span class="msg-dot lida"></span>
            </div>
            <div class="msg-card-corpo-prev">
              <div class="msg-card-linha1">
                <span class="msg-remetente-tit">Para: <b>${esc(destTxt)}</b></span>
                <span class="msg-data-txt">${fmtData(m.criada_em)} ${fmtHora(m.criada_em)}</span>
              </div>
              <div class="msg-card-assunto">${esc(m.assunto)}</div>
              <div class="msg-card-resumo">${esc(m.corpo.substring(0, 120))}${m.corpo.length > 120 ? '…' : ''}</div>
            </div>
          </div>
        `;
      }).join('');

      cont.innerHTML = `<div class="msg-lista-wrapper">${html}</div>`;

      cont.querySelectorAll('.msg-card').forEach(card => {
        card.onclick = () => {
          const id = +card.dataset.msgid;
          const msgObj = msgs.find(x => x.id === id);
          if (msgObj) abrirModalVisualizar(msgObj, 'enviadas');
        };
      });
    }

    function abrirModalVisualizar(m, modo) {
      const rem = m.remetente || {};
      const nomeRem = formatarNomeRemetente(rem.nome_completo, rem.nome_guerra);
      const funcaoRem = rem.funcao_nome || 'Função não definida';
      const papelRem = (rem.papel || '').toUpperCase() + (rem.grupo_nome ? ' — ' + rem.grupo_nome : ' — Global');

      let lidaInfo = '';
      if (m.lida_em) {
        lidaInfo = `<span class="msg-lida-audit">✓ Lida por <b>${esc(m.lida_por_nome || 'membro da equipe')}</b> em ${fmtData(m.lida_em)} às ${fmtHora(m.lida_em)}</span>`;
      } else {
        lidaInfo = `<span class="msg-lida-audit nao-lida">● Mensagem Não Lida</span>`;
      }

      const html = `
        <div class="modal" style="max-width:680px;width:95%">
          <div class="msg-view-cabecalho">
            <div class="msg-view-tit">${esc(m.assunto)}</div>
            <div class="msg-view-meta">
              <div class="msg-view-linha1">${nomeRem} - <span class="destaque-funcao">${esc(funcaoRem)}</span></div>
              <div class="msg-view-linha2">${esc(papelRem)}</div>
              <div class="msg-view-linha3">
                <span>Enviada em ${fmtData(m.criada_em)} às ${fmtHora(m.criada_em)}</span>
                ${lidaInfo}
              </div>
            </div>
          </div>

          <div class="msg-view-corpo">${esc(m.corpo).replace(/\n/g, '<br>')}</div>

          <div class="modal-acoes" style="justify-content:space-between;border-top:1px solid var(--borda);padding-top:14px;margin-top:20px">
            <div>
              ${modo === 'inbox' ? `
                <button type="button" class="acao-linha" id="btExcluirMsg" style="color:var(--verm-txt)">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                  Excluir
                </button>
              ` : ''}
            </div>
            <div style="display:flex;gap:8px">
              ${modo === 'inbox' ? `
                <button type="button" class="acao-linha" id="btResponderMsg">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="9 17 4 12 9 7"/><path d="M20 18v-2a4 4 0 0 0-4-4H4"/></svg>
                  Responder
                </button>
              ` : ''}
              <button type="button" class="primario" id="btFecharMsgView">Fechar</button>
            </div>
          </div>
        </div>
      `;

      const mModal = modal(html);

      // Marcar como lida automaticamente se estiver na inbox
      if (modo === 'inbox' && !m.lida_em) {
        api(`/api/mensagens/${m.id}/ler`, { method: 'POST' }).then(() => {
          m.lida_em = new Date().toISOString();
          const card = cont.querySelector(`.msg-card[data-msgid="${m.id}"]`);
          if (card) {
            card.classList.remove('nao-lida');
            card.classList.add('lida');
            const dot = card.querySelector('.msg-dot');
            if (dot) dot.classList.add('lida');
          }
          atualizarBadges();
        }).catch(() => {});
      }

      mModal.querySelector('#btFecharMsgView').onclick = () => mModal.remove();

      const btnExc = mModal.querySelector('#btExcluirMsg');
      if (btnExc) {
        btnExc.onclick = async () => {
          if (!confirm('Deseja excluir esta mensagem da sua caixa de entrada?')) return;
          try {
            await api(`/api/mensagens/${m.id}/excluir`, { method: 'POST' });
            toast('Mensagem excluída.');
            mModal.remove();
            carregarInbox();
          } catch (e) {}
        };
      }

      const btnResp = mModal.querySelector('#btResponderMsg');
      if (btnResp) {
        btnResp.onclick = () => {
          mModal.remove();
          abrirModalCompor({
            destinatario_id: m.remetente ? m.remetente.papel_id : null,
            assunto: 'Re: ' + m.assunto
          });
        };
      }
    }

    async function abrirModalCompor(dadosIniciais) {
      dadosIniciais = dadosIniciais || {};
      let dests = [];
      try {
        dests = await api('/api/mensagens/destinatarios');
      } catch (e) {
        toast('Erro ao buscar lista de destinatários', 'erro');
        return;
      }

      const opcoesHtml = dests.map(d => {
        const funcao = d.funcao_nome ? ` — ${d.funcao_nome}` : '';
        const grupo = d.grupo_nome ? ` (${d.grupo_nome})` : ' (Global)';
        const pessoa = d.nome_guerra ? ` [${d.nome_guerra}]` : '';
        return `<option value="${d.id}">${esc(d.papel.toUpperCase())}${esc(grupo)}${esc(funcao)}${esc(pessoa)}</option>`;
      }).join('');

      const html = `
        <div class="modal" style="max-width:640px;width:95%">
          <h3 style="margin-top:0;display:flex;align-items:center;gap:8px">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            Nova Mensagem Institucional
          </h3>
          <p style="color:var(--tx2);font-size:12.5px;margin-bottom:14px">O envio será formalizado em nome da sua função atual.</p>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Destinatário (Função / Cadeia de Comando)</label>
            <select id="mDest" style="width:100%;height:40px">
              <option value="">Selecione o destinatário…</option>
              ${opcoesHtml}
            </select>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Assunto</label>
            <input type="text" id="mAssunto" value="${esc(dadosIniciais.assunto || '')}" placeholder="ex.: Diretriz de Serviço, Convocação, Relatório…" style="width:100%">
          </div>

          <div class="campo" style="margin-bottom:16px">
            <label style="font-weight:700">Mensagem</label>
            <textarea id="mCorpo" rows="6" placeholder="Digite aqui o texto da mensagem…" style="width:100%;resize:vertical"></textarea>
          </div>

          <div class="modal-acoes" style="justify-content:flex-end;gap:8px">
            <button type="button" class="acao-linha" id="mBtnCancel">Cancelar</button>
            <button type="button" class="primario" id="mBtnEnviar" style="padding:8px 22px">Enviar Mensagem</button>
          </div>
        </div>
      `;

      const mModal = modal(html);
      const q = s => mModal.querySelector(s);

      if (dadosIniciais.destinatario_id) {
        q('#mDest').value = String(dadosIniciais.destinatario_id);
      }

      q('#mBtnCancel').onclick = () => mModal.remove();

      q('#mBtnEnviar').onclick = async () => {
        const destId = +q('#mDest').value;
        const assunto = q('#mAssunto').value.trim();
        const corpo = q('#mCorpo').value.trim();

        if (!destId) { toast('Selecione o destinatário', 'erro'); return; }
        if (!assunto) { toast('Informe o assunto', 'erro'); return; }
        if (!corpo) { toast('Escreva a mensagem', 'erro'); return; }

        q('#mBtnEnviar').disabled = true;
        try {
          await api('/api/mensagens', {
            method: 'POST',
            body: JSON.stringify({
              destinatario_papel_ids: [destId],
              assunto: assunto,
              corpo: corpo
            })
          });
          toast('Mensagem enviada com sucesso!');
          mModal.remove();
          if (abaAtual === 'enviadas') carregarEnviadas();
        } catch (e) {
          q('#mBtnEnviar').disabled = false;
        }
      };
    }

    // Inicialização da tela
    carregarInbox();
  };

  /* Atualiza os badges de mensagens não lidas no sistema */
  async function atualizarBadges() {
    try {
      const res = await api('/api/mensagens/contador');
      const count = (res && res.nao_lidas) || 0;
      
      const badgeSide = document.getElementById('sbBadgeNotif');
      const badgeMob = document.getElementById('mobBadgeNotif');
      const badgeInbox = document.getElementById('badgeInboxAba');
      const badgeNav = document.getElementById('sbNavBadgeMsg');

      const txt = count === 1 ? '1 mensagem não lida' : `${count} mensagens não lidas`;
      const elMsg1 = document.getElementById('sinoHoverMsgTxt');
      if (elMsg1) elMsg1.textContent = txt;
      const elMsg2 = document.getElementById('mobSinoHoverMsgTxt');
      if (elMsg2) elMsg2.textContent = txt;

      const titleTxt = count > 0 ? `Mensagens (${count} não lidas)` : 'Mensagens e Notificações';
      const btS = document.getElementById('sbBtSinoNotif');
      if (btS) btS.setAttribute('title', titleTxt);
      const btM = document.getElementById('mobSinoNotif');
      if (btM) btM.setAttribute('title', titleTxt);

      [badgeSide, badgeMob, badgeInbox, badgeNav].forEach(b => {
        if (!b) return;
        b.textContent = String(count);
        if (count > 0) b.classList.remove('oculto');
        else b.classList.add('oculto');
      });
    } catch (e) {}
  }
  window.atualizarBadgesMensagens = atualizarBadges;

})();
