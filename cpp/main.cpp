/**
 * Avena — String Filter (C++)
 * Подключается к NATS через сырой TCP (NATS text protocol).
 * Подписывается на discord.event.message_create,
 * публикует moderation.violation при нахождении запрещённых слов.
 * Зависимости: только POSIX сокеты (нет внешних библиотек).
 */
#include <algorithm>
#include <arpa/inet.h>
#include <cstdlib>
#include <cstring>
#include <iostream>
#include <netdb.h>
#include <netinet/in.h>
#include <sstream>
#include <string>
#include <sys/socket.h>
#include <unistd.h>
#include <vector>

// ── Список запрещённых слов (добавляйте по необходимости) ─────────────────
static const std::vector<std::string> BAD_WORDS = {
    // "badword1", "badword2"
};

// ── Проверка текста ────────────────────────────────────────────────────────
static bool contains_violation(const std::string& text) {
    std::string lower = text;
    std::transform(lower.begin(), lower.end(), lower.begin(), ::tolower);
    for (const auto& word : BAD_WORDS) {
        if (lower.find(word) != std::string::npos) return true;
    }
    return false;
}

// ── Разбор nats://host:port ────────────────────────────────────────────────
static void parse_nats_url(const std::string& url,
                            std::string& host, int& port) {
    std::string s = url;
    if (s.compare(0, 7, "nats://") == 0) s = s.substr(7);
    size_t colon = s.find(':');
    if (colon == std::string::npos) {
        host = s;
        port = 4222;
    } else {
        host = s.substr(0, colon);
        try   { port = std::stoi(s.substr(colon + 1)); }
        catch (...) { port = 4222; }
    }
}

// ── Минимальный NATS-клиент через TCP ─────────────────────────────────────
class NatsClient {
    int  fd_ = -1;
    std::string buf_;   // read-buffer

    // Читать точно n байт (+ финальный \r\n)
    std::string read_bytes(size_t n) {
        while (buf_.size() < n + 2) {
            char tmp[4096];
            int got = recv(fd_, tmp, sizeof(tmp), 0);
            if (got <= 0) return {};
            buf_.append(tmp, got);
        }
        std::string data = buf_.substr(0, n);
        buf_.erase(0, n + 2);   // убираем \r\n
        return data;
    }

    // Читать строку до \r\n
    std::string read_line() {
        for (;;) {
            size_t pos = buf_.find("\r\n");
            if (pos != std::string::npos) {
                std::string line = buf_.substr(0, pos);
                buf_.erase(0, pos + 2);
                return line;
            }
            char tmp[4096];
            int got = recv(fd_, tmp, sizeof(tmp), 0);
            if (got <= 0) return {};
            buf_.append(tmp, got);
        }
    }

    bool send_raw(const std::string& s) {
        return send(fd_, s.c_str(), s.size(), 0) > 0;
    }

public:
    // Подключение + хэндшейк NATS
    bool connect(const std::string& host, int port) {
        struct hostent* he = gethostbyname(host.c_str());
        if (!he) {
            std::cerr << "[C++] Не удалось разрешить хост: " << host << "\n";
            return false;
        }
        fd_ = socket(AF_INET, SOCK_STREAM, 0);
        if (fd_ < 0) return false;

        struct sockaddr_in sa{};
        sa.sin_family = AF_INET;
        sa.sin_port   = htons(port);
        std::memcpy(&sa.sin_addr, he->h_addr_list[0], he->h_length);

        if (::connect(fd_, (struct sockaddr*)&sa, sizeof(sa)) < 0) {
            std::cerr << "[C++] Ошибка подключения к NATS\n";
            return false;
        }

        // Ждём INFO
        std::string info = read_line();
        if (info.compare(0, 4, "INFO") != 0) {
            std::cerr << "[C++] Ожидали INFO, получили: " << info << "\n";
            return false;
        }

        // CONNECT + PING
        send_raw(
            "CONNECT {\"verbose\":false,\"pedantic\":false,"
            "\"lang\":\"cpp\",\"version\":\"1.0\",\"protocol\":1}\r\n"
            "PING\r\n"
        );

        // Ждём PONG (может быть +OK перед PONG)
        for (int i = 0; i < 5; ++i) {
            std::string line = read_line();
            if (line == "PONG") {
                std::cout << "[C++] Подключено к NATS " << host << ":" << port << "\n";
                return true;
            }
        }
        std::cerr << "[C++] Не получили PONG\n";
        return false;
    }

    void subscribe(const std::string& subject, int sid = 1) {
        send_raw("SUB " + subject + " " + std::to_string(sid) + "\r\n");
        std::cout << "[C++] Подписан на: " << subject << "\n";
    }

    void publish(const std::string& subject, const std::string& payload) {
        std::string cmd =
            "PUB " + subject + " " +
            std::to_string(payload.size()) + "\r\n" +
            payload + "\r\n";
        send_raw(cmd);
    }

    // Блокирующее ожидание следующего сообщения
    // Возвращает false при закрытии соединения
    bool next(std::string& subject, std::string& payload) {
        for (;;) {
            std::string line = read_line();
            if (line.empty()) return false;

            // Отвечаем на серверные PING
            if (line == "PING") {
                send_raw("PONG\r\n");
                continue;
            }

            // MSG <subject> <sid> [reply-to] <bytes>
            if (line.compare(0, 3, "MSG") == 0) {
                std::istringstream iss(line);
                std::string tag, subj, sid, field1, field2;
                iss >> tag >> subj >> sid >> field1;

                subject = subj;
                int bytes;
                try {
                    bytes = std::stoi(field1);
                } catch (...) {
                    // field1 — reply-to, читаем следующее поле
                    iss >> field2;
                    try   { bytes = std::stoi(field2); }
                    catch (...) { continue; }
                }
                payload = read_bytes(bytes);
                return true;
            }
        }
    }

    ~NatsClient() {
        if (fd_ >= 0) close(fd_);
    }
};

// ── Простой JSON-парсер: извлечь строку по ключу ─────────────────────────
static std::string json_str(const std::string& json, const std::string& key) {
    std::string search = "\"" + key + "\":\"";
    size_t pos = json.find(search);
    if (pos == std::string::npos) return {};
    pos += search.size();
    size_t end = json.find('"', pos);
    if (end == std::string::npos) return {};
    return json.substr(pos, end - pos);
}

// ── main ──────────────────────────────────────────────────────────────────
int main() {
    const char* env_url = std::getenv("NATS_URL");
    std::string nats_url = env_url ? env_url : "nats://nats:4222";

    std::string host;
    int port = 4222;
    parse_nats_url(nats_url, host, port);

    NatsClient client;
    if (!client.connect(host, port)) {
        std::cerr << "[C++] Не удалось подключиться к NATS\n";
        return 1;
    }

    client.subscribe("discord.event.message_create");
    std::cout << "[C++] String-Filter запущен ✅\n";

    std::string subject, payload;
    while (client.next(subject, payload)) {
        std::string content = json_str(payload, "content");
        if (content.empty()) continue;

        if (contains_violation(content)) {
            client.publish("moderation.violation", payload);
            std::cout << "[C++] Нарушение обнаружено\n";
        }
    }

    std::cerr << "[C++] Соединение закрыто\n";
    return 0;
}
