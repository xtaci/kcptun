package smux

import (
	"time"

	"github.com/pion/transport/v5/deadline"
)

func freeDeadline(d *deadline.Deadline) {
	d.Set(time.Time{})
}
