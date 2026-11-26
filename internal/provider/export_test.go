package provider

import "time"

// SetDomainPoll shortens the wait between domain checks in tests.
func SetDomainPoll(d time.Duration) { domainPoll = d }
