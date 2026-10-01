/* SCI — Módulo de Mensageria Interna, Despachos & Fórum de Avisos (v1.2 Fase 2)
   Comunicação formal institucional, Despachos com Retorno Obrigatório,
   Anexos, Pastas e Mural Gerencial com Registro de Ciente e Auditoria. */
(function () {
  'use strict';

  function quem() {
    return (typeof ME !== 'undefined' && ME) || window.ME || null;
  }

  /* Formata o nome completo destacando o nome de guerra em negrito */
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

  function formatarTamanho(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  window.ViewMensagens = async function (subAbaInicial) {
    const u = quem();
    if (!u) { location.hash = '#/login'; return; }
    if (window.navAtiva) navAtiva('#/mensagens');

    const app = document.getElementById('app');
    if (!app) return;

    let abaAtual = subAbaInicial || 'inbox';
    let pastaFiltroAtual = null;
    let pastasUsuario = [];

    app.innerHTML = `
      <div class="msg-header-topo">
        <div>
          <h2 style="margin:0 0 4px">Mensageria, Despachos & Fórum</h2>
          <p style="color:var(--tx2);font-size:13px;margin:0">Comunicação operacional, despachos auditáveis com retorno exigido e comunicados gerenciais.</p>
        </div>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          <button type="button" class="primario" id="btNovaMsg" style="display:flex;align-items:center;gap:6px;padding:8px 16px">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            Nova Mensagem / Despacho
          </button>
          ${(u.papel === 'gerente' || u.papel === 'admin') ? `
            <button type="button" class="acao-linha" id="btNovoAviso" style="display:flex;align-items:center;gap:6px;padding:8px 14px">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
              Publicar Aviso
            </button>
          ` : ''}
        </div>
      </div>

      <div class="msg-abas-container" style="flex-wrap:wrap">
        <button type="button" class="msg-aba-btn ${abaAtual === 'inbox' ? 'ativo' : ''}" id="abaInbox">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
          Entrada <span id="badgeInboxAba" class="badge-mini oculto">0</span>
        </button>
        <button type="button" class="msg-aba-btn ${abaAtual === 'despachos' ? 'ativo' : ''}" id="abaDespachos">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
          Despachos <span id="badgeDespachosAba" class="badge-mini oculto">0</span>
        </button>
        <button type="button" class="msg-aba-btn ${abaAtual === 'enviadas' ? 'ativo' : ''}" id="abaEnviadas">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
          Enviadas
        </button>
        <button type="button" class="msg-aba-btn ${abaAtual === 'arquivadas' ? 'ativo' : ''}" id="abaArquivadas">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="21 8 21 21 3 21 3 8"/><rect x="1" y="3" width="22" height="5"/><line x1="10" y1="12" x2="14" y2="12"/></svg>
          Arquivo
        </button>
        <button type="button" class="msg-aba-btn ${abaAtual === 'avisos' ? 'ativo' : ''}" id="abaAvisos">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
          Mural de Avisos
        </button>
      </div>

      <!-- Barra de Pastas Personalizadas (visível na Inbox) -->
      <div id="msgBarraPastas" style="display:flex;align-items:center;gap:8px;margin-bottom:14px;overflow-x:auto;padding-bottom:4px">
        <span style="font-size:12px;font-weight:700;color:var(--tx3);text-transform:uppercase">Pastas:</span>
        <button type="button" class="acao-linha pasta-chip ativo" data-pastaid="" style="font-size:12px;padding:4px 10px;border-radius:14px">Todas</button>
        <div id="listaPastasChips" style="display:flex;gap:6px"></div>
        <button type="button" class="acao-linha" id="btAddPasta" style="font-size:12px;padding:4px 8px;border-radius:14px" title="Criar nova pasta">+ Pasta</button>
      </div>

      <div id="msgConteudo">
        <div class="carregando">Carregando…</div>
      </div>
    `;

    const cont = document.getElementById('msgConteudo');
    const barraPastas = document.getElementById('msgBarraPastas');

    // Botões de Abas
    const abas = {
      inbox: document.getElementById('abaInbox'),
      despachos: document.getElementById('abaDespachos'),
      enviadas: document.getElementById('abaEnviadas'),
      arquivadas: document.getElementById('abaArquivadas'),
      avisos: document.getElementById('abaAvisos')
    };

    function alternarAba(novaAba) {
      abaAtual = novaAba;
      Object.keys(abas).forEach(k => {
        if (abas[k]) abas[k].classList.toggle('ativo', k === novaAba);
      });
      barraPastas.style.display = (novaAba === 'inbox') ? 'flex' : 'none';

      if (novaAba === 'inbox') carregarInbox();
      else if (novaAba === 'despachos') carregarDespachos();
      else if (novaAba === 'enviadas') carregarEnviadas();
      else if (novaAba === 'arquivadas') carregarArquivadas();
      else if (novaAba === 'avisos') carregarAvisos();
    }

    Object.keys(abas).forEach(k => {
      if (abas[k]) abas[k].onclick = () => alternarAba(k);
    });

    document.getElementById('btNovaMsg').onclick = () => abrirModalCompor();
    const btNovoAviso = document.getElementById('btNovoAviso');
    if (btNovoAviso) btNovoAviso.onclick = () => abrirModalNovoAviso();

    document.getElementById('btAddPasta').onclick = async () => {
      const nome = prompt('Nome da nova pasta pessoal:');
      if (!nome || !nome.trim()) return;
      try {
        await api('/api/mensagens/pastas', { method: 'POST', body: JSON.stringify({ nome: nome.trim() }) });
        toast('Pasta criada!');
        await carregarPastas();
      } catch (e) {}
    };

    async function carregarPastas() {
      try {
        pastasUsuario = await api('/api/mensagens/pastas');
        const contChips = document.getElementById('listaPastasChips');
        if (!contChips) return;
        contChips.innerHTML = pastasUsuario.map(p => `
          <button type="button" class="acao-linha pasta-chip ${pastaFiltroAtual === p.id ? 'ativo' : ''}" data-pastaid="${p.id}" style="font-size:12px;padding:4px 10px;border-radius:14px;display:inline-flex;align-items:center;gap:6px">
            📁 ${esc(p.nome)} <small style="opacity:0.7">(${p.total || 0})</small>
            <span class="del-pasta" data-delpasta="${p.id}" title="Excluir pasta" style="margin-left:2px;opacity:0.6">&times;</span>
          </button>
        `).join('');

        barraPastas.querySelectorAll('.pasta-chip').forEach(btn => {
          btn.onclick = (ev) => {
            if (ev.target.dataset.delpasta) {
              ev.stopPropagation();
              const pId = +ev.target.dataset.delpasta;
              if (confirm('Deseja excluir esta pasta? As mensagens permanecerão na Caixa de Entrada.')) {
                api(`/api/mensagens/pastas/${pId}`, { method: 'DELETE' }).then(() => {
                  toast('Pasta removida.');
                  if (pastaFiltroAtual === pId) pastaFiltroAtual = null;
                  carregarPastas().then(() => carregarInbox());
                });
              }
              return;
            }
            pastaFiltroAtual = btn.dataset.pastaid ? +btn.dataset.pastaid : null;
            barraPastas.querySelectorAll('.pasta-chip').forEach(b => b.classList.remove('ativo'));
            btn.classList.add('ativo');
            carregarInbox();
          };
        });
      } catch (e) {}
    }

    async function carregarInbox() {
      cont.innerHTML = '<div class="carregando">Carregando caixa de entrada…</div>';
      try {
        let url = '/api/mensagens/inbox';
        if (pastaFiltroAtual) url += '?pasta_id=' + pastaFiltroAtual;
        const msgs = await api(url);
        renderListaMensagens(msgs, 'inbox');
        atualizarBadges();
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar mensagens.</div>';
      }
    }

    async function carregarDespachos() {
      cont.innerHTML = '<div class="carregando">Carregando despachos…</div>';
      try {
        const msgs = await api('/api/mensagens/inbox?despacho=1');
        renderListaMensagens(msgs, 'despachos');
        atualizarBadges();
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar despachos.</div>';
      }
    }

    async function carregarArquivadas() {
      cont.innerHTML = '<div class="carregando">Carregando arquivo…</div>';
      try {
        const msgs = await api('/api/mensagens/inbox?arquivadas=1');
        renderListaMensagens(msgs, 'arquivadas');
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar arquivo.</div>';
      }
    }

    async function carregarEnviadas() {
      cont.innerHTML = '<div class="carregando">Carregando enviadas…</div>';
      try {
        const msgs = await api('/api/mensagens/enviadas');
        renderListaEnviadas(msgs);
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar enviadas.</div>';
      }
    }

    function renderListaMensagens(msgs, modo) {
      if (!msgs || msgs.length === 0) {
        let msgVazio = 'Sua caixa de entrada está limpa.';
        if (modo === 'despachos') msgVazio = 'Nenhum despacho ou diligência pendente.';
        if (modo === 'arquivadas') msgVazio = 'Nenhuma mensagem no arquivo.';

        cont.innerHTML = `
          <div class="msg-vazio-card">
            <svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
            <p>${msgVazio}</p>
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

        let badgeDespacho = '';
        if (m.tipo === 'despacho' || m.exige_resposta) {
          if (!m.respondido_em) {
            badgeDespacho = `<span class="badge-despacho-pendente">⚠️ DESPACHO: RESPOSTA EXIGIDA</span>`;
          } else {
            badgeDespacho = `<span class="badge-despacho-ok">✓ DESPACHO ATENDIDO</span>`;
          }
        }

        let badgeAnexos = '';
        if (m.anexos && m.anexos.length > 0) {
          badgeAnexos = `<span style="font-size:11.5px;color:var(--tx3);display:inline-flex;align-items:center;gap:3px">📎 ${m.anexos.length} anexo(s)</span>`;
        }

        return `
          <div class="msg-card ${lida ? 'lida' : 'nao-lida'}" data-msgid="${m.id}" style="cursor:pointer">
            <div class="msg-card-status">
              <span class="msg-dot ${lida ? 'lida' : ''}"></span>
            </div>
            <div class="msg-card-corpo-prev">
              <div class="msg-card-linha1">
                <span class="msg-remetente-tit">${nomeRem} · <small class="msg-funcao-tag">${esc(funcaoRem)}</small></span>
                <span class="msg-data-txt">${fmtData(m.criada_em)} ${fmtHora(m.criada_em)}</span>
              </div>
              <div class="msg-card-linha2" style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin:3px 0 5px">
                <span class="msg-papel-sub">${esc(papelRem)}</span>
                ${badgeDespacho}
                ${badgeAnexos}
              </div>
              <div class="msg-card-assunto">${esc(m.assunto)}</div>
              <div class="msg-card-resumo">${esc(m.corpo.substring(0, 130))}${m.corpo.length > 130 ? '…' : ''}</div>
            </div>
          </div>
        `;
      }).join('');

      cont.innerHTML = `<div class="msg-lista-wrapper">${html}</div>`;

      cont.querySelectorAll('.msg-card').forEach(card => {
        card.onclick = () => {
          const id = +card.dataset.msgid;
          const msgObj = msgs.find(x => x.id === id);
          if (msgObj) abrirModalVisualizar(msgObj, modo);
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

        let badgeDespacho = '';
        if (m.tipo === 'despacho' || m.exige_resposta) {
          const respondidos = dests.filter(d => !!d.respondido_em).length;
          badgeDespacho = `<span class="${respondidos === dests.length ? 'badge-despacho-ok' : 'badge-despacho-pendente'}">DESPACHO: ${respondidos}/${dests.length} RESPONDIDOS</span>`;
        }

        return `
          <div class="msg-card lida" data-msgid="${m.id}" style="cursor:pointer">
            <div class="msg-card-status">
              <span class="msg-dot lida"></span>
            </div>
            <div class="msg-card-corpo-prev">
              <div class="msg-card-linha1">
                <span class="msg-remetente-tit">Para: <b>${esc(destTxt)}</b></span>
                <span class="msg-data-txt">${fmtData(m.criada_em)} ${fmtHora(m.criada_em)}</span>
              </div>
              <div style="margin:2px 0 5px">${badgeDespacho}</div>
              <div class="msg-card-assunto">${esc(m.assunto)}</div>
              <div class="msg-card-resumo">${esc(m.corpo.substring(0, 130))}${m.corpo.length > 130 ? '…' : ''}</div>
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

      // Painel especial de Despacho
      let painelDespacho = '';
      const ehDespacho = m.tipo === 'despacho' || m.exige_resposta;
      if (ehDespacho) {
        if (!m.respondido_em) {
          painelDespacho = `
            <div style="background:rgba(239, 68, 68, 0.1);border:1px solid rgba(239,68,68,0.3);border-radius:6px;padding:12px 14px;margin-bottom:16px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px">
              <div>
                <b style="color:var(--verm-txt);display:block;margin-bottom:2px">⚠️ DESPACHO COM RETORNO OBRIGATÓRIO</b>
                <span style="font-size:12.5px;color:var(--tx2)">Esta solicitação exige resposta formal. O arquivamento ou exclusão está bloqueado até o atendimento.</span>
              </div>
              <button type="button" class="primario" id="btDespacharAgora" style="padding:6px 14px;font-size:12.5px">Responder Agora</button>
            </div>
          `;
        } else {
          painelDespacho = `
            <div style="background:rgba(16, 185, 129, 0.1);border:1px solid rgba(16,185,129,0.3);border-radius:6px;padding:10px 14px;margin-bottom:16px;font-size:12.5px;color:var(--verde-txt)">
              ✓ <b>Despacho Atendido:</b> Resposta formal enviada em ${fmtData(m.respondido_em)} às ${fmtHora(m.respondido_em)}.
            </div>
          `;
        }
      }

      // Anexos
      let anexosHtml = '';
      if (m.anexos && m.anexos.length > 0) {
        anexosHtml = `
          <div style="margin-top:16px;padding-top:12px;border-top:1px dashed var(--borda)">
            <b style="font-size:12px;color:var(--tx2);display:block;margin-bottom:8px">ANEXOS (${m.anexos.length}):</b>
            <div style="display:flex;gap:8px;flex-wrap:wrap">
              ${m.anexos.map((anx, idx) => `
                <a class="msg-anexo-item" data-anxidx="${idx}" download="${esc(anx.nome || 'anexo')}">
                  📎 ${esc(anx.nome || 'arquivo')} <small style="opacity:0.7">(${formatarTamanho(anx.tamanho)})</small>
                </a>
              `).join('')}
            </div>
          </div>
        `;
      }

      // Opções de Mover Pasta
      const opcoesPastas = (pastasUsuario || []).map(p => `
        <option value="${p.id}" ${m.pasta_id === p.id ? 'selected' : ''}>${esc(p.nome)}</option>
      `).join('');

      const html = `
        <div class="modal" style="max-width:700px;width:95%">
          ${painelDespacho}

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

          <div class="msg-view-corpo" style="margin-top:14px">${esc(m.corpo).replace(/\n/g, '<br>')}</div>

          ${anexosHtml}

          <div class="modal-acoes" style="justify-content:space-between;border-top:1px solid var(--borda);padding-top:14px;margin-top:20px;flex-wrap:wrap;gap:10px">
            <div style="display:flex;gap:8px;align-items:center">
              ${modo !== 'enviadas' ? `
                <select id="selMoverPasta" style="height:32px;font-size:12px;padding:2px 8px">
                  <option value="">📁 Mover p/ Pasta…</option>
                  ${opcoesPastas}
                </select>
                <button type="button" class="acao-linha" id="btArquivarMsg" title="${m.arquivada ? 'Desarquivar' : 'Arquivar'}">
                  ${m.arquivada ? 'Desarquivar' : 'Arquivar'}
                </button>
                <button type="button" class="acao-linha" id="btExcluirMsg" style="color:var(--verm-txt)" title="Excluir mensagem">
                  Excluir
                </button>
              ` : ''}
            </div>

            <div style="display:flex;gap:8px">
              ${modo !== 'enviadas' ? `
                <button type="button" class="acao-linha" id="btEncaminharMsg">
                  Encaminhar
                </button>
                <button type="button" class="primario" id="btResponderMsg">
                  ${ehDespacho ? 'Responder / Despachar' : 'Responder'}
                </button>
              ` : ''}
              <button type="button" class="fantasma" id="btFecharMsgView">Fechar</button>
            </div>
          </div>
        </div>
      `;

      const mModal = modal(html);

      // Download de Anexos
      if (m.anexos && m.anexos.length > 0) {
        mModal.querySelectorAll('.msg-anexo-item').forEach(link => {
          link.onclick = (e) => {
            const idx = +link.dataset.anxidx;
            const anx = m.anexos[idx];
            if (anx && anx.dados_base64) {
              link.href = anx.dados_base64;
            }
          };
        });
      }

      // Marcar como lida automaticamente se estiver na inbox
      if (modo !== 'enviadas' && !m.lida_em) {
        api(`/api/mensagens/${m.id}/ler`, { method: 'POST' }).then(() => {
          m.lida_em = new Date().toISOString();
          atualizarBadges();
        }).catch(() => {});
      }

      mModal.querySelector('#btFecharMsgView').onclick = () => mModal.remove();

      // Botões de Despacho / Resposta
      const acaoResponder = () => {
        mModal.remove();
        abrirModalCompor({
          destinatario_id: m.remetente ? m.remetente.papel_id : null,
          assunto: m.assunto.startsWith('Re: ') ? m.assunto : 'Re: ' + m.assunto,
          pai_id: m.id,
          tipo: ehDespacho ? 'despacho' : 'comum'
        });
      };

      const btnResp = mModal.querySelector('#btResponderMsg');
      if (btnResp) btnResp.onclick = acaoResponder;
      const btnDespNow = mModal.querySelector('#btDespacharAgora');
      if (btnDespNow) btnDespNow.onclick = acaoResponder;

      // Encaminhar
      const btnEnc = mModal.querySelector('#btEncaminharMsg');
      if (btnEnc) {
        btnEnc.onclick = () => {
          mModal.remove();
          abrirModalCompor({
            assunto: m.assunto.startsWith('Enc: ') ? m.assunto : 'Enc: ' + m.assunto,
            corpo: `\n\n--- Mensagem Encaminhada ---\nDe: ${rem.nome_guerra || rem.login} (${rem.papel})\nData: ${fmtData(m.criada_em)}\n\n${m.corpo}`,
            anexos: m.anexos || []
          });
        };
      }

      // Arquivar / Desarquivar
      const btnArq = mModal.querySelector('#btArquivarMsg');
      if (btnArq) {
        btnArq.onclick = async () => {
          if (ehDespacho && !m.respondido_em) {
            return toast('Despacho com resposta pendente. Não é possível arquivar antes de responder.', 'erro');
          }
          try {
            const endpoint = m.arquivada ? `/api/mensagens/${m.id}/desarquivar` : `/api/mensagens/${m.id}/arquivar`;
            await api(endpoint, { method: 'POST' });
            toast(m.arquivada ? 'Mensagem restaurada do arquivo.' : 'Mensagem arquivada.');
            mModal.remove();
            alternarAba(abaAtual);
          } catch (e) {}
        };
      }

      // Mover para Pasta
      const selPasta = mModal.querySelector('#selMoverPasta');
      if (selPasta) {
        selPasta.onchange = async () => {
          const pId = selPasta.value ? +selPasta.value : null;
          try {
            await api(`/api/mensagens/${m.id}/mover-pasta`, {
              method: 'POST',
              body: JSON.stringify({ pasta_id: pId })
            });
            toast('Mensagem movida de pasta.');
            mModal.remove();
            carregarPastas();
            alternarAba(abaAtual);
          } catch (e) {}
        };
      }

      // Excluir
      const btnExc = mModal.querySelector('#btExcluirMsg');
      if (btnExc) {
        btnExc.onclick = async () => {
          if (ehDespacho && !m.respondido_em) {
            return toast('Despacho com resposta pendente. Não é possível excluir antes de responder.', 'erro');
          }
          if (!confirm('Deseja excluir esta mensagem da sua caixa?')) return;
          try {
            await api(`/api/mensagens/${m.id}/excluir`, { method: 'POST' });
            toast('Mensagem excluída.');
            mModal.remove();
            alternarAba(abaAtual);
          } catch (e) {}
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

      let anexosCarregados = (dadosIniciais.anexos && Array.isArray(dadosIniciais.anexos)) ? [...dadosIniciais.anexos] : [];

      const html = `
        <div class="modal" style="max-width:680px;width:95%">
          <h3 style="margin-top:0;display:flex;align-items:center;gap:8px">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            Compor Mensagem Institucional ou Despacho
          </h3>

          <div style="display:flex;gap:12px;margin-bottom:14px;background:var(--painel2);padding:10px;border-radius:6px;border:1px solid var(--borda)">
            <label style="display:flex;align-items:center;gap:6px;cursor:pointer;font-weight:700">
              <input type="radio" name="rTipoMsg" value="comum" ${dadosIniciais.tipo !== 'despacho' ? 'checked' : ''}>
              Mensagem Comum
            </label>
            <label style="display:flex;align-items:center;gap:6px;cursor:pointer;font-weight:700;color:var(--verm-txt)">
              <input type="radio" name="rTipoMsg" value="despacho" ${dadosIniciais.tipo === 'despacho' ? 'checked' : ''}>
              ⚖️ Despacho (Retorno Obrigatório)
            </label>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Destinatário (Função / Cadeia de Comando)</label>
            <select id="mDest" style="width:100%;height:38px">
              <option value="">Selecione o destinatário…</option>
              ${opcoesHtml}
            </select>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Assunto</label>
            <input type="text" id="mAssunto" value="${esc(dadosIniciais.assunto || '')}" placeholder="ex.: Diretriz, Convocação, Relatório…" style="width:100%">
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Mensagem / Despacho</label>
            <textarea id="mCorpo" rows="6" placeholder="Digite aqui o texto…" style="width:100%;resize:vertical">${esc(dadosIniciais.corpo || '')}</textarea>
          </div>

          <!-- Upload de Anexos -->
          <div style="margin-bottom:16px">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">
              <label style="font-weight:700;font-size:12.5px">Anexos de Documentos:</label>
              <label class="acao-linha" style="cursor:pointer;font-size:12px;padding:3px 8px">
                📎 Adicionar Arquivo
                <input type="file" id="mInputAnexo" multiple style="display:none">
              </label>
            </div>
            <div id="mListaAnexos" style="display:flex;gap:6px;flex-wrap:wrap"></div>
          </div>

          <div class="modal-acoes" style="justify-content:flex-end;gap:8px">
            <button type="button" class="acao-linha" id="mBtnCancel">Cancelar</button>
            <button type="button" class="primario" id="mBtnEnviar" style="padding:8px 22px">Enviar</button>
          </div>
        </div>
      `;

      const mModal = modal(html);
      const q = s => mModal.querySelector(s);

      if (dadosIniciais.destinatario_id) {
        q('#mDest').value = String(dadosIniciais.destinatario_id);
      }

      function renderAnexosModal() {
        const contAnx = q('#mListaAnexos');
        contAnx.innerHTML = anexosCarregados.map((a, i) => `
          <span class="msg-anexo-item" style="font-size:11.5px">
            📄 ${esc(a.nome)} <small>(${formatarTamanho(a.tamanho)})</small>
            <span data-remanx="${i}" style="cursor:pointer;font-weight:bold;margin-left:4px;color:var(--verm-txt)">&times;</span>
          </span>
        `).join('');

        contAnx.querySelectorAll('[data-remanx]').forEach(btn => {
          btn.onclick = () => {
            anexosCarregados.splice(+btn.dataset.remanx, 1);
            renderAnexosModal();
          };
        });
      }
      renderAnexosModal();

      // Input file to base64
      q('#mInputAnexo').onchange = (e) => {
        const files = Array.from(e.target.files || []);
        files.forEach(file => {
          if (file.size > 25 * 1024 * 1024) {
            return toast(`Arquivo ${file.name} excede o limite de 25MB para mensagens`, 'erro');
          }
          const reader = new FileReader();
          reader.onload = () => {
            anexosCarregados.push({
              nome: file.name,
              tamanho: file.size,
              tipo: file.type,
              dados_base64: reader.result
            });
            renderAnexosModal();
          };
          reader.readAsDataURL(file);
        });
      };

      q('#mBtnCancel').onclick = () => mModal.remove();

      q('#mBtnEnviar').onclick = async () => {
        const destId = +q('#mDest').value;
        const assunto = q('#mAssunto').value.trim();
        const corpo = q('#mCorpo').value.trim();
        const tipo = q('input[name="rTipoMsg"]:checked').value;

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
              corpo: corpo,
              tipo: tipo,
              exige_resposta: tipo === 'despacho',
              pai_id: dadosIniciais.pai_id || null,
              anexos: anexosCarregados
            })
          });
          toast(tipo === 'despacho' ? 'Despacho emitido com sucesso!' : 'Mensagem enviada com sucesso!');
          mModal.remove();
          alternarAba(abaAtual);
        } catch (e) {
          q('#mBtnEnviar').disabled = false;
        }
      };
    }

    // ---------- MURAL DE AVISOS & COMUNICADOS GERENCIAIS ----------

    async function carregarAvisos() {
      cont.innerHTML = '<div class="carregando">Carregando mural de avisos…</div>';
      try {
        const avisos = await api('/api/avisos');
        renderListaAvisos(avisos);
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar mural de avisos.</div>';
      }
    }

    function renderListaAvisos(avisos) {
      if (!avisos || avisos.length === 0) {
        cont.innerHTML = `
          <div class="msg-vazio-card">
            <svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
            <p>Nenhum comunicado ou aviso ativo no mural.</p>
          </div>
        `;
        return;
      }

      const ehGerente = (u.papel === 'gerente' || u.papel === 'admin');

      const html = avisos.map(a => {
        const autor = a.autor || {};
        const nomeAutor = formatarNomeRemetente(autor.nome_completo, autor.nome_guerra);
        const grupoOrig = a.origem && a.origem.grupo_nome ? `<span class="badge-fixado" style="background:rgba(139,92,246,0.15);border-color:rgba(139,92,246,0.4);color:#a78bfa">📢 REPOSTADO DE: ${esc(a.origem.grupo_nome)}</span>` : '';
        const badgeFixado = a.fixado ? `<span class="badge-fixado">📌 FIXADO</span>` : '';

        // Botão de Ciente
        let btnCiente = '';
        if (a.meu_ciente) {
          btnCiente = `<span class="badge-despacho-ok" style="padding:4px 10px">✓ Ciente Registrado</span>`;
        } else {
          btnCiente = `<button type="button" class="primario" data-darcliente="${a.id}" style="padding:4px 12px;font-size:12px">✋ Dar Ciente</button>`;
        }

        // Opção de Repostar (para gerentes subordinados caso o aviso venha de outro grupo)
        let btnRepostar = '';
        if (ehGerente && a.grupo_id !== u.grupo_id) {
          btnRepostar = `<button type="button" class="acao-linha" data-repostar="${a.id}" style="font-size:12px;padding:3px 8px">🔄 Repostar p/ meu Grupo</button>`;
        }

        // Excluir (autor ou admin)
        let btnExcluir = '';
        if (u.papel === 'admin' || (autor.usuario_id === u.id)) {
          btnExcluir = `<button type="button" class="acao-linha" data-delaviso="${a.id}" style="font-size:12px;color:var(--verm-txt);padding:3px 8px">Excluir</button>`;
        }

        return `
          <div class="aviso-card ${a.fixado ? 'fixado' : ''}" id="avisoCard_${a.id}">
            <div class="aviso-card-topo">
              <div>
                <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;flex-wrap:wrap">
                  ${badgeFixado}
                  ${grupoOrig}
                  <span style="font-size:12px;color:var(--tx3)">Publicado em <b>${esc(a.grupo_nome)}</b> · ${fmtData(a.criado_em)} ${fmtHora(a.criado_em)}</span>
                </div>
                <h4 class="aviso-card-titulo">${esc(a.titulo)}</h4>
                <div style="font-size:12.5px;color:var(--tx2)">Por: ${nomeAutor} (${esc(autor.papel.toUpperCase())})</div>
              </div>
              <div style="display:flex;align-items:center;gap:6px">
                ${btnRepostar}
                ${btnExcluir}
              </div>
            </div>

            <div class="aviso-card-corpo">${esc(a.conteudo)}</div>

            <div class="aviso-card-rodape">
              <div style="display:flex;align-items:center;gap:8px">
                ${btnCiente}
                <button type="button" class="acao-linha" data-vercientes="${a.id}" style="font-size:12px;padding:3px 8px">
                  👥 Ver Cientes (${a.total_cientes || 0})
                </button>
              </div>

              <div style="display:flex;align-items:center;gap:8px">
                <button type="button" class="acao-linha" data-togglecomm="${a.id}" style="font-size:12px;padding:3px 8px">
                  💬 Comentários (${a.total_comentarios || 0})
                </button>
              </div>
            </div>

            <div class="aviso-comentarios-box" id="boxComm_${a.id}" style="display:none">
              <div class="carregando">Carregando comentários…</div>
            </div>
          </div>
        `;
      }).join('');

      cont.innerHTML = `<div>${html}</div>`;

      // Handlers do mural
      cont.querySelectorAll('[data-darcliente]').forEach(b => {
        b.onclick = async () => {
          const id = +b.dataset.darcliente;
          try {
            await api(`/api/avisos/${id}/ciente`, { method: 'POST' });
            toast('Ciente registrado com sucesso!');
            carregarAvisos();
            atualizarBadges();
          } catch (e) {}
        };
      });

      cont.querySelectorAll('[data-repostar]').forEach(b => {
        b.onclick = async () => {
          const id = +b.dataset.repostar;
          if (!confirm('Deseja repostar este comunicado formalmente no mural do seu grupo?')) return;
          try {
            await api(`/api/avisos/${id}/repostar`, { method: 'POST' });
            toast('Aviso repostado para sua equipe!');
            carregarAvisos();
          } catch (e) {}
        };
      });

      cont.querySelectorAll('[data-delaviso]').forEach(b => {
        b.onclick = async () => {
          const id = +b.dataset.delaviso;
          if (!confirm('Deseja remover este aviso do mural?')) return;
          try {
            await api(`/api/avisos/${id}`, { method: 'DELETE' });
            toast('Aviso excluído.');
            carregarAvisos();
          } catch (e) {}
        };
      });

      cont.querySelectorAll('[data-vercientes]').forEach(b => {
        b.onclick = async () => {
          const id = +b.dataset.vercientes;
          try {
            const det = await api(`/api/avisos/${id}/detalhes`);
            abrirModalCientes(det.cientes || []);
          } catch (e) {}
        };
      });

      cont.querySelectorAll('[data-togglecomm]').forEach(b => {
        b.onclick = async () => {
          const id = +b.dataset.togglecomm;
          const box = document.getElementById(`boxComm_${id}`);
          if (!box) return;
          if (box.style.display === 'block') {
            box.style.display = 'none';
            return;
          }
          box.style.display = 'block';
          carregarComentariosAviso(id, box);
        };
      });
    }

    async function carregarComentariosAviso(avisoId, box) {
      box.innerHTML = '<div class="carregando">Carregando…</div>';
      try {
        const det = await api(`/api/avisos/${avisoId}/detalhes`);
        const lista = det.comentarios || [];

        let commHtml = lista.map(c => `
          <div class="aviso-comentario-item">
            <div style="display:flex;justify-content:space-between;margin-bottom:2px">
              <b>${esc(c.nome_guerra || c.login)} <small style="font-weight:normal;opacity:0.8">(${esc(c.papel)})</small></b>
              <span style="font-size:11px;color:var(--tx3)">${fmtData(c.criado_em)} ${fmtHora(c.criado_em)}</span>
            </div>
            <div>${esc(c.comentario)}</div>
          </div>
        `).join('');

        if (lista.length === 0) {
          commHtml = '<div style="font-size:12px;color:var(--tx3);padding:6px 0">Nenhum comentário nesta thread ainda.</div>';
        }

        box.innerHTML = `
          <div style="margin-bottom:10px">${commHtml}</div>
          <div style="display:flex;gap:8px">
            <input type="text" id="inComm_${avisoId}" placeholder="Escreva um comentário ou apontamento…" style="flex:1;height:34px;font-size:12.5px">
            <button type="button" class="primario" id="btEnvComm_${avisoId}" style="padding:0 14px;font-size:12.5px">Enviar</button>
          </div>
        `;

        box.querySelector(`#btEnvComm_${avisoId}`).onclick = async () => {
          const input = box.querySelector(`#inComm_${avisoId}`);
          const txt = input.value.trim();
          if (!txt) return;
          try {
            await api(`/api/avisos/${avisoId}/comentar`, {
              method: 'POST',
              body: JSON.stringify({ comentario: txt })
            });
            input.value = '';
            carregarComentariosAviso(avisoId, box);
          } catch (e) {}
        };
      } catch (e) {
        box.innerHTML = '<div class="vazio">Falha ao obter comentários.</div>';
      }
    }

    function abrirModalCientes(cientes) {
      const html = `
        <div class="modal" style="max-width:520px;width:95%">
          <h3 style="margin-top:0">Militares que deram Ciente</h3>
          <p style="color:var(--tx2);font-size:12.5px;margin-bottom:12px">Auditoria e confirmação de leitura deste comunicado:</p>

          <div style="max-height:360px;overflow-y:auto;border:1px solid var(--borda);border-radius:6px;padding:6px 12px">
            ${cientes.length === 0 ? '<div style="padding:20px;text-align:center;color:var(--tx3)">Nenhum ciente registrado até o momento.</div>' : ''}
            ${cientes.map(c => `
              <div style="display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px dashed var(--borda)">
                <div>
                  <b>${esc(c.nome_completo || c.nome_guerra || c.login)}</b>
                  <div style="font-size:11.5px;color:var(--tx3)">Guerra: ${esc(c.nome_guerra || '—')} (${esc(c.papel)})</div>
                </div>
                <div style="font-size:12px;color:var(--verde-txt);text-align:right">
                  ✓ ${fmtData(c.ciente_em)}<br><small>${fmtHora(c.ciente_em)}</small>
                </div>
              </div>
            `).join('')}
          </div>

          <div class="modal-acoes" style="justify-content:flex-end;margin-top:14px">
            <button type="button" class="primario" id="btFecharCientes">Fechar</button>
          </div>
        </div>
      `;
      const m = modal(html);
      m.querySelector('#btFecharCientes').onclick = () => m.remove();
    }

    function abrirModalNovoAviso() {
      const html = `
        <div class="modal" style="max-width:620px;width:95%">
          <h3 style="margin-top:0">Publicar Novo Aviso no Mural</h3>
          <p style="color:var(--tx2);font-size:12.5px;margin-bottom:14px">Comunicado formal visível para toda a equipe subordinada, com confirmação obrigatória de ciente.</p>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Título do Comunicado *</label>
            <input type="text" id="avTitulo" placeholder="ex.: Diretrizes para Operação Especial, Escalas de Serviço…" style="width:100%">
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Conteúdo do Aviso *</label>
            <textarea id="avConteudo" rows="6" placeholder="Descreva os detalhes, determinações e prazos…" style="width:100%;resize:vertical"></textarea>
          </div>

          <div class="campo" style="margin-bottom:16px">
            <label style="display:flex;align-items:center;gap:6px;cursor:pointer">
              <input type="checkbox" id="avFixado">
              📌 <b>Fixar no topo do mural</b> (destaque contínuo)
            </label>
          </div>

          <div class="modal-acoes" style="justify-content:flex-end;gap:8px">
            <button type="button" class="acao-linha" id="avCancel">Cancelar</button>
            <button type="button" class="primario" id="avPublicar" style="padding:8px 20px">Publicar no Mural</button>
          </div>
        </div>
      `;
      const m = modal(html);
      m.querySelector('#avCancel').onclick = () => m.remove();
      m.querySelector('#avPublicar').onclick = async () => {
        const tit = m.querySelector('#avTitulo').value.trim();
        const cont = m.querySelector('#avConteudo').value.trim();
        const fix = m.querySelector('#avFixado').checked;

        if (!tit || !cont) return toast('Preencha título e conteúdo', 'erro');

        m.querySelector('#avPublicar').disabled = true;
        try {
          await api('/api/avisos', {
            method: 'POST',
            body: JSON.stringify({ titulo: tit, conteudo: cont, fixado: fix })
          });
          toast('Aviso publicado com sucesso no mural!');
          m.remove();
          alternarAba('avisos');
        } catch (e) {
          m.querySelector('#avPublicar').disabled = false;
        }
      };
    }

    // Inicialização da tela
    await carregarPastas();
    alternarAba(abaAtual);
  };

  /* Atualiza os badges de mensagens, despachos e avisos no sistema */
  async function atualizarBadges() {
    try {
      const res = await api('/api/mensagens/contador');
      const count = (res && res.nao_lidas) || 0;
      const despachos = (res && res.despachos_pendentes) || 0;

      const badgeSide = document.getElementById('sbBadgeNotif');
      const badgeMob = document.getElementById('mobBadgeNotif');
      const badgeInbox = document.getElementById('badgeInboxAba');
      const badgeDesp = document.getElementById('badgeDespachosAba');
      const badgeNav = document.getElementById('sbNavBadgeMsg');

      if (badgeDesp) {
        badgeDesp.textContent = String(despachos);
        if (despachos > 0) badgeDesp.classList.remove('oculto');
        else badgeDesp.classList.add('oculto');
      }

      const totalPendencias = count + despachos;
      const txt = totalPendencias === 1 ? '1 pendência na mensageria' : `${totalPendencias} mensagens/despachos pendentes`;
      const elMsg1 = document.getElementById('sinoHoverMsgTxt');
      if (elMsg1) elMsg1.textContent = txt;
      const elMsg2 = document.getElementById('mobSinoHoverMsgTxt');
      if (elMsg2) elMsg2.textContent = txt;

      const titleTxt = totalPendencias > 0 ? `Mensagens (${totalPendencias} pendências)` : 'Mensagens e Notificações';
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
