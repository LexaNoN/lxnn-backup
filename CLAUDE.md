# Локальная сборка

Личный форк nxs-backup. Go-тулчейн на машине **не установлен** — всё собирается в Docker
(Docker Desktop, при необходимости запустить: `"%ProgramFiles%\Docker\Docker\Docker Desktop.exe"`).

## Образ для сервера

Основной способ. `.docker/Dockerfile-local` — multi-stage (`golang:1.25` → `debian:12-slim`
с `redis-cli`, `mysql-client`, `postgresql-client`, `mariadb-backup`), аналог официального
`nixyslab/nxs-backup:latest`. Файл в `.gitignore`, живёт только локально.

```bash
docker build -f .docker/Dockerfile-local -t nxs-backup:local --build-arg VERSION=1.0.0-local .
```

Проверка: `docker run --rm nxs-backup:local nxs-backup --version`

Перенос на сервер без пересборки:

```bash
docker save nxs-backup:local | gzip > nxs-backup-local.tar.gz
```

Дальше `scp` архива на сервер, там `gunzip -c nxs-backup-local.tar.gz | docker load`,
в `docker-compose.yaml` — `image: nxs-backup:local`, затем `docker compose up -d`.
Тег без registry, поэтому `docker compose pull` его не перетрёт.

## Быстрая проверка кода (без образа)

```bash
docker run --rm -v "C:\!code_sync\lxnn-backup:/src" -v lxnn-gomod:/go/pkg/mod -v lxnn-gobuild:/root/.cache/go-build -w /src -e CGO_ENABLED=0 golang:1.25 sh -c "go build ./... && go vet ./modules/..."
```

Именованные volume'ы кешируют модули и билд-кеш — без них каждый запуск заново тянет все зависимости.

Длинные `sh -c` через PowerShell ломаются на экранировании кавычек: класть скрипт в файл
и монтировать (`-v <dir>:/out ... sh /out/build.sh`), вывод писать в файл — иначе он обрезается.

## Грабли

- **`misc/version.go` отсутствует в репозитории** — генерируется из `misc/version.go.in`
  (`sed "s@APP_VERSION@<версия>@"`). Без него `go build` падает на `undefined: misc.VERSION`.
  Dockerfile-local делает это сам; при ручной сборке файл нужно создать (он в `.gitignore`).
- **`gofmt -l` помечает весь репозиторий** — рабочая копия выкачана с CRLF. Это не признак
  проблем с форматированием, ориентироваться на него бесполезно.
- **`gh pr create` без `--repo` целится в апстрим `nixys/nxs-backup`** (в `.git/config` у remote
  `upstream` стоит `gh-resolved = base`). Лечится один раз: `gh repo set-default LexaNoN/lxnn-backup`
  (уже применено).
