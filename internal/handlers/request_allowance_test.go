package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
)

// requestAllowance is passed to every App.Test call in this package. Without
// a configuration fiber gives a request one second and then returns
// "i/o timeout"; with the five-second limit some helpers used it closes the
// connection after six and returns "test: got empty response". In both cases
// the handler keeps running after the test has moved on. A creation hashes
// its code with Argon2 and can take longer than either limit on a loaded
// machine or under the race detector; the abandoned handler then used the
// logger and the package-level configuration while the next test replaced
// them. The minute only stops a handler that hangs.
var requestAllowance = fiber.TestConfig{Timeout: time.Minute, FailOnTimeout: true}
