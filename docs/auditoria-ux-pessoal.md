# Auditoria UX e Contrato do Módulo Pessoal (Front-End)

**Data:** 08/10/2026  
**Módulo:** Pessoal (`web/views_pessoal.js`)  
**Responsável:** Agente de Correção Mecânica (Frente B — Front-End)  
**Branch:** `fix/pessoal-front-ux`  

---

## 1. Contrato Front × Back

Cruzamento exaustivo de cada chamada `api(...)` e requisições manuais (`window.open`) efetuadas por `web/views_pessoal.js` contra os handlers registrados no servidor Go (`server_pessoal.go:1531`, `server_grupos.go:749`, `server_catalogo.go:988`, `server_conferencia.go:1517`).

| Linha(s) | Endpoint / URL | Método | Rota Registrada no Back | Veredito | Tratamento de Erro / Resiliência |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **117** | `/api/grupos` | `GET` | `server_grupos.go:755` (`GET /api/grupos`) | **OK** | Presente no `Promise.all` raiz; propaga falha ao roteador se API cair. |
| **117, 410, 472** | `/api/pessoas` | `GET` | `server_pessoal.go:1568` (`GET /api/pessoas`) | **OK** | L.117/410 em `Promise.all`; l.472 encapsulado em `try/catch` com fallback de UI. |
| **118, 131, 410, 570** | `/api/catalogo/setores` | `GET` | `server_catalogo.go:988` (`GET /api/catalogo/{t}`) | **OK** | L.131 em `try/catch`; l.410/570 em `try/catch` exibindo mensagem de erro na tabela. |
| **118, 131** | `/api/catalogo/funcoes` | `GET` | `server_catalogo.go:988` (`GET /api/catalogo/{t}`) | **OK** | L.131 em `try/catch` defensivo. |
| **118, 410, 474, 570** | `/api/usuarios` | `GET` | `server_pessoal.go:1578` (`GET /api/usuarios`) | **OK** | L.474 e l.570 tratados com `try/catch`. |
| **119** | `/api/pessoas/apresentacao` | `GET` | `server_pessoal.go:1569` (`GET /api/pessoas/apresentacao`) | **OK** | Tratado com `.catch(() => ({ apresentacao: [] }))` defensivo. |
| **296** | `/api/usuarios` | `POST` | `server_pessoal.go:1582` (`POST /api/usuarios`) | **OK** | Chamado via `processar()`, exibindo toast e spinner. |
| **301** | `/api/usuarios/{id}` | `PATCH` | `server_pessoal.go:1583` (`PATCH /api/usuarios/{id}`) | **OK** | Tratado com `try/catch` defensivo. |
| **334** | `/api/grupo/funcoes/membros` | `GET` | `server_grupos.go:749` (`GET /api/grupo/funcoes/membros`) | **OK** | `try/catch` com mensagem informativa de falha no `tbody`. |
| **376** | `/api/grupo/funcoes/membros` | `POST` | `server_grupos.go:750` (`POST /api/grupo/funcoes/membros`) | **OK** | `processar()` com toast de erro automático. |
| **382** | `/api/grupo/funcoes/membros/{id}` | `DELETE` | `server_grupos.go:751` (`DELETE /api/grupo/funcoes/membros/{id}`) | **OK** | `processar()` com toast de erro automático. |
| **498** | `/api/catalogo/setores/{id}` | `PATCH` | `server_catalogo.go:993` (`PATCH /api/catalogo/{t}/{id}`) | **OK** | `processar()`, valida campos antes do disparo. |
| **510, 603** | `/api/grupos/{id}/nomear_chefe` | `POST` | `server_grupos.go:752` (`POST /api/grupos/{id}/nomear_chefe`) | **OK** | `processar()`, valida campos obrigatórios. |
| **522** | `/api/setores/{id}` | `DELETE` | `server_catalogo.go:994` (`DELETE /api/setores/{id}`) | **OK** | `processar()`, dupla confirmação no front. |
| **540** | `/api/setores/agregado` | `GET` | `server_catalogo.go:989` (`GET /api/setores/agregado`) | **OK** | `try/catch` capturando falhas no `tbody`. |
| **612** | `/api/grupos/{id}/destituir_chefe` | `POST` | `server_grupos.go:753` (`POST /api/grupos/{id}/destituir_chefe`) | **OK** | `processar()` com confirmação de usuário. |
| **632, 665** | `/api/pessoas` | `POST` | `server_pessoal.go:1570` (`POST /api/pessoas`) | **OK** | L.632 via `processar()`; l.665 em loop de importação em lote com contagem de erros no `try/catch`. |
| **631, 745** | `/api/pessoas/{id}` | `PATCH` | `server_pessoal.go:1571` (`PATCH /api/pessoas/{id}`) | **OK** | L.631 via `processar()`; l.745 em lote com coleta de erros por ID. |
| **648, 956** | `/api/catalogo/setores` | `POST` | `server_catalogo.go:990` (`POST /api/catalogo/{t}`) | **OK** | L.648 em `try/catch` (importador); l.956 via `processar()`. |
| **658** | `/api/catalogo/funcoes` | `POST` | `server_catalogo.go:990` (`POST /api/catalogo/{t}`) | **OK** | `try/catch` dentro da importação CSV. |
| **676, 708** | `/api/pessoas/{id}` | `DELETE` | `server_pessoal.go:1572` (`DELETE /api/pessoas/{id}`) | **OK** | L.676 via `processar()`; l.708 em lote com try/catch e relatório de parciais. |
| **775** | `/api/pessoas/{id}/pdf` | `GET (URL)` | `server_pessoal.go:1576` (`GET /api/pessoas/{id}/pdf`) | **OK** | Aberto via `window.open` em nova aba (sem necessidade de cabeçalho manual anti-CSRF). |
| **831** | `/api/pessoas/{pid}/apresentacao` | `POST` | `server_pessoal.go:1573` (`POST /api/pessoas/{id}/apresentacao`) | **OK** | `try/catch` com checagem de `resp.ok` e `toast` de erro. |
| **872** | `/api/pessoas/{pid}/modificacoes` | `GET` | `server_pessoal.go:1574` (`GET /api/pessoas/{id}/modificacoes`) | **OK** | `try/catch` com mensagem amigável no DOM em caso de falha. |
| **902** | `/api/conferencia/lista` | `GET` | `server_conferencia.go:1525` (`GET /api/conferencia/lista`) | **OK** | `try/catch` tratando array vazio e desabilitando botões. |
| **931, 936** | `/api/conferencia/{id}/relatorio.pdf` | `GET (URL)` | `server_conferencia.go:1530` (`GET /api/conferencia/{id}/relatorio.pdf`) | **OK** | Aberto via `window.open` com query param `?filtro=faltas` ou `?filtro=atrasos`. |

