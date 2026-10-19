# Cais

Interface web local para gerenciar containers Docker, entregue como **um único
executável**. Sem instalação, sem dependência de runtime, sem conta.

Nasceu de uma necessidade concreta: o Docker Desktop exige assinatura paga em
empresas acima de certo porte, mas o **Docker Engine é Apache 2.0 e gratuito**.
Quem migra para o Engine puro perde a interface gráfica e volta para a linha de
comando. Este projeto repõe essa interface.

> **Não é afiliado à Docker, Inc.** É um cliente independente da Engine API.
> Veja [NOTICE.md](NOTICE.md).

## Características

- **Um arquivo.** Templates, CSS, JavaScript e ícones embutidos via `go:embed`.
  Copiar o `.exe` é a instalação inteira — 13 MB, sem instalador.
- **Uma dependência direta.** `modernc.org/sqlite` (Go puro, sem cgo). O SDK
  oficial do Docker foi descartado de propósito: traria cerca de 60 módulos
  transitivos e 20–25 MB ao binário para usar aproximadamente 15 endpoints.
- **Funciona offline.** Nenhum recurso vem de CDN.
- **Sem login.** A proteção é de rede, não de credencial — ver *Segurança*.

## O que dá para fazer

| Tela | Recursos |
|---|---|
| **Dashboard** | Contadores de containers, imagens e volumes; espaço em disco; últimas ações |
| **Containers** | Listar com estado, stack de origem e portas; iniciar, parar, reiniciar, remover; **criar** (imagem, nome, portas, variáveis, volumes, política de reinício, comando); porta publicada vira link clicável |
| **Logs** | Stream ao vivo, filtro de linhas, `tail` ajustável, carimbo de hora, quebra de linha, seguir o fim, limpar e baixar |
| **Stacks** | Registrar um `docker-compose.yml` com validação; subir, reiniciar e derrubar; estado por serviço |
| **Imagens** | Listar com tamanho e uso; **baixar com progresso**; remover; atalho para criar container |
| **Volumes** | Listar; **criar**; remover |
| **Histórico** | Toda operação do painel, com resultado, duração e erro |
| **Configurações** | Cor de destaque (paleta ou cor livre) e dados da conexão |

Em todas as tabelas: busca, ordenação por coluna e copiar para a área de
transferência. Tema claro/escuro, menu retrátil e atalhos de teclado.

## Estado atual

| Fase | Entrega | Estado |
|---|---|---|
| 1 | Servidor local, SQLite, templates, design system | ✅ |
| 2 | Cliente da Engine API, listagem de containers | ✅ |
| 3 | Ciclo de vida (iniciar/parar/reiniciar/remover) e histórico | ✅ |
| 4 | Estatísticas ao vivo (CPU e memória) via SSE | pendente |
| 5 | Logs ao vivo | ✅ |
| 6 | Terminal interativo (`exec`) | pendente |
| 7 | Imagens e volumes | ✅ |
| 8 | Stacks Compose (subir, derrubar, reiniciar) | ✅ |
| 9 | Acabamento e distribuição | parcial |
| — | Criar volume, baixar imagem, criar container | ✅ |

**Pendentes:** estatísticas ao vivo (CPU e memória por container) e terminal
interativo. São os dois itens que faltam para o escopo original fechar.

Enquanto o terminal não existe, para entrar num container ainda é linha de
comando:

```bash
docker exec -it <nome> sh
```

## Instalação

Baixe o executável da página de *releases*, ou compile:

```bash
git clone <url-do-repositorio>
cd cais
go build -o cais.exe .
```

O Windows pode bloquear um executável não assinado baixado da internet. Libere
com `Unblock-File .\cais.exe` no PowerShell.

## Pré-requisito: um Docker Engine acessível

O painel **não embute um motor Docker** — ele conversa com um que já exista,
pela Engine API em TCP.

