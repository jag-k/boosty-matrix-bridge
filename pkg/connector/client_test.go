package connector

import (
	"strconv"
	"sync"
	"testing"
)

func TestConcurrentMessageCursorsDoNotRegress(t *testing.T) {
	b := &BoostyClient{lastSeenMsg: map[string]string{}, lastReadMsg: map[string]string{}}
	var wg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		wg.Go(func() {
			id := strconv.Itoa(i)
			b.setLastSeen("1", id)
			b.markReadMessage("1", id)
			b.getLastSeen("1")
		})
	}
	wg.Wait()
	b.setLastSeen("1", "2")
	if got, _ := b.getLastSeen("1"); got != "100" {
		t.Fatalf("last seen regressed: %s", got)
	}
	if b.markReadMessage("1", "2") {
		t.Fatal("read receipt cursor regressed")
	}
}
