# DN42 peering runbook — AS4242420425

This is an **operator runbook**, not configuration automation. As of October 2026 the Linux examples reflect Clanker (BIRD 2.18) and Wurstcase (BIRD 2.17.5). Adapt interface names, tables, addresses, route filters and firewall paths to the actual POP; never paste example values blindly. hbg1 is FreeBSD and needs an OS-specific procedure.

Use the [routing inventory](ROUTING-ARCHITECTURE.md) and [troubleshooting guide](TRANSIT-TROUBLESHOOTING.md) with this runbook.

## 0. Choose the peering role *before* editing anything

- **Own-prefix-only peering:** exchange routes but advertise only our `172.20.220.48/28` and `fdf0:e12c:5528::/48` aggregates. Do not create per-peer transit tables unless a separate forwarding policy requires them.
- **Controlled full transit:** advertise eligible third-party routes as well. Requires checked import/export policy, symmetric tunnel-to-tunnel firewall permissions for the intended paths, and ingress-specific forwarding tables and policy routing if using this network's anti-hairpin design.
- **Internal iBGP/core:** not the same as an external customer/peer relationship. Confirm route-reflector topology and which POPs are expected to import remote DN42 routes.

Document: remote ASN, contact, POP/location, public endpoint **and IP family**, tunnel port, remote WireGuard public key, remote link-local IP, local interface and port, whether full transit is mutually agreed, bandwidth policy, POP-specific core tunnel interfaces, and rollback plan. Keep private keys and credentials out of Git, emails and tickets.

Before production changes, capture:

```sh
hostname
sudo wg show
sudo birdc show protocols
ip -4 rule show; ip -6 rule show
ip -4 route show default; ip -6 route show default
sudo ufw status verbose
systemctl is-active systemd-networkd
```

## 1. WireGuard transport

1. Agree on endpoint, UDP listening ports, link-local addresses and public keys. A hostname may resolve to a different address family than the remote firewall or listener supports: Clanker initially failed to handshake with MDR using the hostname but succeeded using his published IPv4 `104.207.128.176:20425`.
2. Choose a **Linux interface name of at most 15 characters**; for MDR we used `wg-dn42-mdr`.
3. Generate a **new private key per peer** in a root-readable-only location, e.g.:

   ```sh
   sudo install -d -m 700 /etc/wireguard
   sudo sh -c 'umask 077; wg genkey > /etc/wireguard/PEER.key'
   sudo sh -c 'wg pubkey < /etc/wireguard/PEER.key > /etc/wireguard/PEER.pub'
   ```

   Replace `PEER` with a deliberate unique identifier and refuse to overwrite a pre-existing key; the commands above are **illustrative only**, not a safe overwrite-proof script. Exchange *only* the contents of the `.pub` file.

4. Configure `/etc/wireguard/<interface>.conf` with the root-only private key, local listen port, local tunnel address (commonly `fe80::.../64`), remote peer public key, endpoint and agreed `AllowedIPs`. For BGP-managed full DN42 routing, `Table = off` prevents wg-quick from injecting a default route into Linux's main table. Some existing DN42 tunnels use `AllowedIPs = 0.0.0.0/0, ::/0`; that is a WireGuard packet authorization/routing scope, **not** permission to send general clearnet traffic.
5. Start the tunnel only after syntax/permissions review: `sudo systemctl enable --now wg-quick@<interface>`. Check `sudo wg show <interface>`, link-local reachability, a recent handshake and traffic in both directions. Review the corresponding inbound UDP firewall rule. A handshake alone does not establish BGP or transit forwarding.

MDR working example: local `wg-dn42-mdr`, port `20401`, local `fe80::425`, remote `fe80::401`, remote `104.207.128.176:20425`. Silverha uses Wurstcase's `wg-dn42-silver`; do **not** reuse the MDR configuration there.

## 2. BIRD: routes, validation and export policy

Wurstcase and Clanker use BIRD 2, with configuration included from `/etc/bird/peers/*`, and existing `dn42_roa` / `dn42_roa_v6` tables. Confirm how includes and helpers are defined on the selected machine before creating another file.

For external MP-BGP over IPv6 link-local, configure remote ASN, `interface "<wg-interface>"`, `neighbor fe80::... as <REMOTE_ASN>`, separate `ipv4` / `ipv6` channels, and **extended next hop** for IPv4 over IPv6. Use the existing BIRD files for exact syntax, including neighbor/local address assumptions and import limits. ROA-valid imports should enforce DN42 address-space validation, reject our own allocations when appropriate, and reject invalid route origins. A BGP `Established` state does not mean the filters are right.

