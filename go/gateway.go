package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	nats "github.com/nats-io/nats.go"
)

// ── thread-safe хранилище Application ID ──────────────────────────────────
var (
	mu    sync.RWMutex
	appID string
)

// ── хелперы ───────────────────────────────────────────────────────────────
func perm(p int64) *int64    { return &p }
func fptr(f float64) *float64 { return &f }

// ── список команд ─────────────────────────────────────────────────────────
var commands = []*discordgo.ApplicationCommand{
	{Name: "ping", Description: "Проверить задержку бота"},
	{Name: "info", Description: "Информация о системе"},
	{Name: "owner", Description: "Проверить статус владельца"},
	{
		Name:                     "ban",
		Description:              "Забанить участника",
		DefaultMemberPermissions: perm(discordgo.PermissionBanMembers),
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Участник", Required: true},
			{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Причина"},
		},
	},
	{
		Name:                     "kick",
		Description:              "Кикнуть участника",
		DefaultMemberPermissions: perm(discordgo.PermissionKickMembers),
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Участник", Required: true},
			{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Причина"},
		},
	},
	{
		Name:                     "timeout",
		Description:              "Выдать таймаут участнику",
		DefaultMemberPermissions: perm(discordgo.PermissionModerateMembers),
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Участник", Required: true},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "minutes",
				Description: "Длительность (минуты, макс 40320 = 4 недели)",
				Required:    true,
				MinValue:    fptr(1),
				MaxValue:    40320,
			},
		},
	},
	{
		Name:                     "unban",
		Description:              "Разбанить пользователя по ID",
		DefaultMemberPermissions: perm(discordgo.PermissionBanMembers),
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "user_id", Description: "Discord ID пользователя", Required: true},
		},
	},
	{
		Name:                     "purge",
		Description:              "Удалить сообщения в канале",
		DefaultMemberPermissions: perm(discordgo.PermissionManageMessages),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "amount",
				Description: "Количество сообщений (1-100)",
				Required:    true,
				MinValue:    fptr(1),
				MaxValue:    100,
			},
		},
	},
	{
		Name:                     "role",
		Description:              "Выдать/убрать роль участнику",
		DefaultMemberPermissions: perm(discordgo.PermissionManageRoles),
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Участник", Required: true},
			{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Роль", Required: true},
		},
	},
}

// ── структура ответа от Python/Rust через NATS ────────────────────────────
type NATSResponse struct {
	InteractionID    string `json:"interaction_id"`
	InteractionToken string `json:"interaction_token"`
	Content          string `json:"content"`
}

func main() {
	// ── токен ─────────────────────────────────────────────────────────────
	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	token = strings.Trim(token, "\"'")
	if token == "" {
		log.Fatal("DISCORD_TOKEN не задан")
	}

	// ── NATS ──────────────────────────────────────────────────────────────
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(
		natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("NATS отключён: %v", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Printf("NATS переподключён: %s", c.ConnectedUrl())
		}),
	)
	if err != nil {
		log.Fatalf("Ошибка подключения к NATS: %v", err)
	}
	defer nc.Drain()
	log.Printf("NATS: %s", natsURL)

	// ── Discord сессия ────────────────────────────────────────────────────
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Ошибка создания Discord сессии: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers

	// GUILD_ID: пустой = глобальные команды (до 1 часа)
	//           задан  = гильдийные команды (мгновенно, для тестов)
	guildID := strings.TrimSpace(os.Getenv("GUILD_ID"))

	var registeredCmds []*discordgo.ApplicationCommand

	// ── Ready: авторизация + регистрация команд ───────────────────────────
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		mu.Lock()
		if r.Application != nil && r.Application.ID != "" {
			appID = r.Application.ID
		} else {
			appID = r.User.ID
		}
		mu.Unlock()

		scope := "global"
		if guildID != "" {
			scope = fmt.Sprintf("guild:%s", guildID)
		}
		log.Printf("Авторизован: %s#%s | app:%s | scope:%s",
			r.User.Username, r.User.Discriminator, appID, scope)

		for _, cmd := range commands {
			reg, err := s.ApplicationCommandCreate(r.User.ID, guildID, cmd)
			if err != nil {
				log.Printf("  ❌ /%s: %v", cmd.Name, err)
				continue
			}
			registeredCmds = append(registeredCmds, reg)
			log.Printf("  ✅ /%s", reg.Name)
		}
	})

	// ── Slash команды → NATS ──────────────────────────────────────────────
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.Member == nil {
			// DM — игнорируем
			return
		}

		data := i.ApplicationCommandData()

		// Нужно ли ephemeral для этой команды?
		var flags discordgo.MessageFlags
		if data.Name == "owner" {
			flags = discordgo.MessageFlagsEphemeral
		}

		// Немедленное подтверждение (у нас есть 3 сек до таймаута Discord)
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Flags: flags},
		}); err != nil {
			log.Printf("Ошибка defer: %v", err)
			return
		}

		// Парсим опции в map
		opts := make(map[string]interface{})
		for _, opt := range data.Options {
			opts[opt.Name] = opt.Value
		}

		payload, err := json.Marshal(map[string]interface{}{
			"command":           data.Name,
			"interaction_id":    i.ID,
			"interaction_token": i.Token,
			"channel_id":        i.ChannelID,
			"guild_id":          i.GuildID,
			"user_id":           i.Member.User.ID,
			"username":          i.Member.User.Username,
			"options":           opts,
		})
		if err != nil {
			log.Printf("Ошибка marshal: %v", err)
			return
		}

		if err := nc.Publish("discord.interaction.create", payload); err != nil {
			log.Printf("Ошибка NATS publish: %v", err)
			return
		}
		log.Printf("→ NATS: /%s от %s", data.Name, i.Member.User.Username)
	})

	// ── Ответы от Python/Rust → Discord ──────────────────────────────────
	if _, err := nc.Subscribe("discord.interaction.respond", func(m *nats.Msg) {
		var resp NATSResponse
		if err := json.Unmarshal(m.Data, &resp); err != nil {
			log.Printf("Ошибка unmarshal ответа: %v", err)
			return
		}

		mu.RLock()
		aid := appID
		mu.RUnlock()

		content := resp.Content
		if _, err := dg.InteractionResponseEdit(
			&discordgo.Interaction{AppID: aid, Token: resp.InteractionToken},
			&discordgo.WebhookEdit{Content: &content},
		); err != nil {
			log.Printf("Ошибка edit response: %v", err)
		}
	}); err != nil {
		log.Fatalf("Ошибка подписки NATS respond: %v", err)
	}

	// ── Открываем соединение ──────────────────────────────────────────────
	if err := dg.Open(); err != nil {
		log.Fatalf("Ошибка Discord Open: %v", err)
	}
	defer dg.Close()

	log.Println("Avena Gateway запущен. Ctrl+C для остановки.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Завершение: удаляем команды...")
	for _, cmd := range registeredCmds {
		if err := dg.ApplicationCommandDelete(dg.State.User.ID, guildID, cmd.ID); err != nil {
			log.Printf("Ошибка удаления /%s: %v", cmd.Name, err)
		}
	}
}
