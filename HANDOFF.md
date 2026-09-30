# SCI — Documento de Handoff & Transição Técnica (v1.0)

## 📌 Visão Geral do Projeto
O **SCI (Sistema de Controle Interno)** é uma plataforma enterprise monolítica, autocontida e de alto desempenho projetada para o controle digital de presença, conferências de efetivo, escalas de serviço, armaria/reserva de material e cautelas com anexos digitalizados.

O sistema opera sob o paradigma de **Zero Dependências Externas**:
- **Backend:** Go (compilado nativamente em binário único executável).
- **Banco de Dados:** SQLite embutido com WAL mode, transações ACID e integridade referencial.
- **Frontend:** Single-Page Application (SPA) em Vanilla JavaScript (ES6+), HTML5 e CSS3 moderno (Obsidian Slate Theme com Glassmorphism), embutido diretamente no binário Go via `//go:embed web`.

---

## 🏗️ Estrutura de Arquivos

```
sci/
├── main.go               # Ponto de entrada, flags, watchdog, boot e versão de schema (v16)
├── server.go             # Router HTTP, middlewares, rotas de API e handlers REST
├── store.go              # Camada de banco de dados SQLite, migrações atômicas (v1..v16) e auditoria
├── auth.go               # Autenticação Argon2id, sessões em banco e rate-limiting (Limiter)
├── v1_test.go            # Suite de testes de integração e ciclo de vida completo (Escalas, Material, White-Label)
├── web/
│   ├── index.html        # Shell HTML e ponto de ancoragem do SPA
│   ├── style.css         # Design System v300 (Obsidian Dark Glassmorphism, responsivo mobile)
│   ├── core.js           # Router client-side, shell de navegação, API helper, modals e toasts
│   ├── views_conf.js     # Views de Conferência de Pessoal diária e Relatórios
│   ├── views_gestao.js   # Views de Gestão: #/admin (Explorador Hierárquico de Grupos), #/grupos e #/perfil
│   ├── views_escalas.js  # View de Escalas de Serviço (#/escalas) e cadastro de turnos
│   ├── views_material.js # View de Reserva de Material & Cautelas (#/material) com upload de anexos
│   └── views_config.js   # View de Configurações Globais White-Label (#/configuracoes)
├── TECHNICAL_PLAN.md     # Plano arquitetural e roadmap técnico
├── CHANGELOG.md          # Histórico de alterações e releases
└── README.md             # Instruções de operação no host e deployment
```

---

## 🔑 Hierarquia de Papéis & Regras de Acesso

1. **Admin (`admin`)**:
   - Papel de infraestrutura e gestão global da organização.
   - Não possui `grupo_id` (não pertence a nenhuma unidade operacional específica).
   - Acesso exclusivo a: `#/admin` (Estrutura Organizacional, Auditoria de Contas, Backup), `#/relatorios` e `#/configuracoes`.
   - **Bloqueio Estrutural:** Endpoints operacionais de conferência, escalas e materiais retornam `403 Forbidden` para o Admin, pois estas operações são estritamente atreladas aos grupos.

2. **Gerente (`gerente`)**:
   - Comandante / Chefe da Unidade ou Subgrupo (`grupo_id`).
   - Acesso completo ao escopo do seu grupo e subgrupos: Conferência, Escalas, Material/Cautelas, Banco de Pessoal, Operadores e Relatórios da Unidade.

3. **Operador (`operador`)**:
   - Militar escalado para o lançamento de presença, cautela de materiais e controle diário no escopo do seu grupo.

---

## 🗄️ Esquema do Banco de Dados (Schema v16)

- **`schema_migrations`**: Registro de versões aplicadas (atualmente versão `16`).
- **`grupos`**: Unidades organizacionais com código único de vinculação.
- **`grupo_vinculos`**: Subordinação hierárquica entre grupos (grafo / árvore recursiva).
- **`usuarios`**: Contas de acesso com hash Argon2id e vinculação a grupo/pessoa.
- **`pessoas`**: Banco de dados de militares/servidores ativos e inativos.
- **`conferencias` & `presencas`**: Registro de presenças, faltas, atrasos e justificativas.
- **`escala_tipos`**: Tipos de postos e serviços configuráveis.
- **`escala_turnos` & `escala_pessoas`**: Turnos de escala e militares alocados.
- **`material_categorias`**: Categorias de bens e materiais (Armamento, Viaturas, TI, etc.).
- **`material_itens`**: Itens cadastrados com código de patrimônio e status.
- **`material_cautelas`**: Histórico de retiradas, responsáveis e devoluções.
- **`material_cautela_anexos`**: Armazenamento de digitalizações/PDFs/fotos escaneadas em base64.
- **`configuracoes`**: Chaves e valores de customização White-Label.
- **`auditoria`**: Log imutável de todas as ações no sistema com IP e timestamp.

---

## 🛠️ Procedimento de Compilação & Execução

### 1. Compilar Binário
```powershell
go build -o sci.exe .
```
*(No Linux: `go build -o sci .`)*

### 2. Executar
```powershell
.\sci.exe
```
*Variáveis de ambiente suportadas:*
- `SCI_PORT`: Porta TCP (padrão: `10003`).
- `SCI_DATA_DIR`: Diretório de banco de dados e backups (padrão: `./dados`).
- `SCI_OM_TITULO`: Nome padrão da organização.

### 3. Rodar Testes Automatizados
```powershell
go test -v -count=1 .
```

---

## 🚀 Próximos Passos Sugeridos para a v1.1
1. **Geração de QR Code:** Criação de etiquetas para escaneamento rápido de cautelas via câmera de celular/tablet.
2. **Relatório Gráfico de Escalas:** Visualização em calendário / timeline estilo Gantt.
3. **Notificações Push / Webhooks:** Alertas de cautelas em atraso de devolução.
