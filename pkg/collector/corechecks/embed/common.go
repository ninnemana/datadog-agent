package embed

import (
	"os/exec"
	"time"

	"github.com/ninnemana/datadog-agent/pkg/collector/check"
	"github.com/ninnemana/datadog-agent/pkg/util"
)

const defaultRetryDuration = 5 * time.Second
const defaultRetries = 3

// retryExitError converts `exec.ExitError`s to `check.RetryableError`s, so that checks using this
// are retried.
// embed checks must use this from their `Run` method when exit errors need to be retried.
func retryExitError(err error) error {
	switch err.(type) {
	case *exec.ExitError: // error type returned when the process exits with non-zero status
		return check.RetryableError{Err: err}
	default:
		return err
	}
}

func getHostname() string {
	hostname, _ := util.GetHostname()
	return hostname
}
