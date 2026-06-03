package search

type TrackDocument struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	ArtistName string `json:"artist_name"`
	PlayCount  int64  `json:"play_count"`
	Status     string `json:"status"`
}

type TrackResult struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	ArtistName string  `json:"artist_name"`
	Score      float64 `json:"score"`
}
