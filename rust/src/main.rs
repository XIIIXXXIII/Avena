//! Avena — Rust Moderation Executor
//! Подписывается на discord.moderation.* через NATS,
//! исполняет REST-запросы к Discord API v10,
//! возвращает ответ через discord.interaction.respond.

use async_nats::Client as NatsClient;
use chrono::Utc;
use futures::StreamExt;
use reqwest::Client as Http;
use serde::Deserialize;
use serde_json::{json, Value};
use std::{env, sync::Arc};
use tracing::{error, info};

// ── Структура запроса модерации от Python ─────────────────────────────────
#[derive(Deserialize, Clone)]
struct ModReq {
    interaction_id:    String,
    interaction_token: String,
    guild_id:          Option<String>,
    channel_id:        Option<String>,
    target_user_id:    Option<String>,
    role_id:           Option<String>,
    reason:            Option<String>,
    duration_minutes:  Option<i64>,
    amount:            Option<u64>,
}

// ── Shared state ──────────────────────────────────────────────────────────
struct State {
    http:  Http,
    nc:    NatsClient,
    token: String,
}

impl State {
    /// Универсальный Discord REST запрос
    async fn discord(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
    ) -> Result<Value, String> {
        let url = format!("https://discord.com/api/v10{}", path);
        let auth = format!("Bot {}", self.token);

        let builder = match method {
            "GET"    => self.http.get(&url),
            "POST"   => self.http.post(&url),
            "PUT"    => self.http.put(&url),
            "PATCH"  => self.http.patch(&url),
            "DELETE" => self.http.delete(&url),
            m        => return Err(format!("Неизвестный метод: {}", m)),
        };

        let mut req = builder.header("Authorization", auth);
        if let Some(b) = body {
            req = req.json(&b);
        }

        let res = req.send().await.map_err(|e| e.to_string())?;
        let status = res.status().as_u16();

        match status {
            200..=204 => Ok(json!({"ok": true})),
            _ => {
                let text = res.text().await.unwrap_or_default();
                Err(format!("Discord HTTP {}: {}", status, text))
            }
        }
    }

    /// Отправить ответ в Gateway через NATS
    async fn respond(&self, iid: &str, token: &str, content: &str) {
        let payload = json!({
            "interaction_id":    iid,
            "interaction_token": token,
            "content":           content,
        });
        let bytes = serde_json::to_vec(&payload).unwrap_or_default();
        if let Err(e) = self.nc.publish("discord.interaction.respond", bytes.into()).await {
            error!("NATS respond error: {}", e);
        }
    }
}

