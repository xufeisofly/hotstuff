package consensus

import "time"

type ViewDuration interface {
	ViewStarted()
	ViewSucceeded()
	ViewTimeout()
	GetDuration() time.Duration
}
