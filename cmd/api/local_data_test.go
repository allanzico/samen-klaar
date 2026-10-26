package main

import "testing"

func TestDemoSeedRequiresAnExplicitTarget(t *testing.T) {
	for _, test := range []struct {
		name, origin, mode, database, enabled, expected, action string
		allowed                                                 bool
	}{
		{"local demo", "http://127.0.0.1:8090", "development", "samenklaar_demo", "", "", "seed-demo", true},
		{"hosted opt-in", "https://demo.example.test", "production", "samenklaar_demo", "1", "https://demo.example.test", "seed-demo", true},
		{"hosted import", "https://demo.example.test", "production", "samenklaar_demo", "1", "https://demo.example.test", "import-seed-demo-auth0", true},
		{"existing database with opt-in", "https://demo.example.test", "production", "samenklaar", "1", "https://demo.example.test", "seed-demo", true},
		{"opt-in absent", "https://demo.example.test", "production", "samenklaar_demo", "", "https://demo.example.test", "seed-demo", false},
		{"origin differs", "https://other.example.test", "production", "samenklaar_demo", "1", "https://demo.example.test", "seed-demo", false},
		{"production presentation opt-in", "https://samenklaar.com", "production", "samenklaar", "1", "https://samenklaar.com", "seed-demo", true},
		{"production without opt-in", "https://samenklaar.com", "production", "samenklaar", "", "https://samenklaar.com", "seed-demo", false},
		{"production without origin pin", "https://samenklaar.com", "production", "samenklaar", "1", "", "seed-demo", false},
		{"legacy seed remains local", "https://demo.example.test", "production", "samenklaar_demo", "1", "https://demo.example.test", "seed-pros", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("APP_BASE_URL", test.origin)
			t.Setenv("APP_ENV", test.mode)
			t.Setenv("DATABASE_URL", "postgres://test@127.0.0.1/"+test.database)
			t.Setenv("ENABLE_DEMO_SEED", test.enabled)
			t.Setenv("EXPECTED_DEMO_ORIGIN", test.expected)
			if err := validateSeedTarget(test.action); (err == nil) != test.allowed {
				t.Fatalf("allowed=%v error=%v", test.allowed, err)
			}
		})
	}
}
