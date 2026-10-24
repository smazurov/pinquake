package obs

import (
	"context"
	"log/slog"
	"sync"
)

type browserCheck struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   Conn
	scenes []Scene
	target Target
}

type browserCheckResult struct {
	check   *browserCheck
	warning string
}

func (c *Controller) cancelBrowserCheck() {
	if c.browserCheck != nil {
		c.browserCheck.cancel()
		c.browserCheck = nil
	}
}

func (c *Controller) queueBrowserCheck(scenes []Scene) {
	ctx, cancel := context.WithCancel(context.Background())
	check := &browserCheck{ctx: ctx, cancel: cancel, conn: c.conn, scenes: scenes, target: c.target}
	c.browserCheck = check
	// Only the controller queues work. Replace any pending scan with the
	// newest snapshot instead of accumulating scans during settings edits.
	select {
	case <-c.browserJobs:
	default:
	}
	c.browserJobs <- check
}

func (c *Controller) runBrowserChecks() {
	defer c.wg.Done()
	for {
		select {
		case <-c.stop:
			return
		case check := <-c.browserJobs:
			if check.ctx.Err() != nil {
				continue
			}
			warning := check.run()
			select {
			case c.browserResults <- browserCheckResult{check: check, warning: warning}:
			case <-check.ctx.Done():
			case <-c.stop:
				return
			}
		}
	}
}

func (check *browserCheck) run() string {
	reader, err := check.conn.OpenInputSettings()
	if err != nil {
		if check.ctx.Err() == nil {
			slog.Warn("Could not check OBS browser settings", "error", err)
		}
		return ""
	}
	closeReader := sync.OnceFunc(func() { _ = reader.Close() })
	stopClose := context.AfterFunc(check.ctx, closeReader)
	defer stopClose()
	defer closeReader()
	return browserWarning(check.scenes, check.target, func(item Item) (map[string]any, error) {
		if check.ctx.Err() != nil {
			return nil, nil
		}
		settings, err := reader.InputSettings(item)
		if check.ctx.Err() != nil {
			return nil, nil
		}
		return settings, err
	})
}
