# AS4242420425 routing architecture and inventory

This is a snapshot of observed/configured routes and policies from October 2026, not a generated inventory or assertion that all data paths have passed an end-to-end test. See [peering runbook](PEERING-RUNBOOK.md) for deployment and [troubleshooting](TRANSIT-TROUBLESHOOTING.md) for recovery.

## POP roles

| POP | Role in internal BGP | External-peering notes |
| --- | --- | --- |
| Clanker | Multi-peer Linux transit edge; iBGP to ScopeCreep, Wurstcase, hbg1; dedicated return views for external peers and Wurstcase ingress | Baragoon, RoutedBits, MDR full transit |
| ScopeCreep | Linux routing POP and substantial source of remote DN42 routes to Clanker | Kioubit and iEdon full transit are documented in the README; local policy audit still pending |
| Wurstcase | Frankfurt Linux VPS; iBGP full mesh links to Clanker, ScopeCreep, hbg1 | Silverha DE1 full transit |
| hbg1 | Residential FreeBSD 15.1 POP; downstream/mesh routing, uses MP-BGP extended next hop | PuduNet peering own-prefix-only |
| overlay-gw | Additional iBGP route receiver connected to Clanker | On Clanker, `overlay_gw` is an RR client with `import none`; its session is not a general route source |

The four main routers form a six-link iBGP mesh. **BGP routes and Linux/FreeBSD forwarding policy serve different purposes**: advertising reachability does not by itself cause the kernel to use a peer-specific route table.

Clanker iBGP behavior observed:

- `scopecreep_core`: accepted **1,252 IPv4** / **1,092 IPv6** routes; **420/414** were preferred at observation. ScopeCreep can supply alternative external routes. Two samples sent from Clanker toward ScopeCreep were independently checked on ScopeCreep: `172.22.130.73` forwarded to `wg-dn42-kioubit`, `fdbe:5664:5ef4::1` to `wg-dn42-iedon`. This rules out a loop for those two examples, not every prefix.
- `hbg1_core`: import permits only hbg1's own host route (`172.20.220.53/32` and `fdf0:e12c:5528::53/128`); Clanker exported roughly 1,044/884 routes in the observation. No dedicated ingress transit view is currently documented for hbg1, and this is not by itself an error.
- `overlay_gw`: `rr client`, `import none`, roughly 1,468 IPv4 / 1,320 IPv6 exported. Route-reflector status does **not** mean every other iBGP session is an RR client; check the actual session topology before changing iBGP distribution.
- Wurstcase is treated differently at Clanker's forwarding plane: arriving traffic enters a `wurstcase_transit4/6` filtered view. A direct `scopecreep_core` path can still be legitimate for a destination **behind** ScopeCreep. A same-interface hairpin is not proof of a loop without the remote router's decision.

## Transit ingress map (Linux)

| Router | Ingress | BIRD view | Linux table | Rules (own prefix / lookup / prohibit) |
| --- | --- | --- | ---: | --- |
| Clanker | `wg-dn42-ny1` / Baragoon | `baragoon_transit4/6` | 1732 | 21860 / 21870 / 21875 |
| Clanker | `wg-dn42-ewr1` / RoutedBits | `routedbits_transit4/6` | 2207 | 21880 / 21890 / 21895 |
| Clanker | `wg-dn42-de1` / Wurstcase | `wurstcase_transit4/6` | 4255 | 21900 / 21910 / 21915 |
| Clanker | `wg-dn42-mdr` / MDR | `mdr_transit4/6` | 2401 | 21920 / 21930 / 21940 |
| Wurstcase | `wg-dn42-silver` / Silverha | `silverha_transit4/6` | 2762 | 22000 / 22010 / 22020 |

Each entry uses matching IPv4/IPv6 rules. Every group has an own-allocation exception to `main`, the indicated dedicated-table lookup, then `prohibit` to prevent fallback to the public route. Existing Clanker Baragoon/RoutedBits/Wurstcase oneshot services received `10-fail-closed.conf` drop-ins; MDR and Wurstcase Silverha oneshot services include prohibit directly.

The BIRD tables hold **selected routes from master** filtered through pipes. For a route selected from the ingress peer's BGP session, directly originated (AS_PATH length 1) prefixes remain; third-party routes learned through that peer are excluded. This avoids some same-peer transit returns but does not provide arbitrary alternate path selection. Test destination coverage rather than assuming a full copy of DN42 is installed in the alternate kernel table.

## Observed validation status

| Item | Verified locally | Still unverified / caution |
| --- | --- | --- |
| Clanker / MDR `AS4242420401` | WireGuard recent handshake; BGP Established, 1,411 IPv4 and 1,211 IPv6 exports; table 2401 routes; UFW tunnel forwarding; valid route lookup and fail-closed denied lookup | No independent end-to-end packet forwarded from Mark through Clanker; no assurance every prefix has an alternate route |
| Wurstcase / Silverha `AS4242420762` | BGP Established; ~1,263 IPv4 and 1,143 IPv6 exports on final check; table 2762 populated (1,468 IPv4 / 1,302 IPv6 entries at measurement); valid lookups; fail-closed denied; all 12 expected UFW forwarding rules | No independent remote transit packet proof; alternate-route coverage not exhaustively checked |
| Clanker / Baragoon | Fail-closed IPv4/IPv6 verified after oneshot drop-in | End-to-end retest and persistence |
| Clanker / RoutedBits | Fail-closed IPv4/IPv6 verified; valid lookup selected table 2207 | End-to-end retest and persistence |
| Clanker / Wurstcase ingress | Fail-closed IPv4/IPv6 verified; valid lookup selected table 4255 | End-to-end retest and persistence |
| Clanker / networkd | `/etc/systemd/networkd.conf.d/60-dn42-policy-routing.conf` contains `ManageForeignRoutingPolicyRules=no`; `systemd-analyze cat-config` sees it | A controlled restart to *apply* it and a subsequent persistence test; no conclusive evidence networkd removed the old rules |
| Wurstcase / networkd | `systemd-networkd` inactive when checked | Confirm the actual networking manager if persistence fails later |
| ScopeCreep and remaining POPs | Sample ScopeCreep forwarding destinations verified | Full ingress-policy, firewall, route-distribution and persistence audit |

## Security and documentation boundaries

- The network allocation is `172.20.220.48/28` and `fdf0:e12c:5528::/48`; POP host routes live inside it.
- Do not commit any WireGuard **private** key, BGP session secret, auth token, VPN management address, credentials or raw configuration dump containing secrets.
- Do not treat `BGP Established` or `systemctl is-active` as a conclusive forwarding/persistence test.
- Clanker's default public interface is `ens3`; Wurstcase's is `eth0`. Their public default routes must not be selected by incoming DN42 transit when the peer-specific table has no route.
- On Wurstcase, Silverha UFW route rules allow forwarding only to Clanker `wg-dn42-de1`, ScopeCreep `wg-dn42-sc`, and hbg1 `wg-dn42-hbg1`, bidirectionally, for both IP families. The public uplink is not a DN42 transit egress.
