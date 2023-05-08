package main

import (
	"context"
	"fmt"
	"github.com/cloudflare/cloudflare-go"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	log "github.com/sirupsen/logrus"
	"net"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"

	"github.com/chyeh/pubip"
)

type CloudflareUpdater struct {
	DockerClient  *dockerClient.Client
	CloudflareApi *cloudflare.API
	Ip            net.IP
	Domains       []string
}

func boolPointer(b bool) *bool {
	return &b
}

func (c *CloudflareUpdater) updateDomains() {
	zoneIdentifier := cloudflare.ZoneIdentifier(os.Getenv("CF_ZONE_ID"))

	for _, domain := range c.Domains {
		records, _, err := c.CloudflareApi.ListDNSRecords(context.Background(), zoneIdentifier, cloudflare.ListDNSRecordsParams{Name: domain})
		if err != nil {
			log.Errorln(err)
		} else {
			if len(records) > 0 {
				params := cloudflare.UpdateDNSRecordParams{
					Type:    "A",
					Name:    domain,
					ID:      records[0].ID,
					Content: c.Ip.String(),
					TTL:     1,
					Proxied: boolPointer(true),
					Tags:    []string{"Created from traefik"},
				}
				_, err := c.CloudflareApi.UpdateDNSRecord(context.Background(), zoneIdentifier, params)
				if err != nil {
					log.Errorln(err)
				}
			} else {
				params := cloudflare.CreateDNSRecordParams{
					Type:    "A",
					Name:    domain,
					Content: c.Ip.String(),
					TTL:     1,
					Proxied: boolPointer(true),
					Tags:    []string{"Created from traefik"},
				}
				_, err := c.CloudflareApi.CreateDNSRecord(context.Background(), zoneIdentifier, params)
				if err != nil {
					log.Errorln(err)
				}
			}
		}
	}
}

func (c *CloudflareUpdater) processService(service swarm.Service) {
	for label, value := range service.Spec.Labels {
		if strings.Contains(label, "rule") && strings.Contains(value, "Host") {
			c.checkRule(value)
		}
	}
}

func (c *CloudflareUpdater) checkRule(value string) {
	tld := os.Getenv("TLD")
	if strings.Contains(value, tld) {
		subdomain := strings.TrimSuffix(strings.TrimPrefix(value, "Host(`"), fmt.Sprintf(".%s`)", tld))
		log.Infoln("Found domain: " + subdomain)
		c.Domains = append(c.Domains, subdomain)
	}
}

func (c *CloudflareUpdater) initialCheck() {
	log.Infoln("Running initial check")
	// List all running services in the Docker Swarm with the traefik.enable=true label
	labelFilter := filters.NewArgs()
	labelFilter.Add("label", "traefik.enable=true")

	services, err := c.DockerClient.ServiceList(context.Background(), types.ServiceListOptions{Filters: labelFilter})
	if err != nil {
		log.Fatalf("failed to list Docker services: %v", err)
	}

	for _, service := range services {
		c.processService(service)
	}

	c.updateDomains()
}

func main() {
	api, err := cloudflare.NewWithAPIToken(os.Getenv("CF_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	ip, err := pubip.Get()
	if err != nil {
		fmt.Println("Couldn't get my IP address:", err)
	}

	// Docker client initialization
	client, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("failed to create Docker client: %v", err)
	}

	cloudflareUpdater := CloudflareUpdater{
		DockerClient:  client,
		CloudflareApi: api,
		Ip:            ip,
		Domains:       []string{},
	}

	cloudflareUpdater.initialCheck()

	for {

		time.Sleep(10 * time.Second)
	}

}
