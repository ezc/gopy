// Copyright 2019 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bind

import (
	"fmt"
	"strings"
)

// genFuncSig generates just the signature for binding
// returns false if function is not suitable for python
// binding (e.g., multiple return values)
func (g *pybindGen) genFuncSig(sym *symbol, fsym Func) bool {
	isMethod := (sym != nil)

	if fsym.sig == nil {
		return false
	}

	sig := fsym.sig
	args := sig.Params()
	res := sig.Results()
	nargs := 0
	nres := len(res)

	// note: this is enforced in creation of Func, in newFuncFrom
	if nres > 2 {
		return false
	}
	if nres == 2 && !fsym.err {
		return false
	}

	goArgs := []string{}
	pyArgs := []string{}
	wpArgs := []string{}
	if isMethod {
		goArgs = append(goArgs, "_handle CGoHandle")
		pyArgs = append(pyArgs, fmt.Sprintf("param('%s', '_handle')", PyHandle))
		wpArgs = append(wpArgs, "self")
	}

	nargs = len(args)
	for i := 0; i < nargs; i++ {
		arg := args[i]
		sarg := g.pkg.syms.symtype(arg.GoType())
		if sarg == nil {
			// panic(fmt.Errorf(
			// 	"gopy: could not find symbol for %q",
			// 	arg.Name(),
			// ))
			return false
		}
		// note: this is enforced in creation of Func, in newFuncFrom
		if sarg.isSignature() { // no support for func args
			return false
		}
		goArgs = append(goArgs, fmt.Sprintf("%s %s", arg.Name(), sarg.cgoname))
		pyArgs = append(pyArgs, fmt.Sprintf("param('%s', '%s')", sarg.cpyname, arg.Name()))
		wpArgs = append(wpArgs, arg.Name())
	}

	switch {
	case isMethod:
		mnm := sym.goname + "_" + fsym.GoName()

		g.gofile.Printf("\n//export %s\n", mnm)
		g.gofile.Printf("func %s(", mnm)

		g.pybuild.Printf("mod.add_function('%s', ", mnm)

		g.pywrap.Printf("def %s(", fsym.GoName())

	default:
		g.gofile.Printf("\n//export %s\n", fsym.GoName())
		g.gofile.Printf("func %s(", fsym.GoName())

		g.pybuild.Printf("mod.add_function('%s', ", fsym.GoName())

		g.pywrap.Printf("def %s(", fsym.GoName())

	}

	goRet := ""
	nres = len(res)
	if nres > 0 {
		ret := res[0]
		sret := g.pkg.syms.symtype(ret.GoType())
		if sret == nil {
			panic(fmt.Errorf(
				"gopy: could not find symbol for %q",
				ret.Name(),
			))
		}
		// todo: check for pointer return type
		g.pybuild.Printf("retval('%s')", sret.cpyname)
		goRet = fmt.Sprintf("%s", sret.cgoname)
	} else {
		g.pybuild.Printf("None")
	}

	if len(goArgs) > 0 {
		gstr := strings.Join(goArgs, ", ")
		g.gofile.Printf("%v) %v", gstr, goRet)

		pstr := strings.Join(pyArgs, ", ")
		g.pybuild.Printf(", [%v])\n", pstr)

		wstr := strings.Join(wpArgs, ", ")
		g.pywrap.Printf("%v)", wstr)

	} else {
		g.gofile.Printf(") %v", goRet)

		g.pybuild.Printf(", [])\n")

		g.pywrap.Printf(")")
	}
	return true
}

func (g *pybindGen) genFunc(o Func) {
	if g.genFuncSig(nil, o) {
		g.genFuncBody(nil, o)
	}
}

func (g *pybindGen) genMethod(s Struct, o Func) {
	if g.genFuncSig(s.sym, o) {
		g.genFuncBody(s.sym, o)
	}
}

func (g *pybindGen) genIfcMethod(ifc Interface, o Func) {
	if g.genFuncSig(ifc.sym, o) {
		g.genFuncBody(ifc.sym, o)
	}
}

