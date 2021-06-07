// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import "log"

// Logger is a generic logging interface
type Logger interface {
	Printf(fmt string, v ...interface{})
	Debugf(fmt string, v ...interface{})
	Infof(fmt string, v ...interface{})
	Warnf(fmt string, v ...interface{})
	Errorf(fmt string, v ...interface{})
}

// LogWrapper is a wrapper around Go's log.Logger
type LogWrapper struct {
	Log   *log.Logger
	Debug bool
}

func (lw *LogWrapper) Printf(fmt string, v ...interface{}) {
	lw.Log.Printf(fmt, v...)
}
func (lw *LogWrapper) Debugf(fmt string, v ...interface{}) {
	if lw.Debug {
		lw.Log.Printf("[debug] "+fmt, v...)
	}
}
func (lw *LogWrapper) Infof(fmt string, v ...interface{}) {
	lw.Log.Printf("[info] "+fmt, v...)
}
func (lw *LogWrapper) Warnf(fmt string, v ...interface{}) {
	lw.Log.Printf("[warn] "+fmt, v...)
}
func (lw *LogWrapper) Errorf(fmt string, v ...interface{}) {
	lw.Log.Printf("[error] "+fmt, v...)
}
