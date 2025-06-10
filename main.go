package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

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
	config, err = LoadConfig("webhook.yml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	fmt.Println("Configuration loaded successfully in init().")
}

func main() {
	attachToContainer(config.MCWebhook.ImageNames)
	attachToContainer(config.MCWebhook.BackupImageNames)
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

	log.Println("Successfully attached to the container.")
	log.Println("Type 'exit' or Ctrl+C to quit.")

	go func() {
		scanner := bufio.NewScanner(hijackedResponse.Reader)
		for scanner.Scan() {
			fmt.Printf("[CONTAINER]: %s\n", scanner.Text())
			eventHandler(scanner.Text(), hijackedResponse)

		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading from container output: %v", err)
		}
		log.Println("Container output reader stopped.")
	}()

	inputScanner := bufio.NewScanner(os.Stdin)
	for inputScanner.Scan() {
		line := inputScanner.Text()
		if line == "exit" {
			log.Println("Exiting application...")
			break
		}
	}
}

func eventHandler(event string, hijackedResponse types.HijackedResponse) {
}
