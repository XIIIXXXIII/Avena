use discord_ws::{Shard, ShardSettings, GatewayEvent};
use nats::asynk::Connection;
use log::{info, error};
use serde_json::{to_string, from_slice};
use reqwest::Client;
use std::env;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    env_logger::init();
    info!("Starting Avena Gateway & API Proxy (Rust)...");

    let discord_token = env::var("DISCORD_TOKEN").expect("DISCORD_TOKEN must be set");
    let nats_url = env::var("NATS_URL").unwrap_or_else(|_| "nats://localhost:4222".to_string());

    let nats_conn = Connection::connect(&nats_url).await?;
    info!("Connected to NATS at {}", nats_url);

    let http_client = Client::new();
    let token_header = format!("Bot {}", discord_token);

    // 1. API Proxy: Subscribe to outgoing messages from other microservices
    let nats_clone = nats_conn.clone();
    let token_clone = token_header.clone();
    let http_clone = http_client.clone();

    tokio::spawn(async move {
        if let Ok(sub) = nats_clone.subscribe("discord.api.send_message").await {
            info!("Subscribed to discord.api.send_message");
            while let Some(msg) = sub.next().await {
                if let Ok(payload) = from_slice::<serde_json::Value>(&msg.data) {
                    let channel_id = payload["channel_id"].as_str().unwrap_or("");
                    let content = payload["content"].as_str().unwrap_or("");
                    
                    if !channel_id.is_empty() && !content.is_empty() {
                        let url = format!("https://discord.com/api/v10/channels/{}/messages", channel_id);
                        let _ = http_clone.post(&url)
                            .header("Authorization", &token_clone)
                            .json(&serde_json::json!({ "content": content }))
                            .send()
                            .await;
                    }
                }
            }
        }
    });

    // 2. Gateway Watcher: Connect to Discord Gateway
    let settings = ShardSettings::new(discord_token);
    let (mut shard, mut events) = Shard::new(settings);

    shard.start().await?;
    info!("Connected to Discord Gateway.");

    while let Some(event) = events.recv().await {
        match event {
            GatewayEvent::Dispatch(_seq, event_name, value) => {
                let subject = format!("discord.event.{}", event_name.to_lowercase());
                if let Ok(payload) = to_string(&value) {
                    let _ = nats_conn.publish(&subject, payload.as_bytes()).await;
                }
            },
            _ => {}
        }
    }

    Ok(())
}