No Linux, exponha o daemon em loopback com um *drop-in* do systemd em
`/etc/systemd/system/docker.service.d/override.conf`:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/dockerd -H fd:// -H tcp://127.0.0.1:2375
```

> Declarar `hosts` no `daemon.json` **não funciona** quando a unit já passa
> `-H fd://`: o daemon aborta com *"specified both as a flag and in the
> configuration file"*.

No Windows com WSL2, o mesmo vale dentro da distro; o encaminhamento de
`localhost` do WSL cuida do resto.

### Aviso de segurança

A porta 2375 **não tem autenticação e equivale a acesso root** na máquina que
roda o daemon. Quem alcança essa porta pode subir um container privilegiado com
o disco do host montado.

**Faça o bind sempre em `127.0.0.1`, nunca em `0.0.0.0`.** Em rede não
confiável, não habilite.

## Uso

```bash
cais              # sobe e abre o navegador
cais -dev         # recarrega templates a cada requisição
```

Um segundo duplo-clique não sobe outra instância: o processo detecta o painel
já rodando pelo `/healthz`, abre o navegador nele e encerra.

### Opções

| Opção | Padrão | Descrição |
|---|---|---|
| `-dev` | `false` | Lê templates e estáticos do disco, com *reparse* a cada requisição |
| `-porta` | `45678` | Porta de escuta; `0` usa qualquer porta livre |
| `-no-browser` | — | Não abre o navegador ao iniciar |
| `-docker-host` | `tcp://127.0.0.1:2375` | Endpoint da Engine API |
| `-wsl-distro` | `Ubuntu-24.04` | Distro usada para executar o `docker compose` |
| `-db` | `%LOCALAPPDATA%\Cais\painel.db` | Caminho do banco local |

### Atalhos de teclado

| Tecla | Ação |
|---|---|
| `/` | Focar o campo de busca |
| `Esc` | Limpar a busca |
| `b` | Recolher ou expandir o menu |

## Segurança

O painel não tem login, por desenho: ele serve um único usuário na própria
máquina. A proteção vem de três camadas:

1. **Loopback.** O *listener* só escuta em `127.0.0.1`.
2. **Validação de origem.** `Host` e `Origin` são conferidos a cada requisição,
   o que bloqueia *DNS rebinding*.
3. **CSRF.** Token aleatório por processo, exigido em todo `POST` e na *query
   string* de SSE e WebSocket.

Nenhuma rota faz *proxy* cru para o daemon: cada operação é nomeada e consta de
uma lista explícita de permissões. O valor da cor de destaque, que vai parar
dentro de um bloco `<style>`, é validado contra `^#[0-9a-fA-F]{6}$`.

## Arquitetura

Monolito MVC, sem framework.

```
main.go          entrypoint: listener, instância única, navegador, encerramento
embed.go         go:embed de templates/ e static/
internal/
  config/        flags e caminho do banco
  docker/        cliente da Engine API — client, containers, logs (demux),
                 criar (pull/create), system (imagens, volumes), types
  compose/       execução do Compose dentro do WSL — runner, tradução de caminho
  store/         SQLite: migrations, preferências, stacks, histórico
  models/        entidades e dados de página
  handler/       controllers, middlewares, templates, tema
templates/       html/template (base.html + 10 páginas)
static/          app.css, app.js, icons.svg, vendor/pico.classless.min.css
```

O coletor de estatísticas (`internal/stats/`) ainda não existe — entra com a
fase 4.

### Decisões que valem explicar

**Por que não o SDK oficial do Docker.** Peso. São ~60 módulos transitivos e
20–25 MB no binário para cobrir uma API inteira, quando o painel usa ~15
endpoints. O que o SDK resolveria e não é trivial — o *hijack* da conexão e a
demultiplexação do stream de logs — são cerca de 100 linhas, escritas uma vez.