**Own-prefix-only exports:** explicitly allow our aggregate prefixes and reject all other routes. Ensure those aggregates actually exist as BIRD static routes. A static aggregate can show as `unreachable` in BIRD while correctly originating the aggregate; more-specific routes still serve actual endpoints.

**Full-transit exports:** in addition to our own aggregates, allow eligible DN42 routes learned through other peers while rejecting:
- routes carrying OTC when the relationship does not permit further propagation;
- `NO_EXPORT` / `NO_ADVERTISE` standard communities;
- AS paths containing the receiving external peer's ASN;
- invalid address space, inappropriate self subprefixes or routes prohibited by local policy.

Use the current Clanker MDR and Wurstcase Silverha export filters as **examples, not universal templates**: their conditions differ (Silverha uses `source = RTS_BGP` and `proto != "silverha_de1"`; MDR additionally checks its ASN in the AS path). Keep `dn42_update_link_metadata(...)` metadata honest: `0` for unknown bandwidth/loss, and values only when measured or justified. A latency measurement is not evidence of a bandwidth guarantee.

Back up the working config **outside** the `peers/*` include glob, then validate:

```sh
sudo cp -a /etc/bird/peers/PEER.conf /etc/bird/PEER.conf.pre-change
sudo bird -p -c /etc/bird/bird.conf
sudo birdc configure
sudo birdc show protocols all PROTOCOL_NAME
sudo birdc 'show route export PROTOCOL_NAME.ipv4'
sudo birdc 'show route export PROTOCOL_NAME.ipv6'
```

Stop on parser errors. `birdc configure` may briefly restart a changed BGP session; `Active` immediately afterward can be transient. Confirm `Established` and realistic imported/exported counts **after** reconvergence. Do not silently treat thousands of received routes as thousands of exported routes.

## 3. Dedicated transit views (controlled full transit only)

Purpose: for packets *arriving from peer X*, select a forwarding view that avoids returning **third-party** routes to X, while keeping X's directly originated prefixes reachable. This is a local forwarding policy separate from BGP export filtering.

Known deployed Linux ingress tables:

| Router | Incoming peer | BIRD tables | Kernel table | Own-prefix exception | Table lookup | Terminal prohibit |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| Clanker | Baragoon (`wg-dn42-ny1`) | `baragoon_transit4/6` | 1732 | 21860 | 21870 | 21875 |
| Clanker | RoutedBits (`wg-dn42-ewr1`) | `routedbits_transit4/6` | 2207 | 21880 | 21890 | 21895 |
| Clanker | Wurstcase (`wg-dn42-de1`) | `wurstcase_transit4/6` | 4255 | 21900 | 21910 | 21915 |
| Clanker | MDR (`wg-dn42-mdr`) | `mdr_transit4/6` | 2401 | 21920 | 21930 | 21940 |
| Wurstcase | Silverha (`wg-dn42-silver`) | `silverha_transit4/6` | 2762 | 22000 | 22010 | 22020 |

The per-peer BIRD configuration declares `ipv4 table ...` and `ipv6 table ...`, `protocol pipe` instances from `master4/6` to the alternate tables with `import none` and an export filter, then `protocol kernel` instances exporting those tables into the chosen Linux kernel table number with `import none`. The example below shows only the **filtering idea**, not a complete drop-in:

```bird
# For the alternate table of ingress peer X:
if proto = "PEER_PROTOCOL" then {
    if bgp_path.len = 1 then accept;  # directly originated at that eBGP peer
    reject;                           # routes learned by that peer from others
}
accept;
```

**Important limitation:** a pipe sees the selected route in the source table. If the best route is excluded, BIRD does not automatically substitute the second-best BGP path from the same `master` table. Missing routes can legitimately produce `Permission denied` under fail-closed policy. Check coverage against intended transit commitments; never advertise a route merely because a peer-specific FIB exists.

Before assigning numbers, inspect both IPv4 and IPv6 tables and current `ip rule` priorities. Create the BIRD tables/pipes/kernels first and confirm they populate. Only then enable the Linux ingress rules.

## 4. Linux ingress policy and fail-closed semantics

For each peer, install rules in **both address families** and in this order:

1. `iif <peer> to 172.20.220.48/28 lookup main` (IPv6 equivalent `fdf0:e12c:5528::/48`) so our internal endpoints remain reachable.
2. `iif <peer> lookup <dedicated-table>` for other DN42 destinations.
3. `iif <peer> prohibit` so missing entries **cannot fall through to the public default** in `main`.

Make these persistent through a named systemd oneshot service using `RemainAfterExit=yes`, suitable `After`/`Wants` links, matching `ExecStop` cleanup and distinct priorities. Verify the exact service with `systemctl cat` before changing it. For existing Clanker services, `ExecStartPost`/`ExecStopPost` drop-ins add prohibit rules, with priorities in the table above. A systemd active oneshot does **not** guarantee its rules are still installed.

