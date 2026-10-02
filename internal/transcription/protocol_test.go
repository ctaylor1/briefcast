package transcription

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../docs/whisperx-separation/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFrozenRequestAndResultDigests(t *testing.T) {
	request, canonical, err := DecodeRequest(fixture(t, "put-job.json"))
	if err != nil {
		t.Fatal(err)
	}
	if Digest(canonical) != "1e2a33ce2babc8b68df214870738c2c68b1cc17f67bf411bc5ca0704c1b8ba23" {
		t.Fatal("request digest changed")
	}
	options, err := MarshalCanonical(request.Options)
	if err != nil {
		t.Fatal(err)
	}
	if Digest(options) != "60baeabe17a6baa942c7b52f59aaf6ea87ed413aba156038ed6fdc68a8b2c83e" {
		t.Fatal("options digest changed")
	}
	var result JobResult
	if err := json.Unmarshal(fixture(t, "job-result.json"), &result); err != nil {
		t.Fatal(err)
	}
	transcript, err := CanonicalJSON(result.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if Digest(transcript) != result.ResultSHA256 {
		t.Fatal("result digest changed")
	}
	if err := ValidateTranscript(transcript); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalJSON(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`{"z":-0,"a":1.0}`, `{"a":1,"z":0}`},
		{`[1e-6,1e-7,1e20,1e21,333333333.33333329]`, `[0.000001,1e-7,100000000000000000000,1e+21,333333333.3333333]`},
		{`{"\ufffd":1,"\ud83d\ude00":2,"\u20ac":3}`, `{"€":3,"😀":2,"�":1}`},
		{`"<>&\u2028\u000f\/"`, "\"<>&\u2028\\u000f/\""},
	} {
		got, err := CanonicalJSON([]byte(tc.input))
		if err != nil || string(got) != tc.want {
			t.Errorf("%s => %s %v; want %s", tc.input, got, err, tc.want)
		}
	}
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":{"x":1,"x":2}}`, `{} {}`, `1e999`, `"\ud800"`, `"\udc00"`, strings.Repeat("[", 70) + strings.Repeat("]", 70)} {
		if _, err := CanonicalJSON([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestRejectInvalidRequests(t *testing.T) {
	original := string(fixture(t, "put-job.json"))
	for _, raw := range []string{
		strings.Replace(original, `"align": true,`, "", 1),
		strings.Replace(original, `"align": true`, `"align": null`, 1),
		strings.Replace(original, `"align": true`, `"align": true,"token":"secret"`, 1),
		strings.Replace(original, `"min_speakers": 2`, `"min_speakers": 33`, 1),
		strings.Replace(original, `"contract_version": 1`, `"contract_version": 2`, 1),
		strings.Replace(original, `"relative_path": "audio/example/episode.mp3"`, `"relative_path": "../outside"`, 1),
		strings.Replace(original, `"attempt": 1`, `"attempt": 1.5`, 1),
		strings.Replace(original, `"size_bytes": 123456`, `"size_bytes": 9007199254740992`, 1),
	} {
		if _, _, err := DecodeRequest([]byte(raw)); err == nil {
			t.Errorf("accepted invalid request: %s", raw)
		}
	}
}

func TestTranscriptRejectsMalformedAndPreservesMissingTiming(t *testing.T) {
	var result JobResult
	if err := json.Unmarshal(fixture(t, "job-result.json"), &result); err != nil {
		t.Fatal(err)
	}
	original := string(result.Transcript)
	for _, raw := range []string{`{"error":"failed"}`, strings.Replace(original, `"start": 0,`, `"start": -1,`, 1), strings.Replace(original, `"end": 1.2`, `"end": "bad"`, 1), strings.Replace(original, `"text": "Hello world."`, `"text": null`, 1), strings.Replace(original, `"provider": "whisperx"`, `"provider": "other"`, 1)} {
		if err := ValidateTranscript([]byte(raw)); err == nil {
			t.Errorf("accepted malformed transcript")
		}
	}
	var body map[string]any
	if err := json.Unmarshal(result.Transcript, &body); err != nil {
		t.Fatal(err)
	}
	body["segments"] = []any{map[string]any{"text": "untimed", "words": []any{map[string]any{"word": "word"}}}}
	b, err := MarshalCanonical(body)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateTranscript(b); err != nil {
		t.Fatal(err)
	}
}
