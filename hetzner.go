package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ackersonde/digitaloceans/common"
	"github.com/ackersonde/hetzner_home/hetznercloud"
	"github.com/hetznercloud/hcloud-go/hcloud"
	"golang.org/x/crypto/ssh"
)

var sshPrivateKeyFilePath = "/home/runner/.ssh/id_rsa"
var envFile = "/tmp/new_hetzner_server_params"

func main() {
	client := hcloud.NewClient(hcloud.WithToken(hetznercloud.HETZNER_API_TOKEN))

	fnPtr := flag.String("fn", "createServer|cleanupDeploy|firewallSSH|createSnapshot|checkServer", "which function to run")
	ipPtr := flag.String("ip", "<internet ip addr of github action instance>", "see prev param")
	tagPtr := flag.String("tag", "homepage|vault", "label with which to associate this resource")
	serverPtr := flag.Int("serverID", 0, "server ID to check")
	flag.Parse()

	if *fnPtr == "createServer" {
		createServer(client, *tagPtr)
	} else if *fnPtr == "cleanupDeploy" {
		cleanupDeploy(client, *serverPtr, *tagPtr)
	} else if *fnPtr == "firewallSSH" {
		allowSSHipAddress(client, *ipPtr, *tagPtr)
	} else if *fnPtr == "checkServer" {
		checkServerPowerSwitch(client, *serverPtr)
	} else {
		log.Printf("Sorry, I don't know `%s`. Check valid params with `./hetzner --help`", *fnPtr)
	}

	/* For checking out new server & image types:
	types, _ := client.ServerType.All(context.Background())
	for _, typee := range types {
		fmt.Printf("type[%d] %s x %d cores (%f RAM)\n", typee.ID,
			typee.Description, typee.Cores, typee.Memory)
	}

	images, _ := client.Image.All(context.Background())
	for _, image := range images {
		fmt.Printf("image[%d] %s\n", image.ID, image.Name)
	}

	existingServer := getExistingServer(client)
	fmt.Printf("%d : %s\n", existingServer.ID, existingServer.PublicNet.IPv6.IP.String())
	*/
}

func allowSSHipAddress(client *hcloud.Client, ipAddr string, instanceTag string) {
	ctx := context.Background()

	opts := hcloud.FirewallCreateOpts{
		Name:   "githubBuildDeploy-" + os.Getenv("GITHUB_RUN_ID"),
		Labels: map[string]string{"access": "github"},
		Rules: []hcloud.FirewallRule{{
			Direction: hcloud.FirewallRuleDirectionIn,
			SourceIPs: []net.IPNet{{
				IP:   net.ParseIP(ipAddr),
				Mask: net.CIDRMask(32, 32),
			}},
			Protocol: "tcp",
			Port:     String("22"),
		}},
		ApplyTo: []hcloud.FirewallResource{{
			Type: hcloud.FirewallResourceTypeLabelSelector,
			LabelSelector: &hcloud.FirewallResourceLabelSelector{
				Selector: "label=" + instanceTag},
		}},
	}
	result, response, err := client.Firewall.Create(ctx, opts)
	if err != nil {
		log.Printf("NOPE: %s (%s)", opts.Name, err.Error())
		if strings.Contains(err.Error(), "uniqueness_error") {
			removeDeploymentFirewalls(client, ctx, instanceTag, "access=github")
			allowSSHipAddress(client, ipAddr, instanceTag) // retry
		}
	} else {
		log.Printf("%s: %s", response.Status, result.Firewall.Name)
	}
}

func checkServerPowerSwitch(client *hcloud.Client, serverID int) {
	ctx := context.Background()
	if serverID != 0 {
		server, _, _ := client.Server.GetByID(ctx, serverID)
		if server.Status != hcloud.ServerStatusRunning {
			client.Server.Poweron(ctx, server)
		}
	}
}

/* func listVolume(client *hcloud.Client) {
	volumeID, _ := strconv.Atoi(hetznercloud.HETZNER_VAULT_VOLUME_ID)
	volume, _, err := client.Volume.GetByID(context.Background(), volumeID)
	if err != nil {
		log.Fatalf("error retrieving volume: %s\n", err)
	}
	if volume != nil {
		fmt.Printf("volume %d: %q\n", volumeID, volume.LinuxDevice)
	} else {
		fmt.Printf("volume %d not found\n", volumeID)
	}
} */

