#!/usr/bin/env python3
import sys
from pathlib import Path

root = Path(sys.argv[1] if len(sys.argv) > 1 else ".").resolve()

files = {
    "src/main.go": [
        (
            'defaultIPv6Listen = "[fdf0:e12c:5528::10]:80"',
            'defaultIPv6Listen = "[fdf0:e12c:5528::50]:80"',
        ),
    ],
    "src/static/index.html": [
        (
            '<p class="lede">JoyfulReaper / kgivler on DN42 — New York, US</p>',
            '<p class="lede">JoyfulReaper / kgivler on DN42 — multi-edge hobby network</p>',
        ),
        (
            '''        <dt>Router</dt><dd>Clanker</dd>
        <dt>Routing</dt><dd>BIRD 2</dd>''',
            '''        <dt>Edges</dt><dd>Clanker + ScopeCreep</dd>
        <dt>Routing</dt><dd>BIRD 2 / internal iBGP core</dd>''',
        ),
        (
            '''        <dt>IPv6</dt><dd><code>fdf0:e12c:5528::10</code></dd>
        <dt>DNS</dt><dd><code>kgivler.dn42</code> planned</dd>
        <dt>HTTPS</dt><dd>DN42 CA investigation planned</dd>''',
            '''        <dt>IPv6</dt><dd><code>fdf0:e12c:5528::50</code></dd>
        <dt>DNS</dt><dd><code>joyfulreaper.dn42</code></dd>
        <dt>HTTPS</dt><dd>Available via DN42 CA / Burble ACME</dd>''',
        ),
        (
            '''    <strong>AS4242420425 currently operates as a stub network.</strong>
    I accept DN42 routes from peers and currently export only my own registered
    prefixes. Providing DN42 transit is in scope for future experimentation,
    but peers should not currently depend on this network for transit.''',
            '''    <strong>AS4242420425 is a multi-edge, non-transit hobby network.</strong>
    I accept DN42 routes from external peers on both Clanker and ScopeCreep and
    currently export only my own registered prefixes. The two edge routers
    exchange routes over a dedicated WireGuard core using IPv4 and IPv6 iBGP.
    Providing DN42 transit may become a future experiment, but peers should not
    currently depend on this network for transit.''',
        ),
        (
            '''      <tr><th>Network</th><th>ASN</th><th>Location</th><th>State</th></tr>''',
            '''      <tr><th>Network</th><th>ASN</th><th>Location</th><th>Edge</th><th>State</th></tr>''',
        ),
        (
            '''      <tr><td>RoutedBits</td><td><code>AS4242420207</code></td><td>Newark, NJ</td><td class="status-up">Established</td></tr>
      <tr><td>Baragoon</td><td><code>AS4242421732</code></td><td>New York, NY</td><td class="status-up">Established</td></tr>
      <tr><td>HEADSCARF175</td><td><code>AS4242420842</code></td><td>Piscataway, NJ</td><td class="status-up">Established</td></tr>''',
            '''      <tr><td>RoutedBits</td><td><code>AS4242420207</code></td><td>Newark, NJ</td><td>Clanker</td><td class="status-up">Established</td></tr>
      <tr><td>Baragoon</td><td><code>AS4242421732</code></td><td>New York, NY</td><td>Clanker</td><td class="status-up">Established</td></tr>
      <tr><td>HEADSCARF175</td><td><code>AS4242420842</code></td><td>Piscataway, NJ</td><td>Clanker</td><td class="status-up">Established</td></tr>
      <tr><td>iEdon</td><td><code>AS4242422189</code></td><td>Dallas, TX</td><td>ScopeCreep</td><td class="status-up">Established</td></tr>
      <tr><td>Kioubit</td><td><code>AS4242423914</code></td><td>Los Angeles, CA</td><td>ScopeCreep</td><td class="status-up">Established</td></tr>
      <tr><td>MOE233</td><td><code>AS4242420253</code></td><td>Las Vegas, NV</td><td>ScopeCreep</td><td class="status-up">Established</td></tr>''',
        ),
        (
            '''    <li><strong>DN42Landing</strong> — <code>172.20.220.50</code> / <code>fdf0:e12c:5528::10</code></li>
    <li><strong>Random Steam Game Picker</strong> — planned for DN42, with <code>randomsteam.dn42</code> planned as a dedicated DN42 domain.</li>
    <li>More services will appear as I get around to breaking them.</li>
  </ul>''',
            '''    <li><strong>Network / peering landing</strong> — <a href="https://joyfulreaper.dn42/">joyfulreaper.dn42</a></li>
    <li><strong>Personal site</strong> — <a href="https://kgivler.dn42/">kgivler.dn42</a></li>
    <li><strong>Random Steam Game Picker</strong> — <a href="https://randomsteam.dn42/">randomsteam.dn42</a> (HTTPS required)</li>
    <li><strong>Random GitHub</strong> — <a href="https://randomgit.dn42/">randomgit.dn42</a></li>
    <li>More services will appear as I get around to breaking them.</li>
  </ul>

  <p class="warning">
    <strong>Clearnet resource note:</strong> Some services exposed on DN42 still
    reference resources hosted on the normal Internet, such as CDN-hosted CSS or
    JavaScript, images, fonts, APIs, or other external assets. Visiting a
    <code>.dn42</code> service may therefore cause your browser or client to make
    additional clearnet requests. A DN42 hostname does not currently guarantee
    that every resource used by the page remains inside DN42.
  </p>''',
        ),
    ],
    "README.md": [
        (
            '''- Location: New York, US
- Router: Clanker
- Routing daemon: BIRD 2''',
            '''- Edge routers: Clanker and ScopeCreep
- Routing daemon: BIRD 2
- Internal routing: IPv4/IPv6 iBGP over a dedicated WireGuard core''',
        ),
        (
            '''Planned landing addresses:

- IPv4: `172.20.220.50`
- IPv6: `fdf0:e12c:5528::10`''',
            '''Shared web-service ingress:

- IPv4: `172.20.220.50`
- IPv6: `fdf0:e12c:5528::50`

Current DN42 web services:

- `joyfulreaper.dn42` — network / peering landing
- `kgivler.dn42` — personal site
- `randomsteam.dn42` — Random Steam Game Picker
- `randomgit.dn42` — Random GitHub''',
        ),
        (
            '''AS4242420425 currently operates as a **stub network** and exports only its own
registered prefixes. Providing DN42 transit is explicitly in scope for future
experimentation, but peers must not currently depend on AS4242420425 for
transit.''',
            '''AS4242420425 currently operates as a **multi-edge, non-transit hobby network**.
It accepts DN42 routes from external peers on both Clanker and ScopeCreep and
currently exports only its own registered prefixes. Providing DN42 transit is
in scope for future experimentation, but peers must not currently depend on
AS4242420425 for transit.''',
        ),
        (
            '| `DN42LANDING_IPV6_LISTEN` | `[fdf0:e12c:5528::10]:80` |',
            '| `DN42LANDING_IPV6_LISTEN` | `[fdf0:e12c:5528::50]:80` |',
        ),
        (
            'sudo ip -6 addr add fdf0:e12c:5528::10/128 dev dn42-dummy',
            'sudo ip -6 addr add fdf0:e12c:5528::50/128 dev dn42-dummy',
        ),
        (
            "curl -g -v --connect-timeout 5 'http://[fdf0:e12c:5528::10]/'",
            "curl -g -v --connect-timeout 5 'http://[fdf0:e12c:5528::50]/'",
        ),
        (
            '''## Next steps

After plain HTTP is working:

1. persist the `.50` / `::10` addresses on Clanker
2. prepare ScopeCreep for DN42/Ygg routing through Clanker
3. add authoritative DNS
4. register `kgivler.dn42`
5. register `randomsteam.dn42`
6. add reverse DNS
7. investigate DN42 CA / ACME and HTTPS
8. publish Random Steam over DN42''',
            '''## DN42 web/TLS status

The registered DN42 names are live through the shared `.50` / `::50` nginx
ingress. Certificates are issued through Burble's DN42 ACME service and chain
to the DN42 certificate authority.

`randomsteam.dn42` redirects HTTP to HTTPS. The other current web services
support both HTTP and HTTPS without forcing a redirect.

### Clearnet resource caveat

DN42 reachability does **not** currently imply DN42-only browsing. Some services
may reference resources on the normal Internet, including CDN-hosted CSS or
JavaScript, images, fonts, APIs, and other external assets. A browser visiting a
`.dn42` hostname may therefore make additional clearnet requests.

The landing page itself is intentionally self-contained, but that should not be
assumed for every service linked from it.''',
        ),
    ],
}

changed_files = []

for rel, replacements in files.items():
    path = root / rel
    if not path.is_file():
        raise SystemExit(f"ERROR: expected file not found: {path}")

    text = path.read_text(encoding="utf-8")
    original = text

    for old, new in replacements:
        count = text.count(old)
        if count != 1:
            raise SystemExit(
                f"ERROR: expected exactly one match in {rel}, found {count}:\n{old[:160]}"
            )
        text = text.replace(old, new, 1)

    if text != original:
        path.write_text(text, encoding="utf-8", newline="\n")
        changed_files.append(rel)

print("Updated:")
for rel in changed_files:
    print(f"  {rel}")
print("\nRun:")
print("  git diff --check")
print("  git diff --stat")
print("  git diff")
