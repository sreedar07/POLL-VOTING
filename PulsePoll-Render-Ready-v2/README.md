# PulsePoll — Production-ready live polling

PulsePoll is a full-stack live polling application built around **React + TypeScript + Vite**, **Go + Gin**, **MongoDB**, **Render Key Value (Redis-compatible Valkey)** and **WebSockets**.

It supports:

- Creator signup/login with JWT
- Guest voting — voters do not need accounts
- Unique poll URL/slugs
- QR code generated from the exact invitation URL
- Social sharing links
- Single-choice and multiple-choice polls
- Duplicate-vote protection using an anonymous cookie + IP-derived fingerprint
- Optional time limit (30 seconds–7 days)
- Optional maximum vote count
- Automatic transition to final results on expiration or max-vote closure
- Manual creator closure
- Live vote counts and percentages
- WebSocket push to every connected results viewer
- Winner/tie detection
- Exact final vote distribution
- CSV/JSON export
- Creator dashboard
- Docker deployment
- Single-origin production deployment: Go serves the compiled React app and API

## 1. Architecture

```text
                         ┌─────────────────────────────┐
                         │       Browser / PWA          │
                         │ React voting + results UI    │
                         └──────────────┬──────────────┘
                                        │ HTTPS
                         ┌──────────────▼──────────────┐
                         │       Render Web Service     │
                         │                              │
                         │ Go + Gin                     │
                         │ ├─ REST /api                 │
                         │ ├─ WebSocket /ws             │
                         │ └─ React static files        │
                         │                              │
                         │ Redis-compatible realtime    │
                         │ ├─ atomic counters/cache     │
                         │ ├─ duplicate protection      │
                         │ └─ Pub/Sub                   │
                         └─────────┬───────────┬────────┘
                                   │           │
                         private network       │ TLS
                                   │           │
                    ┌──────────────▼───┐   ┌──▼──────────────┐
                    │ Render Key Value │   │ MongoDB Atlas    │
                    │ Valkey/Redis     │   │ durable storage  │
                    └──────────────────┘   └─────────────────┘
```

MongoDB stores durable poll metadata and aggregate vote counts. Redis/Valkey is used for fast realtime state, duplicate protection, and Pub/Sub. If Redis/Valkey is restarted, the application can rebuild the live counter hash from MongoDB.

## 2. Project structure

```text
pulsepoll/
├── Dockerfile
├── render.yaml
├── .env.example
├── .gitignore
├── README.md
├── docker-compose.yml
├── docker-compose.prod.yml
├── deploy/
│   └── nginx.conf
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   ├── public/
│   └── src/
│       ├── App.tsx
│       ├── api.ts
│       ├── main.tsx
│       └── styles.css
└── backend/
    ├── go.mod
    ├── Dockerfile
    ├── cmd/server/main.go
    └── internal/
        ├── auth/
        ├── handlers/
        ├── models/
        ├── realtime/
        └── store/
```

## 3. Main user flow

### Creator

1. Sign up/login.
2. Create a poll.
3. Backend generates a unique slug such as:
   `what-should-we-build-a8f2`
4. Share page creates:
   - Copyable URL
   - QR code containing the same URL
   - Social sharing actions
5. Creator can open live results or presenter mode.

### Guest voter

1. Opens the URL or scans the QR code.
2. Sees all options.
3. Selects an answer.
4. Clicks **Cast vote**.
5. The app immediately redirects to `/poll/:slug/results`.
6. The result page receives WebSocket updates automatically.

### Poll closing

A poll closes when any of these occurs:

- Creator manually closes it.
- Duration expires.
- Maximum vote count is reached.

The server changes the poll state to `closed` and broadcasts a `poll.closed` WebSocket event. All connected viewers therefore switch to the final results without refreshing.

### Winner/tie logic

- Highest vote count is the winner.
- The displayed percentage is based on total votes.
- If two or more options have the same highest vote count, the UI displays **TIE** and lists all tied options.
- If there are zero votes, the final page shows **No votes were recorded**.

## 4. Local Docker run

Requirements:

- Docker Desktop
- Docker Compose

Run:

```bash
docker compose up --build
```

Open:

```text
http://localhost:5173
```

API:

```text
http://localhost:8080/health
```

For production-like single-origin Docker:

```bash
docker build -t pulsepoll .
docker run --rm -p 10000:10000 \
  -e MONGO_URI=mongodb://host.docker.internal:27017 \
  -e MONGO_DB=pulsepoll \
  -e REDIS_ADDR=host.docker.internal:6379 \
  -e JWT_SECRET=replace-me \
  -e CORS_ORIGIN=same-origin \
  pulsepoll
```

## 5. Environment variables

```text
PORT=10000

# Durable database
MONGO_URI=mongodb+srv://USERNAME:PASSWORD@CLUSTER.mongodb.net/?retryWrites=true&w=majority
MONGO_DB=pulsepoll

# Prefer REDIS_URL on Render
REDIS_URL=redis://...
# Local alternative:
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=

JWT_SECRET=long-random-secret
CORS_ORIGIN=same-origin
```

