// Copyright 2015 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package structs

import (
	"strings"
)

type S struct{}

func (S) Init() {}
func (S) Upper(s string) string {
	return strings.ToUpper(s)
}