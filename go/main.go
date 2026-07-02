package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/nats-io/nats.go"
)

const (
	DISCORD_API_BASE = "https://discord.com/api/v10"
)

type ApplicationCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        int    `json:"type"` // 1 for CHAT_INPUT (slash commands)
}

type InteractionResponse struct {
	Type int `json:"type"` // 4 for CHANNEL_MESSAGE_WITH_SOURCE
	Data struct {
		Content    string                 `json:"content,omitempty"`
		Embeds     []*discordgo.MessageEmbed `json:"embeds,omitempty"`
		Components []discordgo.MessageComponent `json:"components,omitempty"`
	} `json:"data"`
}

// Discord API call to create a voice channel
func createVoiceChannel(guildID, channelName, botToken string) (string, error) {
	url := fmt.Sprintf("%s/guilds/%s/channels", DISCORD_API_BASE, guildID)
	payload := map[string]interface{}{
		"name":      channelName,
		"type":      2, // GUILD_VOICE
		"parent_id": nil, // TODO: Make configurable or dynamic
	}
	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("error creating request to create channel: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request to create channel: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		var errorBody map[string]interface{}
		json.NewDecoder(res.Body).Decode(&errorBody)
		return "", fmt.Errorf("failed to create channel, status: %s, body: %v", res.Status, errorBody)
	}

	var channelData struct {
		ID string `json:"id"`
	}
	json.NewDecoder(res.Body).Decode(&channelData)
	return channelData.ID, nil
}

// Discord API call to delete a voice channel
func deleteVoiceChannel(channelID, botToken string) error {
	url := fmt.Sprintf("%s/channels/%s", DISCORD_API_BASE, channelID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request to delete channel: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+botToken)

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request to delete channel: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorBody map[string]interface{}
		json.NewDecoder(res.Body).Decode(&errorBody)
		return fmt.Errorf("failed to delete channel, status: %s, body: %v", res.Status, errorBody)
	}
	return nil
}

// Discord API call to get channel members (to check if empty)
func getChannelMembers(guildID, botToken string) ([]*discordgo.VoiceState, error) {
	url := fmt.Sprintf("%s/guilds/%s/voice-states", DISCORD_API_BASE, guildID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request to get voice states: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+botToken)

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request to get voice states: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorBody map[string]interface{}
		json.NewDecoder(res.Body).Decode(&errorBody)
		return nil, fmt.Errorf("failed to get voice states, status: %s, body: %v", res.Status, errorBody)
	}

	var voiceStates []*discordgo.VoiceState
	json.NewDecoder(res.Body).Decode(&voiceStates)

	return voiceStates, nil
}

func registerCommands(appID, botToken string) error {
	commands := []ApplicationCommand{
		{Name: "ping", Description: "Checks if the bot is alive.", Type: 1},
		{Name: "info", Description: "Displays system information.", Type: 1},
		{Name: "owner", Description: "Owner-only command.", Type: 1},
	}

	for _, cmd := range commands {
		cmdJSON, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("error marshaling command %s: %w", cmd.Name, err)
		}

		url := fmt.Sprintf("%s/applications/%s/commands", DISCORD_API_BASE, appID)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(cmdJSON))
		if err != nil {
			return fmt.Errorf("error creating request for command %s: %w", cmd.Name, err)
		}

		req.Header.Set("Authorization", "Bot "+botToken)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error sending request for command %s: %w", cmd.Name, err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			var errorBody map[string]interface{}
			json.NewDecoder(res.Body).Decode(&errorBody)
			return fmt.Errorf("failed to register command %s, status: %s, body: %v", cmd.Name, res.Status, errorBody)
		}

		log.Printf("Successfully registered command: /%s", cmd.Name)
	}
	return nil
}

