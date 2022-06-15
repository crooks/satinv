package multire

import (
	"log"
	"regexp"
)

// multiRE
type multiRE struct {
	res []regexp.Regexp
}

// compileRE is an internal function that compiles a string into a Regular Expression
func compileRE(s string) *regexp.Regexp {
	cre, err := regexp.Compile(s)
	if err != nil {
		log.Fatalf("Unable to compile Regex: %s", s)
	}
	return cre
}

// InitRegex contructs a new instance of multiRE and populates it with compiled Regular Expressions.
// The Expressions are based on a provided string slice.
func InitRegex(regexStrings []string) multiRE {
	regexList := new(multiRE)
	for _, s := range regexStrings {
		cre := compileRE(s)
		regexList.res = append(regexList.res, *cre)
	}
	return *regexList
}

// Extend adds a single Regular Expression to an existing multiRE instance
func (mre *multiRE) Extend(s string) {
	cre := compileRE(s)
	mre.res = append(mre.res, *cre)
}

// Match returns true if a given string matches any Regular Expression in a multiRE instance
func (mre *multiRE) Match(s string) bool {
	for _, r := range mre.res {
		if r.Match([]byte(s)) {
			return true
		}
	}
	return false
}
