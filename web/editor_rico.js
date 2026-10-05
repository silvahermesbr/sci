/* editor_rico.js — módulo de editor rich-text contenteditable (onda 05/10).
   Extrai e substitui o editor legado de core.js (document.execCommand solto):
   - EditorRico.init(el, opcoes): transforma um div em editor com toolbar
     (negrito/itálico/sublinhado/listas/link/undo/redo; toolbar:'completa'
     preserva o conjunto antigo — H2/H3/parágrafo/alinhamentos/limpar);
   - sanitização de COLAGEM na entrada e de TODO conteúdo lido/definido:
     allowlist b,i,u,p,div,br,ul,ol,li,a[href] — a mesma de sanitiza.go
     (o servidor re-sanitiza; aqui é a 1ª barreira);
   - undo/redo (botões + Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y);
   - preservação de foco/seleção: mousedown na toolbar não rouba o caret.
   Sem CDN/biblioteca externa (CSP bloqueia; projeto sem build-step). */
(function () {
  'use strict';

  var ALLOWLIST = { B: 1, I: 1, U: 1, P: 1, DIV: 1, BR: 1, UL: 1, OL: 1, LI: 1, A: 1 };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  /* sanitizaNo: devolve HTML seguro do nó (recursivo). Tag fora da allowlist
     desaparece mantendo o texto interno; <a> só com href http(s)/mailto. */
  function sanitizaNo(no) {
    if (no.nodeType === 3) return esc(no.nodeValue);
    if (no.nodeType !== 1) return '';
    var tag = no.tagName;
    var inner = '';
    for (var i = 0; i < no.childNodes.length; i++) inner += sanitizaNo(no.childNodes[i]);
    if (!ALLOWLIST[tag]) return inner;
    var t = tag.toLowerCase();
    if (t === 'br') return '<br>';
    if (t === 'a') {
      var href = (no.getAttribute('href') || '').trim();
      if (!/^(https?:\/\/|mailto:)/i.test(href)) href = '#';
      return '<a href="' + esc(href) + '">' + inner + '</a>';
    }
    return '<' + t + '>' + inner + '</' + t + '>';
  }

  function sanitizaHTML(html) {
    var div = document.createElement('div');
    div.innerHTML = html == null ? '' : String(html);
    var out = '';
    for (var i = 0; i < div.childNodes.length; i++) out += sanitizaNo(div.childNodes[i]);
    return out;
  }

  function executar(editor, cmd, val) {
    editor.focus();
    try { document.execCommand(cmd, false, val || null); } catch (e) { /* comando não suportado */ }
    editor.focus();
  }

  function inserirLink(editor) {
    editor.focus();
    var sel = window.getSelection ? window.getSelection() : null;
    var texto = sel && !sel.isCollapsed ? sel.toString() : '';
    var url = window.prompt('URL do link (https://…)', 'https://');
    if (!url) return;
    url = url.trim();
    if (!/^(https?:\/\/|mailto:)/i.test(url)) url = 'https://' + url;
    var rotulo = texto || url.replace(/^https?:\/\//i, '');
    executar(editor, 'insertHTML', '<a href="' + esc(url) + '">' + esc(rotulo) + '</a>&nbsp;');
  }

  var BOTOES_BASE = [
    { cmd: 'bold', titulo: 'Negrito (Ctrl+B)', html: '<b>B</b>' },
    { cmd: 'italic', titulo: 'Itálico (Ctrl+I)', html: '<i>I</i>' },
    { cmd: 'underline', titulo: 'Sublinhado (Ctrl+U)', html: '<u>U</u>' }
  ];
  var BOTOES_LISTA = [
    { cmd: 'insertUnorderedList', titulo: 'Lista com Marcadores', html: '• List' },
    { cmd: 'insertOrderedList', titulo: 'Lista Numerada', html: '1. List' }
  ];
  var BOTOES_COMPLETA = [
    { cmd: 'formatBlock', val: 'H2', titulo: 'Título (H2)', html: '<b style="font-size:11px">H2</b>' },
    { cmd: 'formatBlock', val: 'H3', titulo: 'Subtítulo (H3)', html: '<b style="font-size:10px">H3</b>' },
    { cmd: 'formatBlock', val: 'P', titulo: 'Parágrafo', html: '<span style="font-size:10px">¶</span>' },
    { cmd: 'justifyLeft', titulo: 'Alinhar à Esquerda', html: '⇤' },
    { cmd: 'justifyCenter', titulo: 'Centralizar', html: '≡' },
    { cmd: 'justifyRight', titulo: 'Alinhar à Direita', html: '⇥' },
    { cmd: 'justifyFull', titulo: 'Justificado', html: '≣' },
    { cmd: 'removeFormat', titulo: 'Limpar Formatação', html: '🧹' }
  ];

  window.EditorRico = {
    sanitizaHTML: sanitizaHTML,

    /* init(el, opcoes): el vira wrapper (toolbar + contenteditable).
       opcoes: { placeholder, valorInicial, toolbar: 'base'|'completa' } */
    init: function (el, opcoes) {
      if (!el) return null;
      opcoes = opcoes || {};
      var completa = opcoes.toolbar === 'completa';
      var grupos = BOTOES_BASE.concat(BOTOES_LISTA, [{ cmd: '__link', titulo: 'Inserir Link', html: '🔗' }, { cmd: 'undo', titulo: 'Desfazer (Ctrl+Z)', html: '↺' }, { cmd: 'redo', titulo: 'Refazer (Ctrl+Y)', html: '↻' }], completa ? BOTOES_COMPLETA : []);

      el.classList.add('rich-editor-wrapper');
      el.innerHTML =
        '<div class="rich-toolbar">' +
        '<div class="rich-btn-group">' +
        grupos.map(function (b) {
          return '<button type="button" class="rich-btn" data-cmd="' + b.cmd + '"' +
            (b.val ? ' data-val="' + b.val + '"' : '') +
            ' title="' + esc(b.titulo) + '">' + b.html + '</button>';
        }).join('') +
        '</div></div>' +
        '<div class="rich-content" contenteditable="true" data-placeholder="' + esc(opcoes.placeholder || '') + '"></div>';

      var editor = el.querySelector('.rich-content');
      if (opcoes.valorInicial) editor.innerHTML = sanitizaHTML(opcoes.valorInicial);

      // Toolbar: preventDefault no mousedown preserva seleção/foco do editor.
      el.querySelectorAll('.rich-btn').forEach(function (btn) {
        btn.onmousedown = function (e) {
          e.preventDefault();
          var cmd = btn.dataset.cmd;
          if (cmd === '__link') { inserirLink(editor); return; }
          executar(editor, cmd, btn.dataset.val || null);
        };
      });

      // Undo/redo por teclado (os botões usam execCommand undo/redo).
      editor.addEventListener('keydown', function (e) {
        if (!(e.ctrlKey || e.metaKey)) return;
        var k = (e.key || '').toLowerCase();
        if (k === 'y' || (k === 'z' && e.shiftKey)) { e.preventDefault(); executar(editor, 'redo'); }
      });

      // Sanitização de COLAGEM: html colado passa pela allowlist; texto puro
      // vira parágrafos/quebras. Tudo o mais (estilos, scripts, imagens) some.
      editor.addEventListener('paste', function (e) {
        e.preventDefault();
        var cd = e.clipboardData || window.clipboardData;
        if (!cd) return;
        var html = cd.getData ? (cd.getData('text/html') || '') : '';
        var texto = cd.getData ? (cd.getData('text/plain') || '') : '';
        var inserir = html ? sanitizaHTML(html)
          : esc(texto).replace(/\r/g, '').split(/\n{2,}/).map(function (p) {
            return '<p>' + p.replace(/\n/g, '<br>') + '</p>';
          }).join('');
        if (inserir) executar(editor, 'insertHTML', inserir);
      });

      return {
        getHTML: function () { return sanitizaHTML(editor.innerHTML).trim(); },
        setHTML: function (h) { editor.innerHTML = sanitizaHTML(h); },
        getText: function () { return (editor.innerText || editor.textContent || '').trim(); },
        focus: function () { editor.focus(); },
        getElement: function () { return editor; },
        destroy: function () { el.innerHTML = ''; }
      };
    }
  };
})();
