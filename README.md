# DN42Landing

Small DN42 landing page and manual peering-request inbox for **AS4242420425**.

The project is intentionally boring:

- one Go binary
- embedded HTML/CSS
- SQLite for peering requests
- optional ntfy notification on submission
- no JavaScript framework
- no automatic WireGuard/BIRD changes
- no public admin UI

## Current network

- ASN: `AS4242420425`
- IPv4 prefix: `172.20.220.48/28`
- IPv6 prefix: `fdf0:e12c:5528::/48`
- Location: New York, US
- Router: Clanker
- Routing daemon: BIRD 2
- Peering transport: WireGuard
- BGP: MP-BGP over IPv6 link-local, Extended Next Hop where supported
- Route validation: DN42 ROA validation

Planned landing addresses:

- IPv4: `172.20.220.50`
- IPv6: `fdf0:e12c:5528::10`

## Peering policy

Manual peering requests are open.

AS4242420425 currently operates as a **stub network** and exports only its own
registered prefixes. Providing DN42 transit is explicitly in scope for future
experimentation, but peers must not currently depend on AS4242420425 for
transit.

This is a hobby network. No SLA or uptime guarantee is provided.

A fresh WireGuard keypair is generated for each approved peer. There is no
single public WireGuard key because peer-specific keys make rotation and
revocation less painful.

## Peering request flow

A requester submits:

- ASN
- network name
- contact
- public endpoint
- WireGuard public key
- IPv6 link-local preference
- notes

Submission:

1. validates basic fields
2. is globally rate-limited to one valid submission every 15 minutes
3. is stored in SQLite
4. optionally fires an ntfy notification
5. does **not** modify WireGuard, BIRD, firewall rules, or routing

After manual approval, the operator creates the peer-specific WireGuard
interface and BIRD configuration and replies with local key/port/link-local
details.

## Build

Requires Go 1.25+.

```bash
cd src
go mod tidy
go build -o dn42landing .
```

## Run locally

```bash
mkdir -p ./data

DN42LANDING_IPV4_LISTEN=127.0.0.1:8080 \
DN42LANDING_IPV6_LISTEN= \
DN42LANDING_DB=./data/peering.db \
./dn42landing
```

Open `http://127.0.0.1:8080/`.

## Configuration

Environment variables:

| Variable | Default |
|---|---|
| `DN42LANDING_IPV4_LISTEN` | `172.20.220.50:80` |
| `DN42LANDING_IPV6_LISTEN` | `[fdf0:e12c:5528::10]:80` |
| `DN42LANDING_DB` | `/var/lib/dn42landing/peering.db` |
| `DN42LANDING_NTFY_URL` | `http://127.0.0.1:5197/dn42-peering` |
| `DN42LANDING_NTFY_TOKEN` | empty |
| `DN42LANDING_RATE_LIMIT` | `15m` |

If `DN42LANDING_NTFY_URL` is empty, ntfy is disabled.

## First Clanker smoke test

The following address commands are intentionally **temporary**. They let us test
before changing however `dn42-dummy` is persisted on Clanker:

```bash
sudo ip addr add 172.20.220.50/32 dev dn42-dummy
sudo ip -6 addr add fdf0:e12c:5528::10/128 dev dn42-dummy

ip -br addr show dn42-dummy
```

Build and install:

```bash
cd src
go mod tidy
go build -o dn42landing .

sudo install -m 0755 dn42landing /usr/local/bin/dn42landing
sudo install -m 0644 ../systemd/dn42landing.service /etc/systemd/system/dn42landing.service

sudo systemctl daemon-reload
sudo systemctl enable --now dn42landing
sudo systemctl status dn42landing
```

Optional ntfy configuration:

```bash
sudo install -m 0600 /dev/null /etc/dn42landing.env
sudo nano /etc/dn42landing.env
```

Example:

```text
DN42LANDING_NTFY_URL=http://127.0.0.1:5197/dn42-peering
DN42LANDING_NTFY_TOKEN=replace-me
```

Then:

```bash
sudo systemctl restart dn42landing
```

Test from another DN42-connected host:

```bash
curl -v --connect-timeout 5 http://172.20.220.50/
curl -g -v --connect-timeout 5 'http://[fdf0:e12c:5528::10]/'
```

## SQLite

The application creates the schema automatically.

Useful inspection:

```bash
sudo sqlite3 /var/lib/dn42landing/peering.db \
  'select id, submitted_utc, asn, network_name, contact, status from peering_requests order by id desc;'
```

No peering request data is exposed through a public HTTP endpoint.

## Next steps

After plain HTTP is working:

1. persist the `.50` / `::10` addresses on Clanker
2. prepare ScopeCreep for DN42/Ygg routing through Clanker
3. add authoritative DNS
4. register `kgivler.dn42`
5. register `randomsteam.dn42`
6. add reverse DNS
7. investigate DN42 CA / ACME and HTTPS
8. publish Random Steam over DN42