// ── Обработка одного запроса модерации ───────────────────────────────────
async fn handle(state: Arc<State>, subject: String, req: ModReq) {
    let iid    = req.interaction_id.clone();
    let token  = req.interaction_token.clone();
    let gid    = req.guild_id.clone().unwrap_or_default();
    let cid    = req.channel_id.clone().unwrap_or_default();
    let uid    = req.target_user_id.clone().unwrap_or_default();
    let rid    = req.role_id.clone().unwrap_or_default();
    let reason = req.reason.clone().unwrap_or_else(|| "Без причины".to_string());
    let mins   = req.duration_minutes.unwrap_or(5);
    let amount = req.amount.unwrap_or(10).min(100);

    let result: Result<Value, String> = match subject.as_str() {
        // ── /ban ──────────────────────────────────────────────────────
        "discord.moderation.ban" => {
            state.discord(
                "PUT",
                &format!("/guilds/{}/bans/{}", gid, uid),
                Some(json!({"delete_message_seconds": 0, "reason": reason})),
            ).await
        }

        // ── /kick ─────────────────────────────────────────────────────
        "discord.moderation.kick" => {
            state.discord(
                "DELETE",
                &format!("/guilds/{}/members/{}", gid, uid),
                None,
            ).await
        }

        // ── /timeout ──────────────────────────────────────────────────
        "discord.moderation.timeout" => {
            let until = (Utc::now() + chrono::Duration::minutes(mins)).to_rfc3339();
            state.discord(
                "PATCH",
                &format!("/guilds/{}/members/{}", gid, uid),
                Some(json!({"communication_disabled_until": until})),
            ).await
        }

        // ── /unban ────────────────────────────────────────────────────
        "discord.moderation.unban" => {
            state.discord(
                "DELETE",
                &format!("/guilds/{}/bans/{}", gid, uid),
                None,
            ).await
        }

        // ── /purge ────────────────────────────────────────────────────
        "discord.moderation.purge" => {
            // Шаг 1: получить ID сообщений
            let msgs = state.discord(
                "GET",
                &format!("/channels/{}/messages?limit={}", cid, amount),
                None,
            ).await;

            match msgs {
                Err(e) => Err(e),
                Ok(v) => {
                    let ids: Vec<Value> = v
                        .as_array()
                        .unwrap_or(&vec![])
                        .iter()
                        .filter_map(|m| m.get("id").cloned())
                        .collect();

                    if ids.is_empty() {
                        Err("Нет сообщений для удаления".to_string())
                    } else if ids.len() == 1 {
                        // bulk-delete требует >= 2, удаляем по одному
                        let id = ids[0].as_str().unwrap_or("");
                        state.discord(
                            "DELETE",
                            &format!("/channels/{}/messages/{}", cid, id),
                            None,
                        ).await
                    } else {
                        // Шаг 2: bulk-delete
                        state.discord(
                            "POST",
                            &format!("/channels/{}/messages/bulk-delete", cid),
                            Some(json!({"messages": ids})),
                        ).await
                    }
                }
            }
        }

        // ── /role ─────────────────────────────────────────────────────
        "discord.moderation.role" => {
            state.discord(
                "PUT",
                &format!("/guilds/{}/members/{}/roles/{}", gid, uid, rid),
                None,
            ).await
        }

        unknown => Err(format!("Неизвестный subject: {}", unknown)),
    };

    // Формируем текст ответа
    let reply = match result {
        Ok(_) => match subject.as_str() {
            "discord.moderation.ban"     => "✅ Пользователь забанен.".to_string(),
            "discord.moderation.kick"    => "✅ Пользователь кикнут.".to_string(),
            "discord.moderation.timeout" => format!("✅ Таймаут на {} мин.", mins),
            "discord.moderation.unban"   => "✅ Пользователь разбанен.".to_string(),
            "discord.moderation.purge"   => format!("✅ Удалено ~{} сообщений.", amount),
            "discord.moderation.role"    => "✅ Роль обновлена.".to_string(),
            _                            => "✅ Выполнено.".to_string(),
        },
        Err(e) => {
            error!("Ошибка {}: {}", subject, e);
            format!("❌ Ошибка: {}", e)
        }
    };

    state.respond(&iid, &token, &reply).await;
}

// ── Entry point ───────────────────────────────────────────────────────────
#[tokio::main]
async fn main() -> Result<(), async_nats::Error> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("avena_executor=info".parse().unwrap()),
        )
        .init();

    let token = env::var("DISCORD_TOKEN").expect("DISCORD_TOKEN не задан");
    let nats_url = env::var("NATS_URL")
        .unwrap_or_else(|_| "nats://nats:4222".to_string());

    let nc = async_nats::connect(&nats_url).await?;
    info!("Rust Executor подключён к NATS: {}", nats_url);

    let state = Arc::new(State {
        http:  Http::new(),
        nc:    nc.clone(),
        token,
    });

    // Список subjects для подписки
    let subjects = [
        "discord.moderation.ban",
        "discord.moderation.kick",
        "discord.moderation.timeout",
        "discord.moderation.unban",
        "discord.moderation.purge",
        "discord.moderation.role",
    ];

    let mut handles = Vec::new();

    for &subj in &subjects {
        let mut sub = nc.subscribe(subj).await?;
        let state   = state.clone();
        let subj_s  = subj.to_string();

        handles.push(tokio::spawn(async move {
            info!("Подписан на {}", subj_s);
            while let Some(msg) = sub.next().await {
                match serde_json::from_slice::<ModReq>(&msg.payload) {
                    Ok(req) => {
                        let s = state.clone();
                        let subj = subj_s.clone();
                        tokio::spawn(async move { handle(s, subj, req).await });
                    }
                    Err(e) => error!("Deserialize error [{}]: {}", subj_s, e),
                }
            }
        }));
    }

    info!("Avena Rust Executor запущен ✅");
    futures::future::join_all(handles).await;
    Ok(())
}
