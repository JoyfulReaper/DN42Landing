# DN42 transit troubleshooting and recovery

Applies to the observed Linux/BIRD 2/UFW configuration of AS4242420425 as of October 2026. This is a **read-only-first** guide. Inspect the actual host before editing services. See [peering runbook](PEERING-RUNBOOK.md) and [routing inventory](ROUTING-ARCHITECTURE.md).

## First: establish which host you are on

```sh
hostname
sudo birdc show status
sudo wg show
ip -4 rule show
ip -6 rule show
```

Clanker's transit-policy services are **not** installed on a local workstation. During the October rollout, a Wurstcase-ingress systemd drop-in was accidentally created on `altnet-workstation`; no matching unit existed. It was safely undone by deleting only that new file, removing its now-empty directory with `rmdir`, and running `systemctl daemon-reload`. Verify the hostname before every write.

## Symptom: WireGuard sends but never receives

```sh
sudo wg show <interface>
ip -br link show <interface>
getent ahosts <remote-hostname>
sudo ufw status verbose
```

Check peer/public keys, endpoint IP and family, UDP listener and firewall, allowed IPs, port, underlay reachability and interface name length (Linux max 15 chars). When MDR `useast01.dn42.markround.com` resolved to IPv6, Clanker did not handshake; using the provided IPv4 endpoint `104.207.128.176:20425` worked. Verify that the remote endpoint actually listens on the selected family. Do not replace a functioning keypair or open UFW to `Anywhere` as an indiscriminate fix.

## Symptom: WireGuard up, BGP not Established

```sh
sudo birdc show protocols all <peer-protocol>
sudo journalctl -u bird -n 80 --no-pager
sudo bird -p -c /etc/bird/bird.conf
```

Confirm remote ASN, BGP addresses and interface scope (`fe80::peer%wg-interface`), TCP/179 on the tunnel, source address, RFC8950 Extended Next Hop and v4/v6 channels. Immediately after `birdc configure`, `BGP state: Active` with a short connect timer can be transient; recheck once rather than restarting everything. If the session persists in Active, inspect logs and remote configuration.

## Symptom: BGP Established but no usable transit

Compare the **imported, exported and preferred** counts:

```sh
sudo birdc show protocols all <peer-protocol>
sudo birdc 'show route export <peer-protocol>.ipv4'
sudo birdc 'show route export <peer-protocol>.ipv6'
sudo birdc 'show route for 172.20.220.48/28 export <peer-protocol>.ipv4'
sudo birdc 'show route for fdf0:e12c:5528::/48 export <peer-protocol>.ipv6'
```

If the peer receives only our aggregates, it is **own-prefix-only** regardless of how many routes Clanker imports. If it receives a full table, review the propagation policy (OTC, NO_EXPORT/NO_ADVERTISE, AS_PATH, ROA-valid ranges). Static `unreachable` for our *aggregate* is expected for prefix origination and does not imply that the router's assigned host addresses are unreachable. ROA check failures in the logs may be normal rejected announcements; review unexpected bulk rejection patterns.

**A BGP session never proves a forwarded packet is working.** Request remote traceroute/ping tests with explicit source/destination and test both address families.

## Symptom: alternate table missing, empty or wrong next hop

```sh
sudo birdc show protocols | grep -E 'transit_(pipe|kernel)'
ip -4 route show table <table-id> | head -20
ip -6 route show table <table-id> | head -20
sudo birdc 'show route for <destination> all'
```

The kernel may report `FIB table does not exist` for a table that has **never been created**; that's different from an existing table with no matching route. Confirm pipe and kernel protocols are UP and the configured BIRD table and Linux table number match. The source/master table may have chosen the ingress peer as best path: a pipe filter excluding that path will **not automatically substitute** an unselected alternate BGP route. Review actual candidate paths before broadening the filter.

The Silverha table on Wurstcase (`2762`) was observed with 1,468 IPv4 and 1,302 IPv6 entries, including directly originated Silverha prefixes; these are snapshots, not a fixed expected count. Both families had no default route.

## Symptom: policy service is active but `ip rule` is missing

```sh
systemctl status <service>
systemctl cat <service>
systemctl show <service> -p ActiveEnterTimestamp -p ActiveState
ip -4 rule show; ip -6 rule show
sudo journalctl -b -u <service> --no-pager -n 100
systemctl is-active systemd-networkd
systemd-analyze cat-config systemd/networkd.conf
```

Clanker had three oneshot transit-policy services reported as active while their rules had disappeared. They were restarted successfully on 2026-10-23 around 16:26–16:27 EDT, restoring the rules. The output **did not establish who removed them**. A `Type=oneshot`/`RemainAfterExit=yes` service reports that its `ExecStart` succeeded; it does not continuously enforce a kernel rule.

Clanker runs `systemd-networkd`, whose default `ManageForeignRoutingPolicyRules=yes` makes deletion of externally created rules during reconfiguration a **plausible** explanation, not confirmed root cause. A drop-in was written:

