package models

import "time"

type User struct {
	ID           string    `bson:"_id" json:"id"`
	Email        string    `bson:"email" json:"email"`
	PasswordHash string    `bson:"passwordHash" json:"-"`
	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
}

type Option struct {
	ID    string `bson:"id" json:"id"`
	Text  string `bson:"text" json:"text"`
	Emoji string `bson:"emoji" json:"emoji"`
}

type Poll struct {
	ID                 string           `bson:"_id" json:"id"`
	Slug               string           `bson:"slug" json:"slug"`
	OwnerID            string           `bson:"ownerId" json:"ownerId"`
	Question           string           `bson:"question" json:"question"`
	Options            []Option         `bson:"options" json:"options"`
	DurationSeconds    int64            `bson:"durationSeconds" json:"durationSeconds"`
	MaxVotes           int64            `bson:"maxVotes" json:"maxVotes"`
	Multiple           bool             `bson:"multiple" json:"multiple"`
	RestrictDuplicates bool             `bson:"restrictDuplicates" json:"restrictDuplicates"`
	Pulse              bool             `bson:"pulse" json:"pulse"`
	Status             string           `bson:"status" json:"status"`
	CreatedAt          time.Time        `bson:"createdAt" json:"createdAt"`
	ClosedAt           *time.Time       `bson:"closedAt,omitempty" json:"closedAt,omitempty"`
	AggregateVotes     int64            `bson:"aggregateVotes,omitempty" json:"-"`
	AggregateCounts    map[string]int64 `bson:"aggregateCounts,omitempty" json:"-"`
}

type ResultOption struct {
	ID      string  `json:"id"`
	Text    string  `json:"text"`
	Emoji   string  `json:"emoji"`
	Votes   int64   `json:"votes"`
	Percent float64 `json:"percent"`
}

type Snapshot struct {
	Slug       string         `json:"slug"`
	TotalVotes int64          `json:"totalVotes"`
	Options    []ResultOption `json:"options"`
	Status     string         `json:"status"`
}
