package utils

import (
	"testing"
)

func TestGetCommonTimezones(t *testing.T) {
	tzs := GetCommonTimezones()
	found := false
	for _, tz := range tzs {
		if tz.Name == "America/Argentina/Buenos_Aires" {
			found = true
			if tz.Flag != "🇦🇷" {
				t.Errorf("Expected flag 🇦🇷, got %s", tz.Flag)
			}
			if tz.Label != "アルゼンチン (ART)" {
				t.Errorf("Expected label アルゼンチン (ART), got %s", tz.Label)
			}
		}
	}
	if !found {
		t.Error("America/Argentina/Buenos_Aires not found in common timezones")
	}
}

func TestParseTimezone(t *testing.T) {
	loc, err := ParseTimezone("ART")
	if err != nil {
		t.Fatalf("Failed to parse ART: %v", err)
	}
	if loc.String() != "America/Argentina/Buenos_Aires" {
		t.Errorf("Expected America/Argentina/Buenos_Aires, got %s", loc.String())
	}

	loc, err = ParseTimezone("art")
	if err != nil {
		t.Fatalf("Failed to parse art: %v", err)
	}
	if loc.String() != "America/Argentina/Buenos_Aires" {
		t.Errorf("Expected America/Argentina/Buenos_Aires, got %s", loc.String())
	}
}

func TestGetTimezoneLabel(t *testing.T) {
	label := GetTimezoneLabel("ART")
	if label != "アルゼンチン" {
		t.Errorf("Expected アルゼンチン, got %s", label)
	}
	
	label = GetTimezoneLabel("America/Argentina/Buenos_Aires")
	if label != "アルゼンチン" {
		t.Errorf("Expected アルゼンチン, got %s", label)
	}
}

func TestGetTimezoneFlag(t *testing.T) {
	flag := GetTimezoneFlag("ART")
	if flag != "🇦🇷" {
		t.Errorf("Expected 🇦🇷, got %s", flag)
	}
	
	flag = GetTimezoneFlag("America/Argentina/Buenos_Aires")
	if flag != "🇦🇷" {
		t.Errorf("Expected 🇦🇷, got %s", flag)
	}
}
