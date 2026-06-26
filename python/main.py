import asyncio
import json
import os
import platform
import psutil
from nats.aio.client import Client as NATS

OWNER_ID = "1459708545238044704"

async def main():
    nc = NATS()
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")
    
    await nc.connect(nats_url)
    print(f"Logic-Engine (Python) connected to NATS at {nats_url}")

    async def message_handler(msg):
        data = json.loads(msg.data.decode())
        content = data.get("content", "")
        channel_id = data.get("channel_id")
        author_id = data.get("author", {}).get("id")

        if content == "/info":
            response = {
                "channel_id": channel_id,
                "content": (
                    "**Avena Hyper-Bot Node: Python**\n"
                    f"OS: {platform.system()} {platform.release()}\n"
                    f"CPU Usage: {psutil.cpu_percent()}%\n"
                    f"RAM Usage: {psutil.virtual_memory().percent}%\n"
                    "Architecture: Distributed Polyglot Cluster"
                )
            }
            await nc.publish("discord.api.send_message", json.dumps(response).encode())

        elif content == "/owner" and author_id == OWNER_ID:
            response = {
                "channel_id": channel_id,
                "content": "Access Granted. Welcome back, Master XIIIXXXIII."
            }
            await nc.publish("discord.api.send_message", json.dumps(response).encode())

    await nc.subscribe("discord.event.message_create", cb=message_handler)

    while True:
        await asyncio.sleep(1)

if __name__ == "__main__":
    asyncio.run(main())
