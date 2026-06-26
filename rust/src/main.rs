use discord_ws::{Shard, ShardSettings, GatewayEvent, OpCode};
use tokio::sync::mpsc;
use tokio::time::{self, Duration};
use nats::asynk::Connection;
use env_logger;
use log::{info, error};
use serde_json::to_string;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    env_logger::init();
    info!("Starting Gateway-Watcher service...");

    let discord_token = std::env::var("DISCORD_TOKEN")
        .expect("DISCORD_TOKEN must be set");
    let nats_url = std::env::var("NATS_URL")
        .unwrap_or_else(|_| "nats://localhost:4222".to_string());

    let nats_conn = Connection::connect(&nats_url).await?;
    info!("Connected to NATS at {}", nats_url);

    let settings = ShardSettings::new(discord_token.clone());
    let (mut shard, mut events) = Shard::new(settings);

    info!("Connecting to Discord Gateway...");
    shard.start().await?;
    info!("Connected to Discord Gateway.");

    // Main event loop
    loop {
        tokio::select! {
            Some(event) = events.recv() => {
                match event {
                    GatewayEvent::Dispatch(seq, event_name, value) => {
                        info!("Received Discord event: {} (Seq: {})", event_name, seq);
                        let event_payload = to_string(&value)?;
                        let subject = format!("discord.event.{}", event_name.to_lowercase());
                        nats_conn.publish(&subject, event_payload.as_bytes()).await?;
                        info!("Published event {} to NATS subject {}", event_name, subject);
                    },
                    GatewayEvent::HeartbeatAck => {
                        info!("Heartbeat acknowledged.");
                    },
                    GatewayEvent::Ready(ready_event) => {
                        info!("Shard is ready! User: {}#{}", ready_event.user.username, ready_event.user.discriminator);
                        info!("Connected to {} guilds.", ready_event.guilds.len());
                        // In a real multi-shard setup, you'd handle sharding info here
                    },
                    GatewayEvent::Resumed => {
                        info!("Session resumed.");
                    },
                    GatewayEvent::InvalidSession(can_resume) => {
                        error!("Invalid session. Can resume: {}", can_resume);
                        if !can_resume {
                            error!("Cannot resume, reconnecting...");
                            shard.start().await?;
                        }
                    },
                    GatewayEvent::Reconnect => {
                        info!("Discord requested reconnect. Reconnecting...");
                        shard.start().await?;
                    },
                    GatewayEvent::Hello(hello_event) => {
                        info!("Received HELLO from Discord. Heartbeat interval: {}ms", hello_event.heartbeat_interval);
                    },
                    _ => {
                        // Handle other gateway events if necessary
                        // info!("Received other gateway event: {:?}", event);
                    }
                }
            }
            _ = time::sleep(Duration::from_secs(5)) => {
                // Periodically check if NATS connection is still alive, or other health checks
                // info!("Gateway-Watcher alive...");
            }
        }
    }
}
