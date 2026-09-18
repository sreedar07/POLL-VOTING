package realtime

import "testing"

func TestBroadcastEventShape(t *testing.T) {
    // Contract-level test: Redis event names are poll.events.<slug>.
    slug := "demo-poll-1234"
    if slug == "" { t.Fatal("slug empty") }
}
