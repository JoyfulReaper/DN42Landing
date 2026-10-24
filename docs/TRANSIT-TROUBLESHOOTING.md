# DN42 transit troubleshooting and recovery

> Quick reference for AS4242420425 Linux POPs. Diagnose before changing production forwarding. Updated 2026-10-24. See [peering runbook](PEERING-RUNBOOK.md) for the full procedure.

## Host and control-plane sanity

Always begin with `hostname`. Clanker and Wurstcase use similar commands but *different* interfaces, BIRD protocols, routing tables, and firewall rules. Running a Clanker drop-in on another Linux workstation once resulted in a harmless orphan configuration directory; remove only the unintended file/directory and reload systemd if that happens.

```bash
hostname
sudo wg show
sudo birdc show protocols
sudo birdc show status
ip -4 rule show
ip -6 rule show
sudo ufw status verbose
```

Do not interpret `show protocols` alone as proof of packet forwarding. “Established” proves a BGP control-plane session; it does not validate UFW, selected Linux FIB routes, return routing or remote acceptance of our advertisements.

## Symptom: WireGuard never handshakes

- Confirm the remote hostname resolves to the intended **address family**; test the agreed IPv4 or IPv6 endpoint directly rather than assuming all DNS answers work.
- Match public endpoint + UDP port, private/public keys, local listen port and allowed IPs.
- Check `systemctl status wg-quick@<interface>`, `sudo wg show <interface>`, and UDP input permission.
- Check for a Linux interface name longer than 15 bytes.
- Only after a handshake, verify link-local ping and BGP TCP/179.

Mark's useast01 handshake succeeded after switching to the known IPv4 underlay endpoint; that incident did not require rewriting BIRD.

## Symptom: BGP Active, or Established with few/no exports

```bash
sudo bird -p -c /etc/bird/bird.conf
sudo birdc show protocols all <protocol>
sudo journalctl -u bird -n 60 --no-pager
sudo birdc 'show route export <protocol>.ipv4'
sudo birdc 'show route export <protocol>.ipv6'
```

- Immediately following `birdc configure`, BIRD can briefly show `Active` while the BGP session reconnects. Check again before restarting anything.
- Check neighbor ASN/address, local AS, BGP IPv6 interface scope, IPv4 extended-next-hop capabilities, BGP TCP/179 and WG connectivity.
- Check import and export filters separately. Own-prefix-only export of **one aggregate per family** may be intentional. Controlled transit should export eligible third-party routes, with OTC, NO_EXPORT, NO_ADVERTISE, AS-path and ROA protections.
- Local aggregate routes may appear in BIRD as `unreachable` static routes. This is deliberate aggregation/advertisement; locally configured host addresses can still be delivered via kernel `local` routes.
- DN42 ROA rejection logs can represent expected filtering. Verify the ROA data is current rather than disabling validation.
- “Kernel dropped some netlink messages, will resync on next scan” means investigate route-table convergence if it persists; do not assume a one-off warning means the FIB is broken.

## Symptom: policy unit active, but `ip rule` entries missing

This **occurred on Clanker** with Baragoon, RoutedBits and Wurstcase: their oneshot services said `active (exited)` while the corresponding kernel policy rules were absent. Restarting the individual units restored rules. The root cause remains **unconfirmed**.

```bash
systemctl show <policy-unit>.service -p ActiveState -p ActiveEnterTimestamp
sudo systemctl cat <policy-unit>.service
ip -4 rule show
ip -6 rule show
sudo journalctl -b -u <policy-unit>.service --no-pager
systemctl is-active systemd-networkd
systemd-analyze cat-config systemd/networkd.conf
```

**Likely mechanism to investigate:** Clanker runs `systemd-networkd`; its default `ManageForeignRoutingPolicyRules=yes` may remove “foreign” rules installed by an external oneshot. No deletion event was captured in the available journal, so this is a hypothesis.

**Current state (2026-10-24):** Clanker has `/etc/systemd/networkd.conf.d/60-dn42-policy-routing.conf` containing:

```ini
[Network]
ManageForeignRoutingPolicyRules=no
```

The drop-in was verified with `systemd-analyze cat-config`, but **networkd has not yet been restarted to apply it and persistence has not been tested**. Do not claim it is active. Use provider console access and a controlled maintenance window for a networkd restart, then validate all BGP/WireGuard sessions, IPv4/IPv6 rules, routing tables and packet paths.

Wurstcase had `systemd-networkd` **inactive** when checked; do not apply the Clanker explanation there without evidence.

## Symptom: table exists, but peer traffic ignores it

A BIRD `pipe` and `kernel` protocol can be `UP` and a separate Linux table populated, but packets will still use `main` without a matching **ingress-specific `ip rule`**.

```bash
sudo birdc show protocols | grep transit
ip -4 route show table <id> | head
ip -6 route show table <id> | head
ip -4 rule show
ip -6 rule show
ip -4 route get <dn42-destination> from <peer-source> iif <incoming-tunnel>
```

Check rule priority order: own-prefix exception, table lookup, then terminal `prohibit`. Rule selectors use the **incoming interface**, so a route lookup without `iif` does not test the forwarded-packet policy. The priority-zero `local` rule may handle a local host address before the own-prefix exception; that is normal.

**Observed, locally tested paths:**

