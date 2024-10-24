package catalog

import (
	"context"
	"strings"

	"golang.org/x/time/rate"
)

type wrappedLimiter struct {
	limiter *rate.Limiter
}

func newWrappedLimiter(r rate.Limit, b int) *wrappedLimiter {
	return &wrappedLimiter{
		limiter: rate.NewLimiter(r, b),
	}
}

func (wl *wrappedLimiter) Wait(ctx context.Context) error {
	err := wl.limiter.Wait(ctx)
	if err == nil {
		return nil
	}
	// The rate limiter has some nasty behavior where if it detects the limiter would wait
	// longer than the context's deadline it goes ahead and returns an error. This would
	// make needing to differentiate this error from others all over the place annoying
	// as we wouldn't want to cancel all the various contexts caused by one routine
	// in an errgroup exiting with this error. Therefore we do the more desirable thing
	// and detect the error and wait for the context to be finished.
	if strings.Contains(err.Error(), "Wait(n=1) would exceed context deadline") {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}
