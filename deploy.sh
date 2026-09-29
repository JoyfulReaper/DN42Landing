#!/usr/bin/env bash

cd /opt/dn42landing/src

gofmt -w main.go
go build -o /tmp/dn42landing-new .

sudo install -m 0755 /tmp/dn42landing-new /usr/local/bin/dn42landing
sudo systemctl restart dn42landing