- Clanker, ingress MDR: `10.26.0.1` via RoutedBits, table `2401`.
- Clanker, ingress RoutedBits: `172.22.130.73` via ScopeCreep, table `2207`.
- Clanker, ingress Wurstcase: `10.26.0.1` via RoutedBits, table `4255`.
- Wurstcase, ingress Silverha: `10.26.0.1` and `fd00:47:ab07::1` via Clanker, table `2762`.

These were `ip route get` simulations, **not** partner-originated packet tests.

## Symptom: traffic can escape onto public Internet interface

If a dedicated table lacks a destination, Linux can continue searching later routing rules and ultimately the `main` default route. A public `ens3`/`eth0` default route should **never** be the fallback for unintended DN42 transit.

Test simulated ingress with documentation destinations (not real traffic):

```bash
ip -4 route get 203.0.113.1 from <peer-v4> iif <incoming-tunnel>
ip -6 route get 2001:db8::1 from <peer-v6> iif <incoming-tunnel>
```

Expected with terminal prohibit: `RTNETLINK answers: Permission denied` for each address family. If lookup selects `ens3` or `eth0`, **stop and investigate** before advertising further transit. Check that the policy unit owns an appropriate terminal `prohibit`, priorities are free, and policy rules still exist. Don't blindly flush/restart all routing services.

Current deployed fail-closed priorities:

| Host | Ingress | Table | Own/main | Peer table | Prohibit |
| --- | --- | ---: | ---: | ---: | ---: |
| Clanker | Baragoon | 1732 | 21860 | 21870 | 21875 |
| Clanker | RoutedBits | 2207 | 21880 | 21890 | 21895 |
| Clanker | Wurstcase | 4255 | 21900 | 21910 | 21915 |
| Clanker | MDR | 2401 | 21920 | 21930 | 21940 |
| Wurstcase | Silverha | 2762 | 22000 | 22010 | 22020 |

Use `systemctl cat` before modifying these. Baragoon, RoutedBits and Wurstcase on Clanker use drop-ins under `/etc/systemd/system/<unit>.service.d/10-fail-closed.conf`; MDR and Silverha have prohibitions in their original dedicated policy units.

## Symptom: apparent same-interface forwarding / potential loop

A master-table route learned from an internal router can point *back* to that router for packets arriving through the same interface. That is a forwarding **hairpin**, not automatically a sustained loop.

In the October 2026 ScopeCreep audit, Clanker's simulated ingress lookups for `172.22.130.73` and `fdbe:5664:5ef4::1` pointed back to `wg-dn42-core`. On ScopeCreep itself, route lookups pointed to independent external peers (Kioubit for IPv4, iEdon for IPv6), so neither sampled path was a demonstrated loop.

Before adding new transit tables to every internal POP, compare the *same destination* from **both sides**:

```bash
ip -4 route get <destination-v4> from <source-v4> iif <core-interface>
ip -6 route get <destination-v6> from <source-v6> iif <core-interface>
sudo birdc 'show route for <destination-v4> all'
sudo birdc 'show route for <destination-v6> all'
```

A route-reflector client, a downstream address-only POP and an internal router advertising hundreds of external paths may legitimately have different policies. Ordinary iBGP split-horizon helps with control-plane propagation but is not a blanket data-plane loop guarantee.

## Symptom: route inaccessible despite other external paths

The deployed pipe approach filters the master-table **selected** route. When the best route is excluded because it came from the ingress neighbor, the transit table might not include a non-best alternate learned from another peer. A terminal prohibit then correctly fails closed, but some otherwise reachable DN42 destinations could become unreachable for that ingress.

Inspect BIRD's alternate routes with `show route ... all` on relevant tables, compare the dedicated kernel FIB, and evaluate whether to change route-selection architecture. Do **not** remove fail-closed safeguards merely to make one lookup succeed.

## Firewall checks

All Linux POPs examined use UFW default `deny (routed)`. Allowed BGP inbound TCP/179 is separate from forwarded tunnel-to-tunnel traffic. Verify the intended pairs in both directions and IPv4/IPv6 (`sudo ufw status verbose` and `sudo ufw status`); do not open generic forwarding between DN42 and public underlay interfaces.

At Wurstcase, Silverha's forwarding pairs to Clanker, ScopeCreep and hbg1 were confirmed (12 IPv4/IPv6 direction-specific UFW rules). At Clanker, MDR has corresponding tunnel pairings; verify current rules before editing.

## Safe operational next steps

1. With console/rollback available, apply and validate Clanker's prepared networkd override; inspect actual `ip rule` entries after restart/reconfiguration.
2. Test rule persistence at boot/service restart on Wurstcase and Clanker; document who owns rules and when they are recreated.
3. Audit ScopeCreep's controlled transit policies and the other POPs' forwarding roles. Don't infer kernel-table IDs from a peer's looking-glass listing.
4. Run **real** source-to-destination IPv4 and IPv6 transit tests with at least one external partner, confirming the return path.
5. Track any observed route loss, `netlink` warnings and gaps between master-table best routes and dedicated ingress views.

For rollback, restore a known-good BIRD config backed up **outside** the wildcard `peers/*` include, validate before `birdc configure`, and stop only the newly added policy service. Use exact priorities and interface names when removing rules; a generic `ip rule del priority` can remove an unintended rule if priorities were reused.
