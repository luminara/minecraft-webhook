package main

import (
	"fmt"
	"os"

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
	Type   string            `yaml:"type"`
	Url    string            `yaml:"url"`
	Events map[string]string `yaml:"events"`
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

func main() {
	config, err := LoadConfig("webhook.yml")
	if err != nil {
		panic(err)
	}

	fmt.Println("Image name:", config.MCWebhook.ImageNames)
}