Do not commit a real `.env` file.

## 6. Render deployment — recommended production layout

Render currently supports Go web services, inbound WebSockets, managed Key Value instances, and custom/private database services. The simplest deployment for this repository is:

- **1 Render Web Service** — runs the Go API and serves React
- **1 Render Key Value** — Redis-compatible Valkey for realtime state
- **1 MongoDB Atlas cluster** — durable MongoDB

Render's Web Service must listen on `0.0.0.0` and should use the `PORT` environment variable. Render also supports inbound WebSockets; public clients should connect with `wss://`, not `ws://`. citeturn0search0turn0search12

### Step A — Push to GitHub

Create a repository and push the project:

```bash
git init
git add .
git commit -m "Initial PulsePoll production build"
git branch -M main
git remote add origin YOUR_GITHUB_REPOSITORY
git push -u origin main
```

### Step B — Create MongoDB Atlas

Render documents an official MongoDB Atlas connection flow. Create an Atlas cluster, create a database user, and allow your Render service's outbound IP addresses in Atlas Network Access. Use the MongoDB driver's application connection string as `MONGO_URI`. citeturn0search1

Create database:

```text
Database: pulsepoll
```

Keep the MongoDB username/password out of GitHub.

For a college/demo deployment, an Atlas free/shared tier may be sufficient. For a real production system, use a durable paid tier and backups appropriate to your requirements.

### Step C — Create Render Key Value

In Render:

1. Dashboard → **New**
2. Select **Key Value**
3. Name it `pulsepoll-redis`
4. Put it in the same Render region as the Web Service.
5. For production, use a paid persistent plan.

New Render Key Value instances use Valkey, which is Redis-compatible for normal Redis clients. Free Key Value is in-memory and can lose its data after a restart, so it should be treated as a development/demo service rather than durable production storage. citeturn1search1turn1search5turn1search0

### Step D — Create the Web Service

1. Render Dashboard → **New → Web Service**
2. Connect your GitHub account.
3. Select the PulsePoll repository.
4. Branch: `main`
5. Runtime: **Docker**
6. Dockerfile: `./Dockerfile`
7. Docker context: repository root.
8. Health check path:

```text
/health
```

9. Deploy.

Render supports Git-connected deployments and Docker-based web services. citeturn0search0turn0search4

### Step E — Environment variables

In the Web Service → **Environment**, add:

```text
PORT=10000
MONGO_URI=<your Atlas connection string>
MONGO_DB=pulsepoll
REDIS_URL=<Render Key Value internal connection string>
JWT_SECRET=<long random secret>
CORS_ORIGIN=same-origin
```

Render recommends using environment variables for secrets rather than committing them to source control. citeturn0search9

If using a Render Blueprint, the included `render.yaml` creates the Web Service and Key Value service. You still supply `MONGO_URI`.

### Step F — Deploy

Click **Manual Deploy → Deploy latest commit**.

The Docker build performs:

```text
Node build
    ↓
React/Vite production bundle
    ↓
Go build
    ↓
minimal Alpine image
    ↓
Go serves API + React
```

The application will receive an address similar to:

```text
https://pulsepoll-xxxx.onrender.com
```

## 7. Verify the deployment

### Health

Open:

```text
https://YOUR-APP.onrender.com/health
```

Expected:

```json
{
  "service": "pulsepoll",
  "status": "ok"
}
```

### WebSocket

Open browser DevTools → Network → WS.

When visiting:

```text
/poll/YOUR_SLUG/results
```

the browser should establish:

```text
wss://YOUR-APP.onrender.com/ws?poll=YOUR_SLUG
```

The initial event is:

```json
{
  "type": "poll.snapshot"
}
```

After a vote:

```json
{
  "type": "poll.vote.updated"
}
```

When a poll closes:

```json
{
  "type": "poll.closed"
}
```

Render explicitly supports inbound WebSockets on web services. Use `wss://` for public HTTPS deployments; using `ws://` can fail because Render redirects HTTP traffic to HTTPS before the WebSocket handshake. citeturn0search12

## 8. Test realtime voting

Open two browser windows:

```text
Window A:
https://YOUR-APP.onrender.com/poll/SLUG/results

Window B:
https://YOUR-APP.onrender.com/poll/SLUG
```

Vote in Window B.

Window A should update without refreshing.

For a stronger test:

1. Open the result URL on two devices.
2. Vote from device 1.
3. Confirm both result pages update.
4. Reach the maximum vote count.
5. Confirm both pages switch from LIVE to FINAL.

## 9. Important production settings

### Use a strong JWT secret

Generate one locally, for example:

```bash
openssl rand -base64 48
```

Paste the result into Render as `JWT_SECRET`.

### Use persistent Key Value for production

The application can recover vote counters from MongoDB, but Redis/Valkey also holds duplicate-voter keys and realtime state. A persistent paid Key Value service is preferable for a production deployment.

