/* SCI — views de GESTÃO (redesign): #/admin · #/grupos (Gerenciar) · #/perfil
   window.ViewAdmin / window.ViewGrupos / window.ViewPerfil (async).
   Consome os helpers globais do core: api, esc, toast, fmtData, fmtHora, pill,
   abrirModal (insere .modal-mask no body e devolve o elemento), confirmar(msg) →
   Promise<boolean>, navAtiva(hash). Vanilla, sem build, sem CDN.
   Comportamento copiado do protótipo web/app.js (v9.11.2) — nada pode sumir. */
/* [FATIADO da views_gestao.js — onda de modularização; recorte puro] */
'use strict';
(function () {
'use strict';
  const $ = s => document.querySelector(s);
  const rotuloPapel = p => p === 'admin' ? 'ADMIN' : p === 'gerente' ? 'GERENTE' : p === 'chefe_setor' ? 'CHEFE DE SETOR' : 'OPERADOR';
  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;
  const ativosDe = l => (l || []).filter(x => x.ativo === 1 || x.ativo === true);


  /* ---------- ordem 06/10 (item 10): controles de TABELA — ordenação + paginação ----------
     window.tblOrdenar / window.tblPaginar (também via window.tabelaControles). Estado por
     tabela (chave em memória do módulo). Aplicar DEPOIS de o tbody ter as linhas renderizadas:
       tblOrdenar('agregado', table, tbody, [ {sel:'td:nth-child(1)', tipo:'txt'}, ... ]);
       tblPaginar('chefes', table, tbody, 20, tituloEl?);
     - tipo 'num': comparação numérica (ids/quantidades); tipo 'txt': localeCompare pt-BR;
     - clique no TH alterna asc/desc com indicador ▲/▼ (th.sorted-asc / th.sorted-desc);
     - paginação 20/página com controles ‹ 1/N › na div .tbl-pag (inserida após a .rolagem);
     - NÃO usar nos RELATÓRIOS (ordenação hierárquica cravada pelo Diretor). */
  const TBL_ESTADO = {}; // chave → { col, asc } | { pag }

  function tblHeaders(table) { return [...table.querySelectorAll('thead th')]; }
  function tblLinhas(tbody) { return [...tbody.querySelectorAll('tr')]; }
  const eLinhaVazio = tr => tr.querySelector('.vazio, .carregando') !== null;
  function tblOrdenarPor(chave, table, tbody, col, tipo, asc) {
    const linhas = tblLinhas(tbody).filter(tr => !eLinhaVazio(tr) && !tr.classList.contains('conf-toggle-detalhe'));
    const valDe = tr => {
      const td = tr.children[col];
      if (!td) return '';
      if (tipo === 'num') {
        const t = parseFloat((td.dataset.v !== undefined ? td.dataset.v : td.textContent).replace(/[^\d.,-]/g, '').replace(',', '.'));
        return isNaN(t) ? '' : t;
      }
      return td.textContent.trim();
    };
    linhas.sort((a, b) => {
      const va = valDe(a), vb = valDe(b);
      let r;
      if (tipo === 'num') r = (parseFloat(va) || 0) - (parseFloat(vb) || 0);
      else r = String(va).localeCompare(String(vb), 'pt', { sensitivity: 'base' }) || String(va).localeCompare(String(vb));
      return asc ? r : -r;
    });
    linhas.forEach(tr => tbody.appendChild(tr));
    tblHeaders(table).forEach((th, i) => {
      th.classList.remove('sorted-asc', 'sorted-desc');
      if (i === col) th.classList.add(asc ? 'sorted-asc' : 'sorted-desc');
    });
    (TBL_ESTADO[chave] = TBL_ESTADO[chave] || {}).col = col;
    TBL_ESTADO[chave].asc = asc;
    TBL_ESTADO[chave].tipo = tipo;
    TBL_ESTADO[chave].pag = 1; // reordenar volta à primeira página
  }
  function tblOrdenar(chave, table, tbody, cols) {
    if (!table || !tbody) return;
    const est = TBL_ESTADO[chave] || (TBL_ESTADO[chave] = {});
    tblHeaders(table).forEach((th, i) => {
      const cfg = (cols || [])[i]; // posicional: {tipo:'num'|'txt'} ou null (coluna sem ordenar, ex. Ações)
      if (!cfg) return;
      th.style.cursor = 'pointer';
      th.title = th.title || 'Clique para ordenar';
      if (th.dataset.tblOrdLigado) return;
      th.dataset.tblOrdLigado = '1';
      th.addEventListener('click', () => {
        const e2 = TBL_ESTADO[chave] || (TBL_ESTADO[chave] = {});
        const asc = !(e2.col === i && e2.asc);
        tblOrdenarPor(chave, table, tbody, i, cfg.tipo || 'txt', asc);
        if (e2._repag) e2._repag();
      });
    });
    if (est.col !== undefined) tblOrdenarPor(chave, table, tbody, est.col, est.tipo || 'txt', est.asc);
  }
  function tblPaginar(chave, table, tbody, tamPag) {
    if (!table || !tbody) return;
    tamPag = tamPag || 20;
    const est = TBL_ESTADO[chave] || (TBL_ESTADO[chave] = {});
    est._repag = () => tblPaginarPara(chave, table, tbody, tamPag, est);
    tblPaginarPara(chave, table, tbody, tamPag, est);
  }
  window.tblOrdenar = tblOrdenar;
  const tabelaControles = (chave, table, tbody, cols, tamPag) => {
    tblOrdenar(chave, table, tbody, cols);
    if (tamPag) tblPaginar(chave, table, tbody, tamPag);
  };
  window.tabelaControles = tabelaControles;


  /* ---------- blocos compartilhados ---------- */

  // abrirModal (core) insere o .modal-mask no body e devolve o elemento; aqui só
  // completa com fechar por ESC / clique fora, como nos modais do protótipo.
  // Onda UX 0510: repassa opcoes (ex. {largura:'850px'}) ao abrirModal do core.






  // Card interativo de nó da árvore com expansão e foco recursivo (Obsidian Graph View Style)



  // Redefinir senha (admin usa confirmação; operadores do gerente, campo único)

  // Mover conta entre grupos (admin: quaisquer; gerente: backend limita à hierarquia)


  /* =====================================================================
     #/admin — ADMINISTRAÇÃO (só admin): Usuários · Grupos · Backup
     ===================================================================== */


  async function admDashboard() {
    try {
      const [grupos, contas, confs] = await Promise.all([api('/api/grupos'), api('/api/usuarios'), api('/api/conferencia/lista').catch(()=>[])]);
      const efetivoTotal = grupos.reduce((acc, g) => acc + (g.efetivo || 0), 0);
      const ativos = contas.filter(c => c.ativo).length;
      const gerentesFaltando = grupos.filter(g => !contas.some(c => c.grupo_id === g.id && c.papel === 'gerente')).length;
      $('#adm').innerHTML = `
        <div class="resumo" style="margin-bottom:14px">
          <div class="caixa"><b>${grupos.length}</b><span>Grupos</span></div>
          <div class="caixa"><b>${efetivoTotal}</b><span>Total de Pessoal (Banco)</span></div>
          <div class="caixa"><b>${contas.length}</b><span>Usuários Registrados</span></div>
          <div class="caixa"><b>${confs.length}</b><span>Conferências</span></div>
        </div>
        <div class="cartao">
          <h3 style="margin-top:0">Saúde da Estrutura</h3>
          <ul style="color:var(--tx2);padding-left:20px;line-height:1.8">
            <li>Proteção de Dados: <span style="color:var(--verde-claro)">Backups Automáticos em Background (Ativo)</span></li>
            <li>Alertas de Gestão: <b style="color:${gerentesFaltando > 0 ? 'var(--verm-txt)' : 'var(--verde-claro)'}">${gerentesFaltando} grupos sem gerente</b></li>
            <li>Controle de Acesso: Bloqueio contra Força Bruta ativo (Limiter em memória isolada).</li>
          </ul>
        </div>`;
    } catch (e) {
      $('#adm').innerHTML = '<span class="vazio">Falha ao carregar dashboard.</span>';
    }
  }

  /* --- ADMIN › usuários: Gestão Completa de Contas, Nomes e Multi-Funções --- */
  async function admUsuarios() {
    const [lista, grupos, funcoesRes, setoresRes] = await Promise.all([
      api('/api/usuarios'),
      api('/api/grupos'),
      api('/api/catalogo/funcoes').catch(() => []),
      api('/api/catalogo/setores').catch(() => [])
    ]);
    const funcoesLista = (funcoesRes && (funcoesRes.funcoes || funcoesRes)) || [];
    const setoresLista = (Array.isArray(setoresRes) ? setoresRes : []).filter(s => s.ativo !== false);
    const nomeGrupo = gid => (grupos.find(g => g.id === gid) || {}).nome || '—';
    // ITEM 2 (ordem Diretor 06/10): login é identificação SENSÍVEL — só o admin
    // vê na UI. Gerente/encarregado/operador veem nome de guerra (ou completo).
    const isAdminViewer = (quem() || {}).papel === 'admin';
    const rotuloConta = u => esc(u.nome_guerra || u.nome_completo || (isAdminViewer ? u.login : '—'));
    const optsGrupos = `<option value="">— Sem grupo (Global / Atribuir depois) —</option>` + grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('');

    function formatarNomeUsuario(completo, guerra) {
      const c = (completo || '').trim();
      const g = (guerra || '').trim();
      if (!c && !g) return '<span style="color:var(--tx3)">—</span>';
      if (!c) return `<b>${esc(g)}</b>`;
      if (!g) return esc(c);
      const idx = c.toLowerCase().indexOf(g.toLowerCase());
      if (idx !== -1) {
        return `${esc(c.substring(0, idx))}<b>${esc(c.substring(idx, idx + g.length))}</b>${esc(c.substring(idx + g.length))}`;
      }
      return `${esc(c)} (<b>${esc(g)}</b>)`;
    }

    $('#adm').innerHTML = `
      <div class="cartao">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
          <div>
            <h3 style="margin:0 0 4px">Gestão de Usuários & Multi-Funções</h3>
            <p style="color:var(--tx2); font-size:13px; margin:0">Gerencie logins, nomes militares e atribua múltiplos papéis/funções por unidade (cadeiras de comando e operação).</p>
          </div>
          <button class="primario" id="btNovoUsuario">+ Novo Usuário</button>
        </div>

        <div class="form-linha" style="margin-bottom:12px; gap:8px; flex-wrap:wrap">
          <div class="campo" style="flex:1; min-width:180px">
            <label>Buscar usuário</label>
            <input id="fULogin" placeholder="${isAdminViewer ? 'Filtrar por login, nome de guerra ou completo…' : 'Filtrar por nome…'}">
          </div>
          <div class="campo" style="min-width:160px">
            <label>Grupo</label>
            <select id="fUGrupo">
              <option value="">todos os grupos</option>
              ${grupos.map(g => `<option value="${g.id}">${esc(g.nome)}</option>`).join('')}
            </select>
          </div>
          <div class="campo" style="min-width:140px">
            <label>Papel</label>
            <select id="fUPapel">
              <option value="">todos os papéis</option>
              <option value="admin">Administrador</option>
              <option value="gerente">Gerente</option>
              <option value="operador">Operador</option>
              <option value="chefe_setor">Chefe de Setor</option>
            </select>
          </div>
          <div class="campo" style="min-width:120px">
            <label>Status</label>
            <select id="fUAtivo">
              <option value="">todas</option>
              <option value="1">ativa</option>
              <option value="0">desativada</option>
            </select>
          </div>
        </div>

        <div class="rolagem">
          <table id="tabU">
            <thead>
              <tr>
                <th style="width:38px; text-align:center">Foto</th>
                <th>ID</th>
                <th>${isAdminViewer ? 'Login' : 'Guerra'}</th>
                <th>Identificação Militar</th>
                <th>Funções / Cadeiras Atribuídas</th>
                <th>Status</th>
                <th>Ações</th>
              </tr>
            </thead>
            <tbody>
              ${lista.map(u => {
                const papeis = u.papeis || [];
                const chips = papeis.map(p => {
                  const rot = rotuloPapel(p.papel);
                  const grp = p.grupo_nome ? p.grupo_nome : (p.papel === 'admin' ? 'Global' : 'Sem grupo');
                  const func = p.funcao_nome ? ` · ${p.funcao_nome}` : '';
                  const ehUnico = p.papel === 'gerente' ? ' (Titular Único)' : '';
                  return `
                    <span class="papel-chip ${p.papel}">
                      <b>${esc(rot)}</b>: ${esc(grp)}${esc(func)}${ehUnico}
                      ${papeis.length > 1 ? `<button type="button" class="btn-del-papel" data-delpapel="${p.id}" data-uid="${u.id}" title="Remover este papel do usuário">✕</button>` : ''}
                    </span>
                  `;
                }).join('') || `<span class="papel-chip ${u.papel}"><b>${rotuloPapel(u.papel)}</b>: ${esc(nomeGrupo(u.grupo_id))}</span>`;

                const papeisStr = papeis.map(p => p.papel).join(' ') + ' ' + u.papel;

                return `
                  <tr data-login="${esc(u.login)}" data-nomes="${esc((u.nome_guerra || '') + ' ' + (u.nome_completo || ''))}" data-gid="${u.grupo_id || 0}" data-papeis="${esc(papeisStr)}" data-ativo="${u.ativo ? 1 : 0}">
                    <td style="text-align:center; vertical-align:middle">
                      <div style="width:32px; height:32px; border-radius:50%; overflow:hidden; background:var(--painel3); border:1px solid var(--borda); display:inline-flex; align-items:center; justify-content:center">
                        ${u.foto_base64 ? `<img src="${u.foto_base64}" style="width:100%; height:100%; object-fit:cover">` : `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`}
                      </div>
                    </td>
                    <td class="num">#${u.id}</td>
                    <td><b>${rotuloConta(u)}</b></td>
                    <td>
                      <div>${formatarNomeUsuario(u.nome_completo, u.nome_guerra)}</div>
                    </td>
                    <td>
                      <div style="display:flex; flex-wrap:wrap; align-items:center; gap:4px">
                        ${chips}
                        <button type="button" class="acao-linha" data-addpapel="${u.id}" data-login="${esc(u.login)}" style="font-size:11px; padding:2px 8px; font-weight:700; color:var(--verde-claro)">+ Função</button>
                      </div>
                    </td>
                    <td><span style="color:${u.ativo ? 'var(--verde-claro)' : 'var(--tx3)'}; font-weight:700">${u.ativo ? 'Ativa' : 'Desativada'}</span></td>
                    <td>
                      <button class="acao-linha" data-edperfil="${u.id}" style="font-weight:600">👤 Perfil</button>
                      <button class="acao-linha" data-ednomes="${u.id}" data-login="${esc(u.login)}" data-guerra="${esc(u.nome_guerra || '')}" data-completo="${esc(u.nome_completo || '')}">✏️ Nomes</button>
                      <button class="acao-linha" data-id="${u.id}" data-login="${esc(u.login)}">🔑 Senha</button>
                      ${u.papel !== 'admin' ? `
                        <button class="acao-linha" data-mv="${u.id}" data-login="${esc(u.login)}" data-papel="${u.papel}">Mover</button>
                        <button class="acao-linha perigo" data-exc="${u.id}" data-login="${esc(u.login)}">Excluir</button>
                      ` : ''}
                    </td>
                  </tr>
                `;
              }).join('') || '<tr><td colspan="6"><span class="vazio">Nenhuma conta encontrada.</span></td></tr>'}
            </tbody>
          </table>
        </div>
        <p style="color:var(--tx2);font-size:12.5px;margin-top:10px">
          💡 <b>Doutrina de Multi-Funções:</b> Usuários podem acumular múltiplas funções em diferentes grupos. Cada grupo possui apenas <b>1 Gerente titular</b> (função única), enquanto pode possuir <b>múltiplos Operadores</b> (função múltipla com caixa de email compartilhada).
        </p>
      </div>
    `;

    const aplicarFiltro = () => {
      const q = $('#fULogin').value.trim().toLowerCase();
      const gid = $('#fUGrupo').value;
      const papel = $('#fUPapel').value;
      const at = $('#fUAtivo').value;
      document.querySelectorAll('#tabU tbody tr[data-login]').forEach(tr => {
        // ITEM 2: filtro por NOME para todos; por login só quando o viewer é admin
        // (data-login permanece como atributo interno — nada é exibido na tela).
        const nomeMatches = tr.dataset.nomes.toLowerCase().includes(q);
        const loginMatches = isAdminViewer && tr.dataset.login.toLowerCase().includes(q);
        const grupoMatches = !gid || tr.dataset.gid === gid;
        const papelMatches = !papel || tr.dataset.papeis.includes(papel);
        const ativoMatches = at === '' || tr.dataset.ativo === at;
        tr.style.display = (nomeMatches || loginMatches) && grupoMatches && papelMatches && ativoMatches ? '' : 'none';
      });
    };

    $('#fULogin').oninput = aplicarFiltro;
    $('#fUGrupo').onchange = aplicarFiltro;
    $('#fUPapel').onchange = aplicarFiltro;
    $('#fUAtivo').onchange = aplicarFiltro;

    // Ação: Novo Usuário
    $('#btNovoUsuario').onclick = () => modalNovoUsuario();

    // Ação: Atribuir Papel
    document.querySelectorAll('#tabU button[data-addpapel]').forEach(b => {
      b.onclick = () => modalAtribuirPapel(b.dataset.addpapel, b.dataset.login);
    });

    // Ação: Remover Papel
    document.querySelectorAll('#tabU button[data-delpapel]').forEach(b => {
      b.onclick = async (ev) => {
        ev.stopPropagation();
        const pId = b.dataset.delpapel;
        const uId = b.dataset.uid;
        if (!(await confirmar('Deseja remover esta função/cadeira atribuída a este usuário?'))) return;
        try {
          await api(`/api/usuarios/${uId}/papeis/${pId}`, { method: 'DELETE' });
          toast('Função removida com sucesso!');
          admUsuarios();
        } catch (e) {}
      };
    });

    // Ação: Editar Perfil Completo
    document.querySelectorAll('#tabU button[data-edperfil]').forEach(b => {
      b.onclick = () => {
        const uid = +b.dataset.edperfil;
        const uAlvo = lista.find(x => x.id === uid);
        if (uAlvo) modalEditarPerfilUsuario(uAlvo);
      };
    });

    // Ação: Editar Nomes
    document.querySelectorAll('#tabU button[data-ednomes]').forEach(b => {
      b.onclick = () => modalEditarNomes(b.dataset.ednomes, b.dataset.login, b.dataset.guerra, b.dataset.completo);
    });

    // Ação: Senha
    document.querySelectorAll('#tabU button[data-id]').forEach(b => {
      b.onclick = () => modalSenha(b.dataset.id, b.dataset.login, true, () => admUsuarios());
    });

    // Ação: Mover
    document.querySelectorAll('#tabU button[data-mv]').forEach(b => {
      b.onclick = () => modalMover(b.dataset.mv, b.dataset.login, optsGrupos,
        'Se for o único gerente do grupo de origem, virará operador no destino e o operador mais antigo assumirá o grupo.',
        () => admUsuarios());
    });

    // Ação: Excluir
    document.querySelectorAll('#tabU button[data-exc]').forEach(b => {
      b.onclick = async () => {
        if (!(await confirmar(`Excluir a conta "${b.dataset.login}"?`))) return;
        try {
          const r = await api(`/api/usuarios/${b.dataset.exc}`, { method: 'DELETE' });
          toast(r.desativado ? 'Conta desativada (histórico preservado)' : 'Conta excluída');
          admUsuarios();
        } catch (e) {}
      };
    });

    function modalEditarPerfilUsuario(u) {
      let fotoAtual = u.foto_base64 || '';
      const sangueOpts = ["", "A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"];
      const html = `
        <div class="modal" style="max-width:620px; max-height:90vh; overflow-y:auto">
          <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:14px">
            <div>
              <h3 style="margin:0 0 2px">👤 Perfil do Usuário — ${esc(u.nome_guerra || u.nome_completo || (isAdminViewer ? u.login : '—'))}</h3>
              <p style="color:var(--tx2);font-size:12px;margin:0">ID #${u.id} · Papel: <b>${rotuloPapel(u.papel)}</b></p>
            </div>
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">✕</button>
          </div>

          <!-- Foto 1x1 e Ações de Imagem -->
          <div style="display:flex; align-items:center; gap:16px; margin-bottom:16px; padding-bottom:14px; border-bottom:1px solid var(--borda)">
            <div id="mPfFotoBox" style="width:76px; height:76px; border-radius:12px; border:2px solid var(--borda2); overflow:hidden; display:flex; align-items:center; justify-content:center; background:var(--painel2); flex-shrink:0">
              ${fotoAtual ? `<img src="${fotoAtual}" style="width:100%; height:100%; object-fit:cover">` : `<svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`}
            </div>
            <div style="display:flex; flex-direction:column; gap:6px">
              <input type="file" id="mPfInpFile" accept="image/*" style="display:none">
              <button type="button" class="acao-linha" id="mPfBtTrocarFoto" style="font-size:12px; padding:4px 10px">📷 Alterar Foto 1x1</button>
              ${fotoAtual ? `<button type="button" class="acao-linha perigo" id="mPfBtRemoverFoto" style="font-size:11.5px; padding:3px 8px">Remover Foto</button>` : ''}
              <small style="color:var(--tx3); font-size:11px">Recorte 1x1 automático via navegador.</small>
            </div>
          </div>

          <!-- Campos Cadastrais -->
          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Nome de Guerra (NOME) *</label>
              <input id="mPfNg" value="${esc(u.nome_guerra || '')}">
            </div>
            <div class="campo" style="flex:1">
              <label>Nome Completo *</label>
              <input id="mPfNc" value="${esc(u.nome_completo || '')}">
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Data de Nascimento</label>
              <input id="mPfDataNasc" type="date" value="${esc(u.data_nascimento || '')}">
            </div>
            <div class="campo" style="flex:1">
              <label>Tipo Sanguíneo</label>
              <select id="mPfTipoSang">
                <option value="">Não informado</option>
                ${sangueOpts.filter(Boolean).map(s => `<option value="${s}" ${u.tipo_sanguineo === s ? 'selected' : ''}>${s}</option>`).join('')}
              </select>
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Telefone / WhatsApp</label>
              <input id="mPfTel" type="tel" value="${esc(u.telefone || '')}" placeholder="(XX) XXXXX-XXXX">
            </div>
            <div class="campo" style="flex:1">
              <label>E-mail</label>
              <input id="mPfEmail" type="email" value="${esc(u.email || '')}" placeholder="militar@eb.mil.br">
            </div>
          </div>

          <div class="campo" style="margin-bottom:8px">
            <label>Endereço Completo</label>
            <input id="mPfEndereco" value="${esc(u.endereco || '')}" placeholder="Rua, número, bairro, cidade - UF">
          </div>

          <div class="campo" style="margin-bottom:16px">
            <label>Unidade / Grupo de Alocação</label>
            <select id="mPfGrupo">
              <option value="">— Sem grupo (Global) —</option>
              ${grupos.map(g => `<option value="${g.id}" ${u.grupo_id === g.id ? 'selected' : ''}>${esc(g.nome)}</option>`).join('')}
            </select>
          </div>

          <div style="display:flex; justify-content:flex-end; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button class="primario" id="mPfSalvar">Salvar Dados</button>
          </div>
        </div>
      `;
      const m = modal(html);
      const fotoBox = m.querySelector('#mPfFotoBox');
      const inpFile = m.querySelector('#mPfInpFile');

      m.querySelector('#mPfBtTrocarFoto').onclick = () => inpFile.click();
      inpFile.onchange = (e) => {
        const file = e.target.files && e.target.files[0];
        if (file) {
          processarFoto1x1(file, (dataUrl) => {
            fotoAtual = dataUrl;
            fotoBox.innerHTML = `<img src="${fotoAtual}" style="width:100%; height:100%; object-fit:cover">`;
          });
        }
      };

      const btRem = m.querySelector('#mPfBtRemoverFoto');
      if (btRem) {
        btRem.onclick = () => {
          fotoAtual = '';
          fotoBox.innerHTML = `<svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`;
          btRem.remove();
        };
      }

      m.querySelector('#mPfSalvar').onclick = async () => {
        const ng = m.querySelector('#mPfNg').value.trim();
        const nc = m.querySelector('#mPfNc').value.trim();
        const dataNasc = m.querySelector('#mPfDataNasc').value;
        const tipoSang = m.querySelector('#mPfTipoSang').value;
        const tel = m.querySelector('#mPfTel').value.trim();
        const email = m.querySelector('#mPfEmail').value.trim();
        const endr = m.querySelector('#mPfEndereco').value.trim();
        const gidVal = m.querySelector('#mPfGrupo').value;
        const gid = gidVal ? +gidVal : 0;

        if (!ng || !nc) {
          toast('Nome de guerra e nome completo são obrigatórios', 'erro');
          return;
        }

        try {
          await api(`/api/usuarios/${u.id}`, {
            method: 'PATCH',
            body: JSON.stringify({
              nome_guerra: ng,
              nome_completo: nc,
              data_nascimento: dataNasc,
              tipo_sanguineo: tipoSang,
              telefone: tel,
              email: email,
              endereco: endr,
              foto_base64: fotoAtual,
              grupo_id: gid
            })
          });
          toast('Perfil do usuário atualizado com sucesso!');
          m.remove();
          admUsuarios();
        } catch (e) {}
      };
    }

    function modalNovoUsuario() {
      const html = `
        <div class="modal" style="max-width:500px">
          <h3 style="margin-top:0">Criar Novo Usuário</h3>
          <p style="color:var(--tx2);font-size:13px;margin-bottom:14px">Cadastre a conta de acesso e atribua o papel inicial.</p>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Nº Identificação (CPF/ID) *</label>
              <input id="nuLogin" placeholder="${isAdminViewer ? 'ex.: 000.000.000-00' : 'gerado pelo sistema a partir do nome'}">
            </div>
            <div class="campo" style="flex:1">
              <label>Senha Inicial (opcional)</label>
              <input type="password" id="nuSenha" placeholder="Padrão: sci">
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Nome Completo *</label>
              <input id="nuCompleto" placeholder="ex.: Hermes da Silva">
            </div>
            <div class="campo" style="flex:1">
              <label>Nome de Guerra *</label>
              <input id="nuGuerra" placeholder="ex.: Silva">
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" style="flex:1">
              <label>Papel Inicial *</label>
              <select id="nuPapel">
                <option value="operador">Operador (Função Múltipla)</option>
                <option value="chefe_setor">Chefe de Setor (Conferência do Setor)</option>
                <option value="gerente">Gerente (Função Única por Grupo)</option>
                <option value="admin">Administrador (Global)</option>
              </select>
            </div>
            <div class="campo" style="flex:1">
              <label>Antiguidade (opcional)</label>
              <select id="nuFuncao">
                <option value="">— Nenhuma —</option>
                ${funcoesLista.map(f => `<option value="${f.id}">${esc(f.nome)}</option>`).join('')}
              </select>
              <div style="font-size:11.5px; color:var(--tx3); margin-top:4px">Grau de antiguidade para exibição e ordem em relatórios. Designação de Encarregados: módulo Pessoal › Aba Funções.</div>
            </div>
          </div>

          <div class="form-linha" style="margin-bottom:8px">
            <div class="campo" id="nuCampoGrupo" style="flex:1">
              <label>Grupo / Unidade (padrão: sem grupo)</label>
              <select id="nuGrupo"><option value="">— Sem grupo (padrão) —</option>${optsGrupos}</select>
            </div>
          </div>

          <div class="form-linha" id="nuLinhaSetor" style="margin-bottom:8px; display:none">
            <div class="campo" style="flex:1">
              <label>Setor / Seção (escopo do chefe de setor)</label>
              <select id="nuSetor"><option value="">— Nenhum —</option>${setoresLista.map(s => `<option value="${s.id}">${esc(s.nome)}${s.sigla ? ' (' + esc(s.sigla) + ')' : ''}</option>`).join('')}</select>
            </div>
          </div>

          <div id="nuAjudaPapel" style="font-size:12px; color:var(--tx2); background:var(--painel2); border:1px solid var(--borda); border-radius:6px; padding:10px; margin-bottom:14px">
            ℹ️ <b>Hierarquia (ordem 04/10):</b> contas criadas pelo admin nascem SEM grupo. O gerente designa os chefes de setor; cada chefe seleciona os operadores do seu setor.
          </div>

          <div class="modal-acoes">
            <button class="fantasma" id="nuX">Cancelar</button>
            <button class="primario" id="nuGo">Criar Conta</button>
          </div>
        </div>
      `;
      const div = modal(html);
      if (!div) return;

      const selP = div.querySelector('#nuPapel');
      const grpField = div.querySelector('#nuCampoGrupo');
      const linhaSetor = div.querySelector('#nuLinhaSetor');
      const ajuda = div.querySelector('#nuAjudaPapel');

      selP.onchange = () => {
        const p = selP.value;
        if (linhaSetor) linhaSetor.style.display = (p === 'chefe_setor') ? 'flex' : 'none';
        if (p === 'admin') {
          grpField.style.display = 'none';
          ajuda.innerHTML = 'ℹ️ <b>Administrador:</b> Acesso irrestrito a configurações globais, backup e governança da estrutura.';
        } else if (p === 'gerente') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🔒 <b>Gerente (Função Única):</b> Comandante da unidade. Cada grupo só pode ter 1 gerente titular ativo.';
        } else if (p === 'chefe_setor') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🏢 <b>Chefe de Setor:</b> Responsável pela conferência de faltas e presenças exclusivamente do seu próprio setor/efetivo.';
        } else {
          grpField.style.display = 'block';
          ajuda.innerHTML = 'ℹ️ <b>Operador (Função Múltipla):</b> Acesso operacional diário às conferências e caixa de email compartilhada do grupo.';
        }
      };

      div.querySelector('#nuX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#nuGo').onclick = async () => {
        const completo = div.querySelector('#nuCompleto').value.trim();
        const guerra = div.querySelector('#nuGuerra').value.trim();
        const papel = selP.value;
        // ITEM 2 (ordem Diretor 06/10): o campo de identificação não é digitado
        // nem exibido para não-admin — o sistema deriva o login do nome de guerra
        // (acentos/espaços normalizados). O admin continua digitando o seu.
        let login;
        if (isAdminViewer) {
          login = div.querySelector('#nuLogin').value.trim();
        } else {
          login = guerra.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase()
            .replace(/[^a-z0-9]+/g, '.').replace(/^\.+|\.+$/g, '') || 'conta' + Date.now();
        }
        const senha = div.querySelector('#nuSenha').value;
        // ordem 04/10: padrão é SEM grupo (admin vincula depois se quiser);
        // gerente continua exigindo unidade.
        const grupoId = +div.querySelector('#nuGrupo').value || null;
        const funcaoId = +div.querySelector('#nuFuncao').value || null;
        // onda itens79: setor vai no create quando papel = chefe_setor (hUsuariosAdd
        // JÁ grava usuarios.setor_id — server.go:4604). chefe sem setor pode escolher
        // depois; se selecionado, manda o id do catálogo.
        const setorSel = div.querySelector('#nuSetor');
        const setorId = (papel === 'chefe_setor' && setorSel && setorSel.value) ? (+setorSel.value || null) : null;

        if (!login || !completo || !guerra) {
          toast('Preencha os campos obrigatórios (*)', 'erro');
          return;
        }
        if (senha && senha.length > 0 && senha.length < 8) {
          toast('Se preencher a senha, use no mínimo 8 caracteres. (Padrão: sci)', 'erro');
          return;
        }
        if (papel === 'gerente' && !grupoId) {
          toast('Gerente deve obrigatoriamente estar vinculado a uma unidade', 'erro');
          return;
        }

        try {
          const res = await api('/api/usuarios', {
            method: 'POST',
            body: JSON.stringify({
              login,
              senha,
              papel,
              grupo_id: grupoId,
              funcao_id: funcaoId,
              setor_id: setorId
            })
          });
          if (res && res.id) {
            await api(`/api/usuarios/${res.id}`, {
              method: 'PATCH',
              body: JSON.stringify({
                nome_completo: completo,
                nome_guerra: guerra
              })
            });
            toast('Usuário criado com sucesso!');
            div.fechar && div.fechar();
            admUsuarios();
          }
        } catch (e) {}
      };
    }

    function modalAtribuirPapel(uid, login) {
      const tituloConta = isAdminViewer ? esc(login) : esc((lista.find(x => x.id === +uid) || {}).nome_guerra || 'usuário');
      const html = `
        <div class="modal" style="max-width:480px">
          <h3 style="margin-top:0">Atribuir Função / Cadeira — ${tituloConta}</h3>
          <p style="color:var(--tx2);font-size:13px;margin-bottom:14px">Permita que este usuário acumule funções adicionais em grupos específicos.</p>

          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="flex:1">
              <label>Papel / Responsabilidade *</label>
              <select id="apPapel">
                <option value="operador">Operador (Função Múltipla)</option>
                <option value="chefe_setor">Chefe de Setor (Conferência do Setor)</option>
                <option value="gerente">Gerente (Função Única por Grupo)</option>
                <option value="admin">Administrador (Global)</option>
              </select>
            </div>
            <div class="campo" id="apCampoGrupo" style="flex:1">
              <label>Grupo / Pelotão *</label>
              <select id="apGrupo">${optsGrupos}</select>
            </div>
          </div>

          <div class="form-linha" id="apLinhaSetor" style="margin-bottom:10px; display:none">
            <div class="campo" style="flex:1">
              <label>Setor / Seção (obrigatório p/ chefe de setor)</label>
              <select id="apSetor"><option value="">— Nenhum —</option>${setoresLista.map(s => `<option value="${s.id}" data-grupo="${s.grupo_id == null ? '' : s.grupo_id}">${esc(s.nome)}${s.sigla ? ' (' + esc(s.sigla) + ')' : ''}</option>`).join('')}</select>
            </div>
          </div>

          <div class="campo" style="margin-bottom:12px">
            <label>Antiguidade (opcional)</label>
            <select id="apFuncao">
              <option value="">— Nenhuma / Padrão —</option>
              ${funcoesLista.map(f => `<option value="${f.id}">${esc(f.nome)}</option>`).join('')}
            </select>
            <div style="font-size:11.5px; color:var(--tx3); margin-top:4px">Grau de antiguidade para exibição e ordem em relatórios. Designação de Encarregados: módulo Pessoal › Aba Funções.</div>
          </div>

          <div id="apAjuda" style="font-size:12px; color:var(--tx2); background:var(--painel2); border:1px solid var(--borda); border-radius:6px; padding:10px; margin-bottom:14px">
            ℹ️ <b>Operador:</b> O grupo pode comportar múltiplos operadores com acesso operacional e email compartilhado.
          </div>

          <div class="modal-acoes">
            <button class="fantasma" id="apX">Cancelar</button>
            <button class="primario" id="apGo">Atribuir Função</button>
          </div>
        </div>
      `;
      const div = modal(html);
      if (!div) return;

      const selP = div.querySelector('#apPapel');
      const grpField = div.querySelector('#apCampoGrupo');
      const linhaSetor = div.querySelector('#apLinhaSetor');
      const selSetor = div.querySelector('#apSetor');
      const selGrupo = div.querySelector('#apGrupo');
      const ajuda = div.querySelector('#apAjuda');

      // onda itens79: a linha de setor aparece só p/ chefe_setor e as opções
      // filtram pelo grupo escolhido (ou globais, grupo NULL). O backend
      // (hUsuarioPapelAdd) valida de novo — 400 se o setor não é do grupo.
      const filtrarSetores = () => {
        if (!selSetor) return;
        const gid = selGrupo.value;
        selSetor.querySelectorAll('option[data-grupo]').forEach(op => {
          const gOp = op.dataset.grupo;
          op.style.display = (!gid || gOp === '' || gOp === gid) ? '' : 'none';
        });
        if (selSetor.selectedOptions[0] && selSetor.selectedOptions[0].style.display === 'none') {
          selSetor.value = '';
        }
      };
      if (selGrupo) selGrupo.onchange = filtrarSetores;

      selP.onchange = () => {
        const p = selP.value;
        if (linhaSetor) {
          const mostrar = (p === 'chefe_setor');
          linhaSetor.style.display = mostrar ? 'flex' : 'none';
          if (mostrar) filtrarSetores();
        }
        if (p === 'admin') {
          grpField.style.display = 'none';
          ajuda.innerHTML = '🌐 <b>Administrador:</b> Acesso global ao sistema.';
        } else if (p === 'gerente') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🔒 <b>Gerente (Função Única):</b> Cada grupo só pode ter 1 gerente titular ativo. Se o grupo já tiver gerente, a operação será recusada.';
        } else if (p === 'chefe_setor') {
          grpField.style.display = 'block';
          ajuda.innerHTML = '🏢 <b>Chefe de Setor:</b> Permite ao usuário conduzir a conferência e marcar faltas/presenças de pessoas do seu setor.';
        } else {
          grpField.style.display = 'block';
          ajuda.innerHTML = '👥 <b>Operador (Função Múltipla):</b> O grupo pode comportar múltiplos operadores com acesso operacional compartilhado.';
        }
      };

      div.querySelector('#apX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#apGo').onclick = async () => {
        const papel = selP.value;
        const grupoId = +div.querySelector('#apGrupo').value || null;
        const funcaoId = +div.querySelector('#apFuncao').value || null;
        // onda itens79: chefe_setor pode nascer já com o setor (conveniência do
        // formulário; nomear_chefe continua sendo a via canônica).
        const setorId = (papel === 'chefe_setor' && selSetor && selSetor.value) ? (+selSetor.value || null) : null;

        if (papel !== 'admin' && !grupoId) {
          toast('Selecione o grupo para esta função', 'erro');
          return;
        }

        try {
          await api(`/api/usuarios/${uid}/papeis`, {
            method: 'POST',
            body: JSON.stringify({
              papel,
              grupo_id: grupoId,
              funcao_id: funcaoId,
              setor_id: setorId
            })
          });
          toast('Nova função atribuída com sucesso!');
          div.fechar && div.fechar();
          admUsuarios();
        } catch (e) {}
      };
    }

    function modalEditarNomes(uid, login, nomeGuerraAtual, nomeCompletoAtual) {
      const html = `
        <div class="modal" style="max-width:440px">
          <h3 style="margin-top:0">Editar Identificação — ${isAdminViewer ? esc(login) : esc(nomeGuerraAtual || nomeCompletoAtual || 'usuário')}</h3>
          <p style="color:var(--tx2);font-size:13px;margin-bottom:14px">O nome curto/de guerra aparecerá destacado nas assinaturas e caixas postais.</p>

          <div class="campo" style="margin-bottom:10px">
            <label>Nome Completo *</label>
            <input id="edCompleto" value="${esc(nomeCompletoAtual)}">
          </div>
          <div class="campo" style="margin-bottom:14px">
            <label>Nome de Guerra / Nome Curto *</label>
            <input id="edGuerra" value="${esc(nomeGuerraAtual)}">
          </div>

          <div class="modal-acoes">
            <button class="fantasma" id="edX">Cancelar</button>
            <button class="primario" id="edGo">Salvar Nomes</button>
          </div>
        </div>
      `;
      const div = modal(html);
      if (!div) return;

      div.querySelector('#edX').onclick = () => div.fechar && div.fechar();
      div.querySelector('#edGo').onclick = async () => {
        const comp = div.querySelector('#edCompleto').value.trim();
        const guer = div.querySelector('#edGuerra').value.trim();
        if (!comp || !guer) {
          toast('Nomes obrigatórios', 'erro');
          return;
        }
        try {
          await api(`/api/usuarios/${uid}`, {
            method: 'PATCH',
            body: JSON.stringify({
              nome_completo: comp,
              nome_guerra: guer
            })
          });
          toast('Nomes atualizados com sucesso!');
          div.fechar && div.fechar();
          admUsuarios();
        } catch (e) {}
      };
    }
  }

  /* --- GESTÃO DE GRUPOS & UNIDADES (Modais Compartilhados Admin & Gerente) --- */
  async function modalGerenciarSetoresGrupo(grupoId, grupoNome) {
    const html = `
      <div class="modal" style="max-width:560px;width:95%">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h3 style="margin:0">🏢 Setores da Unidade — ${esc(grupoNome)}</h3>
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">✕</button>
        </div>
        <p style="color:var(--tx2);font-size:12.5px;margin-bottom:14px">Cadastre e gerencie seções ou pelotões pertencentes exclusivamente a este grupo.</p>
        
        <div style="background:var(--painel2);border:1px solid var(--borda);border-radius:var(--raio);padding:12px;margin-bottom:14px">
          <h4 style="margin:0 0 8px;font-size:13px">Novo Setor</h4>
          <div class="form-linha" style="margin:0;gap:8px">
            <div class="campo" style="flex:2;margin:0">
              <label>Nome do Setor *</label>
              <input id="nSetNome" placeholder="ex.: 1º Pelotão, Almoxarifado...">
            </div>
            <div class="campo" style="flex:1;margin:0">
              <label>Sigla</label>
              <input id="nSetSigla" placeholder="ex.: 1º PEL">
            </div>
            <div class="campo" style="align-self:flex-end;margin:0">
              <button class="primario" id="btAddSetor" style="min-height:38px">Adicionar</button>
            </div>
          </div>
        </div>

        <div id="listaSetoresGrupo" style="max-height:280px;overflow-y:auto;border:1px solid var(--borda);border-radius:6px;padding:8px">
          <div class="carregando">Carregando setores...</div>
        </div>
        <div class="modal-acoes" style="margin-top:14px">
          <button class="fantasma" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;
    const m = modal(html);
    const listaEl = m.querySelector('#listaSetoresGrupo');

    async function carregarLista() {
      try {
        listaEl.innerHTML = '<div class="carregando">Atualizando...</div>';
        const todosSetores = await api('/api/catalogo/setores');
        const setoresGrupo = (todosSetores || []).filter(s => s.grupo_id === grupoId);
        if (setoresGrupo.length === 0) {
          listaEl.innerHTML = '<div class="vazio" style="padding:16px;text-align:center">Nenhum setor cadastrado neste grupo.</div>';
          return;
        }
        listaEl.innerHTML = `
          <table style="width:100%;font-size:12.5px">
            <thead><tr><th>Nome</th><th>Sigla</th><th style="text-align:right">Ações</th></tr></thead>
            <tbody>
              ${setoresGrupo.map(s => `
                <tr>
                  <td><b>${esc(s.nome)}</b></td>
                  <td>${esc(s.sigla || '—')}</td>
                  <td style="text-align:right">
                    <button class="acao-linha" data-editset="${s.id}" data-nome="${esc(s.nome)}" data-sigla="${esc(s.sigla || '')}">Editar</button>
                    <button class="acao-linha perigo" data-excset="${s.id}">Excluir</button>
                  </td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        `;

        listaEl.querySelectorAll('[data-editset]').forEach(bt => {
          bt.onclick = async () => {
            const sid = bt.dataset.editset;
            const novoNome = prompt('Novo nome do setor:', bt.dataset.nome);
            if (!novoNome || !novoNome.trim()) return;
            const novaSigla = prompt('Sigla (opcional):', bt.dataset.sigla);
            try {
              await api(`/api/catalogo/setores/${sid}`, {
                method: 'PATCH',
                body: JSON.stringify({ nome: novoNome.trim(), sigla: (novaSigla || '').trim() })
              });
              toast('Setor atualizado!');
              carregarLista();
            } catch (e) {
              toast(e.message || 'Falha ao atualizar', 'erro');
            }
          };
        });

        listaEl.querySelectorAll('[data-excset]').forEach(bt => {
          bt.onclick = async () => {
            const sid = bt.dataset.excset;
            if (!confirm('Deseja excluir este setor?')) return;
            try {
              await api(`/api/catalogo/setores/${sid}`, { method: 'DELETE' });
              toast('Setor excluído!');
              carregarLista();
            } catch (e) {
              toast(e.message || 'Falha ao excluir setor', 'erro');
            }
          };
        });
      } catch (e) {
        listaEl.innerHTML = '<div class="vazio">Erro ao carregar setores.</div>';
      }
    }

    carregarLista();

    m.querySelector('#btAddSetor').onclick = async () => {
      const nome = m.querySelector('#nSetNome').value.trim();
      const sigla = m.querySelector('#nSetSigla').value.trim();
      if (!nome) { toast('Nome do setor obrigatório', 'erro'); return; }
      try {
        await api('/api/catalogo/setores', {
          method: 'POST',
          body: JSON.stringify({ nome, sigla, grupo_id: grupoId })
        });
        toast('Setor adicionado ao grupo!');
        m.querySelector('#nSetNome').value = '';
        m.querySelector('#nSetSigla').value = '';
        carregarLista();
      } catch (e) {
        toast(e.message || 'Falha ao cadastrar setor', 'erro');
      }
    };
  }


  async function modalAlocarUsuariosGrupo(gid, gnome, contas, aoFim) {
    let cList = contas;
    if (!cList || !cList.length) {
      try { cList = await api('/api/usuarios'); } catch (e) { cList = []; }
    }
    const outrasContas = (cList || []).filter(c => c.papel !== 'admin' && c.ativo && c.grupo_id !== gid);
    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0">👥 Alocar Usuário — ${esc(gnome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">Mover um usuário existente para integrar este grupo.</p>
        
        <div class="campo" style="margin-bottom:16px">
          <label>Selecione o usuário</label>
          <select id="mSelAlocarConta">
            <option value="">— Selecione um usuário cadastrado —</option>
            ${outrasContas.map(c => `<option value="${c.id}">${esc(c.login)} (${rotuloPapel(c.papel)}) - ${esc(c.nome_guerra || c.login)} [${c.grupo_id ? 'Grupo #' + c.grupo_id : 'Sem grupo'}]</option>`).join('')}
          </select>
        </div>

        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
          <button class="primario" id="mBtnAlocarGo">Alocar no Grupo</button>
        </div>
      </div>
    `;
    const m = modal(html);
    m.querySelector('#mBtnAlocarGo').onclick = async () => {
      const uid = m.querySelector('#mSelAlocarConta').value;
      if (!uid) { toast('Selecione um usuário', 'erro'); return; }
      try {
        await api(`/api/usuarios/${uid}/mover`, {
          method: 'PATCH',
          body: JSON.stringify({ grupo_id: gid })
        });
        toast('Usuário alocado na unidade com sucesso!');
        m.remove();
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };
  }

  async function modalTrocarGerente(gid, gnome, contas, aoFim) {
    let cList = contas;
    if (!cList || !cList.length) {
      try { cList = await api('/api/usuarios'); } catch (e) { cList = []; }
    }
    // Ordem Diretor 04/10: gerente pode ser QUALQUER conta indicada pelo admin —
    // não apenas quem já está alocado ao grupo (o backend vincula na troca).
    const contasGrupo = (cList || []).filter(c => c.papel !== 'admin' && c.ativo);
    const temGerente = contasGrupo.some(c => c.papel === 'gerente');
    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0">👤 ${temGerente ? 'Trocar Gerente' : 'Definir Gerente'} — ${esc(gnome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">
          ${temGerente
            ? 'Selecione qualquer conta ativa para assumir a titularidade — de dentro ou de fora do grupo (ordem do Diretor). O gerente atual passará a operador.'
            : 'Selecione qualquer conta ativa para ser o Gerente titular — o vínculo com o grupo é feito automaticamente.'}
        </p>

        ${contasGrupo.length === 0 ? `
          <div class="vazio" style="padding:20px; text-align:center; margin-bottom:14px">
            Nenhuma conta ativa disponível.
          </div>
        ` : `
          <div class="campo" style="margin-bottom:14px">
            <label>Conta a assumir a gerência</label>
            <select id="mSelNovaConta">
              <option value="">— Selecione uma conta —</option>
              ${contasGrupo.map(c => { const atual = c.papel === 'gerente' && c.grupo_id === gid; return `<option value="${esc(c.login)}" ${atual ? 'disabled' : ''}>${esc(c.login)} (${rotuloPapel(c.papel)}) - ${esc(c.nome_guerra || c.login)} ${atual ? '★ Atual Gerente' : ''}</option>`; }).join('')}
            </select>
          </div>
        `}

        <div style="display:flex; justify-content:space-between; align-items:center">
          ${temGerente ? `<button type="button" class="perigo acao-linha" id="mBtnRemoverGer" style="padding:4px 8px">Remover Gerente</button>` : `<div></div>`}
          <div style="display:flex; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            ${contasGrupo.length > 0 ? `<button class="primario" id="mBtnTrocarGer">Promover a Gerente</button>` : ''}
          </div>
        </div>
      </div>
    `;
    const m = modal(html);
    const btnRem = m.querySelector('#mBtnRemoverGer');
    if (btnRem) {
      btnRem.onclick = async () => {
        try {
          await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login: "__REMOVE__" }) });
          toast('Gerência vaga com sucesso.');
          m.remove();
          if (aoFim) aoFim();
          else if (window.ViewAdmin) window.ViewAdmin();
        } catch (e) {}
      };
    }
    const btn = m.querySelector('#mBtnTrocarGer');
    if (btn) {
      btn.onclick = async () => {
        const login = m.querySelector('#mSelNovaConta').value;
        if (!login) { toast('Selecione a conta para promover', 'erro'); return; }
        try {
          await api(`/api/grupos/${gid}/trocar-gerente`, { method: 'POST', body: JSON.stringify({ login }) });
          toast('Gerente atribuído com sucesso! Função herdada.');
          m.remove();
          if (aoFim) aoFim();
          else if (window.ViewAdmin) window.ViewAdmin();
        } catch (e) {}
      };
    }
  }

  async function modalGerenciarSubordinacao(gid, gnome, grupos, aoFim) {
    let gList = grupos;
    if (!gList || !gList.length) {
      try { gList = await api('/api/grupos'); } catch (e) { gList = []; }
    }
    const gAtual = (gList || []).find(x => x.id === gid);
    const sups = (gAtual && gAtual.superiores_ids) || [];
    const supAtualId = sups[0] || null;

    const html = `
      <div class="modal" style="max-width:500px">
        <h3 style="margin-top:0">⛓️ Subordinação — ${esc(gnome)}</h3>
        <p style="color:var(--tx2); font-size:13px; margin-bottom:12px">Defina a qual unidade superior este grupo reporta na cadeia de comando.</p>

        <div class="campo" style="margin-bottom:14px">
          <label>Unidade Superior (Comando Direto)</label>
          <select id="mSelSuperior">
            <option value="">— Sem Superior (Nível Raiz) —</option>
            ${(gList || []).filter(x => x.id !== gid).map(x => `<option value="${x.id}" ${x.id === supAtualId ? 'selected' : ''}>${esc(x.nome)} (#${x.codigo})</option>`).join('')}
          </select>
        </div>

        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:16px">
          ${supAtualId ? `<button type="button" class="perigo" id="mBtnRemoverSub">Desvincular Superior</button>` : '<div></div>'}
          <div style="display:flex; gap:8px">
            <button class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button class="primario" id="mBtnSalvarSub">Salvar Subordinação</button>
          </div>
        </div>
      </div>
    `;
    const m = modal(html);

    m.querySelector('#mBtnSalvarSub').onclick = async () => {
      const novoSupId = +m.querySelector('#mSelSuperior').value;
      try {
        if (supAtualId && supAtualId !== novoSupId) {
          await api(`/api/admin/grupos/vinculo?superior_id=${supAtualId}&subordinado_id=${gid}`, { method: 'DELETE', body: '{}' });
        }
        if (novoSupId > 0) {
          await api('/api/admin/grupos/vinculo', { method: 'POST', body: JSON.stringify({ superior_id: novoSupId, subordinado_id: gid }) });
        }
        toast('Subordinação atualizada');
        m.remove();
        if (aoFim) aoFim();
        else if (window.ViewAdmin) window.ViewAdmin();
      } catch (e) {}
    };

    if (m.querySelector('#mBtnRemoverSub')) {
      m.querySelector('#mBtnRemoverSub').onclick = async () => {
        try {
          await api(`/api/admin/grupos/vinculo?superior_id=${supAtualId}&subordinado_id=${gid}`, { method: 'DELETE', body: '{}' });
          toast('Subordinação removida');
          m.remove();
          if (aoFim) aoFim();
          else if (window.ViewAdmin) window.ViewAdmin();
        } catch (e) {}
      };
    }
  }

  async function modalAuditarContas(gid, nome, contas) {
    let cList = contas;
    if (!cList || !cList.length) {
      try { cList = await api('/api/usuarios'); } catch (e) { cList = []; }
    }
    const cGrupo = (cList || []).filter(c => c.grupo_id === gid);
    const html = `
      <div class="modal" style="max-width:800px; max-height:85vh; display:flex; flex-direction:column">
        <h3 style="margin-top:0">📊 Contas & Usuários: ${esc(nome)}</h3>
        <div style="overflow-y:auto; flex:1">
          <table style="width:100%; margin-top:8px">
            <thead><tr><th>ID</th><th>Login</th><th>Nome de Guerra</th><th>Papel</th><th>Status</th></tr></thead>
            <tbody>
              ${cGrupo.length ? cGrupo.map(c => `
                <tr>
                  <td>#${c.id}</td>
                  <td><b>${esc(c.login)}</b></td>
                  <td>${esc(c.nome_guerra || '—')}</td>
                  <td>${rotuloPapel(c.papel)}</td>
                  <td><span style="color:${c.ativo ? 'var(--verde-claro)' : 'var(--tx3)'}; font-weight:700">${c.ativo ? 'Ativa' : 'Desativada'}</span></td>
                </tr>
              `).join('') : '<tr><td colspan="5"><span class="vazio">Nenhuma conta cadastrada neste grupo.</span></td></tr>'}
            </tbody>
          </table>
        </div>
        <div style="margin-top:16px; text-align:right; border-top:1px solid var(--borda); padding-top:10px">
          <button class="primario" onclick="this.closest('.modal-mask').remove()">Fechar</button>
        </div>
      </div>
    `;
    modal(html);
  }




  /* --- ADMIN › grupos: árvore nested colapsável + foco recursivo + ações completas --- */
  async function admGrupos() {
    const [grupos, contas, arvore] = await Promise.all([api('/api/grupos'), api('/api/usuarios'), api('/api/grupos/arvore')]);
    const gerenteDe = {};
    contas.filter(c => c.papel === 'gerente' && c.ativo && c.grupo_id).forEach(c => { gerenteDe[c.grupo_id] = c.login; });
    marcarGerente(arvore, gerenteDe);

    const renderConteudo = () => {
      $('#adm').innerHTML = `
        <div class="cartao">
          <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-bottom:14px">
            <div>
              <h3 style="margin:0 0 4px">Estrutura & Hierarquia Organizacional</h3>
              <p style="color:var(--tx2); font-size:13px; margin:0">Navegue, expanda e foque em unidades e subgrupos recursivamente.</p>
            </div>
            <div style="display:flex; gap:8px; flex-wrap:wrap">
              <button class="acao-linha" id="btRetornarTopo" title="Retornar à visão do topo/raiz">⬆️ Retornar ao Topo</button>
              <button class="primario" id="btNovoGrupoRaiz">+ Novo Grupo Raiz</button>
            </div>
          </div>

          <div class="campo" style="margin-bottom:14px">
            <input id="fGNome" placeholder="Filtrar por nome do grupo ou código (#ABC123)…">
          </div>

          <div id="arvoreAdm" style="display:flex; flex-direction:column; gap:2px">
            ${arvoreHTML(arvore, true)}
          </div>
        </div>
      `;

      ligarEventosArvore();
    };

    const ligarEventosArvore = () => {
      ligarToggles($('#arvoreAdm'));

      // Botões de focar em um nó
      $('#arvoreAdm').querySelectorAll('[data-focargrupo]').forEach(b => {
        b.onclick = (e) => {
          e.stopPropagation();
          const gid = +b.dataset.focargrupo;
          FOCO_GRUPO_ID = gid > 0 ? gid : null;
          renderConteudo();
        };
      });

      // Retornar ao Topo (Admin vai para a raiz geral; Gerente/Usuário com grupo vai para o seu grupo raiz)
      if ($('#btRetornarTopo')) {
        $('#btRetornarTopo').onclick = () => {
          const eu = quem();
          if (eu && eu.papel !== 'admin' && eu.grupo_id) {
            FOCO_GRUPO_ID = eu.grupo_id;
          } else {
            FOCO_GRUPO_ID = null; // Admin ou sem grupo retorna à raiz
          }
          renderConteudo();
        };
      }

      // Botão de Novo Grupo Raiz
      $('#btNovoGrupoRaiz').onclick = () => modalCriarGrupo(null, null, () => window.ViewAdmin());

      // Botão Consolidado "Editar" (abre modal de gestão da unidade)
      $('#arvoreAdm').querySelectorAll('[data-editargrupo]').forEach(b => {
        b.onclick = (e) => {
          e.stopPropagation();
          const gid = +b.dataset.editargrupo;
          const no = buscarNoPorId(arvore, gid);
          if (no) modalEditarGrupo(no, () => window.ViewAdmin(), { grupos, contas });
        };
      });

      // Busca na árvore
      $('#fGNome').oninput = () => {
        const q = $('#fGNome').value.trim().toLowerCase();
        if (!q) {
          $('#arvoreAdm').innerHTML = arvoreHTML(arvore, true);
          ligarEventosArvore();
          return;
        }
        const mostrar = n => {
          const eu = n.nome.toLowerCase().includes(q) || (n.codigo || '').toLowerCase().includes(q);
          const filhos = (n.filhos || []).map(f => mostrar(f)).filter(Boolean);
          if (!eu && !filhos.length) return null;
          return { ...n, filhos: filhos.length ? filhos : (n.filhos || []) };
        };
        const filtrada = (arvore || []).map(mostrar).filter(Boolean);
        $('#arvoreAdm').innerHTML = arvoreHTML(filtrada, true);
        $('#arvoreAdm').querySelectorAll('.subgrupos-container').forEach(d => { d.style.display = 'block'; });
        $('#arvoreAdm').querySelectorAll('[data-tgl]').forEach(s => { s.textContent = '▾'; });
        ligarEventosArvore();
      };
    };

    renderConteudo();
  }

  /* --- ADMIN › backup: telemetria em tempo real + gerar download .db + IMPORTAR upload com confirmação --- */

  /* =====================================================================
     #/grupos — GERENCIAR (gerente): Pessoal · Tags · Grupos · Operadores
     ===================================================================== */

  /* =====================================================================
     #/perfil — PERFIL DO USUÁRIO & FOTO 1X1 (disponível para todos os perfis)
     ===================================================================== */

  window.admDashboard = admDashboard;
  // exports (referenciados por outros módulos / router)
  window.TBL_ESTADO = TBL_ESTADO;
  window.ativosDe = ativosDe;
  window.eLinhaVazio = eLinhaVazio;
  window.quem = quem;
  window.rotuloPapel = rotuloPapel;
  window.tabelaControles = tabelaControles;
  window.tblLinhas = tblLinhas;
  window.tblOrdenar = tblOrdenar;
})();
