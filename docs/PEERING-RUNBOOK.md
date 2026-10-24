# DN42 peering runbook — AS4242420425

> Operator procedure for a **new** peer on Clanker, ScopeCreep, Wurstcase, or hbg1. This is a checklist and learning reference, **not** a universal script. Review the live configuration and peer agreement before changing routing. Updated 2026-10-24.

For the current topology and table assignments, see [README](../README.md). For failures and recovery, see [TRANSIT-TROUBLESHOOTING](TRANSIT-TROUBLESHOOTING.md).

## 0. Decide the peering relationship first

- **Own-prefix-only peering:** Accept ROA-valid DN42 routes from the neighbor but advertise only `172.20.220.48/28` and `fdf0:e12c:5528::/48`. No additional ingress-specific full-transit plumbing is automatically required.
- **Controlled full transit:** Advertise eligible third-party DN42 routes, accept forwarding traffic *from* the neighbor, and explicitly control its return path using a dedicated BIRD view, Linux routing table, `ip rule` policy, and UFW forwarding rules.
- **Internal iBGP:** Different from an external peer. Routes, route reflection, next-hop behavior and the forwarding role vary by POP. Do not automatically add transit pipes to every iBGP link.

Confirm the intended level of transit with the neighbor. An Established BGP session alone does **not** imply we agreed to forward third-party traffic.

## 1. Record the peering details

Before editing, write down: local POP; peer name and ASN; mutually agreed transit policy; peer WireGuard public key; remote public endpoint and UDP port; local listen UDP port; both link-local addresses; optional tunnel IPs; interface name; BIRD protocol name; throughput limit; contact; and rollback method.

Never commit private WireGuard keys, passwords, credentials or an exported `wg showconf` with secrets. A fresh keypair per peer simplifies revocation. Keep the private key root-readable only. Linux interface names are limited to **15 bytes** (we learned this when `wg-dn42-mdr-useast01` proved too long; `wg-dn42-mdr` works).

Check whether local ports, interface names, Linux table numbers and policy priorities are already in use. Resolve endpoint A and AAAA records explicitly and test the address family the remote UDP endpoint actually listens on: the MDR hostname initially resolved to an endpoint that did not handshake; the agreed IPv4 endpoint did.

Useful read-only discovery:

```bash
ip -br link
sudo wg show
sudo birdc show protocols
ip -4 rule show
ip -6 rule show
sudo ufw status verbose
systemctl is-active systemd-networkd
```

## 2. WireGuard: build transport, not routing policy

Why: WireGuard encrypts the point-to-point underlay; it does **not** exchange DN42 routes. BIRD/BGP does that. Use `wg-quick` with a peer-specific interface and key stored outside Git.

The production design commonly uses:

- `Table = off` in the interface configuration, so `wg-quick` does **not** install broad default routes.
- `AllowedIPs = 0.0.0.0/0, ::/0` **for the tunnel peer** when full DN42 next-hop forwarding is required. This controls WireGuard's cryptokey routing; with `Table = off` it does not itself authorize public Internet forwarding.
- An agreed IPv6 link-local pair (e.g. `fe80::425` on Clanker and a neighbor-specific address) for MP-BGP over the tunnel.
- `PersistentKeepalive = 25` where useful for NAT/underlay reachability.

Generate and store a unique private/public keypair securely. Confirm file permissions and never paste the private key into a ticket or repository. Use the existing peer configs as a reviewed template, **not** an uninspected `sed` substitution.

Activate the specific interface:

```bash
sudo systemctl enable --now wg-quick@<interface>.service
sudo wg show <interface>
ping -6 -I <interface> <peer-link-local>
```

Replace placeholders before executing. Verify a recent handshake, bidirectional byte counters and actual peer ping. If no handshake, check endpoint address family, UDP firewall, listen port, public keys and local service status before touching BGP.

## 3. BIRD: validate what we learn and control what we advertise

Why: BGP supplies prefix/next-hop routes. A route appearing in BIRD is not necessarily installed in Linux, and an accepted import is not automatically safe to export.

- Put the peer BGP configuration under `/etc/bird/peers/`, using a *unique* protocol name.
- Use the existing `is_valid_network()` / `is_valid_network_v6()` helpers and `roa_check(dn42_roa, net, bgp_path.last)` / IPv6 equivalent before importing external DN42 routes. Reject invalid or out-of-scope routes and the local aggregate.
- For an own-prefix-only peer, export **only** the registered aggregates; verify both are present in the relevant BIRD master tables.
- For a full-transit peer, continue advertising the own aggregate, then admit eligible external BGP routes while honoring OTC, NO_EXPORT, NO_ADVERTISE and AS-path loop restrictions. In particular, do not advertise any path containing the receiver's ASN. Examine `source`, `proto`, BIRD's iBGP export rules, and community logic in the existing peer; don't assume a generic “export all” is correct.
- Review `extended next hop on` / `require extended next hop on` for IPv4 NLRI carried over IPv6 and `next hop self` where appropriate for internal iBGP. BIRD version, interface and peer capabilities matter.
- The `dn42_update_link_metadata(...)` helper adds DN42 community metadata, not a substitute for routing policy or link measurement. Use `0` for values genuinely unknown.

