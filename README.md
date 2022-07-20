![Deploy Hetzner Homepage Server](https://github.com/ackersonde/hetzner_home/workflows/Deploy%20Hetzner%20Homepage%20Server/badge.svg)

# Hetzner Home
Since DigitalOcean has increased their prices and Hetzner has some 4x the offerings (half the price + double the resources) I've moved my [homepage](https://ackerson.de), [slack](https://github.com/ackersonde/bender-slackbot) & [telegram](https://github.com/ackersonde/telegram-bot) bots over.

# Build & Deploy [Hetzner Home](https://cloud.digitalocean.com/droplets)
Using the Hetzner's golang api from [godo](https://pkg.go.dev/github.com/hetznercloud/hcloud-go), every push to this repository creates a [custom](https://github.com/ackersonde/hetzner_home/blob/main/scripts/raw_ubuntu_userdata.sh) Ubuntu <img src="https://assets.ubuntu.com/v1/29985a98-ubuntu-logo32.png" width="16"> instance in Nuremberg.

# Automated Deployment
I have a [weekly cronjob](https://github.com/ackersonde/pi-ops/blob/master/scripts/crontab.txt) running on one of my raspberry PIs which triggers this deployment after regenerating the SSL certificate ([only valid for 10d](https://github.com/ackersonde/pi-ops/blob/master/scripts/gen_new_deploy_keys.sh#L18)) required for login.

# WARNING
Every push to this repo will result in a new server created at Hetzner => +$4 / month, tearing down and redeploying websites and bots while also updating DNS entries for *.ackerson.de.

Use git commit msg string snippet `[skip ci]` to avoid this.
