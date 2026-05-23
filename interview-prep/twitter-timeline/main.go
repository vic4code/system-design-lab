// Fan-out-on-write timeline simulation.
// Demonstrates the trade-off between fan-out-on-write vs fan-out-on-read.
// Run: go run main.go
package main

import (
	"fmt"
	"sync"
	"time"
)

// Tweet is an immutable post.
type Tweet struct {
	ID        int64
	AuthorID  int64
	Text      string
	CreatedAt time.Time
}

// InMemoryStore simulates Redis sorted sets for timelines.
type InMemoryStore struct {
	mu        sync.RWMutex
	tweets    map[int64]Tweet
	followers map[int64][]int64 // userID → follower IDs
	timelines map[int64][]int64 // userID → tweet IDs (newest first)
	nextID    int64
}

func NewStore() *InMemoryStore {
	return &InMemoryStore{
		tweets:    make(map[int64]Tweet),
		followers: make(map[int64][]int64),
		timelines: make(map[int64][]int64),
	}
}

func (s *InMemoryStore) Follow(followerID, followeeID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followers[followeeID] = append(s.followers[followeeID], followerID)
}

// PostTweet uses fan-out-on-write: the tweet is pushed to every follower's timeline immediately.
// Pro: timeline reads are O(1). Con: celebrities with millions of followers cause a write storm.
func (s *InMemoryStore) PostTweet(authorID int64, text string) Tweet {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	t := Tweet{ID: s.nextID, AuthorID: authorID, Text: text, CreatedAt: time.Now()}
	s.tweets[t.ID] = t

	// Fan-out: push tweet ID to every follower's timeline
	for _, followerID := range s.followers[authorID] {
		s.timelines[followerID] = prepend(s.timelines[followerID], t.ID, 200)
	}
	return t
}

// GetTimeline returns the latest N tweets for a user.
func (s *InMemoryStore) GetTimeline(userID int64, limit int) []Tweet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.timelines[userID]
	if len(ids) > limit {
		ids = ids[:limit]
	}
	result := make([]Tweet, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.tweets[id]; ok {
			result = append(result, t)
		}
	}
	return result
}

// prepend adds an ID to the front of a slice, capped at maxLen.
func prepend(ids []int64, id int64, maxLen int) []int64 {
	ids = append([]int64{id}, ids...)
	if len(ids) > maxLen {
		ids = ids[:maxLen]
	}
	return ids
}

func main() {
	store := NewStore()

	// Setup: user 1 (celebrity) has 3 followers
	store.Follow(2, 1)
	store.Follow(3, 1)
	store.Follow(4, 1)

	// User 2 also follows user 3
	store.Follow(2, 3)

	fmt.Println("=== Fan-out-on-write timeline demo ===\n")

	t1 := store.PostTweet(1, "Hello from user 1!")
	t2 := store.PostTweet(3, "Hello from user 3!")
	store.PostTweet(1, "Second tweet from user 1")

	fmt.Printf("Tweet %d posted by user %d: %q\n", t1.ID, t1.AuthorID, t1.Text)
	fmt.Printf("Tweet %d posted by user %d: %q\n", t2.ID, t2.AuthorID, t2.Text)
	fmt.Println()

	// Show timelines
	for _, userID := range []int64{2, 3, 4} {
		timeline := store.GetTimeline(userID, 10)
		fmt.Printf("User %d timeline (%d tweets):\n", userID, len(timeline))
		for _, t := range timeline {
			fmt.Printf("  [%d] @user%d: %s\n", t.ID, t.AuthorID, t.Text)
		}
		fmt.Println()
	}

	fmt.Println("Key trade-off:")
	fmt.Println("  Fan-out-on-WRITE: fast reads, slow writes for celebrities")
	fmt.Println("  Fan-out-on-READ:  slow reads (merge N feeds), fast writes")
	fmt.Println("  Twitter hybrid:   fan-out-on-write for normal users,")
	fmt.Println("                    fan-out-on-read for celebrities (>~10K followers)")
}
