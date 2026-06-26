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
	// Clean token from quotes and spaces
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "\"")
	token = strings.Trim(token, "'")

	if token == "" {
		log.Fatal("ERROR: DISCORD_TOKEN is empty. Check your .env file.")
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	fmt.Println("Starting Avena Gateway...")

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Printf("NATS connection error: %v. Retrying...", err)
		// Simple retry logic
		nc, err = nats.Connect(natsURL)
		if err != nil {
			log.Fatal("Could not connect to NATS. Exit.")
		}
	}
	defer nc.Close()
	fmt.Printf("Gateway connected to NATS at %s\n", natsURL)

	// Connect to Discord
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Discord initialization error: %v", err)
	}

	// Handle incoming messages from Discord
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		
		payload, _ := json.Marshal(m)
		nc.Publish("discord.event.message_create", payload)
		fmt.Printf("Event: message from %s: %s\n", m.Author.Username, m.Content)
	})

	// Handle outgoing messages from NATS to Discord
	nc.Subscribe("discord.api.send_message", func(m *nats.Msg) {
		var req struct {
			ChannelID string `json:"channel_id"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal(m.Data, &req); err == nil {
			_, err := dg.ChannelMessageSend(req.ChannelID, req.Content)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			} else {
				fmt.Printf("Message sent to channel %s\n", req.ChannelID)
			}
		}
	})

	err = dg.Open()
	if err != nil {
		log.Fatalf("Discord connection error: %v", err)
	}
	fmt.Println("Avena Gateway is ONLINE. Press CTRL-C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	fmt.Println("Shutting down...")
	dg.Close()
}
