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
            <input id="c_TITULO_ORGANIZACAO" value="${esc(c.TITULO_ORGANIZACAO || 'Organização')}" placeholder="ex.: 3º Batalhão de Comunicações, 15º BPM, Hospital Central">
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
          
          <div style="margin-bottom:14px">
            <label style="font-weight:700; font-size:12.5px">Temas Pré-configurados (Clique para aplicar):</label>
            <div style="display:flex; gap:8px; margin-top:8px; flex-wrap:wrap">
              <button type="button" class="acao-linha" id="thmVerde" style="border-left:4px solid #10b981">🟢 Verde Esmeralda (Padrão)</button>
              <button type="button" class="acao-linha" id="thmAzul" style="border-left:4px solid #3b82f6">🔵 Azul Tático / Segurança</button>
              <button type="button" class="acao-linha" id="thmVermelho" style="border-left:4px solid #ef4444">🔴 Vermelho / Emergência</button>
              <button type="button" class="acao-linha" id="thmAmbar" style="border-left:4px solid #f59e0b">🟡 Âmbar / Operacional</button>
              <button type="button" class="acao-linha" id="thmRoxo" style="border-left:4px solid #8b5cf6">🟣 Roxo / Tecnologia</button>
              <button type="button" class="acao-linha" id="thmSlate" style="border-left:4px solid #64748b">⚫ Grafite / Corporativo</button>
              <button type="button" class="primario" id="thmCustom" style="box-shadow:none; padding:6px 14px">🎨 Personalizado…</button>
            </div>
          </div>

          <div class="form-linha" style="align-items:flex-end">
            <div class="campo">
              <label>Cor Primária</label>
              <div style="display:flex; gap:6px; align-items:center">
                <input type="color" id="pickerCor" value="${c.COR_PRIMARIA || '#10b981'}" style="width:42px; height:40px; padding:2px; cursor:pointer">
                <input id="c_COR_PRIMARIA" value="${c.COR_PRIMARIA || '#10b981'}" style="width:110px">
              </div>
            </div>
            <div class="campo">
              <label>Acento Claro</label>
              <input id="c_COR_PRIMARIA_CLARO" value="${c.COR_PRIMARIA_CLARO || '#34d399'}" style="width:110px">
            </div>
            <div class="campo">
              <label>Acento Escuro</label>
              <input id="c_COR_PRIMARIA_ESCURO" value="${c.COR_PRIMARIA_ESCURO || '#065f46'}" style="width:110px">
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

    function ajustarBrilhoHex(hex, percent) {
      let num = parseInt(hex.replace('#',''), 16);
      if (isNaN(num)) return hex;
      let r = (num >> 16) + Math.round(255 * (percent / 100));
      let g = ((num >> 8) & 0x00ff) + Math.round(255 * (percent / 100));
      let b = (num & 0x0000ff) + Math.round(255 * (percent / 100));
      r = Math.min(255, Math.max(0, r));
      g = Math.min(255, Math.max(0, g));
      b = Math.min(255, Math.max(0, b));
      return '#' + ((1 << 24) + (r << 16) + (g << 8) + b).toString(16).slice(1);
    }

    const setCores = (p, c, e) => {
      $('#c_COR_PRIMARIA').value = p;
      $('#pickerCor').value = p;
      $('#c_COR_PRIMARIA_CLARO').value = c;
      $('#c_COR_PRIMARIA_ESCURO').value = e;
    };

    // Sincronizar color picker
    $('#pickerCor').oninput = ev => {
      const val = ev.target.value;
      setCores(val, ajustarBrilhoHex(val, 25), ajustarBrilhoHex(val, -35));
    };

    // Presets
    $('#thmVerde').onclick = () => setCores('#10b981', '#34d399', '#065f46');
    $('#thmAzul').onclick = () => setCores('#3b82f6', '#60a5fa', '#1d4ed8');
    $('#thmVermelho').onclick = () => setCores('#ef4444', '#f87171', '#991b1b');
    $('#thmAmbar').onclick = () => setCores('#f59e0b', '#fbbf24', '#92400e');
    $('#thmRoxo').onclick = () => setCores('#8b5cf6', '#a78bfa', '#5b21b6');
    $('#thmSlate').onclick = () => setCores('#64748b', '#94a3b8', '#334155');

    $('#thmCustom').onclick = () => {
      modalCorPersonalizada((p, c, e) => setCores(p, c, e));
    };

    function modalCorPersonalizada(onAplicar) {
      const corAtual = $('#c_COR_PRIMARIA').value || '#10b981';
      const html = `
        <div class="modal" style="max-width:440px">
          <h3 style="margin-top:0">🎨 Seletor de Cores da Identidade Visual</h3>
          <p style="color:var(--tx2); font-size:13px; margin-bottom:14px">Selecione a cor institucional da sua organização. O sistema calculará os acentos claro e escuro automaticamente para manter legibilidade e harmonia visual.</p>
          
          <div style="background:var(--painel2); border:1px solid var(--borda); border-radius:var(--raio); padding:16px; margin-bottom:14px">
            <div style="display:flex; align-items:center; gap:12px; margin-bottom:12px">
              <input type="color" id="mCorBase" value="${corAtual}" style="width:54px; height:50px; border-radius:8px; border:2px solid var(--borda2); cursor:pointer">
              <div style="flex:1">
                <label style="font-weight:700; font-size:13px; display:block">Cor Primária (Hexadecimal)</label>
                <input id="mCorBaseHex" value="${corAtual}" style="font-family:monospace; font-size:14px; font-weight:700; margin-top:4px">
              </div>
            </div>

            <div style="display:grid; grid-template-columns:1fr 1fr; gap:8px; margin-top:10px">
              <div>
                <label style="font-size:11.5px; color:var(--tx3)">Acento Claro (Realce)</label>
                <input id="mCorClaroHex" value="${ajustarBrilhoHex(corAtual, 25)}" style="font-family:monospace; font-size:12.5px; margin-top:2px">
              </div>
              <div>
                <label style="font-size:11.5px; color:var(--tx3)">Acento Escuro (Fundo)</label>
                <input id="mCorEscuroHex" value="${ajustarBrilhoHex(corAtual, -35)}" style="font-family:monospace; font-size:12.5px; margin-top:2px">
              </div>
            </div>
          </div>

          <div style="margin-bottom:16px">
            <label style="font-size:12px; font-weight:600; color:var(--tx2)">Pré-visualização da Paleta:</label>
            <div id="mColorPreview" style="display:flex; gap:10px; align-items:center; margin-top:6px; padding:12px; background:var(--bg); border:1px solid var(--borda); border-radius:var(--raio)">
              <button type="button" style="background:${corAtual}; color:#fff; font-weight:700; padding:6px 14px; border-radius:6px; font-size:13px; box-shadow:0 0 10px rgba(0,0,0,0.3)" id="mPreviewBtn">Botão Ativo</button>
              <span style="color:${ajustarBrilhoHex(corAtual, 25)}; font-weight:700; font-size:13px" id="mPreviewLink">Texto & Indicadores</span>
            </div>
          </div>

          <div style="display:flex; justify-content:flex-end; gap:8px">
            <button type="button" class="acao-linha" onclick="this.closest('.modal-mask').remove()">Cancelar</button>
            <button type="button" class="primario" id="mBtnAplicarCores">Aplicar Paleta</button>
          </div>
        </div>
      `;

      const m = modal(html);
      const baseInp = m.querySelector('#mCorBase');
      const baseHex = m.querySelector('#mCorBaseHex');
      const claroHex = m.querySelector('#mCorClaroHex');
      const escuroHex = m.querySelector('#mCorEscuroHex');
      const prevBtn = m.querySelector('#mPreviewBtn');
      const prevLink = m.querySelector('#mPreviewLink');

      const atualizarCores = (cor) => {
        baseInp.value = cor;
        baseHex.value = cor;
        const cClaro = ajustarBrilhoHex(cor, 25);
        const cEscuro = ajustarBrilhoHex(cor, -35);
        claroHex.value = cClaro;
        escuroHex.value = cEscuro;
        prevBtn.style.background = cor;
        prevLink.style.color = cClaro;
      };

      baseInp.oninput = () => atualizarCores(baseInp.value);
      baseHex.oninput = () => {
        let v = baseHex.value.trim();
        if (!v.startsWith('#')) v = '#' + v;
        if (/^#[0-9A-Fa-f]{6}$/.test(v)) atualizarCores(v);
      };

      m.querySelector('#mBtnAplicarCores').onclick = () => {
        if (onAplicar) onAplicar(baseHex.value, claroHex.value, escuroHex.value);
        m.remove();
      };
    }

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
