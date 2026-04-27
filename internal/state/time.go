package state

import "time"

func timeNowSec() int64 { return time.Now().Unix() }
