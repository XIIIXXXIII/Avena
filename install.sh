#!/bin/bash

# Avena Hyper-Bot Automated Installer
# Targets: Debian 12/13, Ubuntu 22.04+

set -e

echo "Starting Avena Hyper-Bot automated installation..."

# 1. Clean up old installations
echo "Cleaning up old Avena installations..."
sudo systemctl stop avena.service || true
sudo systemctl disable avena.service || true
sudo docker compose -f ~/Avena/docker-compose.yml down || true
sudo rm -rf ~/Avena_old ~/Avena

# 2. Clone new repository
echo "Cloning fresh repository..."
git clone https://github.com/XIIIXXXIII/Avena.git ~/Avena
cd ~/Avena

# 3. Setup environment
echo "Setting up environment..."
if [ -f .env ]; then
    echo ".env already exists, skipping..."
else
    # The token will be passed as an argument or prompted
    if [ -z "$1" ]; then
        read -p "Enter your Discord Token: " DISCORD_TOKEN
    else
        DISCORD_TOKEN=$1
    fi
    echo "DISCORD_TOKEN=\"$DISCORD_TOKEN\"" > .env
fi

# 4. Create systemd service for auto-start on boot
echo "Creating systemd service..."
sudo bash -c 'cat <<EOF > /etc/systemd/system/avena.service
[Unit]
Description=Avena Hyper-Bot Docker Cluster
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=simple
WorkingDirectory=/home/'$USER'/Avena
ExecStart=/usr/bin/docker compose up --build
ExecStop=/usr/bin/docker compose down
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF'

# 5. Create auto-update script
echo "Creating auto-update script..."
cat <<EOF > ~/Avena/update.sh
#!/bin/bash
cd ~/Avena
git fetch
LOCAL=\$(git rev-parse @)
REMOTE=\$(git rev-parse @{u})
if [ \$LOCAL != \$REMOTE ]; then
    echo "New version detected on GitHub. Updating..."
    git pull
    sudo systemctl restart avena.service
else
    echo "Avena is up to date."
fi
EOF
chmod +x ~/Avena/update.sh

# 6. Setup crontab for auto-update (every 5 minutes)
echo "Setting up crontab for auto-updates..."
(crontab -l 2>/dev/null | grep -v "update.sh"; echo "*/5 * * * * /home/$USER/Avena/update.sh >> /home/$USER/Avena/update.log 2>&1") | crontab -

# 7. Start the service
echo "Starting Avena service..."
sudo systemctl daemon-reload
sudo systemctl enable avena.service
sudo systemctl start avena.service

echo "-------------------------------------------------------"
echo "Installation Complete!"
echo "Avena will now start automatically on boot."
echo "It will check for updates from GitHub every 5 minutes."
echo "Check status: sudo systemctl status avena.service"
echo "Check logs: sudo docker compose -f ~/Avena/docker-compose.yml logs -f"
echo "-------------------------------------------------------"
