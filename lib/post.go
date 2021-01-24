package lib

import (
	"time"
)

type Post struct {
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	ImageURL  string    `json:"imageurl"`
	CreatedAt time.Time `json:"created_at"`
}
