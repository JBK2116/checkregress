package models

// Result is the result of a comparison between a filtered legacy outcome and a raw candidate outcome.
type Result struct {
	// HeaderDiffs is the headers diff data.
	HeaderDiffs HeaderDiffs
	// BodyDiffs is the body diff data.
	BodyDiffs BodyDiffs
	// StatusMatch is true if the legacy and candidate status codes match.
	StatusMatch bool
}
