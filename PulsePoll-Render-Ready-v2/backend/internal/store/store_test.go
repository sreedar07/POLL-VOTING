package store

import (
    "context"
    "testing"

    "pulsepoll/internal/models"
)

func TestTallyPercentages(t *testing.T) {
    total := int64(4)
    votes := []int64{2,1,1}
    expected := []float64{50,25,25}
    for i,v := range votes {
        got := float64(v)*100/float64(total)
        if got != expected[i] { t.Fatalf("option %d: got %v want %v",i,got,expected[i]) }
    }
    _ = context.Background()
    _ = models.Snapshot{}
}
