import asyncio
import json
import os
import platform
import psutil
from nats.aio.client import Client as NATS

OWNER_ID = "1459708545238044704"

async def main():
    nc = NATS()
    nats_url = os.getenv("NATS_URL", "nats://nats:4222")
    
    await nc.connect(nats_url)
    print(f"Logic-Engine (Python) connected to NATS at {nats_url}")

    async def interaction_handler(msg):
        data = json.loads(msg.data.decode())
        command_name = data.get('data', {}).get('name')
        user_id = data.get('member', {}).get('user', {}).get('id')
        username = data.get('member', {}).get('user', {}).get('username')
        interaction_token = data.get('token')
        interaction_id = data.get('id')

        print(f"Processing command: /{command_name} from {username}")

        response_content = "Unknown command"

        if command_name == "ping":
            response_content = "🏓 Pong! Avena Polyglot Cluster is alive."
        
        elif command_name == "info":
            cpu = psutil.cpu_percent()
            ram = psutil.virtual_memory().percent
            response_content = (
                f"📊 **System Status**\n"
                f"CPU: {cpu}%\n"
                f"RAM: {ram}%\n"
                f"OS: {platform.system()} {platform.release()}\n"
                f"Arch: Multi-language (Go/Python/NATS)"
            )
        
        elif command_name == "owner":
            if user_id == OWNER_ID:
                response_content = f"👑 Welcome back, Master {username}. All systems operational."
            else:
                response_content = "❌ Access denied. Only the creator can use this command."

        # Send response back to Gateway
        response_payload = {
            "interaction_id": interaction_id,
            "interaction_token": interaction_token,
            "content": response_content
        }
        await nc.publish("discord.interaction.respond", json.dumps(response_payload).encode())

    await nc.subscribe("discord.interaction.create", cb=interaction_handler)

    while True:
        await asyncio.sleep(1)

if __name__ == "__main__":
    asyncio.run(main())