func createServer(client *hcloud.Client, instanceTag string) {
	ctx := context.Background()

	// find existing server
	existingServer := getExistingServer(client, instanceTag)
	volumeID := 0

	if instanceTag == "vault" {
		// detach existing volume
		volumeID, _ = strconv.Atoi(hetznercloud.HETZNER_VAULT_VOLUME_ID)
		volume, _, _ := client.Volume.GetByID(ctx, volumeID)
		action, _, _ := client.Volume.Detach(ctx, volume)
		if action != nil {
			client.Action.WatchProgress(ctx, action)
		}
	}

	// prepare new server
	// myKey, _, _ := client.SSHKey.GetByName(ctx, "ackersond")
	deploymentKey := createSSHKey(client, os.Getenv("GITHUB_RUN_ID"))

	ubuntuUserData, _ := ioutil.ReadFile("ubuntu_userdata.sh")

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	serverOpts := hcloud.ServerCreateOpts{
		Name:       instanceTag + "-id" + os.Getenv("GITHUB_RUN_ID") + "-" + timestamp + ".ackerson.de",
		ServerType: &hcloud.ServerType{ID: 22},  // AMD 2 core, 2GB Ram
		Image:      &hcloud.Image{ID: 67794396}, // ubuntu-22.04
		Location:   &hcloud.Location{Name: "nbg1"},
		Labels:     map[string]string{"label": instanceTag},
		Automount:  Bool(false),
		UserData:   string(ubuntuUserData),
		SSHKeys:    []*hcloud.SSHKey{deploymentKey},
	}

	if instanceTag == "vault" {
		serverOpts.Volumes = []*hcloud.Volume{{ID: volumeID}}
	}
	result, _, err := client.Server.Create(ctx, serverOpts)
	if err != nil {
		log.Fatalf("*** unable to create server: %s\n", err)
	}
	if result.Server == nil {
		log.Fatalf("*** no server created?\n")
	} else {
		existingServerVars := ""
		if existingServer.Name != "" {
			existingServerVars = "\nexport OLD_SERVER_IPV6=" +
				existingServer.PublicNet.IPv6.IP.String() + "1"

			// update existingServer Label with "delete":"true" !
			client.Server.Update(ctx, existingServer, hcloud.ServerUpdateOpts{
				Labels: map[string]string{"delete": "true"},
			})
		}

		// Write key metadata from existing/new servers
		envVarsFile := []byte(
			"export NEW_SERVER_IPV4=" + result.Server.PublicNet.IPv4.IP.String() +
				"\nexport NEW_SERVER_IPV6=" + result.Server.PublicNet.IPv6.IP.String() + "1" +
				"\nexport NEW_SERVER_ID=" + strconv.Itoa(result.Server.ID) +
				existingServerVars)

		err = ioutil.WriteFile(envFile, envVarsFile, 0644)
		if err != nil {
			log.Fatalf("Failed to write %s: %s\n", envFile, err)
		} else {
			log.Printf("wrote %s\n", envFile)
		}
	}
}

func cleanupDeploy(client *hcloud.Client, serverID int, instanceTag string) {
	ctx := context.Background()
	opts := hcloud.ServerListOpts{ListOpts: hcloud.ListOpts{LabelSelector: "delete=true"}}
	servers, _ := client.Server.AllWithOpts(ctx, opts)
	for _, server := range servers {
		_, err := client.Server.Delete(ctx, server)
		if err == nil {
			log.Printf("DELETED Server %s\n", server.Name)
		} else {
			log.Fatalf("Unable to delete server %s (%s)!!!\n", server.Name, err)
		}
	}

	deployKeys, _ := client.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: "access=github"},
	})
	for _, deployKey := range deployKeys {
		// on weekly redeploys, we have 4 concurrent builds each with their own deployment key
		// delete the one you created or any > 10mins old
		// TODO: daily security check for unnecessary keys + cleanup (e.g. key >1hr old? nuke it...)
		if strings.HasPrefix(deployKey.Name, os.Getenv("GITHUB_RUN_ID")) || deployKey.Created.Before(time.Now().Add(-10*time.Minute)) {
			_, err := client.SSHKey.Delete(ctx, deployKey)
			if err == nil {
				log.Printf("DELETED SSH key %s\n", deployKey.Name)
			} else {
				log.Fatalf("Unable to delete SSH key %s (%s) !!!\n", deployKey.Name, err)
			}
		}
	}

	removeDeploymentFirewalls(client, ctx, instanceTag, "access=github")

	server, _, _ := client.Server.GetByID(ctx, serverID)
	// Update DNS entries @ DigitalOcean
	if server != nil {
		if instanceTag == "homepage" {
			common.UpdateDNSentry(server.PublicNet.IPv6.IP.String()+"1", "ackerson.de", 23738236)
			common.UpdateDNSentry(server.PublicNet.IPv4.IP.String(), "ackerson.de", 23738257)
			common.UpdateDNSentry(server.PublicNet.IPv6.IP.String()+"1", "hausmeisterservice-planb.de", 302721441)
			common.UpdateDNSentry(server.PublicNet.IPv4.IP.String(), "hausmeisterservice-planb.de", 302721419)
		} else if instanceTag == "vault" {
			common.UpdateDNSentry(server.PublicNet.IPv6.IP.String()+"1", "ackerson.de", 294257276)
			common.UpdateDNSentry(server.PublicNet.IPv4.IP.String(), "ackerson.de", 294257241)
		}
	}
}

