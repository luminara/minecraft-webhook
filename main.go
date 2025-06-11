package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

type Config struct {
	MCWebhook MCWebhookConfig `yaml:"mc-webhook"`
}

type MCWebhookConfig struct {
	ImageNames       string                    `yaml:"image-names"`
	BackupImageNames string                    `yaml:"backup-image-names"`
	Webhooks         map[string]WebhookDetails `yaml:"webhooks"`
}

type WebhookDetails struct {
	Type   string        `yaml:"type"`
	Url    string        `yaml:"url"`
	Events EventMessages `yaml:"events"`
}

type EventMessages struct {
	ServerStarted      string `yaml:"SERVER_STARTED"`
	ServerStopped      string `yaml:"SERVER_STOPPED"`
	PlayerConnected    string `yaml:"PLAYER_CONNECTED"`
	WelcomeMessage     string `yaml:"WELCOME_MESSAGE"`
	PlayerDisconnected string `yaml:"PLAYER_DISCONNECTED"`
	BackupComplete     string `yaml:"BACKUP_COMPLETE"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

var config *Config

func init() {
	var err error
	config, err = LoadConfig("config.yml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	fmt.Println("Configuration loaded successfully in init().")
}

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		attachToContainer(config.MCWebhook.ImageNames)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		attachToContainer(config.MCWebhook.BackupImageNames)
	}()

	select {}
}

func attachToContainer(imageName string) {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		log.Fatalf("Error listing containers: %v", err)
	}

	var containerID string
	var foundContainerName string

	for _, c := range containers {
		if c.Image == imageName {
			containerID = c.ID
			if len(c.Names) > 0 {
				foundContainerName = c.Names[0][1:]
			} else {
				foundContainerName = c.ID[:12]
			}
			break
		}
	}

	if containerID == "" {
		log.Fatalf("Container with image '%s' not found.", imageName)
	}

	log.Printf("Found container '%s' (ID: %s) using image '%s'\n", foundContainerName, containerID, imageName)

	attachOptions := container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	}

	hijackedResponse, err := cli.ContainerAttach(ctx, containerID, attachOptions)
	if err != nil {
		log.Fatalf("Error attaching to container: %v", err)
	}
	defer hijackedResponse.Close()
	log.Printf("Successfully attached to the container '%s'.\n", foundContainerName)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(hijackedResponse.Reader)
		for scanner.Scan() {
			eventHandler(scanner.Text(), hijackedResponse)
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading from container output: %v", err)
		}

		//eventHandler("Stopping server...", hijackedResponse)
		log.Println("Container output reader stopped.")
	}()

	inputScanner := bufio.NewScanner(os.Stdin)
	for inputScanner.Scan() {
		line := inputScanner.Text()
		if line == "exit" {
			log.Println("Exiting log reader for", imageName)
			break
		}
	}
	wg.Wait()
}

func eventHandler(event string, hijackedResponse types.HijackedResponse) {
	for name, webhook := range config.MCWebhook.Webhooks {
		if webhook.Url == "" {
			log.Printf("Skipping webhook '%s': missing URL.", name)
			continue
		}

		webhookMsg, containerMsg := parseLogEvent(event, webhook.Events)

		if webhookMsg != "" {
			log.Printf("[Webhook:%s] Sending: %s\n", name, webhookMsg)
			sendWebhook(webhookMsg, webhook.Url)
		}

		if containerMsg != "" {
			log.Printf("[Webhook:%s] Sending to container: %s\n", name, containerMsg)
			_, err := hijackedResponse.Conn.Write([]byte(containerMsg + "\n"))
			if err != nil {
				log.Printf("Failed to write to container: %v", err)
			}
		}
	}
}

func parseLogEvent(event string, events EventMessages) (webhookMsg string, containerMsg string) {
	if strings.Contains(event, "Server started.") && events.ServerStarted != "" {
		webhookMsg = events.ServerStarted
	} else if strings.Contains(event, "Stopping server") && events.ServerStopped != "" {
		webhookMsg = events.ServerStopped
	} else if strings.Contains(event, "Player Spawned") && events.PlayerConnected != "" {
		playerName := regexp.MustCompile(`Player Spawned: ([^\s]+) xuid:`).FindStringSubmatch(event)
		webhookMsg = strings.Replace(events.PlayerConnected, "%playerName%", playerName[1], -1)

		if events.WelcomeMessage != "" {
			events.WelcomeMessage = strings.Replace(events.WelcomeMessage, "%playerName%", playerName[1], -1)
			time.Sleep(6 * time.Second)
			containerMsg = events.WelcomeMessage
		}
	} else if strings.Contains(event, "Player disconnected") && events.PlayerDisconnected != "" {
		playerName := regexp.MustCompile(`Player disconnected: ([^,]+),`).FindStringSubmatch(event)
		webhookMsg = strings.Replace(events.PlayerDisconnected, "%playerName%", playerName[1], -1)
	} else if strings.Contains(event, "Backed up as:") && events.BackupComplete != "" {
		filename := regexp.MustCompile(`Backed up as: ([^\s]+\.mcworld)`).FindStringSubmatch(event)
		webhookMsg = strings.Replace(events.BackupComplete, "%filename%", filename[1], -1)
	}

	return webhookMsg, containerMsg
}

func sendWebhook(msg string, webhookUrl string) {
	payload := map[string]string{
		"content": msg,
	}
	body, _ := json.Marshal(payload)

	http.Post(webhookUrl, "application/json", bytes.NewBuffer(body))
	log.Printf("webhook sent msg: %s\n", msg)
}
