package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
)

type DiscordEvent struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type MessageCreate struct {
	Content   string `json:"content"`
	ChannelID string `json:"channel_id"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"author"`
}

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	log.Printf("Connected to NATS at %s", natsURL)

	// Subscribe to message creation events
	_, err = nc.Subscribe("discord.event.message_create", func(m *nats.Msg) {
		var msg MessageCreate
		if err := json.Unmarshal(m.Data, &msg); err != nil {
			log.Printf("Error unmarshaling event: %v", err)
			return
		}

		log.Printf("Received message: %s from %s", msg.Content, msg.Author.Username)

		// Simple command handling
		if strings.HasPrefix(msg.Content, "/ping") {
			log.Printf("Ping command detected!")
			
			// In a full setup, we would publish to another NATS subject 
			// that the Discord API Proxy (written in Rust or Go) listens to.
			// For now, we'll just log it.
			response := map[string]string{
				"channel_id": msg.ChannelID,
				"content":    fmt.Sprintf("Pong! Hello from Avena Polyglot Cluster (Go Node). Latency: [stateless]"),
			}
			respData, _ := json.Marshal(response)
			nc.Publish("discord.api.send_message", respData)
		}
	})

	if err != nil {
		log.Fatal(err)
	}

	log.Println("Command-Router (Go) is running. Waiting for events...")

	// Keep alive
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
