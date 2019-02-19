// Copyright 2019 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bind

import (
	"fmt"
	"go/token"
	"path/filepath"
)

// this version uses pybindgen and a generated .go file to do the binding

// for all preambles: 1 = name of package, 2 = full import path of package, 3 = gen command

// GoHandle is the type to use for the Handle map key, go-side
// could be a string for more informative but slower handles
var GoHandle = "int64"
var CGoHandle = "C.longlong"
var PyHandle = "int64_t"

// var GoHandle = "string"
// var CGoHandle = "*C.char"
// var PyHandle = "char*"

const (
	pybGoPreamble = `/*
cgo stubs for package %[2]s.
File is generated by gopy gen. Do not edit.
%[3]s
*/

package main

// #cgo pkg-config: %[4]s
// #define Py_LIMITED_API
// #include <Python.h>
import "C"
import (
	"github.com/goki/gopy/gopyh" // handler
	
	%[2]s
)

func main() {}

// type for the handle -- int64 for speed (can switch to string)
type GoHandle %[5]s
type CGoHandle %[6]s

// boolGoToPy converts a Go bool to python-compatible C.char
func boolGoToPy(b bool) C.char {
	if b {
		return 1
	}
	return 0
}

// boolPyToGo converts a python-compatible C.Char to Go bool
func boolPyToGo(b C.char) bool {
	if b != 0 {
		return true
	}
	return false
}

`

	PyBuildPreamble = `# python build stubs for package %[2]s
# File is generated by gopy gen. Do not edit.
# %[3]s

from pybindgen import retval, param, Module
import sys

mod = Module('_%[1]s')
mod.add_include('"%[1]s_go.h"')
`

	PyWrapPreamble = `%[4]s
# python wrapper for package %[2]s
# This is what you import to use the package.
# File is generated by gopy gen. Do not edit.
# %[3]s


import _%[1]s
import inspect

class GoClass(object):
	"""GoClass is the base class for all GoPy wrapper classes"""
	pass
	
`

	MakefileTemplate = `# Makefile for python interface to Go package %[2]s.
# File is generated by gopy gen. Do not edit.
# %[3]s

GOCMD=go
GOBUILD=$(GOCMD) build
PYTHON=%[4]s
PYTHON_CFG=$(PYTHON)-config
GCC=gcc
LIBEXT=%[5]s

# get the flags used to build python:
CFLAGS = $(shell $(PYTHON_CFG) --cflags)
LDFLAGS = $(shell $(PYTHON_CFG) --ldflags)

all: build

gen:
	%[3]s

build:
	# this will otherwise be built during go build and may be out of date
	- rm %[1]s.c
	# generate %[1]s_go$(LIBEXT) from %[1]s.go -- the cgo wrappers to go functions
	$(GOBUILD) -buildmode=c-shared -ldflags="-s -w" -o %[1]s_go$(LIBEXT) %[1]s.go
	# use pybindgen to build the %[1]s.c file which are the CPython wrappers to cgo wrappers..
	# note: pip install pybindgen to get pybindgen if this fails
	$(PYTHON) build.py
	# build the _%[1]s$(LIBEXT) library that contains the cgo and CPython wrappers
	# generated %[1]s.py python wrapper imports this c-code package
	$(GCC) %[1]s.c -dynamiclib %[1]s_go$(LIBEXT) -o _%[1]s$(LIBEXT) $(CFLAGS) $(LDFLAGS)
`
)

// actually, the weird thing comes from adding symbols to the output, even with dylib!
// ifeq ($(LIBEXT), ".dylib")
// 	# python apparently doesn't recognize .dylib on mac, but .so causes extra weird dylib dir..
// 	- ln -s _%[1]s$(LIBEXT) _%[1]s.so
// endif
//

type pybindGen struct {
	gofile   *printer
	pybuild  *printer
	pywrap   *printer
	makefile *printer

	fset *token.FileSet
	pkg  *Package
	err  ErrorList

	vm     string // python interpreter
	libext string
	lang   int // c-python api version (2,3)
}

