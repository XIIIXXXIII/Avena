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

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()
	fmt.Printf("Gateway connected to NATS at %s\n", natsURL)

	// Connect to Discord
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}

	// Handle incoming messages from Discord
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		
		payload, _ := json.Marshal(m)
		nc.Publish("discord.event.message_create", payload)
		fmt.Printf("Event published: message_create from %s\n", m.Author.Username)
	})

	// Handle outgoing messages from NATS to Discord
	nc.Subscribe("discord.api.send_message", func(m *nats.Msg) {
		var req struct {
			ChannelID string `json:"channel_id"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal(m.Data, &req); err == nil {
			dg.ChannelMessageSend(req.ChannelID, req.Content)
			fmt.Printf("Message sent to channel %s\n", req.ChannelID)
		}
	})

	err = dg.Open()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Avena Gateway is now running. Press CTRL-C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}
