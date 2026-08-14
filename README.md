# nxs-backup

> **⚠️ Форк.** Это форк [nixys/nxs-backup](https://github.com/nixys/nxs-backup), созданный 2026-08-03.
> Пока используется только для личных нужд, изменения вносятся понемногу и без гарантий совместимости с оригиналом.
> ну и да, это ~~нейрослоп~~ код сгенерированный нейросетями, где я и где этот ваш Go

## Что изменено в форке

**Новое:**

- **Итоговый отчёт в Telegram** — одно сообщение по завершении всего прогона: результаты джоб, размер
  созданных бэкапов и насколько они выросли с прошлого раза. Детали каждой джобы спрятаны под сворачиваемую
  цитату, ошибки собраны в отдельный блок. Опционально, настраивается чат и тред. Подробнее — [Final report](#final-report).
- **Heartbeat-пинги** — дёргает вебхуки внешних мониторингов (push-мониторы Uptime Kuma, healthchecks.io и
  подобные), которые поднимают тревогу, если их не позвать раз в N времени. Так алерт сработает и когда бэкап
  не запустился вовсе. Можно привязывать к конкретным джобам. Подробнее — [Heartbeats](#heartbeats).

**Исправления:**

- **Redis: бэкап не работал при заданном пароле** — проверка подключения выполнялась без авторизации, и джоба
  падала с `NOAUTH` ещё на старте.
- **SFTP: мусор от инкрементных бэкапов при ротации** — устаревшие каталоги не удалялись, потому что удаление
  было нерекурсивным и спотыкалось о непустые директории. Остальные хранилища этим не страдали.

Для локальной сборки в Docker (Go-тулчейн на машине не нужен) — см. [CLAUDE.md](CLAUDE.md).

nxs-backup is a tool for creating and delivery backups, rotating it locally and on remote storages, compatible with
GNU/Linux distributions.

## Introduction

### Features

- Full data backup
  - File backups:
    - Discrete files backups
    - Incremental files backups
  - Database backups:
    - Logical backups of MySQL/Percona (5.7/8.0/8.4/_all versions_)
    - Logical backups of MariaDB (10/11/_all versions_)
    - Physical backups by Xtrabackup (2.4/8.0) of MySQL/Percona (5.7/8.0/8.4/_all versions_)
    - Physical backups by MariaDB-backup of MariaDB (10/11/12/_all versions_)
    - Logical backups of PostgreSQL (9/10/11/12/13/14/15/16/17/18/_all versions_)
    - Physical backups by Basebackups of PostgreSQL (9/10/11/12/13/14/15/16/17/18/_all versions_)
    - Backups of MongoDB (4.0/4.2/4.4/5.0/6.0/7.0/8.0/_all versions_)
    - Backups of Redis (_all versions_)
    - Backups of Clickhouse (experimental) 
  - Support of user-defined scripts that extend functionality
- Upload and manage backups to the remote storages:
  - S3 (Simple Storage Service that provides object storage through a web interface. Supported by clouds e.g. AWS, GCP)
  - SSH (SFTP)
  - FTP
  - CIFS (SMB)
  - NFS
  - WebDAV
- Fine-tune the database backup process with additional options for optimization purposes
- Notifications about events of the backup process via email and webhooks
- Optional final report with jobs results and backups size statistics sent to Telegram (see [below](#final-report))
- Heartbeats for the external "dead man's switch" monitorings, e.g. push monitors of the Uptime Kuma (see [below](#heartbeats))
- Collect, export, and save metrics in Prometheus-compatible format
- Limiting resource consumption:
  - CPU usage
  - local disk rate
  - remote storage rate

### Who can use the tool?

- System Administrators
- DevOps Engineers
- Developers
- Anybody who need to do regular backups

## Quickstart

- Clone the repo
  ```sh
  git clone https://github.com/nixys/nxs-backup.git
  ```
  
### On-premise (bare-metal or virtual machine)

- Go to on-premise directory
  ```sh
  cd nxs-backup/.deploy/on-premise/
  ```
- Install nxs-backup, just download and unpack archive for your CPU architecture.
  ```sh
  curl -L https://github.com/nixys/nxs-backup/releases/latest/download/nxs-backup-amd64.tar.gz -o /tmp/nxs-backup.tar.gz
  tar xf /tmp/nxs-backup.tar.gz -C /tmp
  sudo mv /tmp/nxs-backup /usr/sbin/nxs-backup
  sudo chown root:root /usr/sbin/nxs-backup
  ```
> [!NOTE]
> nxs-backup is built for the following processor architectures: amd64 (x86_64), arm (armv8), arm64 (aarch64).
> If you need specific version of nxs-backup, or different architecture, you can find it on [release page](https://github.com/nixys/nxs-backup/releases).
- Check that installation successful:
  ```sh
  sudo nxs-backup --version
  ```
- Generate configuration files like described [here](https://nxs-backup.io/documentation/stable/2-3-1-on-premise/) or update
  provided `nxs-backup.conf` and jobs configs in `cond.d` dir with your parameters (see [docs](https://nxs-backup.io/documentation/stable/3-1-preparing-of-the-main-config/) for details)
- For starting nxs-backup process run:
  ```sh
  sudo nxs-backup start
  ```

### Docker-compose

- Go to docker compose directory
  ```sh
  cd nxs-backup/.deploy/docker-compose/
  ```
- Update provided `nxs-backup.conf` file with your parameters (see [docs](https://nxs-backup.io/documentation/stable/3-1-preparing-of-the-main-config/) for details)
- Launch the nxs-backup with command:
  ```sh
  docker compose up -d --pull
  ```

### Kubernetes

- Go to kubernetes directory
  ```sh
  cd nxs-backup/.deploy/kubernetes/
  ```
- Install [nxs-universal-chart](https://github.com/nixys/nxs-universal-chart) (`Helm 3` is required):
  ```sh
  helm repo add nixys https://registry.nixys.io/chartrepo/public
  ```
- Find examples of `helm values` [here](/docs/example/kubernetes/README.md)
- Fill up your `values.yaml` with correct nxs-backup [configs](https://nxs-backup.io/documentation/stable/3-1-preparing-of-the-main-config/)
- Launch nxs-backup with command:
  ```sh
  helm -n $NAMESPACE_SERVICE_NAME install nxs-backup nixys/nxs-universal-chart -f values.yaml
  ```
  where $NAMESPACE_SERVICE_NAME is the namespace in which to back up your data

## Final report

In addition to the per-event notifications, nxs-backup can send a single summary message when the whole
`nxs-backup start` run is finished. The report contains the results of the performed jobs and, when the data is
available, the size of the created backups and how much they have grown since the previous run.

The report is disabled by default and is delivered to the only channel — a Telegram chat. The message is formatted
with MarkdownV2, so the details of every job are hidden under an expandable block quotation. If Telegram is unable to
parse the markup, the message is resent as a plain text. Messages that are longer than the Telegram limit are split
by the job boundaries.

Add the `report` section to the `notifications` block of the main config:

```yaml
notifications:
  report:
    # send the final report, disabled by default
    enabled: true
    # send the report only if the run finished with errors
    only_on_errors: false
    # include the per-target details (size, growth, duration, delivery status)
    include_targets: true
    # sizes of the previous run backups are stored here to calculate the growth
    state_file: /var/lib/nxs-backup/report.state
    telegram:
      bot_token: 'ENV:TG_BOT_TOKEN'
      # numeric chat id or `@channelname`
      chat_id: '-1001234567890'
      # optional, id of the topic in a forum supergroup
      thread_id: 42
      # optional, defaults
      api_url: https://api.telegram.org
      insecure_tls: false
      disable_notification: false
```

Notes:

- The report doesn't depend on the `server.metrics` settings, it always uses the data collected during the run.
- The backups growth is calculated per target against the last **successful** backup of that target. It's not shown
  until the second run, or if the `state_file` can't be read or written.
- The reported size is the size of the backups created by this run, not the total amount of the data stored
  on the remote storages.

## Heartbeats

Monitorings like the Uptime Kuma push monitors, healthchecks.io or Cronitor work the other way around: they give you
an URL that your job has to call on every run, and raise an alert if the call doesn't come in the expected period of
time. So the alert fires not only when the backup fails, but also when it hasn't been started at all — the cron is
broken, the host is down, the process hangs.

nxs-backup pings such URLs when the `nxs-backup start` run is finished:

```yaml
notifications:
  heartbeats:
  - name: kuma
    enabled: true
    # pinged when the run is finished
    url: https://kuma.example.com/api/push/abc123
    # when to ping the `url`: always (default) | success | failure
    trigger: always
    # optional, pinged instead of the `url` when the run failed (e.g. healthchecks.io `/fail`).
    # It's used regardless of the `trigger`
    failure_url: ''
    # optional, pinged right after the backup is started (e.g. healthchecks.io `/start`)
    start_url: ''
    # optional, report only about the certain jobs instead of the whole run
    jobs: []
    # optional, defaults
    method: GET
    status_params: true
    insecure_tls: false
    connection_timeout: 10
    extra_headers: {}
```

With `status_params: true` (default) the `status=up|down`, `msg=<summary>` and `ping=<duration in ms>` query params
are added to the pinged URL — that's what the Uptime Kuma push monitor expects. Params that are already present in the
configured URL are never overwritten, and services that don't know these params (healthchecks.io) just ignore them.
Set it to `false` to ping the URL exactly as it's written in the config.

Notes:

- A failed ping is retried 3 times, so a short network glitch doesn't raise a false alarm.
- `jobs` binds the heartbeat to the certain jobs: the status is calculated from their results only. If none of them
  has been performed during the run (a different job was started, or today is not a backup day for them),
  the heartbeat is not pinged at all — the monitoring interval has to be set with that in mind.
- Set `trigger: success` to get the classic dead man's switch behaviour: a failed run pings nothing and the alert
  fires by the monitoring timeout.

## Documentation

For detailed information on using the tool please visit [nxs-backup.io](https://nxs-backup.io).

## Roadmap

Following features are already in backlog for our development team and will be released soon:

- Encrypting of backups
- Restore backups by nxs-backup
- API for remote management and metrics monitoring
- Web interface for management
- Proprietary startup scheduler
- New backup types (Clickhouse, Elastic, lvm, etc.)
- Programmatic implementation of backup creation instead of calling external utilities

## Feedback

For support and feedback please contact me:

- Telegram: [@r_andreev](https://t.me/r_andreev)
- Email: r.andreev@nixys.io

For news and discussions subscribe the channels:

- Telegram community (news): [@nxs_backup](https://t.me/nxs_backup)
- Telegram community (chat): [@nxs_backup_chat](https://t.me/nxs_backup_chat)

## License

nxs-backup is released under the [Apache-2.0 license](LICENSE).
