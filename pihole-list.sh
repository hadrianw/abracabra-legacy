#!/bin/sh
set -e

wget -N https://raw.githubusercontent.com/jdiazmx/pi-hole/master/adlists.default

wget -i <(grep -v "^#" adlists.default) -O - > pihole.hosts