Examples of **read-only** validation (replace addresses and interface names with documented peer-specific values):

```sh
ip -4 rule show; ip -6 rule show
ip -4 route show table 2762 | head
ip -6 route show table 2762 | head

# Example on Wurstcase: legitimate traffic arriving from Silverha
ip -4 route get 10.26.0.1 from 172.22.98.193 iif wg-dn42-silver
ip -6 route get fd00:47:ab07::1 from fd99:900f:968f::1 iif wg-dn42-silver

# Documentation destinations must NOT escape via the public interface
ip -4 route get 203.0.113.1 from 172.22.98.193 iif wg-dn42-silver
ip -6 route get 2001:db8::1 from fd99:900f:968f::1 iif wg-dn42-silver
```

Expected valid transit: the **dedicated table** and an authorized DN42 egress. Expected documentation destinations: `RTNETLINK answers: Permission denied`. Linux table `local` at priority 0 takes precedence when the destination is an address assigned to the router itself; that is normal.

## 5. UFW and forwarding

Confirm `net.ipv4.ip_forward=1` and `net.ipv6.conf.all.forwarding=1`. The deployed Linux routing edges use UFW default **deny (routed)**, so approved tunnel pairs require explicit `ufw route allow in on <ingress> out on <egress>` in **each direction**. Check output includes both IPv4 and IPv6 rules. BGP TCP/179 must also be permitted on the appropriate tunnel, and the WireGuard UDP listener on the underlay.

On Wurstcase, Silverha has explicitly verified bidirectional forwarded paths to `wg-dn42-de1`, `wg-dn42-sc` and `wg-dn42-hbg1` (12 IPv4/IPv6 UFW entries total). On Clanker, MDR's authorized DN42 tunnel pairs were checked. **Never permit arbitrary DN42 ingress to forward out the public `eth0`/`ens3` interface** as a shortcut. Rule presence is not proof of successful end-to-end packet forwarding.

## 6. Final acceptance checklist

- [ ] Recorded contact, ASN, endpoint (resolved address family), WireGuard public keys, link-local addresses, role and bandwidth terms.
- [ ] Router hostname checked before editing; unique WireGuard name `<=15` chars; private key file permissions checked.
- [ ] `wg-quick` enabled; handshake, traffic in both directions, tunnel reachability verified.
- [ ] BIRD import filters and ROA tables checked; BGP IPv4/IPv6 `Established`; accepted routes and own-prefix exports verified.
- [ ] Full-transit peers: no unauthorized OTC, standard no-export or peer-own-AS paths exported.
- [ ] Full-transit peers: BIRD pipes/kernel tables up; dedicated IPv4/IPv6 FIB populated; own allocation reachable.
- [ ] Full-transit peers: correct `ip -4/-6 rule show`, prohibit rules, valid route lookups and denied public-fallback lookups.
- [ ] UFW input/forwarding rules correct for all authorized tunnel pairs; no unintended public egress.
- [ ] Policy rules rechecked after any BIRD/network reconfiguration; persistence across controlled restart/reboot tested.
- [ ] If possible, remote peer confirms receipt of routes and an actual **two-way forwarded packet** works (traceroute/ping from a permitted remote source).
- [ ] Backup/rollback recorded; peer operator notified only after local checks pass.

## 7. Rollback and operational safety

- Back up BIRD config before editing; validate with `bird -p`. On a problematic policy rollout, keep console or management access available. Undo the *new* ingress service/rules first (not the entire WireGuard or BGP network), verify forwarding, then restore the previous BIRD config and reconfigure if required.
- Never rely on `ip rule flush`, a blanket `ip route flush`, `ufw reset`, `systemctl restart systemd-networkd`, or network-interface restarts as a first troubleshooting step over SSH.
- `Type=oneshot` with `RemainAfterExit=yes` only records that the commands ran; it does not enforce rules after other software deletes them.
- The **Clanker** drop-in at `/etc/systemd/networkd.conf.d/60-dn42-policy-routing.conf` sets `ManageForeignRoutingPolicyRules=no`. It was installed and visible via `systemd-analyze cat-config` but **had not yet been loaded through a controlled networkd restart** at the time of these notes. Networkd's default foreign-rule management is a plausible explanation for previously vanished rules, **not a proven root cause**. Wurstcase had `systemd-networkd` inactive when checked and did not receive the same drop-in.
- Finish with actual peer-to-peer transit checks and an audit of alternate-route coverage. The local validation performed on Clanker/Mark and Wurstcase/Silverha does **not yet prove an end-to-end forwarded packet from the external peer**.