func main() {
	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable not set")
	}

	appID := os.Getenv("DISCORD_APPLICATION_ID")
	if appID == "" {
		log.Fatal("DISCORD_APPLICATION_ID environment variable not set")
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

	log.Printf("Gateway (Go) connected to NATS at %s", natsURL)

	// Initialize Discordgo session
	dg, err := discordgo.New("Bot " + botToken)
	dg.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsGuildMembers // Added GuildMembers intent
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Register Slash Commands
	log.Println("Registering slash commands...")
	if err := registerCommands(appID, botToken); err != nil {
		log.Fatalf("Failed to register commands: %v", err)
	}

	// Add handlers for Discord events
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Publish interaction to NATS for Logic-Engine
		data, _ := json.Marshal(i)
		if err := nc.Publish("discord.interaction.create", data); err != nil {
			log.Printf("Error publishing interaction to NATS: %v", err)
		}
	})

	dg.AddHandler(func(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
		// Publish voice state update to NATS for Logic-Engine
		data, _ := json.Marshal(v)
		if err := nc.Publish("discord.voice.state.update", data); err != nil {
			log.Printf("Error publishing voice state update to NATS: %v", err)
		}
	})

	// NATS subscription for creating channels
	_, err = nc.Subscribe("discord.channel.create", func(m *nats.Msg) {
		var createPayload struct {
			GuildID     string `json:"guild_id"`
			UserID      string `json:"user_id"`
			ChannelName string `json:"channel_name"`
		}
		if err := json.Unmarshal(m.Data, &createPayload); err != nil {
			log.Printf("Error unmarshaling create channel payload: %v", err)
			return
		}

		channelID, err := createVoiceChannel(createPayload.GuildID, createPayload.ChannelName, botToken)
		if err != nil {
			log.Printf("Failed to create voice channel: %v", err)
			return
		}
		log.Printf("Created voice channel %s for user %s in guild %s", channelID, createPayload.UserID, createPayload.GuildID)

		// Notify Logic-Engine that channel was created
		responsePayload := map[string]string{
			"guild_id":    createPayload.GuildID,
			"channel_id":  channelID,
			"user_id":     createPayload.UserID,
			"channel_name": createPayload.ChannelName,
		}
		respData, _ := json.Marshal(responsePayload)
		nc.Publish("discord.channel.created", respData)
	})
	if err != nil {
		log.Fatal(err)
	}

	// NATS subscription for deleting channels
	_, err = nc.Subscribe("discord.channel.delete", func(m *nats.Msg) {
		var deletePayload struct {
			GuildID   string `json:"guild_id"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(m.Data, &deletePayload); err != nil {
			log.Printf("Error unmarshaling delete channel payload: %v", err)
			return
		}

		if err := deleteVoiceChannel(deletePayload.ChannelID, botToken); err != nil {
			log.Printf("Failed to delete voice channel %s: %v", deletePayload.ChannelID, err)
			return
		}
		log.Printf("Deleted voice channel %s in guild %s", deletePayload.ChannelID, deletePayload.GuildID)
	})
	if err != nil {
		log.Fatal(err)
	}

	// NATS subscription for checking channel emptiness
	_, err = nc.Subscribe("discord.channel.check_empty", func(m *nats.Msg) {
		var checkPayload struct {
			GuildID   string `json:"guild_id"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(m.Data, &checkPayload); err != nil {
			log.Printf("Error unmarshaling check empty channel payload: %v", err)
			return
		}

		members, err := getChannelMembers(checkPayload.GuildID, botToken)
		if err != nil {
			log.Printf("Failed to get members for channel %s: %v", checkPayload.ChannelID, err)
			return
		}

		isEmpty := true
		for _, vs := range members {
			if vs.ChannelID == checkPayload.ChannelID {
				isEmpty = false
				break
			}
		}

		log.Printf("Channel %s in guild %s is empty: %t", checkPayload.ChannelID, checkPayload.GuildID, isEmpty)

		responsePayload := map[string]interface{}{
			"guild_id":   checkPayload.GuildID,
			"channel_id": checkPayload.ChannelID,
			"is_empty":   isEmpty,
		}
		respData, _ := json.Marshal(responsePayload)
		nc.Publish("discord.channel.empty", respData)
	})
	if err != nil {
		log.Fatal(err)
	}

	// NATS subscription for sending messages to Discord
	_, err = nc.Subscribe("discord.message.send", func(m *nats.Msg) {
		var sendPayload struct {
			ChannelID  string                    `json:"channel_id"`
			Content    string                    `json:"content,omitempty"`
			Embeds     []*discordgo.MessageEmbed `json:"embeds,omitempty"`
			Components []discordgo.MessageComponent `json:"components,omitempty"`
		}
		if err := json.Unmarshal(m.Data, &sendPayload); err != nil {
			log.Printf("Error unmarshaling send message payload: %v", err)
			return
		}

		msgSend := &discordgo.MessageSend{
			Content:    sendPayload.Content,
			Embeds:     sendPayload.Embeds,
			Components: sendPayload.Components,
		}

		msg, err := dg.ChannelMessageSendComplex(sendPayload.ChannelID, msgSend)
		if err != nil {
			log.Printf("Failed to send message to channel %s: %v", sendPayload.ChannelID, err)
			return
		}
		log.Printf("Sent message %s to channel %s", msg.ID, msg.ChannelID)

		// Notify Logic-Engine that message was sent
		responsePayload := map[string]string{
			"guild_id":   msg.GuildID,
			"channel_id": msg.ChannelID,
			"message_id": msg.ID,
		}
		respData, _ := json.Marshal(responsePayload)
		nc.Publish("discord.message.sent", respData)
	})
	if err != nil {
		log.Fatal(err)
	}

	// NATS subscription for editing messages in Discord
	_, err = nc.Subscribe("discord.message.edit", func(m *nats.Msg) {
		var editPayload struct {
			ChannelID  string                    `json:"channel_id"`
			MessageID  string                    `json:"message_id"`
			Content    string                    `json:"content,omitempty"`
			Embeds     []*discordgo.MessageEmbed `json:"embeds,omitempty"`
			Components []discordgo.MessageComponent `json:"components,omitempty"`
		}
		if err := json.Unmarshal(m.Data, &editPayload); err != nil {
			log.Printf("Error unmarshaling edit message payload: %v", err)
			return
		}

		msgEdit := &discordgo.MessageEdit{
			ChannelID:  editPayload.ChannelID,
			MessageID:  editPayload.MessageID,
			Content:    &editPayload.Content,
			Embeds:     editPayload.Embeds,
			Components: editPayload.Components,
		}

		_, err := dg.ChannelMessageEditComplex(msgEdit)
		if err != nil {
			log.Printf("Failed to edit message %s in channel %s: %v", editPayload.MessageID, editPayload.ChannelID, err)
			return
		}
		log.Printf("Edited message %s in channel %s", editPayload.MessageID, editPayload.ChannelID)
	})
	if err != nil {
		log.Fatal(err)
	}

	// NATS subscription for sending follow-up messages to Discord interactions
	_, err = nc.Subscribe("discord.followup.message", func(m *nats.Msg) {
		var followupPayload struct {
			ApplicationID    string                    `json:"application_id"`
			InteractionToken string                    `json:"interaction_token"`
			Content          string                    `json:"content,omitempty"`
			Embeds           []*discordgo.MessageEmbed `json:"embeds,omitempty"`
			Components       []discordgo.MessageComponent `json:"components,omitempty"`
		}
		if err := json.Unmarshal(m.Data, &followupPayload); err != nil {
			log.Printf("Error unmarshaling followup message payload: %v", err)
			return
		}

		url := fmt.Sprintf("%s/webhooks/%s/%s", DISCORD_API_BASE, followupPayload.ApplicationID, followupPayload.InteractionToken)
		payload := map[string]interface{}{
			"content":    followupPayload.Content,
			"embeds":     followupPayload.Embeds,
			"components": followupPayload.Components,
		}
		jsonPayload, _ := json.Marshal(payload)

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			log.Printf("Error creating followup message request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			log.Printf("Error sending followup message: %v", err)
			return
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			var errorBody map[string]interface{}
			json.NewDecoder(res.Body).Decode(&errorBody)
			log.Printf("Failed to send followup message, status: %s, body: %v", res.Status, errorBody)
		}
		log.Printf("Successfully sent followup message.")
	})
	if err != nil {
		log.Fatal(err)
	}

	// Open a websocket connection to Discord and begin listening.
	if err = dg.Open(); err != nil {
		log.Fatalf("Error opening Discord session: %v", err)
	}
	defer dg.Close()

	log.Println("Gateway (Go) is running. Waiting for events...")

	// Subscribe to responses from the Logic-Engine (Python) via NATS
	_, err = nc.Subscribe("discord.interaction.respond", func(m *nats.Msg) {
		log.Printf("Received interaction response from NATS.")
		var responsePayload struct {
			InteractionID    string                    `json:"interaction_id"`
			InteractionToken string                    `json:"interaction_token"`
			Type             int                       `json:"type"`
			Data             struct {
				Content    string                 `json:"content,omitempty"`
				Embeds     []*discordgo.MessageEmbed `json:"embeds,omitempty"`
				Components []discordgo.MessageComponent `json:"components,omitempty"`
			} `json:"data"`		
		}

		if err := json.Unmarshal(m.Data, &responsePayload); err != nil {
			log.Printf("Error unmarshaling interaction response: %v", err)
			return
		}

		// Send the response back to Discord using the interaction webhook
		responseURL := fmt.Sprintf("%s/interactions/%s/%s/callback", DISCORD_API_BASE, responsePayload.InteractionID, responsePayload.InteractionToken)

		respJSON, err := json.Marshal(responsePayload)
		if err != nil {
			log.Printf("Error marshaling Discord response: %v", err)
			return
		}

		req, err := http.NewRequest("POST", responseURL, bytes.NewBuffer(respJSON))
		if err != nil {
			log.Printf("Error creating Discord response request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			log.Printf("Error sending Discord response: %v", err)
			return
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			var errorBody map[string]interface{}
			json.NewDecoder(res.Body).Decode(&errorBody)
			log.Printf("Failed to send Discord response, status: %s, body: %v", res.Status, errorBody)
		}
		log.Printf("Successfully sent interaction response to Discord.")
	})
	if err != nil {
		log.Fatal(err)
	}

	// Keep alive
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
