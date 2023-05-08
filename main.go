package main

import (
	"context"
	"fmt"
	"github.com/chyeh/pubip"
	"github.com/cloudflare/cloudflare-go"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	dockerClient "github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"net"
	"os"
	"strings"
	"time"
)

type CloudflareUpdater struct {
	DockerClient  *dockerClient.Client
	CloudflareApi *cloudflare.API
	Ip            net.IP
	Tld           string
}

func boolPointer(b bool) *bool {
	return &b
}

func (c *CloudflareUpdater) updateDomain(domain string) {
	zoneIdentifier := cloudflare.ZoneIdentifier(os.Getenv("CF_ZONE_ID"))

	records, _, err := c.CloudflareApi.ListDNSRecords(context.Background(), zoneIdentifier, cloudflare.ListDNSRecordsParams{Name: domain + "." + c.Tld})
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
				Comment: "Created from traefik",
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
				Comment: "Created from traefik",
			}
			_, err := c.CloudflareApi.CreateDNSRecord(context.Background(), zoneIdentifier, params)
			if err != nil {
				log.Errorln(err)
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
	if strings.Contains(value, c.Tld) {
		subdomain := strings.TrimSuffix(strings.TrimPrefix(value, "Host(`"), fmt.Sprintf(".%s`)", c.Tld))
		log.Infoln("Found domain: " + subdomain)
		c.updateDomain(subdomain)
	}
}

func (c *CloudflareUpdater) init() {
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
}

func (c *CloudflareUpdater) ipLoop() {

	for {
		ip, err := pubip.Get()
		if err != nil {
			fmt.Println("Couldn't get my IP address:", err)
		}

		if !ip.Equal(c.Ip) {
			c.init()
			c.Ip = ip
		}
		time.Sleep(60 * time.Second)
	}

}

func (c *CloudflareUpdater) eventLoop() {
	// Define a filter to only listen for service-related events
	filterArgs := filters.NewArgs()
	filterArgs.Add("type", "service")
	filterArgs.Add("event", "create")
	filterArgs.Add("event", "update")

	// Start the event stream
	eventStream, _ := c.DockerClient.Events(context.Background(), types.EventsOptions{
		Filters: filterArgs,
	})
	//if err != nil {
	//	log.Fatalf("failed to start event stream: %v", err)
	//}

	// Continuously listen for events
	for {
		select {
		case event := <-eventStream:
			if event.Type == events.ServiceEventType {
				// Service-related event, do something with it
				fmt.Println("Received service event:", event.Action, event.Actor.Attributes)
				service, _, err := c.DockerClient.ServiceInspectWithRaw(context.Background(), event.Actor.ID, types.ServiceInspectOptions{})
				if err != nil {
					log.Errorln(err)
					continue
				}
				c.processService(service)
			}
		}
	}
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
		Tld:           os.Getenv("TLD"),
	}

	cloudflareUpdater.init()

	go cloudflareUpdater.ipLoop()

	cloudflareUpdater.eventLoop()

}
