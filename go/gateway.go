package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/nats-io/nats.go"
)

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Check bot latency",
		},
		{
			Name:        "info",
			Description: "Show system information",
		},
		{
			Name:        "owner",
			Description: "Check owner status",
		},
	}
)

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "\"")
	token = strings.Trim(token, "'")

	if token == "" {
		log.Fatal("ERROR: DISCORD_TOKEN is empty.")
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}

	// Handle Slash Commands
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		// Acknowledge the interaction immediately
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		})

		// Send to NATS for processing
		payload, _ := json.Marshal(i)
		nc.Publish("discord.interaction.create", payload)
		fmt.Printf("Interaction: /%s from %s\n", i.ApplicationCommandData().Name, i.Member.User.Username)
	})

	// Handle outgoing messages from NATS back to Discord interactions
	nc.Subscribe("discord.interaction.respond", func(m *nats.Msg) {
		var resp struct {
			InteractionID    string `json:"interaction_id"`
			InteractionToken string `json:"interaction_token"`
			Content          string `json:"content"`
		}
		if err := json.Unmarshal(m.Data, &resp); err == nil {
			_, err := dg.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &resp.Content,
			})
			if err != nil {
				log.Printf("Error responding to interaction: %v", err)
			}
		}
	})

	err = dg.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer dg.Close()

	fmt.Println("Registering slash commands...")
	for _, v := range commands {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
	}

	fmt.Println("Avena Gateway is ONLINE with Slash Commands.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
