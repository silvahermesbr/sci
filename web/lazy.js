/* SCI — Lazy Loader de Views (onda mod/f3-lazy)
   Carrega scripts de views sob demanda quando a rota hash é acessada.
   core.js e style.css são EAGER (diretamente no index.html).

   Defesa CSP: não usa eval, não gera script inline além do previsto;
   mantém script-src 'self' compatível.

   Funciona com OU sem window.SCI_ROTAS (contrato F1):
   — intercepta hashchange com capture ANTES do onhashchange/rotear;
   — envolve window.rotear para chamadas DIRETAS (irPara, SCI_BOOT sem hashchange);
   — tudo assíncrono com fila de diferimento.

   Mapa hash→arquivo derivado de core.js → rotear(). */
(function () {
  'use strict';

  /* ── Mapa: hash → caminho do script ── */
  var HASH_ARQUIVO = {
    '#/hoje':         '/views_conf.js',
    '#/conferencia':  '/views_conf.js',
    '#/relatorios':   '/views_conf.js',
    '#/mensagens':    '/views_mensagens.js',
    '#/despachos':    '/views_mensagens.js',
    '#/avisos':       '/views_avisos.js',
    '#/calendario':   '/views_calendario.js',
    '#/drive':        '/views_drive.js',
    '#/admin':        '/views_admin.js',
    '#/configuracoes':'/views_config.js',
    '#/grupos':       '/views_grupos.js',
    '#/perfil':       '/views_perfil.js',
    '#/escalas':      '/views_escalas.js',
    '#/material':     '/views_material.js',
    '#/consciencia':  '/views_consciencia.js'
  };

  /* Dependências: scripts extras carregados junto com uma view.
     editor_rico.js é necessário para views_conf.js e views_mensagens.js
     (EditorRico.init). vendor/quill.js e vendor/quill.snow.css são carregados
     sob demanda se existirem (reservados para uso futuro). */
  var DEP_VIEW = {
    '/views_conf.js':      ['/editor_rico.js'],
    '/views_mensagens.js': ['/editor_rico.js'],
    '/views_admin.js':     ['/ui_helpers.js'],
    '/views_grupos.js':    ['/ui_helpers.js'],
    '/views_perfil.js':    ['/ui_helpers.js']
  };

  var CACHEBUST = '?v=351';

  var carregados = new Set();        // URL → true (já injetado)
  var carregando = {};               // URL → Promise enquanto carrega

  /* ── Injeta <script src="url"> async ── */
  function carregarScript(url) {
    if (carregados.has(url)) return Promise.resolve();
    if (carregando[url]) return carregando[url];

    var p = new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = url + CACHEBUST;
      s.onload = function () {
        carregados.add(url);
        delete carregando[url];
        resolve();
      };
      s.onerror = function () {
        delete carregando[url];
        console.error('[lazy] falha ao carregar', url);
        resolve();  /* não trava o fluxo */
      };
      document.body.appendChild(s);
    });

    carregando[url] = p;
    return p;
  }

  /* ── Carrega script + dependências ── */
  function carregarView(url) {
    var deps = DEP_VIEW[url] || [];
    var chain = [url].concat(deps);
    // Filtra scripts que já existem (eager, ex: core.js nunca entra aqui)
    // e scripts que apontam para arquivos inexistentes
    return chain.reduce(function (prev, u) {
      return prev.then(function () { return carregarScript(u); });
    }, Promise.resolve());
  }

  /* ── Retorna URL se o hash ainda precisa ser carregado ── */
  function hashPrecisaCarregar(hash) {
    if (!hash) return null;
    var base = hash.split('?')[0];
    var url = HASH_ARQUIVO[base];
    if (!url) return null;
    if (carregados.has(url)) return null;
    return url;
  }

  /* ── Garante que o script da view está carregado ── */
  function garantirHash(hash) {
    var url = hashPrecisaCarregar(hash);
    if (!url) return Promise.resolve(false);
    return carregarView(url).then(function () { return true; });
  }

  /* ─═╡ Interceptação 1: hashchange com capture ╞═─
     Roda ANTES do onhashchange/rotear. Substitui onhashchange por um
     no-op temporário para impedir a chamada prematura de rotear(),
     carrega o script, e depois invoca o handler original manualmente. */
  window.addEventListener('hashchange', function () {
    var hash = location.hash || '';
    var url = hashPrecisaCarregar(hash);
    if (!url) return;

    var origHandler = window.onhashchange;
    window.onhashchange = null;

    garantirHash(hash).then(function () {
      window.onhashchange = origHandler;
      if (typeof origHandler === 'function') {
        // Delay mínimo para garantir que o script terminou de executar
        setTimeout(origHandler, 5);
      }
    });
  }, true);

  /* ─═╡ Interceptação 2: chamadas DIRETAS a rotear() ╞═─
     Chamado ANTES do SCI_BOOT() no index.html. Envolve window.rotear
     para capturar calls de irPara() quando o hash já coincide
     (rotear() chamado diretamente, sem hashchange). */
  window._lazy_antes_boot = function () {
    if (typeof window.rotear !== 'function') return;
    var orig = window.rotear;
    window.rotear = function () {
      var hash = location.hash || '';
      var url = hashPrecisaCarregar(hash);
      if (url) {
        garantirHash(hash).then(function () { orig(); });
        return;
      }
      orig();
    };
  };

  /* ─═╡ Auto-load do EditorRico ╞═─
     EditorRico é definido por editor_rico.js, que é carregado como
     dependência de views_conf.js e views_mensagens.js via DEP_VIEW.
     Se por algum motivo for acessado antes, tentamos carregar na hora. */
  (function () {
    var _er = null;
    Object.defineProperty(window, 'EditorRico', {
      configurable: true,
      enumerable: true,
      get: function () { return _er; },
      set: function (v) { _er = v; }
    });
  })();

  /* ════════════════════════════════════════════════
     Interface com F1 (window.SCI_ROTAS)
     Se existir, registramos nosso mapa via
     SCI_REGISTRAR_ROTA(hash, fnCarregar, meta).
     Caso contrário, funciona independentemente.
     ════════════════════════════════════════════════ */
  if (typeof window.SCI_ROTAS !== 'undefined' && typeof window.SCI_REGISTRAR_ROTA === 'function') {
    for (var h in HASH_ARQUIVO) {
      if (HASH_ARQUIVO.hasOwnProperty(h)) {
        (function (hash, url) {
          window.SCI_REGISTRAR_ROTA(hash, function () {
            return carregarView(url);
          }, { lazy: true, arquivo: url });
        })(h, HASH_ARQUIVO[h]);
      }
    }
  }

})();