package main

import (
	"github.com/cloudflare/cloudflare-go"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
)

func main() {
	api, err := cloudflare.NewWithAPIToken(os.Getenv("CF_TOKEN"))

	if err != nil {
		log.Fatal(err)
	}

	for {
		status := getCurrentStatus()
		log.Infoln(status)
		if status != lastStatus {
			log.Infoln("Sending alert")
			success := sendPBAlert(status, pushBulletApiKey, deviceId)
			if success {
				lastStatus = status
			}
		}
		time.Sleep(10 * time.Second)
	}

}
