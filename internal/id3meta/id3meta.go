package id3meta

import "encoding/json"

type Parsed struct {
	Tags     map[string][]string      `json:"tags"`
	Chapters []map[string]interface{} `json:"chapters"`
}

func ShouldExtract(chaptersJSON, id3TagsJSON, id3ChaptersJSON string) bool {
	return chaptersJSON == "" && id3TagsJSON == "" && id3ChaptersJSON == ""
}

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
