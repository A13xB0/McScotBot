package bootstrap

import (
	"fmt"
	"log"
	"strings"
	"time"

	"meshcore-bots/internal/remoteterm"
)

const (
	maxRetries    = 30
	retryInterval = 2 * time.Second
)

// Run waits for RemoteTerm to be healthy, then ensures all required channels exist.
func Run(client *remoteterm.Client, channelNames []string) error {
	log.Println("Bootstrap: waiting for RemoteTerm...")
	if err := waitForHealth(client); err != nil {
		return fmt.Errorf("RemoteTerm health check failed: %w", err)
	}
	log.Println("Bootstrap: RemoteTerm is healthy")

	existing, err := client.ListChannels()
	if err != nil {
		return fmt.Errorf("listing channels: %w", err)
	}

	existingKeys := make(map[string]bool, len(existing))
	for _, ch := range existing {
		existingKeys[strings.ToUpper(ch.Key)] = true
	}

	for _, name := range channelNames {
		key := remoteterm.HashtagChannelKey(name)
		if existingKeys[key] {
			log.Printf("Bootstrap: channel %s already exists", name)
			continue
		}
		log.Printf("Bootstrap: creating channel %s (key %s)", name, key)
		if err := client.CreateChannel(name); err != nil {
			return fmt.Errorf("creating channel %s: %w", name, err)
		}
		log.Printf("Bootstrap: created channel %s", name)
	}

	return nil
}

func waitForHealth(client *remoteterm.Client) error {
	for i := range maxRetries {
		err := client.HealthCheck()
		if err == nil {
			return nil
		}
		log.Printf("Bootstrap: health check attempt %d/%d failed: %v", i+1, maxRetries, err)
		time.Sleep(retryInterval)
	}
	return fmt.Errorf("health check failed after %d attempts", maxRetries)
}
