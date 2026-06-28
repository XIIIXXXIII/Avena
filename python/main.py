"""
Avena — Logic Engine (Python)
Обрабатывает slash-команды, роутит модерацию в Rust-executor через NATS.
"""
import asyncio
import json
import os
import platform
import psutil
import nats

OWNER_ID = os.getenv("OWNER_ID", "")


async def main() -> None:
    nats_url = os.getenv("NATS_URL", "nats://nats:4222")

    # ── NATS с reconnect ──────────────────────────────────────────────────
    async def on_error(e: Exception) -> None:
        print(f"[NATS] Ошибка: {e}")

    async def on_disconnect() -> None:
        print("[NATS] Отключён, ожидаем переподключения...")

    async def on_reconnect() -> None:
        print("[NATS] Переподключено ✅")

    nc = await nats.connect(
        nats_url,
        error_cb=on_error,
        disconnected_cb=on_disconnect,
        reconnected_cb=on_reconnect,
        max_reconnect_attempts=-1,   # бесконечно
        reconnect_time_wait=2,
    )
    print(f"[Logic Engine] Подключено к NATS: {nats_url}")

    # ── Хелпер: отправить ответ в Gateway через NATS ─────────────────────
    async def respond(iid: str, token: str, content: str) -> None:
        payload = json.dumps({
            "interaction_id":    iid,
            "interaction_token": token,
            "content":           content,
        }).encode()
        await nc.publish("discord.interaction.respond", payload)

    # ── Хелпер: направить в Rust-executor ────────────────────────────────
    async def to_executor(subject: str, data: dict) -> None:
        await nc.publish(subject, json.dumps(data).encode())

    # ── Обработчик команд ─────────────────────────────────────────────────
    async def handle_interaction(msg) -> None:
        try:
            raw = json.loads(msg.data.decode())
        except (json.JSONDecodeError, UnicodeDecodeError) as exc:
            print(f"[Logic Engine] Ошибка декодирования: {exc}")
            return

        cmd      = raw.get("command", "")
        iid      = raw.get("interaction_id", "")
        token    = raw.get("interaction_token", "")
        user_id  = raw.get("user_id", "")
        username = raw.get("username", "Unknown")
        guild_id = raw.get("guild_id", "")
        chan_id  = raw.get("channel_id", "")
        opts     = raw.get("options") or {}

        print(f"[Logic Engine] /{cmd} от {username} ({user_id})")

        # ── /ping ─────────────────────────────────────────────────────────
        if cmd == "ping":
            await respond(iid, token, "🏓 Pong! Avena в сети.")

        # ── /info ─────────────────────────────────────────────────────────
        elif cmd == "info":
            cpu  = psutil.cpu_percent(interval=None)
            ram  = psutil.virtual_memory()
            text = (
                f"📊 **Avena — Системная информация**\n"
                f"```\n"
                f"CPU  : {cpu:.1f}%\n"
                f"RAM  : {ram.percent:.1f}%  "
                f"({ram.used // 1024**2} MB / {ram.total // 1024**2} MB)\n"
                f"OS   : {platform.system()} {platform.release()}\n"
                f"Arch : Go · Python · Rust · C++\n"
                f"```"
            )
            await respond(iid, token, text)

        # ── /owner ────────────────────────────────────────────────────────
        elif cmd == "owner":
            if OWNER_ID and user_id == OWNER_ID:
                await respond(iid, token,
                    f"👑 Добро пожаловать, **{username}**. Все системы работают.")
            else:
                await respond(iid, token, "❌ Доступ запрещён.")

        # ── /ban ──────────────────────────────────────────────────────────
        elif cmd == "ban":
            await to_executor("discord.moderation.ban", {
                "guild_id":          guild_id,
                "target_user_id":    opts.get("user"),
                "reason":            opts.get("reason") or "Без причины",
                "interaction_id":    iid,
                "interaction_token": token,
                "moderator":         username,
            })

        # ── /kick ─────────────────────────────────────────────────────────
        elif cmd == "kick":
            await to_executor("discord.moderation.kick", {
                "guild_id":          guild_id,
                "target_user_id":    opts.get("user"),
                "reason":            opts.get("reason") or "Без причины",
                "interaction_id":    iid,
                "interaction_token": token,
                "moderator":         username,
            })

        # ── /timeout ──────────────────────────────────────────────────────
        elif cmd == "timeout":
            minutes = opts.get("minutes")
            try:
                minutes = int(minutes) if minutes is not None else 5
            except (ValueError, TypeError):
                minutes = 5
            await to_executor("discord.moderation.timeout", {
                "guild_id":          guild_id,
                "target_user_id":    opts.get("user"),
                "duration_minutes":  minutes,
                "interaction_id":    iid,
                "interaction_token": token,
            })

        # ── /unban ────────────────────────────────────────────────────────
        elif cmd == "unban":
            await to_executor("discord.moderation.unban", {
                "guild_id":          guild_id,
                "target_user_id":    opts.get("user_id"),
                "interaction_id":    iid,
                "interaction_token": token,
            })

        # ── /purge ────────────────────────────────────────────────────────
        elif cmd == "purge":
            amount = opts.get("amount")
            try:
                amount = max(1, min(100, int(amount))) if amount is not None else 10
            except (ValueError, TypeError):
                amount = 10
            await to_executor("discord.moderation.purge", {
                "channel_id":        chan_id,
                "amount":            amount,
                "interaction_id":    iid,
                "interaction_token": token,
            })

        # ── /role ─────────────────────────────────────────────────────────
        elif cmd == "role":
            await to_executor("discord.moderation.role", {
                "guild_id":          guild_id,
                "target_user_id":    opts.get("user"),
                "role_id":           opts.get("role"),
                "interaction_id":    iid,
                "interaction_token": token,
            })

        else:
            await respond(iid, token, f"❓ Неизвестная команда: `/{cmd}`")

    # ── Подписка ──────────────────────────────────────────────────────────
    await nc.subscribe("discord.interaction.create", cb=handle_interaction)
    print("[Logic Engine] Готов. Ожидаем команды...")

    try:
        while True:
            await asyncio.sleep(1)
    except asyncio.CancelledError:
        pass
    finally:
        await nc.drain()
        print("[Logic Engine] Завершение.")


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
