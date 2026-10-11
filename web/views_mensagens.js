/* SCI — Email (renome v1.6.0; ex-Email Interno / ex-Mensageria), Despachos & Fórum de Avisos (v1.2 Fase 2; renome/abas nested ordem Diretor 04/10)
   Comunicação formal institucional, Despachos com Retorno Obrigatório,
   Anexos, Pastas e Mural Gerencial com Registro de Ciente e Auditoria. */
(function () {
  'use strict';

  function quem() {
    return (typeof ME !== 'undefined' && ME) || window.ME || null;
  }

  /* v1.6.0-contextos: título do módulo com a subordinação do CONTEXTO ativo —
     fonte ME.grupo_nome / ME.setor_nome (mesmo critério do textoContextoUsuario
     do core.js: conta sem grupo mostra "Global" quando admin; com setor no
     contexto, acrescenta " / <setor>"). */
  function tituloContexto(prefixo) {
    const u = quem();
    const grupo = (u && u.grupo_nome && u.grupo_nome.trim())
      ? u.grupo_nome.trim()
      : ((u && u.papel === 'admin') ? 'Global' : 'Sem grupo');
    const setor = (u && u.setor_nome && u.setor_nome.trim()) ? u.setor_nome.trim() : '';
    return (setor && grupo !== 'Global') ? `${prefixo} — ${grupo} / ${setor}` : `${prefixo} — ${grupo}`;
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
          <h2 style="margin:0 0 4px">${esc(tituloContexto('EMAIL'))}</h2>
          <p style="color:var(--tx2);font-size:13px;margin:0">Caixa de entrada e enviadas para mensagens convencionais; aba própria de despachos, com retorno exigido e finalização pelo destinatário.</p>
        </div>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          <button type="button" class="primario" id="btNovaMsg" style="display:flex;align-items:center;gap:6px;padding:8px 16px">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            <span id="btNovaMsgRotulo">Nova Mensagem</span>
          </button>
        </div>
      </div>

      <!-- Linha 1 (ordem Diretor 05/10): CAIXA DE ENTRADA × DESPACHO -->
      <div class="msg-abas-container" style="display:flex;gap:8px;margin-bottom:8px;flex-wrap:wrap">
        <button type="button" class="msg-aba-btn ${['inbox','enviadas','arquivadas'].includes(abaAtual) ? 'ativo' : ''}" id="linhaEntrada" style="padding:8px 18px">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
          Caixa de Entrada <span id="badgeInboxAba" class="badge-mini oculto">0</span>
        </button>
        <button type="button" class="msg-aba-btn ${['despachos','despachos_enviadas','arquivo_despachos'].includes(abaAtual) ? 'ativo' : ''}" id="linhaDespacho" style="padding:8px 18px">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
          Despacho <span id="badgeDespachosAba" class="badge-mini oculto">0</span>
        </button>
      </div>

      <!-- Linha 2: ENTRADA/ENVIADAS (do grupo ativo) -->
      <div class="msg-abas-container" style="display:flex;gap:6px;margin-bottom:14px;flex-wrap:wrap">
        <button type="button" class="msg-aba-btn msg-subaba ${abaAtual === 'inbox' ? 'ativo' : ''}" data-aba="inbox" style="padding:6px 14px;font-size:12.5px">Entrada</button>
        <button type="button" class="msg-aba-btn msg-subaba ${abaAtual === 'enviadas' ? 'ativo' : ''}" data-aba="enviadas" style="padding:6px 14px;font-size:12.5px">Enviadas</button>
        <button type="button" class="msg-aba-btn msg-subaba ${abaAtual === 'arquivadas' ? 'ativo' : ''}" data-aba="arquivadas" style="padding:6px 14px;font-size:12.5px">Arquivo</button>
        <button type="button" class="msg-aba-btn msg-subaba ${abaAtual === 'despachos' ? 'ativo' : ''}" data-aba="despachos" style="padding:6px 14px;font-size:12.5px">Recebidas</button>
        <button type="button" class="msg-aba-btn msg-subaba ${abaAtual === 'despachos_enviadas' ? 'ativo' : ''}" data-aba="despachos_enviadas" style="padding:6px 14px;font-size:12.5px">Enviadas (Despacho)</button>
        <button type="button" class="msg-aba-btn msg-subaba ${abaAtual === 'arquivo_despachos' ? 'ativo' : ''}" data-aba="arquivo_despachos" style="padding:6px 14px;font-size:12.5px">Arquivo (Despacho)</button>
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

    // Abas NESTED (ordem 04/10): dois grupos — Convencional (inbox/enviadas/arquivadas)
    // e Despachos (recebidas/enviadas/arquivo). Dropdown estilizado por grupo.
    const GRUPO_DE = {
      inbox: 'convencional', enviadas: 'convencional', arquivadas: 'convencional',
      despachos: 'despachos', despachos_enviadas: 'despachos', arquivo_despachos: 'despachos'
    };
    const ROTULO_ABA = {
      inbox: 'Caixa de Entrada', enviadas: 'Enviadas', arquivadas: 'Arquivo de Mensagens',
      despachos: 'Despachos Recebidos', despachos_enviadas: 'Despachos Enviados',
      arquivo_despachos: 'Arquivo de Despachos'
    };
    const ITENS_GRUPO = {
      convencional: ['inbox', 'enviadas', 'arquivadas'],
      despachos: ['despachos', 'despachos_enviadas', 'arquivo_despachos']
    };

    const btEntrada = document.getElementById('linhaEntrada');
    const btDespLinha = document.getElementById('linhaDespacho');
    const subAbas = Array.from(document.querySelectorAll('.msg-subaba'));

    // Visibilidade da linha 2 conforme a linha 1 ativa (ordem Diretor 05/10:
    // 2 linhas de abas — tipo da mensagem em cima, entrada/enviadas embaixo)
    function marcarGruposAtivos() {
      const g = GRUPO_DE[abaAtual] || 'convencional';
      if (btEntrada) btEntrada.classList.toggle('ativo', g === 'convencional');
      if (btDespLinha) btDespLinha.classList.toggle('ativo', g === 'despachos');
      subAbas.forEach(b => {
        const aba = b.dataset.aba;
        b.style.display = (GRUPO_DE[aba] === g) ? '' : 'none';
        b.classList.toggle('ativo', aba === abaAtual);
      });
      const rot = document.getElementById('btNovaMsgRotulo');
      if (rot) rot.textContent = g === 'despachos' ? 'Novo Despacho' : 'Nova Mensagem';
    }

    if (btEntrada) btEntrada.onclick = () => alternarAba('inbox');
    if (btDespLinha) btDespLinha.onclick = () => alternarAba('despachos');
    subAbas.forEach(b => { b.onclick = () => alternarAba(b.dataset.aba); });

    function alternarAba(novaAba) {
      abaAtual = novaAba;
      marcarGruposAtivos();
      barraPastas.style.display = (novaAba === 'inbox') ? 'flex' : 'none';

      if (novaAba === 'inbox') carregarInbox();
      else if (novaAba === 'despachos') carregarDespachos();
      else if (novaAba === 'despachos_enviadas') carregarDespachosEnviadas();
      else if (novaAba === 'arquivo_despachos') carregarArquivoDespachos();
      else if (novaAba === 'enviadas') carregarEnviadas();
      else if (novaAba === 'arquivadas') carregarArquivadas();
    }

    document.getElementById('btNovaMsg').onclick = () => {
      const g = GRUPO_DE[abaAtual] || 'convencional';
      abrirModalCompor({ tipo: g === 'despachos' ? 'despacho' : 'comum' });
    };

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
      cont.innerHTML = '<div class="carregando">Carregando despachos recebidos…</div>';
      try {
        const msgs = await api('/api/mensagens/inbox?despacho=1');
        renderListaMensagens(msgs, 'despachos');
        atualizarBadges();
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar despachos.</div>';
      }
    }

    async function carregarDespachosEnviadas() {
      cont.innerHTML = '<div class="carregando">Carregando despachos enviados…</div>';
      try {
        const msgs = await api('/api/mensagens/enviadas?despacho=1');
        renderListaEnviadas(msgs, true);
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar despachos enviados.</div>';
      }
    }

    async function carregarArquivoDespachos() {
      cont.innerHTML = '<div class="carregando">Carregando arquivo de despachos…</div>';
      try {
        const msgs = await api('/api/mensagens/inbox?despacho=1&arquivadas=1');
        renderListaMensagens(msgs, 'arquivo_despachos');
      } catch (e) {
        cont.innerHTML = '<div class="vazio" style="padding:40px;text-align:center">Falha ao carregar arquivo de despachos.</div>';
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

      // Onda 05/10 (caixa da função): itens possesso da função (da_funcao=true)
      // agrupados na própria seção "Caixa da Função" no TOPO — sem duplicação
      // (a API já entrega cada item uma única vez) e sem arquivamento
      // (destID NULL; trava server-side).
      // v1.6.0-contextos: a listagem vira TABELA (thead/tbody, padrão do
      // restyle) — o conteúdo da era dos cards é preservado; a linha inteira
      // abre a thread e a célula final traz o botão "abrir".
      const linhaMsg = (m, daFuncao) => {
        const lida = daFuncao ? true : !!m.lida_em;
        const rem = m.remetente || {};
        const nomeRem = formatarNomeRemetente(rem.nome_completo, rem.nome_guerra);
        const funcaoRem = daFuncao ? (rem.nome_exibicao || rem.funcao_nome || 'Caixa da Função') : (rem.funcao_nome || 'Função Organizacional');
        const papelRem = (rem.papel || '').toUpperCase() + (rem.grupo_nome ? ' — ' + rem.grupo_nome : ' — Global');

        let badgeDespacho = '';
        if (m.tipo === 'despacho' || m.exige_resposta) {
          if (m.finalizado_em) {
            badgeDespacho = `<span class="badge-despacho-ok">✔️ DESPACHO FINALIZADO</span>`;
          } else if (!m.respondido_em) {
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
          <tr class="msg-linha-tabela" data-msgid="${m.id}" style="cursor:pointer">
            <td style="white-space:nowrap;${daFuncao ? 'box-shadow:inset 3px 0 0 var(--ambar)' : ''}">
              <span style="display:inline-flex;align-items:center;gap:8px">
                <span class="msg-dot ${lida ? 'lida' : ''}"></span>
                ${badgeDespacho || `<span style="color:var(--tx3)" title="${lida ? 'Lida' : 'Não lida'}">—</span>`}
              </span>
            </td>
            <td>
              <span class="msg-remetente-tit">${nomeRem}</span>
              <small class="msg-funcao-tag">${esc(funcaoRem)}</small>
              ${daFuncao ? '<span class="badge-mini" style="background:var(--ambar);color:#1a1a0d;margin-left:6px" title="Possesso da função que você exerce">da função</span>' : ''}
              <div style="font-size:11.5px;color:var(--tx3);margin-top:2px">${esc(papelRem)}</div>
            </td>
            <td>
              <b style="${lida ? '' : 'color:var(--verde-claro)'}">${esc(m.assunto)}</b>
              ${badgeAnexos ? `<div style="margin-top:3px">${badgeAnexos}</div>` : ''}
            </td>
            <td style="white-space:nowrap">${fmtData(m.criada_em)} ${fmtHora(m.criada_em)}</td>
            <td><div class="gpx-acoes" style="justify-content:flex-end"><button type="button" class="acao-linha" data-abrirmsg="${m.id}" title="Abrir thread">abrir</button></div></td>
          </tr>
        `;
      };

      const tabelaMsgs = linhas => `
        <div class="rolagem"><table>
          <thead><tr>
            <th style="width:230px">Status</th>
            <th style="width:26%">De</th>
            <th>Assunto</th>
            <th style="white-space:nowrap">Recebido em</th>
            <th style="text-align:right">Ações</th>
          </tr></thead>
          <tbody>${linhas.join('')}</tbody>
        </table></div>
      `;

      const daFuncao = msgs.filter(m => m.da_funcao);
      const pessoais = msgs.filter(m => !m.da_funcao);
      let secaoFuncao = '';
      if (daFuncao.length > 0) {
        secaoFuncao = `
          <div style="display:flex;align-items:center;gap:8px;margin:2px 0 8px">
            <span style="font-size:13px;font-weight:700;color:var(--ambar)">📋 Caixa da Função (${daFuncao.length})</span>
            <small style="color:var(--tx3);font-size:11px">possesso da função — permanece na troca de titular; não arquivável</small>
          </div>
          <div style="margin-bottom:18px">${tabelaMsgs(daFuncao.map(m => linhaMsg(m, true)))}</div>
        `;
      }
      const secaoPessoal = pessoais.length > 0
        ? tabelaMsgs(pessoais.map(m => linhaMsg(m, false)))
        : '';

      cont.innerHTML = secaoFuncao + secaoPessoal;

      cont.querySelectorAll('.msg-linha-tabela').forEach(tr => {
        tr.onclick = () => {
          const id = +tr.dataset.msgid;
          const msgObj = msgs.find(x => x.id === id);
          if (msgObj) abrirModalVisualizar(msgObj, modo);
        };
      });
    }

    function renderListaEnviadas(msgs, soDespachos) {
      // Onda 05/10: o filtro de despachos é feito pela API (?despacho=1);
      // mantido como 2ª linha de defesa para payload legado em cache.
      if (soDespachos) msgs = (msgs || []).filter(m => m.tipo === 'despacho');
      if (!msgs || msgs.length === 0) {
        cont.innerHTML = `
          <div class="msg-vazio-card">
            <svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
            <p>${soDespachos ? 'Nenhum despacho enviado por esta função ainda.' : 'Nenhuma mensagem enviada por esta função ainda.'}</p>
          </div>
        `;
        return;
      }

      // v1.6.0-contextos: listagem de enviadas em TABELA (padrão do restyle) —
      // colunas Status (andamento do despacho) / Para / Assunto / Data / Ações;
      // a linha inteira continua abrindo a thread.
      const linhas = msgs.map(m => {
        const dests = m.destinatarios || [];
        const destTxt = dests.map(d => `${(d.papel || '').toUpperCase()}${d.grupo_nome ? ' (' + d.grupo_nome + ')' : ''}`).join(', ') || 'Sem destinatários';

        let badgeDespacho = '';
        if (m.tipo === 'despacho' || m.exige_resposta) {
          const respondidos = dests.filter(d => !!d.respondido_em).length;
          badgeDespacho = m.finalizado_em
            ? `<span class="badge-despacho-ok">DESPACHO FINALIZADO: ${respondidos}/${dests.length} RESPONDIDOS</span>`
            : `<span class="${respondidos === dests.length ? 'badge-despacho-ok' : 'badge-despacho-pendente'}">DESPACHO: ${respondidos}/${dests.length} RESPONDIDOS</span>`;
        }

        return `
          <tr class="msg-linha-tabela" data-msgid="${m.id}" style="cursor:pointer">
            <td style="white-space:nowrap">${badgeDespacho || '<span style="color:var(--tx3)">—</span>'}</td>
            <td><span class="msg-remetente-tit">Para: <b>${esc(destTxt)}</b></span></td>
            <td><b>${esc(m.assunto)}</b></td>
            <td style="white-space:nowrap">${fmtData(m.criada_em)} ${fmtHora(m.criada_em)}</td>
            <td><div class="gpx-acoes" style="justify-content:flex-end"><button type="button" class="acao-linha" data-abrirmsg="${m.id}" title="Abrir thread">abrir</button></div></td>
          </tr>
        `;
      });

      cont.innerHTML = `
        <div class="rolagem"><table>
          <thead><tr>
            <th style="width:260px">Status</th>
            <th style="width:26%">Para</th>
            <th>Assunto</th>
            <th style="white-space:nowrap">Enviado em</th>
            <th style="text-align:right">Ações</th>
          </tr></thead>
          <tbody>${linhas.join('')}</tbody>
        </table></div>
      `;

      cont.querySelectorAll('.msg-linha-tabela').forEach(tr => {
        tr.onclick = () => {
          const id = +tr.dataset.msgid;
          const msgObj = msgs.find(x => x.id === id);
          if (msgObj) abrirModalVisualizar(msgObj, soDespachos ? 'despachos_enviadas' : 'enviadas');
        };
      });
    }

    async function abrirModalVisualizar(m, modo) {
      let thread;
      try {
        thread = await api('/api/mensagens/' + m.id + '/thread');
      } catch (e) {
        toast('Erro ao carregar detalhes da mensagem', 'erro');
        return;
      }

      const msg = thread.mensagem;
      const rem = msg.remetente || {};
      const nomeRem = formatarNomeRemetente(rem.nome_completo, rem.nome_guerra);
      const funcaoRem = rem.funcao_nome || 'Função não definida';
      const papelRem = (rem.papel || '').toUpperCase() + (rem.grupo_nome ? ' — ' + rem.grupo_nome : ' — Global');

      const ehDespacho = msg.tipo === 'despacho' || msg.exige_resposta;
      const finalizado = !!msg.finalizado_em;

      // Status do Despacho
      let bannerDespacho = '';
      if (ehDespacho && finalizado) {
        bannerDespacho = `
          <div style="background:rgba(16, 185, 129, 0.1);border:1px solid rgba(16,185,129,0.3);border-radius:6px;padding:10px 14px;margin-bottom:14px;font-size:12.5px;color:var(--verde-txt)">
            ✔️ <b>Despacho FINALIZADO</b> em ${fmtData(msg.finalizado_em)} às ${fmtHora(msg.finalizado_em)} — não exige mais retorno; segue como mensagem comum no arquivo e nas caixas.
          </div>
        `;
      } else if (ehDespacho) {
        if (thread.minha_resposta_pendente) {
          bannerDespacho = `
            <div style="background:rgba(239, 68, 68, 0.12);border:1px solid rgba(239,68,68,0.35);border-radius:6px;padding:12px 14px;margin-bottom:14px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px">
              <div>
                <b style="color:var(--verm-txt);display:block;margin-bottom:2px">⚠️ DESPACHO COM RETORNO OBRIGATÓRIO</b>
                <span style="font-size:12.5px;color:var(--tx2)">Esta solicitação exige resposta formal. O arquivamento ou exclusão está bloqueado até o envio da resposta abaixo nesta thread.</span>
              </div>
              <div style="display:flex;gap:8px;flex-wrap:wrap">
                <button type="button" class="primario" id="btRolarParaResposta" style="padding:6px 14px;font-size:12.5px;white-space:nowrap">⬇️ Responder Abaixo</button>
                <button type="button" class="acao-linha" id="btFinalizarDespacho" style="padding:6px 14px;font-size:12.5px;white-space:nowrap" title="Encerra o despacho sem resposta formal: vira mensagem comum no arquivo e nas caixas normais">✔️ Finalizar Despacho</button>
              </div>
            </div>
          `;
        } else if (thread.eh_destinatario) {
          // Onda 05/10: resposta SEM finalizar — respondido segue com botão de
          // FINALIZAR (resposta e finalização são ações independentes).
          const respTxt = thread.meu_respondido_em
            ? `Resposta formal registrada em ${fmtData(thread.meu_respondido_em)} às ${fmtHora(thread.meu_respondido_em)}.`
            : '';
          bannerDespacho = `
            <div style="background:rgba(16, 185, 129, 0.1);border:1px solid rgba(16,185,129,0.3);border-radius:6px;padding:10px 14px;margin-bottom:14px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px;font-size:12.5px">
              <span style="color:var(--verde-txt)">✓ <b>Despacho Atendido:</b> ${respTxt} A thread segue aberta para acompanhamentos.</span>
              <button type="button" class="acao-linha" id="btFinalizarDespacho" style="padding:6px 14px;font-size:12.5px;white-space:nowrap" title="Encerra o despacho definitivamente: vira mensagem comum no arquivo e nas caixas normais">✔️ Finalizar Despacho</button>
            </div>
          `;
        } else if (thread.eh_remetente) {
          const destsAudit = thread.destinatarios || [];
          bannerDespacho = `
            <div style="background:var(--painel2);border:1px solid var(--borda);border-radius:6px;padding:10px 14px;margin-bottom:14px;font-size:12px">
              <b style="display:block;margin-bottom:4px;color:var(--tx1)">⚖️ AUDITORIA DO DESPACHO:</b>
              <div style="display:flex;flex-direction:column;gap:4px">
                ${destsAudit.map(d => {
                  const nome = formatarNomeRemetente(d.nome_completo, d.nome_guerra);
                  const pTxt = (d.papel || '').toUpperCase() + (d.grupo_nome ? ' — ' + d.grupo_nome : '');
                  const func = d.funcao_nome ? ` (${d.funcao_nome})` : '';
                  let statusResp = d.respondido_em 
                    ? `<span style="color:var(--verde-txt);font-weight:600">✓ Respondido em ${fmtData(d.respondido_em)} às ${fmtHora(d.respondido_em)}</span>`
                    : `<span style="color:var(--verm-txt);font-weight:700">⏳ Resposta Pendente</span>`;
                  let statusVis = d.visualizado_em
                    ? `<span style="color:var(--tx2);margin-left:6px">(Visualizado em ${fmtData(d.visualizado_em)} às ${fmtHora(d.visualizado_em)})</span>`
                    : `<span style="color:var(--tx3);margin-left:6px">(Não visualizado)</span>`;
                  return `<div>• <b>${esc(nome)}</b> - <small>${esc(pTxt)}${esc(func)}</small>: ${statusResp} ${statusVis}</div>`;
                }).join('')}
              </div>
            </div>
          `;
        }
      }

      function gerarHtmlAnexos(anexos, prefixo) {
        if (!anexos || anexos.length === 0) return '';
        return `
          <div style="margin-top:10px;padding-top:8px;border-top:1px dashed var(--borda)">
            <b style="font-size:11.5px;color:var(--tx2);display:block;margin-bottom:6px">ANEXOS (${anexos.length}):</b>
            <div style="display:flex;gap:6px;flex-wrap:wrap">
              ${anexos.map((anx, idx) => `
                <a class="msg-anexo-item ${prefixo}-anexo-link" data-idx="${idx}" download="${esc(anx.nome || 'anexo')}" style="cursor:pointer">
                  📎 ${esc(anx.nome || 'arquivo')} <small style="opacity:0.7">(${formatarTamanho(anx.tamanho)})</small>
                </a>
              `).join('')}
            </div>
          </div>
        `;
      }

      // Timeline de Respostas da Thread
      const respostas = thread.respostas || [];
      const respostasHtml = respostas.map((r, rIdx) => {
        const rRem = r.remetente || {};
        const rNome = formatarNomeRemetente(rRem.nome_completo, rRem.nome_guerra);
        const rFuncao = rRem.funcao_nome ? ` — ${rRem.funcao_nome}` : '';
        const rPapel = (rRem.papel || '').toUpperCase() + (rRem.grupo_nome ? ' — ' + rRem.grupo_nome : '');

        // Identifica se quem postou é destinatário do despacho
        const destMatch = (thread.destinatarios || []).find(d => d.papel_id === rRem.papel_id);
        const ehRespFormal = ehDespacho && !!destMatch;

        return `
          <div class="msg-thread-post ${ehRespFormal ? 'resposta-formal' : ''}">
            <div class="msg-thread-post-topo">
              <div class="msg-thread-post-autor">
                <b>${esc(rNome)}</b>
                <span class="destaque-funcao" style="margin-left:6px;font-size:12px">${esc(rFuncao)}</span>
                <span style="margin-left:6px;font-size:11px;color:var(--tx3)">(${esc(rPapel)})</span>
                ${ehRespFormal ? '<span class="badge-despacho-ok" style="margin-left:8px;font-size:11px;padding:1px 6px">⚖️ RESPOSTA FORMAL AO DESPACHO</span>' : '<span class="badge-tag" style="margin-left:8px;font-size:11px;padding:1px 6px">💬 ACOMPANHAMENTO</span>'}
              </div>
              <div class="msg-thread-post-data">
                ${fmtData(r.criada_em)} às ${fmtHora(r.criada_em)}
              </div>
            </div>
            <div class="msg-thread-post-corpo" style="word-break:break-word;line-height:1.6">${r.corpo}</div>
            ${gerarHtmlAnexos(r.anexos, `r-${rIdx}`)}
          </div>
        `;
      }).join('');

      // Opções de Mover Pasta
      const opcoesPastas = (pastasUsuario || []).map(p => `
        <option value="${p.id}" ${m.pasta_id === p.id ? 'selected' : ''}>${esc(p.nome)}</option>
      `).join('');

      const html = `
        <div class="modal" style="max-width:760px;width:95%;max-height:92vh;display:flex;flex-direction:column">
          ${bannerDespacho}

          <div class="msg-view-cabecalho" style="margin-bottom:12px">
            <div class="msg-view-tit" style="font-size:18px">${esc(msg.assunto)}</div>
            <div style="font-size:12px;color:var(--tx3);margin-top:2px">
              Thread de Comunicação Institucional #${msg.id} &bull; ${ehDespacho ? '⚖️ Despacho com Auditoria' : '✉️ Mensagem Comum'}
            </div>
          </div>

          <!-- Container da Thread com Scroll -->
          <div class="msg-thread-container" id="threadScrollContainer" style="overflow-y:auto;flex:1;max-height:48vh;padding-right:6px;display:flex;flex-direction:column;gap:12px">
            <!-- Post Raiz -->
            <div class="msg-thread-post raiz">
              <div class="msg-thread-post-topo">
                <div class="msg-thread-post-autor">
                  <b>${nomeRem}</b>
                  <span class="destaque-funcao" style="margin-left:6px;font-size:12px">${esc(funcaoRem)}</span>
                  <span style="margin-left:6px;font-size:11px;color:var(--tx3)">(${esc(papelRem)})</span>
                </div>
                <div class="msg-thread-post-data">
                  Post original &bull; ${fmtData(msg.criada_em)} às ${fmtHora(msg.criada_em)}
                </div>
              </div>
              <div class="msg-thread-post-corpo" style="word-break:break-word;line-height:1.6">${msg.corpo}</div>
              ${gerarHtmlAnexos(msg.anexos, 'raiz')}
            </div>

            <!-- Respostas Anteriores -->
            ${respostasHtml}
            <div id="threadFimScroll"></div>
          </div>

          <!-- Box de Nova Resposta / Post na Thread -->
          <div class="msg-thread-box-resposta" id="boxRespostaThread" style="margin-top:12px">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">
              <label style="font-weight:700;font-size:12.5px;display:flex;align-items:center;gap:6px">
                ${thread.minha_resposta_pendente 
                  ? '<span style="color:var(--verm-txt)">⚖️ Sua Resposta Formal ao Despacho (Obrigatória):</span>' 
                  : '<span>💬 Adicionar Resposta / Acompanhamento na Thread:</span>'}
              </label>
              <div style="display:flex;gap:6px">
                <button type="button" class="acao-linha" id="threadBtAnexoDrive" style="cursor:pointer;font-size:11.5px;padding:2px 8px">🗂️ Do Drive</button>
                <label class="acao-linha" style="cursor:pointer;font-size:11.5px;padding:2px 8px">
                  📎 Do computador
                  <input type="file" id="threadInputAnexo" multiple style="display:none">
                </label>
              </div>
            </div>
            <div id="threadNovoPostEditor"></div>
            <div id="threadListaAnexos" style="display:flex;gap:6px;flex-wrap:wrap;margin-top:4px"></div>
            <div style="display:flex;justify-content:flex-end;margin-top:8px">
              <button type="button" class="primario" id="btEnviarRespostaThread" style="padding:6px 18px">
                ${thread.minha_resposta_pendente ? '⚖️ Enviar Resposta Formal ao Despacho' : '💬 Postar Acompanhamento'}
              </button>
            </div>
          </div>

          <!-- Ações Inferiores -->
          <div class="modal-acoes" style="justify-content:space-between;border-top:1px solid var(--borda);padding-top:12px;margin-top:12px;flex-wrap:wrap;gap:10px">
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
              ` : ''}
              <button type="button" class="fantasma" id="btFecharMsgView">Fechar</button>
            </div>
          </div>
        </div>
      `;

      const mModal = modal(html);
      const editorThread = window.EditorRico ? window.EditorRico.init(mModal.querySelector('#threadNovoPostEditor'), { placeholder: thread.minha_resposta_pendente ? 'Digite aqui sua resposta formal ao despacho…' : 'Escreva um acompanhamento…' }) : null;

      // Eventos de Anexos do Post Raiz
      if (msg.anexos && msg.anexos.length > 0) {
        mModal.querySelectorAll('.raiz-anexo-link').forEach(link => {
          link.onclick = () => {
            const idx = +link.dataset.idx;
            const anx = msg.anexos[idx];
            if (anx && anx.dados_base64) link.href = anx.dados_base64;
            else if (anx && anx.drive_arquivo_id) link.href = '/api/drive/download/' + anx.drive_arquivo_id;
          };
        });
      }

      // Eventos de Anexos das Respostas
      respostas.forEach((r, rIdx) => {
        if (r.anexos && r.anexos.length > 0) {
          mModal.querySelectorAll(`.r-${rIdx}-anexo-link`).forEach(link => {
            link.onclick = () => {
              const idx = +link.dataset.idx;
              const anx = r.anexos[idx];
              if (anx && anx.dados_base64) link.href = anx.dados_base64;
              else if (anx && anx.drive_arquivo_id) link.href = '/api/drive/download/' + anx.drive_arquivo_id;
            };
          });
        }
      });

      // Rolar para baixo se tiver respostas
      const scroller = mModal.querySelector('#threadScrollContainer');
      if (scroller && respostas.length > 0) {
        scroller.scrollTop = scroller.scrollHeight;
      }

      // Botão Rolar para Resposta
      const btnRolar = mModal.querySelector('#btRolarParaResposta');
      if (btnRolar) {
        btnRolar.onclick = () => {
          const txtArea = mModal.querySelector('#threadNovoPostCorpo');
          if (txtArea) {
            txtArea.scrollIntoView({ behavior: 'smooth' });
            txtArea.focus();
          }
        };
      }

      // Gestão de Anexos da Nova Resposta
      let anexosResposta = [];
      const contAnxResp = mModal.querySelector('#threadListaAnexos');
      const inputAnxResp = mModal.querySelector('#threadInputAnexo');

      function renderAnexosResposta() {
        contAnxResp.innerHTML = anexosResposta.map((a, i) => `
          <span class="msg-anexo-item" style="font-size:11px">
            ${a.drive_arquivo_id ? '🗂️' : '📄'} ${esc(a.nome)} <small>(${formatarTamanho(a.tamanho)})</small>
            <span data-remanx="${i}" style="cursor:pointer;font-weight:bold;margin-left:4px;color:var(--verm-txt)">&times;</span>
          </span>
        `).join('');
        contAnxResp.querySelectorAll('[data-remanx]').forEach(b => {
          b.onclick = () => {
            anexosResposta.splice(+b.dataset.remanx, 1);
            renderAnexosResposta();
          };
        });
      }

      const btAnxDriveResp = mModal.querySelector('#threadBtAnexoDrive');
      if (btAnxDriveResp) {
        btAnxDriveResp.onclick = () => {
          window.abrirSeletorDrive({
            jaSelecionados: anexosResposta,
            onConfirma: (escolhidos) => {
              anexosResposta = escolhidos;
              renderAnexosResposta();
            }
          });
        };
      }

      if (inputAnxResp) {
        inputAnxResp.onchange = async (e) => {
          const files = Array.from(e.target.files || []);
          for (const file of files) {
            if (file.size > 25 * 1024 * 1024) {
              toast(`Arquivo ${file.name} excede o limite de 25MB`, 'erro');
              continue;
            }
            try {
              const ref = await window.enviarArquivoParaDrive(file);
              anexosResposta.push(ref);
            } catch (err) {
              toast(`Falha ao enviar ${file.name} ao drive`, 'erro');
            }
          }
          e.target.value = '';
          renderAnexosResposta();
        };
      }

      // Enviar Post / Resposta na Thread
      const btnEnviarResp = mModal.querySelector('#btEnviarRespostaThread');
      if (btnEnviarResp) {
        btnEnviarResp.onclick = async () => {
          const corpo = editorThread ? editorThread.getHTML() : '';
          if (!corpo || corpo === '<p><br></p>') {
            toast('Por favor, digite uma mensagem para enviar na thread', 'erro');
            return;
          }
          btnEnviarResp.disabled = true;
          try {
            const resp = await api(`/api/mensagens/${m.id}/responder`, {
              method: 'POST',
              body: JSON.stringify({
                corpo: corpo,
                anexos: anexosResposta
              })
            });
            toast(resp.despacho_atendido ? 'Resposta formal ao despacho registrada com sucesso!' : 'Acompanhamento postado na thread!');
            atualizarBadges();
            mModal.remove();
            abrirModalVisualizar(m, modo);
            alternarAba(abaAtual);
          } catch (e) {
            btnEnviarResp.disabled = false;
          }
        };
      }

      // Encaminhar
      const btnEnc = mModal.querySelector('#btEncaminharMsg');
      if (btnEnc) {
        btnEnc.onclick = () => {
          mModal.remove();
          abrirModalCompor({
            assunto: msg.assunto.startsWith('Enc: ') ? msg.assunto : 'Enc: ' + msg.assunto,
            corpo: `\n\n--- Mensagem Encaminhada ---\nDe: ${nomeRem} (${papelRem})\nData: ${fmtData(msg.criada_em)}\n\n${msg.corpo}`,
            anexos: msg.anexos || []
          });
        };
      }

      // FINALIZAR despacho (ordem 04/10): destinatário encerra SEM responder;
      // vira mensagem comum (sai das pendências, entra na caixa convencional).
      const btnFin = mModal.querySelector('#btFinalizarDespacho');
      if (btnFin) {
        btnFin.onclick = async () => {
          if (!confirm('Finalizar este despacho sem resposta formal? Ele vira mensagem comum e não exige mais retorno.')) return;
          try {
            await api(`/api/mensagens/${m.id}/finalizar`, { method: 'POST' });
            toast('Despacho finalizado. Registrado como mensagem comum.');
            mModal.remove();
            alternarAba(abaAtual);
          } catch (e) {}
        };
      }

      // Arquivar / Desarquivar
      const btnArq = mModal.querySelector('#btArquivarMsg');
      if (btnArq) {
        btnArq.onclick = async () => {
          if (thread.minha_resposta_pendente) {
            return toast('Despacho com resposta pendente. É obrigatório responder formalmente na thread antes de arquivar.', 'erro');
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
          if (thread.minha_resposta_pendente) {
            return toast('Despacho com resposta pendente. É obrigatório responder formalmente na thread antes de excluir.', 'erro');
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

      mModal.querySelector('#btFecharMsgView').onclick = () => mModal.remove();
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

      // Conjunto de destinatários selecionados (CC)
      let selecionadosIds = new Set();
      if (dadosIniciais.destinatario_id) {
        selecionadosIds.add(+dadosIniciais.destinatario_id);
      }

      let anexosCarregados = (dadosIniciais.anexos && Array.isArray(dadosIniciais.anexos)) ? [...dadosIniciais.anexos] : [];

      const opcoesHtml = dests.map(d => {
        const funcao = d.funcao_nome ? ` — ${d.funcao_nome}` : '';
        const grupo = d.grupo_nome ? ` (${d.grupo_nome})` : ' (Global)';
        const pessoa = d.nome_guerra ? ` [${d.nome_guerra}]` : '';
        return `<option value="${d.id}">${esc(d.papel.toUpperCase())}${esc(grupo)}${esc(funcao)}${esc(pessoa)}</option>`;
      }).join('');

      const html = `
        <div class="modal" style="max-width:760px;width:95%">
          <h3 style="margin-top:0;display:flex;align-items:center;gap:8px">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
            ${dadosIniciais.tipo === 'despacho' ? 'Compor Despacho' : 'Nova Mensagem'}
          </h3>

          <div class="campo" style="margin-bottom:12px">
            <div id="mTipoDesc" style="font-size:12px;color:var(--tx2)">
              ${dadosIniciais.tipo === 'despacho'
                ? '⚖️ <b style="color:var(--verm-txt)">Despacho Formal</b> — emitido a partir da aba Despachos. Exige manifestação formal do destinatário (ou finalização por ele).'
                : '✉️ <b style="color:var(--tx1)">Mensagem Convencional</b> — emitida a partir do grupo Convencional, sem trava de resposta.'}
            </div>
          </div>

          <!-- Destinatários & CC -->
          <div class="campo" style="margin-bottom:12px">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;flex-wrap:wrap;gap:6px">
              <label style="font-weight:700">Destinatários & Encaminhamento (CC):</label>
              <div style="display:flex;gap:4px;flex-wrap:wrap">
                <button type="button" class="acao-linha" id="btListGerentes" style="font-size:11px;padding:2px 6px">👥 Todos Gerentes</button>
                <button type="button" class="acao-linha" id="btListOperadores" style="font-size:11px;padding:2px 6px">👷 Todos Operadores</button>
                <button type="button" class="acao-linha" id="btListUnidade" style="font-size:11px;padding:2px 6px">🏢 Minha Unidade</button>
                <button type="button" class="acao-linha" id="btListChefes" style="font-size:11px;padding:2px 6px">⭐ Chefes de Setor</button>
                <button type="button" class="acao-linha" id="btLimparDests" style="font-size:11px;padding:2px 6px;color:var(--verm-txt)">Limpar</button>
              </div>
            </div>

            <!-- Chips / Pills de Selecionados -->
            <div id="mPillsDests" style="display:flex;flex-wrap:wrap;gap:6px;min-height:36px;padding:6px;background:var(--painel2);border:1px solid var(--borda2);border-radius:var(--raio);margin-bottom:6px"></div>

            <div style="display:flex;gap:6px">
              <select id="mSelectDest" style="flex:1;height:36px">
                <option value="">+ Adicionar destinatário individual…</option>
                ${opcoesHtml}
              </select>
              <button type="button" class="acao-linha" id="mBtnAddDest" style="padding:0 12px;font-size:12px">+ Incluir</button>
            </div>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700">Assunto</label>
            <input type="text" id="mAssunto" value="${esc(dadosIniciais.assunto || '')}" placeholder="ex.: Diretriz, Convocação, Relatório…" style="width:100%">
          </div>

          <!-- Barra de Ferramentas / Rich Text -->
          <div class="campo" style="margin-bottom:12px">
            <label style="font-weight:700;margin-bottom:4px;display:block">Mensagem / Despacho (Editor Formatado)</label>
            <div id="mEditorContainer"></div>
          </div>

          <!-- Anexos (ordem 04/10): do DRIVE (referência, sem duplicar) ou do PC (sobe p/ o drive e referencia) -->
          <div style="margin-bottom:16px">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;gap:8px;flex-wrap:wrap">
              <label style="font-weight:700;font-size:12.5px">Anexos de Documentos:</label>
              <div style="display:flex;gap:6px">
                <button type="button" class="acao-linha" id="mBtAnexoDrive" style="cursor:pointer;font-size:12px;padding:3px 8px">🗂️ Do Drive</button>
                <label class="acao-linha" style="cursor:pointer;font-size:12px;padding:3px 8px">
                  💻 Do computador
                  <input type="file" id="mInputAnexo" multiple style="display:none">
                </label>
              </div>
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

      // Instancia Rich Editor
      let editorInst = null;
      if (window.EditorRico) {
        editorInst = window.EditorRico.init(q('#mEditorContainer'), { placeholder: 'Digite aqui o teor da mensagem institucional ou despacho…', valorInicial: dadosIniciais.corpo || '' });
      } else {
        q('#mEditorContainer').innerHTML = `<textarea id="mCorpoFallback" rows="6" style="width:100%;resize:vertical">${esc(dadosIniciais.corpo || '')}</textarea>`;
      }

      function renderDestsPills() {
        const pCont = q('#mPillsDests');
        if (selecionadosIds.size === 0) {
          pCont.innerHTML = '<span style="font-size:12px;color:var(--tx3);line-height:24px">Nenhum destinatário selecionado ainda.</span>';
          return;
        }

        pCont.innerHTML = Array.from(selecionadosIds).map(id => {
          const d = dests.find(x => x.id === id);
          if (!d) return '';
          const funcao = d.funcao_nome ? ` · ${d.funcao_nome}` : '';
          const pessoa = d.nome_guerra ? ` [${d.nome_guerra}]` : '';
          return `
            <span class="dest-pill">
              <span><b>${esc(d.papel.toUpperCase())}</b>${esc(funcao)}${esc(pessoa)}</span>
              <span class="dest-pill-remove" data-remdest="${id}" title="Remover">&times;</span>
            </span>
          `;
        }).join('');

        pCont.querySelectorAll('[data-remdest]').forEach(b => {
          b.onclick = () => {
            selecionadosIds.delete(+b.dataset.remdest);
            renderDestsPills();
          };
        });
      }
      renderDestsPills();

      q('#mBtnAddDest').onclick = () => {
        const val = +q('#mSelectDest').value;
        if (val) {
          selecionadosIds.add(val);
          renderDestsPills();
        }
      };
      q('#mSelectDest').onchange = () => {
        const val = +q('#mSelectDest').value;
        if (val) {
          selecionadosIds.add(val);
          renderDestsPills();
          q('#mSelectDest').value = '';
        }
      };

      // Atalhos de Listas Predefinidas
      q('#btListGerentes').onclick = () => {
        dests.filter(d => d.papel === 'gerente').forEach(d => selecionadosIds.add(d.id));
        renderDestsPills();
      };
      q('#btListOperadores').onclick = () => {
        dests.filter(d => d.papel === 'operador').forEach(d => selecionadosIds.add(d.id));
        renderDestsPills();
      };
      q('#btListUnidade').onclick = () => {
        dests.filter(d => d.grupo_id && d.grupo_id === u.grupo_id).forEach(d => selecionadosIds.add(d.id));
        renderDestsPills();
      };
      q('#btListChefes').onclick = () => {
        dests.filter(d => d.funcao_nome && /chefe|comandante|diretor/i.test(d.funcao_nome)).forEach(d => selecionadosIds.add(d.id));
        renderDestsPills();
      };
      q('#btLimparDests').onclick = () => {
        selecionadosIds.clear();
        renderDestsPills();
      };

      function renderAnexosModal() {
        const contAnx = q('#mListaAnexos');
        contAnx.innerHTML = anexosCarregados.map((a, i) => `
          <span class="msg-anexo-item" style="font-size:11.5px">
            ${a.drive_arquivo_id ? '🗂️' : '📄'} ${esc(a.nome)} <small>(${formatarTamanho(a.tamanho)})</small>
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

      // Anexos DO DRIVE (referência) e DO PC (cópia sobe p/ o drive — ordem 04/10)
      q('#mBtAnexoDrive').onclick = () => {
        window.abrirSeletorDrive({
          jaSelecionados: anexosCarregados,
          onConfirma: (escolhidos) => {
            anexosCarregados = escolhidos;
            renderAnexosModal();
          }
        });
      };

      q('#mInputAnexo').onchange = async (e) => {
        const files = Array.from(e.target.files || []);
        for (const file of files) {
          if (file.size > 25 * 1024 * 1024) {
            toast(`Arquivo ${file.name} excede o limite de 25MB para mensagens`, 'erro');
            continue;
          }
          try {
            const ref = await window.enviarArquivoParaDrive(file);
            anexosCarregados.push(ref);
          } catch (err) {
            toast(`Falha ao enviar ${file.name} ao drive`, 'erro');
          }
        }
        e.target.value = '';
        renderAnexosModal();
      };

      q('#mBtnCancel').onclick = () => mModal.remove();

      q('#mBtnEnviar').onclick = async () => {
        const destArray = Array.from(selecionadosIds);
        const assunto = q('#mAssunto').value.trim();
        const corpo = editorInst ? (typeof editorInst.getHTML === 'function' ? editorInst.getHTML().trim() : (typeof editorInst.getValue === 'function' ? editorInst.getValue().trim() : '')) : (q('#mCorpoFallback') ? q('#mCorpoFallback').value.trim() : '');
        const tipo = (GRUPO_DE[abaAtual] === 'despachos' || dadosIniciais.tipo === 'despacho') ? 'despacho' : 'comum';

        if (destArray.length === 0) { toast('Selecione pelo menos um destinatário (ou CC)', 'erro'); return; }
        if (!assunto) { toast('Informe o assunto', 'erro'); return; }
        if (!corpo || corpo === '<p><br></p>') { toast('Escreva a mensagem', 'erro'); return; }

        q('#mBtnEnviar').disabled = true;
        try {
          await api('/api/mensagens', {
            method: 'POST',
            body: JSON.stringify({
              destinatario_papel_ids: destArray,
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
      const txt = totalPendencias === 1 ? '1 pendência no Email' : `${totalPendencias} mensagens/despachos pendentes`;
      // Notificação de AVISOS do grupo (ordem Diretor 07/10): aviso publicado =
      // todo usuário recebe notificação. Fonte: /api/notificacoes (avisos_pendentes,
      // já existente) somada ao badge do sino — sem duplicar o que já está no inbox.
      let avisos = 0;
      try {
        const nres = await api('/api/notificacoes');
        avisos = (nres && nres.avisos_pendentes) || 0;
      } catch (e2) { /* silencioso: badge fica só com mensagens */ }
      const elAv1 = document.getElementById('sinoHoverAvisosTxt');
      if (elAv1) elAv1.textContent = avisos === 1 ? '1 novo comunicado' : `${avisos} novos comunicados`;
      const elAv2 = document.getElementById('mobSinoHoverAvisosTxt');
      if (elAv2) elAv2.textContent = avisos === 1 ? '1 novo comunicado' : `${avisos} novos comunicados`;
      const badgeAvS = document.getElementById('sbBadgeAvisos');
      const badgeAvM = document.getElementById('mobBadgeAvisos');
      [badgeAvS, badgeAvM].forEach(b => {
        if (!b) return;
        b.textContent = String(avisos);
        if (avisos > 0) b.classList.remove('oculto');
        else b.classList.add('oculto');
      });
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
