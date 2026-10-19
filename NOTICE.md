# Avisos de terceiros e de marcas

## Componentes de terceiros

### Pico CSS v2.1.1 — MIT
`static/vendor/pico.classless.min.css`

```
Pico CSS v2.1.1 (https://picocss.com)
Copyright 2019-2025 - Licensed under MIT
```

Vendorizado no repositório, e não carregado de CDN, para que o executável
funcione sem acesso à rede. O cabeçalho de copyright foi preservado no arquivo
minificado, como a licença MIT exige.

### modernc.org/sqlite — BSD-3-Clause
Dependência Go (ver `go.mod`). Implementação de SQLite em Go puro, sem cgo.
Inclui código transpilado do SQLite original, que é de **domínio público**.

A árvore completa de dependências e suas licenças pode ser listada com:

```bash
go install github.com/google/go-licenses@latest
go-licenses report ./...
```

### Ícones
Os ícones em `static/icons.svg` foram desenhados para este projeto, no estilo
de traço de 1,75px comum a bibliotecas como Feather e Lucide. Nenhum arquivo
dessas bibliotecas foi copiado.

### Design system
`static/app.css` deriva do design system do projeto **CtrlLicença**
(MIT, Copyright (c) 2026 Yuri Bulhões Bertoldi), com alterações substanciais de
tipografia, paleta e densidade. A licença MIT de origem é compatível com a
deste projeto, e a atribuição está registrada aqui.

### Fontes
Nenhuma fonte é distribuída com o projeto. A interface usa a pilha de fontes do
sistema operacional (Segoe UI Variable no Windows 11, com alternativas para
outros sistemas).

---

## Marcas

**Docker** e o logotipo do Docker são marcas registradas da **Docker, Inc.**

Este projeto **não é afiliado, patrocinado nem endossado pela Docker, Inc.**
É um cliente independente da Docker Engine API, distribuído sob licença MIT.

Nenhum logotipo, ícone de baleia ou outro elemento visual de marca da Docker,
Inc. é distribuído neste repositório. A cor usada como padrão (`#2496ed`) é
configurável pelo usuário em *Configurações*.

O nome "Docker" aparece aqui apenas em sentido descritivo — para indicar com
qual software o projeto interopera —, uso que as diretrizes de marca da Docker
admitem. Caso você publique um fork sob outro nome, recomenda-se evitar nomes de
produto que comecem com "Docker".
