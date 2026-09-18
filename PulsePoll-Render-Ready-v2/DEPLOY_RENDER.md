# PulsePoll — Render deployment checklist

## Services

Recommended:

1. Render Web Service: `pulsepoll`
2. Render Key Value: `pulsepoll-redis`
3. MongoDB Atlas: durable MongoDB

The repository also contains `render.yaml` for the Web Service + Key Value setup.

## 1. GitHub

```bash
git init
git add .
git commit -m "Deploy PulsePoll"
git branch -M main
git remote add origin YOUR_REPO_URL
git push -u origin main
```

## 2. MongoDB Atlas

Create a cluster and database user.

Database:

```text
pulsepoll
```

Copy the application connection string into Render:

```text
MONGO_URI=mongodb+srv://...
MONGO_DB=pulsepoll
```

Configure Atlas Network Access so the Render Web Service can connect.

## 3. Render Key Value

Render Dashboard:

```text
New → Key Value
```

Name:

```text
pulsepoll-redis
```

Choose the same region as the web service.

Copy the **internal connection string** and use:

```text
REDIS_URL=redis://...
```

For production, use a persistent paid Key Value plan. Free Key Value is in-memory.

## 4. Render Web Service

```text
New → Web Service
```

Select the GitHub repository.

Use:

```text
Runtime: Docker
Dockerfile: ./Dockerfile
Docker Context: .
Health Check Path: /health
```

The root Dockerfile builds both:

- React frontend
- Go backend

The Go server then serves the React build and `/api` + `/ws` from one origin.

## 5. Environment

Add:

```text
PORT=10000
MONGO_URI=<atlas-uri>
MONGO_DB=pulsepoll
REDIS_URL=<render-key-value-internal-url>
JWT_SECRET=<long-random-secret>
CORS_ORIGIN=same-origin
```

Do not commit real credentials.

## 6. Deploy

```text
Manual Deploy → Deploy latest commit
```

After deployment:

```text
https://YOUR-SERVICE.onrender.com/health
```

Expected:

```json
{"status":"ok","service":"pulsepoll"}
```

## 7. Test WebSockets

Open:

```text
https://YOUR-SERVICE.onrender.com
```

Create a poll.

Open the result page in two browser windows.

In DevTools → Network → WS, verify:

```text
wss://YOUR-SERVICE.onrender.com/ws?poll=<slug>
```

Vote from another browser/device.

The result page should update without refresh.

## 8. Test closure

Test all three:

### Manual

Creator Dashboard → Close.

### Time

Create a poll with a short duration.

### Maximum votes

Create a poll with a maximum vote count and reach the limit.

The result page should change from:

```text
LIVE RESULTS
```

to:

```text
FINAL RESULTS
```

The winning option is highlighted.

For a tie, every option with the highest vote count is displayed in the tie winner section.

## 9. If WebSockets fail

Use `wss://`, not `ws://`, on the Render HTTPS URL.

Check:

- Web Service logs
- Key Value connection
- Browser Network → WS
- `/health`
- correct poll slug

## 10. Free demo warning

Render currently provides free Web Services and free Key Value, but free Key Value is in-memory and can lose data on restart. Render also describes free resources as suitable for testing/hobby use rather than production workloads.

MongoDB Atlas can be used as the durable database. For a real production deployment, use persistent Render Key Value and an appropriate MongoDB tier/backups.
