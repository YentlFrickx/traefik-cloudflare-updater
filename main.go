package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chyeh/pubip"
	"github.com/cloudflare/cloudflare-go"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type CloudflareUpdater struct {
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
	} else if len(records) == 0 {
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
		log.Infoln("Created dns entry for " + domain)
	} else if len(records) > 0 && records[0].Content != c.Ip.String() {
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
		log.Infoln("Updated dns entry for " + domain)
	}
}

func (c *CloudflareUpdater) extractHostnames(jsonData string) ([]string, error) {
	log.Infoln("Extracting hostnames from data")
	log.Infoln(jsonData)
	var data []map[string]interface{}
	err := json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		return nil, err
	}

	var hostnames []string
	for _, entry := range data {
		if rule, ok := entry["rule"].(string); ok {
			if host, err := c.extractHostname(rule); err == nil {
				if strings.HasSuffix(host, "."+c.Tld) {
					hostnames = append(hostnames, strings.TrimSuffix(host, "."+c.Tld))
				}
			}
		}
	}
	return hostnames, nil
}

func (c *CloudflareUpdater) extractHostname(rule string) (string, error) {
	// split rule on backticks
	parts := []rune(rule)
	for i := 0; i < len(parts); i++ {
		if parts[i] == '`' {
			// extract the string between the backticks
			j := i + 1
			for ; j < len(parts) && parts[j] != '`'; j++ {
			}
			if j > i+1 {
				return string(parts[i+1 : j]), nil
			}
		}
	}
	return "", fmt.Errorf("No hostname found in rule '%s'", rule)
}

func (c *CloudflareUpdater) getRoutes() (string, error) {
	url := "http://traefik:80/api/http/routers"
	header := http.Header{}
	header.Set("Host", "traefik.internal")

	res, err := http.Get(url)
	if err != nil {
		log.Errorln("error while fetching routes")
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Errorln("error while reading routes")
		return "", err
	}

	return string(body), nil
}

func (c *CloudflareUpdater) checkHostnames() {
	jsonData, err := c.getRoutes()
	if err != nil {
		log.Errorln(err)
		return
	}

	hostnames, err := c.extractHostnames(jsonData)
	if err != nil {
		log.Errorln(err)
		return
	}

	for _, hostname := range hostnames {
		c.updateDomain(hostname)
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

	cloudflareUpdater := CloudflareUpdater{
		CloudflareApi: api,
		Ip:            ip,
		Tld:           os.Getenv("TLD"),
	}

	for {
		cloudflareUpdater.checkHostnames()
		time.Sleep(60 * time.Second)
	}

}
