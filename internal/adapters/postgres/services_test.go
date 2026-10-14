package postgres

import "testing"

func TestServiceAreaCovers(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		request  string
		want     bool
	}{
		{name: "city coverage includes city-qualified neighborhood", provider: "Amsterdam", request: "Amsterdam-Centrum", want: true},
		{name: "city coverage includes slash-qualified neighborhood", provider: "Amsterdam", request: "Amsterdam/Centrum", want: true},
		{name: "neighborhood coverage matches city-qualified request", provider: "Centrum", request: "Amsterdam-Centrum", want: true},
		{name: "city coverage includes Amsterdam postcode", provider: "Amsterdam", request: "1012 AB", want: true},
		{name: "outside market is rejected", provider: "Amsterdam", request: "Rotterdam", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := serviceAreaCovers(test.provider, test.request); got != test.want {
				t.Fatalf("serviceAreaCovers(%q, %q) = %t, want %t", test.provider, test.request, got, test.want)
			}
		})
	}
}
