// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package release

// NOTE: normally handled by build tags, but this is a package, so make constant

const (
	// NAME is the name of this application.
	NAME = "circonus-trapcheck"
	// VERSION of the release.
	VERSION = "v0.0.4"
)

// // Info contains release information
// type Info struct {
// 	Name      string
// 	Version   string
// 	Commit    string
// 	BuildDate string
// 	Tag       string
// }

// func info() interface{} {
// 	return &Info{
// 		Name:      NAME,
// 		Version:   VERSION,
// 		Commit:    COMMIT,
// 		BuildDate: DATE,
// 		Tag:       TAG,
// 	}
// }