func (g *pybindGen) gen() error {
	pkgimport := g.pkg.pkg.Path()
	_, pyonly := filepath.Split(g.vm)
	cmd := fmt.Sprintf("gopy gen -vm=%s %s", pyonly, pkgimport)
	g.genGoPreamble(cmd)
	g.genPyBuildPreamble(cmd)
	g.genPyWrapPreamble(cmd)
	g.genMakefile(cmd)
	g.genAll()
	n := g.pkg.pkg.Name()
	g.pybuild.Printf("\nmod.generate(open('%v.c', 'w'))\n\n", n)
	g.gofile.Printf("\n\n")
	g.pybuild.Printf("\n\n")
	g.pywrap.Printf("\n\n")
	g.makefile.Printf("\n\n")
	return nil
}

func (g *pybindGen) genGoPreamble(cmd string) {
	n := g.pkg.pkg.Name()
	pkgimport := fmt.Sprintf("%q", g.pkg.pkg.Path())
	for pi, _ := range g.pkg.syms.imports {
		pkgimport += fmt.Sprintf("\n\t%q", pi)
	}
	pypath, pyonly := filepath.Split(g.vm)
	pyroot, _ := filepath.Split(filepath.Clean(pypath))
	libcfg := filepath.Join(filepath.Join(filepath.Join(pyroot, "lib"), "pkgconfig"), pyonly+".pc")
	g.gofile.Printf(pybGoPreamble, n, pkgimport, cmd, libcfg, GoHandle, CGoHandle)
	g.gofile.Printf("\n// --- generated code for package: %[1]s below: ---\n\n", n)
}

func (g *pybindGen) genPyBuildPreamble(cmd string) {
	n := g.pkg.pkg.Name()
	pkgimport := g.pkg.pkg.Path()
	g.pybuild.Printf(PyBuildPreamble, n, pkgimport, cmd)
}

func (g *pybindGen) genPyWrapPreamble(cmd string) {
	n := g.pkg.pkg.Name()
	pkgimport := g.pkg.pkg.Path()
	pkgDoc := g.pkg.doc.Doc
	if pkgDoc != "" {
		g.pywrap.Printf(PyWrapPreamble, n, pkgimport, cmd, `"""`+"\n"+pkgDoc+"\n"+`"""`)
	} else {
		g.pywrap.Printf(PyWrapPreamble, n, pkgimport, cmd, "")
	}
}

func (g *pybindGen) genMakefile(cmd string) {
	n := g.pkg.pkg.Name()
	pkgimport := g.pkg.pkg.Path()
	g.makefile.Printf(MakefileTemplate, n, pkgimport, cmd, g.vm, g.libext)
}

func (g *pybindGen) genAll() {

	g.gofile.Printf("\n// ---- Types ---\n")
	g.pywrap.Printf("\n# ---- Types ---\n")
	names := g.pkg.syms.names()
	for _, n := range names {
		sym := g.pkg.syms.sym(n)
		if !sym.isType() {
			continue
		}
		g.genType(sym)
	}

	g.pywrap.Printf("\n\n#---- Constants from Go: Python can only ask that you please don't change these! ---\n")
	for _, c := range g.pkg.consts {
		g.genConst(c)
	}

	g.gofile.Printf("\n\n// ---- Global Variables: can only use functions to access ---\n")
	g.pywrap.Printf("\n\n# ---- Global Variables: can only use functions to access ---\n")
	for _, v := range g.pkg.vars {
		g.genVar(v)
	}

	g.gofile.Printf("\n\n// ---- Interfaces ---\n")
	g.pywrap.Printf("\n\n# ---- Interfaces ---\n")
	for _, ifc := range g.pkg.ifaces {
		g.genInterface(ifc)
	}

	g.gofile.Printf("\n\n// ---- Structs ---\n")
	g.pywrap.Printf("\n\n# ---- Structs ---\n")
	for _, s := range g.pkg.structs {
		g.genStruct(s)
	}

	// note: these are extracted from reg functions that return full
	// type (not pointer -- should do pointer but didn't work yet)
	g.gofile.Printf("\n\n// ---- Constructors ---\n")
	g.pywrap.Printf("\n\n# ---- Constructors ---\n")
	for _, s := range g.pkg.structs {
		for _, ctor := range s.ctors {
			g.genFunc(ctor)
		}
	}

	g.gofile.Printf("\n\n// ---- Functions ---\n")
	g.pywrap.Printf("\n\n# ---- Functions ---\n")
	for _, f := range g.pkg.funcs {
		g.genFunc(f)
	}
}

func (g *pybindGen) genConst(c Const) {
	g.genConstValue(c)
}

func (g *pybindGen) genVar(v Var) {
	g.genVarGetter(v)
	g.genVarSetter(v)
}