Useful validations:

```bash
sudo bird -p -c /etc/bird/bird.conf
sudo birdc configure
sudo birdc show protocols all <bgp-protocol-name>
sudo birdc 'show route export <bgp-protocol-name>.ipv4'
sudo birdc 'show route export <bgp-protocol-name>.ipv6'
```

Do not proceed after a parse error. The initial `Active` state just after `birdc configure` can be a normal reconnect; confirm it reaches `Established` and both address-family channels report `UP`. Record imported/exported counts. They fluctuate as DN42 routes change; the important checks are intended policy, route content and next-hop validity.

## 4. Full transit: isolated forwarding view

**Skip this section for own-prefix-only peering unless a distinct forwarding requirement justifies it.**

Why: The Linux *main* FIB can choose an ingress peer as the next hop for traffic that *arrived from* that peer, or fall back to the VPS's public default route. Per-peer transit views reduce that risk. They are **not** automatically created by BGP.

Implementation on current Linux POPs:

1. Allocate unique BIRD `ipv4 table` and `ipv6 table` names, one unused Linux kernel routing table ID, and three unused policy priorities per address family.
2. Use a BIRD `pipe` from `master4/master6` into the dedicated table. The deployed approach exports routes selected in the master table and filters peer-learned third-party paths, while preserving directly originated peer routes (`bgp_path.len = 1`) so the peer's own prefixes remain reachable.
3. Use a BIRD `kernel` protocol with `kernel table <table-id>`, `import none`, and the dedicated table as its address-family source.
4. Verify populated IPv4 **and** IPv6 kernel tables **before** changing ingress traffic.
5. Important limitation: the pipe can only copy routes visible/selected in the master table; if the best route is filtered, it does not magically retrieve a non-best alternative. Also `bgp_path.len = 1` is only a heuristic for a directly originated prefix, not a cryptographic origin proof. Review route coverage and ROAs; a destination may intentionally fail closed.

Example (current deployed assignments):

| POP | Ingress | BIRD tables | Linux table |
| --- | --- | --- | ---: |
| Clanker | Baragoon (`wg-dn42-ny1`) | `baragoon_transit4/6` | 1732 |
| Clanker | RoutedBits (`wg-dn42-ewr1`) | `routedbits_transit4/6` | 2207 |
| Clanker | MDR/Mark (`wg-dn42-mdr`) | `mdr_transit4/6` | 2401 |
| Clanker | Wurstcase core (`wg-dn42-de1`) | `wurstcase_transit4/6` | 4255 |
| Wurstcase | Silverha (`wg-dn42-silver`) | `silverha_transit4/6` | 2762 |

ScopeCreep also has controlled alternate-route transit for Kioubit/iEdon, but its exact table IDs and rule persistence must be verified on that host rather than inferred from Clanker.

Inspect the candidate table with:

```bash
sudo birdc show protocols | grep -E 'transit|<peer>'
ip -4 route show table <table-id> | head -20
ip -6 route show table <table-id> | head -20
ip -4 route show table <table-id> default
ip -6 route show table <table-id> default
```

The dedicated view should contain DN42 next hops and intentionally retained direct-peer prefixes, **not** a public `eth0`/`ens3` default route.

## 5. Policy routing: select the view and fail closed

Why: Creating an alternate table doesn't use it. Linux uses prioritized `ip rule` selectors on packets entering the chosen interface.

The proven three-rule pattern **per family** is:

1. **Own allocation exception** (`iif <peer> to <our-prefix> lookup main`) so traffic for our local / connected services can be delivered normally.
2. **Per-peer lookup** (`iif <peer> lookup <table-id>`).
3. **Terminal prohibit** (`iif <peer> prohibit`) so a miss cannot fall through to the public main-table default.

On Clanker the current priority triplets are Baragoon `21860/21870/21875`, RoutedBits `21880/21890/21895`, Wurstcase `21900/21910/21915`, and MDR `21920/21930/21940`. Wurstcase/Silverha uses `22000/22010/22020`.

Implement persistence in a peer-named systemd service under `/etc/systemd/system/` (oneshot / `RemainAfterExit=yes`; ordered after the relevant WireGuard and BIRD services). Include matching cleanup on stop. Check the current units before writing a replacement: some older Clanker units use idempotent shell checks and `ExecStartPost` fail-closed drop-ins; do not paste a second copy of the rules.

```bash
sudo systemctl cat <policy-unit>.service
sudo systemd-analyze verify <policy-unit>.service
sudo systemctl daemon-reload
sudo systemctl enable --now <policy-unit>.service
ip -4 rule show
ip -6 rule show
```

