import asyncio
import json
import os
from nats.aio.client import Client as NATS

async def main():
    nc = NATS()
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    
    await nc.connect(nats_url)
    print(f"Logic-Engine (Python) connected to NATS at {nats_url}")

    async def message_handler(msg):
        data = json.loads(msg.data.decode())
        content = data.get("content", "")
        channel_id = data.get("channel_id")

        if content.startswith("/info"):
            response = {
                "channel_id": channel_id,
                "content": "Avena Hyper-Bot Status:\n- Architecture: Distributed Microservices\n- Nodes: Rust, Go, Python, C++\n- Database: None (Stateless)\n- Environment: Zotac Cluster"
            }
            await nc.publish("discord.api.send_message", json.dumps(response).encode())

    await nc.subscribe("discord.event.message_create", cb=message_handler)

    while True:
        await asyncio.sleep(1)

if __name__ == "__main__":
    asyncio.run(main())
