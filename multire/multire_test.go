package multire

import (
	"testing"
)

func TestRE(t *testing.T) {
	var regexStrings []string
	regexStrings = append(regexStrings, "^foo")
	mre := InitRegex(regexStrings)
	if mre.Match("barfoo") {
		t.Fatal("string \"barfoo\" shouldn't match the test Regex")
	}
	mre.Extend("^bar")
	if !mre.Match("barfoo") {
		t.Fatal("string \"barfoo\" should match the test Regex")
	}
}
