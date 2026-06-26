#!/bin/bash

# Avena Hyper-Bot Automated Installer
# Optimized for Debian 12/13, Ubuntu 22.04+
# Supports both root and non-root users

set -e

echo "Starting Avena Hyper-Bot automated installation..."

# Determine home directory and user
if [ "$USER" = "root" ]; then
    HOME_DIR="/root"
    SUDO=""
else
    HOME_DIR="/home/$USER"
    SUDO="sudo"
fi

AVENA_DIR="$HOME_DIR/Avena"

# 1. Clean up old installations
echo "Cleaning up old Avena installations..."
$SUDO systemctl stop avena.service || true
$SUDO systemctl disable avena.service || true

# Try to stop docker containers if docker is present
if command -v docker &> /dev/null; then
    if [ -d "$AVENA_DIR" ]; then
        cd "$AVENA_DIR"
        $SUDO docker compose down || $SUDO docker-compose down || true
    fi
fi

$SUDO rm -rf "$HOME_DIR/Avena_old" "$AVENA_DIR"

# 2. Clone new repository
echo "Cloning fresh repository into $AVENA_DIR..."
git clone https://github.com/XIIIXXXIII/Avena.git "$AVENA_DIR"
cd "$AVENA_DIR"

# 3. Setup environment
echo "Setting up environment..."
if [ -f .env ]; then
    echo ".env already exists, skipping..."
else
    if [ -z "$1" ]; then
        read -p "Enter your Discord Token: " DISCORD_TOKEN
    else
        DISCORD_TOKEN=$1
    fi
    echo "DISCORD_TOKEN=\"$DISCORD_TOKEN\"" > .env
fi

# 4. Determine Docker Compose command
if docker compose version &> /dev/null; then
    DOCKER_COMPOSE_CMD="docker compose"
elif docker-compose version &> /dev/null; then
    DOCKER_COMPOSE_CMD="docker-compose"
else
    echo "Error: Docker Compose not found. Please install docker-compose or docker-compose-v2."
    exit 1
fi

# 5. Create systemd service
echo "Creating systemd service..."
$SUDO bash -c "cat <<EOF > /etc/systemd/system/avena.service
[Unit]
Description=Avena Hyper-Bot Docker Cluster
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=simple
WorkingDirectory=$AVENA_DIR
ExecStart=/usr/bin/$DOCKER_COMPOSE_CMD up --build
ExecStop=/usr/bin/$DOCKER_COMPOSE_CMD down
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF"

# 6. Create auto-update script
echo "Creating auto-update script..."
cat <<EOF > "$AVENA_DIR/update.sh"
#!/bin/bash
cd "$AVENA_DIR"
git fetch
LOCAL=\$(git rev-parse @)
REMOTE=\$(git rev-parse @{u})
if [ \$LOCAL != \$REMOTE ]; then
    echo "New version detected on GitHub. Updating..."
    git pull
    $SUDO systemctl restart avena.service
else
    echo "Avena is up to date."
fi
EOF
chmod +x "$AVENA_DIR/update.sh"

# 7. Setup crontab for auto-update
echo "Setting up crontab for auto-updates..."
(crontab -l 2>/dev/null | grep -v "update.sh"; echo "*/5 * * * * $AVENA_DIR/update.sh >> $AVENA_DIR/update.log 2>&1") | crontab -

# 8. Start the service
echo "Starting Avena service..."
$SUDO systemctl daemon-reload
$SUDO systemctl enable avena.service
$SUDO systemctl start avena.service

echo "-------------------------------------------------------"
echo "Installation Complete!"
echo "Avena will now start automatically on boot."
echo "Location: $AVENA_DIR"
echo "Check status: $SUDO systemctl status avena.service"
echo "Check logs: $SUDO $DOCKER_COMPOSE_CMD -f $AVENA_DIR/docker-compose.yml logs -f"
echo "-------------------------------------------------------"