func removeDeploymentFirewalls(client *hcloud.Client, ctx context.Context, instanceTag string, firewallTag string) {
	firewalls, _ := client.Firewall.AllWithOpts(context.Background(), hcloud.FirewallListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: firewallTag},
	})
	resources := []hcloud.FirewallResource{
		{
			Type: hcloud.FirewallResourceTypeLabelSelector,
			LabelSelector: &hcloud.FirewallResourceLabelSelector{
				Selector: "label=" + instanceTag},
		},
	}
	for _, firewall := range firewalls {
		// on weekly redeploys, we have 4 concurrent builds each with their own deployment key
		// delete the one you created or any > 10mins old
		// TODO: daily security check for unnecessary firewalls + cleanup (e.g. key >1hr old? nuke it...)
		if strings.HasSuffix(firewall.Name, os.Getenv("GITHUB_RUN_ID")) || firewall.Created.Before(time.Now().Add(-10*time.Minute)) {
			actions, _, _ := client.Firewall.RemoveResources(ctx, firewall, resources)
			for _, action := range actions {
				client.Action.WatchProgress(ctx, action)
			}

			for {
				_, err := client.Firewall.Delete(ctx, firewall)
				if err == nil {
					log.Printf("DELETED firewall %s\n", firewall.Name)
					break
				} else {
					log.Printf("waiting for firewall to be released: %s", err)
					time.Sleep(3 * time.Second)
				}
			}
		}
	}
}

func getExistingServer(client *hcloud.Client, tag string) *hcloud.Server {
	ctx := context.Background()
	opts := hcloud.ServerListOpts{ListOpts: hcloud.ListOpts{LabelSelector: "label=" + tag}}
	existingServers, _ := client.Server.AllWithOpts(ctx, opts)
	server := new(hcloud.Server)
	if len(existingServers) == 1 {
		server = existingServers[0]
	}

	return server
}

func Bool(b bool) *bool { return &b }

func String(s string) *string { return &s }

func createSSHKey(client *hcloud.Client, githubBuild string) *hcloud.SSHKey {
	privateKeyPair, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Printf("rsa.GenerateKey returned error: %v", err)
	}

	publicRsaKey, err := ssh.NewPublicKey(privateKeyPair.Public())
	if err != nil {
		log.Printf("ssh.NewPublicKey returned error: %v", err)
	}
	pubKeyBytes := ssh.MarshalAuthorizedKey(publicRsaKey)

	createRequest := hcloud.SSHKeyCreateOpts{
		Name:      githubBuild + "SSHkey",
		PublicKey: string(pubKeyBytes),
		Labels:    map[string]string{"access": "github"},
	}

	key, _, err := client.SSHKey.Create(context.Background(), createRequest)
	if err != nil {
		log.Printf("Keys.Create returned error: %v", err)
	} else {
		pemdata := pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(privateKeyPair),
			},
		)
		err := ioutil.WriteFile(sshPrivateKeyFilePath, pemdata, 0400)
		if err != nil {
			fmt.Printf("Failed to write %s: %s", sshPrivateKeyFilePath, err.Error())
		}
	}

	return key
}