```ini
# /etc/systemd/networkd.conf.d/60-dn42-policy-routing.conf
[Network]
ManageForeignRoutingPolicyRules=no
```

`systemd-analyze cat-config systemd/networkd.conf` displayed that drop-in. **It was NOT yet applied via a controlled networkd restart** when documented. Do not claim persistence fixed until the setting is active and the rules survive controlled reconfiguration/reboot checks. This setting prevents networkd managing *foreign policy rules*, not its normal routes.

Wurstcase's `systemd-networkd` was **inactive**, so we did not apply that workaround there. An inactive service's default `yes` is not evidence of rule cleanup.

**Recovery:** confirm the actual cause and unit behavior; if authorized, restore only the affected named policy service/rules, then verify both families and fail-closed behavior. Avoid `ip rule flush` or a blind restart of networking. Never reboot a remote VPS without alternate access if you depend on that interface.

## Symptom: route lookup escapes via public `eth0`/`ens3`

A per-ingress table lookup can fall through to `main` if it has no matching route. Each approved ingress has a terminal `prohibit` rule *after* its table lookup:

| Host | Ingress | Terminal rule priority |
| --- | --- | ---: |
| Clanker | Baragoon `wg-dn42-ny1` | 21875 |
| Clanker | RoutedBits `wg-dn42-ewr1` | 21895 |
| Clanker | Wurstcase `wg-dn42-de1` | 21915 |
| Clanker | MDR `wg-dn42-mdr` | 21940 |
| Wurstcase | Silverha `wg-dn42-silver` | 22020 |

Use a simulated *forwarded* lookup with `iif` and realistic peer source address. On Wurstcase:

```sh
ip -4 route get 10.26.0.1 from 172.22.98.193 iif wg-dn42-silver
ip -6 route get fd00:47:ab07::1 from fd99:900f:968f::1 iif wg-dn42-silver
ip -4 route get 203.0.113.1 from 172.22.98.193 iif wg-dn42-silver
ip -6 route get 2001:db8::1 from fd99:900f:968f::1 iif wg-dn42-silver
```

The first two selected table `2762` via Clanker, and both public documentation destinations returned `RTNETLINK answers: Permission denied` in the observed tests. That is the intended fail-closed outcome. It is not a replacement for a real packet test.

A query to a router's own assigned IP may show `table local` instead of `main`, because Linux evaluates the built-in local table at priority zero first.

## Symptom: firewall allows BGP but forwarding still fails

```sh
sudo ufw status verbose
sudo ufw status | grep 'ALLOW FWD'
sudo sysctl net.ipv4.ip_forward net.ipv6.conf.all.forwarding
ip -4 rule show; ip -6 rule show
```

UFW `allow in` to TCP/179 only permits BGP to the router; it is **not** a `route allow` for packets passing through the router. On the Linux POPs, the expected policy is default `deny (routed)` with explicit interface-to-interface `ALLOW FWD` pairs for each intended direction, including IPv6 variants. For Wurstcase/Silverha, 12 rules (both directions across three core links, IPv4/IPv6) were observed. Do not allow arbitrary forwarding to the clearnet interface.

If firewall rules exist, check kernel forwarding toggles, alternate-table coverage, source restrictions, return path and conntrack. Output of `ufw status` is configuration evidence, not evidence packets reached the destination.

## Symptom: apparent ScopeCreep routing loop

Clanker receives many routes from ScopeCreep, and for some destinations a simulated packet with `iif wg-dn42-core` is forwarded back out that same interface. Two destinations were investigated:

- `172.22.130.73`: ScopeCreep selected `wg-dn42-kioubit`.
- `fdbe:5664:5ef4::1`: ScopeCreep selected `wg-dn42-iedon`.

These samples **were not loops**, because ScopeCreep's next hop was an external peer, not Clanker. Same-interface forwarding on Clanker does not by itself establish an actual loop, and separate ScopeCreep ingress tables are not automatically required merely because it is another POP in the same ASN. For a new suspect destination, check *both routers' exact FIB and BIRD best path* before redesigning the iBGP mesh.

## Outstanding operational checklist

- [ ] With VPS console access, arrange a controlled way to apply Clanker's networkd override; recheck `ip rule` both families and each service afterward.
- [ ] Prove policy rules persist after permitted network changes/reboot (not merely `active (exited)`).
- [ ] Verify Mark and Silverha from the remote end with a real forwarded IPv4 and IPv6 transit packet.
- [ ] Audit ScopeCreep's iEdon/Kioubit tables, rule persistence, UFW and fail-closed tests, then hbg1/overlay-gw roles and forwarding.
- [ ] Review alternate-route coverage in each per-peer pipe: a filtered selected route may leave an avoidable gap.
- [ ] Record dates, observed outputs and safe rollback for each resolved issue.
