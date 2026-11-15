package settings

import "testing"

func TestSparkEnrollmentTokenEnvironment(t *testing.T) {
	t.Setenv("SPARK_ENROLLMENT_TOKEN", "one-time-token")
	if got := SparkEnrollmentToken.Value(""); got != "one-time-token" {
		t.Fatalf("Spark enrollment token = %q, want one-time-token", got)
	}
}
