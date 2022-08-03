package hetznercloud

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hetznercloud/hcloud-go/hcloud"
)

var HETZNER_API_TOKEN = os.Getenv("HETZNER_API_TOKEN")
var HETZNER_DNS_API_TOKEN = os.Getenv("HETZNER_DNS_API_TOKEN")
var HETZNER_FIREWALL = os.Getenv("HETZNER_FIREWALL")
var HETZNER_VAULT_VOLUME_ID = os.Getenv("HETZNER_VAULT_VOLUME_ID")

func GetSSHFirewallRules() []string {
	var sshSources []string
	client := hcloud.NewClient(hcloud.WithToken(HETZNER_API_TOKEN))
	firewall, _, _ := client.Firewall.Get(context.Background(), HETZNER_FIREWALL)
	for _, rule := range firewall.Rules {
		if rule.Direction == hcloud.FirewallRuleDirectionIn {
			if rule.Port != nil && *rule.Port == "22" {
				for _, sourceIP := range rule.SourceIPs {
					sshSources = append(sshSources, sourceIP.String())
				}
			}
		}
	}

	return sshSources
}

// bender slackbot methods
func ListAllServers() []*hcloud.Server {
	client := hcloud.NewClient(hcloud.WithToken(HETZNER_API_TOKEN))
	servers, _ := client.Server.All(context.Background())
	return servers
}

func DeleteServer(serverID int) string {
	result := fmt.Sprintf("Successfully deleted server %d: ", serverID)

	client := hcloud.NewClient(hcloud.WithToken(HETZNER_API_TOKEN))
	server, _, err := client.Server.GetByID(context.Background(), serverID)
	if err != nil {
		return fmt.Sprintf("Server %d doesn't exist!\n", serverID)
	}

	_, err = client.Server.Delete(context.Background(), server)
	if err != nil {
		return fmt.Sprintf("Unable to delete server [%d] %s: %s\n", serverID, server.Name, err.Error())
	}

	return result + server.Name
}

func UpdateDNSentry(ipAddr string, ttl string, recordType string,
	name string, recordID string, zoneID string) string {
	// https://dns.hetzner.com/api-docs/#operation/UpdateRecord

	json := []byte(`{
		"value": "` + ipAddr +
		`","ttl": ` + ttl +
		`,"type": "` + recordType +
		`","name": "` + name +
		`","zone_id": "` + zoneID + `"}`)
	body := bytes.NewBuffer(json)

	// Create client
	client := &http.Client{}

	// Create request
	req, _ := http.NewRequest("PUT", "https://dns.hetzner.com/api/v1/records/"+recordID, body)

	// Headers
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Auth-API-Token", HETZNER_DNS_API_TOKEN)

	// Fetch Request
	_, err := client.Do(req)
	if err != nil {
		return "Failure : " + err.Error()
	}
	return ""
}