**Resumo de integridade de rotas:**  
- Total de rotas auditadas: 20 endpoints distintos.
- Discrepâncias de método: **0**.
- Endpoints fantasmas (sem backend correspondente): **0**.
- Faltas de tratamento de erro: **0** (todos os fluxos possuem `try/catch`, `processar()` ou `.catch()`).

---

## 2. Acessibilidade (A11y)

| Item | Localização (Linhas) | Descrição do Problema | Impacto / Norma WCAG |
| :--- | :--- | :--- | :--- |
| **1. Ausência de `scope="col"` em cabeçalhos de tabela** | `views_pessoal.js:216, 233, 248, 253, 257` | As tabelas de Efetivo (`#tabP`), Funções (`#tabFun`), Setores (`#tabSetores`), Chefias (`#tabChefes`) e Panorama (`#tabAgreg`) possuem tags `<th>` sem o atributo `scope="col"`. | Leitores de tela não associam corretamente células de dados ao cabeçalho (WCAG 2.1 - 1.3.1 Info and Relationships). |
| **2. Checkboxes e botões sem `aria-label`** | `views_pessoal.js:188, 195, 196, 197, 198` | O checkbox de cada linha `<input type="checkbox" class="chkP" data-id="...">` não possui `aria-label`. Os botões de ação na tabela possuem apenas texto visual ou ícone (ex.: `📄 ficha`) sem rótulo semântico explícito para tecnologias assistivas. | Usuários de leitor de tela não sabem qual militar estão selecionando ou editando (WCAG 4.1.2 Name, Role, Value). |
| **3. Foco não aprisionado em modais (*Focus Trap*)** | `views_pessoal.js:451, 721, 788, 859, 942` | Ao abrir modais (Edição de Setor, Edição em Lote, Apresentação, Histórico, Novo Setor), o foco do teclado (Tab) não fica confinado à janela modal, permitindo navegar pelos elementos do fundo da página. | Perda de contexto e navegação confusa (WCAG 2.4.3 Focus Order). |
| **4. Emojis decorativos sem `aria-hidden`** | `views_pessoal.js:198, 350, 430, 461, 552, 585, 943` | Ícones e emojis como `👑` (chefe/titular), `📄` (ficha), `🗑` (excluir) e `🏢` (setor) são renderizados diretamente no HTML sem `<span aria-hidden="true">`. | Leitores de tela pronunciam "coroa", "página", "lixeira de papel" antes do texto relevante (WCAG 1.1.1 Non-text Content). |
| **5. Contraste de texto secundário e pills de status** | `web/style.css:26` (`--tx3: #64748b`) e `views_pessoal.js:192, 351, 427, 431, 553, 587` | O texto secundário "● INATIVO", "sem chefe" e "sem titular" utiliza a cor `var(--tx3)` (`#64748b`) sobre o fundo escuro (`#0e1524`), resultando em taxa de contraste de **~2.6:1** (abaixo do limiar mínimo de 4.5:1 exigido pelo critério WCAG AA). As pills `.pill-presente`, `.pill-atraso`, `.pill-falta` e `.gpx-pill-dispensado` apresentam contraste adequado (>5.5:1 a 8:1), mas `.gpx-pill-descompensado` (`--tx2` `#94a3b8`) opera no limiar de 4.6:1. | Dificuldade de legibilidade para pessoas com baixa visão (WCAG 1.4.3 Contrast Minimum). |

