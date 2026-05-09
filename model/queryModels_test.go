package model

import "testing"

func TestParseEpisodeSortAcceptsAllCanonicalValues(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  EpisodeSort
	}{
		{"release_asc", ReleaseAsc},
		{"release_desc", ReleaseDesc},
		{"duration_asc", DurationAsc},
		{"duration_desc", DurationDesc},
	} {
		if got := ParseEpisodeSort(tc.input); got != tc.want {
			t.Fatalf("ParseEpisodeSort(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseEpisodeSortAcceptsLegacyPascalCase(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  EpisodeSort
	}{
		{"ReleaseAsc", ReleaseAsc},
		{"ReleaseDesc", ReleaseDesc},
		{"DurationAsc", DurationAsc},
		{"DurationDesc", DurationDesc},
	} {
		if got := ParseEpisodeSort(tc.input); got != tc.want {
			t.Fatalf("ParseEpisodeSort(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseEpisodeSortReturnsEmptyForUnknown(t *testing.T) {
	for _, input := range []string{"", "invalid", "RELEASE_DESC", "releaseDesc"} {
		if got := ParseEpisodeSort(input); got != "" {
			t.Fatalf("ParseEpisodeSort(%q) = %q, want empty", input, got)
		}
	}
}

func TestVerifyPaginationValuesNormalisesLegacySorting(t *testing.T) {
	filter := EpisodesFilter{Sorting: "ReleaseAsc"}
	filter.VerifyPaginationValues()
	if filter.Sorting != ReleaseAsc {
		t.Fatalf("expected legacy PascalCase to be normalised to %q, got %q", ReleaseAsc, filter.Sorting)
	}
}

func TestVerifyPaginationValuesAppliesDefaults(t *testing.T) {
	filter := EpisodesFilter{}
	filter.VerifyPaginationValues()

	if filter.Count != 20 {
		t.Fatalf("expected default count 20, got %d", filter.Count)
	}
	if filter.Page != 1 {
		t.Fatalf("expected default page 1, got %d", filter.Page)
	}
	if filter.Sorting != ReleaseDesc {
		t.Fatalf("expected default sorting %q, got %q", ReleaseDesc, filter.Sorting)
	}
}

// TestVerifyPaginationValuesPreservesExistingValues handles the corresponding operation.
func TestVerifyPaginationValuesPreservesExistingValues(t *testing.T) {
	filter := EpisodesFilter{
		Pagination: Pagination{
			Page:  3,
			Count: 50,
		},
		Sorting: DurationAsc,
	}
	filter.VerifyPaginationValues()

	if filter.Count != 50 {
		t.Fatalf("expected count to remain 50, got %d", filter.Count)
	}
	if filter.Page != 3 {
		t.Fatalf("expected page to remain 3, got %d", filter.Page)
	}
	if filter.Sorting != DurationAsc {
		t.Fatalf("expected sorting to remain %q, got %q", DurationAsc, filter.Sorting)
	}
}

// TestSetCountsFirstPage handles the corresponding operation.
func TestSetCountsFirstPage(t *testing.T) {
	filter := EpisodesFilter{
		Pagination: Pagination{
			Page:  1,
			Count: 20,
		},
	}

	filter.SetCounts(45)

	if filter.TotalPages != 3 {
		t.Fatalf("expected total pages 3, got %d", filter.TotalPages)
	}
	if filter.NextPage != 2 {
		t.Fatalf("expected next page 2, got %d", filter.NextPage)
	}
	if filter.PreviousPage != 0 {
		t.Fatalf("expected previous page 0, got %d", filter.PreviousPage)
	}
	if filter.TotalCount != 45 {
		t.Fatalf("expected total count 45, got %d", filter.TotalCount)
	}
}

// TestSetCountsLastPage handles the corresponding operation.
func TestSetCountsLastPage(t *testing.T) {
	filter := EpisodesFilter{
		Pagination: Pagination{
			Page:  3,
			Count: 20,
		},
	}

	filter.SetCounts(41)

	if filter.TotalPages != 3 {
		t.Fatalf("expected total pages 3, got %d", filter.TotalPages)
	}
	if filter.NextPage != 0 {
		t.Fatalf("expected next page 0, got %d", filter.NextPage)
	}
	if filter.PreviousPage != 2 {
		t.Fatalf("expected previous page 2, got %d", filter.PreviousPage)
	}
}
