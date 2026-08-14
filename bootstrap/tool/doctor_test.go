package main

import "testing"

func TestDockerAccessResult(t *testing.T) {
	cases := []struct {
		name              string
		account, current  bool
		server            string
		wantStatus        Status
		wantDetailSnippet string
	}{
		{"not registered", false, false, "", StatusWarn, "not in the docker group"},
		{"new login needed", true, false, "", StatusWarn, "not active"},
		{"daemon unavailable", true, true, "", StatusWarn, "not reachable"},
		{"healthy", true, true, "29.7.2", StatusOK, "29.7.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStatus, gotDetail := dockerAccessResult("tester", c.account, c.current, c.server)
			if gotStatus != c.wantStatus {
				t.Errorf("status = %s, want %s", gotStatus, c.wantStatus)
			}
			if !contains(gotDetail, c.wantDetailSnippet) {
				t.Errorf("detail = %q, want it to contain %q", gotDetail, c.wantDetailSnippet)
			}
		})
	}
}

func TestWordPresent(t *testing.T) {
	if !wordPresent("tester sudo docker users", "docker") {
		t.Error("docker group was not found")
	}
	if wordPresent("tester docker-build", "docker") {
		t.Error("matched a group name by substring")
	}
}
