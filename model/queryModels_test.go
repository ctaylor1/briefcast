package model

import "testing"

func TestVerifyPaginationValuesAppliesDefaults(t *testing.T) {
	filter := EpisodesFilter{}
	filter.VerifyPaginationValues()

	if filter.Count != 20 {
		t.Fatalf("expected default count 20, got %d", filter.Count)
	}
	if filter.Page != 1 {
		t.Fatalf("expected default page 1, got %d", filter.Page)
	}
	if filter.Sorting != RELEASE_DESC {
		t.Fatalf("expected default sorting %q, got %q", RELEASE_DESC, filter.Sorting)
	}
}

func TestVerifyPaginationValuesPreservesExistingValues(t *testing.T) {
	filter := EpisodesFilter{
		Pagination: Pagination{
			Page:  3,
			Count: 50,
		},
		Sorting: DURATION_ASC,
	}
	filter.VerifyPaginationValues()

	if filter.Count != 50 {
		t.Fatalf("expected count to remain 50, got %d", filter.Count)
	}
	if filter.Page != 3 {
		t.Fatalf("expected page to remain 3, got %d", filter.Page)
	}
	if filter.Sorting != DURATION_ASC {
		t.Fatalf("expected sorting to remain %q, got %q", DURATION_ASC, filter.Sorting)
	}
}

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