**Por que o Compose roda dentro do WSL.** O Compose não existe na Engine API, é
CLI. E um daemon Linux não entende caminho Windows: executar `docker compose`
pelo Windows contra um daemon no WSL quebra *bind mounts* relativos de forma
silenciosa — o `up` passa limpo e o container sobe com o diretório vazio.

**Por que SQLite em Go puro.** `modernc.org/sqlite` dispensa cgo, e portanto
dispensa uma *toolchain* C na máquina de quem compila.

**Por que o download de imagem é `fetch` + stream, e não SSE.** O `EventSource`
do navegador só faz `GET`, e baixar imagem muda estado — não cabe num `GET`. A
rota é `POST` e devolve NDJSON, lido no front com `ReadableStream`. A barra de
progresso conta **camadas concluídas**, não bytes: contar bytes faria a barra
andar para trás quando uma camada nova aparece no meio do download.

**Por que containers criados publicam só em `127.0.0.1`.** Um container de
desenvolvimento não deveria ficar exposto na rede da empresa porque alguém
esqueceu de restringir. Quem precisar expor tem de mudar isso de propósito.

**Por que o Compose roda sincronamente, mas o log não.** Operações de Compose
terminam em segundos e cabem num `POST` com *redirect*. Log é infinito — exige
stream.

## Desenvolvimento

```bash
go run . -dev      # templates recarregam sem recompilar
go vet ./...
```

Convenções do projeto:

- **Interface em português** com acentuação. **Código em português sem acento**
  (identificadores e comentários), para evitar atrito de codificação de
  caracteres entre editores.
- Página nova: incluir o nome em `pages`, em `internal/handler/handler.go`.
- Migration nova: incrementar a versão no array `migrations`, em
  `internal/store/store.go`. Nunca editar uma migration já aplicada.
- Rota nova: registrar em `internal/handler/routes.go`.
- Acréscimos de CSS deste projeto ficam **no fim** do `app.css`, abaixo do
  comentário de separação — o começo do arquivo é o design system herdado, e
  mantê-lo intacto permite compará-lo com a origem.

### Armadilhas que já custaram tempo

Vale ler antes de mexer no front:

- **Scripts de página rodam antes do `app.js`.** O `app.js` é carregado no
  `<head>` com `defer`, então executa depois do documento parseado. Toda
  inicialização numa página precisa estar dentro de `DOMContentLoaded`. Sem
  isso, estoura `is not defined` e o recurso falha **em silêncio**.
- **O Pico trata `<dialog>` como contêiner de tela cheia** (`display: flex;
  min-width: 100%`) e centraliza o primeiro filho. Todo diálogo precisa de um
  invólucro — `.dialog-box` ou `<form>`. Sem ele, o conteúdo vira itens de flex
  numa linha só. E `max-width` no `<dialog>` não resolve: `min-width` vence
  `max-width` em CSS.
- **O Pico declara `button[type=submit] { width: 100% }`**, especificidade
  (0,1,1), que vence qualquer classe simples. Botões de ícone precisam de
  `!important` na largura.
- **Cuidado com regras genéricas de elemento dentro de contêiner.** Uma regra
  como `.section-card label { display: block }` tem especificidade maior que
  `.minha-classe` e quebra componentes que usam `<label>` como cartão. Prefira
  `:not()` a empilhar `!important`.
- **A cor de destaque é injetada num `<style>` que precisa vir DEPOIS do
  `app.css`.** Os dois definem `--color-accent` em `:root` com a mesma
  especificidade; quem vem por último vence.
- **Estáticos vindos de `go:embed` têm mtime zero**, então o Go não emite
  `Last-Modified` nem `ETag` sozinho. O `cacheEstaticos` em `routes.go` supre
  isso com um `ETag` derivado da versão do build — sem ele, o navegador serve
  CSS antigo depois de atualizar o `.exe`.

## Licença

[MIT](LICENSE). Avisos de terceiros e de marcas em [NOTICE.md](NOTICE.md).