func (g *pybindGen) genFuncBody(sym *symbol, fsym Func) {
	isMethod := (sym != nil)
	isIface := false
	symNm := ""
	if isMethod {
		symNm = sym.gofmt()
		isIface = sym.isInterface()
		if !isIface {
			symNm = "*" + symNm
		}
	}

	pkgname := fsym.Package().Name()

	sig := fsym.Signature()
	res := sig.Results()
	args := sig.Params()
	nres := len(res)

	rvIsErr := false // set to true if the main return is an error
	if nres == 1 {
		ret := res[0]
		if isErrorType(ret.GoType()) {
			rvIsErr = true
		}
	}

	g.pywrap.Printf(":\n")
	g.pywrap.Indent()

	g.gofile.Printf(" {\n")
	g.gofile.Indent()
	if isMethod {
		g.gofile.Printf(
			`vifc, err := gopyh.VarHand.VarFmHandleTry((gopyh.CGoHandle)(_handle), "%s")
if err != nil {
`, symNm)
		g.gofile.Indent()
		if nres > 0 {
			ret := res[0]
			if ret.sym.zval == "" {
				fmt.Printf("gopy: programmer error: empty zval zero value in symbol: %v\n", ret.sym)
			}
			if ret.sym.go2py != "" {
				g.gofile.Printf("return %s(%s)\n", ret.sym.go2py, ret.sym.zval)
			} else {
				g.gofile.Printf("return %s\n", ret.sym.zval)
			}
		} else {
			g.gofile.Printf("return\n")
		}
		g.gofile.Outdent()
		g.gofile.Printf("}\n")
	} else if rvIsErr {
		g.gofile.Printf("var err error\n")
	}

	// pywrap output
	mnm := fsym.GoName()
	if isMethod {
		mnm = sym.goname + "_" + mnm
	}
	rvHasHandle := false
	if nres > 0 {
		ret := res[0]
		if !rvIsErr && ret.sym.hasHandle() {
			rvHasHandle = true
			g.pywrap.Printf("return %s(handle=_%s.%s(", ret.sym.pyname, pkgname, mnm)
		} else {
			g.pywrap.Printf("return _%s.%s(", pkgname, mnm)
		}
	} else {
		g.pywrap.Printf("_%s.%s(", pkgname, mnm)
	}

	callArgs := []string{}
	wrapArgs := []string{}
	if isMethod {
		wrapArgs = append(wrapArgs, "self.handle")
	}
	for _, arg := range args {
		na := ""
		if arg.sym.py2go != "" {
			na = fmt.Sprintf("%s(%s)%s", arg.sym.py2go, arg.Name(), arg.sym.py2goParenEx)
		} else {
			na = arg.Name()
		}
		callArgs = append(callArgs, na)
		if arg.sym.hasHandle() {
			wrapArgs = append(wrapArgs, fmt.Sprintf("%s.handle", arg.Name()))
		} else {
			wrapArgs = append(wrapArgs, arg.Name())
		}
	}

	hasRetCvt := false
	hasAddrOfTmp := false
	if nres > 0 {
		ret := res[0]
		if rvIsErr {
			g.gofile.Printf("err = ")
		} else if nres == 2 {
			g.gofile.Printf("cret, err := ")
		} else if ret.sym.hasHandle() && !ret.sym.isPtrOrIface() {
			hasAddrOfTmp = true
			g.gofile.Printf("cret := ")
		} else {
			if ret.sym.go2py != "" {
				hasRetCvt = true
				g.gofile.Printf("return %s(", ret.sym.go2py)
			} else {
				g.gofile.Printf("return ")
			}
		}
	}
	g.pywrap.Printf("%s)", strings.Join(wrapArgs, ", "))
	if rvHasHandle {
		g.pywrap.Printf(")")
	}

	if isMethod {
		g.gofile.Printf("vifc.(%s).%s(%s)", symNm, fsym.GoName(), strings.Join(callArgs, ", "))
	} else {
		g.gofile.Printf("%s(%s)", fsym.GoFmt(), strings.Join(callArgs, ", "))
	}
	if hasRetCvt {
		g.gofile.Printf(")")
	}
	if rvIsErr || nres == 2 {
		g.gofile.Printf("\n")
		g.gofile.Printf("if err != nil {\n")
		g.gofile.Indent()
		g.gofile.Printf("C.PyErr_SetString(C.PyExc_RuntimeError, C.CString(err.Error()))\n")
		if rvIsErr {
			g.gofile.Printf("return C.CString(err.Error())")
		}
		g.gofile.Outdent()
		g.gofile.Printf("}\n")
		if rvIsErr {
			g.gofile.Printf("return C.CString(\"\")")
		} else {
			ret := res[0]
			if ret.sym.go2py != "" {
				if ret.sym.hasHandle() && !ret.sym.isPtrOrIface() {
					g.gofile.Printf("return %s(&cret)", ret.sym.go2py)
				} else {
					g.gofile.Printf("return %s(cret)", ret.sym.go2py)
				}
			} else {
				g.gofile.Printf("return cret")
			}
		}
	} else if hasAddrOfTmp {
		ret := res[0]
		g.gofile.Printf("\nreturn %s(&cret)", ret.sym.go2py)
	}
	g.gofile.Printf("\n")
	g.gofile.Outdent()
	g.gofile.Printf("}\n")

	g.pywrap.Printf("\n")
	g.pywrap.Outdent()
	g.pywrap.Printf("\n")
}