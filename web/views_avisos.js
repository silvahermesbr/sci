/* =====================================================================
   VIEWS_AVISOS.JS — Fórum & Mural de Avisos Institucional
   Mural Corporativo, Registro de Ciente Obrigatório e Discussão
   ===================================================================== */
(function () {
  'use strict';

  function quem() {
    return (typeof ME !== 'undefined' && ME) || window.ME || null;
  }

  function esc(s) {
    if (!s) return '';
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function fmtData(iso) {
    if (!iso) return '—';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleDateString('pt-BR');
  }

  function fmtHora(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
  }

  function formatarNomeRemetente(completo, guerra) {
    const c = (completo || '').trim();
    const g = (guerra || '').trim();
    if (!c && !g) return '—';
    if (!c) return `<b>${esc(g)}</b>`;
    if (!g) return esc(c);
    const idx = c.toLowerCase().indexOf(g.toLowerCase());
    if (idx !== -1) {
      return `${esc(c.substring(0, idx))}<b>${esc(c.substring(idx, idx + g.length))}</b>${esc(c.substring(idx + g.length))}`;
    }
    return `${esc(c)} (<b>${esc(g)}</b>)`;
  }

  window.ViewAvisos = async function () {
    const u = quem();
    if (!u) { location.hash = '#/login'; return; }
    if (window.navAtiva) navAtiva('#/avisos');

    const app = document.getElementById('app');
    if (!app) return;

    const podePublicar = (u.papel === 'admin' || u.papel === 'gerente');

    app.innerHTML = `
      <div class="msg-topo-wrapper">
        <div class="msg-topo-info">
          <h2 style="margin:0 0 4px;display:flex;align-items:center;gap:8px">
            <span>📢</span> <span>Fórum & Mural de Avisos</span>
          </h2>
          <p style="color:var(--tx2);font-size:13px;margin:0">
            Comunicados institucionais da cadeia de comando com auditoria de ciente e manifestação.
          </p>
        </div>
        <div class="msg-topo-acoes">
          ${podePublicar ? `
            <button type="button" class="primario" id="btNovoAviso" style="display:flex;align-items:center;gap:6px">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
              Publicar Aviso
            </button>
          ` : ''}
        </div>
      </div>

      <div id="muralCorpo" style="margin-top:16px">
        <div class="carregando">Carregando avisos do mural…</div>
      </div>
    `;

    const cont = document.getElementById('muralCorpo');

    async function carregarAvisos() {
      if (!cont) return;
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

      const ehGerente = (u.papel === 'gerente');

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

        // Opção de Repostar
        let btnRepostar = '';
        if (ehGerente && a.grupo_id !== u.grupo_id) {
          btnRepostar = `<button type="button" class="acao-linha" data-repostar="${a.id}" style="font-size:12px;padding:3px 8px">🔄 Repostar p/ meu Grupo</button>`;
        }

        // Excluir (autor, admin ou gerente do grupo)
        let btnExcluir = '';
        if (u.papel === 'admin' || (autor.usuario_id === u.id) || (ehGerente && a.grupo_id === u.grupo_id)) {
          btnExcluir = `<button type="button" class="acao-linha" data-delaviso="${a.id}" style="font-size:12px;color:var(--verm-txt);padding:3px 8px">Excluir</button>`;
        }

        return `
          <div class="aviso-card ${a.fixado ? 'fixado' : ''}" id="avisoCard_${a.id}" data-veraviso="${a.id}" style="margin-bottom:14px;cursor:pointer">
            <div class="aviso-card-topo">
              <div>
                <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;flex-wrap:wrap">
                  ${badgeFixado}
                  ${grupoOrig}
                  <span style="font-size:12px;color:var(--tx3)">Publicado em <b>${esc(a.grupo_nome)}</b> · ${fmtData(a.criado_em)} ${fmtHora(a.criado_em)}</span>
                </div>
                <h4 class="aviso-card-titulo">${esc(a.titulo)}</h4>
                <div style="font-size:12.5px;color:var(--tx2)">Por: ${nomeAutor} (${esc(autor.papel ? autor.papel.toUpperCase() : 'COMANDO')})</div>
              </div>
              <div style="display:flex;align-items:center;gap:6px">
                ${btnRepostar}
                ${btnExcluir}
              </div>
            </div>

            <div class="aviso-card-rodape" style="margin-top:8px">
              <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">
                ${btnCiente}
                <span style="font-size:12px;color:var(--tx3)">👥 ${a.total_cientes || 0} cientes · 💬 ${a.total_comentarios || 0} comentários</span>
              </div>
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
            toast('Ciente formal registrado com sucesso!');
            carregarAvisos();
          } catch (e) {
            toast(e.message || 'Falha ao registrar ciente', 'erro');
          }
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

      // Ver Cientes
      cont.querySelectorAll('[data-vercientes]').forEach(b => {
        b.onclick = async () => {
          const id = +b.dataset.vercientes;
          try {
            const detalhe = await api(`/api/avisos/${id}/detalhes`);
            const cientes = detalhe.cientes || [];
            const htmlCientes = `
              <div class="modal" style="max-width:520px;width:95%">
                <h3 style="margin-top:0">Auditoria de Ciente Formal</h3>
                <p style="color:var(--tx2);font-size:12.5px;margin-bottom:12px">Militares que registraram confirmação de leitura deste comunicado:</p>
                <div style="max-height:300px;overflow-y:auto;border:1px solid var(--borda);border-radius:6px;padding:8px">
                  ${cientes.length === 0 ? '<div class="vazio" style="padding:16px;text-align:center">Nenhum militar registrou ciente ainda.</div>' : `
                    <table style="width:100%;font-size:12px">
                      <thead><tr><th>Militar / Conta</th><th>Função / Grupo</th><th>Data & Hora</th></tr></thead>
                      <tbody>
                        ${cientes.map(c => `
                          <tr>
                            <td><b>${esc(c.nome_guerra || c.login)}</b></td>
                            <td>${esc(c.funcao_nome || c.papel)} (${esc(c.grupo_nome || '—')})</td>
                            <td>${fmtData(c.registrado_em)} ${fmtHora(c.registrado_em)}</td>
                          </tr>
                        `).join('')}
                      </tbody>
                    </table>
                  `}
                </div>
                <div class="modal-acoes" style="margin-top:14px">
                  <button type="button" class="primario" onclick="this.closest('.modal-mask').remove()">Fechar</button>
                </div>
              </div>
            `;
            modal(htmlCientes);
          } catch (e) {
            toast('Erro ao buscar cientes do aviso', 'erro');
          }
        };
      });

      // Toggle Comentários
      // Onda UX 0510 (item 3): clique no card abre modal de LEITURA (padrão Email Interno) —
      // conteúdo completo + ciente AUTOMÁTICO na abertura (fallback: botão manual) + discussão.
      cont.querySelectorAll('[data-veraviso]').forEach(card => {
        card.addEventListener('click', async ev => {
          if (ev.target.closest('button') || ev.target.closest('[data-veraviso] button')) return;
          const id = +card.dataset.veraviso;
          const a = (avisos || []).find(x => x.id === id);
          if (!a) return;
          const autor = a.autor || {};

          // Ciente automático na abertura (se pendente); botão manual permanece como fallback.
          if (!a.meu_ciente) {
            api(`/api/avisos/${id}/ciente`, { method: 'POST' })
              .then(() => { a.meu_ciente = true; toast('Ciente registrado na leitura.'); })
              .catch(() => {});
          }

          const html = `
            <div class="modal" style="max-width:760px;width:95%">
              <h3 style="margin-top:0">${a.fixado ? '📌 ' : ''}${esc(a.titulo)}</h3>
              <div style="font-size:12px;color:var(--tx3);margin-bottom:12px">
                Publicado em <b>${esc(a.grupo_nome)}</b> · ${fmtData(a.criado_em)} ${fmtHora(a.criado_em)} ·
                Por: ${esc(autor.nome_guerra || autor.login || 'Comando')}
              </div>
              <div style="word-break:break-word;line-height:1.65;max-height:40vh;overflow-y:auto;border:1px solid var(--borda);border-radius:8px;padding:14px;background:var(--painel2)">${a.conteudo}</div>
              <div id="avModCiente_${id}" style="margin-top:12px;display:flex;align-items:center;gap:10px">
                ${a.meu_ciente
                  ? '<span class="badge-despacho-ok" style="padding:4px 10px">✓ Ciente Registrado</span>'
                  : '<button type="button" class="primario" id="btCienteMod_' + id + '" style="padding:5px 14px;font-size:12px">✋ Dar Ciente</button><span style="font-size:11.5px;color:var(--tx3)">registrando automaticamente…</span>'}
                <button type="button" class="acao-linha" id="btCientesMod_${id}" style="font-size:12px;padding:3px 8px">👥 Cientes (${a.total_cientes || 0})</button>
              </div>
              <div style="margin-top:14px;border-top:1px solid var(--borda);padding-top:10px">
                <div id="boxCommMod_${id}"><div class="carregando">Carregando discussão…</div></div>
              </div>
              <div class="modal-acoes">
                <button type="button" class="acao-linha" onclick="this.closest('.modal-mask').remove()">Fechar</button>
              </div>
            </div>
          `;
          const m = window.abrirModal(html, null, { largura: '760px' });

          // Fallback manual de ciente (caso o automático falhe)
          const btC = m.modal.querySelector(`#btCienteMod_${id}`);
          if (btC) btC.onclick = async () => {
            try {
              await api(`/api/avisos/${id}/ciente`, { method: 'POST' });
              a.meu_ciente = true;
              btC.outerHTML = '<span class="badge-despacho-ok" style="padding:4px 10px">✓ Ciente Registrado</span>';
              toast('Ciente formal registrado!');
            } catch (e) { toast(e.message || 'Falha ao registrar ciente', 'erro'); }
          };

          // Auditoria de cientes dentro do modal
          m.modal.querySelector(`#btCientesMod_${id}`).onclick = async () => {
            try {
              const d = await api(`/api/avisos/${id}/detalhes`);
              const cs = d.cientes || [];
              toast(cs.length ? `${cs.length} ciente(s) registrado(s).` : 'Nenhum ciente registrado ainda.');
            } catch (e) { toast('Erro ao buscar cientes', 'erro'); }
          };

          // Discussão/comentários no pé do modal (reusa o render extraído)
          const boxMod = m.modal.querySelector(`#boxCommMod_${id}`);
          try {
            const d = await api(`/api/avisos/${id}/detalhes`);
            const comentarios = d.comentarios || [];
            boxMod.innerHTML = `
              <div style="font-weight:700;font-size:13px;margin-bottom:8px">Discussão & Manifestações (${comentarios.length}):</div>
              <div style="display:flex;flex-direction:column;gap:8px;max-height:220px;overflow-y:auto;margin-bottom:12px">
                ${comentarios.length === 0 ? '<div style="font-size:12px;color:var(--tx3)">Nenhum comentário registrado.</div>' : comentarios.map(c => `
                  <div style="background:var(--painel3);border-radius:6px;padding:8px 10px;font-size:12.5px;border:1px solid var(--borda)">
                    <div style="display:flex;justify-content:space-between;margin-bottom:4px;font-size:11.5px;color:var(--tx2)">
                      <span><b>${esc(c.nome_guerra || c.login)}</b> (${esc(c.funcao_nome || c.papel || '')})</span>
                      <span>${fmtData(c.criado_em)} ${fmtHora(c.criado_em)}</span>
                    </div>
                    <div style="color:var(--tx);line-height:1.4">${c.texto || c.comentario || ''}</div>
                  </div>
                `).join('')}
              </div>
              <textarea id="txtCommMod_${id}" placeholder="Escreva seu comentário…" style="width:100%;min-height:60px;background:var(--painel2);border:1px solid var(--borda);border-radius:6px;color:var(--tx);font-size:12.5px;padding:8px"></textarea>
              <div style="display:flex;justify-content:flex-end;margin-top:6px">
                <button type="button" class="primario" id="btEnvCommMod_${id}" style="padding:5px 14px;font-size:12px">Enviar Comentário</button>
              </div>
            `;
            m.modal.querySelector(`#btEnvCommMod_${id}`).onclick = async () => {
              const texto = m.modal.querySelector(`#txtCommMod_${id}`).value.trim();
              if (!texto) { toast('Digite o comentário', 'erro'); return; }
              try {
                await api(`/api/avisos/${id}/comentar`, { method: 'POST', body: JSON.stringify({ texto }) });
                toast('Comentário registrado!');
                m.fechar();
                carregarAvisos();
              } catch (e) { toast(e.message || 'Falha ao comentar', 'erro'); }
            };
          } catch (e) {
            boxMod.innerHTML = '<div class="vazio">Erro ao carregar comentários.</div>';
          }
        });
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
          box.innerHTML = '<div class="carregando">Carregando comentários…</div>';

          // Onda UX 0510 (item 3): renderização de comentários EXTRAÍDA p/ escopo da lista —
          // reutilizada pelo modal de leitura do aviso (abrirAvisoModal).
          async function recarregarComentarios() {
            try {
              const detalhe = await api(`/api/avisos/${id}/detalhes`);
              const comentarios = detalhe.comentarios || [];

              box.innerHTML = `
                <div style="font-weight:700;font-size:13px;margin-bottom:8px">Discussão & Manifestações (${comentarios.length}):</div>
                <div style="display:flex;flex-direction:column;gap:8px;max-height:260px;overflow-y:auto;margin-bottom:12px">
                  ${comentarios.length === 0 ? '<div style="font-size:12px;color:var(--tx3)">Nenhum comentário registrado.</div>' : comentarios.map(c => `
                    <div style="background:var(--painel3);border-radius:6px;padding:8px 10px;font-size:12.5px;border:1px solid var(--borda)">
                      <div style="display:flex;justify-content:space-between;margin-bottom:4px;font-size:11.5px;color:var(--tx2)">
                        <span><b>${esc(c.nome_guerra || c.login)}</b> (${esc(c.funcao_nome || c.papel || '')})</span>
                        <span>${fmtData(c.criado_em)} ${fmtHora(c.criado_em)}</span>
                      </div>
                      <div style="color:var(--tx);line-height:1.4">${c.texto || c.comentario || ''}</div>
                      ${(c.anexos && c.anexos.length) ? `
                        <div style="display:flex;gap:6px;flex-wrap:wrap;margin-top:6px">
                          ${c.anexos.map(anx => anx.drive_arquivo_id
                            ? `<a href="/api/drive/download/${anx.drive_arquivo_id}" class="msg-anexo-item" style="font-size:11.5px">🗂️ ${esc(anx.nome || 'arquivo')} <small>(${formatarTamanhoBytes(anx.tamanho)})</small></a>`
                            : (anx.dados_base64 ? `<a href="${anx.dados_base64}" download="${esc(anx.nome || 'anexo')}" class="msg-anexo-item" style="font-size:11.5px">📄 ${esc(anx.nome || 'anexo')} <small>(${formatarTamanhoBytes(anx.tamanho)})</small></a>` : ''))}
                        </div>` : ''}
                    </div>
                  `).join('')}
                </div>
                <div id="editorComm_${id}" style="margin-bottom:8px"></div>
                <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;flex-wrap:wrap;margin-top:4px">
                  <div style="display:flex;gap:6px;align-items:center">
                    <button type="button" class="acao-linha" id="btAnxDriveComm_${id}" style="cursor:pointer;font-size:11.5px;padding:2px 8px">🗂️ Do Drive</button>
                    <label class="acao-linha" style="cursor:pointer;font-size:11.5px;padding:2px 8px">
                      📎 Do computador
                      <input type="file" id="inputAnxComm_${id}" multiple style="display:none">
                    </label>
                    <div id="listaAnxComm_${id}" style="display:flex;gap:6px;flex-wrap:wrap"></div>
                  </div>
                  <button type="button" class="primario" id="btEnvComm_${id}" style="padding:6px 14px;font-size:12px">Enviar Comentário</button>
                </div>
              `;

              const editorComm = window.montarRichEditor ? window.montarRichEditor(box.querySelector(`#editorComm_${id}`), 'Adicione um comentário...') : null;
              if (!editorComm) {
                box.querySelector(`#editorComm_${id}`).innerHTML = `<textarea id="txtComm_${id}" rows="2" style="width:100%" placeholder="Adicione um comentário..."></textarea>`;
              }

              // Anexos do comentário (ordem 04/10): drive (referência) ou PC → drive
              let anexosComm = [];
              const renderAnexosComm = () => {
                const contA = box.querySelector(`#listaAnxComm_${id}`);
                if (!contA) return;
                contA.innerHTML = anexosComm.map((a, i) => `
                  <span class="msg-anexo-item" style="font-size:11px">
                    ${a.drive_arquivo_id ? '🗂️' : '📄'} ${esc(a.nome)} <small>(${formatarTamanhoBytes(a.tamanho)})</small>
                    <span data-rm="${i}" style="cursor:pointer;font-weight:bold;margin-left:4px;color:var(--verm-txt)">&times;</span>
                  </span>`).join('');
                contA.querySelectorAll('[data-rm]').forEach(b => {
                  b.onclick = () => { anexosComm.splice(+b.dataset.rm, 1); renderAnexosComm(); };
                });
              };
              const btAnxDriveComm = box.querySelector(`#btAnxDriveComm_${id}`);
              if (btAnxDriveComm) {
                btAnxDriveComm.onclick = () => {
                  window.abrirSeletorDrive({
                    jaSelecionados: anexosComm,
                    onConfirma: (escolhidos) => { anexosComm = escolhidos; renderAnexosComm(); }
                  });
                };
              }
              const inputAnxComm = box.querySelector(`#inputAnxComm_${id}`);
              if (inputAnxComm) {
                inputAnxComm.onchange = async (e) => {
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
              }

              box.querySelector(`#btEnvComm_${id}`).onclick = async () => {
                const texto = editorComm ? editorComm.getHTML() : box.querySelector(`#txtComm_${id}`).value.trim();
                if (!texto || texto === '<p><br></p>') { toast('Digite o comentário', 'erro'); return; }
                try {
                  await api(`/api/avisos/${id}/comentar`, {
                    method: 'POST',
                    body: JSON.stringify({ texto, anexos: anexosComm })
                  });
                  toast('Comentário registrado!');
                  recarregarComentarios();
                } catch (e) {
                  toast(e.message || 'Falha ao comentar', 'erro');
                }
              };
            } catch (e) {
              box.innerHTML = '<div class="vazio">Erro ao carregar comentários.</div>';
            }
          }

          recarregarComentarios();
        };
      });
    }

    // Modal Publicar Novo Aviso
    const btNovo = document.getElementById('btNovoAviso');
    if (btNovo) {
      btNovo.onclick = async () => {
        // Onda C2 (05/10): admin é global — escolhe o GRUPO DE DESTINO do aviso.
        let optGrupos = '';
        if (u.papel === 'admin') {
          try {
            const gs = await api('/api/grupos');
            optGrupos = (gs || []).map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');
          } catch (e) {}
        }
        const html = `
          <div class="modal" style="max-width:680px;width:95%">
            <h3 style="margin-top:0">📢 Publicar Novo Aviso no Mural</h3>
            <p style="color:var(--tx2);font-size:12.5px;margin-bottom:14px">Comunicado formal visível para toda a equipe subordinada, com confirmação obrigatória de ciente.</p>

            <div class="campo" style="margin-bottom:12px">
              <label style="font-weight:700">Título do Comunicado *</label>
              <input type="text" id="avTitulo" placeholder="ex.: Diretrizes para Operação Especial, Escalas de Serviço…" style="width:100%">
            </div>
            ${u.papel === 'admin' ? `
            <div class="campo" style="margin-bottom:12px">
              <label style="font-weight:700">Grupo de Destino *</label>
              <select id="avGrupo" style="width:100%"><option value="">— selecione o grupo —</option>${optGrupos}</select>
            </div>` : ''}

            <div class="campo" style="margin-bottom:12px">
              <label style="font-weight:700">Conteúdo do Comunicado (Barra de Formatação DOCX) *</label>
              <div id="avCorpoEditor"></div>
            </div>

            <div class="campo" style="margin-bottom:16px">
              <label style="display:inline-flex;align-items:center;gap:6px;cursor:pointer">
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

        const editorAviso = window.montarRichEditor ? window.montarRichEditor(m.querySelector('#avCorpoEditor'), 'Digite as instruções e diretrizes…') : null;

        m.querySelector('#avPublicar').onclick = async () => {
          const titulo = m.querySelector('#avTitulo').value.trim();
          const conteudo = editorAviso ? editorAviso.getHTML() : '';
          const fixado = m.querySelector('#avFixado').checked;

          if (!titulo || !conteudo || conteudo === '<p><br></p>') {
            toast('Título e conteúdo são obrigatórios', 'erro');
            return;
          }
          let grupoID;
          if (u.papel === 'admin') {
            grupoID = +(m.querySelector('#avGrupo') || {}).value || 0;
            if (!grupoID) { toast('Escolha o grupo de destino', 'erro'); return; }
          }

          m.querySelector('#avPublicar').disabled = true;
          try {
            await api('/api/avisos', {
              method: 'POST',
              body: JSON.stringify({
                titulo,
                conteudo,
                fixado,
                grupo_id: grupoID
              })
            });
            toast('Aviso publicado com sucesso no mural!');
            m.remove();
            carregarAvisos();
          } catch (e) {
            m.querySelector('#avPublicar').disabled = false;
            toast(e.message || 'Falha ao publicar', 'erro');
          }
        };
      };
    }

    carregarAvisos();
  };

})();
