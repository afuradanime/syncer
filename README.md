# Sincronizador da base de dados

## Como formar a base de dados de animes
A base de dados consiste inteiramente de informação open source. A informação da base de dados é proveniente do site [MyAnimeList](https://myanimelist.net/), extraída pelo antigo _API_ não oficial [Jikan](https://jikan.moe/), novo [Tenrai](https://tenrai.org/).

> [!WARNING]  
> O processo de importar animes pode demorar mais de 20 minutos.

## Documentação
### [Requisitos](./docs/01%20-%20requirements.md)
### [Modelo lógico da base de dados](./docs/02%20-%20database%20model.md)
### [Modelo físico da base de dados](./docs/03%20-%20physical%20database%20model.sql)

## Setup

### Variáveis de Ambiente

O syncer utiliza as seguintes variáveis de ambiente:

- `DATABASE_PATH`: Caminho para o ficheiro da base de dados SQLite
- `TENRAI_ENDPOINT`: URL da API Tenrai (opcional, padrão: `https://tenrai.org/api`)

### Ficheiro de configuração

Um exemplo de ficheiro de configuração (`config.ini`) pode ser:

```ini
[Import]
image_type=webp

use_quality_filtering=true
quality_tiers=3

blacklisted_tags=hentai
blacklisted_types=Music,Unknown
blacklisted_studios=Studio Ghibli
blacklisted_producers=Toei Animation

request_timeout=30
max_attempts=3
base_backoff=5
sleep_amount=5
```

### Execução

Usar Makefile para executar as tarefas do syncer ou executar diretamente o binário gerado com as seguintes opções:

```bash
Usage of ./bin/syncer:
  -config string
        Path to the config file (default "config.ini")
  -mode string
        Mode to run the program as (default "full")
```

## Targets
1. **build**: Buildar a app
2. **run-full**: Corre o syncer para inserir todos os animes e mangas
3. **run-partial**: Corre o syncer para re-inserir todos os animes e mangas que estejam a sair no momento e que estejam em falta à versão actual da base de dados
4. **test**: Corre testes unitários
5. **test-system**: Corre testes de sistema
6. **test-race**: Corre testes de race conditions
7. **fmt**: Formata o código
8. **vet**: Corre o go vet
9. **lint**: Faz análise estática
10. **check**: fmt, vet, lint, testes
11. **tidy**: Atualiza dependências do Go
12. **clean**: Remove artefatos de build
