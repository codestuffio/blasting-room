# Deploy: Coolify behind a Cloudflare Tunnel

The public demo runs on a homelab Coolify instance. A Cloudflare Tunnel puts it on a public hostname without opening any ports.

```
visitor ─HTTPS─► Cloudflare edge ─tunnel─► cloudflared (homelab) ─HTTP─► Coolify proxy ─► container :8080
```

Cloudflare handles TLS. Everything after the tunnel is plain HTTP on the homelab network.

## 1. Create the app in Coolify

1. **New Resource → Application**, from the GitHub repo.
   - While the repo is private, Coolify needs access: add it through a GitHub App source in Coolify, or use a deploy key. Once the repo is public, "Public Repository" works.
2. **Build:**
   - Branch `main`
   - Build Pack **Dockerfile**, base directory `/`, Dockerfile `/Dockerfile`
3. **Network:**
   - Ports Exposes: `8080`
4. **Domain:**
   - Enter it as `http://blastingroom.example.com`, with `http://`. TLS ends at Cloudflare, and an `https://` domain makes Coolify's proxy request a certificate and redirect to HTTPS, which loops behind the tunnel.
5. **Environment variables.** These are runtime variables; leave "Build Variable" unticked so the keys aren't baked into the image.

   | Variable | Value |
   |---|---|
   | `ANTHROPIC_API_KEY` | your key |
   | `QLOO_API_KEY` | your hackathon key |
   | `TICKETMASTER_API_KEY` | your Discovery API consumer key (not the secret) |
   | `BR_CLIENT_IP_HEADER` | `CF-Connecting-IP` |

   Optional: `BR_DAILY_RUNS`, `BR_EFFORT`, `BR_MAX_TURNS`, `BR_MODEL` (defaults are in the README). `BR_DAILY_RUNS` (default 100) caps runs per UTC day across all visitors, which bounds the Anthropic bill while the demo is public. Don't set `PORT` or `STATIC_DIR`; the image sets them.

   `BR_CLIENT_IP_HEADER` matters. Behind the tunnel every request reaches the app from the proxy, so without it all visitors share one 6-runs-per-hour limit.
6. **Health check:**
   - The image has its own `HEALTHCHECK`: the binary runs `blastingroom -healthcheck`, which calls `/api/health`.
   - Leave Coolify's own health check off. It runs `curl` or `wget` inside the container, and the distroless image has neither, so deploys would be marked unhealthy.
7. **Deploy.**
   - The build log should end with the Go build.
   - The app log should show `qloo: live` and `rate limit keyed on a proxy header header=CF-Connecting-IP`.

## 2. Route the tunnel to it

1. In the Cloudflare dashboard, go to **Zero Trust → Networks → Tunnels**.
   - Use the existing tunnel if `cloudflared` already runs on the homelab.
   - Otherwise create one and install its connector on the homelab box (Docker or a system service).
2. **Add a public hostname:**
   - Subdomain `blastingroom` on your domain
   - Service type `HTTP`, URL pointing at Coolify's proxy on port 80:
     - `cloudflared` on the Coolify host itself: `localhost:80`
     - `cloudflared` in Docker on that host: put it on the `coolify` Docker network and use `coolify-proxy:80`, or use the host's LAN IP
     - `cloudflared` on another machine: the Coolify host's LAN IP, port 80

   The proxy routes by the `Host` header, which is the public hostname, so it must match the domain from step 1.4.

   Don't point the tunnel straight at the container. Its address changes on every redeploy.

## 3. Make the tunnel the only way in

The app trusts `CF-Connecting-IP` for rate limiting. Anyone who reaches Coolify's proxy directly could forge that header and get around the limit.

- Don't port-forward 80 or 443 from your router to the homelab.
- If the box has a public IP, firewall 80 and 443 to the LAN.

## 4. Check it

```bash
curl -s https://blastingroom.example.com/api/health
```

Expect `{"agent_ready":true,"ok":true,"qloo_mode":"live"}`.

```bash
curl -sN -X POST https://blastingroom.example.com/api/runs -H 'Content-Type: application/json' -d '{"venue_id":"lantern","mode":"fill_date"}'
```

Events should arrive one by one, not all at the end, with a `: keepalive` line every 15 seconds while the agent is quiet. That call starts a real, paid agent run. Then open the site and run one in the browser.

If the stream arrives all at once, something between Cloudflare and the app is buffering. The app sends `Cache-Control: no-cache` and `X-Accel-Buffering: no`, and Cloudflare doesn't cache POSTs, so check for a custom Coolify proxy rule.

## 5. Update

Push to `main`. With the GitHub App source, Coolify can redeploy on push (enable Auto Deploy); otherwise click **Redeploy**. Runs in flight during a redeploy are cut off.
