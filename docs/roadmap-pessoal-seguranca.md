# Roadmap & Fundação Documental: Integração Pessoal e Módulo de Segurança (SCI)

**Status:** Proposta Arquitetural (Sem Código / Documental)  
**Autor:** Agente de Correção Mecânica (Frente A — Pessoal / Backend)  
**Data:** 08/10/2026  
**Diretiva:** Ordem do Diretor 08/10 — Fundação documental para o futuro módulo de segurança / controle de acesso e identificação rápida.

---

## 1. Estado Atual e Fundação Existente

O SCI já dispõe de infraestrutura completa para identificação visual, digital e impressa de militares/integrantes:

1. **Geração de QR Code Própria (Sem Dependências Externas):**
   - **Arquivo:** [`qrcode.go`](file:///c:/Users/hermes/Documents/programming/wt_frenteA_pessoal/qrcode.go)
   - **Capacidades:** Funções `GerarQRCode(texto string)`, `RenderSVG(w io.Writer, size int)` e `RenderPNG(scale, border int)`. Implementação pura e determinística em Go, sem bibliotecas externas.
2. **Endpoint de Emissão de QR Code Individual:**
   - **Arquivo:** [`server_pessoal.go:1350`](file:///c:/Users/hermes/Documents/programming/wt_frenteA_pessoal/server_pessoal.go#L1350) (`hPessoaQRCode`)
   - **Rota:** `GET /api/pessoas/{id}/qr` (suporta `?format=svg` e `image/png`).
   - **Payload Canônico Atual:** `sci://p:{id}:{nome_guerra}` (exemplo: `sci://p:42:SILVA`).
   - **Segurança de Acesso:** Válido apenas dentro do escopo do grupo da pessoa ou para perfil `admin`.
3. **Ficha Cadastral em PDF:**
   - **Arquivo:** [`server_pessoal.go:468`](file:///c:/Users/hermes/Documents/programming/wt_frenteA_pessoal/server_pessoal.go#L468) (`hPessoaPDF`)
   - **Rota:** `GET /api/pessoas/{id}/pdf`
   - **Finalidade:** Ficha imprimível consolidada com dados pessoais, foto, função/cadeira, resumo de presenças/conferências, cautelas ativas de material e escalas recentes.

---

## 2. Proposta de Extensão do Esquema de URI para Visitantes

Para atender ao controle de portaria e recepção de visitantes temporários:

- **Formato Canônico do Payload:** `sci://v:{id}:{token}`
  - `v`: Identificador de entidade tipo **Visitante**.
  - `{id}`: Identificador numérico do visitante (`visitantes.id`).
  - `{token}`: Hash criptográfico ou token aleatório de curta duração para validação offline/rápida anti-falsificação.
- **Vínculo com Responsável Interno:**
  - Todo visitante deve estar atrelado a um `responsavel_pessoa_id` (chave estrangeira referenciando `pessoas.id` no banco de pessoal) ou a um setor/unidade receptora.
  - A leitura do QR na guarita/portaria exibe instantaneamente quem autorizou a entrada e quem é o ponto focal interno.

---

## 3. Proposta de Registro de Veículos por Pessoa

### 3.1 Precedente Arquitetural Existente
No módulo de material ([`store.go:2480`](file:///c:/Users/hermes/Documents/programming/wt_frenteA_pessoal/store.go#L2480)), o padrão estabelecido para extensão de dados de itens especializados utiliza tabelas satélites 1:1 ou 1:N com chave estrangeira explícita (como na tabela `material_viaturas` com `PRIMARY KEY (item_id)` referenciando `material_itens`).

### 3.2 Tabela Sugerida: `pessoa_veiculos`
Para o cadastro de veículos associados ao militar/servidor:

```sql
CREATE TABLE IF NOT EXISTS pessoa_veiculos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pessoa_id INTEGER NOT NULL REFERENCES pessoas(id) ON DELETE CASCADE,
    placa TEXT NOT NULL,
    modelo TEXT NOT NULL,
    cor TEXT NOT NULL,
    observacao TEXT DEFAULT '',
    criado_em TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    atualizado_em TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_pessoa_veiculos_pessoa ON pessoa_veiculos(pessoa_id);
CREATE INDEX IF NOT EXISTS idx_pessoa_veiculos_placa ON pessoa_veiculos(placa);
```

**Diretrizes de Estilo:**
- Placa normalizada (letras maiúsculas sem traço/espaço).
- Uma pessoa pode possuir múltiplos veículos vinculados.
- Na leitura de QR ou consulta de placa na portaria, o veículo é resolvido imediatamente para o proprietário interno.

---

## 4. Arquitetura do Futuro Módulo de Segurança / Guarda

### 4.1 Casos de Uso Principais
1. **Verificação Rápida de Identidade (Portaria / Guarda):**
   - Leitor óptico ou câmera móvel lê o QR (`sci://p:...` ou `sci://v:...`).
   - Sistema valida vigência, status de apresentação do dia (presente, dispensado, serviço externo) e alertas de segurança.
2. **Controle de Acesso Veicular:**
   - Consulta rápida por placa identificando o militar proprietário, setor de destino e autorização de estacionamento.
3. **Registro de Entrada/Saída de Visitantes:**
   - Check-in e check-out com carimbo de data/hora e operador responsável.

### 4.2 Matriz Sugerida de Permissões por Papel
- **`admin` / `gerente`:** Acesso total à gestão de visitantes, veículos e configurações de acesso de sua unidade.
- **`guarda` / `operador_portaria` (novo papel/capacidade):**
  - Leitura de QR de pessoas e visitantes de todas as unidades ou da guarita ativa.
  - Consulta de veículos e placas.
  - Registro de passagens/acessos (log de portaria).
  - Bloqueado para edição cadastral de pessoas e usuários.
- **`operador` / `chefe_setor`:**
  - Solicitação de credenciamento prévio de visitantes para o seu setor.
  - Cadastro de seus próprios veículos via autoatendimento no perfil.

### 4.3 Endpoints Futuros Sugeridos (Especificação de Contrato — Sem Implementação)

- `GET /api/seguranca/verificar?qr={payload}`: Resolução unificada de QR Code (militar ou visitante). Retorna dados resumidos de identificação, foto, situação de presença e autorizações vigentes.
- `GET /api/seguranca/veiculos?placa={placa}`: Busca de veículo e retorno do titular cadastrado.
- `GET /api/pessoas/{id}/veiculos`: Listagem de veículos vinculados a uma pessoa.
- `POST /api/pessoas/{id}/veiculos`: Cadastro de veículo para uma pessoa.
- `DELETE /api/pessoas/{id}/veiculos/{veiculo_id}`: Remoção de vínculo veicular.
- `POST /api/seguranca/visitantes`: Cadastro prévio de visitante com geração de QR de acesso.
- `POST /api/seguranca/acessos`: Registro de evento de entrada/saída na portaria.
