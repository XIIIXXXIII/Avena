#!/bin/bash
# Avena — установщик
set -e

REPO="https://github.com/XIIIXXXIII/Avena.git"
INSTALL_DIR="/opt/avena"
SERVICE_NAME="avena"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

echo -e "${GREEN}"
cat << 'ART'
    ___                         
   /   |_   _____  ____  ____ _
  / /| | | / / _ \/ __ \/ __ `/
 / ___ | |/ /  __/ / / / /_/ / 
/_/  |_|___/\___/_/ /_/\__,_/  
ART
echo -e "${NC}"
echo "Установка Discord-бота Avena"
echo "────────────────────────────"

# Проверяем зависимости
for dep in docker git curl; do
    command -v "$dep" &>/dev/null || {
        echo -e "${RED}[✗] $dep не найден. Установите его и повторите.${NC}"
        exit 1
    }
done

# docker compose v2
docker compose version &>/dev/null || {
    echo -e "${RED}[✗] Требуется docker compose v2 (плагин).${NC}"
    exit 1
}

echo -e "${GREEN}[✓] Зависимости найдены${NC}"

# Удаляем старую установку
if [ -d "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}[!] Найдена старая установка в $INSTALL_DIR — удаляем...${NC}"
    cd "$INSTALL_DIR" && docker compose down 2>/dev/null || true
    rm -rf "$INSTALL_DIR"
fi

# Клонируем
echo "[→] Клонируем репозиторий..."
git clone "$REPO" "$INSTALL_DIR"
cd "$INSTALL_DIR"

# Собираем .env
echo ""
echo "─── Настройка ─────────────────────────────────"

read -rp "Discord Bot Token (обязательно): " DISCORD_TOKEN
if [ -z "$DISCORD_TOKEN" ]; then
    echo -e "${RED}[✗] DISCORD_TOKEN не может быть пустым.${NC}"
    exit 1
fi

read -rp "Guild ID для тестов (Enter = глобальные команды, до 1 часа): " GUILD_ID
read -rp "Ваш Discord User ID (для команды /owner, Enter = пропустить): " OWNER_ID

cat > .env << ENV
DISCORD_TOKEN="${DISCORD_TOKEN}"
GUILD_ID="${GUILD_ID}"
OWNER_ID="${OWNER_ID}"
ENV

chmod 600 .env
echo -e "${GREEN}[✓] .env создан${NC}"

# Собираем и запускаем
echo ""
echo "[→] Сборка контейнеров (первый раз может занять несколько минут)..."
docker compose build --parallel

echo "[→] Запуск сервисов..."
docker compose up -d

echo ""
echo -e "${GREEN}────────────────────────────────────────────${NC}"
echo -e "${GREEN}[✓] Avena запущен!${NC}"
echo ""
echo "Сервисы:"
docker compose ps --format "table {{.Service}}\t{{.Status}}"
echo ""
echo "Логи в реальном времени:  docker compose logs -f"
echo "Остановить:               docker compose down"
echo "Перезапустить:            docker compose restart"
echo ""

if [ -n "$GUILD_ID" ]; then
    echo -e "${YELLOW}[i] GUILD_ID задан — команды появятся в сервере мгновенно.${NC}"
else
    echo -e "${YELLOW}[i] GUILD_ID не задан — глобальные команды появятся до 1 часа.${NC}"
fi

# Опциональный systemd автостарт
echo ""
read -rp "Добавить автозапуск через systemd? [y/N] " AUTO
if [[ "$AUTO" =~ ^[Yy]$ ]]; then
    cat > "/etc/systemd/system/${SERVICE_NAME}.service" << SVC
[Unit]
Description=Avena Discord Bot
Requires=docker.service
After=docker.service

[Service]
WorkingDirectory=${INSTALL_DIR}
ExecStart=/usr/bin/docker compose up
ExecStop=/usr/bin/docker compose down
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SVC
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    echo -e "${GREEN}[✓] systemd юнит создан и включён${NC}"
fi

echo ""
echo -e "${GREEN}Готово! Добавьте бота на сервер с правами:${NC}"
echo "  applications.commands + bot (Administrator или нужные права)"
echo ""
