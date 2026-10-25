package config

import "testing"

func TestReleaseHostingSettingsAreCompleteOrDisabled(t *testing.T) {
	complete := map[string]string{"APP_CONTROL_RELEASE_MANIFEST_FILE": "manifest.json", "APP_CONTROL_RELEASE_PUBLIC_KEY_FILE": "metadata.pub", "APP_CONTROL_RELEASE_ORIGIN": "https://updates.example.org", "APP_CONTROL_RELEASE_CERTIFICATE_SHA256": "public-certificate-fixture"}
	load := func(values map[string]string) (Config, error) {
		return Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	}
	if _, err := load(complete); err != nil {
		t.Fatal(err)
	}
	for missing := range complete {
		partial := map[string]string{}
		for key, value := range complete {
			if key != missing {
				partial[key] = value
			}
		}
		if _, err := load(partial); err == nil {
			t.Fatal("incomplete release settings accepted")
		}
	}
	for _, minimum := range []string{"-1", "x", "2100000001"} {
		values := map[string]string{}
		for key, value := range complete {
			values[key] = value
		}
		values["APP_CONTROL_RELEASE_MIN_SEQUENCE"] = minimum
		if _, err := load(values); err == nil {
			t.Fatal("invalid release sequence floor accepted")
		}
	}
	if _, err := load(map[string]string{"APP_CONTROL_RELEASE_MIN_SEQUENCE": "1"}); err == nil {
		t.Fatal("release sequence floor without hosting settings accepted")
	}
}
