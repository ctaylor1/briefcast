package service

import "testing"

// The transcription_completed log used to report every job as resumed: it tested
// the job status's checkpoint progress, which every job that writes a
// checkpoint sets. It now reports the worker's own record in the result.
func TestTranscriptResumedFromCheckpointReadsTheWorkerRecord(t *testing.T) {
	for name, tc := range map[string]struct {
		result string
		want   bool
	}{
		"fresh job":   {`{"metadata":{"resumed_from_checkpoint":false,"checkpoint_segment_count":0}}`, false},
		"resumed job": {`{"metadata":{"resumed_from_checkpoint":true,"checkpoint_segment_count":12}}`, true},
		"no metadata": {`{"segments":[]}`, false},
		"unreadable":  {`not json`, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := transcriptResumedFromCheckpoint([]byte(tc.result)); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