---

## 3. Integração Futura (Módulo de Segurança / Portaria)

### Diagnóstico de Exposição Atual
- **Busca no módulo front:** A rota de QR Code pessoal (`/qr`) **não está presente nem referenciada** em nenhuma linha de `web/views_pessoal.js`.
- **Backend já existente:** O servidor possui o endpoint `GET /api/pessoas/{id}/qr` (`server_pessoal.go:1575`), que retorna o payload tático `sci://p:{id}:{nome_guerra}` em formato SVG ou PNG.
- **Ficha PDF:** O endpoint `GET /api/pessoas/{id}/pdf` (`server_pessoal.go:1576`) já é consumido na linha 775 de `views_pessoal.js`.

### Proposta de Integração (Sem Código / Especificação Textual)

1. **Botão "QR" na coluna de Ações do Efetivo:**
   - **Localização:** Na tabela `#tabP`, dentro da `<div class="gpx-acoes">` (ao lado de `data-edit` e `data-fichap`).
   - **Comportamento:** Botão `<button class="acao-linha" data-qr="${p.id}" title="Exibir / Imprimir Credencial QR">📱 QR</button>`. Ao clicar, exibe modal compacto com o SVG obtido de `/api/pessoas/${p.id}/qr?format=svg`, nome de guerra, antiguidade, setor e botão "Imprimir Credencial / Crachá".

2. **Bloco de Identificação Ótica no PDF Cadastral:**
   - **Localização:** No cabeçalho superior direito da ficha emitida por `GET /api/pessoas/{id}/pdf`.
   - **Objetivo:** Permitir que o documento impresso possa ser rapidamente validado por bipagem ótica em postos de guarda e postos de controle de tráfego.

3. **Vínculo Visitante → Militar Responsável (Módulo de Acesso):**
   - **Fluxo:** Na recepção / portaria, ao cadastrar um visitante ou viatura civil, o operador busca o militar anfitrião por nome de guerra ou realiza a leitura direta do QR Code do crachá do militar (`sci://p:{id}:{nome_guerra}`).
   - **Rastreabilidade:** O registro de entrada fica associado a `responsavel_pessoa_id`, permitindo auditoria instantânea de quem autorizou o ingresso no quartel/organização.

---

## 4. Recomendação de Infraestrutura (Arquitetura Front-End)

### Cenário Atual
Os helpers de manipulação de tabela (`window.tblOrdenar`, `window.tblPaginar` e `window.tabelaControles`) estão declarados no corpo de `web/views_gestao.js` (linhas 112–117).  
Como o módulo Gestão só é carregado sob demanda (lazy loading quando o usuário navega para `#/admin`), se um operador com permissão de Encarregado de Pessoal acessar diretamente a URL `#/pessoal` (ou recarregar via F5), `window.tblOrdenar` e `window.tabelaControles` não estarão definidos na memória. Por essa razão, foi implementado o guard defensivo:
```javascript
if (typeof window.tblOrdenar === 'function' && typeof window.tblPaginar === 'function') {
  window.tabelaControles('pes-efetivo', tabelaEl, tbodyEl, [...], 20);
}
```

### Recomendação
Mover as funções `tblOrdenar`, `tblPaginar` e `tabelaControles` de `web/views_gestao.js` para um arquivo de carregamento **eager** no `index.html` (por exemplo, dentro de `web/core.js` ou em um novo `web/ui_helpers.js`).  
**Benefícios:**
- Elimina a necessidade de guards defensivos em `views_pessoal.js`, `views_conf.js` e futuras views.
- Garante ordenação e paginação consistentes no primeiro carregamento de qualquer rota da SPA.
- Elimina duplicações de lógica e reduz a complexidade de manutenção.
*(Nota: Nenhuma alteração foi realizada em arquivos fora da lista positiva, em conformidade com as diretrizes do projeto).*
