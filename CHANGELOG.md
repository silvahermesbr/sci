# SCI — Changelog Oficial

## [v1.0.0] — 2026-09-29

### 🚀 Novidades & Módulos Principais
- **Módulo de Escalas & Serviços Integrados (`/api/escalas/*`, `#/escalas`):**
  - Cadastro de Tipos de Serviço / Postos configuráveis (Oficial de Dia, Adjunto, Guarda, Sentinela, etc.).
  - Grade mensal e diária de turnos com seleção múltipla de militares e funções de posto.
  - **Motor Inteligente de Integração com Conferência:** Militares escalados no dia são automaticamente pré-preenchidos como *Justificada (Serviço de Escala)* ao abrir nova conferência.
  - Painel de visualização de efetivo de serviço ativo hoje.

- **Módulo de Material, Reserva & Cautelas com Anexos (`/api/material/*`, `#/material`):**
  - Inventário completo de bens, armamento, viaturas, chaves, rádios e equipamentos com controle de patrimônio e número de série.
  - **Modal Expresso de Cautela ("⚡ Iniciar Nova Cautela")** acessível diretamente do topo e das tabelas.
  - **Sistema Ilimitado de Anexos & Digitalizações:** Permite anexar múltiplos PDFs, fotos e fichas assinadas escaneadas no ato da cautela ou a posteriori.
  - Gerenciador de documentos anexos com download inline (`/api/material/anexos/{id}`) e visualização.
  - Operações completas de devolução e baixa patrimonial.
  - **Exclusão Definitiva Atômica:** Opção de excluir o bem e limpar seu histórico e anexos com integridade referencial em transação atômica.
  - Filtro inteligente de status no inventário (Ativos, Disponíveis, Acautelados, Manutenção, Baixados).

- **Módulo de Configurações Globais & White-Label (`/api/configuracoes`, `#/configuracoes`):**
  - Painel administrativo para customização de identidade visual, nome da instituição/organização, rótulos de pessoal/gerente e cores.
  - Armazenamento em chave-valor no SQLite (`configuracoes`).

- **Explorador Hierárquico Interativo de Grupos & Subgrupos (`#/admin`):**
  - Visualização recursiva em árvore em qualquer nível de profundidade (Comando › Brigada › Batalhão › Companhia › Pelotão › etc.).
  - **Modo de Foco com Breadcrumbs:** Navegação focada no grupo clicado com trilha de ancestrais interativa.
  - **Cálculo Recursivo de Efetivo:** Total de pessoal somando todos os subgrupos dependentes (`Efetivo Total: X | Próprio: Y`).
  - Ações diretas por unidade: Criar Subgrupo Subordinado, Trocar Gerente, Mudar Subordinação, Auditar Contas e Excluir.

### 🎨 Design System & Identidade Visual
- **Tema Obsidian Slate Dark:** Gradientes modernos escuros profundos (`#070a0f` → `#0d131f`) com cartões em glassmorphism fosco (`backdrop-filter: blur(12px)`).
- **Tipografia & Contraste:** Textos com alto contraste, chips semitransparentes com bordas refinadas.
- **Responsividade Mobile:** Drawer menu lateral com overlay, alvos de toque mínimos de 44px e tabelas fluidas.

### 🛡️ Segurança & Arquitetura
- **Guarda de Acesso por Escopo:**
  - Os módulos de **Escalas** e **Material** são restritos aos papéis de grupo (`gerente` e `operador`). O papel **Admin** (sistêmico) é bloqueado com `403 Forbidden` e redirecionado para a gestão global (`#/admin`).
- **Persistência & Migrações:**
  - Atualização para Schema v16 (tabelas `escala_tipos`, `escala_turnos`, `escala_pessoas`, `material_categorias`, `material_itens`, `material_cautelas`, `material_cautela_anexos`, `configuracoes`).
  - Zero dependência externa de runtime (Go + SQLite puro compilado em binário único autocontido).

### 🧪 Testes Automatizados
- Cobertura completa de migrações, sementes padrão, integração inteligente de escalas na conferência, ciclo de vida de materiais/anexos, baixas, exclusão definitiva e white-label (`v1_test.go`).