**Caution:** a oneshot unit can remain `active (exited)` after some other service removes the kernel rules. Check the actual rules, not only `systemctl is-active`. Route lookups aimed at local addresses may be resolved by priority-0 `local` before the own-prefix exception—this is normal.

## 6. UFW: permit only intended forwarding directions

Why: Accepting BGP TCP/179 on a tunnel does **not** permit transit packets. The Linux POPs use UFW default `deny (routed)`. For every approved tunnel pair, add **both directions** of `ufw route allow in on <incoming> out on <outgoing>`, with descriptive comments. UFW's IPv6 setting must be enabled for matching IPv6 forwarding rules.

Keep the Internet-facing `eth0`/`ens3` out of the DN42 transit allow-list. Before adding rules, inspect existing interface pairings to avoid duplicates. On Wurstcase, Silverha has forwarding pairs to Clanker (`wg-dn42-de1`), ScopeCreep (`wg-dn42-sc`) and hbg1 (`wg-dn42-hbg1`); Clanker MDR has corresponding internal/external tunnel pairings.

Verify with `sudo ufw status verbose` / `sudo ufw status`, confirming both address families, pair directions and default routed policy.

## 7. Acceptance tests (control plane ≠ data plane)

Capture output for a change record. On the *correct host* (`hostname` first):

```bash
hostname
sudo wg show <interface>
sudo birdc show protocols all <bgp-protocol-name>
ip -4 rule show
ip -6 rule show
systemctl is-enabled wg-quick@<interface>.service <policy-unit>.service
systemctl is-active <policy-unit>.service
```

Then simulate forwarding **from a realistic DN42 source on the ingress peer**:

```bash
ip -4 route get <known-dn42-v4-destination> from <peer-v4> iif <interface>
ip -6 route get <known-dn42-v6-destination> from <peer-v6> iif <interface>
ip -4 route get 203.0.113.1 from <peer-v4> iif <interface>
ip -6 route get 2001:db8::1 from <peer-v6> iif <interface>
```

Expected: valid destinations use the dedicated table and an intended DN42 interface; the documentation destinations return `RTNETLINK answers: Permission denied`, never the public VPS interface. Validate reachability to our own services too. Finally, arrange an **actual end-to-end forwarded-packet test from the remote peer or another DN42 POP**, ideally in each address family. Route lookups and BGP export counts do not prove complete bidirectional packet flow.

For regressions, inspect `journalctl -u bird`, `wg show`, BIRD route selection, kernel table contents, UFW route rules and real next-hop decisions.

## 8. Change safety, rollback and known persistence caveat

- Back up the affected BIRD and systemd files **outside** a wildcard peer include directory; don't accidentally load a `.conf` backup twice.
- Run `bird -p -c /etc/bird/bird.conf` **before** `birdc configure`. Validate table population before installing `ip rule`.
- Avoid restarts of networkd, BIRD, WireGuard, UFW, or remote interfaces without a rollback plan and provider console when required.
- If activating a new policy fails, stop/disable only the new peer policy service (or delete only its documented priorities), restore the reviewed BIRD backup, validate and reconfigure; verify unrelated peers still work. `ip rule del priority` is broad: verify the corresponding priority is uniquely owned before deleting.
- Clanker previously lost three older ingress rule sets even though the oneshot units remained active. `systemd-networkd`'s default `ManageForeignRoutingPolicyRules=yes` is a **plausible**, not proven, cause. The drop-in `/etc/systemd/networkd.conf.d/60-dn42-policy-routing.conf` sets it to `no` and is **installed on Clanker**, but **activation by a controlled networkd restart and persistence testing are pending**.
- `systemd-networkd` was inactive on Wurstcase when checked; do not assume the same explanation or workaround applies there.
- Remaining work (2026-10-24): activate/validate Clanker's networkd setting safely; re-test policy-rule persistence, audit ScopeCreep and remaining POP roles, confirm alternate-route coverage, and perform real end-to-end transit tests with partners.

## Peer closeout checklist

- [ ] Record contact, ASN, endpoint, public key, link-locals, assigned interface, port, transport, policy and bandwidth agreement
- [ ] Key permissions correct; no private secrets in repository or ticket
- [ ] WireGuard up, bidirectional counters, recent handshake
- [ ] BIRD syntax check passes; IPv4/IPv6 BGP Established
- [ ] ROA imports and intended own-only/full-transit exports checked
- [ ] Full transit only: pipes and Linux tables populated with vetted routes
- [ ] Full transit only: policy rules and terminal prohibits installed in both families
- [ ] UFW tunnel pairings correct, both directions and address families; public underlay not transit-allowed
- [ ] DN42 lookup succeeds, public-default fallback fails, own prefixes reachable
- [ ] Verify actual end-to-end forwarding and response path
- [ ] Persistence/rollback documented and peer notified
