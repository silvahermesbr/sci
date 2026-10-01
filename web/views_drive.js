/* SCI — Módulo de Drive Local (v1.2 Fase 3)
   Armazenamento Físico de Arquivos em ./dados/drive, Pastas Hierárquicas,
   Permissões Granulares estilo Google Drive (Grupo, Papel, Usuário) e Auditoria Completa. */
(function () {
  'use strict';

  function quem() {
    return (typeof ME !== 'undefined' && ME) || window.ME || null;
  }

  function formatarTamanho(bytes) {
    if (!bytes || bytes <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  function iconeArquivo(tipo, nome) {
    const ext = (nome || '').split('.').pop().toLowerCase();
    if (tipo.includes('pdf') || ext === 'pdf') {
      return '<span class="drive-badge-tipo badge-pdf">PDF</span>';
    }
    if (tipo.includes('image') || ['png', 'jpg', 'jpeg', 'webp', 'svg'].includes(ext)) {
      return '<span class="drive-badge-tipo badge-img">IMG</span>';
    }
    if (['doc', 'docx', 'odt', 'txt', 'rtf'].includes(ext)) {
      return '<span class="drive-badge-tipo badge-doc">DOC</span>';
    }
    if (['xls', 'xlsx', 'ods', 'csv'].includes(ext)) {
      return '<span class="drive-badge-tipo badge-plan">XLS</span>';
    }
    if (['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)) {
      return '<span class="drive-badge-tipo badge-zip">ZIP</span>';
    }
    return '<span class="drive-badge-tipo badge-bin">FILE</span>';
  }

  window.ViewDrive = async function (pastaIdParam) {
    const u = quem();
    if (!u) { location.hash = '#/login'; return; }
    if (window.navAtiva) navAtiva('#/drive');

    const app = document.getElementById('app');
    if (!app) return;

    let pastaAtualID = pastaIdParam || 0;
    let visualizandoCompartilhados = false;
    let termoBusca = '';

    app.innerHTML = `
      <div class="drive-topo-wrapper">
        <div class="drive-topo-info">
          <h2 style="margin:0 0 4px">Drive Local</h2>
          <p style="color:var(--tx2);font-size:13px;margin:0">Repositório corporativo seguro com arquivos físicos em disco e controle de acesso hierárquico.</p>
        </div>
        <div class="drive-topo-acoes">
          <div class="drive-tabs">
            <button type="button" class="btn-tab-drive ativo" id="tabMeuDrive">Meu Drive</button>
            <button type="button" class="btn-tab-drive" id="tabCompDrive">Compartilhados Comigo</button>
          </div>
          <div style="display:flex;gap:8px;flex-wrap:wrap">
            <button type="button" class="btn-acao-drive primario" id="btUploadDrive">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12"/></svg>
              Enviar Arquivo
            </button>
            <button type="button" class="btn-acao-drive" id="btNovaPastaDrive">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2zM12 11v6M9 14h6"/></svg>
              Nova Pasta
            </button>
          </div>
        </div>
      </div>

      <!-- Barra de Navegação e Breadcrumbs -->
      <div class="drive-bar-navegacao">
        <div class="drive-breadcrumbs" id="driveBreadcrumbs">
          <span class="crumb-item ativo">Meu Drive</span>
        </div>
        <div class="drive-busca-box">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
          <input type="text" id="driveBusca" placeholder="Filtrar arquivos ou pastas…" value="">
        </div>
      </div>

      <!-- Zona de Conteúdo / Dropzone -->
      <div class="drive-dropzone" id="driveDropzone">
        <div id="driveCorpo"><div class="carregando">Carregando itens do Drive…</div></div>
      </div>

      <!-- Input de Arquivo Oculto -->
      <input type="file" id="inpUploadFisico" style="display:none" multiple>
    `;

    const carregarItens = async () => {
      const container = document.getElementById('driveCorpo');
      if (!container) return;
      container.innerHTML = '<div class="carregando">Atualizando diretório…</div>';

      try {
        let url = `/api/drive/itens?pasta_id=${pastaAtualID}`;
        if (visualizandoCompartilhados) {
          url += '&compartilhados=1';
        }
        const res = await api(url);

        // Renderizar Breadcrumbs
        const bcEl = document.getElementById('driveBreadcrumbs');
        if (bcEl && res.breadcrumbs) {
          if (visualizandoCompartilhados) {
            bcEl.innerHTML = `<span class="crumb-item ativo">Compartilhados Comigo</span>`;
          } else {
            bcEl.innerHTML = res.breadcrumbs.map((bc, idx) => {
              const ehUltimo = idx === res.breadcrumbs.length - 1;
              if (ehUltimo) {
                return `<span class="crumb-item ativo">${esc(bc.nome)}</span>`;
              }
              return `<span class="crumb-item clicavel" data-pid="${bc.id}">${esc(bc.nome)}</span><span class="crumb-sep">/</span>`;
            }).join('');

            bcEl.querySelectorAll('.crumb-item.clicavel').forEach(c => {
              c.onclick = () => {
                pastaAtualID = +c.dataset.pid;
                carregarItens();
              };
            });
          }
        }

        // Aplicar filtro de busca client-side
        const q = termoBusca.trim().toLowerCase();
        const pastas = (res.pastas || []).filter(p => !q || p.nome.toLowerCase().includes(q));
        const arquivos = (res.arquivos || []).filter(a => !q || a.nome_original.toLowerCase().includes(q));

        if (pastas.length === 0 && arquivos.length === 0) {
          container.innerHTML = `
            <div class="drive-vazio">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
              <h4>Diretório Vazio</h4>
              <p>Nenhum arquivo ou pasta encontrado aqui. Arraste arquivos para cá ou use os botões acima.</p>
            </div>
          `;
          return;
        }

        let html = '';

        // Seção de Pastas
        if (pastas.length > 0) {
          html += `
            <div class="drive-secao-titulo">Pastas (${pastas.length})</div>
            <div class="drive-grid-pastas">
              ${pastas.map(p => `
                <div class="drive-cartao-pasta" data-pid="${p.id}">
                  <div class="pasta-info-topo">
                    <div class="pasta-icone-nome">
                      <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor" style="color:var(--ambar)"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
                      <span class="pasta-nome" title="${esc(p.nome)}">${esc(p.nome)}</span>
                    </div>
                    <div class="pasta-acoes">
                      <button type="button" class="btn-icone-mini btnPastaOpcoes" data-pid="${p.id}" data-pnome="${esc(p.nome)}" data-pedit="${p.pode_editar ? 1 : 0}" title="Mais Opções">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/></svg>
                      </button>
                    </div>
                  </div>
                  <div class="pasta-meta">
                    <span>${p.qtd_itens || 0} item(ns)</span>
                    ${p.compartilhada ? '<span class="badge-comp" title="Compartilhada">👥</span>' : ''}
                  </div>
                </div>
              `).join('')}
            </div>
          `;
        }

        // Seção de Arquivos
        if (arquivos.length > 0) {
          html += `
            <div class="drive-secao-titulo" style="margin-top:20px">Arquivos (${arquivos.length})</div>
            <div class="drive-grid-arquivos">
              ${arquivos.map(a => `
                <div class="drive-cartao-arquivo" data-aid="${a.id}">
                  <div class="arquivo-preview-box">
                    ${iconeArquivo(a.tipo, a.nome_original)}
                    <span class="arquivo-tamanho">${formatarTamanho(a.tamanho)}</span>
                  </div>
                  <div class="arquivo-info">
                    <div class="arquivo-nome" title="${esc(a.nome_original)}">${esc(a.nome_original)}</div>
                    <div class="arquivo-subinfo">
                      <span>${esc(a.autor_nome)}</span>
                      ${a.compartilhado ? '<span class="badge-comp" title="Compartilhado">👥</span>' : ''}
                    </div>
                  </div>
                  <div class="arquivo-botoes-hover">
                    <a href="/api/drive/download/${a.id}?inline=1" target="_blank" class="btn-mini-acao" title="Visualizar Inline">
                      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
                    </a>
                    <a href="/api/drive/download/${a.id}" class="btn-mini-acao" title="Baixar Arquivo">
                      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/></svg>
                    </a>
                    <button type="button" class="btn-mini-acao btnArqOpcoes" data-aid="${a.id}" data-anome="${esc(a.nome_original)}" data-aedit="${a.pode_editar ? 1 : 0}" title="Opções e Compartilhamento">
                      <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/></svg>
                    </button>
                  </div>
                </div>
              `).join('')}
            </div>
          `;
        }

        container.innerHTML = html;

        // Eventos de clique nas pastas
        container.querySelectorAll('.drive-cartao-pasta').forEach(el => {
          el.onclick = (e) => {
            if (e.target.closest('.btnPastaOpcoes')) return;
            pastaAtualID = +el.dataset.pid;
            carregarItens();
          };
        });

        // Menu de Opções de Pasta
        container.querySelectorAll('.btnPastaOpcoes').forEach(btn => {
          btn.onclick = (e) => {
            e.stopPropagation();
            const pid = +btn.dataset.pid;
            const pnome = btn.dataset.pnome;
            const pedit = btn.dataset.pedit === '1';
            abrirMenuPasta(pid, pnome, pedit);
          };
        });

        // Menu de Opções de Arquivo
        container.querySelectorAll('.btnArqOpcoes').forEach(btn => {
          btn.onclick = (e) => {
            e.stopPropagation();
            const aid = +btn.dataset.aid;
            const anome = btn.dataset.anome;
            const aedit = btn.dataset.aedit === '1';
            abrirMenuArquivo(aid, anome, aedit);
          };
        });

      } catch (err) {
        console.error('[DRIVE]', err);
        container.innerHTML = '<div class="carregando">Erro ao carregar diretório. Verifique sua conexão.</div>';
      }
    };

    // Alternância de Abas: Meu Drive vs Compartilhados Comigo
    const tabMeu = document.getElementById('tabMeuDrive');
    const tabComp = document.getElementById('tabCompDrive');

    tabMeu.onclick = () => {
      tabMeu.classList.add('ativo');
      tabComp.classList.remove('ativo');
      visualizandoCompartilhados = false;
      pastaAtualID = 0;
      carregarItens();
    };

    tabComp.onclick = () => {
      tabComp.classList.add('ativo');
      tabMeu.classList.remove('ativo');
      visualizandoCompartilhados = true;
      pastaAtualID = 0;
      carregarItens();
    };

    // Campo de Busca
    const inpBusca = document.getElementById('driveBusca');
    inpBusca.oninput = () => {
      termoBusca = inpBusca.value;
      carregarItens();
    };

    // Criar Nova Pasta
    document.getElementById('btNovaPastaDrive').onclick = () => {
      abrirModalNovaPasta(pastaAtualID, () => carregarItens());
    };

    // Upload de Arquivo
    const inpFisico = document.getElementById('inpUploadFisico');
    document.getElementById('btUploadDrive').onclick = () => {
      inpFisico.value = '';
      inpFisico.click();
    };

    inpFisico.onchange = async () => {
      const files = Array.from(inpFisico.files || []);
      if (files.length === 0) return;
      await enviarArquivosFisicos(files, pastaAtualID, () => carregarItens());
    };

    // Suporte Drag-and-Drop na Dropzone
    const dropzone = document.getElementById('driveDropzone');
    ['dragenter', 'dragover'].forEach(evName => {
      dropzone.addEventListener(evName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropzone.classList.add('drag-ativo');
      });
    });
    ['dragleave', 'drop'].forEach(evName => {
      dropzone.addEventListener(evName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropzone.classList.remove('drag-ativo');
      });
    });
    dropzone.addEventListener('drop', async (e) => {
      const files = Array.from((e.dataTransfer && e.dataTransfer.files) || []);
      if (files.length > 0) {
        await enviarArquivosFisicos(files, pastaAtualID, () => carregarItens());
      }
    });

    carregarItens();
  };

  // Envio de Arquivos Físicos com FormData Multipart
  async function enviarArquivosFisicos(files, pastaID, aoTerminar) {
    let enviados = 0;
    toast(`Iniciando upload de ${files.length} arquivo(s)…`, 'info');

    for (const f of files) {
      const fd = new FormData();
      fd.append('arquivo', f);
      if (pastaID > 0) {
        fd.append('pasta_id', String(pastaID));
      }

      try {
        const resp = await fetch('/api/drive/upload', {
          method: 'POST',
          headers: { 'X-SCI': '1' },
          body: fd,
        });
        if (!resp.ok) {
          const rJson = await resp.json().catch(() => ({}));
          toast(`Falha no upload de "${f.name}": ${rJson.erro || 'Erro'}`, 'erro');
        } else {
          enviados++;
        }
      } catch (err) {
        toast(`Erro de conexão ao enviar "${f.name}"`, 'erro');
      }
    }

    if (enviados > 0) {
      toast(`${enviados} arquivo(s) gravado(s) com sucesso em disco!`);
      if (aoTerminar) aoTerminar();
    }
  }

  // Modal de Criação de Pasta
  function abrirModalNovaPasta(pastaPaiID, aoCriar) {
    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:420px;margin:auto">
        <h3 style="margin:0 0 12px">Nova Pasta</h3>
        <div class="campo">
          <label>Nome da Pasta</label>
          <input type="text" id="inpNomeNovaPasta" placeholder="ex: Diretrizes, Escalas, Operação..." autofocus required>
        </div>
        <div style="display:flex;justify-content:flex-end;gap:8px;margin-top:16px">
          <button type="button" class="btn-cancelar" id="btCancNP">Cancelar</button>
          <button type="button" class="primario" id="btConfNP">Criar Pasta</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);

    m.modal.querySelector('#btCancNP').onclick = m.fechar;
    m.modal.querySelector('#btConfNP').onclick = async () => {
      const nome = m.modal.querySelector('#inpNomeNovaPasta').value.trim();
      if (!nome) { toast('Informe o nome da pasta', 'erro'); return; }

      try {
        const payload = { nome: nome };
        if (pastaPaiID > 0) payload.pasta_id = pastaPaiID;
        await api('/api/drive/pastas', { method: 'POST', body: JSON.stringify(payload) });
        toast('Pasta criada com sucesso!');
        m.fechar();
        if (aoCriar) aoCriar();
      } catch (err) {
        toast(err.message || 'Falha ao criar pasta', 'erro');
      }
    };
  }

  // Menu / Modal de Ações da Pasta
  function abrirMenuPasta(pastaID, nomeAtual, podeEditar) {
    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:400px;margin:auto">
        <h3 style="margin:0 0 8px">${esc(nomeAtual)}</h3>
        <p style="color:var(--tx3);font-size:12px;margin:0 0 16px">Gerenciamento e permissões da pasta</p>
        <div class="menu-acoes-drive-lista">
          <button type="button" class="btn-drive-menu-item" id="btCompPasta">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><line x1="20" y1="8" x2="20" y2="14"/><line x1="23" y1="11" x2="17" y2="11"/></svg>
            Compartilhar Acesso (Estilo Google Drive)
          </button>
          ${podeEditar ? `
            <button type="button" class="btn-drive-menu-item" id="btRenomearPasta">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
              Renomear Pasta
            </button>
            <button type="button" class="btn-drive-menu-item perigo" id="btExcluirPasta">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
              Excluir Pasta e Arquivos Físicos
            </button>
          ` : ''}
        </div>
        <div style="text-align:right;margin-top:16px">
          <button type="button" class="btn-cancelar" id="btFecharMP">Fechar</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btFecharMP').onclick = m.fechar;

    m.modal.querySelector('#btCompPasta').onclick = () => {
      m.fechar();
      abrirModalCompartilhamento('pasta', pastaID, nomeAtual);
    };

    if (podeEditar) {
      m.modal.querySelector('#btRenomearPasta').onclick = () => {
        m.fechar();
        const novo = prompt('Novo nome para a pasta:', nomeAtual);
        if (novo && novo.trim() && novo.trim() !== nomeAtual) {
          api(`/api/drive/pastas/${pastaID}`, { method: 'PATCH', body: JSON.stringify({ nome: novo.trim() }) })
            .then(() => { toast('Pasta renomeada!'); location.hash = '#/drive'; })
            .catch(e => toast(e.message || 'Erro', 'erro'));
        }
      };

      m.modal.querySelector('#btExcluirPasta').onclick = async () => {
        m.fechar();
        if (confirm(`Tem certeza que deseja excluir "${nomeAtual}" e TODOS os seus arquivos do disco? Esta ação é irreversível.`)) {
          try {
            await api(`/api/drive/pastas/${pastaID}`, { method: 'DELETE' });
            toast('Pasta e arquivos excluídos do disco local!');
            window.ViewDrive();
          } catch (e) {
            toast(e.message || 'Falha ao excluir', 'erro');
          }
        }
      };
    }
  }

  // Menu / Modal de Ações de Arquivo
  function abrirMenuArquivo(arquivoID, nomeAtual, podeEditar) {
    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:400px;margin:auto">
        <h3 style="margin:0 0 8px">${esc(nomeAtual)}</h3>
        <p style="color:var(--tx3);font-size:12px;margin:0 0 16px">Gerenciamento do documento físico</p>
        <div class="menu-acoes-drive-lista">
          <a href="/api/drive/download/${arquivoID}?inline=1" target="_blank" class="btn-drive-menu-item">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
            Visualizar no Navegador
          </a>
          <a href="/api/drive/download/${arquivoID}" class="btn-drive-menu-item">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/></svg>
            Baixar Cópia
          </a>
          <button type="button" class="btn-drive-menu-item" id="btCompArq">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><line x1="20" y1="8" x2="20" y2="14"/><line x1="23" y1="11" x2="17" y2="11"/></svg>
            Compartilhar Acesso
          </button>
          ${podeEditar ? `
            <button type="button" class="btn-drive-menu-item" id="btRenomearArq">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
              Renomear Arquivo
            </button>
            <button type="button" class="btn-drive-menu-item perigo" id="btExcluirArq">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
              Excluir do Disco Local
            </button>
          ` : ''}
        </div>
        <div style="text-align:right;margin-top:16px">
          <button type="button" class="btn-cancelar" id="btFecharMA">Fechar</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btFecharMA').onclick = m.fechar;

    m.modal.querySelector('#btCompArq').onclick = () => {
      m.fechar();
      abrirModalCompartilhamento('arquivo', arquivoID, nomeAtual);
    };

    if (podeEditar) {
      m.modal.querySelector('#btRenomearArq').onclick = () => {
        m.fechar();
        const novo = prompt('Novo nome para o arquivo:', nomeAtual);
        if (novo && novo.trim() && novo.trim() !== nomeAtual) {
          api(`/api/drive/arquivos/${arquivoID}`, { method: 'PATCH', body: JSON.stringify({ nome: novo.trim() }) })
            .then(() => { toast('Arquivo renomeado!'); window.ViewDrive(); })
            .catch(e => toast(e.message || 'Erro', 'erro'));
        }
      };

      m.modal.querySelector('#btExcluirArq').onclick = async () => {
        m.fechar();
        if (confirm(`Excluir permanentemente "${nomeAtual}" do disco?`)) {
          try {
            await api(`/api/drive/arquivos/${arquivoID}`, { method: 'DELETE' });
            toast('Arquivo removido do disco local!');
            window.ViewDrive();
          } catch (e) {
            toast(e.message || 'Falha ao excluir', 'erro');
          }
        }
      };
    }
  }

  // Modal de Compartilhamento Granular estilo Google Drive
  async function abrirModalCompartilhamento(tipo, itemID, nomeItem) {
    const html = `
      <div class="cartao modal-conteudo-box" style="max-width:540px;margin:auto">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
          <h3 style="margin:0">Compartilhar "${esc(nomeItem)}"</h3>
          <button type="button" class="btn-fechar-modal" id="btXComp">✕</button>
        </div>
        <p style="color:var(--tx3);font-size:12px;margin:0 0 16px">Defina quem pode visualizar ou editar este ${tipo}.</p>

        <!-- Formulário Adicionar Permissão -->
        <div class="drive-form-compartilhar">
          <div class="campo" style="margin-bottom:8px">
            <label>Tipo de Destinatário</label>
            <select id="selAlvoTipo">
              <option value="grupo">Grupo / Unidade Inteira</option>
              <option value="usuario">Militar / Usuário Específico</option>
            </select>
          </div>
          <div class="campo" style="margin-bottom:8px">
            <label id="lblAlvoID">Selecione o Grupo</label>
            <select id="selAlvoID"><option value="">Carregando opções…</option></select>
          </div>
          <div class="campo" style="margin-bottom:12px">
            <label>Nível de Acesso</label>
            <select id="selPodeEditar">
              <option value="0">Visualizador (Somente Leitura / Download)</option>
              <option value="1">Editor (Pode renomear, excluir e enviar)</option>
            </select>
          </div>
          <button type="button" class="primario" id="btAddPermissao" style="width:100%">Conceder Acesso</button>
        </div>

        <!-- Lista de Compartilhamentos Ativos -->
        <div style="margin-top:20px">
          <h4 style="margin:0 0 8px;font-size:13px;color:var(--tx2)">Pessoas e Grupos com Acesso</h4>
          <div id="listaCompartilhamentosAtivos" style="max-height:160px;overflow-y:auto;border:1px solid var(--borda);border-radius:6px;padding:8px">
            <div class="carregando" style="padding:10px">Buscando compartilhamentos…</div>
          </div>
        </div>

        <div style="text-align:right;margin-top:16px">
          <button type="button" class="primario" id="btConcluirComp">Concluído</button>
        </div>
      </div>
    `;
    const m = abrirModal(html);
    m.modal.querySelector('#btXComp').onclick = m.fechar;
    m.modal.querySelector('#btConcluirComp').onclick = m.fechar;

    const selTipo = m.modal.querySelector('#selAlvoTipo');
    const selAlvo = m.modal.querySelector('#selAlvoID');
    const lblAlvo = m.modal.querySelector('#lblAlvoID');

    // Preencher opções de Grupos e Usuários
    let gruposCache = [], usuariosCache = [];
    try {
      const gRes = await api('/api/grupos').catch(() => []);
      gruposCache = Array.isArray(gRes) ? gRes : (gRes.grupos || []);
    } catch (e) {}

    try {
      const uRes = await api('/api/usuarios').catch(() => []);
      usuariosCache = Array.isArray(uRes) ? uRes : (uRes.usuarios || []);
    } catch (e) {}

    const atualizarOpcoesAlvo = () => {
      const t = selTipo.value;
      if (t === 'grupo') {
        lblAlvo.textContent = 'Selecione o Grupo / Unidade';
        selAlvo.innerHTML = gruposCache.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');
      } else {
        lblAlvo.textContent = 'Selecione o Usuário';
        selAlvo.innerHTML = usuariosCache.map(u => `<option value="${u.id}">${esc(u.nome_guerra || u.login)} (${esc(u.papel)})</option>`).join('');
      }
    };
    selTipo.onchange = atualizarOpcoesAlvo;
    atualizarOpcoesAlvo();

    // Carregar compartilhamentos atuais
    const carregarComps = async () => {
      const cBox = m.modal.querySelector('#listaCompartilhamentosAtivos');
      const param = tipo === 'pasta' ? `pasta_id=${itemID}` : `arquivo_id=${itemID}`;
      try {
        const res = await api(`/api/drive/compartilhamentos?${param}`);
        const comps = res.compartilhamentos || [];
        if (comps.length === 0) {
          cBox.innerHTML = '<div style="font-size:12px;color:var(--tx3);text-align:center;padding:10px">Apenas você e os gerentes da sua unidade possuem acesso no momento.</div>';
          return;
        }
        cBox.innerHTML = comps.map(c => `
          <div class="item-comp-linha" style="display:flex;align-items:center;justify-content:space-between;padding:6px 4px;border-bottom:1px solid var(--borda)">
            <div>
              <div style="font-weight:600;font-size:13px">${esc(c.alvo_nome)} <span class="badge-mini" style="font-size:10px">${esc(c.alvo_tipo)}</span></div>
              <div style="font-size:11px;color:var(--tx3)">${c.pode_editar ? 'Editor' : 'Visualizador'}</div>
            </div>
            <button type="button" class="btn-icone-mini btnRevogarComp" data-cid="${c.id}" title="Remover Permissão" style="color:var(--verm)">✕</button>
          </div>
        `).join('');

        cBox.querySelectorAll('.btnRevogarComp').forEach(b => {
          b.onclick = async () => {
            const cid = +b.dataset.cid;
            try {
              await api(`/api/drive/compartilhamentos/${cid}`, { method: 'DELETE' });
              toast('Compartilhamento revogado!');
              carregarComps();
            } catch (err) {
              toast('Falha ao revogar', 'erro');
            }
          };
        });
      } catch (e) {
        cBox.innerHTML = '<div style="color:var(--verm);font-size:12px">Erro ao carregar permissões.</div>';
      }
    };

    carregarComps();

    // Adicionar Permissão
    m.modal.querySelector('#btAddPermissao').onclick = async () => {
      const alvoId = +selAlvo.value;
      if (!alvoId) { toast('Selecione um alvo válido', 'erro'); return; }

      const payload = {
        alvo_tipo: selTipo.value,
        alvo_id: alvoId,
        pode_editar: selTipo.form ? (selTipo.form.selPodeEditar.value === '1') : (m.modal.querySelector('#selPodeEditar').value === '1'),
      };
      if (tipo === 'pasta') payload.pasta_id = itemID;
      else payload.arquivo_id = itemID;

      try {
        await api('/api/drive/compartilhar', { method: 'POST', body: JSON.stringify(payload) });
        toast('Permissão concedida com sucesso!');
        carregarComps();
      } catch (err) {
        toast(err.message || 'Falha ao conceder acesso', 'erro');
      }
    };
  }

})();
