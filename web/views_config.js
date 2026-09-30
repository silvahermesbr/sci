(function () {
  'use strict';

  /* =====================================================================
     #/configuracoes — MÓDULO DE CONFIGURAÇÕES & WHITE-LABEL (v1.0)
     ===================================================================== */

  const quem = () => (typeof ME !== 'undefined' && ME) || window.ME || null;

  window.ViewConfiguracoes = async function () {
    const eu = quem();
    if (!eu || eu.papel !== 'admin') { location.hash = '#/hoje'; return; }
    navAtiva('#/configuracoes');

    const app = document.getElementById('app');
    app.innerHTML = `
      <div style="margin-bottom:16px">
        <h2 style="margin:0 0 4px">Configurações & Personalização do Sistema</h2>
        <p style="color:var(--tx2); font-size:13px; margin:0">Personalize títulos, termos institucionais e identidade visual (White-Label).</p>
      </div>

      <div id="cfgFormCont"><div class="carregando">Carregando configurações…</div></div>
    `;

    try {
      const res = await api('/api/configuracoes');
      const cfg = res.configuracoes || {};
      renderFormConfig(cfg);
    } catch (e) {
      $('#cfgFormCont').innerHTML = '<span class="vazio">Falha ao carregar configurações.</span>';
    }
  };

  function renderFormConfig(c) {
    const cont = $('#cfgFormCont');
    if (!cont) return;

    cont.innerHTML = `
      <div style="display:flex; flex-direction:column; gap:16px; max-width:800px">
        
        <!-- 1. Identidade -->
        <div class="cartao">
          <h3 style="margin-top:0">🏢 Identidade da Organização / Unidade</h3>
          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="max-width:180px">
              <label>Sigla do Sistema</label>
              <input id="c_NOME_SISTEMA" value="${esc(c.NOME_SISTEMA || 'SCI')}">
            </div>
            <div class="campo" style="flex:1">
              <label>Subtítulo / Descrição</label>
              <input id="c_SUBTITULO_SISTEMA" value="${esc(c.SUBTITULO_SISTEMA || 'Controle Interno')}">
            </div>
          </div>
          <div class="campo">
            <label>Nome / Título da Organização Militar ou Instituição</label>
            <input id="c_TITULO_ORGANIZACAO" value="${esc(c.TITULO_ORGANIZACAO || '3º B Com GE')}" placeholder="ex.: 3º Batalhão de Comunicações, 15º BPM, Hospital Central">
          </div>
        </div>

        <!-- 2. Nomenclaturas & Rótulos -->
        <div class="cartao">
          <h3 style="margin-top:0">🏷️ Nomenclaturas Personalizadas (Adaptação Institucional)</h3>
          <p style="color:var(--tx2); font-size:12.5px; margin-bottom:12px">Adapte os termos do sistema para a realidade da sua instituição (Exército, Polícia, Saúde ou Corporativo).</p>
          
          <div class="form-linha" style="margin-bottom:10px">
            <div class="campo" style="flex:1">
              <label>Termo para Grupos Maiores</label>
              <input id="c_ROTULO_GRUPO" value="${esc(c.ROTULO_GRUPO || 'Companhia / Subunidade')}" placeholder="ex.: Companhia, Departamento, Batalhão">
            </div>
            <div class="campo" style="flex:1">
              <label>Termo para Setores / Subgrupos</label>
              <input id="c_ROTULO_SETOR" value="${esc(c.ROTULO_SETOR || 'Pelotão / Seção')}" placeholder="ex.: Pelotão, Divisão, Ala">
            </div>
          </div>

          <div class="form-linha">
            <div class="campo" style="flex:1">
              <label>Termo para Membros do Efetivo</label>
              <input id="c_ROTULO_PESSOA" value="${esc(c.ROTULO_PESSOA || 'Militar')}" placeholder="ex.: Militar, Policial, Servidor, Funcionário">
            </div>
            <div class="campo" style="flex:1">
              <label>Termo para Identificador Principal</label>
              <input id="c_ROTULO_IDENTIFICADOR" value="${esc(c.ROTULO_IDENTIFICADOR || 'Nome de Guerra')}" placeholder="ex.: Nome de Guerra, Crachá, Matrícula">
            </div>
          </div>
        </div>

        <!-- 3. Identidade Visual & Cores -->
        <div class="cartao">
          <h3 style="margin-top:0">🎨 Identidade Visual & Cores do Tema</h3>
          
          <div style="margin-bottom:12px">
            <label style="font-weight:700; font-size:12.5px">Temas Pré-configurados (Clique para aplicar):</label>
            <div style="display:flex; gap:8px; margin-top:6px; flex-wrap:wrap">
              <button type="button" class="acao-linha" id="thmVerde" style="border-left:4px solid #57a173">🟢 Verde Militar (Padrão)</button>
              <button type="button" class="acao-linha" id="thmAzul" style="border-left:4px solid #4a8cdb">🔵 Azul Tático / Segurança</button>
              <button type="button" class="acao-linha" id="thmVermelho" style="border-left:4px solid #d9655b">🔴 Vermelho / Bombeiros</button>
              <button type="button" class="acao-linha" id="thmSlate" style="border-left:4px solid #78909c">⚫ Grafite / Corporativo</button>
            </div>
          </div>

          <div class="form-linha" style="align-items:flex-end">
            <div class="campo">
              <label>Cor Primária</label>
              <div style="display:flex; gap:6px; align-items:center">
                <input type="color" id="pickerCor" value="${c.COR_PRIMARIA || '#57a173'}" style="width:42px; height:40px; padding:2px; cursor:pointer">
                <input id="c_COR_PRIMARIA" value="${c.COR_PRIMARIA || '#57a173'}" style="width:110px">
              </div>
            </div>
            <div class="campo">
              <label>Acento Claro</label>
              <input id="c_COR_PRIMARIA_CLARO" value="${c.COR_PRIMARIA_CLARO || '#8fd2a9'}" style="width:110px">
            </div>
            <div class="campo">
              <label>Acento Escuro</label>
              <input id="c_COR_PRIMARIA_ESCURO" value="${c.COR_PRIMARIA_ESCURO || '#275e42'}" style="width:110px">
            </div>
          </div>
        </div>

        <!-- Botão Salvar -->
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <button class="primario" id="btSalvarConfig" style="font-size:15px; padding:10px 24px">
            💾 Salvar Configurações
          </button>
        </div>

      </div>
    `;

    // Sincronizar color picker
    $('#pickerCor').oninput = ev => {
      const val = ev.target.value;
      $('#c_COR_PRIMARIA').value = val;
    };

    // Presets
    $('#thmVerde').onclick = () => {
      $('#c_COR_PRIMARIA').value = '#57a173';
      $('#pickerCor').value = '#57a173';
      $('#c_COR_PRIMARIA_CLARO').value = '#8fd2a9';
      $('#c_COR_PRIMARIA_ESCURO').value = '#275e42';
    };
    $('#thmAzul').onclick = () => {
      $('#c_COR_PRIMARIA').value = '#4a8cdb';
      $('#pickerCor').value = '#4a8cdb';
      $('#c_COR_PRIMARIA_CLARO').value = '#8ec2ff';
      $('#c_COR_PRIMARIA_ESCURO').value = '#1d4a82';
    };
    $('#thmVermelho').onclick = () => {
      $('#c_COR_PRIMARIA').value = '#d9655b';
      $('#pickerCor').value = '#d9655b';
      $('#c_COR_PRIMARIA_CLARO').value = '#ffa099';
      $('#c_COR_PRIMARIA_ESCURO').value = '#75251e';
    };
    $('#thmSlate').onclick = () => {
      $('#c_COR_PRIMARIA').value = '#78909c';
      $('#pickerCor').value = '#78909c';
      $('#c_COR_PRIMARIA_CLARO').value = '#b0bec5';
      $('#c_COR_PRIMARIA_ESCURO').value = '#37474f';
    };

    // Salvar
    $('#btSalvarConfig').onclick = async () => {
      const payload = {
        NOME_SISTEMA: $('#c_NOME_SISTEMA').value.trim(),
        SUBTITULO_SISTEMA: $('#c_SUBTITULO_SISTEMA').value.trim(),
        TITULO_ORGANIZACAO: $('#c_TITULO_ORGANIZACAO').value.trim(),
        ROTULO_GRUPO: $('#c_ROTULO_GRUPO').value.trim(),
        ROTULO_SETOR: $('#c_ROTULO_SETOR').value.trim(),
        ROTULO_PESSOA: $('#c_ROTULO_PESSOA').value.trim(),
        ROTULO_IDENTIFICADOR: $('#c_ROTULO_IDENTIFICADOR').value.trim(),
        COR_PRIMARIA: $('#c_COR_PRIMARIA').value.trim(),
        COR_PRIMARIA_CLARO: $('#c_COR_PRIMARIA_CLARO').value.trim(),
        COR_PRIMARIA_ESCURO: $('#c_COR_PRIMARIA_ESCURO').value.trim(),
        MODO_RESERVA: '1' // ordem Tenente 30/09: escala/material em reserva (reativar = '0')
      };

      try {
        await api('/api/configuracoes', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        toast('Configurações salvas e aplicadas!');
        if (window.aplicarConfiguracoes) {
          window.aplicarConfiguracoes(payload);
        }
      } catch (e) {}
    };
  }

})();
