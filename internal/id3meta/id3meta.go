package id3meta

import "encoding/json"

// Parsed represents a public type.
type Parsed struct {
	Tags     map[string][]string      `json:"tags"`
	Chapters []map[string]interface{} `json:"chapters"`
}

// ShouldExtract handles the corresponding operation.
func ShouldExtract(chaptersJSON, id3TagsJSON, id3ChaptersJSON string) bool {
	return chaptersJSON == "" && id3TagsJSON == "" && id3ChaptersJSON == ""
}

// SplitRaw handles the corresponding operation.
func SplitRaw(raw []byte) (tagsJSON string, chaptersJSON string, hasTags bool, hasChapters bool, err error) {
	var parsed Parsed
	if len(raw) == 0 {
		return "", "", false, false, nil
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return "", "", false, false, err
	}

	if len(parsed.Tags) > 0 {
		if data, marshalErr := json.Marshal(parsed.Tags); marshalErr == nil {
			tagsJSON = string(data)
			hasTags = true
		}
	}
	if len(parsed.Chapters) > 0 {
		if data, marshalErr := json.Marshal(parsed.Chapters); marshalErr == nil {
			chaptersJSON = string(data)
			hasChapters = true
		}
	}
	return tagsJSON, chaptersJSON, hasTags, hasChapters, nil
}
