package sftp

import "testing"

func TestDownloadCoverageIsBoundedAndDoesNotCountRepeatedReads(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spans    []readSpan
		size     int64
		complete bool
	}{
		{"empty", nil, 0, true},
		{"partial", []readSpan{{0, 5}}, 10, false},
		{"repeat", []readSpan{{0, 5}, {0, 5}}, 10, false},
		{"out of order", []readSpan{{5, 10}, {0, 5}}, 10, true},
		{"overlapping", []readSpan{{0, 6}, {4, 10}}, 10, true},
		{"gap", []readSpan{{0, 4}, {5, 10}}, 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var coverage readCoverage
			for _, span := range tc.spans {
				coverage.add(span.start, span.end)
			}

			if coverage.complete(tc.size) != tc.complete {
				t.Fatal(coverage)
			}
		})
	}

	var sparse readCoverage
	for i := range int64(1000) {
		sparse.add(i*2, i*2+1)
	}

	sparse.add(0, 2000)

	if !sparse.uncertain || sparse.complete(2000) || len(sparse.spans) > 64 {
		t.Fatal("unbounded or falsely complete sparse coverage", sparse)
	}
}

func TestHandshakeFailureLoggingIsRateLimited(t *testing.T) {
	s := &Server{}
	s.authenticationFailure()

	at := s.securityLogged
	for range 100 {
		s.authenticationFailure()
	}

	if at.IsZero() || !s.securityLogged.Equal(at) || s.securityFailures != 100 {
		t.Fatal("authentication failures were not aggregated")
	}
}
