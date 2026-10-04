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
- Public peering edges: Clanker and ScopeCreep
- Residential POP: hbg1 (FreeBSD 15.1, Harrisburg, PA)
- Routing daemon: BIRD 2
- Internal routing:
  - Clanker ↔ ScopeCreep: dedicated WireGuard core with internal IPv4/IPv6 BGP
  - Clanker ↔ hbg1: WireGuard over native IPv6 with one IPv6 MP-BGP session
  - hbg1 ↔ ScopeCreep: WireGuard over native IPv6 with one IPv6 MP-BGP session
  - IPv4 NLRI on both hbg1 core sessions uses RFC 8950 Extended Next Hop
  - Clanker, ScopeCreep, and hbg1 form a full three-router iBGP mesh
- Peering transport: WireGuard
- External BGP: MP-BGP over IPv6 link-local where supported
- Route validation: DN42 ROA validation

Shared web-service ingress:

- IPv4: `172.20.220.50`
- IPv6: `fdf0:e12c:5528::50`

Current DN42 web services:

- `joyfulreaper.dn42` — network / peering landing
- `kgivler.dn42` — personal site
- `randomsteam.dn42` — Random Steam Game Picker
- `randomgit.dn42` — Random GitHub

## Peering policy

Manual peering requests are open.

AS4242420425 operates as a **multi-edge hobby network experimenting with
controlled transit**. It accepts DN42 routes from external peers on both
Clanker and ScopeCreep. Most external sessions remain own-prefix-only exports
unless explicitly configured otherwise.

Clanker currently provides controlled full-table IPv4 and IPv6 transit to
Baragoon (`AS4242421732`) and RoutedBits (`AS4242420207`). Each peer uses
dedicated BIRD alternate routing tables together with a dedicated Linux policy
routing table: `baragoon_transit4` / `baragoon_transit6` with table `1732`,
and `routedbits_transit4` / `routedbits_transit6` with table `2207`. Traffic
to AS4242420425's own prefixes bypasses the transit policy, while third-party
routes learned from the ingress peer are excluded from that peer's return
transit view to prevent hairpinning. Directly originated peer prefixes remain
reachable over the direct session. Both peering interfaces are rate-limited
to 50 Mbps in each direction.

ScopeCreep also provides controlled full-table IPv4 and IPv6 transit to iEdon
(`AS4242422189`) using a dedicated alternate-route policy view with equivalent
anti-hairpin behavior. That peering interface is also limited to 50 Mbps in
both directions.

Transit is experimental. This is a hobby network; no SLA or uptime guarantee
is provided.

### Residential POP / hbg1

hbg1 is a FreeBSD 15.1 residential POP in Harrisburg, PA. Its DN42 router
identities are `172.20.220.53/32` and `fdf0:e12c:5528::53/128`.

Its internal core links to both Clanker and ScopeCreep run WireGuard over
native residential IPv6. Each uses an IPv6 MP-BGP session carrying both IPv4
and IPv6 routes; IPv4 uses RFC 8950 Extended Next Hop. Together, Clanker,
ScopeCreep, and hbg1 form a full three-router iBGP mesh. Failover has been
tested by taking the hbg1-to-Clanker BGP session down and confirming hbg1
retained its DN42 routes through ScopeCreep.

Manual external peering requests are accepted for hbg1, but the external
WireGuard underlay is IPv6-only: peers must provide a publicly reachable IPv6
WireGuard endpoint. This is only an underlay requirement; the MP-BGP session can
still carry both IPv4 and IPv6 DN42 NLRI.

Because hbg1 sits on a 200 Mbps residential connection, each external peer will
be capped at 5 Mbps in each direction. hbg1 peering is best-effort/experimental
and does not include general transit by default. Endpoint details are supplied
after manual approval rather than publishing a residential address here.

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
| `DN42LANDING_IPV6_LISTEN` | `[fdf0:e12c:5528::50]:80` |
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
sudo ip -6 addr add fdf0:e12c:5528::50/128 dev dn42-dummy

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
curl -g -v --connect-timeout 5 'http://[fdf0:e12c:5528::50]/'
```

## SQLite

The application creates the schema automatically.

Useful inspection:

```bash
sudo sqlite3 /var/lib/dn42landing/peering.db \
  'select id, submitted_utc, asn, network_name, contact, status from peering_requests order by id desc;'
```

No peering request data is exposed through a public HTTP endpoint.

## DN42 web/TLS status

The registered DN42 names are live through the shared `.50` / `::50` nginx
ingress. Certificates are issued through Burble's DN42 ACME service and chain
to the DN42 certificate authority.

The primary web hostnames use HTTPS as their canonical form:

- `http://kgivler.dn42/*` redirects to `https://kgivler.dn42/*`
- `http://randomgit.dn42/*` redirects to `https://randomgit.dn42/*`
- `http://randomsteam.dn42/*` redirects to `https://randomsteam.dn42/*`

The corresponding `www.*.dn42` aliases are present in DNS and covered by the
Burble-issued certificates. Both HTTP and HTTPS requests to those aliases
redirect to the bare HTTPS hostname while preserving the path and query string.

### Clearnet resource caveat

DN42 reachability does **not** currently imply DN42-only browsing. Some services
may reference resources on the normal Internet, including CDN-hosted CSS or
JavaScript, images, fonts, APIs, and other external assets. A browser visiting a
`.dn42` hostname may therefore make additional clearnet requests.

The landing page itself is intentionally self-contained, but that should not be
assumed for every service linked from it.
