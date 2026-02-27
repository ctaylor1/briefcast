package model

import "fmt"

// PodcastAlreadyExistsError represents a public type.
type PodcastAlreadyExistsError struct {
	URL string
}

// Error handles the corresponding operation.
func (e *PodcastAlreadyExistsError) Error() string {
	return "Podcast with this url already exists"
}

// TagAlreadyExistsError represents a public type.
type TagAlreadyExistsError struct {
	Label string
}

// Error handles the corresponding operation.
func (e *TagAlreadyExistsError) Error() string {
	return fmt.Sprintf("Tag with this label already exists : %s", e.Label)
}
