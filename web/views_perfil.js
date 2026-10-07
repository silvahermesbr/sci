/* SCI — views de GESTÃO (redesign): #/admin · #/grupos (Gerenciar) · #/perfil
   window.ViewAdmin / window.ViewGrupos / window.ViewPerfil (async).
   Consome os helpers globais do core: api, esc, toast, fmtData, fmtHora, pill,
   abrirModal (insere .modal-mask no body e devolve o elemento), confirmar(msg) →
   Promise<boolean>, navAtiva(hash). Vanilla, sem build, sem CDN.
   Comportamento copiado do protótipo web/app.js (v9.11.2) — nada pode sumir. */
/* [FATIADO da views_gestao.js — onda de modularização; recorte puro] */
'use strict';
  const $ = (s) => document.querySelector(s);
(function () {
'use strict';
  window.ViewPerfil = async function () {
    const eu = quem();
    if (!eu) { location.hash = '#/login'; return; }
    navAtiva('#/perfil');
    $('#app').innerHTML = '<div class="carregando">Carregando perfil…</div>';
    
    let d;
    try {
      d = await api('/api/perfil');
    } catch (e) {
      $('#app').innerHTML = '<div class="vazio">Falha ao carregar perfil.</div>';
      return;
    }

    const u = d.usuario;
    let fotoAtual = u.foto_base64 || '';
    const sangueOpts = ["", "A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"];

    $('#app').innerHTML = `
      <div style="margin-bottom:18px">
        <h2 style="margin:0 0 4px">Meu Perfil de Usuário</h2>
        <p style="color:var(--tx2);font-size:13px;margin:0">Informações cadastrais, identificação institucional e foto de perfil 1x1.</p>
      </div>

      <div class="perfil-card">
        <!-- Topo: Foto 1x1 e Resumo de Identificação -->
        <div class="perfil-topo">
          <div class="perfil-foto-wrapper">
            <div class="perfil-foto-preview" id="pfFotoBox">
              ${fotoAtual ? `<img src="${fotoAtual}" alt="Foto 1x1">` : `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`}
            </div>
          </div>
          <div class="perfil-foto-acoes">
            <div style="font-size:17px; font-weight:800; color:var(--tx)">
              ${esc(u.nome_guerra || u.login)}
              ${u.tipo_sanguineo ? `<span style="font-size:11.5px; margin-left:6px; background:rgba(239,68,68,0.18); color:var(--verm-txt); border:1px solid rgba(239,68,68,0.35); padding:2px 7px; border-radius:10px; font-weight:700">🩸 ${esc(u.tipo_sanguineo)}</span>` : ''}
            </div>
            <div style="font-size:12.5px; color:var(--tx3)">
              ${esc(u.nome_completo || 'Nome completo não informado')}
            </div>
            <div style="display:flex; gap:8px; align-items:center; margin-top:4px">
              <input type="file" id="pfInpFile" accept="image/*" style="display:none">
              <button type="button" class="primario" id="pfBtAlterarFoto" style="font-size:12px; padding:6px 14px">📷 Alterar Foto 1x1</button>
              ${fotoAtual ? `<button type="button" class="acao-linha perigo" id="pfBtRemoverFoto" style="font-size:12px; padding:6px 12px">Remover Foto</button>` : ''}
            </div>
            <small style="color:var(--tx3); font-size:11px">Foto 1x1: o sistema recorta e compacta centralizadamente no seu navegador.</small>
          </div>
        </div>

        <!-- Seção 1: Identificação no Sistema (Leitura) -->
        <div class="perfil-secao-tit">1. Identificação no Sistema</div>
        <div class="perfil-grid-campos">
          <div class="campo">
            <label>ID do Usuário</label>
            <input value="#${u.id}" disabled style="background:var(--painel3); font-weight:700">
          </div>
          <div class="campo">
            <label>Login de Acesso</label>
            <input value="${esc(u.login)}" disabled style="background:var(--painel3); font-weight:700">
          </div>
          <div class="campo">
            <label>Função / Papel na Conta</label>
            <input value="${esc(rotuloPapel(u.papel))}" disabled style="background:var(--painel3)">
          </div>
          <div class="campo">
            <label>Unidade / Grupo</label>
            <input value="${esc(d.grupo || 'Global / Sem grupo')}" disabled style="background:var(--painel3)">
          </div>
        </div>

        <!-- Seção 2: Dados Pessoais & Militares -->
        <div class="perfil-secao-tit">2. Dados Pessoais & Militares</div>
        <div class="perfil-grid-campos">
          <div class="campo">
            <label>NOME (Nome de Guerra) *</label>
            <input id="pfNg" value="${esc(u.nome_guerra || '')}" placeholder="ex.: SILVA">
          </div>
          <div class="campo">
            <label>NOME COMPLETO *</label>
            <input id="pfNc" value="${esc(u.nome_completo || '')}" placeholder="Nome civil completo">
          </div>
          <div class="campo">
            <label>DATA NASC (Data de Nascimento)</label>
            <input id="pfDataNasc" type="date" value="${esc(u.data_nascimento || '')}">
          </div>
          <div class="campo">
            <label>TIPO SANGUÍNEO</label>
            <select id="pfTipoSang">
              <option value="">Não informado</option>
              ${sangueOpts.filter(Boolean).map(s => `<option value="${s}" ${u.tipo_sanguineo === s ? 'selected' : ''}>${s}</option>`).join('')}
            </select>
          </div>
        </div>

        <!-- Seção 3: Comunicação & Endereço -->
        <div class="perfil-secao-tit">3. Comunicação & Endereço</div>
        <div class="perfil-grid-campos">
          <div class="campo">
            <label>TELEFONE (Celular / WhatsApp)</label>
            <input id="pfTel" type="tel" value="${esc(u.telefone || '')}" placeholder="(XX) XXXXX-XXXX">
          </div>
          <div class="campo">
            <label>EMAIL (Institucional ou Pessoal)</label>
            <input id="pfEmail" type="email" value="${esc(u.email || '')}" placeholder="usuario@dominio.eb.mil.br">
          </div>
        </div>
        <div class="campo" style="margin-top:10px">
          <label>ENDEREÇO (Residencial / Contato)</label>
          <input id="pfEndereco" value="${esc(u.endereco || '')}" placeholder="Logradouro, número, complemento, bairro, cidade - UF">
        </div>

        <!-- Botões de Ação -->
        <div style="display:flex; justify-content:space-between; align-items:center; margin-top:24px; padding-top:16px; border-top:1px solid var(--borda); flex-wrap:wrap; gap:10px">
          <button type="button" class="acao-linha" id="pfBtMudarSenha">🔑 Alterar Minha Senha</button>
          <div style="display:flex; gap:10px">
            <button type="button" class="primario" id="pfGo" style="padding:10px 22px; font-weight:700">💾 Salvar Perfil</button>
          </div>
        </div>
      </div>
    `;

    // Eventos de Foto 1x1
    const fotoBox = $('#pfFotoBox');
    const inpFile = $('#pfInpFile');
    $('#pfBtAlterarFoto').onclick = () => inpFile.click();

    inpFile.onchange = (e) => {
      const file = e.target.files && e.target.files[0];
      if (file) {
        processarFoto1x1(file, (dataUrl) => {
          fotoAtual = dataUrl;
          fotoBox.innerHTML = `<img src="${fotoAtual}" alt="Foto 1x1">`;
          toast('Foto 1x1 processada. Clique em "Salvar Perfil" para confirmar.', 'ok');
        });
      }
    };

    const btRemFoto = $('#pfBtRemoverFoto');
    if (btRemFoto) {
      btRemFoto.onclick = () => {
        fotoAtual = '';
        fotoBox.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5"/></svg>`;
        btRemFoto.remove();
        toast('Foto removida. Clique em "Salvar Perfil" para confirmar.');
      };
    }

    // Modal de Senha (ordem Diretor 04/10/26: atual + nova + confirmação antes do save).
    // window.modalSenha = versão do core.js (preflight em /api/senha); o modalSenha local
    // daqui é o de REDEFINIÇÃO por admin/gerente (POST /api/usuarios/{id}/senha) — chamar
    // o local sem id era o bug do "ID não reconhecido" no Meu Perfil.
    $('#pfBtMudarSenha').onclick = () => window.modalSenha();

    // Salvar Perfil
    $('#pfGo').onclick = async () => {
      const ng = $('#pfNg').value.trim();
      const nc = $('#pfNc').value.trim();
      const dataNasc = $('#pfDataNasc').value;
      const tipoSang = $('#pfTipoSang').value;
      const tel = $('#pfTel').value.trim();
      const email = $('#pfEmail').value.trim();
      const endr = $('#pfEndereco').value.trim();

      if (!ng || !nc) {
        toast('Nome de guerra e nome completo são obrigatórios', 'erro');
        return;
      }

      $('#pfGo').disabled = true;
      try {
        await api('/api/perfil', {
          method: 'PATCH',
          body: JSON.stringify({
            nome_guerra: ng,
            nome_completo: nc,
            data_nascimento: dataNasc,
            tipo_sanguineo: tipoSang,
            telefone: tel,
            email: email,
            endereco: endr,
            foto_base64: fotoAtual
          })
        });

        // Atualiza o estado em memória local ME
        const atual = quem();
        if (atual) {
          atual.nome_guerra = ng;
          atual.nome_completo = nc;
          atual.data_nascimento = dataNasc;
          atual.tipo_sanguineo = tipoSang;
          atual.telefone = tel;
          atual.email = email;
          atual.endereco = endr;
          atual.foto_base64 = fotoAtual;
        }

        // Atualiza avatar e nome na Sidebar em tempo real
        const sbAvatar = document.getElementById('sbAvatarWrapper');
        if (sbAvatar) {
          if (fotoAtual) {
            sbAvatar.innerHTML = `<img src="${fotoAtual}" class="sidebar-avatar-img" alt="Foto">`;
          } else {
            sbAvatar.innerHTML = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle cx="12" cy="8" r="4" stroke="currentColor" stroke-width="2"/><path d="M4.5 20c1.4-3.2 4.2-5 7.5-5s6.1 1.8 7.5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`;
          }
        }
        const sbNome = document.querySelector('.sidebar-usuario-nome');
        if (sbNome) sbNome.textContent = ng || atual.login;

        toast('Perfil salvo com sucesso!');
        $('#pfGo').disabled = false;
        window.ViewPerfil();
      } catch (e) {
        $('#pfGo').disabled = false;
      }
    };
  };

  // exports (referenciados por outros módulos / router)
  window.ViewPerfil = ViewPerfil;
})();
