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
- Public VPS peering edges: Clanker, ScopeCreep, and Wurstcase (Frankfurt, Germany)
- Residential POP: hbg1 (FreeBSD 15.1, Harrisburg, PA)
- Routing daemon: BIRD 2
- Internal routing:
  - Clanker ↔ ScopeCreep: dedicated WireGuard core with internal IPv4/IPv6 BGP
  - Clanker ↔ hbg1: WireGuard over native IPv6 with one IPv6 MP-BGP session
  - hbg1 ↔ ScopeCreep: WireGuard over native IPv6 with one IPv6 MP-BGP session
  - IPv4 NLRI on both hbg1 core sessions uses RFC 8950 Extended Next Hop
  - Clanker ↔ Wurstcase: WireGuard core with IPv6 MP-BGP/RFC 8950
  - ScopeCreep ↔ Wurstcase: WireGuard core with IPv6 MP-BGP/RFC 8950
  - hbg1 ↔ Wurstcase: WireGuard core with IPv6 MP-BGP/RFC 8950
  - Clanker, ScopeCreep, hbg1, and Wurstcase form a full four-router iBGP mesh (six links)
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
controlled transit**. It accepts DN42 routes from external peers across
all four POPs. Most external sessions remain own-prefix-only exports unless
explicitly configured otherwise.

Clanker currently provides controlled full-table IPv4 and IPv6 transit to
Baragoon (`AS4242421732`), RoutedBits (`AS4242420207`), and MDR/Mark Round
(`AS4242420401`, useast01/New Jersey). These external sessions have dedicated
BIRD alternate routing tables and ingress-specific Linux policy routing. An
additional ingress-specific view handles traffic arriving from the Wurstcase
iBGP core. The current Clanker assignments are:

| Ingress | BIRD tables | Linux table | Policy priorities (own / lookup / prohibit) |
| --- | --- | ---: | --- |
| Baragoon | `baragoon_transit4/6` | 1732 | `21860 / 21870 / 21875` |
| RoutedBits | `routedbits_transit4/6` | 2207 | `21880 / 21890 / 21895` |
| Wurstcase core | `wurstcase_transit4/6` | 4255 | `21900 / 21910 / 21915` |
| MDR | `mdr_transit4/6` | 2401 | `21920 / 21930 / 21940` |

Traffic to AS4242420425's own prefixes can use the main routing table, while
the peer-specific views filter selected third-party routes learned from that
ingress peer, retaining directly originated peer prefixes. Each policy has a
terminal `prohibit` rule to stop unmatched ingress traffic falling back to a
public Internet default route. These filters reduce hairpinning but do not
guarantee a non-best alternate path is present for every DN42 destination.
The MDR export filters also reject OTC, NO_EXPORT/NO_ADVERTISE and paths
containing MDR's ASN. Baragoon and RoutedBits interfaces are rate-limited to
50 Mbps in each direction; an MDR bandwidth cap has not been documented.

ScopeCreep also provides controlled full-table IPv4 and IPv6 transit to iEdon
(`AS4242422189`) and Kioubit (`AS4242423914`), using dedicated per-peer
alternate-route policy views with equivalent anti-hairpin behavior. Those
peering interfaces are limited to 50 Mbps in both directions.

Wurstcase is the Frankfurt VPS POP, with an established Silverha DE1
(`AS4242420762`) session using controlled full-table transit. Silverha ingress
uses dedicated BIRD tables `silverha_transit4/6`, Linux routing table `2762`,
and the `22000 / 22010 / 22020` IPv4/IPv6 policy-rule priorities (own,
lookup, prohibit). The selected DN42 table routes and fail-closed lookups were
verified locally, as were UFW forwarding pairs for Clanker, ScopeCreep and hbg1.
hbg1's PuduNet NY01 (`AS4242422227`) session exports only our own prefixes
and is capped at 5 Mbps per direction; it does not offer general transit.

Transit is experimental. This is a hobby network; no SLA or uptime guarantee
is provided.

### Operator runbooks and outstanding validation

- [Peering runbook](docs/PEERING-RUNBOOK.md) — adding an own-prefix-only or
  full-transit peer, WireGuard/BIRD setup, ROA filters, ingress pipes, Linux
  policy rules, UFW, tests and rollback.
- [Transit troubleshooting](docs/TRANSIT-TROUBLESHOOTING.md) — missing rules,
  BGP reconnections, fail-closed checks, forwarding hairpins and recovery.

**Status as of 2026-10-24:** Mark and Silverha are configured for controlled
full transit; BGP, dedicated Linux route tables, selected forwarding lookups,
and fail-closed policy routing have been verified locally. **End-to-end
forwarded-packet tests with the remote peers are still pending.** Clanker
previously lost older `ip rule` entries while their systemd oneshot services
continued reporting active. A `systemd-networkd` drop-in setting
`ManageForeignRoutingPolicyRules=no` has been installed on Clanker but
**has not yet been activated or validated via a controlled networkd restart**.
Networkd cleanup is a plausible cause of the missing rules, not a confirmed
root cause. Wurstcase did not have networkd active when checked. Further
policy-rule persistence tests and remaining POP audits are pending.

### Residential POP / hbg1

hbg1 is a FreeBSD 15.1 residential POP in Harrisburg, PA. Its DN42 router
identities are `172.20.220.53/32` and `fdf0:e12c:5528::53/128`.

Its internal core links to both Clanker and ScopeCreep run WireGuard over
native residential IPv6. Each uses an IPv6 MP-BGP session carrying both IPv4
and IPv6 routes; IPv4 uses RFC 8950 Extended Next Hop. Together with Wurstcase,
the four routers form a full six-link iBGP mesh. Failover has been tested by
taking the hbg1-to-Clanker BGP session down and
confirming hbg1 retained its DN42 routes through ScopeCreep.

Manual external peering requests are accepted for hbg1. hbg1 can initiate
WireGuard toward an approved peer's reachable IPv4 or IPv6 endpoint; unsolicited
inbound reachability on its residential underlay should not be assumed.
Within the tunnel, IPv6 link-local MP-BGP carries both IPv4 and IPv6 DN42 NLRI.

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
