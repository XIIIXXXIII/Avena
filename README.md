# Avena

Многоязычный Discord-бот с микросервисной архитектурой. Без баз данных, полностью open-source.

## Архитектура

```
Discord ←WebSocket→ [Go Gateway] ←NATS→ [Python Logic Engine]
                                  ←NATS→ [Rust Moderation Executor] → Discord REST API
                                  ←NATS→ [C++ String Filter]
```

| Сервис | Язык | Роль |
|---|---|---|
| `gateway` | Go | Discord WebSocket, регистрация команд, NATS-роутер |
| `logic-engine` | Python | Обработка slash-команд, роутинг |
| `executor` | Rust | Discord REST API (бан, кик, таймаут, etc.) |
| `string-filter` | C++ | Контент-фильтр сообщений |
| `nats` | — | Message bus (нет прямых связей между сервисами) |

## Команды

| Команда | Описание | Права |
|---|---|---|
| `/ping` | Проверка задержки | — |
| `/info` | Системная информация | — |
| `/owner` | Статус владельца (ephemeral) | — |
| `/ban` | Забанить участника | Ban Members |
| `/kick` | Кикнуть участника | Kick Members |
| `/timeout` | Таймаут (1–40320 мин) | Moderate Members |
| `/unban` | Разбанить по ID | Ban Members |
| `/purge` | Удалить сообщения (1–100) | Manage Messages |
| `/role` | Выдать/убрать роль | Manage Roles |

## Быстрый старт

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/XIIIXXXIII/Avena/main/install.sh)
```

### Ручная установка

```bash
git clone https://github.com/XIIIXXXIII/Avena.git && cd Avena

cat > .env << EOF
DISCORD_TOKEN="ваш_токен"
GUILD_ID="id_сервера_для_тестов"   # пусто = глобальные команды
OWNER_ID="ваш_discord_id"
EOF

docker compose up -d --build
```

## Тестирование vs Продакшн

- **`GUILD_ID` задан** → команды в этом сервере появляются **мгновенно** (удобно при разработке)  
- **`GUILD_ID` пуст** → глобальные команды, обновляются **до 1 часа**

## NATS Topics

| Topic | Откуда | Куда |
|---|---|---|
| `discord.interaction.create` | Go | Python |
| `discord.interaction.respond` | Python/Rust | Go |
| `discord.moderation.*` | Python | Rust |
| `discord.event.message_create` | Go | C++ |
| `moderation.violation` | C++ | Python/Go |

## Логи

```bash
docker compose logs -f            # все сервисы
docker compose logs -f gateway    # только Go
docker compose logs -f executor   # только Rust
```

## Лицензия

MIT