### Keep MongoDB private

Use Atlas Network Access correctly and a database user with only the permissions required by the application.

### HTTPS

Use the Render-provided HTTPS URL or a custom domain with TLS. The WebSocket client automatically switches to:

```text
wss://
```

when the page is HTTPS.

## 10. Render troubleshooting

### Build fails at npm install

Check:

```text
frontend/package.json
```

The root Dockerfile uses `npm install`, so a package-lock file is not required.

### Go build fails

Check Render logs for the exact Go module error.

The backend uses Go modules and Render supports Go deployments natively. The Dockerfile also performs `go mod download` before compiling. citeturn0search3

### `PORT` error

The application reads:

```text
PORT
```

and listens on:

```text
0.0.0.0:$PORT
```

Render requires public web services to bind to `0.0.0.0`. citeturn0search0

### Mongo connection timeout

Check:

1. Atlas Network Access.
2. Atlas database user.
3. Password URL encoding.
4. `MONGO_URI`.
5. `MONGO_DB`.
6. Atlas cluster is running.

Render's MongoDB Atlas guide specifically requires configuring Atlas Network Access for Render's outbound IP addresses. citeturn0search1

### Redis/Key Value connection fails

Check:

```text
REDIS_URL
```

Use the **internal** Key Value connection string when the Web Service and Key Value are in the same Render region.

### WebSocket says 301/handshake failed

Do not hard-code:

```text
ws://
```

for production.

The frontend derives the protocol from the current page and uses:

```text
wss://
```

when served through HTTPS. Render documents this requirement for public WebSockets. citeturn0search12

### Results do not update

Check:

1. Browser DevTools → Network → WS.
2. Confirm `/ws?poll=...` is connected.
3. Check Render logs.
4. Confirm `REDIS_URL` works.
5. Open `/health`.
6. Confirm both clients use the same poll slug.

### Poll closes but old results remain

The frontend listens for `poll.closed` and `poll.vote.updated`. Hard refresh once to verify the server's persisted status. If it still shows open, inspect MongoDB's poll document and Render logs.

### Free Render service sleeps

Free Render web services have limitations and are intended for testing/hobby use rather than production workloads. Free Key Value is also non-persistent. For a real deployment, use appropriate paid services and persistent data storage. citeturn1search0

## 11. API reference

```text
POST /api/auth/signup
POST /api/auth/login

POST /api/polls                    creator
GET  /api/polls/:slug              public

POST /api/polls/:slug/vote         guest voter
POST /api/polls/:slug/reaction     guest

GET  /api/admin/polls              creator
GET  /api/admin/polls/:slug/export?format=json|csv
POST /api/admin/polls/:slug/close
DELETE /api/admin/polls/:slug

GET  /health
GET  /ws?poll=:slug
```

## 12. WebSocket event model

### Initial connection

```json
{
  "type": "poll.snapshot",
  "poll": {
    "slug": "demo-1234",
    "totalVotes": 5,
    "status": "open",
    "options": []
  }
}
```

### Vote

```json
{
  "type": "poll.vote.updated",
  "slug": "demo-1234",
  "poll": {
    "totalVotes": 6,
    "status": "open",
    "options": []
  }
}
```

### Closed

```json
{
  "type": "poll.closed",
  "slug": "demo-1234",
  "poll": {
    "totalVotes": 100,
    "status": "closed",
    "options": []
  }
}
```

## 13. Security notes

Implemented:

- Password hashing with bcrypt
- JWT authentication for creators
- Guest voting without collecting voter accounts
- HTTP rate limiting
- Duplicate protection
- Server-side option validation
- Maximum vote limit enforced atomically in Redis
- Poll ownership checks for management/export
- HTTPS/WSS deployment support

For a high-stakes public production deployment, additionally consider:

- CAPTCHA/bot mitigation
- stronger IP/device abuse controls
- audit logging
- secret rotation
- database backups
- monitoring/alerting
- structured logs
- stricter WebSocket origin validation
- CSRF strategy if authentication moves to cookies
- privacy/legal review for the jurisdiction and use case

## 14. Important data-model decision

Vote counts are maintained in MongoDB as durable aggregate counters and mirrored into Redis/Valkey for fast realtime access.

This means Redis is **not the only durable copy of the results**.

MongoDB does not store voter identities. The anonymous duplicate-protection fingerprint is kept in Redis with an expiry rather than as a permanent voter profile.

## 15. Deployment recommendation

For a college/demo deployment:

```text
Render Free Web Service
        +
Render Free Key Value
        +
MongoDB Atlas free/shared
```

For a real production deployment:

```text
Render paid Web Service
        +
Render persistent Key Value
        +
MongoDB Atlas paid/durable tier
        +
backups + monitoring
```

Render currently offers free Web Services and Key Value instances, but documents important free-tier limitations; free Key Value is in-memory and can lose its data after restart, while free resources are intended for testing/hobby use. citeturn1search0
